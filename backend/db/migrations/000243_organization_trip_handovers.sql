-- EMP-TRIP-012. Driver handover during a trip (B-BR-EMP-394..398,
-- EDR-254..256). Additive: one insert-only, forced-RLS handover fact per
-- revision moves the responsibility for an open (IN_PROGRESS or PAUSED) trip
-- from its current responsible member to another member at the server's
-- instant; no trip column changes (responsibility is derived from the facts)
-- and no earlier fact is rewritten. A handover is a trip revision. Number
-- 000243: the reserved 000234 would sort before the 000233 and 000237 bodies
-- that this migration extends, so it stays unused. Redefined from their
-- latest bodies, keeping every other line: 000233 transition,
-- held-transition and finish sources; 000235 org_trip_revision, logbook,
-- held-logbook, review-entry and closure sources; 000236 held source and
-- offboarding guard; 000229 held-submission source; 000230 incident report
-- source (a report inside a trip).
-- Handover keys reference the trip, memberships, employee records and the
-- audit event only: no asset, unit or cost-centre key, so no usage source
-- changes; org_trip_source, org_trip_complete and the 000237 resolution
-- guards are unchanged.
CREATE TABLE org_trip_handovers(
 org_id UUID NOT NULL,
 trip_id UUID NOT NULL,
 revision BIGINT NOT NULL CHECK(revision BETWEEN 2 AND 1000000),
 operation_id UUID NOT NULL CHECK(operation_id<>'00000000-0000-0000-0000-000000000000'::uuid),
 request_hash TEXT NOT NULL CHECK(request_hash~'^[a-f0-9]{64}$'),
 from_membership_id UUID NOT NULL,
 from_employee_id UUID NOT NULL,
 to_membership_id UUID NOT NULL,
 to_employee_id UUID NOT NULL,
 recorder_membership_id UUID NOT NULL,
 recorder_employee_id UUID NOT NULL,
 reason TEXT NOT NULL CHECK(org_asset_text_valid(reason,280,true)),
 handed_over_at TIMESTAMPTZ NOT NULL DEFAULT clock_timestamp(),
 PRIMARY KEY(org_id,trip_id,revision),
 CONSTRAINT org_trip_handovers_operation_key UNIQUE(org_id,operation_id),
 CONSTRAINT org_trip_handovers_trip_fkey FOREIGN KEY(org_id,trip_id) REFERENCES org_trips(org_id,id),
 CONSTRAINT org_trip_handovers_from_fkey FOREIGN KEY(org_id,from_membership_id) REFERENCES org_memberships(org_id,id),
 CONSTRAINT org_trip_handovers_from_employee_fkey FOREIGN KEY(org_id,from_employee_id) REFERENCES org_employee_identities(org_id,id),
 CONSTRAINT org_trip_handovers_to_fkey FOREIGN KEY(org_id,to_membership_id) REFERENCES org_memberships(org_id,id),
 CONSTRAINT org_trip_handovers_to_employee_fkey FOREIGN KEY(org_id,to_employee_id) REFERENCES org_employee_identities(org_id,id),
 CONSTRAINT org_trip_handovers_recorder_fkey FOREIGN KEY(org_id,recorder_membership_id) REFERENCES org_memberships(org_id,id),
 CONSTRAINT org_trip_handovers_recorder_employee_fkey FOREIGN KEY(org_id,recorder_employee_id) REFERENCES org_employee_identities(org_id,id),
 CONSTRAINT org_trip_handovers_audit_fkey FOREIGN KEY(org_id,operation_id) REFERENCES org_audit_events(org_id,operation_id) DEFERRABLE INITIALLY DEFERRED,
 -- B-BR-EMP-397: the giving and receiving members differ; the recorder is neither.
 CONSTRAINT org_trip_handovers_parties CHECK(from_membership_id<>to_membership_id AND recorder_membership_id<>from_membership_id AND recorder_membership_id<>to_membership_id),
 CHECK(isfinite(handed_over_at) AND handed_over_at>'0001-01-01 00:00:00+00'::timestamptz AND handed_over_at<'10000-01-01 00:00:00+00'::timestamptz)
);
CREATE INDEX org_trip_handovers_receiver ON org_trip_handovers(org_id,to_membership_id);
CREATE INDEX org_trip_handovers_recorded ON org_trip_handovers(org_id,handed_over_at,operation_id);

-- B-BR-EMP-395: the member responsible for a trip at an instant (NULL: now):
-- the receiver of the latest handover recorded at or before it, else the
-- member who started the trip, with that member's employee record.
CREATE FUNCTION org_trip_responsible(p_org UUID,p_trip UUID,p_at TIMESTAMPTZ,p_sub SMALLINT) RETURNS TABLE(membership_id UUID,employee_id UUID)
 LANGUAGE SQL STABLE SET search_path=pg_catalog AS $$
 SELECT COALESCE(h.to_membership_id,t.author_membership_id),COALESCE(h.to_employee_id,t.author_employee_id)
 FROM public.org_trips t LEFT JOIN LATERAL(SELECT x.to_membership_id,x.to_employee_id FROM public.org_trip_handovers x
  WHERE x.org_id=t.org_id AND x.trip_id=t.id AND(p_at IS NULL OR(x.handed_over_at,0::smallint)<=(p_at,COALESCE(p_sub,0::smallint)))
  ORDER BY x.revision DESC LIMIT 1) h ON true
 WHERE t.org_id=p_org AND t.id=p_trip
 $$;
-- The current responsible membership as a scalar (offboarding open work and
-- the receiver's own open trips read it; with its SET clause it is never
-- inlined, so a read that finds no open trip touches no handover row).
CREATE FUNCTION org_trip_responsible_member(p_org UUID,p_trip UUID) RETURNS UUID LANGUAGE SQL STABLE SET search_path=pg_catalog AS $$
 SELECT r.membership_id FROM public.org_trip_responsible(p_org,p_trip,NULL,NULL) r
 $$;

-- B-BR-EMP-394/396/397: under the organization root and trip row locks (every
-- lifecycle, finish and logbook trigger locks the same rows in this order),
-- an open trip, the next revision, the giving member as the current
-- responsible member, an active receiver and recorder with active unmerged
-- employee records, the separation by account, membership and canonical
-- employee, and a server instant not before the previous handover.
CREATE FUNCTION org_trip_handover_source() RETURNS trigger LANGUAGE plpgsql SET search_path=pg_catalog AS $$
DECLARE t public.org_trips%ROWTYPE;rm UUID;re UUID;previous TIMESTAMPTZ;from_person UUID;to_person UUID;recorder_person UUID;
BEGIN
 IF NEW.org_id IS DISTINCT FROM NULLIF(current_setting('anpfuel.org_id',true),'')::uuid THEN RAISE EXCEPTION USING ERRCODE='42501',MESSAGE='organization scope denied';END IF;
 PERFORM 1 FROM public.organizations WHERE org_id=NEW.org_id AND has_snapshot FOR UPDATE;
 IF NOT FOUND THEN RAISE EXCEPTION USING ERRCODE='23503',MESSAGE='private reference not found';END IF;
 SELECT * INTO t FROM public.org_trips WHERE org_id=NEW.org_id AND id=NEW.trip_id FOR UPDATE;
 IF NOT FOUND THEN RAISE EXCEPTION USING ERRCODE='23503',MESSAGE='private reference not found';END IF;
 IF t.state NOT IN('IN_PROGRESS','PAUSED') THEN RAISE EXCEPTION USING ERRCODE='23514',MESSAGE='trip already closed';END IF;
 IF NEW.revision<>public.org_trip_revision(NEW.org_id,NEW.trip_id)+1 THEN RAISE EXCEPTION USING ERRCODE='23514',MESSAGE='trip handover stale';END IF;
 SELECT r.membership_id,r.employee_id INTO rm,re FROM public.org_trip_responsible(NEW.org_id,NEW.trip_id,NULL,NULL) r;
 IF NEW.from_membership_id IS DISTINCT FROM rm OR NEW.from_employee_id IS DISTINCT FROM re THEN RAISE EXCEPTION USING ERRCODE='23514',MESSAGE='trip handover stale';END IF;
 IF NOT EXISTS(SELECT 1 FROM public.org_memberships m JOIN public.org_employee_identities e ON(e.org_id,e.id)=(m.org_id,m.employee_id)
  WHERE m.org_id=NEW.org_id AND m.id=NEW.to_membership_id AND m.employee_id=NEW.to_employee_id AND m.state='ACTIVE' AND e.state='ACTIVE' AND e.merged_into IS NULL)
 THEN RAISE EXCEPTION USING ERRCODE='23514',MESSAGE='trip handover receiver unavailable';END IF;
 IF NOT EXISTS(SELECT 1 FROM public.org_memberships m JOIN public.org_employee_identities e ON(e.org_id,e.id)=(m.org_id,m.employee_id)
  WHERE m.org_id=NEW.org_id AND m.id=NEW.recorder_membership_id AND m.employee_id=NEW.recorder_employee_id AND m.state='ACTIVE' AND e.state='ACTIVE' AND e.merged_into IS NULL)
 THEN RAISE EXCEPTION USING ERRCODE='23514',MESSAGE='trip handover recorder unavailable';END IF;
 from_person=public.org_sync_review_employee_canonical(NEW.org_id,NEW.from_employee_id);
 to_person=public.org_sync_review_employee_canonical(NEW.org_id,NEW.to_employee_id);
 recorder_person=public.org_sync_review_employee_canonical(NEW.org_id,NEW.recorder_employee_id);
 IF from_person IS NULL OR to_person IS NULL OR recorder_person IS NULL OR to_person=from_person
 THEN RAISE EXCEPTION USING ERRCODE='23514',MESSAGE='trip handover receiver unavailable';END IF;
 IF recorder_person IN(from_person,to_person) OR EXISTS(SELECT 1 FROM public.org_memberships r JOIN public.org_memberships p ON(p.org_id,p.account_id)=(r.org_id,r.account_id)
  WHERE r.org_id=NEW.org_id AND r.id=NEW.recorder_membership_id AND p.id IN(NEW.from_membership_id,NEW.to_membership_id))
 THEN RAISE EXCEPTION USING ERRCODE='23514',MESSAGE='trip self handover refused';END IF;
 NEW.handed_over_at=clock_timestamp();
 SELECT max(h.handed_over_at) INTO previous FROM public.org_trip_handovers h WHERE h.org_id=NEW.org_id AND h.trip_id=NEW.trip_id;
 IF previous IS NOT NULL AND NEW.handed_over_at<previous THEN RAISE EXCEPTION USING ERRCODE='23514',MESSAGE='trip handover instant unavailable';END IF;
 RETURN NEW;
END $$;
CREATE TRIGGER org_trip_handover_source BEFORE INSERT ON org_trip_handovers FOR EACH ROW EXECUTE FUNCTION org_trip_handover_source();

-- At commit a handover is published only with its trip.trip.handed_over
-- audit event by the recorder's account and digest.
CREATE FUNCTION org_trip_handover_published() RETURNS trigger LANGUAGE plpgsql SET search_path=pg_catalog AS $$
DECLARE prior_org TEXT;
BEGIN
 prior_org=current_setting('anpfuel.org_id',true);PERFORM set_config('anpfuel.org_id',NEW.org_id::text,true);
 IF NOT EXISTS(SELECT 1 FROM public.org_audit_events a WHERE a.org_id=NEW.org_id AND a.operation_id=NEW.operation_id AND a.target_type='trip' AND a.target_id=NEW.trip_id
  AND a.action='trip.trip.handed_over' AND a.request_hash=NEW.request_hash
  AND a.actor_id=(SELECT m.account_id FROM public.org_memberships m WHERE m.org_id=NEW.org_id AND m.id=NEW.recorder_membership_id))
 THEN RAISE EXCEPTION USING ERRCODE='23514',MESSAGE='trip handover publication unavailable';END IF;
 PERFORM set_config('anpfuel.org_id',COALESCE(prior_org,''),true);RETURN NULL;
END $$;
CREATE CONSTRAINT TRIGGER org_trip_handover_published AFTER INSERT ON org_trip_handovers DEFERRABLE INITIALLY DEFERRED FOR EACH ROW EXECUTE FUNCTION org_trip_handover_published();

ALTER TABLE org_trip_handovers ENABLE ROW LEVEL SECURITY;ALTER TABLE org_trip_handovers FORCE ROW LEVEL SECURITY;
CREATE POLICY org_trip_handovers_scope ON org_trip_handovers USING(org_id=NULLIF(current_setting('anpfuel.org_id',true),'')::uuid) WITH CHECK(org_id=NULLIF(current_setting('anpfuel.org_id',true),'')::uuid);
CREATE TRIGGER org_trip_handovers_immutable BEFORE UPDATE OR DELETE OR TRUNCATE ON org_trip_handovers FOR EACH STATEMENT EXECUTE FUNCTION org_trip_keep_history();
REVOKE ALL ON org_trip_handovers FROM PUBLIC;


-- The 235 trip revision with each handover counted (B-BR-EMP-396).
CREATE OR REPLACE FUNCTION org_trip_revision(p_org UUID,p_trip UUID) RETURNS BIGINT LANGUAGE SQL STABLE SET search_path=pg_catalog AS $$
 SELECT 1+(SELECT count(*) FROM public.org_trip_transitions x WHERE x.org_id=p_org AND x.trip_id=p_trip)
  +(SELECT count(*) FROM public.org_trip_finishes f WHERE f.org_id=p_org AND f.trip_id=p_trip)
  -- EMP-TRIP-014: the closure ends the trip at the next revision like a finish.
  +(SELECT count(*) FROM public.org_trip_closures c WHERE c.org_id=p_org AND c.trip_id=p_trip)
  -- EMP-TRIP-012: each handover is one revision of the trip.
  +(SELECT count(*) FROM public.org_trip_handovers h WHERE h.org_id=p_org AND h.trip_id=p_trip)
 $$;

-- The 233 lifecycle source: the command's author is the current responsible member.
CREATE OR REPLACE FUNCTION org_trip_transition_source() RETURNS trigger LANGUAGE plpgsql SET search_path=pg_catalog AS $$
DECLARE t public.org_trips%ROWTYPE;rm UUID;re UUID;
BEGIN
 IF NEW.org_id IS DISTINCT FROM NULLIF(current_setting('anpfuel.org_id',true),'')::uuid THEN RAISE EXCEPTION USING ERRCODE='42501',MESSAGE='organization scope denied';END IF;
 PERFORM 1 FROM public.organizations WHERE org_id=NEW.org_id AND has_snapshot FOR UPDATE;
 IF NOT FOUND THEN RAISE EXCEPTION USING ERRCODE='23503',MESSAGE='private reference not found';END IF;
 SELECT * INTO t FROM public.org_trips WHERE org_id=NEW.org_id AND id=NEW.trip_id FOR UPDATE;
 IF NOT FOUND THEN RAISE EXCEPTION USING ERRCODE='23503',MESSAGE='private reference not found';END IF;
 -- EMP-TRIP-012: the current responsible member and that member's employee.
 SELECT r.membership_id,r.employee_id INTO rm,re FROM public.org_trip_responsible(NEW.org_id,NEW.trip_id,NULL,NULL) r;
 IF NOT public.org_trip_operation_valid(NEW.org_id,NEW.operation_id,'trip.'||lower(NEW.command),NEW.trip_id,NEW.payload_digest,NEW.author_membership_id,NEW.enrollment_id,re)
 OR NEW.author_membership_id<>rm OR NEW.author_employee_id<>re
 THEN RAISE EXCEPTION USING ERRCODE='23514',MESSAGE='trip transition source unavailable';END IF;
 IF NEW.from_state<>t.state OR NEW.to_state IS DISTINCT FROM public.org_trip_lifecycle_next(t.state,NEW.command)
 OR NEW.revision<>public.org_trip_revision(NEW.org_id,NEW.trip_id)+1
 THEN RAISE EXCEPTION USING ERRCODE='23514',MESSAGE='trip transition stale';END IF;
 IF NOT EXISTS(SELECT 1 FROM pg_timezone_names WHERE name=NEW.entry_zone) OR(NEW.occurred_at,NEW.occurred_submicro)<(t.occurred_at,t.occurred_submicro)
 THEN RAISE EXCEPTION USING ERRCODE='23514',MESSAGE='invalid trip transition';END IF;
 NEW.received_at=clock_timestamp();RETURN NEW;
END $$;

-- The 233 held lifecycle source: held only for the current responsible member.
CREATE OR REPLACE FUNCTION org_trip_held_transition_source() RETURNS trigger LANGUAGE plpgsql SET search_path=pg_catalog AS $$
DECLARE i public.org_sync_review_items%ROWTYPE;t public.org_trips%ROWTYPE;rm UUID;re UUID;
BEGIN
 IF NEW.org_id IS DISTINCT FROM NULLIF(current_setting('anpfuel.org_id',true),'')::uuid THEN RAISE EXCEPTION USING ERRCODE='42501',MESSAGE='organization scope denied';END IF;
 PERFORM 1 FROM public.organizations WHERE org_id=NEW.org_id AND has_snapshot FOR UPDATE;
 SELECT * INTO i FROM public.org_sync_review_items WHERE org_id=NEW.org_id AND id=NEW.review_item_id AND operation_id=NEW.operation_id;
 IF NOT FOUND THEN RAISE EXCEPTION USING ERRCODE='23514',MESSAGE='held trip transition intent unavailable';END IF;
 SELECT * INTO t FROM public.org_trips WHERE org_id=NEW.org_id AND id=NEW.trip_id FOR UPDATE;
 IF NOT FOUND THEN RAISE EXCEPTION USING ERRCODE='23503',MESSAGE='private reference not found';END IF;
 -- EMP-TRIP-012: the current responsible member and that member's employee.
 SELECT r.membership_id,r.employee_id INTO rm,re FROM public.org_trip_responsible(NEW.org_id,NEW.trip_id,NULL,NULL) r;
 IF i.operation_type<>'trip.'||lower(NEW.command) OR i.operation_schema<>1 OR i.code NOT IN('TRIP_ALREADY_CLOSED','STALE_REVISION')
 OR NOT public.org_trip_operation_valid(NEW.org_id,NEW.operation_id,i.operation_type,NEW.trip_id,i.payload_digest,rm,NULL,re)
 OR i.author_employee_id<>re OR NOT EXISTS(SELECT 1 FROM pg_timezone_names WHERE name=NEW.entry_zone)
 OR EXISTS(SELECT 1 FROM public.org_trip_transitions x WHERE x.org_id=NEW.org_id AND x.operation_id=NEW.operation_id)
 OR(i.code='TRIP_ALREADY_CLOSED' AND t.state NOT IN('COMPLETED','CANCELLED'))
 OR(i.code='STALE_REVISION' AND(t.state NOT IN('IN_PROGRESS','PAUSED')
  OR(NEW.expected_revision=public.org_trip_revision(NEW.org_id,NEW.trip_id) AND public.org_trip_lifecycle_next(t.state,NEW.command) IS NOT NULL)))
 THEN RAISE EXCEPTION USING ERRCODE='23514',MESSAGE='held trip transition source unavailable';END IF;
 RETURN NEW;
END $$;

-- The 233 finish source: the finish's author is the current responsible member.
CREATE OR REPLACE FUNCTION org_trip_finish_source() RETURNS trigger LANGUAGE plpgsql SET search_path=pg_catalog AS $$
DECLARE t public.org_trips%ROWTYPE;rm UUID;re UUID;
BEGIN
 IF NEW.org_id IS DISTINCT FROM NULLIF(current_setting('anpfuel.org_id',true),'')::uuid THEN RAISE EXCEPTION USING ERRCODE='42501',MESSAGE='organization scope denied';END IF;
 PERFORM 1 FROM public.organizations WHERE org_id=NEW.org_id AND has_snapshot FOR UPDATE;
 SELECT * INTO t FROM public.org_trips WHERE org_id=NEW.org_id AND id=NEW.trip_id FOR UPDATE;
 IF NOT FOUND THEN RAISE EXCEPTION USING ERRCODE='23503',MESSAGE='private reference not found';END IF;
 -- EMP-TRIP-012: the current responsible member and that member's employee.
 SELECT r.membership_id,r.employee_id INTO rm,re FROM public.org_trip_responsible(NEW.org_id,NEW.trip_id,NULL,NULL) r;
 IF NOT public.org_trip_operation_valid(NEW.org_id,NEW.operation_id,'trip.finish',NEW.trip_id,NEW.payload_digest,NEW.author_membership_id,NEW.enrollment_id,re)
 OR NEW.author_membership_id<>rm THEN RAISE EXCEPTION USING ERRCODE='23514',MESSAGE='trip finish source unavailable';END IF;
 IF t.state NOT IN('IN_PROGRESS','PAUSED') THEN RAISE EXCEPTION USING ERRCODE='23514',MESSAGE='trip already closed';END IF;
 IF(NEW.occurred_at,NEW.occurred_submicro)<(t.occurred_at,t.occurred_submicro) OR NOT EXISTS(SELECT 1 FROM pg_timezone_names WHERE name=NEW.entry_zone)
 OR NEW.end_day<>(NEW.occurred_at AT TIME ZONE t.reporting_zone)::date THEN RAISE EXCEPTION USING ERRCODE='23514',MESSAGE='invalid trip finish';END IF;
 IF NEW.meter_id IS NOT NULL AND NOT EXISTS(SELECT 1 FROM public.org_meters m WHERE m.org_id=NEW.org_id AND m.id=NEW.meter_id AND m.asset_id=t.asset_id)
 THEN RAISE EXCEPTION USING ERRCODE='23514',MESSAGE='trip finish reading unavailable';END IF;
 NEW.received_at=clock_timestamp();RETURN NEW;
END $$;

-- The 236 held source: a late finish and a held finish of a closed trip come
-- from the current responsible member.
CREATE OR REPLACE FUNCTION org_trip_held_source() RETURNS trigger LANGUAGE plpgsql SET search_path=pg_catalog AS $$
DECLARE i public.org_sync_review_items%ROWTYPE;t public.org_trips%ROWTYPE;o public.org_sync_operations%ROWTYPE;late_end TIMESTAMPTZ;rm UUID;re UUID;
BEGIN
 IF NEW.org_id IS DISTINCT FROM NULLIF(current_setting('anpfuel.org_id',true),'')::uuid THEN RAISE EXCEPTION USING ERRCODE='42501',MESSAGE='organization scope denied';END IF;
 PERFORM 1 FROM public.organizations WHERE org_id=NEW.org_id AND has_snapshot FOR UPDATE;
 SELECT * INTO i FROM public.org_sync_review_items WHERE org_id=NEW.org_id AND id=NEW.review_item_id AND operation_id=NEW.operation_id;
 IF NOT FOUND THEN RAISE EXCEPTION USING ERRCODE='23514',MESSAGE='held trip intent unavailable';END IF;
 IF NOT EXISTS(SELECT 1 FROM pg_timezone_names WHERE name=NEW.entry_zone) THEN RAISE EXCEPTION USING ERRCODE='23514',MESSAGE='invalid held trip declaration';END IF;
 IF TG_TABLE_NAME='org_trip_held_starts' THEN
  IF i.operation_type<>'trip.start' OR i.operation_schema<>1 OR i.code NOT IN('ASSET_BUSY','ASSET_UNAVAILABLE','PERMISSION_CHANGED')
  OR NOT public.org_trip_operation_valid(NEW.org_id,NEW.operation_id,'trip.start',NEW.trip_id,i.payload_digest,NULL,NULL,i.author_employee_id)
  THEN RAISE EXCEPTION USING ERRCODE='23514',MESSAGE='held trip source unavailable';END IF;
  IF EXISTS(SELECT 1 FROM public.org_trips WHERE org_id=NEW.org_id AND id=NEW.trip_id) THEN RAISE EXCEPTION USING ERRCODE='23505',CONSTRAINT='org_trips_pkey',MESSAGE='trip identity unavailable';END IF;
  PERFORM 1 FROM public.org_assets WHERE org_id=NEW.org_id AND id=NEW.asset_id FOR UPDATE;
  IF NOT FOUND THEN RAISE EXCEPTION USING ERRCODE='23503',MESSAGE='private reference not found';END IF;
  -- ASSET_BUSY needs a trip in progress or one holding the declared instant;
  -- ASSET_UNAVAILABLE (lifecycle, block, reservation, maintenance, missing
  -- assignment) and PERMISSION_CHANGED are decided by their owning ports in
  -- this transaction and are not re-derived here.
  -- EMP-TRIP-011: a paused trip holds the asset; a cancelled one until its cancellation.
  IF i.code='ASSET_BUSY' AND NOT EXISTS(SELECT 1 FROM public.org_trips t2 LEFT JOIN public.org_trip_finishes f2 ON(f2.org_id,f2.trip_id)=(t2.org_id,t2.id)
   LEFT JOIN public.org_trip_transitions c2 ON(c2.org_id,c2.trip_id,c2.command)=(t2.org_id,t2.id,'CANCEL')
   WHERE t2.org_id=NEW.org_id AND t2.asset_id=NEW.asset_id AND(t2.state IN('IN_PROGRESS','PAUSED') OR((t2.occurred_at,t2.occurred_submicro)<=(NEW.occurred_at,NEW.occurred_submicro)
   AND(COALESCE(f2.occurred_at,c2.occurred_at),COALESCE(f2.occurred_submicro,c2.occurred_submicro))>(NEW.occurred_at,NEW.occurred_submicro))))
  THEN RAISE EXCEPTION USING ERRCODE='23514',MESSAGE='held trip condition unavailable';END IF;
 ELSE
  SELECT * INTO t FROM public.org_trips WHERE org_id=NEW.org_id AND id=NEW.trip_id FOR UPDATE;
  IF NOT FOUND THEN RAISE EXCEPTION USING ERRCODE='23503',MESSAGE='private reference not found';END IF;
  -- EMP-TRIP-018 (OFFL-009): a finish of the trip's author declared, or
  -- claimed, before the end of its exact ended period, received late.
  IF i.code='ACCESS_ENDED_BEFORE_RECEIPT' THEN
   -- EMP-TRIP-012: a finish comes from the current responsible member.
   SELECT r.membership_id,r.employee_id INTO rm,re FROM public.org_trip_responsible(NEW.org_id,NEW.trip_id,NULL,NULL) r;
   SELECT * INTO o FROM public.org_sync_operations WHERE org_id=NEW.org_id AND operation_id=NEW.operation_id;
   IF FOUND THEN
    SELECT l.ended_at INTO late_end FROM public.org_sync_late_end(o.org_id,o.membership_id,o.enrollment_id,o.owner_account_id) l;
   END IF;
   IF i.operation_type<>'trip.finish' OR i.operation_schema<>1 OR o.operation_id IS NULL OR late_end IS NULL
   OR o.operation_type<>'trip.finish' OR o.operation_schema<>1 OR o.entity_type<>'trip' OR o.entity_id<>NEW.trip_id OR o.payload_digest<>i.payload_digest
   OR o.membership_id IS DISTINCT FROM rm OR i.author_employee_id IS DISTINCT FROM re
   OR NOT(o.claimed_at<late_end OR NEW.occurred_at<late_end)
   OR EXISTS(SELECT 1 FROM public.org_trip_finishes f WHERE f.org_id=NEW.org_id AND f.operation_id=NEW.operation_id)
   OR(NEW.occurred_at,NEW.occurred_submicro)<(t.occurred_at,t.occurred_submicro)
   OR(NEW.meter_id IS NOT NULL AND NOT EXISTS(SELECT 1 FROM public.org_meters m WHERE m.org_id=NEW.org_id AND m.id=NEW.meter_id AND m.asset_id=t.asset_id))
   THEN RAISE EXCEPTION USING ERRCODE='23514',MESSAGE='held trip finish source unavailable';END IF;
   RETURN NEW;
  END IF;
  -- EMP-TRIP-012: the current responsible member and that member's employee.
  SELECT r.membership_id,r.employee_id INTO rm,re FROM public.org_trip_responsible(NEW.org_id,NEW.trip_id,NULL,NULL) r;
  IF i.operation_type<>'trip.finish' OR i.operation_schema<>1 OR i.code<>'TRIP_ALREADY_CLOSED' OR t.state NOT IN('COMPLETED','CANCELLED')
  OR NOT public.org_trip_operation_valid(NEW.org_id,NEW.operation_id,'trip.finish',NEW.trip_id,i.payload_digest,rm,NULL,re)
  OR EXISTS(SELECT 1 FROM public.org_trip_finishes f WHERE f.org_id=NEW.org_id AND f.operation_id=NEW.operation_id)
  OR(NEW.occurred_at,NEW.occurred_submicro)<(t.occurred_at,t.occurred_submicro)
  OR(NEW.meter_id IS NOT NULL AND NOT EXISTS(SELECT 1 FROM public.org_meters m WHERE m.org_id=NEW.org_id AND m.id=NEW.meter_id AND m.asset_id=t.asset_id))
  THEN RAISE EXCEPTION USING ERRCODE='23514',MESSAGE='held trip finish source unavailable';END IF;
 END IF;
 RETURN NEW;
END $$;

-- The 235 logbook source: the author is responsible at the declared instant.
CREATE OR REPLACE FUNCTION org_trip_event_source() RETURNS trigger LANGUAGE plpgsql SET search_path=pg_catalog AS $$
DECLARE t public.org_trips%ROWTYPE;f public.org_trip_finishes%ROWTYPE;rm UUID;re UUID;
BEGIN
 IF NEW.org_id IS DISTINCT FROM NULLIF(current_setting('anpfuel.org_id',true),'')::uuid THEN RAISE EXCEPTION USING ERRCODE='42501',MESSAGE='organization scope denied';END IF;
 PERFORM 1 FROM public.organizations WHERE org_id=NEW.org_id AND has_snapshot FOR UPDATE;
 IF NOT FOUND THEN RAISE EXCEPTION USING ERRCODE='23503',MESSAGE='private reference not found';END IF;
 SELECT * INTO t FROM public.org_trips WHERE org_id=NEW.org_id AND id=NEW.trip_id FOR UPDATE;
 IF NOT FOUND THEN RAISE EXCEPTION USING ERRCODE='23503',MESSAGE='private reference not found';END IF;
 -- EMP-TRIP-012: the member responsible at the event's declared instant.
 SELECT r.membership_id,r.employee_id INTO rm,re FROM public.org_trip_responsible(NEW.org_id,NEW.trip_id,NEW.occurred_at,NEW.occurred_submicro) r;
 IF NOT public.org_trip_event_operation_valid(NEW.org_id,NEW.operation_id,NEW.id,NEW.payload_digest,NEW.author_membership_id,NEW.enrollment_id,re)
 OR NEW.author_membership_id<>rm OR NEW.author_employee_id<>re
 THEN RAISE EXCEPTION USING ERRCODE='23514',MESSAGE='trip event source unavailable';END IF;
 IF EXISTS(SELECT 1 FROM public.org_trip_held_events h WHERE h.org_id=NEW.org_id AND h.id=NEW.id)
 THEN RAISE EXCEPTION USING ERRCODE='23505',CONSTRAINT='org_trip_events_pkey',MESSAGE='trip event identity unavailable';END IF;
 IF NOT EXISTS(SELECT 1 FROM pg_timezone_names WHERE name=NEW.entry_zone) OR(NEW.occurred_at,NEW.occurred_submicro)<(t.occurred_at,t.occurred_submicro)
 THEN RAISE EXCEPTION USING ERRCODE='23514',MESSAGE='invalid trip event';END IF;
 SELECT * INTO f FROM public.org_trip_finishes WHERE org_id=NEW.org_id AND trip_id=NEW.trip_id;
 IF FOUND AND(NEW.occurred_at,NEW.occurred_submicro)>(f.occurred_at,f.occurred_submicro)
 THEN RAISE EXCEPTION USING ERRCODE='23514',MESSAGE='trip already closed';END IF;
 -- EMP-TRIP-011: a cancellation closes the logbook like a finish.
 IF EXISTS(SELECT 1 FROM public.org_trip_transitions x WHERE x.org_id=NEW.org_id AND x.trip_id=NEW.trip_id AND x.command='CANCEL'
  AND(NEW.occurred_at,NEW.occurred_submicro)>(x.occurred_at,x.occurred_submicro))
 THEN RAISE EXCEPTION USING ERRCODE='23514',MESSAGE='trip already closed';END IF;
 -- EMP-TRIP-014: a trip closed by a supervisor takes no direct fact.
 IF EXISTS(SELECT 1 FROM public.org_trip_closures c WHERE c.org_id=NEW.org_id AND c.trip_id=NEW.trip_id)
 THEN RAISE EXCEPTION USING ERRCODE='23514',MESSAGE='trip already closed';END IF;
 -- EMP-TRIP-015: a submitted, decided or amended revision takes no direct fact.
 IF public.org_trip_revision_closed(NEW.org_id,NEW.trip_id) THEN RAISE EXCEPTION USING ERRCODE='23514',MESSAGE='trip revision closed';END IF;
 NEW.received_at=clock_timestamp();RETURN NEW;
END $$;

-- The 235 held-logbook source: held only for the member responsible at the declared instant.
CREATE OR REPLACE FUNCTION org_trip_held_event_source() RETURNS trigger LANGUAGE plpgsql SET search_path=pg_catalog AS $$
DECLARE i public.org_sync_review_items%ROWTYPE;t public.org_trips%ROWTYPE;f public.org_trip_finishes%ROWTYPE;end_at TIMESTAMPTZ;end_sub SMALLINT;closed BOOLEAN;rm UUID;re UUID;
BEGIN
 IF NEW.org_id IS DISTINCT FROM NULLIF(current_setting('anpfuel.org_id',true),'')::uuid THEN RAISE EXCEPTION USING ERRCODE='42501',MESSAGE='organization scope denied';END IF;
 PERFORM 1 FROM public.organizations WHERE org_id=NEW.org_id AND has_snapshot FOR UPDATE;
 SELECT * INTO i FROM public.org_sync_review_items WHERE org_id=NEW.org_id AND id=NEW.review_item_id AND operation_id=NEW.operation_id;
 IF NOT FOUND THEN RAISE EXCEPTION USING ERRCODE='23514',MESSAGE='held trip event intent unavailable';END IF;
 SELECT * INTO t FROM public.org_trips WHERE org_id=NEW.org_id AND id=NEW.trip_id FOR UPDATE;
 IF NOT FOUND THEN RAISE EXCEPTION USING ERRCODE='23503',MESSAGE='private reference not found';END IF;
 SELECT * INTO f FROM public.org_trip_finishes WHERE org_id=NEW.org_id AND trip_id=NEW.trip_id;
 -- EMP-TRIP-011: the end of a closed trip is its finish or its cancellation.
 SELECT COALESCE(f.occurred_at,x.occurred_at),COALESCE(f.occurred_submicro,x.occurred_submicro) INTO end_at,end_sub
  FROM(SELECT 1) one LEFT JOIN public.org_trip_transitions x ON x.org_id=NEW.org_id AND x.trip_id=NEW.trip_id AND x.command='CANCEL';
 -- EMP-TRIP-014: a trip closed by a supervisor has no device end instant.
 closed=EXISTS(SELECT 1 FROM public.org_trip_closures c WHERE c.org_id=NEW.org_id AND c.trip_id=NEW.trip_id);
 -- EMP-TRIP-012: the member responsible at the event's declared instant.
 SELECT r.membership_id,r.employee_id INTO rm,re FROM public.org_trip_responsible(NEW.org_id,NEW.trip_id,NEW.occurred_at,NEW.occurred_submicro) r;
 IF i.operation_type<>'trip.event.record' OR i.operation_schema<>1 OR i.code NOT IN('TRIP_ALREADY_CLOSED','STALE_REVISION') OR t.state NOT IN('COMPLETED','CANCELLED') OR end_at IS NULL AND NOT closed
 OR NOT public.org_trip_event_operation_valid(NEW.org_id,NEW.operation_id,NEW.id,i.payload_digest,rm,NULL,re)
 OR i.author_employee_id<>re
 OR(i.code='TRIP_ALREADY_CLOSED' AND NOT closed AND(NEW.occurred_at,NEW.occurred_submicro)<=(end_at,end_sub))
 -- EMP-TRIP-014: after a supervisor closure, any instant from the start on.
 OR(i.code='TRIP_ALREADY_CLOSED' AND closed AND(NEW.occurred_at,NEW.occurred_submicro)<(t.occurred_at,t.occurred_submicro))
 -- EMP-TRIP-015: within the trip, but its revision is submitted, decided or amended.
 OR(i.code='STALE_REVISION' AND(t.state<>'COMPLETED' OR closed OR(NEW.occurred_at,NEW.occurred_submicro)>(f.occurred_at,f.occurred_submicro)
  OR(NEW.occurred_at,NEW.occurred_submicro)<(t.occurred_at,t.occurred_submicro) OR NOT public.org_trip_revision_closed(NEW.org_id,NEW.trip_id)))
 OR NOT EXISTS(SELECT 1 FROM pg_timezone_names WHERE name=NEW.entry_zone)
 THEN RAISE EXCEPTION USING ERRCODE='23514',MESSAGE='held trip event source unavailable';END IF;
 IF EXISTS(SELECT 1 FROM public.org_trip_events e WHERE e.org_id=NEW.org_id AND(e.id=NEW.id OR e.operation_id=NEW.operation_id))
 THEN RAISE EXCEPTION USING ERRCODE='23505',CONSTRAINT='org_trip_events_pkey',MESSAGE='trip event identity unavailable';END IF;
 RETURN NEW;
END $$;

-- The 229 held-submission source: a held submission is the current responsible member's.
CREATE OR REPLACE FUNCTION org_trip_held_submission_source() RETURNS trigger LANGUAGE plpgsql SET search_path=pg_catalog AS $$
DECLARE i public.org_sync_review_items%ROWTYPE;t public.org_trips%ROWTYPE;last_entry public.org_trip_review_entries%ROWTYPE;rm UUID;re UUID;
BEGIN
 IF NEW.org_id IS DISTINCT FROM NULLIF(current_setting('anpfuel.org_id',true),'')::uuid THEN RAISE EXCEPTION USING ERRCODE='42501',MESSAGE='organization scope denied';END IF;
 PERFORM 1 FROM public.organizations WHERE org_id=NEW.org_id AND has_snapshot FOR UPDATE;
 SELECT * INTO i FROM public.org_sync_review_items WHERE org_id=NEW.org_id AND id=NEW.review_item_id AND operation_id=NEW.operation_id;
 IF NOT FOUND THEN RAISE EXCEPTION USING ERRCODE='23514',MESSAGE='held trip submission intent unavailable';END IF;
 SELECT * INTO t FROM public.org_trips WHERE org_id=NEW.org_id AND id=NEW.trip_id FOR UPDATE;
 IF NOT FOUND THEN RAISE EXCEPTION USING ERRCODE='23503',MESSAGE='private reference not found';END IF;
 -- EMP-TRIP-012: the current responsible member and that member's employee.
 SELECT r.membership_id,r.employee_id INTO rm,re FROM public.org_trip_responsible(NEW.org_id,NEW.trip_id,NULL,NULL) r;
 IF i.operation_type<>'trip.submit' OR i.operation_schema<>1 OR i.code<>'STALE_REVISION' OR t.state<>'COMPLETED' OR i.author_employee_id<>re
 OR NOT EXISTS(SELECT 1 FROM public.org_sync_operations o JOIN public.org_memberships m ON(m.org_id,m.id)=(o.org_id,o.membership_id)
  JOIN public.org_device_enrollments e ON(e.org_id,e.id,e.membership_id)=(o.org_id,o.enrollment_id,o.membership_id)
  WHERE o.org_id=NEW.org_id AND o.operation_id=NEW.operation_id AND o.operation_type='trip.submit' AND o.operation_schema=1 AND o.entity_type='trip_review'
  AND o.entity_id=NEW.trip_id AND o.payload_digest=i.payload_digest AND o.membership_id=rm AND m.employee_id=re
  AND m.account_id=o.owner_account_id AND m.state='ACTIVE' AND e.state='ENROLLED')
 OR NOT EXISTS(SELECT 1 FROM pg_timezone_names WHERE name=NEW.entry_zone)
 OR EXISTS(SELECT 1 FROM public.org_trip_review_entries x WHERE x.org_id=NEW.org_id AND x.operation_id=NEW.operation_id)
 THEN RAISE EXCEPTION USING ERRCODE='23514',MESSAGE='held trip submission source unavailable';END IF;
 SELECT * INTO last_entry FROM public.org_trip_review_entries WHERE org_id=NEW.org_id AND trip_id=NEW.trip_id ORDER BY sequence DESC LIMIT 1;
 IF(last_entry.sequence IS NULL AND NEW.revision=1)
 OR(last_entry.state='RETURNED' AND NEW.revision=last_entry.revision AND NOT EXISTS(SELECT 1 FROM public.org_trip_review_entries x WHERE x.org_id=NEW.org_id AND x.trip_id=NEW.trip_id AND x.kind IN('AMEND','APPROVE')))
 THEN RAISE EXCEPTION USING ERRCODE='23514',MESSAGE='held trip submission condition unavailable';END IF;
 RETURN NEW;
END $$;

-- The 235 review-entry source: submission by the current responsible member;
-- every handover party is excluded from review decisions (B-BR-EMP-397).
CREATE OR REPLACE FUNCTION org_trip_review_entry_source() RETURNS trigger LANGUAGE plpgsql SET search_path=pg_catalog AS $$
DECLARE t public.org_trips%ROWTYPE;last_entry public.org_trip_review_entries%ROWTYPE;actor_person UUID;author_person UUID;driver_person UUID;
 amended BOOLEAN;effective BIGINT;amendment public.org_trip_review_entries%ROWTYPE;amendment_person UUID;closure public.org_trip_closures%ROWTYPE;closer_person UUID;rm UUID;re UUID;
BEGIN
 IF NEW.org_id IS DISTINCT FROM NULLIF(current_setting('anpfuel.org_id',true),'')::uuid THEN RAISE EXCEPTION USING ERRCODE='42501',MESSAGE='organization scope denied';END IF;
 PERFORM 1 FROM public.organizations WHERE org_id=NEW.org_id AND has_snapshot FOR UPDATE;
 IF NOT FOUND THEN RAISE EXCEPTION USING ERRCODE='23503',MESSAGE='private reference not found';END IF;
 SELECT * INTO t FROM public.org_trips WHERE org_id=NEW.org_id AND id=NEW.trip_id FOR UPDATE;
 IF NOT FOUND THEN RAISE EXCEPTION USING ERRCODE='23503',MESSAGE='private reference not found';END IF;
 IF t.state<>'COMPLETED' THEN RAISE EXCEPTION USING ERRCODE='23514',MESSAGE='trip review needs a completed trip';END IF;
 SELECT * INTO last_entry FROM public.org_trip_review_entries WHERE org_id=NEW.org_id AND trip_id=NEW.trip_id ORDER BY sequence DESC LIMIT 1;
 amended=EXISTS(SELECT 1 FROM public.org_trip_review_entries WHERE org_id=NEW.org_id AND trip_id=NEW.trip_id AND kind='AMEND');
 SELECT max(revision) INTO effective FROM public.org_trip_review_entries WHERE org_id=NEW.org_id AND trip_id=NEW.trip_id AND kind='APPROVE';
 -- EMP-TRIP-015: the author resubmits a returned field revision (revision
 -- + 1); an amendment opens the next revision after an approval, a
 -- rejected amendment or a returned amendment, with its content row.
 IF NOT((last_entry.sequence IS NULL AND NEW.sequence=1 AND NEW.kind='SUBMIT' AND NEW.revision=1)
 OR(last_entry.sequence IS NOT NULL AND NEW.sequence=last_entry.sequence+1 AND(
  (NEW.revision=last_entry.revision AND((last_entry.state='SUBMITTED' AND NEW.kind='OPEN') OR(last_entry.state='IN_REVIEW' AND NEW.kind IN('APPROVE','RETURN','REJECT'))))
  OR(NEW.kind='RESUBMIT' AND NEW.revision=last_entry.revision+1 AND last_entry.state='RETURNED' AND NOT amended AND effective IS NULL)
  OR(NEW.kind='AMEND' AND NEW.revision=last_entry.revision+1 AND effective IS NOT NULL
   AND(last_entry.state='APPROVED' OR(last_entry.state IN('REJECTED','RETURNED') AND last_entry.revision>effective))
   AND EXISTS(SELECT 1 FROM public.org_trip_amendments a WHERE a.org_id=NEW.org_id AND a.trip_id=NEW.trip_id AND a.revision=NEW.revision
    AND a.operation_id=NEW.operation_id AND a.actor_membership_id=NEW.actor_membership_id AND a.actor_employee_id=NEW.actor_employee_id)))))
 THEN RAISE EXCEPTION USING ERRCODE='23514',MESSAGE='invalid trip review transition';END IF;
 IF NEW.kind IN('SUBMIT','RESUBMIT') THEN
  -- EMP-TRIP-012: the current responsible member submits.
  SELECT r.membership_id,r.employee_id INTO rm,re FROM public.org_trip_responsible(NEW.org_id,NEW.trip_id,NULL,NULL) r;
  IF NEW.actor_membership_id<>rm OR NEW.actor_employee_id<>re OR NOT EXISTS(
   SELECT 1 FROM public.org_sync_operations o JOIN public.org_memberships m ON(m.org_id,m.id)=(o.org_id,o.membership_id)
   JOIN public.org_device_enrollments e ON(e.org_id,e.id,e.membership_id)=(o.org_id,o.enrollment_id,o.membership_id)
   WHERE o.org_id=NEW.org_id AND o.operation_id=NEW.operation_id AND o.operation_type='trip.submit' AND o.operation_schema=1 AND o.entity_type='trip_review'
   AND o.entity_id=NEW.trip_id AND o.membership_id=rm AND m.employee_id=re AND m.account_id=o.owner_account_id
   AND m.state='ACTIVE' AND e.state='ENROLLED')
  THEN RAISE EXCEPTION USING ERRCODE='23514',MESSAGE='trip submission source unavailable';END IF;
 ELSIF NEW.kind='AMEND' THEN
  IF NOT EXISTS(SELECT 1 FROM public.org_memberships m JOIN public.org_employee_identities e ON(e.org_id,e.id)=(m.org_id,m.employee_id)
   WHERE m.org_id=NEW.org_id AND m.id=NEW.actor_membership_id AND m.employee_id=NEW.actor_employee_id AND m.state='ACTIVE' AND e.state='ACTIVE' AND e.merged_into IS NULL)
  THEN RAISE EXCEPTION USING ERRCODE='23514',MESSAGE='trip amendment author unavailable';END IF;
 ELSE
  IF NOT EXISTS(SELECT 1 FROM public.org_memberships m JOIN public.org_employee_identities e ON(e.org_id,e.id)=(m.org_id,m.employee_id)
   WHERE m.org_id=NEW.org_id AND m.id=NEW.actor_membership_id AND m.employee_id=NEW.actor_employee_id AND m.state='ACTIVE' AND e.state='ACTIVE' AND e.merged_into IS NULL)
  THEN RAISE EXCEPTION USING ERRCODE='23514',MESSAGE='trip reviewer unavailable';END IF;
  actor_person=public.org_sync_review_employee_canonical(NEW.org_id,NEW.actor_employee_id);
  author_person=public.org_sync_review_employee_canonical(t.org_id,t.author_employee_id);
  driver_person=public.org_sync_review_employee_canonical(t.org_id,t.driver_employee_id);
  IF actor_person IS NULL OR author_person IS NULL OR driver_person IS NULL OR actor_person IN(author_person,driver_person)
  OR NEW.actor_membership_id=t.author_membership_id OR EXISTS(SELECT 1 FROM public.org_memberships a JOIN public.org_memberships r ON(r.org_id,r.account_id)=(a.org_id,a.account_id)
   WHERE a.org_id=NEW.org_id AND a.id=t.author_membership_id AND r.id=NEW.actor_membership_id)
  THEN RAISE EXCEPTION USING ERRCODE='23514',MESSAGE='trip self approval refused';END IF;
  -- BR-REV-003 for an amendment revision: its author never reviews it.
  SELECT * INTO amendment FROM public.org_trip_review_entries WHERE org_id=NEW.org_id AND trip_id=NEW.trip_id AND kind='AMEND' AND revision=NEW.revision;
  IF FOUND THEN
   amendment_person=public.org_sync_review_employee_canonical(NEW.org_id,amendment.actor_employee_id);
   IF amendment_person IS NULL OR actor_person=amendment_person OR NEW.actor_membership_id=amendment.actor_membership_id
   OR EXISTS(SELECT 1 FROM public.org_memberships a JOIN public.org_memberships r ON(r.org_id,r.account_id)=(a.org_id,a.account_id)
    WHERE a.org_id=NEW.org_id AND a.id=amendment.actor_membership_id AND r.id=NEW.actor_membership_id)
   THEN RAISE EXCEPTION USING ERRCODE='23514',MESSAGE='trip self approval refused';END IF;
  END IF;
  -- EMP-TRIP-012 (B-BR-EMP-397): every party of every handover of the trip
  -- (giving, receiving and recording member) never decides its review.
  IF EXISTS(SELECT 1 FROM public.org_trip_handovers h CROSS JOIN LATERAL(VALUES(h.from_membership_id,h.from_employee_id),(h.to_membership_id,h.to_employee_id),
    (h.recorder_membership_id,h.recorder_employee_id)) party(membership_id,employee_id)
   JOIN public.org_memberships pm ON(pm.org_id,pm.id)=(h.org_id,party.membership_id) JOIN public.org_memberships am ON(am.org_id,am.id)=(h.org_id,NEW.actor_membership_id)
   WHERE h.org_id=NEW.org_id AND h.trip_id=NEW.trip_id AND(party.membership_id=NEW.actor_membership_id OR pm.account_id=am.account_id
    OR public.org_sync_review_employee_canonical(h.org_id,party.employee_id) IS NOT DISTINCT FROM actor_person))
  THEN RAISE EXCEPTION USING ERRCODE='23514',MESSAGE='trip self approval refused';END IF;
  -- EMP-TRIP-014 (BR-REV-003): the member who closed the trip never reviews it.
  SELECT * INTO closure FROM public.org_trip_closures WHERE org_id=NEW.org_id AND trip_id=NEW.trip_id;
  IF FOUND THEN
   closer_person=public.org_sync_review_employee_canonical(NEW.org_id,closure.closer_employee_id);
   IF closer_person IS NULL OR actor_person=closer_person OR NEW.actor_membership_id=closure.closer_membership_id
   OR EXISTS(SELECT 1 FROM public.org_memberships a JOIN public.org_memberships r ON(r.org_id,r.account_id)=(a.org_id,a.account_id)
    WHERE a.org_id=NEW.org_id AND a.id=closure.closer_membership_id AND r.id=NEW.actor_membership_id)
   THEN RAISE EXCEPTION USING ERRCODE='23514',MESSAGE='trip self approval refused';END IF;
  END IF;
 END IF;
 IF NEW.kind IN('OPEN','APPROVE','RETURN','REJECT','AMEND') AND EXISTS(SELECT 1 FROM public.org_finance_periods p
  WHERE p.org_id=NEW.org_id AND p.period_month=date_trunc('month',t.reporting_day)::date AND p.state='LOCKED')
 THEN RAISE EXCEPTION USING ERRCODE='55000',MESSAGE='trip reporting period locked';END IF;
 NEW.recorded_at=clock_timestamp();RETURN NEW;
END $$;

-- The 230 incident report source: a report inside a trip comes from the
-- member responsible for it at the declared instant.
CREATE OR REPLACE FUNCTION org_incident_report_source() RETURNS trigger LANGUAGE plpgsql SET search_path=pg_catalog AS $$
DECLARE a public.org_assets%ROWTYPE;t public.org_trips%ROWTYPE;f public.org_trip_finishes%ROWTYPE;actual_unit UUID;actual_centre UUID;rm UUID;re UUID;
BEGIN
 IF NEW.org_id IS DISTINCT FROM NULLIF(current_setting('anpfuel.org_id',true),'')::uuid THEN RAISE EXCEPTION USING ERRCODE='42501',MESSAGE='organization scope denied';END IF;
 PERFORM 1 FROM public.organizations WHERE org_id=NEW.org_id AND has_snapshot FOR UPDATE;
 IF NOT FOUND THEN RAISE EXCEPTION USING ERRCODE='23503',MESSAGE='private reference not found';END IF;
 IF NOT public.org_incident_report_operation_valid(NEW.org_id,NEW.operation_id,NEW.id,NEW.payload_digest,NEW.reporter_membership_id,NEW.enrollment_id,NEW.reporter_account_id,NEW.reporter_employee_id)
 THEN RAISE EXCEPTION USING ERRCODE='23514',MESSAGE='incident report source unavailable';END IF;
 SELECT * INTO a FROM public.org_assets WHERE org_id=NEW.org_id AND id=NEW.asset_id FOR UPDATE;
 IF NOT FOUND THEN RAISE EXCEPTION USING ERRCODE='23503',MESSAGE='private reference not found';END IF;
 IF a.state='DRAFT' OR NEW.state<>'REPORTED' OR NEW.revision<>1 THEN RAISE EXCEPTION USING ERRCODE='23514',MESSAGE='asset does not accept a report';END IF;
 IF NOT EXISTS(SELECT 1 FROM pg_timezone_names WHERE name=NEW.entry_zone) THEN RAISE EXCEPTION USING ERRCODE='23514',MESSAGE='invalid incident report declaration';END IF;
 -- BR-AUTH-005: the FLEET-005 allocation at the declared instant.
 SELECT p.unit_id INTO actual_unit FROM public.org_asset_unit_periods p WHERE p.org_id=NEW.org_id AND p.asset_id=NEW.asset_id AND p.valid_from<=NEW.occurred_at AND(p.valid_to IS NULL OR NEW.occurred_at<p.valid_to);
 SELECT p.cost_centre_id INTO actual_centre FROM public.org_asset_cost_centre_periods p WHERE p.org_id=NEW.org_id AND p.asset_id=NEW.asset_id AND p.valid_from<=NEW.occurred_at AND(p.valid_to IS NULL OR NEW.occurred_at<p.valid_to);
 IF NEW.unit_id IS DISTINCT FROM actual_unit OR NEW.cost_centre_id IS DISTINCT FROM(CASE WHEN actual_unit IS NULL THEN NULL ELSE actual_centre END)
 THEN RAISE EXCEPTION USING ERRCODE='23514',MESSAGE='incident report allocation snapshot unavailable';END IF;
 IF NEW.trip_id IS NOT NULL THEN
  SELECT * INTO t FROM public.org_trips WHERE org_id=NEW.org_id AND id=NEW.trip_id FOR UPDATE;
  IF NOT FOUND THEN RAISE EXCEPTION USING ERRCODE='23503',MESSAGE='private reference not found';END IF;
  SELECT * INTO f FROM public.org_trip_finishes WHERE org_id=NEW.org_id AND trip_id=NEW.trip_id;
  -- EMP-TRIP-012: the member responsible for the trip at the declared instant.
  SELECT r.membership_id,r.employee_id INTO rm,re FROM public.org_trip_responsible(NEW.org_id,NEW.trip_id,NEW.occurred_at,NEW.occurred_submicro) r;
  IF t.asset_id<>NEW.asset_id OR rm IS DISTINCT FROM NEW.reporter_membership_id OR(NEW.occurred_at,NEW.occurred_submicro)<(t.occurred_at,t.occurred_submicro)
  OR(f.trip_id IS NOT NULL AND(NEW.occurred_at,NEW.occurred_submicro)>(f.occurred_at,f.occurred_submicro))
  THEN RAISE EXCEPTION USING ERRCODE='23514',MESSAGE='incident report trip unavailable';END IF;
 END IF;
 NEW.received_at=clock_timestamp();NEW.updated_at=NEW.received_at;RETURN NEW;
END $$;

-- The 236 offboarding guard: a TRIP flag names a trip the ending membership is
-- currently responsible for, and none of those trips is left unflagged.
CREATE OR REPLACE FUNCTION org_offboarding_guard() RETURNS trigger LANGUAGE plpgsql SET search_path=pg_catalog AS $$
DECLARE item JSONB;k TEXT;parts TEXT[];
BEGIN
 IF NEW.result-ARRAY['membership_id','employee_id','access_model','state','revision','suspension_origin','end_reason','employee_state','employee_revision','handover_digest','items']<>'{}'::jsonb
 OR NOT(NEW.result ?& ARRAY['membership_id','employee_id','access_model','state','revision','suspension_origin','end_reason','employee_state','employee_revision','handover_digest','items'])
 OR jsonb_typeof(NEW.result->'items')<>'array' OR jsonb_array_length(NEW.result->'items')>500
 OR NOT EXISTS(SELECT 1 FROM public.org_memberships m JOIN public.org_employee_identities e ON(e.org_id,e.id)=(m.org_id,m.employee_id)
 WHERE m.org_id=NEW.org_id AND m.id=NEW.membership_id AND m.access_model='EXISTING_ACCOUNT' AND m.state='ENDED'
 AND NEW.result->>'membership_id'=m.id::text AND NEW.result->>'employee_id'=m.employee_id::text AND NEW.result->>'access_model'=m.access_model AND NEW.result->>'state'=m.state
 AND NEW.result->>'revision'=m.revision::text AND NEW.result->>'suspension_origin'=m.suspension_origin AND NEW.result->>'end_reason'=m.end_reason
 AND m.end_reason=CASE NEW.kind WHEN 'LEAVE' THEN 'LEFT' ELSE 'OFFBOARDED' END
 AND(NEW.kind<>'LEAVE' OR m.account_id=NEW.actor_account_id)
 AND NEW.result->>'employee_state'=e.state AND NEW.result->>'employee_revision'=e.revision::text)
 OR NOT EXISTS(SELECT 1 FROM public.org_audit_events a WHERE a.org_id=NEW.org_id AND a.operation_id=NEW.operation_id AND a.actor_id=NEW.actor_account_id AND a.target_type='membership' AND a.target_id=NEW.membership_id AND a.request_hash=NEW.request_hash AND a.action='idn.membership.ended' AND a.reason_code=CASE NEW.kind WHEN 'LEAVE' THEN 'left' ELSE 'offboarded' END)
 THEN RAISE EXCEPTION USING ERRCODE='P9002',MESSAGE='offboarding source unavailable';END IF;
 -- Flags reference actual retained source facts after their reductions, not
 -- client-created work or a claim about modules that are not installed.
 FOR item IN SELECT value FROM jsonb_array_elements(NEW.result->'items') LOOP
  k=item->>'key';parts=string_to_array(k,':');
  IF item-ARRAY['key','source_revision']<>'{}'::jsonb OR NOT(item ?& ARRAY['key','source_revision']) OR NOT(
   (parts[1]='TEAM_LINK' AND array_length(parts,1)=2 AND item->>'source_revision'='1' AND EXISTS(SELECT 1 FROM public.org_team_links l JOIN public.org_memberships m ON(m.org_id,m.employee_id)=(l.org_id,l.employee_id) WHERE m.org_id=NEW.org_id AND m.id=NEW.membership_id AND l.id::text=parts[2] AND l.end_reason='MEMBERSHIP_ENDED' AND l.source_membership_id=NEW.membership_id AND l.source_membership_revision::text=NEW.result->>'revision'))
   OR(parts[1]='SETTING' AND array_length(parts,1)=3 AND EXISTS(SELECT 1 FROM public.org_settings_designations d WHERE d.org_id=NEW.org_id AND d.member_id=NEW.membership_id AND d.settings_revision::text=item->>'source_revision' AND d.responsibility=parts[2] AND d.slot=parts[3] AND EXISTS(SELECT 1 FROM public.org_settings_versions v WHERE v.org_id=d.org_id AND v.revision=d.settings_revision+1 AND v.source='MEMBERSHIP_REDUCTION' AND v.source_membership_id=NEW.membership_id AND v.source_membership_revision::text=NEW.result->>'revision')))
   -- EMP-TRIP-018: a trip in progress that the ending membership started;
   -- EMP-TRIP-012: the trip it is currently responsible for.
   OR(parts[1]='TRIP' AND array_length(parts,1)=2 AND item->>'source_revision'='1' AND EXISTS(SELECT 1 FROM public.org_trips t WHERE t.org_id=NEW.org_id AND t.id::text=parts[2] AND public.org_trip_responsible_member(t.org_id,t.id)=NEW.membership_id AND t.state IN('IN_PROGRESS','PAUSED'))))
  THEN RAISE EXCEPTION USING ERRCODE='P9002',MESSAGE='handover source unavailable';END IF;
 END LOOP;
 -- EMP-TRIP-018 (B-BR-EMP-404): no trip in progress of the ending membership
 -- is left without its flag; offboarding never closes or deletes it.
 IF EXISTS(SELECT 1 FROM public.org_trips t WHERE t.org_id=NEW.org_id AND t.state IN('IN_PROGRESS','PAUSED') AND public.org_trip_responsible_member(t.org_id,t.id)=NEW.membership_id
  AND NOT EXISTS(SELECT 1 FROM jsonb_array_elements(NEW.result->'items') x WHERE x.value->>'key'='TRIP:'||t.id::text))
 THEN RAISE EXCEPTION USING ERRCODE='P9002',MESSAGE='handover source unavailable';END IF;
 NEW.occurred_at=clock_timestamp();RETURN NEW;
END $$;

-- The 235 closure source: the closer is not the member currently responsible
-- for the trip either (a handover receiver closes nothing it drives).
CREATE OR REPLACE FUNCTION org_trip_closure_source() RETURNS trigger LANGUAGE plpgsql SET search_path=pg_catalog AS $$
DECLARE t public.org_trips%ROWTYPE;closer_person UUID;author_person UUID;driver_person UUID;rm UUID;re UUID;
BEGIN
 IF NEW.org_id IS DISTINCT FROM NULLIF(current_setting('anpfuel.org_id',true),'')::uuid THEN RAISE EXCEPTION USING ERRCODE='42501',MESSAGE='organization scope denied';END IF;
 PERFORM 1 FROM public.organizations WHERE org_id=NEW.org_id AND has_snapshot FOR UPDATE;
 IF NOT FOUND THEN RAISE EXCEPTION USING ERRCODE='23503',MESSAGE='private reference not found';END IF;
 SELECT * INTO t FROM public.org_trips WHERE org_id=NEW.org_id AND id=NEW.trip_id FOR UPDATE;
 IF NOT FOUND THEN RAISE EXCEPTION USING ERRCODE='23503',MESSAGE='private reference not found';END IF;
 -- EMP-TRIP-011: a paused trip is open; a cancelled one is closed.
 IF t.state NOT IN('IN_PROGRESS','PAUSED') OR EXISTS(SELECT 1 FROM public.org_trip_finishes f WHERE f.org_id=NEW.org_id AND f.trip_id=NEW.trip_id)
 THEN RAISE EXCEPTION USING ERRCODE='23514',MESSAGE='trip already closed';END IF;
 IF NOT EXISTS(SELECT 1 FROM public.org_memberships m JOIN public.org_employee_identities e ON(e.org_id,e.id)=(m.org_id,m.employee_id)
  WHERE m.org_id=NEW.org_id AND m.id=NEW.closer_membership_id AND m.employee_id=NEW.closer_employee_id AND m.state='ACTIVE' AND e.state='ACTIVE' AND e.merged_into IS NULL)
 THEN RAISE EXCEPTION USING ERRCODE='23514',MESSAGE='trip closer unavailable';END IF;
 closer_person=public.org_sync_review_employee_canonical(NEW.org_id,NEW.closer_employee_id);
 author_person=public.org_sync_review_employee_canonical(t.org_id,t.author_employee_id);
 driver_person=public.org_sync_review_employee_canonical(t.org_id,t.driver_employee_id);
 IF closer_person IS NULL OR author_person IS NULL OR driver_person IS NULL OR closer_person IN(author_person,driver_person)
 OR NEW.closer_membership_id=t.author_membership_id OR EXISTS(SELECT 1 FROM public.org_memberships a JOIN public.org_memberships r ON(r.org_id,r.account_id)=(a.org_id,a.account_id)
  WHERE a.org_id=NEW.org_id AND a.id=t.author_membership_id AND r.id=NEW.closer_membership_id)
 THEN RAISE EXCEPTION USING ERRCODE='23514',MESSAGE='trip self closure refused';END IF;
 -- EMP-TRIP-012: nor the member currently responsible for it after a handover.
 SELECT r.membership_id,r.employee_id INTO rm,re FROM public.org_trip_responsible(NEW.org_id,NEW.trip_id,NULL,NULL) r;
 IF closer_person IS NOT DISTINCT FROM public.org_sync_review_employee_canonical(NEW.org_id,re) OR NEW.closer_membership_id=rm
 OR EXISTS(SELECT 1 FROM public.org_memberships a JOIN public.org_memberships r ON(r.org_id,r.account_id)=(a.org_id,a.account_id)
  WHERE a.org_id=NEW.org_id AND a.id=rm AND r.id=NEW.closer_membership_id)
 THEN RAISE EXCEPTION USING ERRCODE='23514',MESSAGE='trip self closure refused';END IF;
 NEW.closed_at=clock_timestamp();RETURN NEW;
END $$;

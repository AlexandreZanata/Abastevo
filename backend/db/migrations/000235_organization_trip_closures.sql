-- EMP-TRIP-014. Supervisor closure of another member's open trip
-- (B-BR-EMP-399..403, EDR-257..259). Additive: one insert-only closure fact
-- per trip completes an open (IN_PROGRESS or PAUSED, EMP-TRIP-011) trip
-- without a finish at its next revision, with the closing
-- member, the reason and the explicit reason why no final reading exists. A
-- closure holds no reading, value, segment or distance. The closer is never
-- the trip's author or driver and never reviews the trip. Logbook events
-- received after a closure are held TRIP_ALREADY_CLOSED. The closure keys
-- reference only the trip, the membership, the employee record and the audit
-- event: no asset, unit or cost-centre key, so no usage source changes.
CREATE TABLE org_trip_closures(
 org_id UUID NOT NULL,
 trip_id UUID NOT NULL,
 operation_id UUID NOT NULL CHECK(operation_id<>'00000000-0000-0000-0000-000000000000'::uuid),
 request_hash TEXT NOT NULL CHECK(request_hash~'^[a-f0-9]{64}$'),
 closer_membership_id UUID NOT NULL,
 closer_employee_id UUID NOT NULL,
 reason TEXT NOT NULL CHECK(org_asset_text_valid(reason,280,true)),
 missing_reading_reason TEXT NOT NULL CHECK(org_asset_text_valid(missing_reading_reason,280,true)),
 closed_at TIMESTAMPTZ NOT NULL DEFAULT clock_timestamp(),
 PRIMARY KEY(org_id,trip_id),
 CONSTRAINT org_trip_closures_operation_key UNIQUE(org_id,operation_id),
 CONSTRAINT org_trip_closures_trip_fkey FOREIGN KEY(org_id,trip_id) REFERENCES org_trips(org_id,id),
 CONSTRAINT org_trip_closures_closer_fkey FOREIGN KEY(org_id,closer_membership_id) REFERENCES org_memberships(org_id,id),
 CONSTRAINT org_trip_closures_closer_employee_fkey FOREIGN KEY(org_id,closer_employee_id) REFERENCES org_employee_identities(org_id,id),
 CONSTRAINT org_trip_closures_audit_fkey FOREIGN KEY(org_id,operation_id) REFERENCES org_audit_events(org_id,operation_id) DEFERRABLE INITIALLY DEFERRED,
 CHECK(isfinite(closed_at) AND closed_at>'0001-01-01 00:00:00+00'::timestamptz AND closed_at<'10000-01-01 00:00:00+00'::timestamptz)
);

-- B-BR-EMP-399/401: a trip in progress, an active unmerged closer, and the
-- separation from the author and driver (account, membership, canonical
-- employee), under the organization root and trip row locks (the finish
-- trigger locks the same rows in the same order).
CREATE FUNCTION org_trip_closure_source() RETURNS trigger LANGUAGE plpgsql SET search_path=pg_catalog AS $$
DECLARE t public.org_trips%ROWTYPE;closer_person UUID;author_person UUID;driver_person UUID;
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
 NEW.closed_at=clock_timestamp();RETURN NEW;
END $$;
CREATE TRIGGER org_trip_closure_source BEFORE INSERT ON org_trip_closures FOR EACH ROW EXECUTE FUNCTION org_trip_closure_source();
CREATE FUNCTION org_trip_closure_apply() RETURNS trigger LANGUAGE plpgsql SET search_path=pg_catalog AS $$
BEGIN
 UPDATE public.org_trips SET state='COMPLETED' WHERE org_id=NEW.org_id AND id=NEW.trip_id AND state IN('IN_PROGRESS','PAUSED');
 IF NOT FOUND THEN RAISE EXCEPTION USING ERRCODE='23514',MESSAGE='trip already closed';END IF;
 RETURN NULL;
END $$;
CREATE TRIGGER org_trip_closure_apply AFTER INSERT ON org_trip_closures FOR EACH ROW EXECUTE FUNCTION org_trip_closure_apply();

-- At commit a closure is published only with its trip.trip.closed audit
-- event by the closer's account and digest, on a completed trip.
CREATE FUNCTION org_trip_closure_published() RETURNS trigger LANGUAGE plpgsql SET search_path=pg_catalog AS $$
DECLARE prior_org TEXT;
BEGIN
 prior_org=current_setting('anpfuel.org_id',true);PERFORM set_config('anpfuel.org_id',NEW.org_id::text,true);
 IF NOT EXISTS(SELECT 1 FROM public.org_trips t WHERE t.org_id=NEW.org_id AND t.id=NEW.trip_id AND t.state='COMPLETED')
 OR NOT EXISTS(SELECT 1 FROM public.org_audit_events a WHERE a.org_id=NEW.org_id AND a.operation_id=NEW.operation_id AND a.target_type='trip' AND a.target_id=NEW.trip_id
  AND a.action='trip.trip.closed' AND a.request_hash=NEW.request_hash
  AND a.actor_id=(SELECT m.account_id FROM public.org_memberships m WHERE m.org_id=NEW.org_id AND m.id=NEW.closer_membership_id))
 THEN RAISE EXCEPTION USING ERRCODE='23514',MESSAGE='trip closure publication unavailable';END IF;
 PERFORM set_config('anpfuel.org_id',COALESCE(prior_org,''),true);RETURN NULL;
END $$;
CREATE CONSTRAINT TRIGGER org_trip_closure_published AFTER INSERT ON org_trip_closures DEFERRABLE INITIALLY DEFERRED FOR EACH ROW EXECUTE FUNCTION org_trip_closure_published();

ALTER TABLE org_trip_closures ENABLE ROW LEVEL SECURITY;ALTER TABLE org_trip_closures FORCE ROW LEVEL SECURITY;
CREATE POLICY org_trip_closures_scope ON org_trip_closures USING(org_id=NULLIF(current_setting('anpfuel.org_id',true),'')::uuid) WITH CHECK(org_id=NULLIF(current_setting('anpfuel.org_id',true),'')::uuid);
CREATE TRIGGER org_trip_closures_immutable BEFORE UPDATE OR DELETE OR TRUNCATE ON org_trip_closures FOR EACH STATEMENT EXECUTE FUNCTION org_trip_keep_history();
REVOKE ALL ON org_trip_closures FROM PUBLIC;

-- The 233 trip revision with the closure counted like a finish.
CREATE OR REPLACE FUNCTION org_trip_revision(p_org UUID,p_trip UUID) RETURNS BIGINT LANGUAGE SQL STABLE SET search_path=pg_catalog AS $$
 SELECT 1+(SELECT count(*) FROM public.org_trip_transitions x WHERE x.org_id=p_org AND x.trip_id=p_trip)
  +(SELECT count(*) FROM public.org_trip_finishes f WHERE f.org_id=p_org AND f.trip_id=p_trip)
  -- EMP-TRIP-014: the closure ends the trip at the next revision like a finish.
  +(SELECT count(*) FROM public.org_trip_closures c WHERE c.org_id=p_org AND c.trip_id=p_trip)
 $$;

-- The 233 completion guard with one more admitted move: an open trip to
-- COMPLETED beside the closure inserted in this transaction.
CREATE OR REPLACE FUNCTION org_trip_completion_guard() RETURNS trigger LANGUAGE plpgsql SET search_path=pg_catalog AS $$
BEGIN
 IF to_jsonb(NEW)-'state'<>to_jsonb(OLD)-'state' OR NOT(
  (OLD.state IN('IN_PROGRESS','PAUSED') AND NEW.state='COMPLETED' AND EXISTS(SELECT 1 FROM public.org_trip_finishes f WHERE f.org_id=OLD.org_id AND f.trip_id=OLD.id))
  -- EMP-TRIP-011: a lifecycle move only with the transition row of the trip's latest revision.
  OR EXISTS(SELECT 1 FROM public.org_trip_transitions x WHERE x.org_id=OLD.org_id AND x.trip_id=OLD.id AND x.from_state=OLD.state AND x.to_state=NEW.state
   AND x.revision=public.org_trip_revision(OLD.org_id,OLD.id))
  -- EMP-TRIP-014: an open trip completes with the closure inserted in this transaction.
  OR(OLD.state IN('IN_PROGRESS','PAUSED') AND NEW.state='COMPLETED' AND EXISTS(SELECT 1 FROM public.org_trip_closures c WHERE c.org_id=OLD.org_id AND c.trip_id=OLD.id)))
 THEN RAISE EXCEPTION USING ERRCODE='55000',MESSAGE='organization trip history is insert-only';END IF;
 RETURN NEW;
END $$;

-- The 233 logbook source unchanged except one clause: a trip closed by a
-- supervisor takes no direct fact (B-BR-EMP-402).
CREATE OR REPLACE FUNCTION org_trip_event_source() RETURNS trigger LANGUAGE plpgsql SET search_path=pg_catalog AS $$
DECLARE t public.org_trips%ROWTYPE;f public.org_trip_finishes%ROWTYPE;
BEGIN
 IF NEW.org_id IS DISTINCT FROM NULLIF(current_setting('anpfuel.org_id',true),'')::uuid THEN RAISE EXCEPTION USING ERRCODE='42501',MESSAGE='organization scope denied';END IF;
 PERFORM 1 FROM public.organizations WHERE org_id=NEW.org_id AND has_snapshot FOR UPDATE;
 IF NOT FOUND THEN RAISE EXCEPTION USING ERRCODE='23503',MESSAGE='private reference not found';END IF;
 SELECT * INTO t FROM public.org_trips WHERE org_id=NEW.org_id AND id=NEW.trip_id FOR UPDATE;
 IF NOT FOUND THEN RAISE EXCEPTION USING ERRCODE='23503',MESSAGE='private reference not found';END IF;
 IF NOT public.org_trip_event_operation_valid(NEW.org_id,NEW.operation_id,NEW.id,NEW.payload_digest,NEW.author_membership_id,NEW.enrollment_id,t.author_employee_id)
 OR NEW.author_membership_id<>t.author_membership_id OR NEW.author_employee_id<>t.author_employee_id
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

-- The 233 held-event source with TRIP_ALREADY_CLOSED on a trip closed by a
-- supervisor (any instant at or after the start: the closure has no device
-- instant); STALE_REVISION stays a finished trip's; every other check
-- unchanged.
CREATE OR REPLACE FUNCTION org_trip_held_event_source() RETURNS trigger LANGUAGE plpgsql SET search_path=pg_catalog AS $$
DECLARE i public.org_sync_review_items%ROWTYPE;t public.org_trips%ROWTYPE;f public.org_trip_finishes%ROWTYPE;end_at TIMESTAMPTZ;end_sub SMALLINT;closed BOOLEAN;
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
 IF i.operation_type<>'trip.event.record' OR i.operation_schema<>1 OR i.code NOT IN('TRIP_ALREADY_CLOSED','STALE_REVISION') OR t.state NOT IN('COMPLETED','CANCELLED') OR end_at IS NULL AND NOT closed
 OR NOT public.org_trip_event_operation_valid(NEW.org_id,NEW.operation_id,NEW.id,i.payload_digest,t.author_membership_id,NULL,t.author_employee_id)
 OR i.author_employee_id<>t.author_employee_id
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

-- The 229 review-entry source with the closer's separation (B-BR-EMP-401):
-- the member who closed the trip never opens or decides any of its review
-- revisions; every other check unchanged.
CREATE OR REPLACE FUNCTION org_trip_review_entry_source() RETURNS trigger LANGUAGE plpgsql SET search_path=pg_catalog AS $$
DECLARE t public.org_trips%ROWTYPE;last_entry public.org_trip_review_entries%ROWTYPE;actor_person UUID;author_person UUID;driver_person UUID;
 amended BOOLEAN;effective BIGINT;amendment public.org_trip_review_entries%ROWTYPE;amendment_person UUID;closure public.org_trip_closures%ROWTYPE;closer_person UUID;
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
  IF NEW.actor_membership_id<>t.author_membership_id OR NEW.actor_employee_id<>t.author_employee_id OR NOT EXISTS(
   SELECT 1 FROM public.org_sync_operations o JOIN public.org_memberships m ON(m.org_id,m.id)=(o.org_id,o.membership_id)
   JOIN public.org_device_enrollments e ON(e.org_id,e.id,e.membership_id)=(o.org_id,o.enrollment_id,o.membership_id)
   WHERE o.org_id=NEW.org_id AND o.operation_id=NEW.operation_id AND o.operation_type='trip.submit' AND o.operation_schema=1 AND o.entity_type='trip_review'
   AND o.entity_id=NEW.trip_id AND o.membership_id=t.author_membership_id AND m.employee_id=t.author_employee_id AND m.account_id=o.owner_account_id
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

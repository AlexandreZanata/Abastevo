-- EMP-TRIP-011. Pause, resume and cancellation of a trip through the signed
-- intake (B-BR-EMP-390..393, EDR-252/253). A trip keeps one identity: each
-- accepted trip.pause, trip.resume or trip.cancel appends one insert-only
-- lifecycle row at the trip's next revision (1 after the start, one more per
-- command or finish) and moves the trip's state only together with it. A
-- paused trip still holds its asset; a cancelled one is terminal, keeps every
-- earlier fact, has no finish and releases the asset at its cancellation. A
-- command on a closed trip (TRIP_ALREADY_CLOSED) or naming a stale revision
-- (STALE_REVISION) is an OFFL-008 held fact. Lifecycle rows reference only
-- the trip, the operation and the author records: no asset, unit or
-- cost-centre key, so no usage source changes. Redefined bodies keep every
-- other line of their latest version (000220 completion guard, finish
-- source, finish apply, publication and held source; 000229 logbook and
-- held-logbook sources and review-source hook; 000232 mission vehicle source).
ALTER TABLE org_trips DROP CONSTRAINT org_trips_state_check,
 ADD CONSTRAINT org_trips_state_check CHECK(state IN('IN_PROGRESS','PAUSED','COMPLETED','CANCELLED'));
-- VAL-07 with B-BR-EMP-392: a paused trip still holds its asset.
DROP INDEX org_trips_one_active_asset;
CREATE UNIQUE INDEX org_trips_one_active_asset ON org_trips(org_id,asset_id) WHERE state IN('IN_PROGRESS','PAUSED');

CREATE TABLE org_trip_transitions(
 org_id UUID NOT NULL,
 trip_id UUID NOT NULL,
 revision BIGINT NOT NULL CHECK(revision BETWEEN 2 AND 1000000),
 command TEXT NOT NULL CHECK(command IN('PAUSE','RESUME','CANCEL')),
 from_state TEXT NOT NULL CHECK(from_state IN('IN_PROGRESS','PAUSED')),
 to_state TEXT NOT NULL CHECK(to_state IN('IN_PROGRESS','PAUSED','CANCELLED')),
 reason TEXT CHECK(reason IS NULL OR org_asset_text_valid(reason,280,true)),
 operation_id UUID NOT NULL,
 payload_digest TEXT NOT NULL CHECK(payload_digest~'^sha256:[a-f0-9]{64}$'),
 author_membership_id UUID NOT NULL,
 author_employee_id UUID NOT NULL,
 enrollment_id UUID NOT NULL,
 occurred_at TIMESTAMPTZ NOT NULL,
 occurred_submicro SMALLINT NOT NULL CHECK(occurred_submicro BETWEEN 0 AND 999),
 entry_zone TEXT NOT NULL,
 device_offset_seconds BIGINT NOT NULL CHECK(device_offset_seconds BETWEEN -64800 AND 64800),
 received_at TIMESTAMPTZ NOT NULL DEFAULT clock_timestamp(),
 PRIMARY KEY(org_id,trip_id,revision),
 CONSTRAINT org_trip_transitions_operation_key UNIQUE(org_id,operation_id),
 CONSTRAINT org_trip_transitions_trip_fkey FOREIGN KEY(org_id,trip_id) REFERENCES org_trips(org_id,id),
 CONSTRAINT org_trip_transitions_operation_fkey FOREIGN KEY(org_id,operation_id) REFERENCES org_sync_operations(org_id,operation_id),
 CONSTRAINT org_trip_transitions_author_fkey FOREIGN KEY(org_id,author_membership_id) REFERENCES org_memberships(org_id,id),
 CONSTRAINT org_trip_transitions_author_employee_fkey FOREIGN KEY(org_id,author_employee_id) REFERENCES org_employee_identities(org_id,id),
 CONSTRAINT org_trip_transitions_enrollment_fkey FOREIGN KEY(org_id,enrollment_id) REFERENCES org_device_enrollments(org_id,id),
 -- B-BR-EMP-390: the table of the open trip; a cancellation states its reason.
 CONSTRAINT org_trip_transitions_move CHECK((command='PAUSE' AND from_state='IN_PROGRESS' AND to_state='PAUSED')
  OR(command='RESUME' AND from_state='PAUSED' AND to_state='IN_PROGRESS') OR(command='CANCEL' AND to_state='CANCELLED' AND reason IS NOT NULL)),
 CHECK(isfinite(occurred_at) AND occurred_at>='0001-01-01 00:00:00+00'::timestamptz AND occurred_at<'10000-01-01 00:00:00+00'::timestamptz),
 CHECK(isfinite(received_at) AND received_at>'0001-01-01 00:00:00+00'::timestamptz AND received_at<'10000-01-01 00:00:00+00'::timestamptz)
);
CREATE UNIQUE INDEX org_trip_transitions_one_cancel ON org_trip_transitions(org_id,trip_id) WHERE command='CANCEL';

-- OFFL-008 held lifecycle commands: the whole intent, never a trip change.
CREATE TABLE org_trip_held_transitions(
 org_id UUID NOT NULL,review_item_id UUID NOT NULL,operation_id UUID NOT NULL,
 trip_id UUID NOT NULL,
 command TEXT NOT NULL CHECK(command IN('PAUSE','RESUME','CANCEL')),
 expected_revision BIGINT NOT NULL CHECK(expected_revision BETWEEN 1 AND 1000000),
 reason TEXT CHECK(reason IS NULL OR org_asset_text_valid(reason,280,true)),
 occurred_at TIMESTAMPTZ NOT NULL,occurred_submicro SMALLINT NOT NULL CHECK(occurred_submicro BETWEEN 0 AND 999),entry_zone TEXT NOT NULL,
 device_offset_seconds BIGINT NOT NULL CHECK(device_offset_seconds BETWEEN -64800 AND 64800),
 PRIMARY KEY(org_id,review_item_id),UNIQUE(org_id,operation_id),
 CONSTRAINT org_trip_held_transitions_review_fkey FOREIGN KEY(org_id,review_item_id,operation_id) REFERENCES org_sync_review_items(org_id,id,operation_id),
 CONSTRAINT org_trip_held_transitions_trip_fkey FOREIGN KEY(org_id,trip_id) REFERENCES org_trips(org_id,id),
 CHECK(command<>'CANCEL' OR reason IS NOT NULL),
 CHECK(isfinite(occurred_at) AND occurred_at>='0001-01-01 00:00:00+00'::timestamptz AND occurred_at<'10000-01-01 00:00:00+00'::timestamptz)
);

-- The trip's operational revision: 1 after the start, one more per
-- lifecycle row or finish (B-BR-EMP-390).
CREATE FUNCTION org_trip_revision(p_org UUID,p_trip UUID) RETURNS BIGINT LANGUAGE SQL STABLE SET search_path=pg_catalog AS $$
 SELECT 1+(SELECT count(*) FROM public.org_trip_transitions x WHERE x.org_id=p_org AND x.trip_id=p_trip)
  +(SELECT count(*) FROM public.org_trip_finishes f WHERE f.org_id=p_org AND f.trip_id=p_trip)
 $$;
-- The lifecycle table of an open trip; NULL when the command is not admitted.
CREATE FUNCTION org_trip_lifecycle_next(from_state TEXT,command TEXT) RETURNS TEXT LANGUAGE SQL IMMUTABLE SET search_path=pg_catalog AS $$
 SELECT CASE WHEN from_state='IN_PROGRESS' AND command='PAUSE' THEN 'PAUSED' WHEN from_state='PAUSED' AND command='RESUME' THEN 'IN_PROGRESS'
  WHEN from_state IN('IN_PROGRESS','PAUSED') AND command='CANCEL' THEN 'CANCELLED' END
 $$;

-- A lifecycle row re-proves its signed operation, the trip's author, the
-- current state and the next revision under the root and trip locks.
CREATE FUNCTION org_trip_transition_source() RETURNS trigger LANGUAGE plpgsql SET search_path=pg_catalog AS $$
DECLARE t public.org_trips%ROWTYPE;
BEGIN
 IF NEW.org_id IS DISTINCT FROM NULLIF(current_setting('anpfuel.org_id',true),'')::uuid THEN RAISE EXCEPTION USING ERRCODE='42501',MESSAGE='organization scope denied';END IF;
 PERFORM 1 FROM public.organizations WHERE org_id=NEW.org_id AND has_snapshot FOR UPDATE;
 IF NOT FOUND THEN RAISE EXCEPTION USING ERRCODE='23503',MESSAGE='private reference not found';END IF;
 SELECT * INTO t FROM public.org_trips WHERE org_id=NEW.org_id AND id=NEW.trip_id FOR UPDATE;
 IF NOT FOUND THEN RAISE EXCEPTION USING ERRCODE='23503',MESSAGE='private reference not found';END IF;
 IF NOT public.org_trip_operation_valid(NEW.org_id,NEW.operation_id,'trip.'||lower(NEW.command),NEW.trip_id,NEW.payload_digest,NEW.author_membership_id,NEW.enrollment_id,t.author_employee_id)
 OR NEW.author_membership_id<>t.author_membership_id OR NEW.author_employee_id<>t.author_employee_id
 THEN RAISE EXCEPTION USING ERRCODE='23514',MESSAGE='trip transition source unavailable';END IF;
 IF NEW.from_state<>t.state OR NEW.to_state IS DISTINCT FROM public.org_trip_lifecycle_next(t.state,NEW.command)
 OR NEW.revision<>public.org_trip_revision(NEW.org_id,NEW.trip_id)+1
 THEN RAISE EXCEPTION USING ERRCODE='23514',MESSAGE='trip transition stale';END IF;
 IF NOT EXISTS(SELECT 1 FROM pg_timezone_names WHERE name=NEW.entry_zone) OR(NEW.occurred_at,NEW.occurred_submicro)<(t.occurred_at,t.occurred_submicro)
 THEN RAISE EXCEPTION USING ERRCODE='23514',MESSAGE='invalid trip transition';END IF;
 NEW.received_at=clock_timestamp();RETURN NEW;
END $$;
CREATE TRIGGER org_trip_transition_source BEFORE INSERT ON org_trip_transitions FOR EACH ROW EXECUTE FUNCTION org_trip_transition_source();
CREATE FUNCTION org_trip_transition_apply() RETURNS trigger LANGUAGE plpgsql SET search_path=pg_catalog AS $$
BEGIN
 UPDATE public.org_trips SET state=NEW.to_state WHERE org_id=NEW.org_id AND id=NEW.trip_id AND state=NEW.from_state;
 IF NOT FOUND THEN RAISE EXCEPTION USING ERRCODE='23514',MESSAGE='trip transition stale';END IF;
 RETURN NULL;
END $$;
CREATE TRIGGER org_trip_transition_apply AFTER INSERT ON org_trip_transitions FOR EACH ROW EXECUTE FUNCTION org_trip_transition_apply();
-- At commit a lifecycle row is published only with its ACCEPTED receipt and
-- effect at its revision.
CREATE FUNCTION org_trip_transition_complete() RETURNS trigger LANGUAGE plpgsql SET search_path=pg_catalog AS $$
DECLARE prior_org TEXT;
BEGIN
 prior_org=current_setting('anpfuel.org_id',true);PERFORM set_config('anpfuel.org_id',NEW.org_id::text,true);
 IF NOT EXISTS(SELECT 1 FROM public.org_trip_transitions WHERE org_id=NEW.org_id AND trip_id=NEW.trip_id AND revision=NEW.revision AND operation_id=NEW.operation_id)
 OR NOT public.org_trip_published(NEW.org_id,NEW.operation_id,NEW.revision)
 THEN RAISE EXCEPTION USING ERRCODE='23514',MESSAGE='trip transition publication unavailable';END IF;
 PERFORM set_config('anpfuel.org_id',COALESCE(prior_org,''),true);RETURN NULL;
END $$;
CREATE CONSTRAINT TRIGGER org_trip_transition_complete AFTER INSERT ON org_trip_transitions DEFERRABLE INITIALLY DEFERRED FOR EACH ROW EXECUTE FUNCTION org_trip_transition_complete();

-- A held lifecycle command re-proves its source and its condition: a closed
-- trip (TRIP_ALREADY_CLOSED), or an open trip whose revision is not the one
-- named or whose state no longer admits the command (STALE_REVISION).
CREATE FUNCTION org_trip_held_transition_source() RETURNS trigger LANGUAGE plpgsql SET search_path=pg_catalog AS $$
DECLARE i public.org_sync_review_items%ROWTYPE;t public.org_trips%ROWTYPE;
BEGIN
 IF NEW.org_id IS DISTINCT FROM NULLIF(current_setting('anpfuel.org_id',true),'')::uuid THEN RAISE EXCEPTION USING ERRCODE='42501',MESSAGE='organization scope denied';END IF;
 PERFORM 1 FROM public.organizations WHERE org_id=NEW.org_id AND has_snapshot FOR UPDATE;
 SELECT * INTO i FROM public.org_sync_review_items WHERE org_id=NEW.org_id AND id=NEW.review_item_id AND operation_id=NEW.operation_id;
 IF NOT FOUND THEN RAISE EXCEPTION USING ERRCODE='23514',MESSAGE='held trip transition intent unavailable';END IF;
 SELECT * INTO t FROM public.org_trips WHERE org_id=NEW.org_id AND id=NEW.trip_id FOR UPDATE;
 IF NOT FOUND THEN RAISE EXCEPTION USING ERRCODE='23503',MESSAGE='private reference not found';END IF;
 IF i.operation_type<>'trip.'||lower(NEW.command) OR i.operation_schema<>1 OR i.code NOT IN('TRIP_ALREADY_CLOSED','STALE_REVISION')
 OR NOT public.org_trip_operation_valid(NEW.org_id,NEW.operation_id,i.operation_type,NEW.trip_id,i.payload_digest,t.author_membership_id,NULL,t.author_employee_id)
 OR i.author_employee_id<>t.author_employee_id OR NOT EXISTS(SELECT 1 FROM pg_timezone_names WHERE name=NEW.entry_zone)
 OR EXISTS(SELECT 1 FROM public.org_trip_transitions x WHERE x.org_id=NEW.org_id AND x.operation_id=NEW.operation_id)
 OR(i.code='TRIP_ALREADY_CLOSED' AND t.state NOT IN('COMPLETED','CANCELLED'))
 OR(i.code='STALE_REVISION' AND(t.state NOT IN('IN_PROGRESS','PAUSED')
  OR(NEW.expected_revision=public.org_trip_revision(NEW.org_id,NEW.trip_id) AND public.org_trip_lifecycle_next(t.state,NEW.command) IS NOT NULL)))
 THEN RAISE EXCEPTION USING ERRCODE='23514',MESSAGE='held trip transition source unavailable';END IF;
 RETURN NEW;
END $$;
CREATE TRIGGER org_trip_held_transition_source BEFORE INSERT ON org_trip_held_transitions FOR EACH ROW EXECUTE FUNCTION org_trip_held_transition_source();
CREATE CONSTRAINT TRIGGER org_trip_held_transition_complete AFTER INSERT ON org_trip_held_transitions DEFERRABLE INITIALLY DEFERRED FOR EACH ROW EXECUTE FUNCTION org_sync_review_complete();

-- The 220 guard: the trip row changes only its state, to COMPLETED with its
-- finish (now also from PAUSED) or with the lifecycle row of its latest revision.
CREATE OR REPLACE FUNCTION org_trip_completion_guard() RETURNS trigger LANGUAGE plpgsql SET search_path=pg_catalog AS $$
BEGIN
 IF to_jsonb(NEW)-'state'<>to_jsonb(OLD)-'state' OR NOT(
  (OLD.state IN('IN_PROGRESS','PAUSED') AND NEW.state='COMPLETED' AND EXISTS(SELECT 1 FROM public.org_trip_finishes f WHERE f.org_id=OLD.org_id AND f.trip_id=OLD.id))
  -- EMP-TRIP-011: a lifecycle move only with the transition row of the trip's latest revision.
  OR EXISTS(SELECT 1 FROM public.org_trip_transitions x WHERE x.org_id=OLD.org_id AND x.trip_id=OLD.id AND x.from_state=OLD.state AND x.to_state=NEW.state
   AND x.revision=public.org_trip_revision(OLD.org_id,OLD.id)))
 THEN RAISE EXCEPTION USING ERRCODE='55000',MESSAGE='organization trip history is insert-only';END IF;
 RETURN NEW;
END $$;

-- The 220 finish source: a paused trip may be finished.
CREATE OR REPLACE FUNCTION org_trip_finish_source() RETURNS trigger LANGUAGE plpgsql SET search_path=pg_catalog AS $$
DECLARE t public.org_trips%ROWTYPE;
BEGIN
 IF NEW.org_id IS DISTINCT FROM NULLIF(current_setting('anpfuel.org_id',true),'')::uuid THEN RAISE EXCEPTION USING ERRCODE='42501',MESSAGE='organization scope denied';END IF;
 PERFORM 1 FROM public.organizations WHERE org_id=NEW.org_id AND has_snapshot FOR UPDATE;
 SELECT * INTO t FROM public.org_trips WHERE org_id=NEW.org_id AND id=NEW.trip_id FOR UPDATE;
 IF NOT FOUND THEN RAISE EXCEPTION USING ERRCODE='23503',MESSAGE='private reference not found';END IF;
 IF NOT public.org_trip_operation_valid(NEW.org_id,NEW.operation_id,'trip.finish',NEW.trip_id,NEW.payload_digest,NEW.author_membership_id,NEW.enrollment_id,t.author_employee_id)
 OR NEW.author_membership_id<>t.author_membership_id THEN RAISE EXCEPTION USING ERRCODE='23514',MESSAGE='trip finish source unavailable';END IF;
 IF t.state NOT IN('IN_PROGRESS','PAUSED') THEN RAISE EXCEPTION USING ERRCODE='23514',MESSAGE='trip already closed';END IF;
 IF(NEW.occurred_at,NEW.occurred_submicro)<(t.occurred_at,t.occurred_submicro) OR NOT EXISTS(SELECT 1 FROM pg_timezone_names WHERE name=NEW.entry_zone)
 OR NEW.end_day<>(NEW.occurred_at AT TIME ZONE t.reporting_zone)::date THEN RAISE EXCEPTION USING ERRCODE='23514',MESSAGE='invalid trip finish';END IF;
 IF NEW.meter_id IS NOT NULL AND NOT EXISTS(SELECT 1 FROM public.org_meters m WHERE m.org_id=NEW.org_id AND m.id=NEW.meter_id AND m.asset_id=t.asset_id)
 THEN RAISE EXCEPTION USING ERRCODE='23514',MESSAGE='trip finish reading unavailable';END IF;
 NEW.received_at=clock_timestamp();RETURN NEW;
END $$;
-- The 220 finish apply: completes an IN_PROGRESS or PAUSED trip.
CREATE OR REPLACE FUNCTION org_trip_finish_apply() RETURNS trigger LANGUAGE plpgsql SET search_path=pg_catalog AS $$
BEGIN
 UPDATE public.org_trips SET state='COMPLETED' WHERE org_id=NEW.org_id AND id=NEW.trip_id AND state IN('IN_PROGRESS','PAUSED');
 IF NOT FOUND THEN RAISE EXCEPTION USING ERRCODE='23514',MESSAGE='trip already closed';END IF;
 RETURN NULL;
END $$;

-- The 220 publication: a finish is published at the trip revision after it.
CREATE OR REPLACE FUNCTION org_trip_complete() RETURNS trigger LANGUAGE plpgsql SET search_path=pg_catalog AS $$
DECLARE t public.org_trips%ROWTYPE;f public.org_trip_finishes%ROWTYPE;prior_org TEXT;
BEGIN
 prior_org=current_setting('anpfuel.org_id',true);PERFORM set_config('anpfuel.org_id',NEW.org_id::text,true);
 IF TG_TABLE_NAME='org_trips' THEN
  SELECT * INTO t FROM public.org_trips WHERE org_id=NEW.org_id AND id=NEW.id;
  IF NOT FOUND OR NOT public.org_trip_published(t.org_id,t.operation_id,1)
  OR NOT public.org_trip_reading_valid(t.org_id,t.asset_id,t.meter_id,t.segment_id,t.reading_id,'TRIP_START',t.operation_id,t.author_membership_id,t.enrollment_id,t.occurred_at,t.occurred_submicro,t.entry_zone,t.device_offset_seconds)
  THEN RAISE EXCEPTION USING ERRCODE='23514',MESSAGE='trip publication unavailable';END IF;
 ELSE
  SELECT * INTO f FROM public.org_trip_finishes WHERE org_id=NEW.org_id AND trip_id=NEW.trip_id;
  SELECT * INTO t FROM public.org_trips WHERE org_id=NEW.org_id AND id=NEW.trip_id;
  IF f.operation_id IS NULL OR t.state IS DISTINCT FROM 'COMPLETED' OR NOT public.org_trip_published(f.org_id,f.operation_id,public.org_trip_revision(f.org_id,f.trip_id))
  OR NOT public.org_trip_reading_valid(f.org_id,t.asset_id,f.meter_id,f.segment_id,f.reading_id,'TRIP_END',f.operation_id,f.author_membership_id,f.enrollment_id,f.occurred_at,f.occurred_submicro,f.entry_zone,f.device_offset_seconds)
  THEN RAISE EXCEPTION USING ERRCODE='23514',MESSAGE='trip finish publication unavailable';END IF;
 END IF;
 PERFORM set_config('anpfuel.org_id',COALESCE(prior_org,''),true);RETURN NULL;
END $$;

-- The 220 held source: ASSET_BUSY counts paused trips and a cancelled trip
-- until its cancellation; a finish of a cancelled trip is TRIP_ALREADY_CLOSED.
CREATE OR REPLACE FUNCTION org_trip_held_source() RETURNS trigger LANGUAGE plpgsql SET search_path=pg_catalog AS $$
DECLARE i public.org_sync_review_items%ROWTYPE;t public.org_trips%ROWTYPE;
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
  IF i.operation_type<>'trip.finish' OR i.operation_schema<>1 OR i.code<>'TRIP_ALREADY_CLOSED' OR t.state NOT IN('COMPLETED','CANCELLED')
  OR NOT public.org_trip_operation_valid(NEW.org_id,NEW.operation_id,'trip.finish',NEW.trip_id,i.payload_digest,t.author_membership_id,NULL,t.author_employee_id)
  OR EXISTS(SELECT 1 FROM public.org_trip_finishes f WHERE f.org_id=NEW.org_id AND f.operation_id=NEW.operation_id)
  OR(NEW.occurred_at,NEW.occurred_submicro)<(t.occurred_at,t.occurred_submicro)
  OR(NEW.meter_id IS NOT NULL AND NOT EXISTS(SELECT 1 FROM public.org_meters m WHERE m.org_id=NEW.org_id AND m.id=NEW.meter_id AND m.asset_id=t.asset_id))
  THEN RAISE EXCEPTION USING ERRCODE='23514',MESSAGE='held trip finish source unavailable';END IF;
 END IF;
 RETURN NEW;
END $$;

-- The 229 logbook source with the cancellation closing the logbook.
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
 -- EMP-TRIP-015: a submitted, decided or amended revision takes no direct fact.
 IF public.org_trip_revision_closed(NEW.org_id,NEW.trip_id) THEN RAISE EXCEPTION USING ERRCODE='23514',MESSAGE='trip revision closed';END IF;
 NEW.received_at=clock_timestamp();RETURN NEW;
END $$;

-- The 229 held-logbook source: a cancelled trip closes at its cancellation.
CREATE OR REPLACE FUNCTION org_trip_held_event_source() RETURNS trigger LANGUAGE plpgsql SET search_path=pg_catalog AS $$
DECLARE i public.org_sync_review_items%ROWTYPE;t public.org_trips%ROWTYPE;f public.org_trip_finishes%ROWTYPE;end_at TIMESTAMPTZ;end_sub SMALLINT;
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
 IF i.operation_type<>'trip.event.record' OR i.operation_schema<>1 OR i.code NOT IN('TRIP_ALREADY_CLOSED','STALE_REVISION') OR t.state NOT IN('COMPLETED','CANCELLED') OR end_at IS NULL
 OR NOT public.org_trip_event_operation_valid(NEW.org_id,NEW.operation_id,NEW.id,i.payload_digest,t.author_membership_id,NULL,t.author_employee_id)
 OR i.author_employee_id<>t.author_employee_id
 OR(i.code='TRIP_ALREADY_CLOSED' AND(NEW.occurred_at,NEW.occurred_submicro)<=(end_at,end_sub))
 -- EMP-TRIP-015: within the trip, but its revision is submitted, decided or amended.
 OR(i.code='STALE_REVISION' AND(t.state<>'COMPLETED' OR(NEW.occurred_at,NEW.occurred_submicro)>(f.occurred_at,f.occurred_submicro)
  OR(NEW.occurred_at,NEW.occurred_submicro)<(t.occurred_at,t.occurred_submicro) OR NOT public.org_trip_revision_closed(NEW.org_id,NEW.trip_id)))
 OR NOT EXISTS(SELECT 1 FROM pg_timezone_names WHERE name=NEW.entry_zone)
 THEN RAISE EXCEPTION USING ERRCODE='23514',MESSAGE='held trip event source unavailable';END IF;
 IF EXISTS(SELECT 1 FROM public.org_trip_events e WHERE e.org_id=NEW.org_id AND(e.id=NEW.id OR e.operation_id=NEW.operation_id))
 THEN RAISE EXCEPTION USING ERRCODE='23505',CONSTRAINT='org_trip_events_pkey',MESSAGE='trip event identity unavailable';END IF;
 RETURN NEW;
END $$;

-- The 232 mission vehicle source: a paused trip still runs on its vehicle.
CREATE OR REPLACE FUNCTION org_trip_mission_vehicle_source() RETURNS trigger LANGUAGE plpgsql SET search_path=pg_catalog AS $$
DECLARE t public.org_trip_mission_transitions%ROWTYPE;m public.org_trip_missions%ROWTYPE;
BEGIN
 IF NEW.org_id IS DISTINCT FROM NULLIF(current_setting('anpfuel.org_id',true),'')::uuid THEN RAISE EXCEPTION USING ERRCODE='42501',MESSAGE='organization scope denied';END IF;
 PERFORM 1 FROM public.organizations WHERE org_id=NEW.org_id AND has_snapshot FOR UPDATE;
 IF NOT FOUND THEN RAISE EXCEPTION USING ERRCODE='23503',MESSAGE='private reference not found';END IF;
 SELECT * INTO t FROM public.org_trip_mission_transitions WHERE org_id=NEW.org_id AND mission_id=NEW.mission_id AND revision=NEW.revision;
 SELECT * INTO m FROM public.org_trip_missions WHERE org_id=NEW.org_id AND id=NEW.mission_id FOR UPDATE;
 IF t.kind IS DISTINCT FROM 'VEHICLE' OR t.operation_id<>NEW.operation_id OR t.actor_account_id<>NEW.changed_by OR t.actor_membership_id<>NEW.changed_by_membership_id
 OR NOT public.org_trip_mission_actor_valid(NEW.org_id,NEW.changed_by_membership_id,NEW.changed_by)
 OR m.revision IS DISTINCT FROM NEW.revision-1 OR m.asset_id IS DISTINCT FROM NEW.previous_asset_id
 THEN RAISE EXCEPTION USING ERRCODE='23514',MESSAGE='mission vehicle source unavailable';END IF;
 IF NOT EXISTS(SELECT 1 FROM public.org_assets a WHERE a.org_id=NEW.org_id AND a.id=NEW.asset_id AND a.state='ACTIVE' AND a.kind<>'IMPLEMENT')
 THEN RAISE EXCEPTION USING ERRCODE='23514',MESSAGE='mission vehicle unavailable';END IF;
 IF EXISTS(SELECT 1 FROM public.org_trips r WHERE r.org_id=NEW.org_id AND r.asset_id=NEW.asset_id AND r.state IN('IN_PROGRESS','PAUSED'))
 THEN RAISE EXCEPTION USING ERRCODE='23514',MESSAGE='mission vehicle in use';END IF;
 IF NEW.previous_asset_id IS NOT NULL AND m.assignee_employee_id IS NOT NULL AND EXISTS(SELECT 1 FROM public.org_trips r WHERE r.org_id=NEW.org_id
  AND r.asset_id=NEW.previous_asset_id AND r.driver_employee_id=m.assignee_employee_id AND r.state IN('IN_PROGRESS','PAUSED'))
 THEN RAISE EXCEPTION USING ERRCODE='23514',MESSAGE='mission trip in progress';END IF;
 NEW.recorded_at=clock_timestamp();RETURN NEW;
END $$;

-- The 229 review-source hook with the three lifecycle owners; every other
-- branch is unchanged and unknown owners still fail closed.
CREATE OR REPLACE FUNCTION org_sync_review_source_valid(org UUID,review UUID,owner_type TEXT,owner_schema BIGINT) RETURNS BOOLEAN
 LANGUAGE SQL VOLATILE SET search_path=pg_catalog AS $$
 SELECT(owner_type='meter.reading.record' AND owner_schema=1 AND EXISTS(
 SELECT 1 FROM public.org_sync_review_items i JOIN public.org_meter_held_readings h ON(h.org_id,h.review_item_id,h.operation_id)=(i.org_id,i.id,i.operation_id)
 JOIN public.org_sync_operations o ON(o.org_id,o.operation_id,o.operation_type,o.operation_schema,o.payload_digest)=(i.org_id,i.operation_id,i.operation_type,i.operation_schema,i.payload_digest)
 JOIN public.org_sync_acknowledgements a ON(a.org_id,a.operation_id,a.review_item_id,a.result,a.code,a.review_proof_version)=(i.org_id,i.operation_id,i.id,i.acknowledgement_result,i.code,i.proof_version)
 JOIN public.org_memberships m ON(m.org_id,m.id,m.employee_id)=(i.org_id,o.membership_id,i.author_employee_id)
 WHERE i.org_id=org AND i.id=review AND i.operation_type=owner_type AND i.operation_schema=owner_schema AND o.entity_type='meter_reading' AND o.entity_id=h.reading_id))
 OR(owner_type='trip.start' AND owner_schema=1 AND EXISTS(
 SELECT 1 FROM public.org_sync_review_items i JOIN public.org_trip_held_starts h ON(h.org_id,h.review_item_id,h.operation_id)=(i.org_id,i.id,i.operation_id)
 JOIN public.org_sync_operations o ON(o.org_id,o.operation_id,o.operation_type,o.operation_schema,o.payload_digest)=(i.org_id,i.operation_id,i.operation_type,i.operation_schema,i.payload_digest)
 JOIN public.org_sync_acknowledgements a ON(a.org_id,a.operation_id,a.review_item_id,a.result,a.code,a.review_proof_version)=(i.org_id,i.operation_id,i.id,i.acknowledgement_result,i.code,i.proof_version)
 JOIN public.org_memberships m ON(m.org_id,m.id,m.employee_id)=(i.org_id,o.membership_id,i.author_employee_id)
 WHERE i.org_id=org AND i.id=review AND i.operation_type=owner_type AND i.operation_schema=owner_schema AND o.entity_type='trip' AND o.entity_id=h.trip_id))
 OR(owner_type='trip.finish' AND owner_schema=1 AND EXISTS(
 SELECT 1 FROM public.org_sync_review_items i JOIN public.org_trip_held_finishes h ON(h.org_id,h.review_item_id,h.operation_id)=(i.org_id,i.id,i.operation_id)
 JOIN public.org_sync_operations o ON(o.org_id,o.operation_id,o.operation_type,o.operation_schema,o.payload_digest)=(i.org_id,i.operation_id,i.operation_type,i.operation_schema,i.payload_digest)
 JOIN public.org_sync_acknowledgements a ON(a.org_id,a.operation_id,a.review_item_id,a.result,a.code,a.review_proof_version)=(i.org_id,i.operation_id,i.id,i.acknowledgement_result,i.code,i.proof_version)
 JOIN public.org_memberships m ON(m.org_id,m.id,m.employee_id)=(i.org_id,o.membership_id,i.author_employee_id)
 WHERE i.org_id=org AND i.id=review AND i.operation_type=owner_type AND i.operation_schema=owner_schema AND o.entity_type='trip' AND o.entity_id=h.trip_id))
 OR(owner_type='trip.event.record' AND owner_schema=1 AND EXISTS(
 SELECT 1 FROM public.org_sync_review_items i JOIN public.org_trip_held_events h ON(h.org_id,h.review_item_id,h.operation_id)=(i.org_id,i.id,i.operation_id)
 JOIN public.org_sync_operations o ON(o.org_id,o.operation_id,o.operation_type,o.operation_schema,o.payload_digest)=(i.org_id,i.operation_id,i.operation_type,i.operation_schema,i.payload_digest)
 JOIN public.org_sync_acknowledgements a ON(a.org_id,a.operation_id,a.review_item_id,a.result,a.code,a.review_proof_version)=(i.org_id,i.operation_id,i.id,i.acknowledgement_result,i.code,i.proof_version)
 JOIN public.org_memberships m ON(m.org_id,m.id,m.employee_id)=(i.org_id,o.membership_id,i.author_employee_id)
 WHERE i.org_id=org AND i.id=review AND i.operation_type=owner_type AND i.operation_schema=owner_schema AND o.entity_type='trip_event' AND o.entity_id=h.id))
 OR(owner_type='trip.submit' AND owner_schema=1 AND EXISTS(
 SELECT 1 FROM public.org_sync_review_items i JOIN public.org_trip_held_submissions h ON(h.org_id,h.review_item_id,h.operation_id)=(i.org_id,i.id,i.operation_id)
 JOIN public.org_sync_operations o ON(o.org_id,o.operation_id,o.operation_type,o.operation_schema,o.payload_digest)=(i.org_id,i.operation_id,i.operation_type,i.operation_schema,i.payload_digest)
 JOIN public.org_sync_acknowledgements a ON(a.org_id,a.operation_id,a.review_item_id,a.result,a.code,a.review_proof_version)=(i.org_id,i.operation_id,i.id,i.acknowledgement_result,i.code,i.proof_version)
 JOIN public.org_memberships m ON(m.org_id,m.id,m.employee_id)=(i.org_id,o.membership_id,i.author_employee_id)
 WHERE i.org_id=org AND i.id=review AND i.operation_type=owner_type AND i.operation_schema=owner_schema AND o.entity_type='trip_review' AND o.entity_id=h.trip_id))
 OR(owner_type IN('trip.pause','trip.resume','trip.cancel') AND owner_schema=1 AND EXISTS(
 SELECT 1 FROM public.org_sync_review_items i JOIN public.org_trip_held_transitions h ON(h.org_id,h.review_item_id,h.operation_id)=(i.org_id,i.id,i.operation_id)
 JOIN public.org_sync_operations o ON(o.org_id,o.operation_id,o.operation_type,o.operation_schema,o.payload_digest)=(i.org_id,i.operation_id,i.operation_type,i.operation_schema,i.payload_digest)
 JOIN public.org_sync_acknowledgements a ON(a.org_id,a.operation_id,a.review_item_id,a.result,a.code,a.review_proof_version)=(i.org_id,i.operation_id,i.id,i.acknowledgement_result,i.code,i.proof_version)
 JOIN public.org_memberships m ON(m.org_id,m.id,m.employee_id)=(i.org_id,o.membership_id,i.author_employee_id)
 WHERE i.org_id=org AND i.id=review AND i.operation_type=owner_type AND i.operation_schema=owner_schema AND o.entity_type='trip' AND o.entity_id=h.trip_id
 AND owner_type='trip.'||lower(h.command)))
 $$;

DO $$DECLARE name TEXT;BEGIN
 FOREACH name IN ARRAY ARRAY['org_trip_transitions','org_trip_held_transitions'] LOOP
 EXECUTE format('ALTER TABLE %I ENABLE ROW LEVEL SECURITY',name);EXECUTE format('ALTER TABLE %I FORCE ROW LEVEL SECURITY',name);
 EXECUTE format('CREATE POLICY %I ON %I USING(org_id=NULLIF(current_setting(''anpfuel.org_id'',true),'''')::uuid) WITH CHECK(org_id=NULLIF(current_setting(''anpfuel.org_id'',true),'''')::uuid)',name||'_scope',name);
 EXECUTE format('CREATE TRIGGER %I BEFORE UPDATE OR DELETE OR TRUNCATE ON %I FOR EACH STATEMENT EXECUTE FUNCTION org_trip_keep_history()',name||'_immutable',name);
 EXECUTE format('REVOKE ALL ON %I FROM PUBLIC',name);
 END LOOP;
END $$;

-- EMP-TRIP-003. Typed logbook events appended to a driver-initiated trip
-- through the signed intake (B-BR-EMP-320..324, EDR-207..209). An accepted
-- event is one insert-only fact of its trip: stable device identity, type,
-- optional text, author, enrollment, the device declaration and the server
-- receipt. It never changes the trip. An event declared after the finish of
-- a completed trip is kept as an OFFL-008 held fact (TRIP_ALREADY_CLOSED).
-- Events reference only the trip, the operation and the author records: no
-- asset, unit or cost-centre key, so no usage source changes.
CREATE TABLE org_trip_events(
 org_id UUID NOT NULL,
 id UUID NOT NULL CHECK(id<>'00000000-0000-0000-0000-000000000000'::uuid),
 trip_id UUID NOT NULL,
 operation_id UUID NOT NULL,
 payload_digest TEXT NOT NULL CHECK(payload_digest~'^sha256:[a-f0-9]{64}$'),
 event_type TEXT NOT NULL CHECK(event_type IN('NOTE','STOP','ARRIVAL','DEPARTURE')),
 event_text TEXT CHECK(event_text IS NULL OR org_asset_text_valid(event_text,280,true)),
 author_membership_id UUID NOT NULL,
 author_employee_id UUID NOT NULL,
 enrollment_id UUID NOT NULL,
 occurred_at TIMESTAMPTZ NOT NULL,
 occurred_submicro SMALLINT NOT NULL CHECK(occurred_submicro BETWEEN 0 AND 999),
 entry_zone TEXT NOT NULL,
 device_offset_seconds BIGINT NOT NULL CHECK(device_offset_seconds BETWEEN -64800 AND 64800),
 received_at TIMESTAMPTZ NOT NULL DEFAULT clock_timestamp(),
 PRIMARY KEY(org_id,id),
 CONSTRAINT org_trip_events_operation_key UNIQUE(org_id,operation_id),
 CONSTRAINT org_trip_events_trip_fkey FOREIGN KEY(org_id,trip_id) REFERENCES org_trips(org_id,id),
 CONSTRAINT org_trip_events_operation_fkey FOREIGN KEY(org_id,operation_id) REFERENCES org_sync_operations(org_id,operation_id),
 CONSTRAINT org_trip_events_author_fkey FOREIGN KEY(org_id,author_membership_id) REFERENCES org_memberships(org_id,id),
 CONSTRAINT org_trip_events_author_employee_fkey FOREIGN KEY(org_id,author_employee_id) REFERENCES org_employee_identities(org_id,id),
 CONSTRAINT org_trip_events_enrollment_fkey FOREIGN KEY(org_id,enrollment_id) REFERENCES org_device_enrollments(org_id,id),
 CONSTRAINT org_trip_events_note_text CHECK(event_type<>'NOTE' OR event_text IS NOT NULL),
 CHECK(isfinite(occurred_at) AND occurred_at>='0001-01-01 00:00:00+00'::timestamptz AND occurred_at<'10000-01-01 00:00:00+00'::timestamptz),
 CHECK(isfinite(received_at) AND received_at>'0001-01-01 00:00:00+00'::timestamptz AND received_at<'10000-01-01 00:00:00+00'::timestamptz)
);
CREATE INDEX org_trip_events_trip ON org_trip_events(org_id,trip_id,occurred_at,occurred_submicro,id);

-- OFFL-008 held events: the whole fact, never a logbook fact of the trip.
CREATE TABLE org_trip_held_events(
 org_id UUID NOT NULL,review_item_id UUID NOT NULL,operation_id UUID NOT NULL,
 id UUID NOT NULL CHECK(id<>'00000000-0000-0000-0000-000000000000'::uuid),
 trip_id UUID NOT NULL,
 event_type TEXT NOT NULL CHECK(event_type IN('NOTE','STOP','ARRIVAL','DEPARTURE')),
 event_text TEXT CHECK(event_text IS NULL OR org_asset_text_valid(event_text,280,true)),
 occurred_at TIMESTAMPTZ NOT NULL,occurred_submicro SMALLINT NOT NULL CHECK(occurred_submicro BETWEEN 0 AND 999),entry_zone TEXT NOT NULL,
 device_offset_seconds BIGINT NOT NULL CHECK(device_offset_seconds BETWEEN -64800 AND 64800),
 PRIMARY KEY(org_id,review_item_id),UNIQUE(org_id,operation_id),UNIQUE(org_id,id),
 CONSTRAINT org_trip_held_events_review_fkey FOREIGN KEY(org_id,review_item_id,operation_id) REFERENCES org_sync_review_items(org_id,id,operation_id),
 CONSTRAINT org_trip_held_events_trip_fkey FOREIGN KEY(org_id,trip_id) REFERENCES org_trips(org_id,id),
 CHECK(event_type<>'NOTE' OR event_text IS NOT NULL),
 CHECK(isfinite(occurred_at) AND occurred_at>='0001-01-01 00:00:00+00'::timestamptz AND occurred_at<'10000-01-01 00:00:00+00'::timestamptz)
);

-- The device operation of a logbook event: exact type, entity, digest and an
-- active enrolled author whose employee record is the given one.
CREATE FUNCTION org_trip_event_operation_valid(p_org UUID,p_op UUID,p_event UUID,p_digest TEXT,p_member UUID,p_enrollment UUID,p_employee UUID) RETURNS BOOLEAN
 LANGUAGE SQL STABLE SET search_path=pg_catalog AS $$
 SELECT EXISTS(SELECT 1 FROM public.org_sync_operations o JOIN public.org_memberships m ON(m.org_id,m.id)=(o.org_id,o.membership_id)
  JOIN public.org_device_enrollments e ON(e.org_id,e.id,e.membership_id)=(o.org_id,o.enrollment_id,o.membership_id)
  WHERE o.org_id=p_org AND o.operation_id=p_op AND o.operation_type='trip.event.record' AND o.operation_schema=1 AND o.entity_type='trip_event' AND o.entity_id=p_event
  AND(p_digest IS NULL OR o.payload_digest=p_digest) AND(p_member IS NULL OR o.membership_id=p_member) AND(p_enrollment IS NULL OR o.enrollment_id=p_enrollment)
  AND m.employee_id=p_employee AND m.account_id=o.owner_account_id AND m.state='ACTIVE' AND e.state='ENROLLED')
 $$;

-- B-BR-EMP-321/323: the source, the trip author and the time window, decided
-- under the organization root and trip row locks (the finish trigger locks
-- the same trip row).
CREATE FUNCTION org_trip_event_source() RETURNS trigger LANGUAGE plpgsql SET search_path=pg_catalog AS $$
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
 NEW.received_at=clock_timestamp();RETURN NEW;
END $$;
CREATE TRIGGER org_trip_event_source BEFORE INSERT ON org_trip_events FOR EACH ROW EXECUTE FUNCTION org_trip_event_source();

-- At commit an event is published only with its ACCEPTED receipt and effect.
CREATE FUNCTION org_trip_event_complete() RETURNS trigger LANGUAGE plpgsql SET search_path=pg_catalog AS $$
DECLARE prior_org TEXT;
BEGIN
 prior_org=current_setting('anpfuel.org_id',true);PERFORM set_config('anpfuel.org_id',NEW.org_id::text,true);
 IF NOT EXISTS(SELECT 1 FROM public.org_trip_events WHERE org_id=NEW.org_id AND id=NEW.id AND operation_id=NEW.operation_id)
 OR NOT public.org_trip_published(NEW.org_id,NEW.operation_id,1)
 THEN RAISE EXCEPTION USING ERRCODE='23514',MESSAGE='trip event publication unavailable';END IF;
 PERFORM set_config('anpfuel.org_id',COALESCE(prior_org,''),true);RETURN NULL;
END $$;
CREATE CONSTRAINT TRIGGER org_trip_event_complete AFTER INSERT ON org_trip_events DEFERRABLE INITIALLY DEFERRED FOR EACH ROW EXECUTE FUNCTION org_trip_event_complete();

-- A held event re-proves its source and TRIP_ALREADY_CLOSED: a completed
-- trip of the same author whose finish is before the event's instant.
CREATE FUNCTION org_trip_held_event_source() RETURNS trigger LANGUAGE plpgsql SET search_path=pg_catalog AS $$
DECLARE i public.org_sync_review_items%ROWTYPE;t public.org_trips%ROWTYPE;f public.org_trip_finishes%ROWTYPE;
BEGIN
 IF NEW.org_id IS DISTINCT FROM NULLIF(current_setting('anpfuel.org_id',true),'')::uuid THEN RAISE EXCEPTION USING ERRCODE='42501',MESSAGE='organization scope denied';END IF;
 PERFORM 1 FROM public.organizations WHERE org_id=NEW.org_id AND has_snapshot FOR UPDATE;
 SELECT * INTO i FROM public.org_sync_review_items WHERE org_id=NEW.org_id AND id=NEW.review_item_id AND operation_id=NEW.operation_id;
 IF NOT FOUND THEN RAISE EXCEPTION USING ERRCODE='23514',MESSAGE='held trip event intent unavailable';END IF;
 SELECT * INTO t FROM public.org_trips WHERE org_id=NEW.org_id AND id=NEW.trip_id FOR UPDATE;
 IF NOT FOUND THEN RAISE EXCEPTION USING ERRCODE='23503',MESSAGE='private reference not found';END IF;
 SELECT * INTO f FROM public.org_trip_finishes WHERE org_id=NEW.org_id AND trip_id=NEW.trip_id;
 IF i.operation_type<>'trip.event.record' OR i.operation_schema<>1 OR i.code<>'TRIP_ALREADY_CLOSED' OR t.state<>'COMPLETED' OR NOT FOUND
 OR NOT public.org_trip_event_operation_valid(NEW.org_id,NEW.operation_id,NEW.id,i.payload_digest,t.author_membership_id,NULL,t.author_employee_id)
 OR i.author_employee_id<>t.author_employee_id
 OR(NEW.occurred_at,NEW.occurred_submicro)<=(f.occurred_at,f.occurred_submicro)
 OR NOT EXISTS(SELECT 1 FROM pg_timezone_names WHERE name=NEW.entry_zone)
 THEN RAISE EXCEPTION USING ERRCODE='23514',MESSAGE='held trip event source unavailable';END IF;
 IF EXISTS(SELECT 1 FROM public.org_trip_events e WHERE e.org_id=NEW.org_id AND(e.id=NEW.id OR e.operation_id=NEW.operation_id))
 THEN RAISE EXCEPTION USING ERRCODE='23505',CONSTRAINT='org_trip_events_pkey',MESSAGE='trip event identity unavailable';END IF;
 RETURN NEW;
END $$;
CREATE TRIGGER org_trip_held_event_source BEFORE INSERT ON org_trip_held_events FOR EACH ROW EXECUTE FUNCTION org_trip_held_event_source();
CREATE CONSTRAINT TRIGGER org_trip_held_event_complete AFTER INSERT ON org_trip_held_events DEFERRABLE INITIALLY DEFERRED FOR EACH ROW EXECUTE FUNCTION org_sync_review_complete();

-- The 220 review-source hook with the trip.event.record owner; the meter,
-- trip.start and trip.finish branches are unchanged and unknown owners still
-- fail closed.
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
 $$;

DO $$DECLARE name TEXT;BEGIN
 FOREACH name IN ARRAY ARRAY['org_trip_events','org_trip_held_events'] LOOP
 EXECUTE format('ALTER TABLE %I ENABLE ROW LEVEL SECURITY',name);EXECUTE format('ALTER TABLE %I FORCE ROW LEVEL SECURITY',name);
 EXECUTE format('CREATE POLICY %I ON %I USING(org_id=NULLIF(current_setting(''anpfuel.org_id'',true),'''')::uuid) WITH CHECK(org_id=NULLIF(current_setting(''anpfuel.org_id'',true),'''')::uuid)',name||'_scope',name);
 EXECUTE format('CREATE TRIGGER %I BEFORE UPDATE OR DELETE OR TRUNCATE ON %I FOR EACH STATEMENT EXECUTE FUNCTION org_trip_keep_history()',name||'_immutable',name);
 EXECUTE format('REVOKE ALL ON %I FROM PUBLIC',name);
 END LOOP;
END $$;

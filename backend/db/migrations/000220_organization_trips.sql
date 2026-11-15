-- EMP-TRIP-002. Driver-initiated organization trips through the signed
-- intake (B-BR-EMP-312..319, EDR-202..206). The start fact is immutable; its
-- only permitted update is IN_PROGRESS -> COMPLETED, performed by the insert
-- of the trip's one insert-only finish fact. At most one trip per asset is in
-- progress. A second admissible start (ASSET_BUSY, ASSET_UNAVAILABLE,
-- PERMISSION_CHANGED) and the finish of a completed trip (TRIP_ALREADY_CLOSED)
-- are kept as OFFL-008 held facts; their resolution belongs to EMP-TRIP-007.
-- Reading evidence (origins TRIP_START, TRIP_END) is fleet evidence stored in
-- the same transaction as its event.
CREATE TABLE org_trips(
 org_id UUID NOT NULL REFERENCES organizations(org_id),
 id UUID NOT NULL CHECK(id<>'00000000-0000-0000-0000-000000000000'::uuid),
 operation_id UUID NOT NULL,
 payload_digest TEXT NOT NULL CHECK(payload_digest~'^sha256:[a-f0-9]{64}$'),
 asset_id UUID NOT NULL,
 driver_employee_id UUID NOT NULL,
 author_membership_id UUID NOT NULL,
 author_employee_id UUID NOT NULL,
 enrollment_id UUID NOT NULL,
 occurred_at TIMESTAMPTZ NOT NULL,
 occurred_submicro SMALLINT NOT NULL CHECK(occurred_submicro BETWEEN 0 AND 999),
 entry_zone TEXT NOT NULL,
 device_offset_seconds BIGINT NOT NULL CHECK(device_offset_seconds BETWEEN -64800 AND 64800),
 boot_id TEXT CHECK(boot_id~'^[A-Za-z0-9._-]{1,64}$'),
 elapsed_millis BIGINT CHECK(elapsed_millis>=0),
 received_at TIMESTAMPTZ NOT NULL DEFAULT clock_timestamp(),
 meter_id UUID,
 segment_id UUID,
 reading_id UUID,
 missing_reading_reason TEXT,
 unit_id UUID,
 cost_centre_id UUID,
 reporting_day DATE NOT NULL,
 reporting_zone TEXT NOT NULL,
 reporting_source TEXT NOT NULL CHECK(reporting_source IN('unit','organization')),
 state TEXT NOT NULL CHECK(state IN('IN_PROGRESS','COMPLETED')),
 PRIMARY KEY(org_id,id),
 CONSTRAINT org_trips_operation_key UNIQUE(org_id,operation_id),
 CONSTRAINT org_trips_operation_fkey FOREIGN KEY(org_id,operation_id) REFERENCES org_sync_operations(org_id,operation_id),
 CONSTRAINT org_trips_asset_fkey FOREIGN KEY(org_id,asset_id) REFERENCES org_assets(org_id,id),
 CONSTRAINT org_trips_driver_fkey FOREIGN KEY(org_id,driver_employee_id) REFERENCES org_employee_identities(org_id,id),
 CONSTRAINT org_trips_author_employee_fkey FOREIGN KEY(org_id,author_employee_id) REFERENCES org_employee_identities(org_id,id),
 CONSTRAINT org_trips_author_fkey FOREIGN KEY(org_id,author_membership_id) REFERENCES org_memberships(org_id,id),
 CONSTRAINT org_trips_enrollment_fkey FOREIGN KEY(org_id,enrollment_id) REFERENCES org_device_enrollments(org_id,id),
 CONSTRAINT org_trips_reading_fkey FOREIGN KEY(org_id,meter_id,reading_id) REFERENCES org_meter_readings(org_id,meter_id,id),
 CONSTRAINT org_trips_unit_fkey FOREIGN KEY(org_id,unit_id) REFERENCES org_units(org_id,id),
 CONSTRAINT org_trips_cost_centre_fkey FOREIGN KEY(org_id,cost_centre_id) REFERENCES org_cost_centres(org_id,id),
 CONSTRAINT org_trips_reading_evidence CHECK(num_nonnulls(reading_id,missing_reading_reason)=1
  AND num_nonnulls(meter_id,segment_id,reading_id) IN(0,3)
  AND(missing_reading_reason IS NULL OR org_asset_text_valid(missing_reading_reason,280,true))),
 CONSTRAINT org_trips_run_clock CHECK((boot_id IS NULL)=(elapsed_millis IS NULL)),
 CONSTRAINT org_trips_snapshot CHECK(cost_centre_id IS NULL OR unit_id IS NOT NULL),
 CHECK(isfinite(occurred_at) AND occurred_at>='0001-01-01 00:00:00+00'::timestamptz AND occurred_at<'10000-01-01 00:00:00+00'::timestamptz),
 CHECK(isfinite(received_at) AND received_at>'0001-01-01 00:00:00+00'::timestamptz AND received_at<'10000-01-01 00:00:00+00'::timestamptz)
);
-- VAL-07: two trips of one asset never count as in progress.
CREATE UNIQUE INDEX org_trips_one_active_asset ON org_trips(org_id,asset_id) WHERE state='IN_PROGRESS';
CREATE INDEX org_trips_asset ON org_trips(org_id,asset_id,occurred_at,occurred_submicro,id);

CREATE TABLE org_trip_finishes(
 org_id UUID NOT NULL,
 trip_id UUID NOT NULL,
 operation_id UUID NOT NULL,
 payload_digest TEXT NOT NULL CHECK(payload_digest~'^sha256:[a-f0-9]{64}$'),
 author_membership_id UUID NOT NULL,
 enrollment_id UUID NOT NULL,
 occurred_at TIMESTAMPTZ NOT NULL,
 occurred_submicro SMALLINT NOT NULL CHECK(occurred_submicro BETWEEN 0 AND 999),
 entry_zone TEXT NOT NULL,
 device_offset_seconds BIGINT NOT NULL CHECK(device_offset_seconds BETWEEN -64800 AND 64800),
 boot_id TEXT CHECK(boot_id~'^[A-Za-z0-9._-]{1,64}$'),
 elapsed_millis BIGINT CHECK(elapsed_millis>=0),
 received_at TIMESTAMPTZ NOT NULL DEFAULT clock_timestamp(),
 meter_id UUID,
 segment_id UUID,
 reading_id UUID,
 missing_reading_reason TEXT,
 end_day DATE NOT NULL,
 PRIMARY KEY(org_id,trip_id),
 CONSTRAINT org_trip_finishes_operation_key UNIQUE(org_id,operation_id),
 CONSTRAINT org_trip_finishes_trip_fkey FOREIGN KEY(org_id,trip_id) REFERENCES org_trips(org_id,id),
 CONSTRAINT org_trip_finishes_operation_fkey FOREIGN KEY(org_id,operation_id) REFERENCES org_sync_operations(org_id,operation_id),
 CONSTRAINT org_trip_finishes_author_fkey FOREIGN KEY(org_id,author_membership_id) REFERENCES org_memberships(org_id,id),
 CONSTRAINT org_trip_finishes_enrollment_fkey FOREIGN KEY(org_id,enrollment_id) REFERENCES org_device_enrollments(org_id,id),
 CONSTRAINT org_trip_finishes_reading_fkey FOREIGN KEY(org_id,meter_id,reading_id) REFERENCES org_meter_readings(org_id,meter_id,id),
 CONSTRAINT org_trip_finishes_reading_evidence CHECK(num_nonnulls(reading_id,missing_reading_reason)=1
  AND num_nonnulls(meter_id,segment_id,reading_id) IN(0,3)
  AND(missing_reading_reason IS NULL OR org_asset_text_valid(missing_reading_reason,280,true))),
 CONSTRAINT org_trip_finishes_run_clock CHECK((boot_id IS NULL)=(elapsed_millis IS NULL)),
 CHECK(isfinite(occurred_at) AND occurred_at>='0001-01-01 00:00:00+00'::timestamptz AND occurred_at<'10000-01-01 00:00:00+00'::timestamptz),
 CHECK(isfinite(received_at) AND received_at>'0001-01-01 00:00:00+00'::timestamptz AND received_at<'10000-01-01 00:00:00+00'::timestamptz)
);

-- OFFL-008 held facts. A held start is never a trip and never active use.
CREATE TABLE org_trip_held_starts(
 org_id UUID NOT NULL,review_item_id UUID NOT NULL,operation_id UUID NOT NULL,
 trip_id UUID NOT NULL CHECK(trip_id<>'00000000-0000-0000-0000-000000000000'::uuid),
 asset_id UUID NOT NULL,driver_employee_id UUID NOT NULL,
 occurred_at TIMESTAMPTZ NOT NULL,occurred_submicro SMALLINT NOT NULL CHECK(occurred_submicro BETWEEN 0 AND 999),entry_zone TEXT NOT NULL,
 device_offset_seconds BIGINT NOT NULL CHECK(device_offset_seconds BETWEEN -64800 AND 64800),
 boot_id TEXT CHECK(boot_id~'^[A-Za-z0-9._-]{1,64}$'),elapsed_millis BIGINT CHECK(elapsed_millis>=0),
 meter_id UUID,segment_id UUID,reading_id UUID,reading_value BIGINT CHECK(reading_value>=0),missing_reading_reason TEXT,
 PRIMARY KEY(org_id,review_item_id),UNIQUE(org_id,operation_id),UNIQUE(org_id,trip_id),
 FOREIGN KEY(org_id,review_item_id,operation_id) REFERENCES org_sync_review_items(org_id,id,operation_id),
 CONSTRAINT org_trip_held_starts_asset_fkey FOREIGN KEY(org_id,asset_id) REFERENCES org_assets(org_id,id),
 FOREIGN KEY(org_id,driver_employee_id) REFERENCES org_employee_identities(org_id,id),
 FOREIGN KEY(org_id,asset_id,meter_id) REFERENCES org_meters(org_id,asset_id,id),
 FOREIGN KEY(org_id,meter_id,segment_id) REFERENCES org_meter_segments(org_id,meter_id,id),
 CHECK(num_nonnulls(reading_id,missing_reading_reason)=1 AND num_nonnulls(meter_id,segment_id,reading_id,reading_value) IN(0,4)
  AND(missing_reading_reason IS NULL OR org_asset_text_valid(missing_reading_reason,280,true))),
 CHECK((boot_id IS NULL)=(elapsed_millis IS NULL)),
 CHECK(isfinite(occurred_at) AND occurred_at>='0001-01-01 00:00:00+00'::timestamptz AND occurred_at<'10000-01-01 00:00:00+00'::timestamptz)
);
CREATE TABLE org_trip_held_finishes(
 org_id UUID NOT NULL,review_item_id UUID NOT NULL,operation_id UUID NOT NULL,trip_id UUID NOT NULL,
 occurred_at TIMESTAMPTZ NOT NULL,occurred_submicro SMALLINT NOT NULL CHECK(occurred_submicro BETWEEN 0 AND 999),entry_zone TEXT NOT NULL,
 device_offset_seconds BIGINT NOT NULL CHECK(device_offset_seconds BETWEEN -64800 AND 64800),
 boot_id TEXT CHECK(boot_id~'^[A-Za-z0-9._-]{1,64}$'),elapsed_millis BIGINT CHECK(elapsed_millis>=0),
 meter_id UUID,segment_id UUID,reading_id UUID,reading_value BIGINT CHECK(reading_value>=0),missing_reading_reason TEXT,
 PRIMARY KEY(org_id,review_item_id),UNIQUE(org_id,operation_id),
 FOREIGN KEY(org_id,review_item_id,operation_id) REFERENCES org_sync_review_items(org_id,id,operation_id),
 FOREIGN KEY(org_id,trip_id) REFERENCES org_trips(org_id,id),
 FOREIGN KEY(org_id,meter_id,segment_id) REFERENCES org_meter_segments(org_id,meter_id,id),
 CHECK(num_nonnulls(reading_id,missing_reading_reason)=1 AND num_nonnulls(meter_id,segment_id,reading_id,reading_value) IN(0,4)
  AND(missing_reading_reason IS NULL OR org_asset_text_valid(missing_reading_reason,280,true))),
 CHECK((boot_id IS NULL)=(elapsed_millis IS NULL)),
 CHECK(isfinite(occurred_at) AND occurred_at>='0001-01-01 00:00:00+00'::timestamptz AND occurred_at<'10000-01-01 00:00:00+00'::timestamptz)
);

-- The device operation of a trip event: exact type, entity, digest and an
-- active enrolled author whose employee record is the given one.
CREATE FUNCTION org_trip_operation_valid(p_org UUID,p_op UUID,p_kind TEXT,p_trip UUID,p_digest TEXT,p_member UUID,p_enrollment UUID,p_employee UUID) RETURNS BOOLEAN
 LANGUAGE SQL STABLE SET search_path=pg_catalog AS $$
 SELECT EXISTS(SELECT 1 FROM public.org_sync_operations o JOIN public.org_memberships m ON(m.org_id,m.id)=(o.org_id,o.membership_id)
  JOIN public.org_device_enrollments e ON(e.org_id,e.id,e.membership_id)=(o.org_id,o.enrollment_id,o.membership_id)
  WHERE o.org_id=p_org AND o.operation_id=p_op AND o.operation_type=p_kind AND o.operation_schema=1 AND o.entity_type='trip' AND o.entity_id=p_trip
  AND(p_digest IS NULL OR o.payload_digest=p_digest) AND(p_member IS NULL OR o.membership_id=p_member) AND(p_enrollment IS NULL OR o.enrollment_id=p_enrollment)
  AND m.employee_id=p_employee AND m.account_id=o.owner_account_id AND m.state='ACTIVE' AND e.state='ENROLLED')
 $$;

CREATE FUNCTION org_trip_source() RETURNS trigger LANGUAGE plpgsql SET search_path=pg_catalog AS $$
DECLARE a public.org_assets%ROWTYPE;actual_unit UUID;actual_centre UUID;zone TEXT;
BEGIN
 IF NEW.org_id IS DISTINCT FROM NULLIF(current_setting('anpfuel.org_id',true),'')::uuid THEN RAISE EXCEPTION USING ERRCODE='42501',MESSAGE='organization scope denied';END IF;
 PERFORM 1 FROM public.organizations WHERE org_id=NEW.org_id AND has_snapshot FOR UPDATE;
 IF NOT FOUND THEN RAISE EXCEPTION USING ERRCODE='23503',MESSAGE='private reference not found';END IF;
 IF NOT public.org_trip_operation_valid(NEW.org_id,NEW.operation_id,'trip.start',NEW.id,NEW.payload_digest,NEW.author_membership_id,NEW.enrollment_id,NEW.author_employee_id)
 THEN RAISE EXCEPTION USING ERRCODE='23514',MESSAGE='trip source unavailable';END IF;
 SELECT * INTO a FROM public.org_assets WHERE org_id=NEW.org_id AND id=NEW.asset_id FOR UPDATE;
 IF NOT FOUND THEN RAISE EXCEPTION USING ERRCODE='23503',MESSAGE='private reference not found';END IF;
 IF NEW.state<>'IN_PROGRESS' OR a.state<>'ACTIVE' OR(a.disposed_at IS NOT NULL AND(NEW.occurred_at,NEW.occurred_submicro)>=(a.disposed_at,a.disposed_submicro))
 THEN RAISE EXCEPTION USING ERRCODE='23514',MESSAGE='asset does not accept a trip';END IF;
 IF EXISTS(SELECT 1 FROM public.org_trip_held_starts h WHERE h.org_id=NEW.org_id AND h.trip_id=NEW.id)
 THEN RAISE EXCEPTION USING ERRCODE='23505',CONSTRAINT='org_trips_pkey',MESSAGE='trip identity unavailable';END IF;
 IF NOT EXISTS(SELECT 1 FROM public.org_employee_identities WHERE org_id=NEW.org_id AND id=NEW.driver_employee_id AND state='ACTIVE' AND merged_into IS NULL)
 THEN RAISE EXCEPTION USING ERRCODE='23514',MESSAGE='trip driver unavailable';END IF;
 IF NOT EXISTS(SELECT 1 FROM pg_timezone_names WHERE name=NEW.entry_zone) THEN RAISE EXCEPTION USING ERRCODE='23514',MESSAGE='invalid trip declaration';END IF;
 -- BR-AUTH-005: the FLEET-005 allocation at the declared instant.
 SELECT p.unit_id INTO actual_unit FROM public.org_asset_unit_periods p WHERE p.org_id=NEW.org_id AND p.asset_id=NEW.asset_id AND p.valid_from<=NEW.occurred_at AND(p.valid_to IS NULL OR NEW.occurred_at<p.valid_to);
 SELECT p.cost_centre_id INTO actual_centre FROM public.org_asset_cost_centre_periods p WHERE p.org_id=NEW.org_id AND p.asset_id=NEW.asset_id AND p.valid_from<=NEW.occurred_at AND(p.valid_to IS NULL OR NEW.occurred_at<p.valid_to);
 IF NEW.unit_id IS DISTINCT FROM actual_unit OR NEW.cost_centre_id IS DISTINCT FROM(CASE WHEN actual_unit IS NULL THEN NULL ELSE actual_centre END)
 THEN RAISE EXCEPTION USING ERRCODE='23514',MESSAGE='trip allocation snapshot unavailable';END IF;
 -- BR-TIME-004: the start day in the snapshot unit's zone, else the organization's.
 IF NEW.unit_id IS NULL THEN SELECT time_zone INTO zone FROM public.organizations WHERE org_id=NEW.org_id;
 ELSE SELECT time_zone INTO zone FROM public.org_units WHERE org_id=NEW.org_id AND id=NEW.unit_id;END IF;
 IF zone IS NULL OR NEW.reporting_zone<>zone OR NEW.reporting_source<>(CASE WHEN NEW.unit_id IS NULL THEN 'organization' ELSE 'unit' END)
 OR NEW.reporting_day<>(NEW.occurred_at AT TIME ZONE zone)::date THEN RAISE EXCEPTION USING ERRCODE='23514',MESSAGE='trip reporting attribution unavailable';END IF;
 NEW.received_at=clock_timestamp();RETURN NEW;
END $$;
CREATE TRIGGER org_trip_source BEFORE INSERT ON org_trips FOR EACH ROW EXECUTE FUNCTION org_trip_source();

-- The only update: completion by the finish fact inserted in this transaction.
CREATE FUNCTION org_trip_completion_guard() RETURNS trigger LANGUAGE plpgsql SET search_path=pg_catalog AS $$
BEGIN
 IF OLD.state<>'IN_PROGRESS' OR NEW.state<>'COMPLETED' OR to_jsonb(NEW)-'state'<>to_jsonb(OLD)-'state'
 OR NOT EXISTS(SELECT 1 FROM public.org_trip_finishes f WHERE f.org_id=OLD.org_id AND f.trip_id=OLD.id)
 THEN RAISE EXCEPTION USING ERRCODE='55000',MESSAGE='organization trip history is insert-only';END IF;
 RETURN NEW;
END $$;
CREATE TRIGGER org_trip_completion_guard BEFORE UPDATE ON org_trips FOR EACH ROW EXECUTE FUNCTION org_trip_completion_guard();

CREATE FUNCTION org_trip_finish_source() RETURNS trigger LANGUAGE plpgsql SET search_path=pg_catalog AS $$
DECLARE t public.org_trips%ROWTYPE;
BEGIN
 IF NEW.org_id IS DISTINCT FROM NULLIF(current_setting('anpfuel.org_id',true),'')::uuid THEN RAISE EXCEPTION USING ERRCODE='42501',MESSAGE='organization scope denied';END IF;
 PERFORM 1 FROM public.organizations WHERE org_id=NEW.org_id AND has_snapshot FOR UPDATE;
 SELECT * INTO t FROM public.org_trips WHERE org_id=NEW.org_id AND id=NEW.trip_id FOR UPDATE;
 IF NOT FOUND THEN RAISE EXCEPTION USING ERRCODE='23503',MESSAGE='private reference not found';END IF;
 IF NOT public.org_trip_operation_valid(NEW.org_id,NEW.operation_id,'trip.finish',NEW.trip_id,NEW.payload_digest,NEW.author_membership_id,NEW.enrollment_id,t.author_employee_id)
 OR NEW.author_membership_id<>t.author_membership_id THEN RAISE EXCEPTION USING ERRCODE='23514',MESSAGE='trip finish source unavailable';END IF;
 IF t.state<>'IN_PROGRESS' THEN RAISE EXCEPTION USING ERRCODE='23514',MESSAGE='trip already closed';END IF;
 IF(NEW.occurred_at,NEW.occurred_submicro)<(t.occurred_at,t.occurred_submicro) OR NOT EXISTS(SELECT 1 FROM pg_timezone_names WHERE name=NEW.entry_zone)
 OR NEW.end_day<>(NEW.occurred_at AT TIME ZONE t.reporting_zone)::date THEN RAISE EXCEPTION USING ERRCODE='23514',MESSAGE='invalid trip finish';END IF;
 IF NEW.meter_id IS NOT NULL AND NOT EXISTS(SELECT 1 FROM public.org_meters m WHERE m.org_id=NEW.org_id AND m.id=NEW.meter_id AND m.asset_id=t.asset_id)
 THEN RAISE EXCEPTION USING ERRCODE='23514',MESSAGE='trip finish reading unavailable';END IF;
 NEW.received_at=clock_timestamp();RETURN NEW;
END $$;
CREATE TRIGGER org_trip_finish_source BEFORE INSERT ON org_trip_finishes FOR EACH ROW EXECUTE FUNCTION org_trip_finish_source();
CREATE FUNCTION org_trip_finish_apply() RETURNS trigger LANGUAGE plpgsql SET search_path=pg_catalog AS $$
BEGIN
 UPDATE public.org_trips SET state='COMPLETED' WHERE org_id=NEW.org_id AND id=NEW.trip_id AND state='IN_PROGRESS';
 IF NOT FOUND THEN RAISE EXCEPTION USING ERRCODE='23514',MESSAGE='trip already closed';END IF;
 RETURN NULL;
END $$;
CREATE TRIGGER org_trip_finish_apply AFTER INSERT ON org_trip_finishes FOR EACH ROW EXECUTE FUNCTION org_trip_finish_apply();

-- At commit a start or finish is published only with its accepted receipt
-- and effect and with its reading evidence.
CREATE FUNCTION org_trip_reading_valid(p_org UUID,p_asset UUID,p_meter UUID,p_segment UUID,p_reading UUID,p_origin TEXT,p_op UUID,p_member UUID,p_enrollment UUID,p_at TIMESTAMPTZ,p_sub SMALLINT,p_zone TEXT,p_offset BIGINT) RETURNS BOOLEAN
 LANGUAGE SQL STABLE SET search_path=pg_catalog AS $$
 SELECT p_reading IS NULL OR EXISTS(SELECT 1 FROM public.org_meter_readings r JOIN public.org_meters m ON(m.org_id,m.id)=(r.org_id,r.meter_id)
  WHERE r.org_id=p_org AND r.meter_id=p_meter AND r.id=p_reading AND r.segment_id=p_segment AND m.asset_id=p_asset AND r.origin=p_origin AND r.operation_id=p_op
  AND r.author_membership_id=p_member AND r.capture_kind='ENROLLED_DEVICE' AND r.enrollment_id=p_enrollment AND r.method='TYPED'
  AND(r.occurred_at,r.occurred_submicro)=(p_at,p_sub) AND r.entry_zone=p_zone AND r.device_offset_seconds=p_offset)
 $$;
CREATE FUNCTION org_trip_published(p_org UUID,p_op UUID,p_revision BIGINT) RETURNS BOOLEAN LANGUAGE SQL STABLE SET search_path=pg_catalog AS $$
 SELECT EXISTS(SELECT 1 FROM public.org_sync_acknowledgements a JOIN public.org_sync_effects e USING(org_id,operation_id)
  WHERE a.org_id=p_org AND a.operation_id=p_op AND a.result='ACCEPTED' AND a.entity_revision=p_revision AND e.entity_revision=p_revision)
 $$;
CREATE FUNCTION org_trip_complete() RETURNS trigger LANGUAGE plpgsql SET search_path=pg_catalog AS $$
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
  IF f.operation_id IS NULL OR t.state IS DISTINCT FROM 'COMPLETED' OR NOT public.org_trip_published(f.org_id,f.operation_id,2)
  OR NOT public.org_trip_reading_valid(f.org_id,t.asset_id,f.meter_id,f.segment_id,f.reading_id,'TRIP_END',f.operation_id,f.author_membership_id,f.enrollment_id,f.occurred_at,f.occurred_submicro,f.entry_zone,f.device_offset_seconds)
  THEN RAISE EXCEPTION USING ERRCODE='23514',MESSAGE='trip finish publication unavailable';END IF;
 END IF;
 PERFORM set_config('anpfuel.org_id',COALESCE(prior_org,''),true);RETURN NULL;
END $$;
CREATE CONSTRAINT TRIGGER org_trip_complete AFTER INSERT ON org_trips DEFERRABLE INITIALLY DEFERRED FOR EACH ROW EXECUTE FUNCTION org_trip_complete();
CREATE CONSTRAINT TRIGGER org_trip_finish_complete AFTER INSERT ON org_trip_finishes DEFERRABLE INITIALLY DEFERRED FOR EACH ROW EXECUTE FUNCTION org_trip_complete();

-- Held facts re-prove their source and the condition that holds them.
CREATE FUNCTION org_trip_held_source() RETURNS trigger LANGUAGE plpgsql SET search_path=pg_catalog AS $$
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
  IF i.code='ASSET_BUSY' AND NOT EXISTS(SELECT 1 FROM public.org_trips t2 LEFT JOIN public.org_trip_finishes f2 ON(f2.org_id,f2.trip_id)=(t2.org_id,t2.id)
   WHERE t2.org_id=NEW.org_id AND t2.asset_id=NEW.asset_id AND(t2.state='IN_PROGRESS' OR((t2.occurred_at,t2.occurred_submicro)<=(NEW.occurred_at,NEW.occurred_submicro)
   AND(f2.occurred_at,f2.occurred_submicro)>(NEW.occurred_at,NEW.occurred_submicro))))
  THEN RAISE EXCEPTION USING ERRCODE='23514',MESSAGE='held trip condition unavailable';END IF;
 ELSE
  SELECT * INTO t FROM public.org_trips WHERE org_id=NEW.org_id AND id=NEW.trip_id FOR UPDATE;
  IF NOT FOUND THEN RAISE EXCEPTION USING ERRCODE='23503',MESSAGE='private reference not found';END IF;
  IF i.operation_type<>'trip.finish' OR i.operation_schema<>1 OR i.code<>'TRIP_ALREADY_CLOSED' OR t.state<>'COMPLETED'
  OR NOT public.org_trip_operation_valid(NEW.org_id,NEW.operation_id,'trip.finish',NEW.trip_id,i.payload_digest,t.author_membership_id,NULL,t.author_employee_id)
  OR EXISTS(SELECT 1 FROM public.org_trip_finishes f WHERE f.org_id=NEW.org_id AND f.operation_id=NEW.operation_id)
  OR(NEW.occurred_at,NEW.occurred_submicro)<(t.occurred_at,t.occurred_submicro)
  OR(NEW.meter_id IS NOT NULL AND NOT EXISTS(SELECT 1 FROM public.org_meters m WHERE m.org_id=NEW.org_id AND m.id=NEW.meter_id AND m.asset_id=t.asset_id))
  THEN RAISE EXCEPTION USING ERRCODE='23514',MESSAGE='held trip finish source unavailable';END IF;
 END IF;
 RETURN NEW;
END $$;
CREATE TRIGGER org_trip_held_start_source BEFORE INSERT ON org_trip_held_starts FOR EACH ROW EXECUTE FUNCTION org_trip_held_source();
CREATE TRIGGER org_trip_held_finish_source BEFORE INSERT ON org_trip_held_finishes FOR EACH ROW EXECUTE FUNCTION org_trip_held_source();
CREATE CONSTRAINT TRIGGER org_trip_held_start_complete AFTER INSERT ON org_trip_held_starts DEFERRABLE INITIALLY DEFERRED FOR EACH ROW EXECUTE FUNCTION org_sync_review_complete();
CREATE CONSTRAINT TRIGGER org_trip_held_finish_complete AFTER INSERT ON org_trip_held_finishes DEFERRABLE INITIALLY DEFERRED FOR EACH ROW EXECUTE FUNCTION org_sync_review_complete();

-- The 199 review-source hook with the two trip owners; the meter branch is
-- unchanged and unknown owners still fail closed.
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
 $$;

CREATE FUNCTION org_trip_keep_history() RETURNS trigger LANGUAGE plpgsql SET search_path=pg_catalog AS $$
BEGIN RAISE EXCEPTION USING ERRCODE='55000',MESSAGE='organization trip history is insert-only';END $$;
DO $$DECLARE name TEXT;BEGIN
 FOREACH name IN ARRAY ARRAY['org_trips','org_trip_finishes','org_trip_held_starts','org_trip_held_finishes'] LOOP
 EXECUTE format('ALTER TABLE %I ENABLE ROW LEVEL SECURITY',name);EXECUTE format('ALTER TABLE %I FORCE ROW LEVEL SECURITY',name);
 EXECUTE format('CREATE POLICY %I ON %I USING(org_id=NULLIF(current_setting(''anpfuel.org_id'',true),'''')::uuid) WITH CHECK(org_id=NULLIF(current_setting(''anpfuel.org_id'',true),'''')::uuid)',name||'_scope',name);
 IF name='org_trips' THEN
  EXECUTE format('CREATE TRIGGER %I BEFORE DELETE OR TRUNCATE ON %I FOR EACH STATEMENT EXECUTE FUNCTION org_trip_keep_history()',name||'_immutable',name);
 ELSE
  EXECUTE format('CREATE TRIGGER %I BEFORE UPDATE OR DELETE OR TRUNCATE ON %I FOR EACH STATEMENT EXECUTE FUNCTION org_trip_keep_history()',name||'_immutable',name);
 END IF;
 EXECUTE format('REVOKE ALL ON %I FROM PUBLIC',name);
 END LOOP;
END $$;

-- The 217 asset-use source (the 215 body plus the FLEET-007 reservation and
-- block keys) with trips and held trip facts recognised as accepted
-- operations on the asset (they fix its kind and keep a draft).
CREATE OR REPLACE FUNCTION org_asset_current_usage(selected_org UUID,selected_asset UUID) RETURNS JSONB
 LANGUAGE plpgsql SET search_path=pg_catalog AS $$
BEGIN
 IF selected_org IS DISTINCT FROM NULLIF(current_setting('anpfuel.org_id',true),'')::uuid THEN RAISE EXCEPTION USING ERRCODE='42501',MESSAGE='organization scope denied';END IF;
 PERFORM 1 FROM public.organizations WHERE org_id=selected_org AND has_snapshot FOR UPDATE;
 IF NOT FOUND OR NOT EXISTS(SELECT 1 FROM public.org_assets WHERE org_id=selected_org AND id=selected_asset) THEN RAISE EXCEPTION USING ERRCODE='23503',MESSAGE='private reference not found';END IF;
 IF EXISTS(SELECT 1 FROM pg_constraint c WHERE c.contype='f' AND c.confrelid='public.org_assets'::regclass
 AND NOT((c.conrelid='public.org_asset_tanks'::regclass AND c.conname='org_asset_tanks_asset_fkey')
 OR(c.conrelid='public.org_asset_expected'::regclass AND c.conname='org_asset_expected_asset_fkey')
 OR(c.conrelid='public.org_asset_unit_allocations'::regclass AND c.conname='org_asset_allocation_asset_fkey')
 OR(c.conrelid='public.org_asset_revisions'::regclass AND c.conname='org_asset_revision_asset_fkey')
 OR(c.conrelid='public.org_meters'::regclass AND c.conname='org_meters_asset_fkey')
 OR(c.conrelid='public.org_meter_held_readings'::regclass AND c.conname='org_meter_held_readings_asset_fkey')
 OR(c.conrelid='public.org_asset_unit_periods'::regclass AND c.conname='org_asset_unit_periods_asset_fkey')
 OR(c.conrelid='public.org_asset_cost_centre_periods'::regclass AND c.conname='org_asset_cost_centre_periods_asset_fkey')
 OR(c.conrelid='public.org_role_assignments'::regclass AND c.conname='org_role_assignments_scope_asset_fkey')
 OR(c.conrelid=to_regclass('public.org_document_links') AND c.conname='org_document_links_asset_fkey')
 OR(c.conrelid='public.org_asset_assignments'::regclass AND c.conname='org_asset_assignments_asset_fkey')
 OR(c.conrelid='public.org_fuelings'::regclass AND c.conname='org_fuelings_asset_fkey')
 OR(c.conrelid='public.org_asset_reservations'::regclass AND c.conname='org_asset_reservations_asset_fkey')
 OR(c.conrelid='public.org_asset_blocks'::regclass AND c.conname='org_asset_blocks_asset_fkey')
 OR(c.conrelid='public.org_trips'::regclass AND c.conname='org_trips_asset_fkey')
 OR(c.conrelid='public.org_trip_held_starts'::regclass AND c.conname='org_trip_held_starts_asset_fkey'))) THEN RAISE EXCEPTION USING ERRCODE='P7904',MESSAGE='asset usage source unavailable';END IF;
 RETURN jsonb_build_object('readings',(SELECT count(*) FROM public.org_meter_readings r JOIN public.org_meters m ON(m.org_id,m.id)=(r.org_id,r.meter_id) WHERE m.org_id=selected_org AND m.asset_id=selected_asset),'operations',(SELECT count(*) FROM public.org_sync_operations o JOIN public.org_sync_effects e USING(org_id,operation_id) WHERE o.org_id=selected_org AND o.entity_id=selected_asset AND o.entity_type='fleet_asset')+(SELECT count(*) FROM public.org_meter_held_readings h WHERE h.org_id=selected_org AND h.asset_id=selected_asset)+(SELECT count(*) FROM public.org_fuelings f WHERE f.org_id=selected_org AND f.asset_id=selected_asset)+(SELECT count(*) FROM public.org_trips t WHERE t.org_id=selected_org AND t.asset_id=selected_asset)+(SELECT count(*) FROM public.org_trip_held_starts h WHERE h.org_id=selected_org AND h.asset_id=selected_asset));
END $$;

-- The 215 unit-use source with the trip's historical unit snapshot
-- recognised as an association, never as current use (BR-AUTH-005).
CREATE OR REPLACE FUNCTION org_unit_current_usage(selected_org UUID,selected_unit UUID) RETURNS JSONB LANGUAGE plpgsql SET search_path=pg_catalog AS $$
DECLARE employee_count BIGINT;asset_count BIGINT;
BEGIN
 IF selected_org IS DISTINCT FROM NULLIF(current_setting('anpfuel.org_id',true),'')::uuid THEN RAISE EXCEPTION USING ERRCODE='42501',MESSAGE='organization scope denied';END IF;
 PERFORM 1 FROM public.organizations WHERE org_id=selected_org AND has_snapshot FOR UPDATE;
 IF NOT FOUND OR NOT EXISTS(SELECT 1 FROM public.org_units WHERE org_id=selected_org AND id=selected_unit) THEN RAISE EXCEPTION USING ERRCODE='23503',MESSAGE='private reference not found';END IF;
 IF EXISTS(SELECT 1 FROM pg_constraint c WHERE c.contype='f' AND c.confrelid='public.org_units'::regclass
 AND NOT((c.conrelid='public.org_onboarding_commands'::regclass AND c.conname='org_onboarding_commands_org_id_unit_id_fkey')
 OR(c.conrelid='public.org_unit_commands'::regclass AND c.conname='org_unit_commands_unit_fkey')
 OR(c.conrelid='public.org_employee_profiles'::regclass AND c.conname='org_employee_profile_unit_fkey')
 OR(c.conrelid='public.org_teams'::regclass AND c.conname='org_team_unit_fkey')
 OR(c.conrelid='public.org_assets'::regclass AND c.conname='org_asset_unit_fkey')
 OR(c.conrelid='public.org_asset_unit_allocations'::regclass AND c.conname='org_asset_allocation_unit_fkey')
 OR(c.conrelid='public.org_meter_held_readings'::regclass AND c.conname='org_meter_held_readings_unit_fkey')
 OR(c.conrelid='public.org_asset_unit_periods'::regclass AND c.conname='org_asset_unit_periods_unit_fkey')
 OR(c.conrelid='public.org_role_assignments'::regclass AND c.conname='org_role_assignments_scope_unit_fkey')
 OR(c.conrelid='public.org_fuelings'::regclass AND c.conname='org_fuelings_unit_fkey')
 OR(c.conrelid='public.org_trips'::regclass AND c.conname='org_trips_unit_fkey'))) THEN RAISE EXCEPTION USING ERRCODE='P7704',MESSAGE='unit usage source unavailable';END IF;
 SELECT count(DISTINCT employee_id) INTO employee_count FROM(
 SELECT e.id AS employee_id FROM public.org_employee_profiles p JOIN public.org_employee_identities e ON(e.org_id,e.id)=(p.org_id,p.employee_id)
 WHERE p.org_id=selected_org AND p.home_unit_id=selected_unit AND e.state='ACTIVE'
 UNION
 SELECT e.id AS employee_id FROM public.org_team_links l JOIN public.org_teams t ON(t.org_id,t.id)=(l.org_id,l.team_id)
 JOIN public.org_employee_identities e ON(e.org_id,e.id)=(l.org_id,l.employee_id)
 WHERE l.org_id=selected_org AND t.unit_id=selected_unit AND l.ended_at IS NULL AND e.state='ACTIVE') current_employees;
 SELECT count(DISTINCT p.asset_id) INTO asset_count FROM public.org_asset_unit_periods p JOIN public.org_assets a ON(a.org_id,a.id)=(p.org_id,p.asset_id)
 WHERE p.org_id=selected_org AND p.unit_id=selected_unit AND(p.valid_to IS NULL OR p.valid_to>clock_timestamp()) AND a.state='ACTIVE';
 RETURN jsonb_build_object('assets',asset_count,'employees',employee_count,'budgets',0,'operations',0);
END $$;

-- The 215 cost-centre-use source with the trip's historical snapshot
-- recognised as an association, never as current use.
CREATE OR REPLACE FUNCTION org_cost_centre_current_usage(selected_org UUID,selected_centre UUID) RETURNS JSONB
 LANGUAGE plpgsql SET search_path=pg_catalog AS $$
BEGIN
 IF selected_org IS DISTINCT FROM NULLIF(current_setting('anpfuel.org_id',true),'')::uuid THEN RAISE EXCEPTION USING ERRCODE='42501',MESSAGE='organization scope denied';END IF;
 PERFORM 1 FROM public.organizations WHERE org_id=selected_org AND has_snapshot FOR UPDATE;
 IF NOT FOUND OR NOT EXISTS(SELECT 1 FROM public.org_cost_centres WHERE org_id=selected_org AND id=selected_centre) THEN RAISE EXCEPTION USING ERRCODE='23503',MESSAGE='private reference not found';END IF;
 IF EXISTS(SELECT 1 FROM pg_constraint c WHERE c.contype='f' AND c.confrelid IN('public.org_cost_centres'::regclass,'public.org_cost_centre_codes'::regclass)
  AND NOT((c.conrelid='public.org_cost_centre_codes'::regclass AND c.conname='org_cost_centre_codes_centre_fkey')
   OR(c.conrelid='public.org_cost_centre_commands'::regclass AND c.conname='org_cost_centre_commands_centre_fkey')
   OR(c.conrelid='public.org_cost_centres'::regclass AND c.conname='org_cost_centre_current_code_fkey')
   OR(c.conrelid='public.org_asset_cost_centre_periods'::regclass AND c.conname='org_asset_cost_centre_periods_centre_fkey')
   OR(c.conrelid='public.org_role_assignments'::regclass AND c.conname='org_role_assignments_scope_cost_centre_fkey')
   OR(c.conrelid='public.org_fuelings'::regclass AND c.conname='org_fuelings_cost_centre_fkey')
   OR(c.conrelid='public.org_trips'::regclass AND c.conname='org_trips_cost_centre_fkey'))) THEN
  RAISE EXCEPTION USING ERRCODE='P7804',MESSAGE='cost-centre usage source unavailable';
 END IF;
 RETURN jsonb_build_object('assets',(SELECT count(DISTINCT p.asset_id) FROM public.org_asset_cost_centre_periods p JOIN public.org_assets a ON(a.org_id,a.id)=(p.org_id,p.asset_id)
  WHERE p.org_id=selected_org AND p.cost_centre_id=selected_centre AND(p.valid_to IS NULL OR p.valid_to>clock_timestamp()) AND a.state='ACTIVE'),'employees',0,'budgets',0,'operations',0);
END $$;

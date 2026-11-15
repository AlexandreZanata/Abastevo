-- EMP-TRIP-007. Asset-overlap conflict resolution (B-BR-EMP-407..411,
-- EDR-262..264). Additive: no table and no foreign key. The OFFL-008
-- resolution of a held trip fact is the existing org_sync_review_resolutions
-- row; this migration widens its outcome check and replaces, keeping every
-- check of their latest bodies, the guards that must recognise a trip owner:
-- the 199 resolution source and publication guards, the 216 observation
-- guard (a reviewed TRIP_START reading), the 199 effect source, the 220 trip
-- source and the 000233 (EMP-TRIP-011) trip publication: a held start
-- accepted as its own trip never overlaps another trip of the asset. Usage
-- sources are unchanged.

-- 1. The 199 outcome check: an accepted resolution names entity revision 1.
-- The meter branch of the source guard still requires the held reading; an
-- accepted trip start may carry a stated missing reading instead.
DO $$DECLARE c TEXT;n INTEGER;BEGIN
 SELECT count(*),min(conname) INTO n,c FROM pg_constraint WHERE conrelid='public.org_sync_review_resolutions'::regclass AND contype='c' AND pg_get_constraintdef(oid) LIKE '%reading_id IS NOT NULL%';
 IF n<>1 THEN RAISE EXCEPTION USING ERRCODE='55000',MESSAGE='review outcome check unavailable';END IF;
 EXECUTE format('ALTER TABLE public.org_sync_review_resolutions DROP CONSTRAINT %I',c);
END $$;
ALTER TABLE org_sync_review_resolutions ADD CONSTRAINT org_sync_review_resolutions_outcome
 CHECK((outcome='ACCEPTED' AND entity_revision=1) OR(outcome='REFUSED' AND entity_revision IS NULL AND reading_id IS NULL AND reason<>''));

-- 2. The 199 resolution source with one branch per owner. Meter: unchanged.
-- trip.start: its held start; an acceptance names exactly its held reading
-- (or none). trip.finish, trip.event.record, trip.submit and the EMP-TRIP-011
-- lifecycle commands (trip.pause, trip.resume, trip.cancel): refusal only.
-- Self-review also refuses a held start's declared driver (BR-REV-003).
CREATE OR REPLACE FUNCTION org_sync_review_resolution_source() RETURNS trigger LANGUAGE plpgsql SET search_path=pg_catalog AS $$
DECLARE i public.org_sync_review_items%ROWTYPE;o public.org_sync_operations%ROWTYPE;h public.org_meter_held_readings%ROWTYPE;s public.org_trip_held_starts%ROWTYPE;
 j JSONB;employee UUID;author UUID;driver UUID;received TIMESTAMPTZ;
BEGIN
 IF NEW.org_id IS DISTINCT FROM NULLIF(current_setting('anpfuel.org_id',true),'')::uuid THEN RAISE EXCEPTION USING ERRCODE='42501',MESSAGE='organization scope denied';END IF;
 PERFORM 1 FROM public.organizations WHERE org_id=NEW.org_id AND state='ACTIVE' AND has_snapshot FOR UPDATE;
 IF NOT FOUND THEN RAISE EXCEPTION USING ERRCODE='55000',MESSAGE='organization does not accept review decisions';END IF;
 SELECT * INTO i FROM public.org_sync_review_items WHERE org_id=NEW.org_id AND id=NEW.review_item_id FOR UPDATE;
 IF NOT FOUND THEN RAISE EXCEPTION USING ERRCODE='23503',MESSAGE='private review not found';END IF;
 SELECT * INTO o FROM public.org_sync_operations WHERE org_id=i.org_id AND operation_id=i.operation_id;
 IF i.operation_type='meter.reading.record' THEN
  SELECT * INTO h FROM public.org_meter_held_readings WHERE org_id=i.org_id AND review_item_id=i.id;
  IF NOT FOUND THEN RAISE EXCEPTION USING ERRCODE='23514',MESSAGE='actual held review source unavailable';END IF;
  IF NEW.outcome='ACCEPTED' AND NEW.reading_id IS DISTINCT FROM h.reading_id THEN RAISE EXCEPTION USING ERRCODE='23514',MESSAGE='review applied reading unavailable';END IF;
 ELSIF i.operation_type='trip.start' THEN
  SELECT * INTO s FROM public.org_trip_held_starts WHERE org_id=i.org_id AND review_item_id=i.id AND operation_id=i.operation_id;
  IF NOT FOUND THEN RAISE EXCEPTION USING ERRCODE='23514',MESSAGE='actual held review source unavailable';END IF;
  IF NEW.outcome='ACCEPTED' AND NEW.reading_id IS DISTINCT FROM s.reading_id THEN RAISE EXCEPTION USING ERRCODE='23514',MESSAGE='review applied reading unavailable';END IF;
  driver=public.org_sync_review_employee_canonical(s.org_id,s.driver_employee_id);
  IF driver IS NULL THEN RAISE EXCEPTION USING ERRCODE='23514',MESSAGE='reviewer source unavailable';END IF;
 ELSIF i.operation_type IN('trip.finish','trip.event.record','trip.submit','trip.pause','trip.resume','trip.cancel') THEN
  IF NOT((i.operation_type='trip.finish' AND EXISTS(SELECT 1 FROM public.org_trip_held_finishes x WHERE x.org_id=i.org_id AND x.review_item_id=i.id AND x.operation_id=i.operation_id))
  OR(i.operation_type='trip.event.record' AND EXISTS(SELECT 1 FROM public.org_trip_held_events x WHERE x.org_id=i.org_id AND x.review_item_id=i.id AND x.operation_id=i.operation_id))
  OR(i.operation_type='trip.submit' AND EXISTS(SELECT 1 FROM public.org_trip_held_submissions x WHERE x.org_id=i.org_id AND x.review_item_id=i.id AND x.operation_id=i.operation_id))
  OR(i.operation_type IN('trip.pause','trip.resume','trip.cancel') AND EXISTS(SELECT 1 FROM public.org_trip_held_transitions x WHERE x.org_id=i.org_id AND x.review_item_id=i.id AND x.operation_id=i.operation_id)))
  THEN RAISE EXCEPTION USING ERRCODE='23514',MESSAGE='actual held review source unavailable';END IF;
  IF NEW.outcome<>'REFUSED' OR NEW.reading_id IS NOT NULL THEN RAISE EXCEPTION USING ERRCODE='23514',MESSAGE='held trip fact can only be refused';END IF;
 ELSE
  RAISE EXCEPTION USING ERRCODE='23514',MESSAGE='actual held review source unavailable';
 END IF;
 SELECT public.org_sync_review_employee_canonical(p.org_id,p.employee_id) INTO employee FROM public.org_memberships p JOIN public.company_web_sessions c ON c.id=NEW.reviewer_session_id AND c.account_id=p.account_id
 WHERE p.org_id=NEW.org_id AND p.id=NEW.reviewer_membership_id AND p.account_id=NEW.reviewer_account_id AND p.state='ACTIVE';
 author=public.org_sync_review_employee_canonical(i.org_id,i.author_employee_id);
 IF employee IS NULL OR author IS NULL OR employee IS DISTINCT FROM NEW.reviewer_employee_id THEN RAISE EXCEPTION USING ERRCODE='23514',MESSAGE='reviewer source unavailable';END IF;
 IF NEW.reviewer_account_id=o.owner_account_id OR NEW.reviewer_membership_id=o.membership_id OR employee=author OR(driver IS NOT NULL AND employee=driver) THEN RAISE EXCEPTION USING ERRCODE='P9006',MESSAGE='self review is not allowed';END IF;
 SELECT a.received_at INTO received FROM public.org_sync_acknowledgements a WHERE a.org_id=i.org_id AND a.operation_id=i.operation_id AND a.review_item_id=i.id AND a.review_proof_version=i.proof_version;
 IF NOT FOUND OR NEW.resolved_at<received OR NEW.resolved_at>clock_timestamp() THEN RAISE EXCEPTION USING ERRCODE='23514',MESSAGE='review decision time unavailable';END IF;
 j=convert_from(NEW.canonical_resolution,'UTF8')::jsonb;
 IF jsonb_typeof(j)<>'object' OR NOT(j ?& ARRAY['schema_version','operation_id','payload_digest','review_item_id','resolution_id','review_revision','outcome','reason','resolved_at','entity_revision'])
 OR j-ARRAY['schema_version','operation_id','payload_digest','review_item_id','resolution_id','review_revision','outcome','reason','resolved_at','entity_revision']<>'{}'::jsonb
 OR j->'schema_version' IS DISTINCT FROM '1'::jsonb OR j->>'operation_id' IS DISTINCT FROM o.operation_id::text OR j->>'payload_digest' IS DISTINCT FROM o.payload_digest
 OR j->>'review_item_id' IS DISTINCT FROM i.id::text OR j->>'resolution_id' IS DISTINCT FROM NEW.id::text OR j->'review_revision' IS DISTINCT FROM '2'::jsonb
 OR j->>'outcome' IS DISTINCT FROM NEW.outcome OR j->>'reason' IS DISTINCT FROM NEW.reason OR(j->>'resolved_at')::timestamptz IS DISTINCT FROM NEW.resolved_at
 OR j->'entity_revision' IS DISTINCT FROM COALESCE(to_jsonb(NEW.entity_revision),'null'::jsonb)
 THEN RAISE EXCEPTION USING ERRCODE='23514',MESSAGE='review resolution source unavailable';END IF;
 RETURN NEW;
END $$;

-- 3. The 199 resolution publication: the audit event for every owner; an
-- accepted meter item needs its reviewed reading, an accepted trip start its
-- stored trip of the same operation and, when it declared one, its reviewed
-- TRIP_START reading; every acceptance needs its readiness effect.
CREATE OR REPLACE FUNCTION org_sync_review_resolution_complete() RETURNS trigger LANGUAGE plpgsql SET search_path=pg_catalog AS $$
DECLARE kind TEXT;
BEGIN
 SELECT operation_type INTO kind FROM public.org_sync_review_items WHERE org_id=NEW.org_id AND id=NEW.review_item_id;
 IF NOT EXISTS(SELECT 1 FROM public.org_audit_events a WHERE a.org_id=NEW.org_id AND a.operation_id=NEW.id AND a.actor_id=NEW.reviewer_account_id AND a.action='offl.review.resolved' AND a.target_type='offl.review' AND a.target_id=NEW.review_item_id AND a.request_hash=NEW.request_hash)
 THEN RAISE EXCEPTION USING ERRCODE='23514',MESSAGE='review resolution publication unavailable';END IF;
 IF NEW.outcome<>'ACCEPTED' THEN RETURN NULL;END IF;
 IF kind='meter.reading.record' THEN
  IF NOT EXISTS(SELECT 1 FROM public.org_meter_readings r JOIN public.org_meter_held_readings h ON(h.org_id,h.review_item_id)=(r.org_id,NEW.review_item_id) JOIN public.org_sync_review_items i ON(i.org_id,i.id)=(h.org_id,h.review_item_id) JOIN public.org_sync_operations o ON(o.org_id,o.operation_id)=(i.org_id,i.operation_id)
  JOIN public.org_sync_acknowledgements a ON(a.org_id,a.operation_id)=(o.org_id,o.operation_id)
  WHERE r.org_id=NEW.org_id AND r.id=NEW.reading_id AND r.review_resolution_id=NEW.id AND r.operation_id=o.operation_id AND r.meter_id=h.meter_id AND r.segment_id=h.segment_id AND r.value=h.value
  AND(r.occurred_at,r.occurred_submicro)=(h.occurred_at,h.occurred_submicro) AND r.author_membership_id=o.membership_id AND r.capture_kind='ENROLLED_DEVICE' AND r.enrollment_id=o.enrollment_id AND r.received_at=a.received_at)
  THEN RAISE EXCEPTION USING ERRCODE='23514',MESSAGE='review resolution publication unavailable';END IF;
 ELSIF kind='trip.start' THEN
  IF NOT EXISTS(SELECT 1 FROM public.org_trip_held_starts h JOIN public.org_trips t ON(t.org_id,t.id,t.operation_id)=(h.org_id,h.trip_id,h.operation_id)
  JOIN public.org_sync_acknowledgements a ON(a.org_id,a.operation_id)=(h.org_id,h.operation_id)
  WHERE h.org_id=NEW.org_id AND h.review_item_id=NEW.review_item_id AND t.received_at=a.received_at
  AND((h.reading_id IS NULL AND NEW.reading_id IS NULL) OR EXISTS(SELECT 1 FROM public.org_meter_readings r WHERE r.org_id=h.org_id AND r.id=h.reading_id AND r.id=NEW.reading_id
   AND r.review_resolution_id=NEW.id AND r.operation_id=h.operation_id AND r.origin='TRIP_START' AND r.received_at=a.received_at)))
  THEN RAISE EXCEPTION USING ERRCODE='23514',MESSAGE='review resolution publication unavailable';END IF;
 ELSE
  RAISE EXCEPTION USING ERRCODE='23514',MESSAGE='review resolution publication unavailable';
 END IF;
 IF NOT EXISTS(SELECT 1 FROM public.org_sync_effects e JOIN public.org_sync_review_items i ON(i.org_id,i.operation_id)=(e.org_id,e.operation_id) WHERE i.org_id=NEW.org_id AND i.id=NEW.review_item_id AND e.entity_revision=NEW.entity_revision)
 THEN RAISE EXCEPTION USING ERRCODE='23514',MESSAGE='review resolution publication unavailable';END IF;
 RETURN NULL;
END $$;

-- 4. The 216 observation guard with the reviewed TRIP_START reading of an
-- accepted held start; the owner is read first so a meter or an ordinary
-- reading never reads a trip table. Every other check is unchanged.
CREATE OR REPLACE FUNCTION org_meter_observation_guard() RETURNS trigger LANGUAGE plpgsql SET search_path=pg_catalog AS $$
DECLARE m public.org_meters%ROWTYPE;s public.org_meter_segments%ROWTYPE;original_received TIMESTAMPTZ;kind TEXT;
BEGIN
 IF NEW.org_id IS DISTINCT FROM NULLIF(current_setting('anpfuel.org_id',true),'')::uuid THEN RAISE EXCEPTION USING ERRCODE='42501',MESSAGE='organization scope denied';END IF;
 PERFORM 1 FROM public.organizations WHERE org_id=NEW.org_id AND has_snapshot FOR UPDATE;
 SELECT * INTO m FROM public.org_meters WHERE org_id=NEW.org_id AND id=NEW.meter_id FOR UPDATE;
 IF NOT FOUND THEN RAISE EXCEPTION USING ERRCODE='23503',MESSAGE='private meter not found';END IF;
 SELECT * INTO s FROM public.org_meter_segments WHERE org_id=NEW.org_id AND meter_id=NEW.meter_id AND id=NEW.segment_id;
 IF NOT FOUND OR(NEW.occurred_at,NEW.occurred_submicro)<(m.installed_at,m.installed_submicro) OR(m.removed_at IS NOT NULL AND(NEW.occurred_at,NEW.occurred_submicro)>=(m.removed_at,m.removed_submicro))
 OR(NEW.occurred_at,NEW.occurred_submicro)<(s.opened_at,s.opened_submicro) OR(s.closed_at IS NOT NULL AND(NEW.occurred_at,NEW.occurred_submicro)>=(s.closed_at,s.closed_submicro))
 THEN RAISE EXCEPTION USING ERRCODE='23514',MESSAGE='observation outside meter service interval';END IF;
 IF NEW.value>(CASE m.kind WHEN 'ODOMETER' THEN 1000000000000 ELSE 3600000000 END) OR(m.modulus IS NOT NULL AND NEW.value>=m.modulus)
 OR NOT EXISTS(SELECT 1 FROM pg_timezone_names WHERE name=NEW.entry_zone)
 THEN RAISE EXCEPTION USING ERRCODE='23514',MESSAGE='invalid meter observation';END IF;
 IF NEW.review_resolution_id IS NOT NULL THEN
  SELECT i.operation_type INTO kind FROM public.org_sync_review_resolutions z JOIN public.org_sync_review_items i ON(i.org_id,i.id)=(z.org_id,z.review_item_id)
  WHERE z.org_id=NEW.org_id AND z.id=NEW.review_resolution_id;
  IF kind='trip.start' THEN
   SELECT a.received_at INTO original_received FROM public.org_sync_review_resolutions z JOIN public.org_sync_review_items i ON(i.org_id,i.id)=(z.org_id,z.review_item_id)
   JOIN public.org_trip_held_starts h ON(h.org_id,h.review_item_id,h.operation_id)=(i.org_id,i.id,i.operation_id) JOIN public.org_sync_operations o ON(o.org_id,o.operation_id)=(i.org_id,i.operation_id)
   JOIN public.org_sync_acknowledgements a ON(a.org_id,a.operation_id,a.review_item_id,a.review_proof_version)=(i.org_id,i.operation_id,i.id,i.proof_version)
   WHERE z.org_id=NEW.org_id AND z.id=NEW.review_resolution_id AND z.outcome='ACCEPTED' AND z.reading_id=NEW.id AND NEW.id=h.reading_id
   AND NEW.meter_id=h.meter_id AND NEW.segment_id=h.segment_id AND NEW.value=h.reading_value AND(NEW.occurred_at,NEW.occurred_submicro)=(h.occurred_at,h.occurred_submicro)
   AND NEW.entry_zone=h.entry_zone AND NEW.device_offset_seconds=h.device_offset_seconds AND NEW.operation_id=o.operation_id
   AND NEW.author_membership_id=o.membership_id AND NEW.capture_kind='ENROLLED_DEVICE' AND NEW.enrollment_id=o.enrollment_id AND NEW.web_session_id IS NULL AND NEW.origin='TRIP_START' AND NEW.method='TYPED' AND NEW.state IN('VALID','SUSPECT');
  ELSE
   SELECT a.received_at INTO original_received FROM public.org_sync_review_resolutions z JOIN public.org_sync_review_items i ON(i.org_id,i.id)=(z.org_id,z.review_item_id)
   JOIN public.org_meter_held_readings h ON(h.org_id,h.review_item_id)=(i.org_id,i.id) JOIN public.org_sync_operations o ON(o.org_id,o.operation_id)=(i.org_id,i.operation_id)
   JOIN public.org_sync_acknowledgements a ON(a.org_id,a.operation_id,a.review_item_id,a.review_proof_version)=(i.org_id,i.operation_id,i.id,i.proof_version)
   WHERE z.org_id=NEW.org_id AND z.id=NEW.review_resolution_id AND z.outcome='ACCEPTED' AND z.reading_id=NEW.id AND NEW.id=h.reading_id
   AND NEW.meter_id=h.meter_id AND NEW.segment_id=h.segment_id AND NEW.value=h.value AND(NEW.occurred_at,NEW.occurred_submicro)=(h.occurred_at,h.occurred_submicro)
   AND NEW.entry_zone=h.entry_zone AND NEW.device_offset_seconds=h.device_offset_seconds AND NEW.operation_id=o.operation_id AND NEW.request_hash=h.request_hash
   AND NEW.author_membership_id=o.membership_id AND NEW.capture_kind='ENROLLED_DEVICE' AND NEW.enrollment_id=o.enrollment_id AND NEW.web_session_id IS NULL AND NEW.origin=h.origin AND NEW.method=h.method AND NEW.state IN('VALID','SUSPECT');
  END IF;
  IF NOT FOUND THEN RAISE EXCEPTION USING ERRCODE='23514',MESSAGE='reviewed observation source unavailable';END IF;
  NEW.received_at=original_received;RETURN NEW;
 END IF;
 IF EXISTS(SELECT 1 FROM public.org_meter_held_readings WHERE org_id=NEW.org_id AND(reading_id=NEW.id OR operation_id=NEW.operation_id)) THEN RAISE EXCEPTION USING ERRCODE='23505',CONSTRAINT='org_meter_readings_pkey',MESSAGE='meter reading identity unavailable';END IF;
 IF EXISTS(SELECT 1 FROM public.org_assets a WHERE a.org_id=NEW.org_id AND a.id=m.asset_id AND a.disposed_at IS NOT NULL AND(NEW.occurred_at,NEW.occurred_submicro)>=(a.disposed_at,a.disposed_submicro)) THEN RAISE EXCEPTION USING ERRCODE='23514',MESSAGE='observation after asset disposal requires review';END IF;
 IF NEW.capture_kind='ENROLLED_DEVICE' THEN
  IF NOT EXISTS(SELECT 1 FROM public.org_device_enrollments WHERE org_id=NEW.org_id AND id=NEW.enrollment_id AND membership_id=NEW.author_membership_id AND state='ENROLLED') THEN RAISE EXCEPTION USING ERRCODE='23503',MESSAGE='observation source not found';END IF;
 ELSE
  IF NOT EXISTS(SELECT 1 FROM public.company_web_sessions c JOIN public.org_memberships p ON p.account_id=c.account_id WHERE p.org_id=NEW.org_id AND p.id=NEW.author_membership_id AND c.id=NEW.web_session_id) THEN RAISE EXCEPTION USING ERRCODE='23503',MESSAGE='observation source not found';END IF;
 END IF;
 NEW.received_at=clock_timestamp();RETURN NEW;
END $$;

-- 5. The 199 effect source: an accepted receipt, else the accepted reviewed
-- resolution of a meter item or of a trip start whose trip is stored. Each
-- source is its own statement and only an operation that has an accepted
-- resolution reads an owner's tables, so an ordinary or unresolved effect
-- needs no privilege beyond the 199 body's.
CREATE OR REPLACE FUNCTION org_sync_effect_source() RETURNS trigger LANGUAGE plpgsql SET search_path=pg_catalog AS $$
BEGIN
 IF EXISTS(SELECT 1 FROM public.org_sync_acknowledgements a WHERE a.org_id=NEW.org_id AND a.operation_id=NEW.operation_id AND a.result='ACCEPTED' AND a.entity_revision=NEW.entity_revision) THEN RETURN NEW;END IF;
 IF NOT EXISTS(SELECT 1 FROM public.org_sync_review_items i JOIN public.org_sync_review_resolutions z ON(z.org_id,z.review_item_id)=(i.org_id,i.id)
 WHERE i.org_id=NEW.org_id AND i.operation_id=NEW.operation_id AND z.outcome='ACCEPTED' AND z.entity_revision=NEW.entity_revision)
 THEN RAISE EXCEPTION USING ERRCODE='23514',MESSAGE='intake applied effect source unavailable';END IF;
 IF EXISTS(SELECT 1 FROM public.org_sync_review_items i JOIN public.org_sync_review_resolutions z ON(z.org_id,z.review_item_id)=(i.org_id,i.id)
 JOIN public.org_meter_held_readings h ON(h.org_id,h.review_item_id)=(i.org_id,i.id) JOIN public.org_meter_readings r ON(r.org_id,r.id,r.review_resolution_id)=(z.org_id,z.reading_id,z.id)
 JOIN public.org_sync_acknowledgements a ON(a.org_id,a.operation_id,a.review_item_id,a.review_proof_version)=(i.org_id,i.operation_id,i.id,i.proof_version)
 WHERE i.org_id=NEW.org_id AND i.operation_id=NEW.operation_id AND z.outcome='ACCEPTED' AND z.entity_revision=NEW.entity_revision AND r.operation_id=i.operation_id AND r.id=h.reading_id) THEN RETURN NEW;END IF;
 IF EXISTS(SELECT 1 FROM public.org_sync_review_items i JOIN public.org_sync_review_resolutions z ON(z.org_id,z.review_item_id)=(i.org_id,i.id)
 JOIN public.org_trip_held_starts h ON(h.org_id,h.review_item_id,h.operation_id)=(i.org_id,i.id,i.operation_id) JOIN public.org_trips t ON(t.org_id,t.id,t.operation_id)=(h.org_id,h.trip_id,h.operation_id)
 JOIN public.org_sync_acknowledgements a ON(a.org_id,a.operation_id,a.review_item_id,a.review_proof_version)=(i.org_id,i.operation_id,i.id,i.proof_version)
 WHERE i.org_id=NEW.org_id AND i.operation_id=NEW.operation_id AND z.outcome='ACCEPTED' AND z.entity_revision=NEW.entity_revision) THEN RETURN NEW;END IF;
 RAISE EXCEPTION USING ERRCODE='23514',MESSAGE='intake applied effect source unavailable';
END $$;

-- 6. The 220 trip source. A held start's identity stays reserved except for
-- the accepted resolution of exactly that held start: every held fact equal,
-- received at its original receipt, and never overlapping a stored trip of
-- the asset (none in progress or paused, finished or cancelled after the
-- declared start, VAL-07).
CREATE OR REPLACE FUNCTION org_trip_source() RETURNS trigger LANGUAGE plpgsql SET search_path=pg_catalog AS $$
DECLARE a public.org_assets%ROWTYPE;actual_unit UUID;actual_centre UUID;zone TEXT;held public.org_trip_held_starts%ROWTYPE;original TIMESTAMPTZ;
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
 SELECT * INTO held FROM public.org_trip_held_starts h WHERE h.org_id=NEW.org_id AND h.trip_id=NEW.id;
 IF FOUND THEN
  SELECT k.received_at INTO original FROM public.org_sync_review_resolutions z JOIN public.org_sync_review_items i ON(i.org_id,i.id)=(z.org_id,z.review_item_id)
  JOIN public.org_sync_acknowledgements k ON(k.org_id,k.operation_id,k.review_item_id,k.review_proof_version)=(i.org_id,i.operation_id,i.id,i.proof_version)
  WHERE z.org_id=NEW.org_id AND z.review_item_id=held.review_item_id AND z.outcome='ACCEPTED' AND z.entity_revision=1 AND i.operation_id=held.operation_id AND NEW.operation_id=held.operation_id
  AND NEW.asset_id=held.asset_id AND NEW.driver_employee_id=held.driver_employee_id AND(NEW.occurred_at,NEW.occurred_submicro)=(held.occurred_at,held.occurred_submicro)
  AND NEW.entry_zone=held.entry_zone AND NEW.device_offset_seconds=held.device_offset_seconds AND NEW.boot_id IS NOT DISTINCT FROM held.boot_id AND NEW.elapsed_millis IS NOT DISTINCT FROM held.elapsed_millis
  AND NEW.meter_id IS NOT DISTINCT FROM held.meter_id AND NEW.segment_id IS NOT DISTINCT FROM held.segment_id AND NEW.reading_id IS NOT DISTINCT FROM held.reading_id
  AND NEW.missing_reading_reason IS NOT DISTINCT FROM held.missing_reading_reason AND NEW.author_employee_id=i.author_employee_id;
  IF NOT FOUND THEN RAISE EXCEPTION USING ERRCODE='23505',CONSTRAINT='org_trips_pkey',MESSAGE='trip identity unavailable';END IF;
  -- EMP-TRIP-011: a paused trip holds the asset; a cancelled one until its cancellation.
  IF EXISTS(SELECT 1 FROM public.org_trips t2 LEFT JOIN public.org_trip_finishes f2 ON(f2.org_id,f2.trip_id)=(t2.org_id,t2.id)
   LEFT JOIN public.org_trip_transitions c2 ON(c2.org_id,c2.trip_id,c2.command)=(t2.org_id,t2.id,'CANCEL')
   WHERE t2.org_id=NEW.org_id AND t2.asset_id=NEW.asset_id AND t2.id<>NEW.id AND(t2.state NOT IN('COMPLETED','CANCELLED')
   OR(COALESCE(f2.occurred_at,c2.occurred_at),COALESCE(f2.occurred_submicro,c2.occurred_submicro))>(NEW.occurred_at,NEW.occurred_submicro)))
  THEN RAISE EXCEPTION USING ERRCODE='23514',MESSAGE='accepted trip overlaps another trip of the asset';END IF;
 END IF;
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
 NEW.received_at=COALESCE(original,clock_timestamp());RETURN NEW;
END $$;

-- 7. The trip publication (the 000233 body of EMP-TRIP-011): an accepted
-- start receipt, else the reviewed publication of an accepted held start
-- (its resolution and readiness), checked in that order; the finish branch
-- is the 000233 one unchanged.
CREATE FUNCTION org_trip_review_published(p_org UUID,p_trip UUID,p_op UUID) RETURNS BOOLEAN LANGUAGE SQL STABLE SET search_path=pg_catalog AS $$
 SELECT EXISTS(SELECT 1 FROM public.org_trip_held_starts h JOIN public.org_sync_review_resolutions z ON(z.org_id,z.review_item_id)=(h.org_id,h.review_item_id)
  JOIN public.org_sync_effects e ON(e.org_id,e.operation_id)=(h.org_id,h.operation_id)
  WHERE h.org_id=p_org AND h.trip_id=p_trip AND h.operation_id=p_op AND z.outcome='ACCEPTED' AND z.entity_revision=1 AND e.entity_revision=1)
 $$;
CREATE OR REPLACE FUNCTION org_trip_complete() RETURNS trigger LANGUAGE plpgsql SET search_path=pg_catalog AS $$
DECLARE t public.org_trips%ROWTYPE;f public.org_trip_finishes%ROWTYPE;prior_org TEXT;
BEGIN
 prior_org=current_setting('anpfuel.org_id',true);PERFORM set_config('anpfuel.org_id',NEW.org_id::text,true);
 IF TG_TABLE_NAME='org_trips' THEN
  SELECT * INTO t FROM public.org_trips WHERE org_id=NEW.org_id AND id=NEW.id;
  IF NOT FOUND THEN RAISE EXCEPTION USING ERRCODE='23514',MESSAGE='trip publication unavailable';END IF;
  IF NOT public.org_trip_published(t.org_id,t.operation_id,1) THEN
   IF NOT public.org_trip_review_published(t.org_id,t.id,t.operation_id) THEN RAISE EXCEPTION USING ERRCODE='23514',MESSAGE='trip publication unavailable';END IF;
  END IF;
  IF NOT public.org_trip_reading_valid(t.org_id,t.asset_id,t.meter_id,t.segment_id,t.reading_id,'TRIP_START',t.operation_id,t.author_membership_id,t.enrollment_id,t.occurred_at,t.occurred_submicro,t.entry_zone,t.device_offset_seconds)
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

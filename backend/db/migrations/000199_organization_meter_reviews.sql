-- EMP-OFFL-008. Existing REVIEW receipts remain immutable, explicitly unproven
-- legacy history. Only new version1 proofs can participate in the core queue.
ALTER TABLE org_sync_acknowledgements ADD COLUMN review_proof_version SMALLINT CHECK(review_proof_version=1);
ALTER TABLE org_sync_operations ADD CONSTRAINT org_sync_operation_review_identity UNIQUE(org_id,operation_id,operation_type,operation_schema,payload_digest);
ALTER TABLE org_sync_acknowledgements ADD CONSTRAINT org_sync_ack_review_identity UNIQUE(org_id,operation_id,review_item_id,result,code,review_proof_version);

CREATE TABLE org_sync_review_items(
 org_id UUID NOT NULL, id UUID NOT NULL CHECK(id<>'00000000-0000-0000-0000-000000000000'::uuid),operation_id UUID NOT NULL,
 operation_type TEXT NOT NULL,operation_schema BIGINT NOT NULL,payload_digest TEXT NOT NULL,
 code TEXT NOT NULL CHECK(code IN('ORPHAN_REFERENCE','ACCESS_ENDED_BEFORE_RECEIPT','PERMISSION_CHANGED','ASSET_UNAVAILABLE','ASSET_BUSY','TRIP_ALREADY_CLOSED','PERIOD_LOCKED','STALE_REVISION','ORG_SUSPENDED_GRACE','PLAN_CHANGED')),
 author_employee_id UUID NOT NULL,proof_version SMALLINT NOT NULL DEFAULT 1 CHECK(proof_version=1),
 acknowledgement_result TEXT NOT NULL DEFAULT 'ACCEPTED_FOR_REVIEW' CHECK(acknowledgement_result='ACCEPTED_FOR_REVIEW'),
 expiry_policy TEXT NOT NULL DEFAULT 'review-expiry-v1' CHECK(expiry_policy='review-expiry-v1'),expiry_days SMALLINT NOT NULL DEFAULT 60 CHECK(expiry_days=60),
 PRIMARY KEY(org_id,id),UNIQUE(org_id,operation_id),UNIQUE(org_id,operation_id,id,proof_version),UNIQUE(org_id,id,operation_id),
 FOREIGN KEY(org_id,operation_id,operation_type,operation_schema,payload_digest) REFERENCES org_sync_operations(org_id,operation_id,operation_type,operation_schema,payload_digest),
 FOREIGN KEY(org_id,author_employee_id) REFERENCES org_employee_identities(org_id,id),
 FOREIGN KEY(org_id,operation_id,id,acknowledgement_result,code,proof_version) REFERENCES org_sync_acknowledgements(org_id,operation_id,review_item_id,result,code,review_proof_version) DEFERRABLE INITIALLY DEFERRED
);
-- MATCH SIMPLE deliberately exempts only retained legacy NULL markers. The
-- INSERT trigger below forces every newly-created REVIEW into version1.
ALTER TABLE org_sync_acknowledgements ADD CONSTRAINT org_sync_ack_review_proof_fkey
 FOREIGN KEY(org_id,operation_id,review_item_id,review_proof_version) REFERENCES org_sync_review_items(org_id,operation_id,id,proof_version) MATCH SIMPLE DEFERRABLE INITIALLY DEFERRED;

CREATE TABLE org_meter_held_readings(
 org_id UUID NOT NULL,review_item_id UUID NOT NULL,operation_id UUID NOT NULL,reading_id UUID NOT NULL CHECK(reading_id<>'00000000-0000-0000-0000-000000000000'::uuid),
 asset_id UUID NOT NULL,meter_id UUID NOT NULL,segment_id UUID NOT NULL,value BIGINT NOT NULL CHECK(value>=0),
 occurred_at TIMESTAMPTZ NOT NULL,occurred_submicro SMALLINT NOT NULL CHECK(occurred_submicro BETWEEN 0 AND 999),entry_zone TEXT NOT NULL,
 device_offset_seconds BIGINT NOT NULL CHECK(device_offset_seconds BETWEEN -64800 AND 64800),origin TEXT NOT NULL CHECK(origin='STANDALONE'),method TEXT NOT NULL CHECK(method='TYPED'),
 request_hash TEXT NOT NULL CHECK(request_hash~'^[a-f0-9]{64}$'),unit_id UUID,
 PRIMARY KEY(org_id,review_item_id),UNIQUE(org_id,meter_id,operation_id),UNIQUE(org_id,reading_id),
 FOREIGN KEY(org_id,review_item_id,operation_id) REFERENCES org_sync_review_items(org_id,id,operation_id),
 CONSTRAINT org_meter_held_readings_asset_fkey FOREIGN KEY(org_id,asset_id) REFERENCES org_assets(org_id,id),
 FOREIGN KEY(org_id,asset_id,meter_id) REFERENCES org_meters(org_id,asset_id,id),
 FOREIGN KEY(org_id,meter_id,segment_id) REFERENCES org_meter_segments(org_id,meter_id,id),
 CONSTRAINT org_meter_held_readings_unit_fkey FOREIGN KEY(org_id,unit_id) REFERENCES org_units(org_id,id),
 CHECK(isfinite(occurred_at) AND occurred_at>='0001-01-01 00:00:00+00'::timestamptz AND occurred_at<'10000-01-01 00:00:00+00'::timestamptz)
);

CREATE TABLE org_sync_review_resolutions(
 org_id UUID NOT NULL,id UUID NOT NULL CHECK(id<>'00000000-0000-0000-0000-000000000000'::uuid),review_item_id UUID NOT NULL,
 expected_revision BIGINT NOT NULL CHECK(expected_revision=1),review_revision BIGINT NOT NULL DEFAULT 2 CHECK(review_revision=2),
 outcome TEXT NOT NULL CHECK(outcome IN('ACCEPTED','REFUSED')),reason TEXT NOT NULL CHECK(reason='' OR org_asset_text_valid(reason,280,true)),
 reviewer_account_id UUID NOT NULL,reviewer_membership_id UUID NOT NULL,reviewer_employee_id UUID NOT NULL,reviewer_session_id UUID NOT NULL,
 request_hash TEXT NOT NULL CHECK(request_hash~'^[a-f0-9]{64}$'),resolved_at TIMESTAMPTZ NOT NULL,
 entity_revision BIGINT,reading_id UUID,canonical_resolution BYTEA NOT NULL CHECK(octet_length(canonical_resolution) BETWEEN 1 AND 4096),
 PRIMARY KEY(org_id,id),UNIQUE(org_id,review_item_id),
 FOREIGN KEY(org_id,review_item_id) REFERENCES org_sync_review_items(org_id,id),
 FOREIGN KEY(org_id,reviewer_membership_id) REFERENCES org_memberships(org_id,id),
 FOREIGN KEY(org_id,reviewer_employee_id) REFERENCES org_employee_identities(org_id,id),
 FOREIGN KEY(org_id,id) REFERENCES org_audit_events(org_id,operation_id) DEFERRABLE INITIALLY DEFERRED,
 FOREIGN KEY(org_id,reading_id) REFERENCES org_meter_readings(org_id,id) DEFERRABLE INITIALLY DEFERRED,
 CHECK(isfinite(resolved_at) AND resolved_at>'0001-01-01 00:00:00+00'::timestamptz AND resolved_at<'10000-01-01 00:00:00+00'::timestamptz),
 CHECK((outcome='ACCEPTED' AND entity_revision=1 AND reading_id IS NOT NULL) OR(outcome='REFUSED' AND entity_revision IS NULL AND reading_id IS NULL AND reason<>''))
);
ALTER TABLE org_meter_readings ADD COLUMN review_resolution_id UUID;
ALTER TABLE org_meter_readings ADD CONSTRAINT org_meter_reading_review_resolution_fkey FOREIGN KEY(org_id,review_resolution_id) REFERENCES org_sync_review_resolutions(org_id,id) DEFERRABLE INITIALLY DEFERRED;

CREATE FUNCTION org_sync_new_review_proof() RETURNS trigger LANGUAGE plpgsql SET search_path=pg_catalog AS $$
BEGIN
 IF NEW.result='ACCEPTED_FOR_REVIEW' THEN
  IF NEW.review_proof_version IS NOT NULL AND NEW.review_proof_version<>1 THEN RAISE EXCEPTION USING ERRCODE='23514',MESSAGE='review proof version unavailable';END IF;
  NEW.review_proof_version=1;
 ELSIF NEW.review_proof_version IS NOT NULL THEN RAISE EXCEPTION USING ERRCODE='23514',MESSAGE='unexpected review proof';END IF;
 RETURN NEW;
END $$;
CREATE TRIGGER org_sync_ack_review_proof BEFORE INSERT ON org_sync_acknowledgements FOR EACH ROW EXECUTE FUNCTION org_sync_new_review_proof();

-- This source hook knows only delivered owners. An extension must check that
-- owner's real held row; arbitrary identifiers and unknown owners fail closed.
CREATE FUNCTION org_sync_review_source_valid(org UUID,review UUID,owner_type TEXT,owner_schema BIGINT) RETURNS BOOLEAN
 LANGUAGE SQL VOLATILE SET search_path=pg_catalog AS $$
 SELECT owner_type='meter.reading.record' AND owner_schema=1 AND EXISTS(
 SELECT 1 FROM public.org_sync_review_items i JOIN public.org_meter_held_readings h ON(h.org_id,h.review_item_id,h.operation_id)=(i.org_id,i.id,i.operation_id)
 JOIN public.org_sync_operations o ON(o.org_id,o.operation_id,o.operation_type,o.operation_schema,o.payload_digest)=(i.org_id,i.operation_id,i.operation_type,i.operation_schema,i.payload_digest)
 JOIN public.org_sync_acknowledgements a ON(a.org_id,a.operation_id,a.review_item_id,a.result,a.code,a.review_proof_version)=(i.org_id,i.operation_id,i.id,i.acknowledgement_result,i.code,i.proof_version)
 JOIN public.org_memberships m ON(m.org_id,m.id,m.employee_id)=(i.org_id,o.membership_id,i.author_employee_id)
 WHERE i.org_id=org AND i.id=review AND i.operation_type=owner_type AND i.operation_schema=owner_schema AND o.entity_type='meter_reading' AND o.entity_id=h.reading_id)
 $$;
CREATE FUNCTION org_sync_review_complete() RETURNS trigger LANGUAGE plpgsql SET search_path=pg_catalog AS $$
DECLARE i public.org_sync_review_items%ROWTYPE;prior_org TEXT;
BEGIN
 prior_org=current_setting('anpfuel.org_id',true);PERFORM set_config('anpfuel.org_id',NEW.org_id::text,true);
 IF TG_TABLE_NAME='org_sync_review_items' THEN SELECT * INTO i FROM public.org_sync_review_items WHERE org_id=NEW.org_id AND id=NEW.id;
 ELSE SELECT * INTO i FROM public.org_sync_review_items WHERE org_id=NEW.org_id AND id=NEW.review_item_id;END IF;
 IF NOT FOUND OR NOT public.org_sync_review_source_valid(i.org_id,i.id,i.operation_type,i.operation_schema) THEN RAISE EXCEPTION USING ERRCODE='23514',MESSAGE='actual held review source unavailable';END IF;
 PERFORM set_config('anpfuel.org_id',COALESCE(prior_org,''),true);RETURN NULL;
END $$;
CREATE CONSTRAINT TRIGGER org_sync_review_complete AFTER INSERT ON org_sync_review_items DEFERRABLE INITIALLY DEFERRED FOR EACH ROW EXECUTE FUNCTION org_sync_review_complete();
CREATE CONSTRAINT TRIGGER org_meter_held_complete AFTER INSERT ON org_meter_held_readings DEFERRABLE INITIALLY DEFERRED FOR EACH ROW EXECUTE FUNCTION org_sync_review_complete();

CREATE FUNCTION org_meter_held_source() RETURNS trigger LANGUAGE plpgsql SET search_path=pg_catalog AS $$
DECLARE o public.org_sync_operations%ROWTYPE;i public.org_sync_review_items%ROWTYPE;m public.org_meters%ROWTYPE;s public.org_meter_segments%ROWTYPE;actual_unit UUID;
BEGIN
 IF NEW.org_id IS DISTINCT FROM NULLIF(current_setting('anpfuel.org_id',true),'')::uuid THEN RAISE EXCEPTION USING ERRCODE='42501',MESSAGE='organization scope denied';END IF;
 SELECT * INTO i FROM public.org_sync_review_items WHERE org_id=NEW.org_id AND id=NEW.review_item_id AND operation_id=NEW.operation_id;
 IF NOT FOUND OR i.operation_type<>'meter.reading.record' OR i.operation_schema<>1 THEN RAISE EXCEPTION USING ERRCODE='23514',MESSAGE='held meter intent unavailable';END IF;
 SELECT * INTO o FROM public.org_sync_operations WHERE org_id=NEW.org_id AND operation_id=NEW.operation_id;
 IF NOT FOUND OR o.entity_type<>'meter_reading' OR o.entity_id<>NEW.reading_id OR NOT EXISTS(SELECT 1 FROM public.org_device_enrollments e JOIN public.org_memberships p ON(p.org_id,p.id)=(e.org_id,e.membership_id) WHERE e.org_id=o.org_id AND e.id=o.enrollment_id AND e.membership_id=o.membership_id AND e.state='ENROLLED' AND p.state='ACTIVE' AND p.account_id=o.owner_account_id AND p.employee_id=i.author_employee_id)
 THEN RAISE EXCEPTION USING ERRCODE='23514',MESSAGE='held meter source unavailable';END IF;
 PERFORM 1 FROM public.organizations WHERE org_id=NEW.org_id AND has_snapshot FOR UPDATE;
 -- The held fact reserves its reading identity; it cannot claim an applied one.
 IF EXISTS(SELECT 1 FROM public.org_meter_readings WHERE org_id=NEW.org_id AND(id=NEW.reading_id OR operation_id=NEW.operation_id)) THEN RAISE EXCEPTION USING ERRCODE='23505',CONSTRAINT='org_meter_readings_pkey',MESSAGE='meter reading identity unavailable';END IF;
 SELECT unit_id INTO actual_unit FROM public.org_assets WHERE org_id=NEW.org_id AND id=NEW.asset_id FOR UPDATE;
 IF NOT FOUND OR NEW.unit_id IS DISTINCT FROM actual_unit THEN RAISE EXCEPTION USING ERRCODE='23514',MESSAGE='held meter context unavailable';END IF;
 SELECT * INTO m FROM public.org_meters WHERE org_id=NEW.org_id AND id=NEW.meter_id AND asset_id=NEW.asset_id FOR UPDATE;
 IF NOT FOUND THEN RAISE EXCEPTION USING ERRCODE='23503',MESSAGE='private meter not found';END IF;
 SELECT * INTO s FROM public.org_meter_segments WHERE org_id=NEW.org_id AND id=NEW.segment_id AND meter_id=NEW.meter_id;
 IF NOT FOUND OR(NEW.occurred_at,NEW.occurred_submicro)<(m.installed_at,m.installed_submicro) OR(m.removed_at IS NOT NULL AND(NEW.occurred_at,NEW.occurred_submicro)>=(m.removed_at,m.removed_submicro))
 OR(NEW.occurred_at,NEW.occurred_submicro)<(s.opened_at,s.opened_submicro) OR(s.closed_at IS NOT NULL AND(NEW.occurred_at,NEW.occurred_submicro)>=(s.closed_at,s.closed_submicro))
 OR NEW.value>(CASE m.kind WHEN 'ODOMETER' THEN 1000000000000 ELSE 3600000000 END) OR(m.modulus IS NOT NULL AND NEW.value>=m.modulus)
 OR NOT EXISTS(SELECT 1 FROM pg_timezone_names WHERE name=NEW.entry_zone)
 THEN RAISE EXCEPTION USING ERRCODE='23514',MESSAGE='invalid held meter observation';END IF;
 RETURN NEW;
END $$;
CREATE TRIGGER org_meter_held_source BEFORE INSERT ON org_meter_held_readings FOR EACH ROW EXECUTE FUNCTION org_meter_held_source();

CREATE FUNCTION org_sync_review_employee_canonical(org UUID,employee UUID) RETURNS UUID LANGUAGE plpgsql VOLATILE SET search_path=pg_catalog AS $$
DECLARE current_id UUID:=employee;target UUID;seen UUID[]:='{}';
BEGIN
 FOR hop IN 0..64 LOOP
  IF current_id=ANY(seen) THEN RETURN NULL;END IF;seen=array_append(seen,current_id);
  SELECT merged_into INTO target FROM public.org_employee_identities WHERE org_id=org AND id=current_id;
  IF NOT FOUND THEN RETURN NULL;END IF;IF target IS NULL THEN RETURN current_id;END IF;current_id=target;
 END LOOP;RETURN NULL;
END $$;
CREATE FUNCTION org_sync_review_resolution_source() RETURNS trigger LANGUAGE plpgsql SET search_path=pg_catalog AS $$
DECLARE i public.org_sync_review_items%ROWTYPE;o public.org_sync_operations%ROWTYPE;h public.org_meter_held_readings%ROWTYPE;j JSONB;employee UUID;author UUID;received TIMESTAMPTZ;
BEGIN
 IF NEW.org_id IS DISTINCT FROM NULLIF(current_setting('anpfuel.org_id',true),'')::uuid THEN RAISE EXCEPTION USING ERRCODE='42501',MESSAGE='organization scope denied';END IF;
 PERFORM 1 FROM public.organizations WHERE org_id=NEW.org_id AND state='ACTIVE' AND has_snapshot FOR UPDATE;
 IF NOT FOUND THEN RAISE EXCEPTION USING ERRCODE='55000',MESSAGE='organization does not accept review decisions';END IF;
 SELECT * INTO i FROM public.org_sync_review_items WHERE org_id=NEW.org_id AND id=NEW.review_item_id FOR UPDATE;
 IF NOT FOUND THEN RAISE EXCEPTION USING ERRCODE='23503',MESSAGE='private review not found';END IF;
 SELECT * INTO o FROM public.org_sync_operations WHERE org_id=i.org_id AND operation_id=i.operation_id;
 SELECT * INTO h FROM public.org_meter_held_readings WHERE org_id=i.org_id AND review_item_id=i.id;
 IF NOT FOUND THEN RAISE EXCEPTION USING ERRCODE='23514',MESSAGE='actual held review source unavailable';END IF;
 SELECT public.org_sync_review_employee_canonical(p.org_id,p.employee_id) INTO employee FROM public.org_memberships p JOIN public.company_web_sessions s ON s.id=NEW.reviewer_session_id AND s.account_id=p.account_id
 WHERE p.org_id=NEW.org_id AND p.id=NEW.reviewer_membership_id AND p.account_id=NEW.reviewer_account_id AND p.state='ACTIVE';
 author=public.org_sync_review_employee_canonical(i.org_id,i.author_employee_id);
 IF employee IS NULL OR author IS NULL OR employee IS DISTINCT FROM NEW.reviewer_employee_id THEN RAISE EXCEPTION USING ERRCODE='23514',MESSAGE='reviewer source unavailable';END IF;
 IF NEW.reviewer_account_id=o.owner_account_id OR NEW.reviewer_membership_id=o.membership_id OR employee=author THEN RAISE EXCEPTION USING ERRCODE='P9006',MESSAGE='self review is not allowed';END IF;
 IF(NEW.outcome='ACCEPTED' AND NEW.reading_id<>h.reading_id) THEN RAISE EXCEPTION USING ERRCODE='23514',MESSAGE='review applied reading unavailable';END IF;
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
CREATE TRIGGER org_sync_review_resolution_source BEFORE INSERT ON org_sync_review_resolutions FOR EACH ROW EXECUTE FUNCTION org_sync_review_resolution_source();
CREATE FUNCTION org_sync_review_resolution_complete() RETURNS trigger LANGUAGE plpgsql SET search_path=pg_catalog AS $$
BEGIN
 IF NOT EXISTS(SELECT 1 FROM public.org_audit_events a WHERE a.org_id=NEW.org_id AND a.operation_id=NEW.id AND a.actor_id=NEW.reviewer_account_id AND a.action='offl.review.resolved' AND a.target_type='offl.review' AND a.target_id=NEW.review_item_id AND a.request_hash=NEW.request_hash)
 OR(NEW.outcome='ACCEPTED' AND NOT EXISTS(SELECT 1 FROM public.org_meter_readings r JOIN public.org_meter_held_readings h ON(h.org_id,h.review_item_id)=(r.org_id,NEW.review_item_id) JOIN public.org_sync_review_items i ON(i.org_id,i.id)=(h.org_id,h.review_item_id) JOIN public.org_sync_operations o ON(o.org_id,o.operation_id)=(i.org_id,i.operation_id)
 JOIN public.org_sync_acknowledgements a ON(a.org_id,a.operation_id)=(o.org_id,o.operation_id)
 WHERE r.org_id=NEW.org_id AND r.id=NEW.reading_id AND r.review_resolution_id=NEW.id AND r.operation_id=o.operation_id AND r.meter_id=h.meter_id AND r.segment_id=h.segment_id AND r.value=h.value
 AND(r.occurred_at,r.occurred_submicro)=(h.occurred_at,h.occurred_submicro) AND r.author_membership_id=o.membership_id AND r.capture_kind='ENROLLED_DEVICE' AND r.enrollment_id=o.enrollment_id AND r.received_at=a.received_at))
 OR(NEW.outcome='ACCEPTED' AND NOT EXISTS(SELECT 1 FROM public.org_sync_effects e JOIN public.org_sync_review_items i ON(i.org_id,i.operation_id)=(e.org_id,e.operation_id) WHERE i.org_id=NEW.org_id AND i.id=NEW.review_item_id AND e.entity_revision=NEW.entity_revision))
 THEN RAISE EXCEPTION USING ERRCODE='23514',MESSAGE='review resolution publication unavailable';END IF;
 RETURN NULL;
END $$;
CREATE CONSTRAINT TRIGGER org_sync_review_resolution_complete AFTER INSERT ON org_sync_review_resolutions DEFERRABLE INITIALLY DEFERRED FOR EACH ROW EXECUTE FUNCTION org_sync_review_resolution_complete();

DO $$DECLARE name TEXT;BEGIN
 FOREACH name IN ARRAY ARRAY['org_sync_review_items','org_meter_held_readings','org_sync_review_resolutions'] LOOP
 EXECUTE format('ALTER TABLE %I ENABLE ROW LEVEL SECURITY',name);EXECUTE format('ALTER TABLE %I FORCE ROW LEVEL SECURITY',name);
 EXECUTE format('CREATE POLICY %I ON %I USING(org_id=NULLIF(current_setting(''anpfuel.org_id'',true),'''')::uuid) WITH CHECK(org_id=NULLIF(current_setting(''anpfuel.org_id'',true),'''')::uuid)',name||'_scope',name);
 EXECUTE format('CREATE TRIGGER %I BEFORE UPDATE OR DELETE OR TRUNCATE ON %I FOR EACH STATEMENT EXECUTE FUNCTION org_sync_keep_history()',name||'_immutable',name);
 END LOOP;
END $$;
REVOKE ALL ON org_sync_review_items,org_meter_held_readings,org_sync_review_resolutions FROM PUBLIC;

-- Preserve the ordinary live-source branch; the additive reviewed branch can
-- only consume the exact source fact and an accepted resolution in this TX.
CREATE OR REPLACE FUNCTION org_meter_observation_guard() RETURNS trigger LANGUAGE plpgsql SET search_path=pg_catalog AS $$
DECLARE m public.org_meters%ROWTYPE;s public.org_meter_segments%ROWTYPE;original_received TIMESTAMPTZ;
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
  SELECT a.received_at INTO original_received FROM public.org_sync_review_resolutions z JOIN public.org_sync_review_items i ON(i.org_id,i.id)=(z.org_id,z.review_item_id)
  JOIN public.org_meter_held_readings h ON(h.org_id,h.review_item_id)=(i.org_id,i.id) JOIN public.org_sync_operations o ON(o.org_id,o.operation_id)=(i.org_id,i.operation_id)
  JOIN public.org_sync_acknowledgements a ON(a.org_id,a.operation_id,a.review_item_id,a.review_proof_version)=(i.org_id,i.operation_id,i.id,i.proof_version)
  WHERE z.org_id=NEW.org_id AND z.id=NEW.review_resolution_id AND z.outcome='ACCEPTED' AND z.reading_id=NEW.id AND NEW.id=h.reading_id
  AND NEW.meter_id=h.meter_id AND NEW.segment_id=h.segment_id AND NEW.value=h.value AND(NEW.occurred_at,NEW.occurred_submicro)=(h.occurred_at,h.occurred_submicro)
  AND NEW.entry_zone=h.entry_zone AND NEW.device_offset_seconds=h.device_offset_seconds AND NEW.operation_id=o.operation_id AND NEW.request_hash=h.request_hash
  AND NEW.author_membership_id=o.membership_id AND NEW.capture_kind='ENROLLED_DEVICE' AND NEW.enrollment_id=o.enrollment_id AND NEW.web_session_id IS NULL AND NEW.origin=h.origin AND NEW.method=h.method AND NEW.state='VALID';
  IF NOT FOUND THEN RAISE EXCEPTION USING ERRCODE='23514',MESSAGE='reviewed observation source unavailable';END IF;
  NEW.received_at=original_received;RETURN NEW;
 END IF;
 IF EXISTS(SELECT 1 FROM public.org_meter_held_readings WHERE org_id=NEW.org_id AND(reading_id=NEW.id OR operation_id=NEW.operation_id)) THEN RAISE EXCEPTION USING ERRCODE='23505',CONSTRAINT='org_meter_readings_pkey',MESSAGE='meter reading identity unavailable';END IF;
 IF NEW.capture_kind='ENROLLED_DEVICE' THEN
  IF NOT EXISTS(SELECT 1 FROM public.org_device_enrollments WHERE org_id=NEW.org_id AND id=NEW.enrollment_id AND membership_id=NEW.author_membership_id AND state='ENROLLED') THEN RAISE EXCEPTION USING ERRCODE='23503',MESSAGE='observation source not found';END IF;
 ELSE
  IF NOT EXISTS(SELECT 1 FROM public.company_web_sessions c JOIN public.org_memberships p ON p.account_id=c.account_id WHERE p.org_id=NEW.org_id AND p.id=NEW.author_membership_id AND c.id=NEW.web_session_id) THEN RAISE EXCEPTION USING ERRCODE='23503',MESSAGE='observation source not found';END IF;
 END IF;
 NEW.received_at=clock_timestamp();RETURN NEW;
END $$;
CREATE OR REPLACE FUNCTION org_sync_effect_source() RETURNS trigger LANGUAGE plpgsql SET search_path=pg_catalog AS $$
BEGIN
 IF NOT EXISTS(SELECT 1 FROM public.org_sync_acknowledgements a WHERE a.org_id=NEW.org_id AND a.operation_id=NEW.operation_id AND a.result='ACCEPTED' AND a.entity_revision=NEW.entity_revision)
 AND NOT EXISTS(SELECT 1 FROM public.org_sync_review_items i JOIN public.org_sync_review_resolutions z ON(z.org_id,z.review_item_id)=(i.org_id,i.id)
 JOIN public.org_meter_held_readings h ON(h.org_id,h.review_item_id)=(i.org_id,i.id) JOIN public.org_meter_readings r ON(r.org_id,r.id,r.review_resolution_id)=(z.org_id,z.reading_id,z.id)
 JOIN public.org_sync_acknowledgements a ON(a.org_id,a.operation_id,a.review_item_id,a.review_proof_version)=(i.org_id,i.operation_id,i.id,i.proof_version)
 WHERE i.org_id=NEW.org_id AND i.operation_id=NEW.operation_id AND z.outcome='ACCEPTED' AND z.entity_revision=NEW.entity_revision AND r.operation_id=i.operation_id AND r.id=h.reading_id)
 THEN RAISE EXCEPTION USING ERRCODE='23514',MESSAGE='intake applied effect source unavailable';END IF;
 RETURN NEW;
END $$;

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
 OR(c.conrelid='public.org_meter_held_readings'::regclass AND c.conname='org_meter_held_readings_asset_fkey'))) THEN RAISE EXCEPTION USING ERRCODE='P7904',MESSAGE='asset usage source unavailable';END IF;
 RETURN jsonb_build_object('readings',(SELECT count(*) FROM public.org_meter_readings r JOIN public.org_meters m ON(m.org_id,m.id)=(r.org_id,r.meter_id) WHERE m.org_id=selected_org AND m.asset_id=selected_asset),'operations',(SELECT count(*) FROM public.org_sync_operations o JOIN public.org_sync_effects e USING(org_id,operation_id) WHERE o.org_id=selected_org AND o.entity_id=selected_asset AND o.entity_type='fleet_asset')+(SELECT count(*) FROM public.org_meter_held_readings h WHERE h.org_id=selected_org AND h.asset_id=selected_asset));
END $$;

-- A held fact's unit snapshot is retained history, never current unit usage.
-- Preserve 197's employee UNION/DISTINCT and asset counts exactly.
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
 OR(c.conrelid='public.org_meter_held_readings'::regclass AND c.conname='org_meter_held_readings_unit_fkey'))) THEN RAISE EXCEPTION USING ERRCODE='P7704',MESSAGE='unit usage source unavailable';END IF;
 SELECT count(DISTINCT employee_id) INTO employee_count FROM(
 SELECT e.id AS employee_id FROM public.org_employee_profiles p JOIN public.org_employee_identities e ON(e.org_id,e.id)=(p.org_id,p.employee_id)
 WHERE p.org_id=selected_org AND p.home_unit_id=selected_unit AND e.state='ACTIVE'
 UNION
 SELECT e.id AS employee_id FROM public.org_team_links l JOIN public.org_teams t ON(t.org_id,t.id)=(l.org_id,l.team_id)
 JOIN public.org_employee_identities e ON(e.org_id,e.id)=(l.org_id,l.employee_id)
 WHERE l.org_id=selected_org AND t.unit_id=selected_unit AND l.ended_at IS NULL AND e.state='ACTIVE') current_employees;
 SELECT count(*) INTO asset_count FROM public.org_asset_unit_allocations p JOIN public.org_assets a ON(a.org_id,a.id)=(p.org_id,p.asset_id)
 WHERE p.org_id=selected_org AND p.unit_id=selected_unit AND p.ended_at IS NULL AND a.state='ACTIVE';
 RETURN jsonb_build_object('assets',asset_count,'employees',employee_count,'budgets',0,'operations',0);
END $$;

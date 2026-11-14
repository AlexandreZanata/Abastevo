-- EMP-OFFL-002. No payload, credentials, runtime grants or activated producer.
CREATE TABLE org_sync_operations(
 org_id UUID NOT NULL REFERENCES organizations(org_id),
 operation_id UUID NOT NULL CHECK(operation_id<>'00000000-0000-0000-0000-000000000000'::uuid),
 owner_account_id UUID NOT NULL CHECK(owner_account_id<>'00000000-0000-0000-0000-000000000000'::uuid),
 api_origin TEXT NOT NULL CHECK(octet_length(api_origin) BETWEEN 1 AND 8192),
 membership_id UUID NOT NULL,enrollment_id UUID NOT NULL,
 operation_type TEXT NOT NULL CHECK(operation_type~'^[a-z][a-z0-9]*(_[a-z0-9]+)*(\.[a-z][a-z0-9]*(_[a-z0-9]+)*)+$'),
 operation_schema BIGINT NOT NULL CHECK(operation_schema>0),
 entity_type TEXT NOT NULL CHECK(entity_type~'^[a-z][a-z0-9]*(_[a-z0-9]+)*$'),
 entity_id UUID NOT NULL CHECK(entity_id<>'00000000-0000-0000-0000-000000000000'::uuid),
 payload_digest TEXT NOT NULL CHECK(payload_digest~'^sha256:[a-f0-9]{64}$'),
 claimed_at TIMESTAMPTZ NOT NULL DEFAULT clock_timestamp() CHECK(isfinite(claimed_at)),
 PRIMARY KEY(org_id,operation_id),
 FOREIGN KEY(org_id,membership_id) REFERENCES org_memberships(org_id,id),
 FOREIGN KEY(org_id,enrollment_id) REFERENCES org_device_enrollments(org_id,id)
);
CREATE TABLE org_sync_acknowledgements(
 org_id UUID NOT NULL,operation_id UUID NOT NULL,
 result TEXT NOT NULL CHECK(result IN('ACCEPTED','ACCEPTED_FOR_REVIEW','REJECTED')),
 code TEXT NOT NULL DEFAULT '',entity_revision BIGINT CHECK(entity_revision>0),review_item_id UUID,
 received_at TIMESTAMPTZ NOT NULL CHECK(isfinite(received_at) AND received_at>'0001-01-01 00:00:00+00'::timestamptz AND received_at<'10000-01-01 00:00:00+00'::timestamptz),
 warnings TEXT[] NOT NULL DEFAULT '{}' CHECK(cardinality(warnings)<=1 AND warnings<@ARRAY['CLOCK_SKEW']::text[] AND array_position(warnings,NULL) IS NULL),
 canonical_ack BYTEA NOT NULL CHECK(octet_length(canonical_ack) BETWEEN 1 AND 8192),
 audit_hash TEXT NOT NULL CHECK(audit_hash~'^[a-f0-9]{64}$'),
 PRIMARY KEY(org_id,operation_id),
 FOREIGN KEY(org_id,operation_id) REFERENCES org_sync_operations(org_id,operation_id),
 FOREIGN KEY(org_id,operation_id) REFERENCES org_audit_events(org_id,operation_id) DEFERRABLE INITIALLY DEFERRED,
 CHECK((result='ACCEPTED' AND code='' AND entity_revision IS NOT NULL AND review_item_id IS NULL)
  OR(result='ACCEPTED_FOR_REVIEW' AND code IN('ORPHAN_REFERENCE','ACCESS_ENDED_BEFORE_RECEIPT','PERMISSION_CHANGED','ASSET_UNAVAILABLE','ASSET_BUSY','TRIP_ALREADY_CLOSED','PERIOD_LOCKED','STALE_REVISION','ORG_SUSPENDED_GRACE','PLAN_CHANGED') AND review_item_id IS NOT NULL AND review_item_id<>'00000000-0000-0000-0000-000000000000'::uuid)
  OR(result='REJECTED' AND code IN('OPERATION_ID_REUSED','MALFORMED','DEVICE_REVOKED','MEMBERSHIP_ENDED','INTAKE_WINDOW_CLOSED','FORBIDDEN','ORG_SUSPENDED') AND entity_revision IS NULL AND review_item_id IS NULL))
);
CREATE TABLE org_sync_effects(
 org_id UUID NOT NULL,operation_id UUID NOT NULL,entity_revision BIGINT NOT NULL CHECK(entity_revision>0),
 applied_at TIMESTAMPTZ NOT NULL DEFAULT clock_timestamp() CHECK(isfinite(applied_at)),
 PRIMARY KEY(org_id,operation_id),FOREIGN KEY(org_id,operation_id) REFERENCES org_sync_acknowledgements(org_id,operation_id)
);
CREATE FUNCTION org_sync_keep_history() RETURNS trigger LANGUAGE plpgsql SET search_path=pg_catalog AS $$
BEGIN RAISE EXCEPTION USING ERRCODE='55000',MESSAGE='organization intake history is append-only';END $$;
CREATE TRIGGER org_sync_operations_immutable BEFORE UPDATE OR DELETE OR TRUNCATE ON org_sync_operations FOR EACH STATEMENT EXECUTE FUNCTION org_sync_keep_history();
CREATE TRIGGER org_sync_acknowledgements_immutable BEFORE UPDATE OR DELETE OR TRUNCATE ON org_sync_acknowledgements FOR EACH STATEMENT EXECUTE FUNCTION org_sync_keep_history();
CREATE TRIGGER org_sync_effects_immutable BEFORE UPDATE OR DELETE OR TRUNCATE ON org_sync_effects FOR EACH STATEMENT EXECUTE FUNCTION org_sync_keep_history();
ALTER TABLE org_sync_operations ENABLE ROW LEVEL SECURITY;ALTER TABLE org_sync_operations FORCE ROW LEVEL SECURITY;
ALTER TABLE org_sync_acknowledgements ENABLE ROW LEVEL SECURITY;ALTER TABLE org_sync_acknowledgements FORCE ROW LEVEL SECURITY;
ALTER TABLE org_sync_effects ENABLE ROW LEVEL SECURITY;ALTER TABLE org_sync_effects FORCE ROW LEVEL SECURITY;
CREATE POLICY org_sync_operations_scope ON org_sync_operations USING(org_id=NULLIF(current_setting('anpfuel.org_id',true),'')::uuid) WITH CHECK(org_id=NULLIF(current_setting('anpfuel.org_id',true),'')::uuid);
CREATE POLICY org_sync_acknowledgements_scope ON org_sync_acknowledgements USING(org_id=NULLIF(current_setting('anpfuel.org_id',true),'')::uuid) WITH CHECK(org_id=NULLIF(current_setting('anpfuel.org_id',true),'')::uuid);
CREATE POLICY org_sync_effects_scope ON org_sync_effects USING(org_id=NULLIF(current_setting('anpfuel.org_id',true),'')::uuid) WITH CHECK(org_id=NULLIF(current_setting('anpfuel.org_id',true),'')::uuid);
REVOKE ALL ON org_sync_operations,org_sync_acknowledgements,org_sync_effects FROM PUBLIC;
CREATE FUNCTION org_sync_operation_source() RETURNS trigger LANGUAGE plpgsql SET search_path=pg_catalog AS $$
BEGIN
 IF NEW.owner_account_id='00000000-0000-0000-0000-000000000000'::uuid OR NOT EXISTS(
 SELECT 1 FROM public.org_device_enrollments e JOIN public.org_device_commands c ON(c.org_id,c.operation_id)=(e.org_id,e.created_operation_id)
 WHERE e.org_id=NEW.org_id AND e.id=NEW.enrollment_id AND e.membership_id=NEW.membership_id AND c.kind='ENROLL' AND c.device_id=e.id AND c.actor_id=NEW.owner_account_id)
 THEN RAISE EXCEPTION USING ERRCODE='23514',MESSAGE='intake source references unavailable';END IF;
 RETURN NEW;
END $$;
CREATE TRIGGER org_sync_operation_source BEFORE INSERT ON org_sync_operations FOR EACH ROW EXECUTE FUNCTION org_sync_operation_source();
CREATE FUNCTION org_sync_ack_source() RETURNS trigger LANGUAGE plpgsql SET search_path=pg_catalog AS $$
DECLARE j JSONB;c public.org_sync_operations%ROWTYPE;action_name TEXT;
BEGIN
 SELECT * INTO c FROM public.org_sync_operations WHERE org_id=NEW.org_id AND operation_id=NEW.operation_id;
 IF NOT FOUND THEN RAISE EXCEPTION USING ERRCODE='23514',MESSAGE='intake acknowledgement source unavailable';END IF;
 j=convert_from(NEW.canonical_ack,'UTF8')::jsonb;
 action_name=CASE NEW.result WHEN 'ACCEPTED' THEN 'offl.operation.accepted' WHEN 'ACCEPTED_FOR_REVIEW' THEN 'offl.operation.review' ELSE 'offl.operation.refused' END;
 IF jsonb_typeof(j)<>'object' OR NOT(j ?& ARRAY['schema_version','operation_id','payload_digest','result','code','entity_revision','received_at','review_item_id','warnings'])
 OR j-ARRAY['schema_version','operation_id','payload_digest','result','code','entity_revision','received_at','review_item_id','warnings']<>'{}'::jsonb
 OR j->'schema_version' IS DISTINCT FROM '1'::jsonb OR j->>'operation_id' IS DISTINCT FROM NEW.operation_id::text
 OR j->>'payload_digest' IS DISTINCT FROM c.payload_digest OR j->>'result' IS DISTINCT FROM NEW.result
 OR j->'code' IS DISTINCT FROM COALESCE(to_jsonb(NULLIF(NEW.code,'')),'null'::jsonb)
 OR j->'entity_revision' IS DISTINCT FROM COALESCE(to_jsonb(NEW.entity_revision),'null'::jsonb)
 OR j->'review_item_id' IS DISTINCT FROM COALESCE(to_jsonb(NEW.review_item_id::text),'null'::jsonb)
 OR j->'warnings' IS DISTINCT FROM to_jsonb(NEW.warnings) OR(j->>'received_at')::timestamptz IS DISTINCT FROM NEW.received_at
 OR NOT EXISTS(SELECT 1 FROM public.org_audit_events a WHERE a.org_id=NEW.org_id AND a.operation_id=NEW.operation_id AND a.actor_id=c.owner_account_id
  AND a.action=action_name AND a.target_type='offl.operation' AND a.target_id=NEW.operation_id AND a.request_hash=NEW.audit_hash)
 THEN RAISE EXCEPTION USING ERRCODE='23514',MESSAGE='intake acknowledgement source unavailable';END IF;
 RETURN NEW;
END $$;
CREATE TRIGGER org_sync_ack_source BEFORE INSERT ON org_sync_acknowledgements FOR EACH ROW EXECUTE FUNCTION org_sync_ack_source();
CREATE FUNCTION org_sync_effect_source() RETURNS trigger LANGUAGE plpgsql SET search_path=pg_catalog AS $$
BEGIN
 IF NOT EXISTS(SELECT 1 FROM public.org_sync_acknowledgements a WHERE a.org_id=NEW.org_id AND a.operation_id=NEW.operation_id AND a.result='ACCEPTED' AND a.entity_revision=NEW.entity_revision)
 THEN RAISE EXCEPTION USING ERRCODE='23514',MESSAGE='intake applied effect source unavailable';END IF;
 RETURN NEW;
END $$;
CREATE TRIGGER org_sync_effect_source BEFORE INSERT ON org_sync_effects FOR EACH ROW EXECUTE FUNCTION org_sync_effect_source();

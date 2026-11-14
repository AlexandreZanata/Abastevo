-- EMP-METER-002. Exact source-bound insert-only observation history.
CREATE TABLE org_meters(
 org_id UUID NOT NULL REFERENCES organizations(org_id),id UUID NOT NULL CHECK(id<>'00000000-0000-0000-0000-000000000000'::uuid),asset_id UUID NOT NULL,
 kind TEXT NOT NULL CHECK(kind IN('ODOMETER','HOUR_METER')),label TEXT NOT NULL CHECK(org_asset_text_valid(label,0,true)),modulus BIGINT CHECK(modulus>0),
 installed_at TIMESTAMPTZ NOT NULL CHECK(isfinite(installed_at) AND installed_at>='0001-01-01 00:00:00+00'::timestamptz AND installed_at<'10000-01-01 00:00:00+00'::timestamptz),
 installed_submicro SMALLINT NOT NULL DEFAULT 0 CHECK(installed_submicro BETWEEN 0 AND 999),
 removed_at TIMESTAMPTZ,removed_submicro SMALLINT NOT NULL DEFAULT 0 CHECK(removed_submicro BETWEEN 0 AND 999 AND(removed_at IS NOT NULL OR removed_submicro=0)),created_operation_id UUID NOT NULL,
 PRIMARY KEY(org_id,id),UNIQUE(org_id,asset_id,id),CONSTRAINT org_meters_asset_fkey FOREIGN KEY(org_id,asset_id) REFERENCES org_assets(org_id,id),
 FOREIGN KEY(org_id,created_operation_id) REFERENCES org_audit_events(org_id,operation_id) DEFERRABLE INITIALLY DEFERRED,
 CHECK(removed_at IS NULL OR(isfinite(removed_at) AND(removed_at,removed_submicro)>=(installed_at,installed_submicro)))
);
CREATE UNIQUE INDEX org_meters_one_in_service_kind ON org_meters(org_id,asset_id,kind) WHERE removed_at IS NULL;
CREATE TABLE org_meter_segments(
 org_id UUID NOT NULL,id UUID NOT NULL CHECK(id<>'00000000-0000-0000-0000-000000000000'::uuid),meter_id UUID NOT NULL,
 kind TEXT NOT NULL CHECK(kind='INSTALLATION'),opened_at TIMESTAMPTZ NOT NULL,opened_submicro SMALLINT NOT NULL DEFAULT 0 CHECK(opened_submicro BETWEEN 0 AND 999),closed_at TIMESTAMPTZ,closed_submicro SMALLINT NOT NULL DEFAULT 0 CHECK(closed_submicro BETWEEN 0 AND 999 AND(closed_at IS NOT NULL OR closed_submicro=0)),
 opening_reading_id UUID NOT NULL,PRIMARY KEY(org_id,id),UNIQUE(org_id,meter_id,id),
 FOREIGN KEY(org_id,meter_id) REFERENCES org_meters(org_id,id),
 CHECK(isfinite(opened_at) AND opened_at>='0001-01-01 00:00:00+00'::timestamptz AND opened_at<'10000-01-01 00:00:00+00'::timestamptz),
 CHECK(closed_at IS NULL OR(isfinite(closed_at) AND(closed_at,closed_submicro)>=(opened_at,opened_submicro)))
);
CREATE UNIQUE INDEX org_meter_one_open_segment ON org_meter_segments(org_id,meter_id) WHERE closed_at IS NULL;
CREATE TABLE org_meter_readings(
 org_id UUID NOT NULL,id UUID NOT NULL CHECK(id<>'00000000-0000-0000-0000-000000000000'::uuid),meter_id UUID NOT NULL,segment_id UUID NOT NULL,
 value BIGINT NOT NULL CHECK(value>=0),occurred_at TIMESTAMPTZ NOT NULL,occurred_submicro SMALLINT NOT NULL DEFAULT 0 CHECK(occurred_submicro BETWEEN 0 AND 999),received_at TIMESTAMPTZ NOT NULL DEFAULT clock_timestamp(),
 entry_zone TEXT NOT NULL,device_offset_seconds BIGINT NOT NULL CHECK(device_offset_seconds BETWEEN -64800 AND 64800),
 author_membership_id UUID NOT NULL,capture_kind TEXT NOT NULL CHECK(capture_kind IN('ENROLLED_DEVICE','WEB_SESSION')),
 enrollment_id UUID,web_session_id UUID,
 operation_id UUID NOT NULL CHECK(operation_id<>'00000000-0000-0000-0000-000000000000'::uuid),request_hash TEXT NOT NULL CHECK(request_hash~'^[a-f0-9]{64}$'),
 origin TEXT NOT NULL CHECK(origin IN('TRIP_START','TRIP_END','HANDOVER','FUELING','CHECKLIST','MAINTENANCE','STANDALONE','IMPORT','METER_INSTALLATION','METER_REMOVAL')),
 method TEXT NOT NULL CHECK(method IN('TYPED','TYPED_WITH_PHOTO','IMPORTED','INTEGRATION')),state TEXT NOT NULL CHECK(state IN('VALID','SUSPECT')),
 PRIMARY KEY(org_id,id),UNIQUE(org_id,meter_id,id),UNIQUE(org_id,meter_id,operation_id),
 FOREIGN KEY(org_id,meter_id,segment_id) REFERENCES org_meter_segments(org_id,meter_id,id),
 FOREIGN KEY(org_id,author_membership_id) REFERENCES org_memberships(org_id,id),
 FOREIGN KEY(org_id,enrollment_id) REFERENCES org_device_enrollments(org_id,id),
 CHECK((capture_kind='ENROLLED_DEVICE' AND enrollment_id IS NOT NULL AND enrollment_id<>'00000000-0000-0000-0000-000000000000'::uuid AND web_session_id IS NULL)
 OR(capture_kind='WEB_SESSION' AND enrollment_id IS NULL AND web_session_id IS NOT NULL AND web_session_id<>'00000000-0000-0000-0000-000000000000'::uuid)),
 CHECK(isfinite(occurred_at) AND occurred_at>='0001-01-01 00:00:00+00'::timestamptz AND occurred_at<'10000-01-01 00:00:00+00'::timestamptz),
 CHECK(isfinite(received_at) AND received_at>'0001-01-01 00:00:00+00'::timestamptz AND received_at<'10000-01-01 00:00:00+00'::timestamptz)
);
-- A web locator is immutable provenance, never a live FK to expiring personal
-- sessions/accounts. Its real ownership is checked at insertion, not invented.
ALTER TABLE org_meter_segments ADD CONSTRAINT org_meter_opening_reading_fkey FOREIGN KEY(org_id,meter_id,opening_reading_id) REFERENCES org_meter_readings(org_id,meter_id,id) DEFERRABLE INITIALLY DEFERRED;
CREATE INDEX org_meter_latest_valid ON org_meter_readings(org_id,meter_id,segment_id,occurred_at DESC,occurred_submicro DESC,received_at DESC,id DESC) WHERE state='VALID';
CREATE TABLE org_meter_commands(
 org_id UUID NOT NULL REFERENCES organizations(org_id),operation_id UUID NOT NULL,actor_id UUID NOT NULL,
 asset_id UUID NOT NULL,meter_id UUID NOT NULL,segment_id UUID NOT NULL,opening_reading_id UUID NOT NULL,
 request_hash TEXT NOT NULL CHECK(request_hash~'^[a-f0-9]{64}$'),asset_revision BIGINT NOT NULL CHECK(asset_revision>1),
 created_at TIMESTAMPTZ NOT NULL DEFAULT clock_timestamp(),PRIMARY KEY(org_id,operation_id),
 FOREIGN KEY(org_id,meter_id,segment_id) REFERENCES org_meter_segments(org_id,meter_id,id),
 FOREIGN KEY(org_id,meter_id,opening_reading_id) REFERENCES org_meter_readings(org_id,meter_id,id),
 FOREIGN KEY(org_id,operation_id) REFERENCES org_audit_events(org_id,operation_id) DEFERRABLE INITIALLY DEFERRED,
 FOREIGN KEY(org_id,asset_id,asset_revision) REFERENCES org_asset_revisions(org_id,asset_id,revision) DEFERRABLE INITIALLY DEFERRED
);
CREATE FUNCTION org_meter_parent_guard() RETURNS trigger LANGUAGE plpgsql SET search_path=pg_catalog AS $$
DECLARE root_state TEXT;
BEGIN
 IF NEW.org_id IS DISTINCT FROM NULLIF(current_setting('anpfuel.org_id',true),'')::uuid THEN RAISE EXCEPTION USING ERRCODE='42501',MESSAGE='organization scope denied';END IF;
 SELECT state INTO root_state FROM public.organizations WHERE org_id=NEW.org_id AND has_snapshot FOR UPDATE;
 IF NOT FOUND OR root_state NOT IN('PENDING_SETUP','ACTIVE') THEN RAISE EXCEPTION USING ERRCODE='55000',MESSAGE='organization does not accept meter installation';END IF;
 PERFORM 1 FROM public.org_assets WHERE org_id=NEW.org_id AND id=NEW.asset_id AND state IN('DRAFT','ACTIVE') FOR UPDATE;
 IF NOT FOUND THEN RAISE EXCEPTION USING ERRCODE='23503',MESSAGE='private reference not found';END IF;
 RETURN NEW;
END $$;
CREATE TRIGGER org_meter_parent_source BEFORE INSERT ON org_meters FOR EACH ROW EXECUTE FUNCTION org_meter_parent_guard();
CREATE FUNCTION org_meter_observation_guard() RETURNS trigger LANGUAGE plpgsql SET search_path=pg_catalog AS $$
DECLARE m public.org_meters%ROWTYPE;s public.org_meter_segments%ROWTYPE;
BEGIN
 IF NEW.org_id IS DISTINCT FROM NULLIF(current_setting('anpfuel.org_id',true),'')::uuid THEN RAISE EXCEPTION USING ERRCODE='42501',MESSAGE='organization scope denied';END IF;
 PERFORM 1 FROM public.organizations WHERE org_id=NEW.org_id AND has_snapshot FOR UPDATE;
 SELECT * INTO m FROM public.org_meters WHERE org_id=NEW.org_id AND id=NEW.meter_id FOR UPDATE;
 IF NOT FOUND THEN RAISE EXCEPTION USING ERRCODE='23503',MESSAGE='private meter not found';END IF;
 SELECT * INTO s FROM public.org_meter_segments WHERE org_id=NEW.org_id AND meter_id=NEW.meter_id AND id=NEW.segment_id;
 IF NOT FOUND OR (NEW.occurred_at,NEW.occurred_submicro)<(m.installed_at,m.installed_submicro) OR(m.removed_at IS NOT NULL AND (NEW.occurred_at,NEW.occurred_submicro)>=(m.removed_at,m.removed_submicro))
 OR (NEW.occurred_at,NEW.occurred_submicro)<(s.opened_at,s.opened_submicro) OR(s.closed_at IS NOT NULL AND (NEW.occurred_at,NEW.occurred_submicro)>=(s.closed_at,s.closed_submicro))
 THEN RAISE EXCEPTION USING ERRCODE='23514',MESSAGE='observation outside meter service interval';END IF;
 IF NEW.value>(CASE m.kind WHEN 'ODOMETER' THEN 1000000000000 ELSE 3600000000 END) OR(m.modulus IS NOT NULL AND NEW.value>=m.modulus)
 OR NOT EXISTS(SELECT 1 FROM pg_timezone_names WHERE name=NEW.entry_zone)
 THEN RAISE EXCEPTION USING ERRCODE='23514',MESSAGE='invalid meter observation';END IF;
 IF NEW.capture_kind='ENROLLED_DEVICE' THEN
  IF NOT EXISTS(SELECT 1 FROM public.org_device_enrollments WHERE org_id=NEW.org_id AND id=NEW.enrollment_id AND membership_id=NEW.author_membership_id AND state='ENROLLED') THEN RAISE EXCEPTION USING ERRCODE='23503',MESSAGE='observation source not found';END IF;
 ELSE
  IF NOT EXISTS(SELECT 1 FROM public.company_web_sessions c JOIN public.org_memberships p ON p.account_id=c.account_id WHERE p.org_id=NEW.org_id AND p.id=NEW.author_membership_id AND c.id=NEW.web_session_id) THEN RAISE EXCEPTION USING ERRCODE='23503',MESSAGE='observation source not found';END IF;
 END IF;
 NEW.received_at=clock_timestamp();RETURN NEW;
END $$;
CREATE TRIGGER org_meter_observation_source BEFORE INSERT ON org_meter_readings FOR EACH ROW EXECUTE FUNCTION org_meter_observation_guard();
CREATE FUNCTION org_meter_installation_complete() RETURNS trigger LANGUAGE plpgsql SET search_path=pg_catalog AS $$
DECLARE selected_org UUID;selected_meter UUID;
BEGIN
 selected_org=NEW.org_id;IF TG_TABLE_NAME='org_meters' THEN selected_meter=NEW.id;ELSE selected_meter=NEW.meter_id;END IF;
 IF NOT EXISTS(SELECT 1 FROM public.org_meters m JOIN public.org_meter_segments s ON(s.org_id,s.meter_id)=(m.org_id,m.id)
 JOIN public.org_meter_readings r ON(r.org_id,r.meter_id,r.segment_id,r.id)=(s.org_id,s.meter_id,s.id,s.opening_reading_id)
 WHERE m.org_id=selected_org AND m.id=selected_meter AND s.kind='INSTALLATION' AND (s.opened_at,s.opened_submicro)=(m.installed_at,m.installed_submicro)
 AND (r.occurred_at,r.occurred_submicro)=(s.opened_at,s.opened_submicro) AND r.origin='METER_INSTALLATION' AND r.operation_id=m.created_operation_id)
 THEN RAISE EXCEPTION USING ERRCODE='23514',MESSAGE='observed meter opening unavailable';END IF;
 RETURN NULL;
END $$;
CREATE CONSTRAINT TRIGGER org_meter_opening_complete AFTER INSERT ON org_meters DEFERRABLE INITIALLY DEFERRED FOR EACH ROW EXECUTE FUNCTION org_meter_installation_complete();
CREATE CONSTRAINT TRIGGER org_meter_segment_complete AFTER INSERT ON org_meter_segments DEFERRABLE INITIALLY DEFERRED FOR EACH ROW EXECUTE FUNCTION org_meter_installation_complete();
DO $$DECLARE name TEXT;BEGIN
 FOREACH name IN ARRAY ARRAY['org_meters','org_meter_segments','org_meter_readings','org_meter_commands'] LOOP
 EXECUTE format('ALTER TABLE %I ENABLE ROW LEVEL SECURITY',name);EXECUTE format('ALTER TABLE %I FORCE ROW LEVEL SECURITY',name);
 EXECUTE format('CREATE POLICY %I ON %I USING(org_id=NULLIF(current_setting(''anpfuel.org_id'',true),'''')::uuid) WITH CHECK(org_id=NULLIF(current_setting(''anpfuel.org_id'',true),'''')::uuid)',name||'_scope',name);
 EXECUTE format('CREATE TRIGGER %I BEFORE UPDATE OR DELETE OR TRUNCATE ON %I FOR EACH STATEMENT EXECUTE FUNCTION org_asset_keep_history()',name||'_immutable',name);
 EXECUTE format('REVOKE ALL ON %I FROM PUBLIC',name);
 END LOOP;
END $$;

-- Integrate actual meter consumers; preserve all existing unknown-FK guards.
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
 OR(c.conrelid='public.org_meters'::regclass AND c.conname='org_meters_asset_fkey'))) THEN RAISE EXCEPTION USING ERRCODE='P7904',MESSAGE='asset usage source unavailable';END IF;
 RETURN jsonb_build_object('readings',(SELECT count(*) FROM public.org_meter_readings r JOIN public.org_meters m ON(m.org_id,m.id)=(r.org_id,r.meter_id) WHERE m.org_id=selected_org AND m.asset_id=selected_asset),'operations',(SELECT count(*) FROM public.org_sync_operations o JOIN public.org_sync_effects e USING(org_id,operation_id) WHERE o.org_id=selected_org AND o.entity_id=selected_asset AND o.entity_type='fleet_asset'));
END $$;
CREATE OR REPLACE FUNCTION org_asset_guard() RETURNS trigger LANGUAGE plpgsql SET search_path=pg_catalog AS $$
DECLARE selected_org UUID;root_state TEXT;usage JSONB;
BEGIN
 selected_org=CASE WHEN TG_OP='DELETE' THEN OLD.org_id ELSE NEW.org_id END;
 IF selected_org IS DISTINCT FROM NULLIF(current_setting('anpfuel.org_id',true),'')::uuid THEN RAISE EXCEPTION USING ERRCODE='42501',MESSAGE='organization scope denied';END IF;
 SELECT state INTO root_state FROM public.organizations WHERE org_id=selected_org AND has_snapshot FOR UPDATE;
 IF NOT FOUND THEN RAISE EXCEPTION USING ERRCODE='23503',MESSAGE='private reference not found';END IF;
 IF root_state NOT IN('PENDING_SETUP','ACTIVE') THEN RAISE EXCEPTION USING ERRCODE='55000',MESSAGE='organization does not accept asset changes';END IF;
 IF TG_OP='DELETE' THEN
  IF OLD.state<>'DRAFT' THEN RAISE EXCEPTION USING ERRCODE='P7903',MESSAGE='asset is not an unused draft';END IF;
  usage=public.org_asset_current_usage(OLD.org_id,OLD.id);
  IF (usage->>'readings')::bigint<>0 OR(usage->>'operations')::bigint<>0 THEN RAISE EXCEPTION USING ERRCODE='P7903',MESSAGE='asset is not an unused draft';END IF;
  RETURN OLD;
 END IF;
 IF NEW.fleet_key IS DISTINCT FROM public.org_asset_identifier(NEW.fleet_number,false) OR NEW.plate_key IS DISTINCT FROM public.org_asset_identifier(NEW.plate,true) THEN RAISE EXCEPTION USING ERRCODE='23514',MESSAGE='asset identifier projection mismatch';END IF;
 IF TG_OP='INSERT' THEN
  IF NEW.state<>'DRAFT' OR NEW.revision<>1 THEN RAISE EXCEPTION USING ERRCODE='23514',MESSAGE='assets start as draft';END IF;
  NEW.created_at=clock_timestamp();NEW.updated_at=NEW.created_at;
 ELSE
  IF NEW.org_id<>OLD.org_id OR NEW.id<>OLD.id OR OLD.revision=9223372036854775807 OR NEW.revision<>OLD.revision+1 THEN RAISE EXCEPTION USING ERRCODE='23514',MESSAGE='invalid asset identity or revision';END IF;
  IF NEW.state IS DISTINCT FROM OLD.state AND NOT(OLD.state='DRAFT' AND NEW.state='ACTIVE') THEN RAISE EXCEPTION USING ERRCODE='P7903',MESSAGE='asset transition unavailable';END IF;
  IF NEW.kind<>OLD.kind THEN
   usage=public.org_asset_current_usage(OLD.org_id,OLD.id);
   IF (usage->>'readings')::bigint<>0 OR(usage->>'operations')::bigint<>0 THEN RAISE EXCEPTION USING ERRCODE='P7903',MESSAGE='asset kind fixed by accepted operation';END IF;
  END IF;
  IF NEW.state='ACTIVE' AND NEW.kind<>'IMPLEMENT' THEN
   IF NOT EXISTS(SELECT 1 FROM public.org_asset_tanks WHERE org_id=NEW.org_id AND asset_id=NEW.id) OR NOT EXISTS(SELECT 1 FROM public.org_meters WHERE org_id=NEW.org_id AND asset_id=NEW.id AND (installed_at,installed_submicro)<=(clock_timestamp(),0) AND removed_at IS NULL) THEN RAISE EXCEPTION USING ERRCODE='23514',MESSAGE='asset activation evidence incomplete';END IF;
  END IF;
  IF OLD.state='ACTIVE' AND NEW.unit_id IS DISTINCT FROM OLD.unit_id THEN RAISE EXCEPTION USING ERRCODE='P7903',MESSAGE='active asset unit change unavailable';END IF;
  IF NEW.state='ACTIVE' AND OLD.state='DRAFT' THEN
   IF NEW.kind<>'IMPLEMENT' THEN
   IF NOT EXISTS(SELECT 1 FROM public.org_asset_tanks WHERE org_id=NEW.org_id AND asset_id=NEW.id) OR NOT EXISTS(SELECT 1 FROM public.org_meters WHERE org_id=NEW.org_id AND asset_id=NEW.id AND (installed_at,installed_submicro)<=(clock_timestamp(),0) AND removed_at IS NULL) THEN RAISE EXCEPTION USING ERRCODE='23514',MESSAGE='asset activation evidence incomplete';END IF;
  END IF;
   PERFORM public.org_unit_admit(NEW.org_id,NEW.unit_id);
  END IF;
  NEW.created_at=OLD.created_at;NEW.updated_at=clock_timestamp();
 END IF;
 IF NEW.unit_id IS NOT NULL AND NOT EXISTS(SELECT 1 FROM public.org_units WHERE org_id=NEW.org_id AND id=NEW.unit_id AND state='ACTIVE') THEN RAISE EXCEPTION USING ERRCODE='23503',MESSAGE='active private reference not found';END IF;
 RETURN NEW;
END $$;

-- Validate final ACTIVE configuration after the complete source rewrite.
CREATE OR REPLACE FUNCTION org_asset_configuration_complete() RETURNS trigger LANGUAGE plpgsql SET search_path=pg_catalog AS $$
DECLARE selected_org UUID;selected_asset UUID;
BEGIN
 selected_org=CASE WHEN TG_OP='DELETE' THEN OLD.org_id ELSE NEW.org_id END;
 selected_asset=CASE WHEN TG_OP='DELETE' THEN OLD.asset_id ELSE NEW.asset_id END;
 IF EXISTS(SELECT 1 FROM public.org_asset_tanks t WHERE t.org_id=selected_org AND t.asset_id=selected_asset AND NOT EXISTS(SELECT 1 FROM public.org_asset_tank_products p WHERE(p.org_id,p.asset_id,p.tank_position)=(t.org_id,t.asset_id,t.position)))
 OR EXISTS(SELECT 1 FROM public.org_asset_tank_products p JOIN public.org_asset_tank_products q USING(org_id,asset_id,product) WHERE p.org_id=selected_org AND p.asset_id=selected_asset AND p.unit<>q.unit)
 OR EXISTS(SELECT 1 FROM public.org_asset_expected e WHERE e.org_id=selected_org AND e.asset_id=selected_asset AND NOT EXISTS(SELECT 1 FROM public.org_asset_tank_products p WHERE(p.org_id,p.asset_id,p.product,p.unit)=(e.org_id,e.asset_id,e.product,e.unit)))
 OR EXISTS(SELECT 1 FROM public.org_assets a WHERE a.org_id=selected_org AND a.id=selected_asset AND a.state='ACTIVE' AND a.kind<>'IMPLEMENT' AND(NOT EXISTS(SELECT 1 FROM public.org_asset_tanks t WHERE(t.org_id,t.asset_id)=(a.org_id,a.id)) OR NOT EXISTS(SELECT 1 FROM public.org_meters m WHERE(m.org_id,m.asset_id)=(a.org_id,a.id) AND(m.installed_at,m.installed_submicro)<=(clock_timestamp(),0) AND m.removed_at IS NULL)))
 THEN RAISE EXCEPTION USING ERRCODE='23514',MESSAGE='incomplete asset configuration';END IF;
 RETURN NULL;
END $$;

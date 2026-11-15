-- EMP-CHECK-001. Checklist templates per asset category (B-BR-EMP-330..335,
-- EDR-213..215). Additive: six forced-RLS tables, their guards, one deferred
-- completeness/exclusivity check, and the asset-use source with the new asset
-- target key allow-listed. Applied migrations stay unchanged; no existing row
-- is written.

CREATE FUNCTION org_checklist_keep_history() RETURNS trigger LANGUAGE plpgsql SET search_path=pg_catalog AS $$
BEGIN RAISE EXCEPTION USING ERRCODE='55000',MESSAGE='checklist template history is insert-only';END $$;

-- Every write runs under the ACTIVE organization root lock (the FLEET-007
-- serialisation of organization configuration writes).
CREATE FUNCTION org_checklist_root(selected_org UUID) RETURNS VOID LANGUAGE plpgsql SET search_path=pg_catalog AS $$
BEGIN
 IF selected_org IS DISTINCT FROM NULLIF(current_setting('anpfuel.org_id',true),'')::uuid THEN RAISE EXCEPTION USING ERRCODE='42501',MESSAGE='organization scope denied';END IF;
 PERFORM 1 FROM public.organizations WHERE org_id=selected_org AND has_snapshot AND state='ACTIVE' FOR UPDATE;
 IF NOT FOUND THEN RAISE EXCEPTION USING ERRCODE='55000',MESSAGE='organization does not accept checklist changes';END IF;
END $$;

-- One receipt per command, bound to its audit event; the stored result is
-- the acknowledgement a replay returns.
CREATE TABLE org_checklist_template_commands(
 org_id UUID NOT NULL REFERENCES organizations(org_id),
 operation_id UUID NOT NULL CHECK(operation_id<>'00000000-0000-0000-0000-000000000000'::uuid),
 actor_id UUID NOT NULL CHECK(actor_id<>'00000000-0000-0000-0000-000000000000'::uuid),
 template_id UUID NOT NULL CHECK(template_id<>'00000000-0000-0000-0000-000000000000'::uuid),
 kind TEXT NOT NULL CHECK(kind IN('CREATE','PUBLISH','POLICY','ARCHIVE')),
 request_hash TEXT NOT NULL CHECK(request_hash~'^[a-f0-9]{64}$'),
 result_revision BIGINT NOT NULL CHECK(result_revision>0),
 result_version INTEGER NOT NULL CHECK(result_version>0),
 result_policy_revision INTEGER NOT NULL CHECK(result_policy_revision>0),
 result_state TEXT NOT NULL CHECK(result_state IN('ACTIVE','ARCHIVED')),
 created_at TIMESTAMPTZ NOT NULL DEFAULT clock_timestamp() CHECK(isfinite(created_at)),
 PRIMARY KEY(org_id,operation_id),
 CONSTRAINT org_checklist_template_commands_audit_fkey FOREIGN KEY(org_id,operation_id) REFERENCES org_audit_events(org_id,operation_id) DEFERRABLE INITIALLY DEFERRED,
 CONSTRAINT org_checklist_template_commands_result_check CHECK((kind='ARCHIVE')=(result_state='ARCHIVED')
  AND(kind<>'CREATE' OR(result_revision=1 AND result_version=1 AND result_policy_revision=1)) AND(kind='CREATE' OR result_revision>1))
);
CREATE INDEX org_checklist_template_commands_template ON org_checklist_template_commands(org_id,template_id,created_at);

-- B-BR-EMP-331: the template head; its context never changes and its state,
-- current version and current policy revision move only through the guard.
CREATE TABLE org_checklist_templates(
 org_id UUID NOT NULL REFERENCES organizations(org_id),
 id UUID NOT NULL CHECK(id<>'00000000-0000-0000-0000-000000000000'::uuid),
 context TEXT NOT NULL CHECK(context IN('DEPARTURE','RETURN','AD_HOC')),
 state TEXT NOT NULL CHECK(state IN('ACTIVE','ARCHIVED')),
 revision BIGINT NOT NULL CHECK(revision>0),
 version INTEGER NOT NULL CHECK(version>0),
 policy_revision INTEGER NOT NULL CHECK(policy_revision>0),
 operation_id UUID NOT NULL,
 created_by UUID NOT NULL CHECK(created_by<>'00000000-0000-0000-0000-000000000000'::uuid),
 created_at TIMESTAMPTZ NOT NULL DEFAULT clock_timestamp(),
 archived_at TIMESTAMPTZ,archive_operation_id UUID,
 archived_by UUID CHECK(archived_by<>'00000000-0000-0000-0000-000000000000'::uuid),
 PRIMARY KEY(org_id,id),
 CONSTRAINT org_checklist_templates_command_fkey FOREIGN KEY(org_id,operation_id) REFERENCES org_checklist_template_commands(org_id,operation_id) DEFERRABLE INITIALLY DEFERRED,
 CONSTRAINT org_checklist_templates_archive_command_fkey FOREIGN KEY(org_id,archive_operation_id) REFERENCES org_checklist_template_commands(org_id,operation_id) DEFERRABLE INITIALLY DEFERRED,
 CONSTRAINT org_checklist_templates_archive_check CHECK((state='ARCHIVED')=(archived_at IS NOT NULL) AND(archived_at IS NULL)=(archive_operation_id IS NULL)
  AND(archived_at IS NULL)=(archived_by IS NULL) AND isfinite(created_at) AND(archived_at IS NULL OR isfinite(archived_at)))
);
CREATE INDEX org_checklist_templates_context ON org_checklist_templates(org_id,context,state);

-- B-BR-EMP-330: one immutable version; its item and target counts close it.
CREATE TABLE org_checklist_template_versions(
 org_id UUID NOT NULL,template_id UUID NOT NULL,
 version INTEGER NOT NULL CHECK(version>0),
 name TEXT NOT NULL CHECK(org_asset_text_valid(name,80,true)),
 target_kind TEXT NOT NULL CHECK(target_kind IN('ORGANIZATION','CATEGORIES','ASSETS')),
 target_count INTEGER NOT NULL,
 item_count INTEGER NOT NULL CHECK(item_count BETWEEN 1 AND 40),
 operation_id UUID NOT NULL,
 published_by UUID NOT NULL CHECK(published_by<>'00000000-0000-0000-0000-000000000000'::uuid),
 published_at TIMESTAMPTZ NOT NULL DEFAULT clock_timestamp() CHECK(isfinite(published_at)),
 PRIMARY KEY(org_id,template_id,version),
 CONSTRAINT org_checklist_template_versions_target_check CHECK(target_count BETWEEN 0 AND 100 AND(target_kind='ORGANIZATION')=(target_count=0)),
 CONSTRAINT org_checklist_template_versions_template_fkey FOREIGN KEY(org_id,template_id) REFERENCES org_checklist_templates(org_id,id),
 CONSTRAINT org_checklist_template_versions_command_fkey FOREIGN KEY(org_id,operation_id) REFERENCES org_checklist_template_commands(org_id,operation_id) DEFERRABLE INITIALLY DEFERRED
);

CREATE TABLE org_checklist_template_items(
 org_id UUID NOT NULL,template_id UUID NOT NULL,version INTEGER NOT NULL,
 position INTEGER NOT NULL CHECK(position BETWEEN 1 AND 40),
 label TEXT NOT NULL CHECK(org_asset_text_valid(label,120,true)),
 critical BOOLEAN NOT NULL,optional BOOLEAN NOT NULL,photo_on_not_ok BOOLEAN NOT NULL,
 PRIMARY KEY(org_id,template_id,version,position),
 CONSTRAINT org_checklist_template_items_flags_check CHECK(NOT(critical AND optional)),
 CONSTRAINT org_checklist_template_items_version_fkey FOREIGN KEY(org_id,template_id,version) REFERENCES org_checklist_template_versions(org_id,template_id,version)
);

-- Exactly one of a category or an asset of the same organization.
CREATE TABLE org_checklist_template_targets(
 org_id UUID NOT NULL,template_id UUID NOT NULL,version INTEGER NOT NULL,
 category_id UUID,asset_id UUID,
 CONSTRAINT org_checklist_template_targets_kind_check CHECK(num_nonnulls(category_id,asset_id)=1),
 CONSTRAINT org_checklist_template_targets_version_fkey FOREIGN KEY(org_id,template_id,version) REFERENCES org_checklist_template_versions(org_id,template_id,version),
 CONSTRAINT org_checklist_template_targets_category_fkey FOREIGN KEY(org_id,category_id) REFERENCES org_asset_categories(org_id,id),
 CONSTRAINT org_checklist_template_targets_asset_fkey FOREIGN KEY(org_id,asset_id) REFERENCES org_assets(org_id,id),
 CONSTRAINT org_checklist_template_targets_category_unique UNIQUE(org_id,template_id,version,category_id),
 CONSTRAINT org_checklist_template_targets_asset_unique UNIQUE(org_id,template_id,version,asset_id)
);
CREATE INDEX org_checklist_template_targets_asset ON org_checklist_template_targets(org_id,asset_id) WHERE asset_id IS NOT NULL;
CREATE INDEX org_checklist_template_targets_category ON org_checklist_template_targets(org_id,category_id) WHERE category_id IS NOT NULL;

-- B-BR-EMP-333: the per-template policy history.
CREATE TABLE org_checklist_policy_revisions(
 org_id UUID NOT NULL,template_id UUID NOT NULL,
 revision INTEGER NOT NULL CHECK(revision>0),
 mode TEXT NOT NULL CHECK(mode IN('BLOCK','WARN')),
 validity_seconds INTEGER NOT NULL CHECK(validity_seconds BETWEEN 3600 AND 259200),
 operation_id UUID NOT NULL,
 set_by UUID NOT NULL CHECK(set_by<>'00000000-0000-0000-0000-000000000000'::uuid),
 effective_at TIMESTAMPTZ NOT NULL DEFAULT clock_timestamp() CHECK(isfinite(effective_at)),
 PRIMARY KEY(org_id,template_id,revision),
 CONSTRAINT org_checklist_policy_revisions_template_fkey FOREIGN KEY(org_id,template_id) REFERENCES org_checklist_templates(org_id,id),
 CONSTRAINT org_checklist_policy_revisions_command_fkey FOREIGN KEY(org_id,operation_id) REFERENCES org_checklist_template_commands(org_id,operation_id) DEFERRABLE INITIALLY DEFERRED
);

-- The head always names an existing current version and policy revision.
ALTER TABLE org_checklist_templates
 ADD CONSTRAINT org_checklist_templates_version_fkey FOREIGN KEY(org_id,id,version) REFERENCES org_checklist_template_versions(org_id,template_id,version) DEFERRABLE INITIALLY DEFERRED,
 ADD CONSTRAINT org_checklist_templates_policy_fkey FOREIGN KEY(org_id,id,policy_revision) REFERENCES org_checklist_policy_revisions(org_id,template_id,revision) DEFERRABLE INITIALLY DEFERRED;

DO $$
DECLARE name TEXT;
BEGIN
 FOREACH name IN ARRAY ARRAY['org_checklist_template_commands','org_checklist_templates','org_checklist_template_versions','org_checklist_template_items','org_checklist_template_targets','org_checklist_policy_revisions'] LOOP
 EXECUTE format('ALTER TABLE %I ENABLE ROW LEVEL SECURITY',name);EXECUTE format('ALTER TABLE %I FORCE ROW LEVEL SECURITY',name);
 EXECUTE format('CREATE POLICY %I ON %I USING(org_id=NULLIF(current_setting(''anpfuel.org_id'',true),'''')::uuid) WITH CHECK(org_id=NULLIF(current_setting(''anpfuel.org_id'',true),'''')::uuid)',name||'_scope',name);
 EXECUTE format('REVOKE ALL ON %I FROM PUBLIC',name);
 END LOOP;
 FOREACH name IN ARRAY ARRAY['org_checklist_template_commands','org_checklist_template_versions','org_checklist_template_items','org_checklist_template_targets','org_checklist_policy_revisions'] LOOP
 EXECUTE format('CREATE TRIGGER %I BEFORE UPDATE OR DELETE OR TRUNCATE ON %I FOR EACH STATEMENT EXECUTE FUNCTION org_checklist_keep_history()',name||'_immutable',name);
 END LOOP;
END $$;
CREATE TRIGGER org_checklist_templates_immutable BEFORE DELETE OR TRUNCATE ON org_checklist_templates FOR EACH STATEMENT EXECUTE FUNCTION org_checklist_keep_history();

-- Insert: ACTIVE, revision, version and policy revision 1. Update: identity
-- and context fixed, ACTIVE only, revision +1 and exactly one of publish
-- (version +1), policy (policy revision +1) or archive (terminal).
CREATE FUNCTION org_checklist_template_guard() RETURNS trigger LANGUAGE plpgsql SET search_path=pg_catalog AS $$
BEGIN
 PERFORM public.org_checklist_root(NEW.org_id);
 IF TG_OP='INSERT' THEN
  IF NEW.state<>'ACTIVE' OR NEW.revision<>1 OR NEW.version<>1 OR NEW.policy_revision<>1 OR NEW.archived_at IS NOT NULL OR NEW.archive_operation_id IS NOT NULL OR NEW.archived_by IS NOT NULL THEN
   RAISE EXCEPTION USING ERRCODE='23514',MESSAGE='invalid initial checklist template';
  END IF;
  NEW.created_at=clock_timestamp();
  RETURN NEW;
 END IF;
 IF ROW(NEW.org_id,NEW.id,NEW.context,NEW.operation_id,NEW.created_by,NEW.created_at) IS DISTINCT FROM ROW(OLD.org_id,OLD.id,OLD.context,OLD.operation_id,OLD.created_by,OLD.created_at)
  OR OLD.state<>'ACTIVE' OR NEW.revision IS DISTINCT FROM OLD.revision+1 THEN
  RAISE EXCEPTION USING ERRCODE='55000',MESSAGE='checklist template is immutable';
 END IF;
 IF NEW.state='ACTIVE' AND NEW.archived_at IS NULL AND NEW.archive_operation_id IS NULL AND NEW.archived_by IS NULL
  AND(NEW.version=OLD.version+1 AND NEW.policy_revision=OLD.policy_revision OR NEW.version=OLD.version AND NEW.policy_revision=OLD.policy_revision+1) THEN
  RETURN NEW;
 END IF;
 IF NEW.state='ARCHIVED' AND NEW.version=OLD.version AND NEW.policy_revision=OLD.policy_revision AND NEW.archive_operation_id IS NOT NULL AND NEW.archived_by IS NOT NULL THEN
  NEW.archived_at=clock_timestamp();
  RETURN NEW;
 END IF;
 RAISE EXCEPTION USING ERRCODE='55000',MESSAGE='checklist template transition unavailable';
END $$;
CREATE TRIGGER org_checklist_template_write BEFORE INSERT OR UPDATE ON org_checklist_templates FOR EACH ROW EXECUTE FUNCTION org_checklist_template_guard();

-- A version is the head's current version, numbered after every earlier one.
CREATE FUNCTION org_checklist_version_guard() RETURNS trigger LANGUAGE plpgsql SET search_path=pg_catalog AS $$
DECLARE head_version INTEGER;head_state TEXT;
BEGIN
 PERFORM public.org_checklist_root(NEW.org_id);
 SELECT version,state INTO head_version,head_state FROM public.org_checklist_templates WHERE org_id=NEW.org_id AND id=NEW.template_id;
 IF NOT FOUND THEN RAISE EXCEPTION USING ERRCODE='23503',CONSTRAINT='org_checklist_template_versions_template_fkey',MESSAGE='private reference not found';END IF;
 IF head_state<>'ACTIVE' OR NEW.version<>head_version
  OR NEW.version<>COALESCE((SELECT max(v.version) FROM public.org_checklist_template_versions v WHERE v.org_id=NEW.org_id AND v.template_id=NEW.template_id),0)+1 THEN
  RAISE EXCEPTION USING ERRCODE='55000',MESSAGE='checklist version sequence unavailable';
 END IF;
 NEW.published_at=clock_timestamp();
 RETURN NEW;
END $$;
CREATE TRIGGER org_checklist_version_write BEFORE INSERT ON org_checklist_template_versions FOR EACH ROW EXECUTE FUNCTION org_checklist_version_guard();

-- Items and targets fit the counts their version stated; a committed
-- version is complete, so nothing can be added to it afterwards.
CREATE FUNCTION org_checklist_child_guard() RETURNS trigger LANGUAGE plpgsql SET search_path=pg_catalog AS $$
DECLARE kind TEXT;targets INTEGER;items INTEGER;asset_state TEXT;
BEGIN
 IF NEW.org_id IS DISTINCT FROM NULLIF(current_setting('anpfuel.org_id',true),'')::uuid THEN RAISE EXCEPTION USING ERRCODE='42501',MESSAGE='organization scope denied';END IF;
 SELECT target_kind,target_count,item_count INTO kind,targets,items FROM public.org_checklist_template_versions WHERE org_id=NEW.org_id AND template_id=NEW.template_id AND version=NEW.version;
 IF NOT FOUND THEN RAISE EXCEPTION USING ERRCODE='23503',CONSTRAINT=TG_TABLE_NAME||'_version_fkey',MESSAGE='private reference not found';END IF;
 IF TG_TABLE_NAME='org_checklist_template_items' THEN
  IF NEW.position>items THEN RAISE EXCEPTION USING ERRCODE='55000',MESSAGE='checklist version is closed';END IF;
  RETURN NEW;
 END IF;
 IF(NEW.category_id IS NOT NULL)<>(kind='CATEGORIES') OR(NEW.asset_id IS NOT NULL)<>(kind='ASSETS') THEN
  RAISE EXCEPTION USING ERRCODE='23514',MESSAGE='checklist target kind mismatch';
 END IF;
 IF(SELECT count(*) FROM public.org_checklist_template_targets t WHERE t.org_id=NEW.org_id AND t.template_id=NEW.template_id AND t.version=NEW.version)>=targets THEN
  RAISE EXCEPTION USING ERRCODE='55000',MESSAGE='checklist version is closed';
 END IF;
 IF NEW.asset_id IS NOT NULL THEN
  SELECT state INTO asset_state FROM public.org_assets WHERE org_id=NEW.org_id AND id=NEW.asset_id;
  IF NOT FOUND THEN RAISE EXCEPTION USING ERRCODE='23503',CONSTRAINT='org_checklist_template_targets_asset_fkey',MESSAGE='private reference not found';END IF;
  IF asset_state NOT IN('ACTIVE','INACTIVE') THEN RAISE EXCEPTION USING ERRCODE='23514',MESSAGE='asset does not accept a checklist target';END IF;
 END IF;
 RETURN NEW;
END $$;
CREATE TRIGGER org_checklist_item_write BEFORE INSERT ON org_checklist_template_items FOR EACH ROW EXECUTE FUNCTION org_checklist_child_guard();
CREATE TRIGGER org_checklist_target_write BEFORE INSERT ON org_checklist_template_targets FOR EACH ROW EXECUTE FUNCTION org_checklist_child_guard();

-- A policy revision is the head's current one, numbered after every earlier
-- one, effective at its insertion.
CREATE FUNCTION org_checklist_policy_guard() RETURNS trigger LANGUAGE plpgsql SET search_path=pg_catalog AS $$
DECLARE head_revision INTEGER;head_state TEXT;
BEGIN
 PERFORM public.org_checklist_root(NEW.org_id);
 SELECT policy_revision,state INTO head_revision,head_state FROM public.org_checklist_templates WHERE org_id=NEW.org_id AND id=NEW.template_id;
 IF NOT FOUND THEN RAISE EXCEPTION USING ERRCODE='23503',CONSTRAINT='org_checklist_policy_revisions_template_fkey',MESSAGE='private reference not found';END IF;
 IF head_state<>'ACTIVE' OR NEW.revision<>head_revision
  OR NEW.revision<>COALESCE((SELECT max(p.revision) FROM public.org_checklist_policy_revisions p WHERE p.org_id=NEW.org_id AND p.template_id=NEW.template_id),0)+1 THEN
  RAISE EXCEPTION USING ERRCODE='55000',MESSAGE='checklist policy sequence unavailable';
 END IF;
 NEW.effective_at=clock_timestamp();
 RETURN NEW;
END $$;
CREATE TRIGGER org_checklist_policy_write BEFORE INSERT ON org_checklist_policy_revisions FOR EACH ROW EXECUTE FUNCTION org_checklist_policy_guard();

-- At commit a version is published complete and, while its template is
-- ACTIVE, names no category, asset or organization default that another
-- ACTIVE template of the same context names in its current version
-- (B-BR-EMP-332). Writers hold the root lock, so the check is serialised.
CREATE FUNCTION org_checklist_version_complete() RETURNS trigger LANGUAGE plpgsql SET search_path=pg_catalog AS $$
DECLARE v public.org_checklist_template_versions%ROWTYPE;head public.org_checklist_templates%ROWTYPE;prior_org TEXT;
BEGIN
 prior_org=current_setting('anpfuel.org_id',true);PERFORM set_config('anpfuel.org_id',NEW.org_id::text,true);
 SELECT * INTO v FROM public.org_checklist_template_versions WHERE org_id=NEW.org_id AND template_id=NEW.template_id AND version=NEW.version;
 SELECT * INTO head FROM public.org_checklist_templates WHERE org_id=NEW.org_id AND id=NEW.template_id;
 IF v.version IS NULL OR head.id IS NULL OR head.version<>v.version
  OR(SELECT count(*) FROM public.org_checklist_template_items i WHERE i.org_id=v.org_id AND i.template_id=v.template_id AND i.version=v.version)<>v.item_count
  OR(SELECT count(*) FROM public.org_checklist_template_targets t WHERE t.org_id=v.org_id AND t.template_id=v.template_id AND t.version=v.version)<>v.target_count THEN
  RAISE EXCEPTION USING ERRCODE='23514',MESSAGE='checklist version publication unavailable';
 END IF;
 IF head.state='ACTIVE' AND EXISTS(SELECT 1 FROM public.org_checklist_templates o JOIN public.org_checklist_template_versions ov ON(ov.org_id,ov.template_id,ov.version)=(o.org_id,o.id,o.version)
  WHERE o.org_id=v.org_id AND o.id<>v.template_id AND o.state='ACTIVE' AND o.context=head.context
  AND(v.target_kind='ORGANIZATION' AND ov.target_kind='ORGANIZATION'
   OR EXISTS(SELECT 1 FROM public.org_checklist_template_targets mine JOIN public.org_checklist_template_targets theirs ON theirs.org_id=mine.org_id
    AND(theirs.category_id=mine.category_id OR theirs.asset_id=mine.asset_id)
    WHERE mine.org_id=v.org_id AND mine.template_id=v.template_id AND mine.version=v.version AND theirs.template_id=o.id AND theirs.version=o.version))) THEN
  RAISE EXCEPTION USING ERRCODE='23505',CONSTRAINT='org_checklist_templates_target_taken',MESSAGE='checklist target already taken';
 END IF;
 PERFORM set_config('anpfuel.org_id',COALESCE(prior_org,''),true);RETURN NULL;
END $$;
CREATE CONSTRAINT TRIGGER org_checklist_version_complete AFTER INSERT ON org_checklist_template_versions DEFERRABLE INITIALLY DEFERRED FOR EACH ROW EXECUTE FUNCTION org_checklist_version_complete();

-- The 217 asset-use source with the checklist asset target recognised: a
-- target names only an ACTIVE or INACTIVE asset, never a discardable draft,
-- so the counts are unchanged.
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
 OR(c.conrelid='public.org_checklist_template_targets'::regclass AND c.conname='org_checklist_template_targets_asset_fkey'))) THEN RAISE EXCEPTION USING ERRCODE='P7904',MESSAGE='asset usage source unavailable';END IF;
 RETURN jsonb_build_object('readings',(SELECT count(*) FROM public.org_meter_readings r JOIN public.org_meters m ON(m.org_id,m.id)=(r.org_id,r.meter_id) WHERE m.org_id=selected_org AND m.asset_id=selected_asset),'operations',(SELECT count(*) FROM public.org_sync_operations o JOIN public.org_sync_effects e USING(org_id,operation_id) WHERE o.org_id=selected_org AND o.entity_id=selected_asset AND o.entity_type='fleet_asset')+(SELECT count(*) FROM public.org_meter_held_readings h WHERE h.org_id=selected_org AND h.asset_id=selected_asset)+(SELECT count(*) FROM public.org_fuelings f WHERE f.org_id=selected_org AND f.asset_id=selected_asset));
END $$;

-- EMP-DOC-003. Owned same-organization links between an EMP-DOC-002 document
-- and the target records that exist today (EDR-162). One row per document,
-- target kind and target UUID; exactly the kind's typed reference column
-- carries the target through a composite same-organization foreign key, so
-- no link can dangle or cross organizations. FUELING, EXPENSE, WORK_ORDER and
-- TRIP are refused until their owning tasks add a column, its foreign key and
-- a resolver in a new migration. A link carries no amount, content or
-- locator and conveys no access. The creating membership is a historical
-- reference like audit actors (EDR-146), without a membership foreign key.
CREATE TABLE org_document_links(
 org_id UUID NOT NULL REFERENCES organizations(org_id),
 document_id UUID NOT NULL,
 target_kind TEXT NOT NULL CHECK(target_kind IN('ASSET','EMPLOYEE','METER_READING')),
 target_id UUID NOT NULL CHECK(target_id<>'00000000-0000-0000-0000-000000000000'::uuid),
 asset_id UUID,
 employee_id UUID,
 reading_id UUID,
 held_reading_id UUID,
 operation_id UUID NOT NULL CHECK(operation_id<>'00000000-0000-0000-0000-000000000000'::uuid),
 created_by UUID NOT NULL CHECK(created_by<>'00000000-0000-0000-0000-000000000000'::uuid),
 created_at TIMESTAMPTZ NOT NULL DEFAULT clock_timestamp(),
 PRIMARY KEY(org_id,document_id,target_kind,target_id),
 CONSTRAINT org_document_links_operation_key UNIQUE(org_id,operation_id),
 CONSTRAINT org_document_links_operation_fkey FOREIGN KEY(org_id,operation_id) REFERENCES org_audit_events(org_id,operation_id) DEFERRABLE INITIALLY DEFERRED,
 CONSTRAINT org_document_links_document_fkey FOREIGN KEY(org_id,document_id) REFERENCES org_documents(org_id,id),
 CONSTRAINT org_document_links_asset_fkey FOREIGN KEY(org_id,asset_id) REFERENCES org_assets(org_id,id),
 CONSTRAINT org_document_links_employee_fkey FOREIGN KEY(org_id,employee_id) REFERENCES org_employee_identities(org_id,id),
 CONSTRAINT org_document_links_reading_fkey FOREIGN KEY(org_id,reading_id) REFERENCES org_meter_readings(org_id,id),
 CONSTRAINT org_document_links_held_reading_fkey FOREIGN KEY(org_id,held_reading_id) REFERENCES org_meter_held_readings(org_id,reading_id),
 -- Exactly the kind's reference column carries the target; a stored and a
 -- held reading share one reading identity (B-BR-EMP-239).
 CONSTRAINT org_document_links_binding CHECK(CASE target_kind
  WHEN 'ASSET' THEN asset_id IS NOT NULL AND asset_id=target_id AND num_nonnulls(employee_id,reading_id,held_reading_id)=0
  WHEN 'EMPLOYEE' THEN employee_id IS NOT NULL AND employee_id=target_id AND num_nonnulls(asset_id,reading_id,held_reading_id)=0
  WHEN 'METER_READING' THEN num_nonnulls(asset_id,employee_id)=0 AND num_nonnulls(reading_id,held_reading_id)=1
   AND COALESCE(reading_id,held_reading_id)=target_id
  ELSE false END)
);
CREATE INDEX org_document_links_target ON org_document_links(org_id,target_kind,target_id,document_id);

CREATE FUNCTION org_document_link_guard() RETURNS trigger LANGUAGE plpgsql SET search_path=pg_catalog AS $$
DECLARE event_action TEXT;event_type TEXT;event_target UUID;storage TEXT;asset_state TEXT;
BEGIN
 IF TG_OP<>'INSERT' THEN
  RAISE EXCEPTION USING ERRCODE='55000',MESSAGE='document links are immutable';
 END IF;
 IF NEW.org_id IS DISTINCT FROM NULLIF(current_setting('anpfuel.org_id',true),'')::uuid THEN
  RAISE EXCEPTION USING ERRCODE='42501',MESSAGE='organization scope denied';
 END IF;
 SELECT action,target_type,target_id INTO event_action,event_type,event_target
  FROM public.org_audit_events WHERE org_id=NEW.org_id AND operation_id=NEW.operation_id;
 IF NOT FOUND OR event_action<>'document.link.created' OR event_type<>'document' OR event_target<>NEW.document_id THEN
  RAISE EXCEPTION USING ERRCODE='23503',MESSAGE='document link event not found';
 END IF;
 -- A document that can no longer hold an original accepts no new link.
 SELECT storage_state INTO storage FROM public.org_documents WHERE org_id=NEW.org_id AND id=NEW.document_id;
 IF FOUND AND storage IN('FAILED','REPLACED','DELETED') THEN
  RAISE EXCEPTION USING ERRCODE='23514',MESSAGE='document does not accept links';
 END IF;
 -- A DRAFT asset may still be discarded physically; it never gains a reference.
 IF NEW.asset_id IS NOT NULL THEN
  SELECT state INTO asset_state FROM public.org_assets WHERE org_id=NEW.org_id AND id=NEW.asset_id;
  IF FOUND AND asset_state='DRAFT' THEN
   RAISE EXCEPTION USING ERRCODE='23514',MESSAGE='draft asset does not accept links';
  END IF;
 END IF;
 NEW.created_at=clock_timestamp();
 RETURN NEW;
END $$;
CREATE TRIGGER org_document_links_guard BEFORE INSERT OR UPDATE ON org_document_links FOR EACH ROW EXECUTE FUNCTION org_document_link_guard();

-- Links are evidence: unlink and correction retain EMP-DOC-011/012 ownership.
CREATE FUNCTION org_document_link_keep() RETURNS trigger LANGUAGE plpgsql SET search_path=pg_catalog AS $$
BEGIN RAISE EXCEPTION USING ERRCODE='55000',MESSAGE='document links are retained';END $$;
CREATE TRIGGER org_document_links_retained BEFORE DELETE OR TRUNCATE ON org_document_links FOR EACH STATEMENT EXECUTE FUNCTION org_document_link_keep();

ALTER TABLE org_document_links ENABLE ROW LEVEL SECURITY;
ALTER TABLE org_document_links FORCE ROW LEVEL SECURITY;
CREATE POLICY org_document_links_scope ON org_document_links
 USING(org_id=NULLIF(current_setting('anpfuel.org_id',true),'')::uuid)
 WITH CHECK(org_id=NULLIF(current_setting('anpfuel.org_id',true),'')::uuid);
REVOKE ALL ON org_document_links FROM PUBLIC;

-- The 208 asset-use source (FLEET-005 periods and the AUTH-006 asset role
-- scope) with the link reference recognised as an
-- association, never as use: links never reference a DRAFT asset, so they
-- cannot block a discard, and they do not fix the asset kind.
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
 OR(c.conrelid='public.org_document_links'::regclass AND c.conname='org_document_links_asset_fkey'))) THEN RAISE EXCEPTION USING ERRCODE='P7904',MESSAGE='asset usage source unavailable';END IF;
 RETURN jsonb_build_object('readings',(SELECT count(*) FROM public.org_meter_readings r JOIN public.org_meters m ON(m.org_id,m.id)=(r.org_id,r.meter_id) WHERE m.org_id=selected_org AND m.asset_id=selected_asset),'operations',(SELECT count(*) FROM public.org_sync_operations o JOIN public.org_sync_effects e USING(org_id,operation_id) WHERE o.org_id=selected_org AND o.entity_id=selected_asset AND o.entity_type='fleet_asset')+(SELECT count(*) FROM public.org_meter_held_readings h WHERE h.org_id=selected_org AND h.asset_id=selected_asset));
END $$;

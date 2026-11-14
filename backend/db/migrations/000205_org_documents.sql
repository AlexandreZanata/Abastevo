-- EMP-DOC-002. Organization-scoped private document containers (EMP-DOC-001).
-- A row owns the private original's identity: the orgmedia object key, type,
-- size and digest derive only from it. Synthetic C1 receipts only. No bytes,
-- file name, storage key, URL or capability is stored. The creating
-- membership is a historical reference like audit actors (EDR-146), so it has
-- no membership foreign key; current authority is checked by the owner.
CREATE TABLE org_documents(
 org_id UUID NOT NULL REFERENCES organizations(org_id),
 id UUID NOT NULL CHECK(id<>'00000000-0000-0000-0000-000000000000'::uuid),
 kind TEXT NOT NULL CHECK(kind IN('RECEIPT')),
 data_mode TEXT NOT NULL CHECK(data_mode='SYNTHETIC'),
 operation_id UUID NOT NULL CHECK(operation_id<>'00000000-0000-0000-0000-000000000000'::uuid),
 created_by UUID NOT NULL CHECK(created_by<>'00000000-0000-0000-0000-000000000000'::uuid),
 original_sha256 TEXT NOT NULL CHECK(original_sha256~'^[0-9a-f]{64}$'),
 original_bytes BIGINT NOT NULL,
 original_type TEXT NOT NULL CHECK(original_type IN('image/jpeg')),
 storage_state TEXT NOT NULL CHECK(storage_state IN('UPLOADING','RECEIVED','STORED','QUARANTINED','FAILED','REPLACED','DELETED')),
 storage_revision BIGINT NOT NULL CHECK(storage_revision>0),
 extraction_state TEXT NOT NULL CHECK(extraction_state IN('NONE','PENDING','DONE','FAILED')),
 extraction_revision BIGINT NOT NULL CHECK(extraction_revision>0),
 structure_state TEXT NOT NULL CHECK(structure_state IN('NOT_APPLICABLE','UNCHECKED','STRUCTURE_OK','STRUCTURE_INVALID')),
 structure_revision BIGINT NOT NULL CHECK(structure_revision>0),
 conference_state TEXT NOT NULL CHECK(conference_state IN('UNCONFERRED','CONFERRED_OK','DIVERGENT','ILLEGIBLE')),
 conference_revision BIGINT NOT NULL CHECK(conference_revision>0),
 -- Derived from exact allocations (EMP-DOC-007/010); never commanded.
 reconciliation_state TEXT NOT NULL CHECK(reconciliation_state IN('UNALLOCATED','PARTIALLY_ALLOCATED','RECONCILED','OVER_ALLOCATED')),
 fiscal_state TEXT NOT NULL CHECK(fiscal_state IN('NOT_CONSULTED','AUTHORIZED_AS_OF','CANCELLED_AS_OF','UNAVAILABLE')),
 fiscal_revision BIGINT NOT NULL CHECK(fiscal_revision>0),
 fiscal_consulted_at TIMESTAMPTZ CHECK(fiscal_consulted_at IS NULL OR isfinite(fiscal_consulted_at)),
 created_at TIMESTAMPTZ NOT NULL DEFAULT clock_timestamp(),
 updated_at TIMESTAMPTZ NOT NULL DEFAULT clock_timestamp(),
 PRIMARY KEY(org_id,id),
 CONSTRAINT org_documents_operation_key UNIQUE(org_id,operation_id),
 CONSTRAINT org_documents_operation_fkey FOREIGN KEY(org_id,operation_id) REFERENCES org_audit_events(org_id,operation_id) DEFERRABLE INITIALLY DEFERRED,
 -- BR-PRIV-020 budget of the only enabled C1 type.
 CHECK(original_bytes BETWEEN 1 AND 5242880),
 CHECK((fiscal_state='NOT_CONSULTED')=(fiscal_consulted_at IS NULL))
);

-- The EMP-DOC-001 command graph per commanded dimension. REPLACED is entered
-- only by replacement with a successor (EMP-DOC-011), which this table cannot
-- yet record, so no edge enters it here.
CREATE FUNCTION org_document_transition_allowed(dimension TEXT,from_state TEXT,to_state TEXT) RETURNS BOOLEAN
 LANGUAGE SQL IMMUTABLE PARALLEL SAFE SET search_path=pg_catalog AS $$
 SELECT (dimension,from_state,to_state) IN (VALUES
  ('STORAGE','UPLOADING','FAILED'),('STORAGE','UPLOADING','RECEIVED'),
  ('STORAGE','RECEIVED','FAILED'),('STORAGE','RECEIVED','QUARANTINED'),('STORAGE','RECEIVED','STORED'),
  ('STORAGE','STORED','DELETED'),('STORAGE','QUARANTINED','DELETED'),('STORAGE','REPLACED','DELETED'),
  ('EXTRACTION','NONE','PENDING'),('EXTRACTION','PENDING','DONE'),('EXTRACTION','PENDING','FAILED'),('EXTRACTION','FAILED','PENDING'),
  ('STRUCTURE','NOT_APPLICABLE','UNCHECKED'),('STRUCTURE','UNCHECKED','STRUCTURE_INVALID'),('STRUCTURE','UNCHECKED','STRUCTURE_OK'),
  ('STRUCTURE','STRUCTURE_INVALID','UNCHECKED'),
  ('CONFERENCE','UNCONFERRED','CONFERRED_OK'),('CONFERENCE','UNCONFERRED','DIVERGENT'),('CONFERENCE','UNCONFERRED','ILLEGIBLE'),
  ('CONFERENCE','CONFERRED_OK','DIVERGENT'),('CONFERENCE','CONFERRED_OK','ILLEGIBLE'),('CONFERENCE','CONFERRED_OK','UNCONFERRED'),
  ('CONFERENCE','DIVERGENT','CONFERRED_OK'),('CONFERENCE','DIVERGENT','ILLEGIBLE'),('CONFERENCE','DIVERGENT','UNCONFERRED'),
  ('CONFERENCE','ILLEGIBLE','CONFERRED_OK'),('CONFERENCE','ILLEGIBLE','DIVERGENT'),('CONFERENCE','ILLEGIBLE','UNCONFERRED'),
  ('FISCAL','NOT_CONSULTED','AUTHORIZED_AS_OF'),('FISCAL','NOT_CONSULTED','CANCELLED_AS_OF'),('FISCAL','NOT_CONSULTED','UNAVAILABLE'),
  ('FISCAL','AUTHORIZED_AS_OF','AUTHORIZED_AS_OF'),('FISCAL','AUTHORIZED_AS_OF','CANCELLED_AS_OF'),('FISCAL','AUTHORIZED_AS_OF','UNAVAILABLE'),
  ('FISCAL','CANCELLED_AS_OF','AUTHORIZED_AS_OF'),('FISCAL','CANCELLED_AS_OF','CANCELLED_AS_OF'),('FISCAL','CANCELLED_AS_OF','UNAVAILABLE'),
  ('FISCAL','UNAVAILABLE','AUTHORIZED_AS_OF'),('FISCAL','UNAVAILABLE','CANCELLED_AS_OF'),('FISCAL','UNAVAILABLE','UNAVAILABLE'))
$$;

-- One dimension changes by one revision along the graph, or not at all.
CREATE FUNCTION org_document_step(dimension TEXT,old_state TEXT,new_state TEXT,old_revision BIGINT,new_revision BIGINT,changed BOOLEAN) RETURNS BOOLEAN
 LANGUAGE SQL IMMUTABLE PARALLEL SAFE SET search_path=pg_catalog AS $$
 SELECT CASE WHEN old_state=new_state AND NOT changed THEN new_revision=old_revision
  ELSE old_revision<9223372036854775807 AND new_revision=old_revision+1
   AND public.org_document_transition_allowed(dimension,old_state,new_state) END
$$;

CREATE FUNCTION org_document_guard() RETURNS trigger LANGUAGE plpgsql SET search_path=pg_catalog AS $$
DECLARE event_action TEXT;event_type TEXT;event_target UUID;consulted BOOLEAN;
BEGIN
 IF NEW.org_id IS DISTINCT FROM NULLIF(current_setting('anpfuel.org_id',true),'')::uuid THEN
  RAISE EXCEPTION USING ERRCODE='42501',MESSAGE='organization scope denied';
 END IF;
 IF TG_OP='INSERT' THEN
  IF NEW.storage_state<>'UPLOADING' OR NEW.extraction_state<>'NONE' OR NEW.structure_state<>'NOT_APPLICABLE'
   OR NEW.conference_state<>'UNCONFERRED' OR NEW.reconciliation_state<>'UNALLOCATED' OR NEW.fiscal_state<>'NOT_CONSULTED'
   OR NEW.fiscal_consulted_at IS NOT NULL OR NEW.storage_revision<>1 OR NEW.extraction_revision<>1 OR NEW.structure_revision<>1
   OR NEW.conference_revision<>1 OR NEW.fiscal_revision<>1 THEN
   RAISE EXCEPTION USING ERRCODE='23514',MESSAGE='documents start at the initial states';
  END IF;
  SELECT action,target_type,target_id INTO event_action,event_type,event_target
   FROM public.org_audit_events WHERE org_id=NEW.org_id AND operation_id=NEW.operation_id;
  IF NOT FOUND OR event_action<>'document.upload.created' OR event_type<>'document' OR event_target<>NEW.id THEN
   RAISE EXCEPTION USING ERRCODE='23503',MESSAGE='document creation event not found';
  END IF;
  NEW.created_at=clock_timestamp();NEW.updated_at=NEW.created_at;
  RETURN NEW;
 END IF;
 IF (NEW.org_id,NEW.id,NEW.kind,NEW.data_mode,NEW.operation_id,NEW.created_by,NEW.original_sha256,NEW.original_bytes,NEW.original_type,NEW.created_at)
  IS DISTINCT FROM (OLD.org_id,OLD.id,OLD.kind,OLD.data_mode,OLD.operation_id,OLD.created_by,OLD.original_sha256,OLD.original_bytes,OLD.original_type,OLD.created_at) THEN
  RAISE EXCEPTION USING ERRCODE='23514',MESSAGE='document identity is immutable';
 END IF;
 IF NEW.reconciliation_state IS DISTINCT FROM OLD.reconciliation_state THEN
  RAISE EXCEPTION USING ERRCODE='23514',MESSAGE='reconciliation is derived';
 END IF;
 consulted=NEW.fiscal_consulted_at IS DISTINCT FROM OLD.fiscal_consulted_at;
 IF consulted AND NOT(NEW.fiscal_consulted_at IS NOT NULL AND(OLD.fiscal_consulted_at IS NULL OR NEW.fiscal_consulted_at>OLD.fiscal_consulted_at)) THEN
  RAISE EXCEPTION USING ERRCODE='23514',MESSAGE='fiscal consultation must be newer';
 END IF;
 IF NOT public.org_document_step('STORAGE',OLD.storage_state,NEW.storage_state,OLD.storage_revision,NEW.storage_revision,false)
  OR NOT public.org_document_step('EXTRACTION',OLD.extraction_state,NEW.extraction_state,OLD.extraction_revision,NEW.extraction_revision,false)
  OR NOT public.org_document_step('STRUCTURE',OLD.structure_state,NEW.structure_state,OLD.structure_revision,NEW.structure_revision,false)
  OR NOT public.org_document_step('CONFERENCE',OLD.conference_state,NEW.conference_state,OLD.conference_revision,NEW.conference_revision,false)
  OR NOT public.org_document_step('FISCAL',OLD.fiscal_state,NEW.fiscal_state,OLD.fiscal_revision,NEW.fiscal_revision,consulted)
  OR (NEW.fiscal_state IS DISTINCT FROM OLD.fiscal_state AND NOT consulted) THEN
  RAISE EXCEPTION USING ERRCODE='23514',MESSAGE='document transition not allowed';
 END IF;
 NEW.updated_at=clock_timestamp();
 RETURN NEW;
END $$;
CREATE TRIGGER org_documents_guard BEFORE INSERT OR UPDATE ON org_documents FOR EACH ROW EXECUTE FUNCTION org_document_guard();

-- Originals stay retained until the reviewed retention workflow (EMP-DOC-012).
CREATE FUNCTION org_document_keep() RETURNS trigger LANGUAGE plpgsql SET search_path=pg_catalog AS $$
BEGIN RAISE EXCEPTION USING ERRCODE='55000',MESSAGE='documents are retained';END $$;
CREATE TRIGGER org_documents_retained BEFORE DELETE OR TRUNCATE ON org_documents FOR EACH STATEMENT EXECUTE FUNCTION org_document_keep();

ALTER TABLE org_documents ENABLE ROW LEVEL SECURITY;
ALTER TABLE org_documents FORCE ROW LEVEL SECURITY;
CREATE POLICY org_documents_scope ON org_documents
 USING(org_id=NULLIF(current_setting('anpfuel.org_id',true),'')::uuid)
 WITH CHECK(org_id=NULLIF(current_setting('anpfuel.org_id',true),'')::uuid);
REVOKE ALL ON org_documents FROM PUBLIC;

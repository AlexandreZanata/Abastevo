-- EMP-DOC-014. Upload safety (EDR-169/170). Additive and transactional:
-- every QUARANTINED or FAILED document entered from now on carries exactly
-- one closed reason fixed by its transition; an upload session can be closed
-- as abandoned only after its 24-hour window plus a five-minute grace
-- (database clock); documents still pending get their closure job.
-- org_document_guard() below is the latest (000205) body plus the reason and
-- window rules. Rows that entered QUARANTINED or FAILED before this
-- migration (possible only through direct SQL) keep a NULL reason. The
-- previous binary never writes a reason or a terminal state, so it stays
-- compatible with this schema.
ALTER TABLE org_documents
 ADD COLUMN storage_reason TEXT
  CONSTRAINT org_documents_storage_reason_check CHECK(storage_reason IN('UNSAFE_CONTENT','UNSUPPORTED_CONTENT',
   'INTEGRITY_MISMATCH','CHECK_ATTEMPTS_EXHAUSTED','UPLOAD_ABANDONED')),
 ADD CONSTRAINT org_documents_storage_reason_state_check CHECK(storage_reason IS NULL OR storage_state IN('QUARANTINED','FAILED','DELETED'));

-- The closed reason of each terminal storage transition.
CREATE FUNCTION org_document_reason_allowed(from_state TEXT,to_state TEXT,reason TEXT) RETURNS BOOLEAN
 LANGUAGE SQL IMMUTABLE PARALLEL SAFE SET search_path=pg_catalog AS $$
 SELECT (from_state,to_state,reason) IN (VALUES
  ('RECEIVED','QUARANTINED','UNSAFE_CONTENT'),
  ('RECEIVED','FAILED','UNSUPPORTED_CONTENT'),('RECEIVED','FAILED','INTEGRITY_MISMATCH'),('RECEIVED','FAILED','CHECK_ATTEMPTS_EXHAUSTED'),
  ('UPLOADING','FAILED','UPLOAD_ABANDONED'))
$$;

-- A session closes 24 hours after its declaration; capabilities are minted
-- only if they expire by then, and five more minutes cover a transfer that
-- verified its capability just before the close.
CREATE FUNCTION org_document_upload_abandoned(created_at TIMESTAMPTZ) RETURNS BOOLEAN
 LANGUAGE SQL VOLATILE SET search_path=pg_catalog AS $$
 SELECT created_at + interval '24 hours 5 minutes' <= clock_timestamp()
$$;

CREATE OR REPLACE FUNCTION org_document_guard() RETURNS trigger LANGUAGE plpgsql SET search_path=pg_catalog AS $$
DECLARE event_action TEXT;event_type TEXT;event_target UUID;consulted BOOLEAN;
BEGIN
 IF NEW.org_id IS DISTINCT FROM NULLIF(current_setting('anpfuel.org_id',true),'')::uuid THEN
  RAISE EXCEPTION USING ERRCODE='42501',MESSAGE='organization scope denied';
 END IF;
 IF TG_OP='INSERT' THEN
  IF NEW.storage_state<>'UPLOADING' OR NEW.extraction_state<>'NONE' OR NEW.structure_state<>'NOT_APPLICABLE'
   OR NEW.conference_state<>'UNCONFERRED' OR NEW.reconciliation_state<>'UNALLOCATED' OR NEW.fiscal_state<>'NOT_CONSULTED'
   OR NEW.fiscal_consulted_at IS NOT NULL OR NEW.storage_revision<>1 OR NEW.extraction_revision<>1 OR NEW.structure_revision<>1
   OR NEW.conference_revision<>1 OR NEW.fiscal_revision<>1 OR NEW.storage_reason IS NOT NULL THEN
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
 -- EMP-DOC-014: a reason is written once, by its own terminal transition.
 IF NEW.storage_reason IS DISTINCT FROM OLD.storage_reason THEN
  IF OLD.storage_reason IS NOT NULL OR NOT public.org_document_reason_allowed(OLD.storage_state,NEW.storage_state,NEW.storage_reason) THEN
   RAISE EXCEPTION USING ERRCODE='23514',MESSAGE='storage reason is fixed by its transition';
  END IF;
  IF NEW.storage_reason='UPLOAD_ABANDONED' AND NOT public.org_document_upload_abandoned(OLD.created_at) THEN
   RAISE EXCEPTION USING ERRCODE='23514',MESSAGE='upload session still open';
  END IF;
 ELSIF NEW.storage_state IS DISTINCT FROM OLD.storage_state AND NEW.storage_state IN('QUARANTINED','FAILED') THEN
  RAISE EXCEPTION USING ERRCODE='23514',MESSAGE='storage outcome needs its reason';
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

-- Pending documents of the previous binary get the per-document closure job
-- that new declarations queue themselves (EDR-170). Forced row-level
-- security is lifted only inside this atomic migration so that the owner can
-- read every organization's pending rows; the payload carries references only.
ALTER TABLE org_documents NO FORCE ROW LEVEL SECURITY;
INSERT INTO job_queue(id,kind,payload,dedupe_key,max_attempts,not_before)
SELECT gen_random_uuid(),'org-document-safety',
 jsonb_build_object('version',1,'org_id',org_id::text,'document_id',id::text),
 'org-document-safety:expiry:'||org_id::text||':'||id::text,10,
 GREATEST(created_at+interval '24 hours 5 minutes',clock_timestamp())
FROM org_documents WHERE storage_state IN('UPLOADING','RECEIVED')
ON CONFLICT (dedupe_key) WHERE dedupe_key IS NOT NULL DO NOTHING;
ALTER TABLE org_documents FORCE ROW LEVEL SECURITY;

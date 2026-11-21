-- EMP-INTEG-004: a new result follows the verified storage of its original.
-- Existing receipts remain replayable through the unchanged admission trigger.
CREATE INDEX org_document_fiscal_stored_audit_idx
    ON org_audit_events (org_id, target_id, recorded_at)
    WHERE target_type = 'document' AND action = 'document.upload.stored';

CREATE OR REPLACE FUNCTION org_document_apply_fiscal_receipt() RETURNS trigger
LANGUAGE plpgsql SET search_path = pg_catalog AS $$
BEGIN
    UPDATE public.org_documents d
    SET fiscal_state = NEW.status, fiscal_consulted_at = NEW.consulted_at, fiscal_revision = NEW.fiscal_revision
    WHERE (d.org_id, d.id) = (NEW.org_id, NEW.document_id)
      AND d.storage_state = 'STORED' AND d.fiscal_revision = NEW.fiscal_revision - 1
      AND EXISTS (
          SELECT 1 FROM public.org_audit_events a
          WHERE (a.org_id, a.target_id) = (d.org_id, d.id)
            AND a.target_type = 'document' AND a.action = 'document.upload.stored'
            AND a.recorded_at BETWEEN d.created_at AND NEW.consulted_at
      );
    IF NOT FOUND THEN
        RAISE EXCEPTION USING ERRCODE = '23514', MESSAGE = 'verified original or expected fiscal revision unavailable';
    END IF;
    RETURN NULL;
END $$;

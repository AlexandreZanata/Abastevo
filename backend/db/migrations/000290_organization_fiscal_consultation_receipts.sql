-- EMP-INTEG-004: fiscal status is a projection of retained source evidence.
-- This records results only; deployment does not provision a recorder or an
-- active fiscal connector. Existing observations are retained in place.
CREATE TABLE org_document_fiscal_consultations (
    org_id UUID NOT NULL,
    operation_id UUID NOT NULL CHECK (operation_id <> '00000000-0000-0000-0000-000000000000'::uuid),
    document_id UUID NOT NULL,
    fiscal_revision BIGINT NOT NULL CHECK (fiscal_revision > 1),
    connection_id UUID NOT NULL,
    connection_revision BIGINT NOT NULL CHECK (connection_revision > 0),
    status TEXT NOT NULL CHECK (status IN ('AUTHORIZED_AS_OF','CANCELLED_AS_OF','UNAVAILABLE')),
    source_result_code TEXT NOT NULL CHECK (source_result_code ~ '^[A-Za-z0-9][A-Za-z0-9_-]{0,23}$'),
    response_sha256 TEXT NOT NULL CHECK (response_sha256 ~ '^[0-9a-f]{64}$'),
    consulted_at TIMESTAMPTZ NOT NULL CHECK (isfinite(consulted_at)),
    recorded_at TIMESTAMPTZ NOT NULL DEFAULT clock_timestamp() CHECK (isfinite(recorded_at) AND consulted_at <= recorded_at),
    PRIMARY KEY (org_id, operation_id),
    UNIQUE (org_id, document_id, fiscal_revision),
    FOREIGN KEY (org_id, document_id) REFERENCES org_documents(org_id, id),
    FOREIGN KEY (org_id, connection_id) REFERENCES org_connections(org_id, id)
);
ALTER TABLE org_document_fiscal_consultations ENABLE ROW LEVEL SECURITY;
ALTER TABLE org_document_fiscal_consultations FORCE ROW LEVEL SECURITY;
CREATE POLICY org_document_fiscal_consultations_scope ON org_document_fiscal_consultations
    USING (org_id = NULLIF(current_setting('anpfuel.org_id', true), '')::uuid)
    WITH CHECK (org_id = NULLIF(current_setting('anpfuel.org_id', true), '')::uuid);
REVOKE ALL ON org_document_fiscal_consultations FROM PUBLIC;
CREATE TRIGGER org_document_fiscal_consultations_retained BEFORE UPDATE OR DELETE OR TRUNCATE ON org_document_fiscal_consultations
    FOR EACH STATEMENT EXECUTE FUNCTION org_document_keep();

CREATE FUNCTION org_document_fiscal_consultation_admit() RETURNS trigger
LANGUAGE plpgsql SET search_path = pg_catalog AS $$
DECLARE
    previous public.org_document_fiscal_consultations%ROWTYPE;
    selected_approval UUID;
    selected_revision BIGINT;
    activated_at TIMESTAMPTZ;
BEGIN
    IF NEW.org_id IS DISTINCT FROM NULLIF(current_setting('anpfuel.org_id', true), '')::uuid THEN
        RAISE EXCEPTION USING ERRCODE = '42501', MESSAGE = 'organization scope denied';
    END IF;
    PERFORM pg_advisory_xact_lock(hashtextextended('org-document-fiscal:' || NEW.org_id::text || ':' || NEW.operation_id::text, 0));
    SELECT * INTO previous FROM public.org_document_fiscal_consultations r
    WHERE (r.org_id, r.operation_id) = (NEW.org_id, NEW.operation_id);
    IF FOUND THEN
        IF (previous.document_id, previous.fiscal_revision, previous.connection_id, previous.connection_revision,
            previous.status, previous.source_result_code, previous.response_sha256, previous.consulted_at)
            IS DISTINCT FROM (NEW.document_id, NEW.fiscal_revision, NEW.connection_id, NEW.connection_revision,
            NEW.status, NEW.source_result_code, NEW.response_sha256, NEW.consulted_at) THEN
            RAISE EXCEPTION USING ERRCODE = '23505', MESSAGE = 'fiscal consultation operation reused';
        END IF;
        RETURN NULL;
    END IF;

    -- The connector lock order is root, connection, then current approval.
    PERFORM 1 FROM public.organizations o WHERE o.org_id = NEW.org_id AND o.state = 'ACTIVE' FOR SHARE;
    IF NOT FOUND THEN
        RAISE EXCEPTION USING ERRCODE = '23514', MESSAGE = 'active organization required for fiscal consultation';
    END IF;
    SELECT c.approval_id, c.approval_revision, c.changed_at INTO selected_approval, selected_revision, activated_at
    FROM public.org_connections c WHERE (c.org_id, c.id, c.revision) = (NEW.org_id, NEW.connection_id, NEW.connection_revision)
      AND c.state = 'ACTIVE' FOR SHARE;
    IF NOT FOUND THEN
        RAISE EXCEPTION USING ERRCODE = '23514', MESSAGE = 'fiscal connection unavailable';
    END IF;
    PERFORM pg_advisory_xact_lock_shared(hashtextextended('org-connector-approval:' || NEW.org_id::text || ':' || selected_approval::text, 0));
    NEW.recorded_at = clock_timestamp();
    IF NEW.consulted_at < activated_at OR NOT EXISTS (
        SELECT 1 FROM public.org_connector_approvals a
        WHERE (a.org_id, a.id, a.revision) = (NEW.org_id, selected_approval, selected_revision)
          AND a.kind = 'FISCAL_LOOKUP' AND a.decision = 'APPROVED' AND a.resource_id = NEW.document_id
          AND a.credential_ref IS NOT NULL AND a.ready_at <= NEW.consulted_at
          AND NOT EXISTS (SELECT 1 FROM public.org_connector_approvals later
              WHERE (later.org_id, later.id) = (a.org_id, a.id) AND later.revision > a.revision)
    ) THEN
        RAISE EXCEPTION USING ERRCODE = '23514', MESSAGE = 'current fiscal review required';
    END IF;
    RETURN NEW;
END $$;
CREATE TRIGGER org_document_fiscal_consultations_admit BEFORE INSERT ON org_document_fiscal_consultations
    FOR EACH ROW EXECUTE FUNCTION org_document_fiscal_consultation_admit();

CREATE FUNCTION org_document_require_fiscal_receipt() RETURNS trigger
LANGUAGE plpgsql SET search_path = pg_catalog AS $$
BEGIN
    IF NOT EXISTS (
        SELECT 1 FROM public.org_document_fiscal_consultations r
        WHERE (r.org_id, r.document_id, r.fiscal_revision, r.status, r.consulted_at) =
            (NEW.org_id, NEW.id, NEW.fiscal_revision, NEW.fiscal_state, NEW.fiscal_consulted_at)
    ) THEN
        RAISE EXCEPTION USING ERRCODE = '23514', MESSAGE = 'recorded fiscal consultation required';
    END IF;
    RETURN NEW;
END $$;
CREATE TRIGGER org_documents_require_fiscal_receipt BEFORE UPDATE ON org_documents
    FOR EACH ROW WHEN ((NEW.fiscal_state, NEW.fiscal_revision, NEW.fiscal_consulted_at)
        IS DISTINCT FROM (OLD.fiscal_state, OLD.fiscal_revision, OLD.fiscal_consulted_at))
    EXECUTE FUNCTION org_document_require_fiscal_receipt();

CREATE FUNCTION org_document_apply_fiscal_receipt() RETURNS trigger
LANGUAGE plpgsql SET search_path = pg_catalog AS $$
BEGIN
    UPDATE public.org_documents SET fiscal_state = NEW.status,
        fiscal_consulted_at = NEW.consulted_at, fiscal_revision = NEW.fiscal_revision
    WHERE (org_id, id) = (NEW.org_id, NEW.document_id)
      AND storage_state = 'STORED' AND fiscal_revision = NEW.fiscal_revision - 1;
    IF NOT FOUND THEN
        RAISE EXCEPTION USING ERRCODE = '23514', MESSAGE = 'stored document or expected fiscal revision unavailable';
    END IF;
    RETURN NULL;
END $$;
CREATE TRIGGER org_document_fiscal_consultations_apply AFTER INSERT ON org_document_fiscal_consultations
    FOR EACH ROW EXECUTE FUNCTION org_document_apply_fiscal_receipt();

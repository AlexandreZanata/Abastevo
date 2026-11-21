-- EMP-DATA-003: reviewed request rates, exact term use and idempotent admissions.
CREATE TABLE enterprise_data_rate_policies (
    product_code TEXT NOT NULL,
    plan_code TEXT NOT NULL,
    plan_revision BIGINT NOT NULL,
    window_seconds INTEGER NOT NULL CHECK (window_seconds > 0),
    window_calls BIGINT NOT NULL CHECK (window_calls >= 0),
    approval_ref TEXT NOT NULL CHECK (octet_length(approval_ref) BETWEEN 1 AND 200 AND approval_ref=btrim(approval_ref) AND approval_ref !~ '[[:cntrl:]]'),
    PRIMARY KEY (product_code,plan_code,plan_revision),
    FOREIGN KEY (product_code,plan_code,plan_revision) REFERENCES enterprise_plan_catalog(product_code,plan_code,revision)
);
CREATE TRIGGER enterprise_data_rate_policies_immutable BEFORE UPDATE OR DELETE OR TRUNCATE ON enterprise_data_rate_policies
    FOR EACH STATEMENT EXECUTE FUNCTION enterprise_plan_catalog_immutable();

CREATE UNIQUE INDEX org_data_keys_one_rotation_parent ON org_data_api_keys(org_id,replacement_id) WHERE replacement_id IS NOT NULL;

CREATE TABLE org_data_usage_terms (
    org_id UUID NOT NULL REFERENCES organizations(org_id),
    term_start TIMESTAMPTZ NOT NULL CHECK (isfinite(term_start)),
    term_end TIMESTAMPTZ NOT NULL CHECK (isfinite(term_end) AND term_end > term_start),
    calls BIGINT NOT NULL CHECK (calls > 0),
    PRIMARY KEY (org_id,term_start)
);

CREATE TABLE org_data_rate_windows (
    org_id UUID NOT NULL,
    family_id UUID NOT NULL,
    window_start TIMESTAMPTZ NOT NULL CHECK (isfinite(window_start)),
    window_seconds INTEGER NOT NULL CHECK (window_seconds > 0),
    calls BIGINT NOT NULL CHECK (calls > 0),
    PRIMARY KEY (org_id,family_id,window_start,window_seconds),
    FOREIGN KEY (org_id,family_id) REFERENCES org_data_api_keys(org_id,id)
);

CREATE TABLE org_data_request_receipts (
    org_id UUID NOT NULL,
    family_id UUID NOT NULL,
    request_id UUID NOT NULL CHECK (request_id <> '00000000-0000-0000-0000-000000000000'::uuid),
    key_id UUID NOT NULL,
    term_start TIMESTAMPTZ NOT NULL,
    accepted_at TIMESTAMPTZ NOT NULL CHECK (isfinite(accepted_at) AND accepted_at >= term_start),
    request_hash TEXT NOT NULL CHECK (request_hash ~ '^[a-f0-9]{64}$'),
    PRIMARY KEY (org_id,family_id,request_id),
    FOREIGN KEY (org_id,term_start) REFERENCES org_data_usage_terms(org_id,term_start),
    FOREIGN KEY (org_id,family_id) REFERENCES org_data_api_keys(org_id,id),
    FOREIGN KEY (org_id,key_id) REFERENCES org_data_api_keys(org_id,id)
);

ALTER TABLE org_data_usage_terms ENABLE ROW LEVEL SECURITY;
ALTER TABLE org_data_usage_terms FORCE ROW LEVEL SECURITY;
CREATE POLICY org_data_usage_terms_scope ON org_data_usage_terms
    USING (org_id = NULLIF(current_setting('anpfuel.org_id',true),'')::uuid)
    WITH CHECK (org_id = NULLIF(current_setting('anpfuel.org_id',true),'')::uuid);

ALTER TABLE org_data_rate_windows ENABLE ROW LEVEL SECURITY;
ALTER TABLE org_data_rate_windows FORCE ROW LEVEL SECURITY;
CREATE POLICY org_data_rate_windows_scope ON org_data_rate_windows
    USING (org_id = NULLIF(current_setting('anpfuel.org_id',true),'')::uuid)
    WITH CHECK (org_id = NULLIF(current_setting('anpfuel.org_id',true),'')::uuid);

ALTER TABLE org_data_request_receipts ENABLE ROW LEVEL SECURITY;
ALTER TABLE org_data_request_receipts FORCE ROW LEVEL SECURITY;
CREATE POLICY org_data_request_receipts_scope ON org_data_request_receipts
    USING (org_id = NULLIF(current_setting('anpfuel.org_id',true),'')::uuid)
    WITH CHECK (org_id = NULLIF(current_setting('anpfuel.org_id',true),'')::uuid);

CREATE TRIGGER org_data_request_receipts_immutable BEFORE UPDATE OR DELETE OR TRUNCATE ON org_data_request_receipts
    FOR EACH STATEMENT EXECUTE FUNCTION enterprise_plan_catalog_immutable();
REVOKE ALL ON enterprise_data_rate_policies,org_data_usage_terms,org_data_rate_windows,org_data_request_receipts FROM PUBLIC;

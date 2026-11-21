-- EMP-SUBS-007: A reviewed trial grant pins eligibility and terms for an
-- organization. The runtime can consume it through the audited operator path.
CREATE TABLE org_billing_trial_grants (
    org_id UUID NOT NULL REFERENCES organizations(org_id),
    id UUID NOT NULL CHECK (id <> '00000000-0000-0000-0000-000000000000'::uuid),
    product_code TEXT NOT NULL,
    plan_code TEXT NOT NULL,
    plan_revision BIGINT NOT NULL,
    eligible_from TIMESTAMPTZ NOT NULL CHECK (isfinite(eligible_from)),
    eligible_until TIMESTAMPTZ NOT NULL CHECK (isfinite(eligible_until) AND eligible_until > eligible_from),
    duration_seconds INTEGER NOT NULL CHECK (duration_seconds > 0),
    approval_ref TEXT NOT NULL CHECK (octet_length(approval_ref) BETWEEN 1 AND 200 AND approval_ref ~ '^[!-~]+$'),
    PRIMARY KEY (org_id,id),
    FOREIGN KEY (product_code,plan_code,plan_revision)
        REFERENCES enterprise_plan_catalog(product_code,plan_code,revision)
);

ALTER TABLE org_billing_trial_grants ENABLE ROW LEVEL SECURITY;
ALTER TABLE org_billing_trial_grants FORCE ROW LEVEL SECURITY;
CREATE POLICY org_billing_trial_grants_scope ON org_billing_trial_grants
    USING (org_id = NULLIF(current_setting('anpfuel.org_id',true),'')::uuid)
    WITH CHECK (org_id = NULLIF(current_setting('anpfuel.org_id',true),'')::uuid);
CREATE TRIGGER org_billing_trial_grants_immutable
    BEFORE UPDATE OR DELETE OR TRUNCATE ON org_billing_trial_grants
    FOR EACH STATEMENT EXECUTE FUNCTION organization_billing_keep_history();
REVOKE ALL ON org_billing_trial_grants FROM PUBLIC;

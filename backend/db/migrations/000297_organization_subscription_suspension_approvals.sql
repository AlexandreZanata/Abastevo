-- EMP-SUBS-004: an independent review binds a suspension to one past-due
-- subscription revision and one accountable operator.
CREATE TABLE org_billing_suspension_approvals (
    org_id UUID NOT NULL,
    id UUID NOT NULL CHECK (id <> '00000000-0000-0000-0000-000000000000'::uuid),
    subscription_id UUID NOT NULL,
    expected_revision BIGINT NOT NULL CHECK (expected_revision > 0),
    operator_id UUID NOT NULL CHECK (operator_id <> '00000000-0000-0000-0000-000000000000'::uuid),
    decision_ref TEXT NOT NULL CHECK (decision_ref ~ '^[!-~]{1,200}$'),
    valid_from TIMESTAMPTZ NOT NULL CHECK (isfinite(valid_from)),
    valid_until TIMESTAMPTZ NOT NULL CHECK (isfinite(valid_until) AND valid_until > valid_from),
    PRIMARY KEY (org_id,id),
    FOREIGN KEY (org_id,subscription_id,expected_revision)
        REFERENCES org_billing_subscription_events(org_id,subscription_id,revision)
);
ALTER TABLE org_billing_suspension_approvals ENABLE ROW LEVEL SECURITY;
ALTER TABLE org_billing_suspension_approvals FORCE ROW LEVEL SECURITY;
CREATE POLICY org_billing_suspension_approvals_scope ON org_billing_suspension_approvals
    USING (org_id = NULLIF(current_setting('anpfuel.org_id',true),'')::uuid)
    WITH CHECK (org_id = NULLIF(current_setting('anpfuel.org_id',true),'')::uuid);
REVOKE ALL ON org_billing_suspension_approvals FROM PUBLIC;
CREATE TRIGGER org_billing_suspension_approvals_immutable
    BEFORE UPDATE OR DELETE OR TRUNCATE ON org_billing_suspension_approvals
    FOR EACH STATEMENT EXECUTE FUNCTION org_billing_subscription_events_immutable();

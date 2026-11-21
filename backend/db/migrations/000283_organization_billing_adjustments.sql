-- EMP-BACK-006: reviewed credits and debit corrections are separate from
-- provider settlements and subscription lifecycle events.
CREATE TABLE org_billing_adjustment_approvals (
    org_id UUID NOT NULL,
    id UUID NOT NULL CHECK (id<>'00000000-0000-0000-0000-000000000000'::uuid),
    subscription_id UUID NOT NULL,
    subscription_revision BIGINT NOT NULL,
    operator_id UUID NOT NULL CHECK (operator_id<>'00000000-0000-0000-0000-000000000000'::uuid),
    kind TEXT NOT NULL CHECK (kind IN ('SERVICE_CREDIT','DEBIT_CORRECTION')),
    amount_centavos BIGINT NOT NULL CHECK (amount_centavos BETWEEN 1 AND 100000000000),
    currency TEXT NOT NULL CHECK (currency='BRL'),
    approval_ref TEXT NOT NULL CHECK (approval_ref ~ '^[A-Za-z0-9][A-Za-z0-9._:/-]{0,199}$'),
    reason_ref TEXT NOT NULL CHECK (reason_ref ~ '^[A-Za-z0-9][A-Za-z0-9._:/-]{0,199}$'),
    valid_from TIMESTAMPTZ NOT NULL CHECK (isfinite(valid_from)),
    valid_until TIMESTAMPTZ NOT NULL CHECK (isfinite(valid_until) AND valid_until>valid_from),
    PRIMARY KEY (org_id,id),
    UNIQUE (org_id,approval_ref),
    FOREIGN KEY (org_id,subscription_id,subscription_revision)
        REFERENCES org_billing_subscription_events(org_id,subscription_id,revision)
);
CREATE TABLE org_billing_adjustment_postings (
    org_id UUID NOT NULL,
    operation_id UUID NOT NULL,
    approval_id UUID NOT NULL,
    delta_centavos BIGINT NOT NULL CHECK (delta_centavos<>0 AND delta_centavos BETWEEN -100000000000 AND 100000000000),
    posted_at TIMESTAMPTZ NOT NULL DEFAULT clock_timestamp() CHECK (isfinite(posted_at)),
    PRIMARY KEY (org_id,operation_id),
    UNIQUE (org_id,approval_id),
    FOREIGN KEY (org_id,approval_id) REFERENCES org_billing_adjustment_approvals(org_id,id),
    FOREIGN KEY (org_id,operation_id) REFERENCES org_audit_events(org_id,operation_id) DEFERRABLE INITIALLY DEFERRED
);
DO $$
DECLARE relation TEXT;
BEGIN
    FOREACH relation IN ARRAY ARRAY['org_billing_adjustment_approvals','org_billing_adjustment_postings'] LOOP
        EXECUTE format('ALTER TABLE %I ENABLE ROW LEVEL SECURITY',relation);
        EXECUTE format('ALTER TABLE %I FORCE ROW LEVEL SECURITY',relation);
        EXECUTE format('CREATE POLICY %I ON %I USING (org_id=NULLIF(current_setting(''anpfuel.org_id'',true),'''')::uuid) WITH CHECK (org_id=NULLIF(current_setting(''anpfuel.org_id'',true),'''')::uuid)',
            relation||'_scope',relation);
        EXECUTE format('CREATE TRIGGER %I BEFORE UPDATE OR DELETE OR TRUNCATE ON %I FOR EACH STATEMENT EXECUTE FUNCTION org_billing_subscription_events_immutable()',
            relation||'_retained',relation);
        EXECUTE format('REVOKE ALL ON %I FROM PUBLIC',relation);
    END LOOP;
END $$;

CREATE FUNCTION org_billing_adjustment_posting_proof() RETURNS trigger
LANGUAGE plpgsql SET search_path=pg_catalog AS $$
BEGIN
    NEW.posted_at=clock_timestamp();
    IF NOT EXISTS (
        SELECT 1 FROM public.org_billing_adjustment_approvals a
        JOIN public.org_audit_events audit ON (audit.org_id,audit.operation_id)=(NEW.org_id,NEW.operation_id)
        WHERE (a.org_id,a.id)=(NEW.org_id,NEW.approval_id)
          AND a.valid_from<=NEW.posted_at AND NEW.posted_at<a.valid_until
          AND (audit.actor_id,audit.target_id)=(a.operator_id,a.subscription_id)
          AND audit.action='subs.adjustment.posted' AND audit.target_type='subscription'
          AND audit.reason_code=lower(a.kind)
          AND NEW.delta_centavos=(CASE a.kind WHEN 'SERVICE_CREDIT' THEN -a.amount_centavos ELSE a.amount_centavos END)
    ) THEN
        RAISE EXCEPTION USING ERRCODE='23514',MESSAGE='billing adjustment proof unavailable';
    END IF;
    RETURN NEW;
END $$;
CREATE TRIGGER org_billing_adjustment_posting_proof BEFORE INSERT ON org_billing_adjustment_postings
    FOR EACH ROW EXECUTE FUNCTION org_billing_adjustment_posting_proof();

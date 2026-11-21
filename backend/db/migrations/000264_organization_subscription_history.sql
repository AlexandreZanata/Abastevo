-- EMP-SUBS-002: Private append-only lifecycle history. The newest revision
-- is the current state. The revision key resolves competing events without a
-- mutable projection, and the catalog FK pins the terms used by each revision.
CREATE TABLE org_billing_subscription_events (
    org_id UUID NOT NULL REFERENCES organizations(org_id),
    subscription_id UUID NOT NULL CHECK (subscription_id <> '00000000-0000-0000-0000-000000000000'::uuid),
    revision BIGINT NOT NULL CHECK (revision > 0),
    product_code TEXT NOT NULL,
    plan_code TEXT NOT NULL,
    plan_revision BIGINT NOT NULL,
    from_state TEXT NOT NULL CHECK (from_state IN ('','TRIALING','ACTIVE','PAST_DUE','SUSPENDED','CANCELLING')),
    state TEXT NOT NULL CHECK (state IN ('TRIALING','ACTIVE','PAST_DUE','SUSPENDED','CANCELLING','CANCELLED')),
    prior_state TEXT NOT NULL,
    event_kind TEXT NOT NULL CHECK (event_kind IN ('START_TRIAL','CONFIRM_SETTLEMENT','REPORT_PAST_DUE','SUSPEND_ACCESS',
        'RESTORE_ACCESS','SCHEDULE_CANCELLATION','CONFIRM_CANCELLATION','FINISH_TERM')),
    event_source TEXT NOT NULL CHECK (event_source IN ('TRIAL_POLICY','VERIFIED_BILLING','REVIEWED_OPERATOR','SERVER_CLOCK')),
    evidence_reference TEXT NOT NULL CHECK (octet_length(evidence_reference) BETWEEN 1 AND 200 AND evidence_reference ~ '^[!-~]+$'),
    occurred_at TIMESTAMPTZ NOT NULL CHECK (isfinite(occurred_at)),
    term_start TIMESTAMPTZ NOT NULL CHECK (isfinite(term_start)),
    term_end TIMESTAMPTZ NOT NULL CHECK (isfinite(term_end) AND term_end > term_start),
    recorded_at TIMESTAMPTZ NOT NULL DEFAULT clock_timestamp() CHECK (isfinite(recorded_at)),
    PRIMARY KEY (org_id, subscription_id, revision),
    FOREIGN KEY (product_code, plan_code, plan_revision)
        REFERENCES enterprise_plan_catalog(product_code, plan_code, revision),
    CHECK ((revision = 1 AND from_state = '' AND state IN ('TRIALING','ACTIVE')) OR (revision > 1 AND from_state <> '')),
    CHECK ((state = 'SUSPENDED' AND prior_state IN ('TRIALING','ACTIVE','PAST_DUE')) OR
           (state = 'CANCELLING' AND prior_state IN ('TRIALING','ACTIVE','PAST_DUE','SUSPENDED')) OR
           (state NOT IN ('SUSPENDED','CANCELLING') AND prior_state = '')),
    CHECK (occurred_at >= term_start)
);

ALTER TABLE org_billing_subscription_events ENABLE ROW LEVEL SECURITY;
ALTER TABLE org_billing_subscription_events FORCE ROW LEVEL SECURITY;
CREATE POLICY org_billing_subscription_events_scope ON org_billing_subscription_events
    USING (org_id = NULLIF(current_setting('anpfuel.org_id', true), '')::uuid)
    WITH CHECK (org_id = NULLIF(current_setting('anpfuel.org_id', true), '')::uuid);
REVOKE ALL ON org_billing_subscription_events FROM PUBLIC;

CREATE FUNCTION org_billing_subscription_events_guard() RETURNS trigger
LANGUAGE plpgsql SET search_path = pg_catalog AS $$
BEGIN
    IF NEW.revision > 1 AND NOT EXISTS (
        SELECT 1 FROM public.org_billing_subscription_events previous
        WHERE previous.org_id = NEW.org_id AND previous.subscription_id = NEW.subscription_id
          AND previous.revision = NEW.revision - 1 AND previous.state = NEW.from_state
          AND previous.product_code = NEW.product_code AND previous.occurred_at <= NEW.occurred_at
    ) THEN
        RAISE EXCEPTION USING ERRCODE = '23514', MESSAGE = 'invalid subscription predecessor';
    END IF;
    RETURN NEW;
END $$;
CREATE TRIGGER org_billing_subscription_events_predecessor
    BEFORE INSERT ON org_billing_subscription_events
    FOR EACH ROW EXECUTE FUNCTION org_billing_subscription_events_guard();

CREATE FUNCTION org_billing_subscription_events_immutable() RETURNS trigger
LANGUAGE plpgsql SET search_path = pg_catalog AS $$
BEGIN
    RAISE EXCEPTION USING ERRCODE = '55000', MESSAGE = 'subscription history is immutable';
END $$;
CREATE TRIGGER org_billing_subscription_events_immutable
    BEFORE UPDATE OR DELETE OR TRUNCATE ON org_billing_subscription_events
    FOR EACH STATEMENT EXECUTE FUNCTION org_billing_subscription_events_immutable();

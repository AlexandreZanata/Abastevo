-- EMP-SUBS-006: exact reviewed plan revisions can replace the current plan
-- when the next term is funded. The prior ledger row remains available.
CREATE OR REPLACE FUNCTION org_billing_subscription_events_guard() RETURNS trigger
LANGUAGE plpgsql SET search_path = pg_catalog AS $$
DECLARE prior public.org_billing_subscription_events%ROWTYPE;
BEGIN
    IF NEW.revision = 1 THEN
        RETURN NEW;
    END IF;
    SELECT * INTO prior FROM public.org_billing_subscription_events e
    WHERE (e.org_id,e.subscription_id,e.revision) = (NEW.org_id,NEW.subscription_id,NEW.revision - 1);
    IF NOT FOUND OR prior.state <> NEW.from_state OR prior.product_code <> NEW.product_code OR prior.occurred_at > NEW.occurred_at THEN
        RAISE EXCEPTION USING ERRCODE = '23514', MESSAGE = 'invalid subscription predecessor';
    END IF;
    IF (prior.plan_code,prior.plan_revision) IS DISTINCT FROM (NEW.plan_code,NEW.plan_revision)
      AND (NEW.event_kind <> 'CONFIRM_SETTLEMENT' OR NEW.event_source <> 'VERIFIED_BILLING'
           OR NEW.state <> 'ACTIVE' OR prior.state NOT IN ('TRIALING','ACTIVE','PAST_DUE')
           OR NEW.term_start <> prior.term_end OR NEW.occurred_at >= NEW.term_end) THEN
        RAISE EXCEPTION USING ERRCODE = '23514', MESSAGE = 'invalid subscription plan change';
    END IF;
    RETURN NEW;
END $$;

CREATE FUNCTION org_billing_plan_change_receipt() RETURNS trigger
LANGUAGE plpgsql SET search_path = pg_catalog AS $$
BEGIN
    IF EXISTS (
        SELECT 1 FROM public.org_billing_subscription_events prior
        WHERE (prior.org_id,prior.subscription_id,prior.revision) = (NEW.org_id,NEW.subscription_id,NEW.revision - 1)
          AND (prior.plan_code,prior.plan_revision) = (NEW.plan_code,NEW.plan_revision)
    ) THEN
        RETURN NULL;
    END IF;
    IF NOT EXISTS (
        SELECT 1
        FROM public.org_billing_settlement_receipts r
        JOIN public.org_billing_charge_bindings b
          ON (b.org_id,b.provider,b.environment,b.merchant,b.resource_ref,b.subscription_id) =
             (r.org_id,r.provider,r.environment,r.merchant,r.resource_ref,r.subscription_id)
        JOIN public.enterprise_billing_offers o ON o.id = b.offer_id
        WHERE (r.org_id,r.subscription_id,r.applied_revision) = (NEW.org_id,NEW.subscription_id,NEW.revision)
          AND (o.product_code,o.plan_code,o.plan_revision,b.term_start,b.term_end) =
              (NEW.product_code,NEW.plan_code,NEW.plan_revision,NEW.term_start,NEW.term_end)
    ) THEN
        RAISE EXCEPTION USING ERRCODE = '23514', MESSAGE = 'subscription plan change receipt unavailable';
    END IF;
    RETURN NULL;
END $$;

CREATE CONSTRAINT TRIGGER org_billing_plan_change_receipt
    AFTER INSERT ON org_billing_subscription_events DEFERRABLE INITIALLY DEFERRED
    FOR EACH ROW WHEN (NEW.revision > 1 AND NEW.event_kind = 'CONFIRM_SETTLEMENT')
    EXECUTE FUNCTION org_billing_plan_change_receipt();

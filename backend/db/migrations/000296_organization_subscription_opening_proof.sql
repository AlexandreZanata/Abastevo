-- EMP-SUBS-004 / EMP-SUBS-007: new opening rows must carry the scoped proof
-- even when written directly. Keep the terms of all preexisting history.
CREATE FUNCTION org_billing_require_opening_proof() RETURNS trigger
LANGUAGE plpgsql SET search_path = pg_catalog AS $$
BEGIN
    IF NEW.state = 'ACTIVE' AND NEW.event_kind = 'CONFIRM_SETTLEMENT'
       AND NEW.event_source = 'VERIFIED_BILLING' AND EXISTS (
        SELECT 1 FROM public.org_billing_settlement_receipts r
        JOIN public.org_billing_charge_bindings b
          ON (b.org_id,b.provider,b.environment,b.merchant,b.resource_ref,b.subscription_id) =
             (r.org_id,r.provider,r.environment,r.merchant,r.resource_ref,r.subscription_id)
        JOIN public.enterprise_billing_offers o ON o.id = b.offer_id
        WHERE (r.org_id,r.subscription_id,r.applied_revision) = (NEW.org_id,NEW.subscription_id,NEW.revision)
          AND (o.product_code,o.plan_code,o.plan_revision,b.term_start,b.term_end) =
              (NEW.product_code,NEW.plan_code,NEW.plan_revision,NEW.term_start,NEW.term_end)
          AND GREATEST(r.settled_at,b.term_start) = NEW.occurred_at
          AND NEW.occurred_at < NEW.term_end
    ) THEN
        RETURN NULL;
    END IF;
    IF NEW.state = 'TRIALING' AND NEW.event_kind = 'START_TRIAL'
       AND NEW.event_source = 'TRIAL_POLICY' AND EXISTS (
        SELECT 1 FROM public.org_billing_trial_grants g
        JOIN public.org_audit_events a
          ON (a.org_id,a.target_id) = (g.org_id,g.id)
        WHERE (g.org_id,g.id,g.product_code,g.plan_code,g.plan_revision,g.approval_ref) =
              (NEW.org_id,NEW.subscription_id,NEW.product_code,NEW.plan_code,NEW.plan_revision,NEW.evidence_reference)
          AND a.action = 'billing.trial.started' AND a.target_type = 'subscription'
          AND a.request_hash = pg_catalog.encode(pg_catalog.sha256(
              pg_catalog.convert_to(NEW.org_id::text || ':' || g.id::text, 'UTF8')), 'hex')
          AND NEW.occurred_at >= g.eligible_from AND NEW.occurred_at < g.eligible_until
          AND NEW.occurred_at < NEW.term_start + interval '1 second'
          AND NEW.term_end = NEW.term_start + g.duration_seconds * interval '1 second'
    ) THEN
        RETURN NULL;
    END IF;
    RAISE EXCEPTION USING ERRCODE = '23514', MESSAGE = 'subscription opening proof unavailable';
END $$;

CREATE CONSTRAINT TRIGGER org_billing_subscription_opening_proof
    AFTER INSERT ON org_billing_subscription_events DEFERRABLE INITIALLY DEFERRED
    FOR EACH ROW WHEN (NEW.revision = 1)
    EXECUTE FUNCTION org_billing_require_opening_proof();

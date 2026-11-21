-- EMP-SUBS-005: Retain an exact, immutable subscription witness for the
-- existing offline review path. Install no new allowance or historical event.
CREATE FUNCTION org_sync_subscription_entitled_at(
    organization UUID, capability_code TEXT, instant TIMESTAMPTZ
) RETURNS TABLE(subscription_id UUID, subscription_revision BIGINT)
LANGUAGE sql STABLE SET search_path = pg_catalog AS $$
    WITH current_at_instant AS (
        SELECT DISTINCT ON (s.subscription_id) s.*
        FROM public.org_billing_subscription_events s
        WHERE s.org_id = organization AND s.occurred_at <= instant AND s.recorded_at <= instant
        ORDER BY s.subscription_id, s.revision DESC
    )
    SELECT s.subscription_id, s.revision
    FROM current_at_instant s
    JOIN public.enterprise_plan_catalog p
      ON (p.product_code,p.plan_code,p.revision) = (s.product_code,s.plan_code,s.plan_revision)
    WHERE s.term_start <= instant AND instant < s.term_end
      AND (s.state IN ('TRIALING','ACTIVE') OR (s.state = 'CANCELLING' AND s.prior_state IN ('TRIALING','ACTIVE')))
      AND capability_code = ANY(p.capabilities)
    ORDER BY s.subscription_id
    LIMIT 1
$$;

ALTER TABLE org_sync_state_intakes
    ADD COLUMN subscription_id UUID,
    ADD COLUMN subscription_revision BIGINT,
    ADD COLUMN pilot_capability TEXT GENERATED ALWAYS AS
        (CASE WHEN subscription_id IS NULL THEN capability END) STORED,
    DROP CONSTRAINT org_sync_state_intakes_org_id_capability_fkey,
    DROP CONSTRAINT org_sync_state_intakes_check,
    ADD CONSTRAINT org_sync_state_intakes_pilot_capability_fkey FOREIGN KEY (org_id,pilot_capability)
        REFERENCES org_capabilities(org_id,capability),
    ADD CONSTRAINT org_sync_state_intakes_subscription_fkey FOREIGN KEY (org_id,subscription_id,subscription_revision)
        REFERENCES org_billing_subscription_events(org_id,subscription_id,revision),
    ADD CONSTRAINT org_sync_state_intakes_provenance CHECK (
      (condition = 'SUSPENSION' AND capability IS NULL AND capability_revision IS NULL AND plan_changed_at IS NULL
       AND subscription_id IS NULL AND subscription_revision IS NULL
       AND ((disposition = 'HOLD' AND ((result = 'ACCEPTED_FOR_REVIEW' AND code = 'ORG_SUSPENDED_GRACE')
                                      OR (result = 'REJECTED' AND code = 'MALFORMED')))
         OR (disposition = 'REFUSE' AND result = 'REJECTED' AND code = 'ORG_SUSPENDED')))
      OR
      (condition = 'PLAN' AND capability IS NOT NULL AND disposition = 'HOLD'
       AND ((result = 'ACCEPTED_FOR_REVIEW' AND code = 'PLAN_CHANGED') OR (result = 'REJECTED' AND code = 'MALFORMED'))
       AND ((subscription_id IS NULL AND subscription_revision IS NULL
             AND capability_revision IS NOT NULL AND capability_revision > 1 AND plan_changed_at IS NOT NULL)
         OR (subscription_id IS NOT NULL AND subscription_revision IS NOT NULL
             AND capability_revision IS NULL AND plan_changed_at IS NULL AND declared_at < received_at)))
    );

CREATE OR REPLACE FUNCTION org_sync_state_intake_source() RETURNS trigger LANGUAGE plpgsql SET search_path=pg_catalog AS $$
DECLARE o public.org_sync_operations%ROWTYPE;suspension TEXT;c public.org_capabilities%ROWTYPE;
BEGIN
 IF NEW.org_id IS DISTINCT FROM NULLIF(current_setting('anpfuel.org_id',true),'')::uuid THEN RAISE EXCEPTION USING ERRCODE='42501',MESSAGE='organization scope denied';END IF;
 IF NEW.subscription_id IS NOT NULL THEN
  PERFORM pg_advisory_xact_lock_shared(hashtextextended('organization-entitlement:' || NEW.org_id::text,0));
 END IF;
 -- Serialize with operator suspension changes and capability changes.
 PERFORM 1 FROM public.organizations WHERE org_id=NEW.org_id AND has_snapshot FOR UPDATE;
 SELECT * INTO o FROM public.org_sync_operations WHERE org_id=NEW.org_id AND operation_id=NEW.operation_id;
 IF NOT FOUND OR NOT EXISTS(SELECT 1 FROM public.org_device_enrollments e JOIN public.org_memberships p ON(p.org_id,p.id)=(e.org_id,e.membership_id)
  WHERE e.org_id=o.org_id AND e.id=o.enrollment_id AND e.membership_id=o.membership_id AND e.state='ENROLLED' AND p.state='ACTIVE' AND p.account_id=o.owner_account_id)
 THEN RAISE EXCEPTION USING ERRCODE='23514',MESSAGE='state intake source unavailable';END IF;
 IF NEW.received_at>clock_timestamp() THEN RAISE EXCEPTION USING ERRCODE='23514',MESSAGE='state intake receipt unavailable';END IF;
 suspension=public.org_sync_suspension_disposition(NEW.org_id,NEW.declared_at,NEW.received_at);
 IF NEW.condition='SUSPENSION' THEN
  IF suspension IS DISTINCT FROM (CASE NEW.disposition WHEN 'HOLD' THEN 'REVIEW' ELSE 'REFUSE' END) THEN RAISE EXCEPTION USING ERRCODE='23514',MESSAGE='state intake decision unavailable';END IF;
 ELSIF NEW.subscription_id IS NOT NULL THEN
  IF suspension IS DISTINCT FROM 'ALLOW'
   OR EXISTS (SELECT 1 FROM public.org_capabilities
              WHERE org_id=NEW.org_id AND capability=NEW.capability AND NOT enabled)
   OR NOT EXISTS (SELECT 1 FROM public.org_sync_subscription_entitled_at(NEW.org_id,NEW.capability,NEW.declared_at) prior
                  WHERE (prior.subscription_id,prior.subscription_revision)=(NEW.subscription_id,NEW.subscription_revision))
   OR EXISTS (SELECT 1 FROM public.org_sync_subscription_entitled_at(NEW.org_id,NEW.capability,NEW.received_at))
  THEN RAISE EXCEPTION USING ERRCODE='23514',MESSAGE='subscription state intake decision unavailable';END IF;
 ELSE
  SELECT * INTO c FROM public.org_capabilities WHERE org_id=NEW.org_id AND capability=NEW.capability;
  IF suspension IS DISTINCT FROM 'ALLOW' OR NOT FOUND OR c.revision<>NEW.capability_revision OR c.updated_at IS DISTINCT FROM NEW.plan_changed_at
   OR public.org_sync_plan_disposition((CASE WHEN c.enabled THEN 'ENABLED' ELSE 'DISABLED' END),c.revision,c.updated_at,NEW.declared_at,NEW.received_at) IS DISTINCT FROM 'HOLD'
  THEN RAISE EXCEPTION USING ERRCODE='23514',MESSAGE='state intake decision unavailable';END IF;
 END IF;
 RETURN NEW;
END $$;

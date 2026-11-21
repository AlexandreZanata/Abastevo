-- JRN-11: bind a late signed fueling to the exact vehicle plan archive and
-- the actual predecessor entitlement in force when its author declared it.
CREATE FUNCTION org_sync_vehicle_plan_archive_at(
    target_org UUID, target_asset UUID, declared TIMESTAMPTZ, received TIMESTAMPTZ
) RETURNS TABLE(selection_id UUID, subscription_id UUID, subscription_revision BIGINT)
LANGUAGE SQL STABLE SET search_path=pg_catalog AS $$
    SELECT selection.operation_id, selection.subscription_id,
        COALESCE(CASE WHEN prior.term_end=lower_term.term_start AND prior.term_start<=declared AND declared<prior.term_end
              AND (prior.state IN ('ACTIVE','TRIALING') OR (prior.state='CANCELLING' AND prior.prior_state IN ('ACTIVE','TRIALING')))
              AND 'organizations'=ANY(plan.capabilities) THEN prior.revision END,0)::bigint
    FROM public.org_billing_vehicle_plan_archives archive
    JOIN public.org_billing_vehicle_plan_selections selection ON
        (selection.org_id,selection.operation_id)=(archive.org_id,archive.selection_id)
    JOIN public.org_billing_subscription_events lower_term ON
        (lower_term.org_id,lower_term.subscription_id,lower_term.revision) =
        (selection.org_id,selection.subscription_id,selection.subscription_revision)
    JOIN public.org_assets asset ON (asset.org_id,asset.id)=(archive.org_id,archive.asset_id)
    LEFT JOIN LATERAL (
        SELECT event.* FROM public.org_billing_subscription_events event
        WHERE (event.org_id,event.subscription_id)=(selection.org_id,selection.subscription_id)
          AND event.revision<selection.subscription_revision AND event.occurred_at<=declared AND event.recorded_at<=declared
        ORDER BY event.revision DESC LIMIT 1
    ) prior ON true
    LEFT JOIN public.enterprise_plan_catalog plan
        ON (plan.product_code,plan.plan_code,plan.revision)=(prior.product_code,prior.plan_code,prior.plan_revision)
    WHERE (archive.org_id,archive.asset_id)=(target_org,target_asset)
      AND selection.recorded_at<=received AND asset.kind='VEHICLE' AND asset.state='INACTIVE'
      AND NOT EXISTS (
          SELECT 1 FROM public.org_asset_lifecycle_events newer
          WHERE (newer.org_id,newer.asset_id)=(archive.org_id,archive.asset_id) AND newer.revision>archive.asset_revision
      )
$$;
REVOKE ALL ON FUNCTION org_sync_vehicle_plan_archive_at(UUID,UUID,TIMESTAMPTZ,TIMESTAMPTZ) FROM PUBLIC;

ALTER TABLE org_sync_state_intakes
    ADD COLUMN vehicle_plan_selection_id UUID,
    ADD COLUMN vehicle_plan_asset_id UUID,
    ADD CONSTRAINT org_sync_state_intakes_vehicle_plan_archive_fkey
        FOREIGN KEY (org_id,vehicle_plan_selection_id,vehicle_plan_asset_id)
        REFERENCES org_billing_vehicle_plan_archives(org_id,selection_id,asset_id),
    ADD CONSTRAINT org_sync_state_intakes_vehicle_plan_shape CHECK (
        (vehicle_plan_selection_id IS NULL AND vehicle_plan_asset_id IS NULL)
        OR (vehicle_plan_selection_id IS NOT NULL AND vehicle_plan_asset_id IS NOT NULL
            AND condition='PLAN' AND capability='organizations' AND subscription_id IS NOT NULL)
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
 ELSIF NEW.vehicle_plan_selection_id IS NOT NULL THEN
  IF suspension IS DISTINCT FROM 'ALLOW'
   OR o.operation_type<>'fueling.submit' OR o.operation_schema<>1 OR o.entity_type<>'fueling'
   OR EXISTS (SELECT 1 FROM public.org_capabilities
              WHERE org_id=NEW.org_id AND capability=NEW.capability AND NOT enabled)
   OR NOT EXISTS (SELECT 1 FROM public.org_sync_vehicle_plan_archive_at(
        NEW.org_id,NEW.vehicle_plan_asset_id,NEW.declared_at,NEW.received_at) proof
        WHERE (proof.selection_id,proof.subscription_id,proof.subscription_revision)=
              (NEW.vehicle_plan_selection_id,NEW.subscription_id,NEW.subscription_revision))
   OR (NEW.result='ACCEPTED_FOR_REVIEW' AND NOT EXISTS (
       SELECT 1 FROM public.org_held_fuelings held WHERE
        (held.org_id,held.operation_id,held.fueling_id,held.asset_id)=
        (NEW.org_id,NEW.operation_id,o.entity_id,NEW.vehicle_plan_asset_id)))
  THEN RAISE EXCEPTION USING ERRCODE='23514',MESSAGE='vehicle plan intake decision unavailable';END IF;
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

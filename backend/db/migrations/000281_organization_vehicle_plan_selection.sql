-- EMP-SUBS-006: exact keep-active choices after a verified lower paid term.
-- A plan archive preserves the normal reversible fleet deactivation.
CREATE FUNCTION org_billing_vehicle_downgrade(target_org UUID, target_subscription UUID, target_revision BIGINT, evaluated_at TIMESTAMPTZ)
RETURNS TABLE(ceiling BIGINT, term_start TIMESTAMPTZ, previous_start TIMESTAMPTZ)
LANGUAGE SQL STABLE SET search_path=pg_catalog AS $$
    SELECT current_plan.active_vehicles, current_term.term_start, previous.term_start
    FROM public.org_billing_subscription_events current_term
    JOIN public.org_billing_subscription_events previous
      ON (previous.org_id,previous.subscription_id,previous.revision) =
         (current_term.org_id,current_term.subscription_id,current_term.revision-1)
    JOIN public.enterprise_plan_catalog current_plan
      ON (current_plan.product_code,current_plan.plan_code,current_plan.revision) =
         (current_term.product_code,current_term.plan_code,current_term.plan_revision)
    JOIN public.enterprise_plan_catalog previous_plan
      ON (previous_plan.product_code,previous_plan.plan_code,previous_plan.revision) =
         (previous.product_code,previous.plan_code,previous.plan_revision)
    WHERE (current_term.org_id,current_term.subscription_id,current_term.revision)=(target_org,target_subscription,target_revision)
      AND current_term.state='ACTIVE' AND current_term.event_kind='CONFIRM_SETTLEMENT' AND current_term.event_source='VERIFIED_BILLING'
      AND previous.state IN ('ACTIVE','PAST_DUE','TRIALING') AND previous.term_end=current_term.term_start
      AND current_plan.active_vehicles < previous_plan.active_vehicles
      AND 'organizations'=ANY(current_plan.capabilities) AND 'organizations'=ANY(previous_plan.capabilities)
      AND evaluated_at>=current_term.term_start AND evaluated_at<current_term.term_end
      AND NOT EXISTS (
          SELECT 1 FROM public.org_billing_subscription_events newer
          WHERE (newer.org_id,newer.subscription_id)=(current_term.org_id,current_term.subscription_id)
            AND newer.revision>current_term.revision
      )
$$;
REVOKE ALL ON FUNCTION org_billing_vehicle_downgrade(UUID,UUID,BIGINT,TIMESTAMPTZ) FROM PUBLIC;

CREATE TABLE org_billing_vehicle_plan_selections (
    org_id UUID NOT NULL,
    operation_id UUID NOT NULL,
    subscription_id UUID NOT NULL,
    subscription_revision BIGINT NOT NULL,
    kept UUID[] NOT NULL CHECK (array_position(kept,NULL) IS NULL),
    archived UUID[] NOT NULL CHECK (cardinality(archived)>0 AND array_position(archived,NULL) IS NULL),
    recorded_at TIMESTAMPTZ NOT NULL DEFAULT clock_timestamp() CHECK (isfinite(recorded_at)),
    PRIMARY KEY (org_id,operation_id),
    UNIQUE (org_id,subscription_id,subscription_revision),
    FOREIGN KEY (org_id,subscription_id,subscription_revision)
        REFERENCES org_billing_subscription_events(org_id,subscription_id,revision),
    FOREIGN KEY (org_id,operation_id)
        REFERENCES org_audit_events(org_id,operation_id) DEFERRABLE INITIALLY DEFERRED
);

CREATE TABLE org_billing_vehicle_plan_archives (
    org_id UUID NOT NULL,
    selection_id UUID NOT NULL,
    asset_id UUID NOT NULL,
    asset_revision BIGINT NOT NULL,
    PRIMARY KEY (org_id,selection_id,asset_id),
    UNIQUE (org_id,asset_id,asset_revision),
    FOREIGN KEY (org_id,selection_id) REFERENCES org_billing_vehicle_plan_selections(org_id,operation_id),
    FOREIGN KEY (org_id,asset_id,asset_revision)
        REFERENCES org_asset_lifecycle_events(org_id,asset_id,revision)
);

DO $$
DECLARE relation TEXT;
BEGIN
    FOREACH relation IN ARRAY ARRAY['org_billing_vehicle_plan_selections','org_billing_vehicle_plan_archives'] LOOP
        EXECUTE format('ALTER TABLE %I ENABLE ROW LEVEL SECURITY',relation);
        EXECUTE format('ALTER TABLE %I FORCE ROW LEVEL SECURITY',relation);
        EXECUTE format('CREATE POLICY %I ON %I USING (org_id=NULLIF(current_setting(''anpfuel.org_id'',true),'''')::uuid) WITH CHECK (org_id=NULLIF(current_setting(''anpfuel.org_id'',true),'''')::uuid)',
            relation||'_scope',relation);
        EXECUTE format('CREATE TRIGGER %I BEFORE UPDATE OR DELETE OR TRUNCATE ON %I FOR EACH STATEMENT EXECUTE FUNCTION org_billing_subscription_events_immutable()',
            relation||'_retained',relation);
        EXECUTE format('REVOKE ALL ON %I FROM PUBLIC',relation);
    END LOOP;
END $$;

CREATE FUNCTION org_vehicle_plan_selection_proof() RETURNS trigger
LANGUAGE plpgsql SET search_path=pg_catalog AS $$
DECLARE expected BIGINT; current_vehicles UUID[]; expected_archives UUID[];
BEGIN
    NEW.recorded_at=clock_timestamp();
    SELECT d.ceiling INTO expected
    FROM public.org_billing_vehicle_downgrade(NEW.org_id,NEW.subscription_id,NEW.subscription_revision,NEW.recorded_at) d;
    SELECT COALESCE(array_agg(id ORDER BY id),'{}'::uuid[]) INTO current_vehicles
    FROM public.org_assets WHERE org_id=NEW.org_id AND kind='VEHICLE' AND state='ACTIVE';
    SELECT COALESCE(array_agg(id ORDER BY id),'{}'::uuid[]) INTO expected_archives
    FROM unnest(current_vehicles) AS v(id) WHERE NOT (id=ANY(NEW.kept));
    IF expected IS NULL OR cardinality(NEW.kept)<>expected OR NOT (current_vehicles @> NEW.kept)
       OR cardinality(expected_archives)<>cardinality(current_vehicles)-expected
       OR cardinality(expected_archives)=0 THEN
        RAISE EXCEPTION USING ERRCODE='23514',MESSAGE='vehicle plan selection basis unavailable';
    END IF;
    NEW.archived=expected_archives;
    RETURN NEW;
END $$;
CREATE TRIGGER org_vehicle_plan_selection_proof BEFORE INSERT ON org_billing_vehicle_plan_selections
    FOR EACH ROW EXECUTE FUNCTION org_vehicle_plan_selection_proof();

CREATE FUNCTION org_vehicle_plan_archive_proof() RETURNS trigger
LANGUAGE plpgsql SET search_path=pg_catalog AS $$
BEGIN
    IF NOT EXISTS (
        SELECT 1 FROM public.org_billing_vehicle_plan_selections s
        JOIN public.org_asset_lifecycle_events lifecycle ON (lifecycle.org_id,lifecycle.asset_id,lifecycle.revision) =
           (NEW.org_id,NEW.asset_id,NEW.asset_revision)
        JOIN public.org_assets asset ON (asset.org_id,asset.id)=(lifecycle.org_id,lifecycle.asset_id)
        JOIN public.org_audit_events selection_audit ON (selection_audit.org_id,selection_audit.operation_id)=(s.org_id,s.operation_id)
        WHERE (s.org_id,s.operation_id)=(NEW.org_id,NEW.selection_id)
          AND NEW.asset_id=ANY(s.archived) AND lifecycle.actor_id=selection_audit.actor_id
          AND lifecycle.command='DEACTIVATE' AND lifecycle.from_state='ACTIVE' AND lifecycle.to_state='INACTIVE'
          AND lifecycle.recorded_at>=s.recorded_at AND asset.kind='VEHICLE' AND asset.state='INACTIVE'
          AND asset.revision=NEW.asset_revision
    ) THEN
        RAISE EXCEPTION USING ERRCODE='23514',MESSAGE='vehicle plan archive basis unavailable';
    END IF;
    RETURN NEW;
END $$;
CREATE TRIGGER org_vehicle_plan_archive_proof BEFORE INSERT ON org_billing_vehicle_plan_archives
    FOR EACH ROW EXECUTE FUNCTION org_vehicle_plan_archive_proof();

CREATE FUNCTION org_vehicle_plan_selection_complete() RETURNS trigger
LANGUAGE plpgsql SET search_path=pg_catalog AS $$
BEGIN
    IF EXISTS (
        SELECT 1 FROM public.org_assets a WHERE a.org_id=NEW.org_id AND a.kind='VEHICLE'
          AND a.state='ACTIVE' AND NOT (a.id=ANY(NEW.kept))
    ) OR (SELECT count(*) FROM public.org_assets a WHERE a.org_id=NEW.org_id AND a.kind='VEHICLE'
          AND a.state='ACTIVE' AND a.id=ANY(NEW.kept))<>cardinality(NEW.kept)
       OR (SELECT count(*) FROM public.org_billing_vehicle_plan_archives a
           WHERE (a.org_id,a.selection_id)=(NEW.org_id,NEW.operation_id))<>cardinality(NEW.archived) THEN
        RAISE EXCEPTION USING ERRCODE='23514',MESSAGE='vehicle plan selection is incomplete';
    END IF;
    RETURN NULL;
END $$;
CREATE CONSTRAINT TRIGGER org_vehicle_plan_selection_complete AFTER INSERT ON org_billing_vehicle_plan_selections
    DEFERRABLE INITIALLY DEFERRED FOR EACH ROW EXECUTE FUNCTION org_vehicle_plan_selection_complete();

-- EMP-DATA-005: one member's watch and its admitted survey checkpoint.
CREATE TABLE org_data_watchlists (
    org_id UUID NOT NULL,
    id UUID NOT NULL CHECK (id<>'00000000-0000-0000-0000-000000000000'::uuid),
    membership_id UUID NOT NULL,
    account_id UUID NOT NULL CHECK (account_id<>'00000000-0000-0000-0000-000000000000'::uuid),
    dataset_key TEXT NOT NULL,
    fuel_product TEXT NOT NULL CHECK (fuel_product ~ '^[A-Z][A-Z0-9_]{0,47}$'),
    baseline_revision BIGINT NOT NULL CHECK (baseline_revision>0),
    last_revision BIGINT NOT NULL CHECK (last_revision>=baseline_revision),
    active BOOLEAN NOT NULL DEFAULT true,
    next_check BIGINT NOT NULL CHECK (next_check>0),
    PRIMARY KEY (org_id,id),
    FOREIGN KEY (org_id,membership_id) REFERENCES org_memberships(org_id,id),
    FOREIGN KEY (dataset_key,baseline_revision) REFERENCES enterprise_regional_series(dataset_key,dataset_revision),
    FOREIGN KEY (dataset_key,last_revision) REFERENCES enterprise_regional_series(dataset_key,dataset_revision),
    FOREIGN KEY (org_id,id) REFERENCES org_audit_events(org_id,operation_id) DEFERRABLE INITIALLY DEFERRED
);
CREATE UNIQUE INDEX org_data_watchlists_active
    ON org_data_watchlists(org_id,membership_id,dataset_key,fuel_product) WHERE active;
CREATE INDEX org_data_watchlists_member ON org_data_watchlists(org_id,membership_id,id);
ALTER TABLE org_data_watchlists ENABLE ROW LEVEL SECURITY;
ALTER TABLE org_data_watchlists FORCE ROW LEVEL SECURITY;
CREATE POLICY org_data_watchlists_scope ON org_data_watchlists
    USING (org_id=NULLIF(current_setting('anpfuel.org_id',true),'')::uuid)
    WITH CHECK (org_id=NULLIF(current_setting('anpfuel.org_id',true),'')::uuid);
CREATE TRIGGER org_data_watchlists_retained BEFORE DELETE OR TRUNCATE ON org_data_watchlists
    FOR EACH STATEMENT EXECUTE FUNCTION enterprise_plan_catalog_immutable();
REVOKE ALL ON org_data_watchlists FROM PUBLIC;

CREATE FUNCTION org_data_watchlists_proof() RETURNS trigger LANGUAGE plpgsql SET search_path=pg_catalog AS $$
BEGIN
    IF TG_OP='INSERT' THEN
        IF NOT NEW.active OR NEW.baseline_revision<>NEW.last_revision OR NOT EXISTS (
            SELECT 1 FROM public.org_memberships m WHERE (m.org_id,m.id,m.account_id)=(NEW.org_id,NEW.membership_id,NEW.account_id)
              AND m.state='ACTIVE'
        ) THEN
            RAISE EXCEPTION USING ERRCODE='23514',MESSAGE='data watch owner unavailable';
        END IF;
    ELSIF (NEW.org_id,NEW.id,NEW.membership_id,NEW.account_id,NEW.dataset_key,NEW.fuel_product,NEW.baseline_revision)
          IS DISTINCT FROM (OLD.org_id,OLD.id,OLD.membership_id,OLD.account_id,OLD.dataset_key,OLD.fuel_product,OLD.baseline_revision)
       OR NOT OLD.active OR NEW.last_revision<OLD.last_revision
       OR (NEW.active AND NEW.next_check<=OLD.next_check)
       OR (NOT NEW.active AND (NEW.next_check,NEW.last_revision)<>(OLD.next_check,OLD.last_revision)) THEN
        RAISE EXCEPTION USING ERRCODE='23514',MESSAGE='data watch checkpoint unavailable';
    END IF;
    RETURN NEW;
END $$;
CREATE TRIGGER org_data_watchlists_proof BEFORE INSERT OR UPDATE ON org_data_watchlists
    FOR EACH ROW EXECUTE FUNCTION org_data_watchlists_proof();

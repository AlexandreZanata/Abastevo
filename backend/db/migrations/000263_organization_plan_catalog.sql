-- EMP-SUBS-001: Platform-owned plan metadata. This table has no organization
-- rows, member roles, account tiers, providers or prices. Every revision is
-- an immutable snapshot. There is deliberately no production catalog seed.
CREATE TABLE enterprise_plan_catalog (
    product_code TEXT NOT NULL CHECK (octet_length(product_code) <= 64 AND product_code ~ '^[a-z][a-z0-9]*(-[a-z0-9]+)*$'),
    plan_code TEXT NOT NULL CHECK (octet_length(plan_code) <= 64 AND plan_code ~ '^[a-z][a-z0-9]*(-[a-z0-9]+)*$'),
    revision BIGINT NOT NULL CHECK (revision > 0),
    display_name TEXT NOT NULL CHECK (octet_length(display_name) BETWEEN 1 AND 120 AND display_name = btrim(display_name) AND display_name !~ '[[:cntrl:]]'),
    capabilities TEXT[] NOT NULL DEFAULT '{}' CHECK (array_position(capabilities, NULL) IS NULL),
    active_vehicles BIGINT CHECK (active_vehicles >= 0),
    active_stations BIGINT CHECK (active_stations >= 0),
    storage_bytes BIGINT CHECK (storage_bytes >= 0),
    api_calls BIGINT CHECK (api_calls >= 0),
    recorded_at TIMESTAMPTZ NOT NULL DEFAULT clock_timestamp() CHECK (isfinite(recorded_at)),
    PRIMARY KEY (product_code, plan_code, revision)
);

CREATE FUNCTION enterprise_plan_catalog_immutable() RETURNS trigger
LANGUAGE plpgsql SET search_path = pg_catalog AS $$
BEGIN
    RAISE EXCEPTION USING ERRCODE = '55000', MESSAGE = 'enterprise plan revisions are immutable';
END $$;
CREATE TRIGGER enterprise_plan_catalog_immutable
    BEFORE UPDATE OR DELETE OR TRUNCATE ON enterprise_plan_catalog
    FOR EACH STATEMENT EXECUTE FUNCTION enterprise_plan_catalog_immutable();

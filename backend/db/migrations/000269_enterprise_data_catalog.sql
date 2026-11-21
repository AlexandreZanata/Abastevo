-- EMP-DATA-001: Versioned public-input rights and regional dataset definitions.
-- Legal decisions are recorded by the platform and reevaluated on access.
CREATE TABLE enterprise_data_source_reviews (
    source_code TEXT NOT NULL CHECK (source_code IN ('S-ESP-01','S-ESP-02')),
    source_digest TEXT NOT NULL CHECK (source_digest ~ '^sha256:[a-f0-9]{64}$'),
    revision BIGINT NOT NULL CHECK (revision > 0),
    status TEXT NOT NULL CHECK (status IN ('RIGHTS_PENDING','APPROVED','WITHDRAWN')),
    decision_ref TEXT NOT NULL CHECK (octet_length(decision_ref) BETWEEN 1 AND 200 AND decision_ref ~ '^[!-~]+$'),
    license_ref TEXT NOT NULL DEFAULT '' CHECK (octet_length(license_ref) <= 300),
    attribution TEXT NOT NULL DEFAULT '' CHECK (octet_length(attribution) <= 300),
    recorded_at TIMESTAMPTZ NOT NULL DEFAULT clock_timestamp() CHECK (isfinite(recorded_at)),
    PRIMARY KEY (source_code,source_digest,revision),
    CHECK (status <> 'APPROVED' OR (btrim(license_ref) <> '' AND btrim(attribution) <> ''))
);

CREATE TABLE enterprise_regional_datasets (
    dataset_key TEXT NOT NULL CHECK (octet_length(dataset_key) BETWEEN 1 AND 64 AND dataset_key ~ '^[a-z][a-z0-9]*(-[a-z0-9]+)*$'),
    revision BIGINT NOT NULL CHECK (revision > 0),
    display_name TEXT NOT NULL CHECK (octet_length(display_name) BETWEEN 1 AND 120 AND btrim(display_name) <> ''),
    state_code TEXT NOT NULL CHECK (state_code IN ('AC','AL','AP','AM','BA','CE','DF','ES','GO','MA','MT','MS','MG','PA','PB','PR','PE','PI','RJ','RN','RS','RO','RR','SC','SP','SE','TO')),
    minimum_stations INTEGER NOT NULL CHECK (minimum_stations >= 5),
    method_version TEXT NOT NULL CHECK (method_version = 'abastevo-metrics-v1'),
    snapshot_digest TEXT NOT NULL CHECK (snapshot_digest ~ '^sha256:[a-f0-9]{64}$'),
    official_revision UUID NOT NULL CHECK (official_revision <> '00000000-0000-0000-0000-000000000000'::uuid),
    registry_source TEXT GENERATED ALWAYS AS ('S-ESP-01'::text) STORED,
    registry_digest TEXT NOT NULL,
    registry_review BIGINT NOT NULL,
    price_source TEXT GENERATED ALWAYS AS ('S-ESP-02'::text) STORED,
    price_digest TEXT NOT NULL,
    price_review BIGINT NOT NULL,
    PRIMARY KEY (dataset_key,revision),
    FOREIGN KEY (registry_source,registry_digest,registry_review)
        REFERENCES enterprise_data_source_reviews(source_code,source_digest,revision),
    FOREIGN KEY (price_source,price_digest,price_review)
        REFERENCES enterprise_data_source_reviews(source_code,source_digest,revision)
);

CREATE FUNCTION enterprise_data_keep_revision() RETURNS trigger
LANGUAGE plpgsql SET search_path = pg_catalog AS $$
BEGIN
    RAISE EXCEPTION USING ERRCODE = '55000', MESSAGE = 'enterprise Data revisions are immutable';
END $$;
CREATE TRIGGER enterprise_data_rights_immutable BEFORE UPDATE OR DELETE OR TRUNCATE ON enterprise_data_source_reviews
    FOR EACH STATEMENT EXECUTE FUNCTION enterprise_data_keep_revision();
CREATE TRIGGER enterprise_regional_datasets_immutable BEFORE UPDATE OR DELETE OR TRUNCATE ON enterprise_regional_datasets
    FOR EACH STATEMENT EXECUTE FUNCTION enterprise_data_keep_revision();
REVOKE ALL ON enterprise_data_source_reviews, enterprise_regional_datasets FROM PUBLIC;

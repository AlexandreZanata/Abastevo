-- EMP-DATA-004/007: reviewed plan scope and official-only materialized summaries.
CREATE TABLE enterprise_data_plan_datasets (
    product_code TEXT NOT NULL,
    plan_code TEXT NOT NULL,
    plan_revision BIGINT NOT NULL,
    dataset_key TEXT NOT NULL,
    dataset_revision BIGINT NOT NULL,
    approval_ref TEXT NOT NULL CHECK (octet_length(approval_ref) BETWEEN 1 AND 200 AND approval_ref ~ '^[!-~]+$'),
    PRIMARY KEY (product_code,plan_code,plan_revision,dataset_key,dataset_revision),
    FOREIGN KEY (product_code,plan_code,plan_revision) REFERENCES enterprise_plan_catalog(product_code,plan_code,revision),
    FOREIGN KEY (dataset_key,dataset_revision) REFERENCES enterprise_regional_datasets(dataset_key,revision)
);

CREATE TABLE enterprise_regional_series (
    dataset_key TEXT NOT NULL,
    dataset_revision BIGINT NOT NULL,
    survey_start DATE NOT NULL CHECK (isfinite(survey_start)),
    survey_end DATE NOT NULL CHECK (isfinite(survey_end) AND survey_end >= survey_start),
    published_at TIMESTAMPTZ NOT NULL CHECK (isfinite(published_at)),
    source_url TEXT NOT NULL CHECK (octet_length(source_url) <= 2048 AND source_url LIKE 'https://%'),
    result_sha256 TEXT NOT NULL CHECK (result_sha256 ~ '^[a-f0-9]{64}$'),
    PRIMARY KEY (dataset_key,dataset_revision),
    FOREIGN KEY (dataset_key,dataset_revision) REFERENCES enterprise_regional_datasets(dataset_key,revision)
);

CREATE TABLE enterprise_regional_series_samples (
    dataset_key TEXT NOT NULL,
    dataset_revision BIGINT NOT NULL,
    fuel_product TEXT NOT NULL CHECK (fuel_product IN ('ETHANOL','GASOLINE_REGULAR','GASOLINE_ADDITIVED','GASOLINE_PREMIUM_GRADE','DIESEL_S500','DIESEL_S10','CNG','LPG_P13')),
    unit_code TEXT NOT NULL CHECK ((fuel_product='CNG' AND unit_code='M3')
        OR (fuel_product='LPG_P13' AND unit_code='KG_13')
        OR (fuel_product NOT IN ('CNG','LPG_P13') AND unit_code='L')),
    stations INTEGER NOT NULL CHECK (stations >= 5),
    observations INTEGER NOT NULL CHECK (observations >= stations),
    sum_milli_brl BIGINT NOT NULL,
    minimum_milli_brl BIGINT NOT NULL CHECK (minimum_milli_brl BETWEEN 1 AND 1000000),
    maximum_milli_brl BIGINT NOT NULL CHECK (maximum_milli_brl BETWEEN minimum_milli_brl AND 1000000),
    PRIMARY KEY (dataset_key,dataset_revision,fuel_product),
    FOREIGN KEY (dataset_key,dataset_revision) REFERENCES enterprise_regional_series(dataset_key,dataset_revision),
    CHECK (sum_milli_brl BETWEEN minimum_milli_brl*stations AND maximum_milli_brl*stations)
);

CREATE TRIGGER enterprise_data_plan_datasets_immutable BEFORE UPDATE OR DELETE OR TRUNCATE ON enterprise_data_plan_datasets
    FOR EACH STATEMENT EXECUTE FUNCTION enterprise_data_keep_revision();
CREATE TRIGGER enterprise_regional_series_immutable BEFORE UPDATE OR DELETE OR TRUNCATE ON enterprise_regional_series
    FOR EACH STATEMENT EXECUTE FUNCTION enterprise_data_keep_revision();
CREATE TRIGGER enterprise_regional_series_samples_immutable BEFORE UPDATE OR DELETE OR TRUNCATE ON enterprise_regional_series_samples
    FOR EACH STATEMENT EXECUTE FUNCTION enterprise_data_keep_revision();

REVOKE ALL ON enterprise_data_plan_datasets,enterprise_regional_series,enterprise_regional_series_samples FROM PUBLIC;

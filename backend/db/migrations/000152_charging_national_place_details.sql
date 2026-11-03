-- National metadata is additive. Historical place and group rows keep their
-- constraints and names. Empty national names represent absent source names.
ALTER TABLE charging_places
    ADD COLUMN catalog_details JSONB,
    DROP CONSTRAINT charging_places_name_check,
    ADD CONSTRAINT charging_places_name_check CHECK (
        CASE WHEN catalog_details IS NULL THEN octet_length(name) BETWEEN 1 AND 160
             ELSE octet_length(name) <= 512 END
    ),
    ADD CONSTRAINT charging_places_catalog_details_check CHECK (
        catalog_details IS NULL OR COALESCE(
            jsonb_typeof(catalog_details) = 'object'
            AND catalog_details->>'confidence' IN ('single-source', 'two-source')
            AND catalog_details->>'uf' IN ('AC','AL','AM','AP','BA','CE','DF','ES','GO','MA','MG','MS','MT',
                'PA','PB','PE','PI','PR','RJ','RN','RO','RR','RS','SC','SE','SP','TO','UNASSIGNED')
            AND jsonb_typeof(catalog_details->'sources') = 'array'
            AND jsonb_array_length(catalog_details->'sources') > 0, FALSE)
    ),
    ADD CONSTRAINT charging_places_batch_id_id_unique UNIQUE (batch_id, id);

ALTER TABLE charging_connection_groups
    ADD COLUMN catalog_source_key TEXT,
    DROP CONSTRAINT charging_connection_groups_connector_code_check,
    DROP CONSTRAINT charging_connection_groups_raw_connector_code_check,
    ADD CONSTRAINT charging_connection_groups_connector_code_check CHECK (
        connector_code IN ('CCS2','TYPE2','CHADEMO','NACS','UNKNOWN')
        OR (catalog_source_key IS NOT NULL AND
            connector_code IN ('CCS1','TYPE1','GBT_AC','GBT_DC','NBR14136','TESLA_LEGACY'))
    ),
    ADD CONSTRAINT charging_connection_groups_raw_connector_code_check CHECK (
        octet_length(raw_connector_code) <= 256 AND
        (catalog_source_key IS NOT NULL OR octet_length(raw_connector_code) >= 1)
    ),
    ADD CONSTRAINT charging_connection_groups_catalog_source_key_check CHECK (
        catalog_source_key IS NULL OR catalog_source_key ~ '^(ocm:[1-9][0-9]*|osm:(node|way|relation):[1-9][0-9]*)$'
    );

CREATE TABLE charging_catalog_place_aliases (
    batch_id UUID NOT NULL REFERENCES charging_catalog_generations(batch_id) ON DELETE RESTRICT,
    public_id UUID NOT NULL,
    place_id UUID NOT NULL,
    is_primary BOOLEAN NOT NULL,
    PRIMARY KEY (batch_id, public_id),
    FOREIGN KEY (batch_id, place_id) REFERENCES charging_places(batch_id, id) ON DELETE RESTRICT
);
CREATE UNIQUE INDEX charging_catalog_place_one_primary
    ON charging_catalog_place_aliases(batch_id, place_id) WHERE is_primary;

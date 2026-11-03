-- Retain the OCM root key emitted by the national source preparer.
ALTER TABLE charging_connection_groups
    DROP CONSTRAINT charging_connection_groups_catalog_source_key_check,
    ADD CONSTRAINT charging_connection_groups_catalog_source_key_check CHECK (
        catalog_source_key IS NULL OR
        catalog_source_key ~ '^(ocm:poi:[1-9][0-9]*|osm:(node|way|relation):[1-9][0-9]*)$'
    );

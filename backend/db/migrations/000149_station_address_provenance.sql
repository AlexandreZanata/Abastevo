-- SNA-04B: retain selected prepared-address evidence alongside immutable facts.
-- Existing assertions and canonical addresses are left untouched.
ALTER TABLE registry_assertions
    ADD COLUMN address_provenance JSONB,
    ADD CONSTRAINT registry_assertion_address_provenance_check CHECK (
        address_provenance IS NULL OR (
            jsonb_typeof(address_provenance) = 'object' AND
            address_source IS NOT NULL AND address_source IN ('rfb','osm','cnefe')
        )
    );

-- EV charging/reference batches are source-owned and immutable once
-- stored. Existing quality, directory and other module rows survive;
-- no cross-module generated SQL. Cryptographic identities are verified
-- by the Go bridge before staging; SQL enforces structural integrity
-- (enums, bounds, same-batch constraints, uniqueness) without
-- reimplementing hashes. Directory links stay null until a reviewed
-- linking unit populates them; nothing here binds stations.
CREATE TABLE charging_batches (
 id UUID PRIMARY KEY,
 format TEXT NOT NULL CHECK (format IN ('charging-prepared-v1','vehicle-energy-reference-v1')),
 profile_id TEXT NOT NULL CHECK (octet_length(profile_id) BETWEEN 1 AND 128),
 source_revision TEXT NOT NULL CHECK (octet_length(source_revision) BETWEEN 1 AND 128),
 manifest_sha256 TEXT NOT NULL CHECK (manifest_sha256 ~ '^[0-9a-f]{64}$'),
 input_count INTEGER NOT NULL CHECK (input_count BETWEEN 0 AND 500000),
 accepted_count INTEGER NOT NULL CHECK (accepted_count BETWEEN 0 AND 500000),
 UNIQUE(profile_id, source_revision)
);

CREATE TABLE charging_places (
 id UUID PRIMARY KEY,
 batch_id UUID NOT NULL REFERENCES charging_batches(id) ON DELETE CASCADE,
 entity_key TEXT NOT NULL CHECK (entity_key ~ '^[0-9a-f]{64}$'),
 external_id TEXT NOT NULL CHECK (octet_length(external_id) BETWEEN 1 AND 256 AND external_id !~ '[[:cntrl:]]'),
 name TEXT NOT NULL CHECK (octet_length(name) BETWEEN 1 AND 160),
 country TEXT NOT NULL CHECK (country = 'BR'),
 latitude TEXT NOT NULL CHECK (latitude ~ '^-?(?:0|[1-9][0-9]?)(?:\.[0-9]{1,16})?$'),
 longitude TEXT NOT NULL CHECK (longitude ~ '^-?(?:0|[1-9][0-9]{0,2})(?:\.[0-9]{1,16})?$'),
 public_access TEXT NOT NULL CHECK (public_access IN ('PUBLIC','UNKNOWN')),
 reported_points INTEGER NOT NULL CHECK (reported_points BETWEEN 0 AND 500000),
 station_id UUID REFERENCES directory_stations(id) ON DELETE RESTRICT,
 UNIQUE(batch_id, entity_key),
 UNIQUE(batch_id, external_id)
);

CREATE TABLE charging_connection_groups (
 id UUID PRIMARY KEY,
 batch_id UUID NOT NULL REFERENCES charging_batches(id) ON DELETE CASCADE,
 place_id UUID NOT NULL REFERENCES charging_places(id) ON DELETE CASCADE,
 entity_key TEXT NOT NULL CHECK (entity_key ~ '^[0-9a-f]{64}$'),
 external_id TEXT NOT NULL CHECK (octet_length(external_id) BETWEEN 1 AND 256 AND external_id !~ '[[:cntrl:]]'),
 connector_code TEXT NOT NULL CHECK (connector_code IN ('CCS2','TYPE2','CHADEMO','NACS','UNKNOWN')),
 raw_connector_code TEXT NOT NULL CHECK (octet_length(raw_connector_code) BETWEEN 1 AND 256),
 current TEXT NOT NULL CHECK (current IN ('AC','DC','UNKNOWN')),
 power_w INTEGER CHECK (power_w BETWEEN 1 AND 1000000000),
 reported_quantity INTEGER CHECK (reported_quantity BETWEEN 1 AND 500000),
 UNIQUE(batch_id, entity_key),
 UNIQUE(batch_id, external_id),
 CHECK ((place_id IS NOT NULL))
);

CREATE TABLE charging_vehicle_references (
 id UUID PRIMARY KEY,
 batch_id UUID NOT NULL REFERENCES charging_batches(id) ON DELETE CASCADE,
 entity_key TEXT NOT NULL CHECK (entity_key ~ '^[0-9a-f]{64}$'),
 model_code TEXT NOT NULL CHECK (octet_length(model_code) BETWEEN 1 AND 256 AND model_code !~ '[[:cntrl:]]'),
 UNIQUE(batch_id, entity_key),
 UNIQUE(batch_id, model_code)
);
CREATE INDEX charging_places_batch_idx ON charging_places(batch_id);
CREATE INDEX charging_groups_batch_place_idx ON charging_connection_groups(batch_id, place_id);
CREATE INDEX charging_vehicles_batch_idx ON charging_vehicle_references(batch_id);

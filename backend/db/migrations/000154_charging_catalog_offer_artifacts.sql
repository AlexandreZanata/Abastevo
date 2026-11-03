-- Store the exact admitted offer next to its national generation.
CREATE TABLE charging_catalog_offer_artifacts (
    batch_id UUID NOT NULL REFERENCES charging_catalog_generations(batch_id) ON DELETE RESTRICT,
    kind TEXT NOT NULL CHECK (kind IN ('data','notice')),
    sha256 TEXT NOT NULL CHECK (sha256 ~ '^[0-9a-f]{64}$'),
    bytes INTEGER NOT NULL CHECK (bytes BETWEEN 1 AND 536870912 AND
        (kind = 'data' OR bytes <= 8388608)),
    PRIMARY KEY (batch_id, kind)
);

CREATE TABLE charging_catalog_offer_chunks (
    batch_id UUID NOT NULL,
    kind TEXT NOT NULL,
    position INTEGER NOT NULL CHECK (position BETWEEN 0 AND 2047),
    data BYTEA NOT NULL CHECK (octet_length(data) BETWEEN 1 AND 262144),
    PRIMARY KEY (batch_id, kind, position),
    FOREIGN KEY (batch_id, kind) REFERENCES charging_catalog_offer_artifacts(batch_id, kind) ON DELETE RESTRICT
);

-- National catalog admission is separate from legacy batch storage.
-- Existing profiles and batch identities remain unchanged.
CREATE TABLE charging_catalog_generations (
    batch_id UUID PRIMARY KEY REFERENCES charging_batches(id) ON DELETE RESTRICT,
    rights_sha256 TEXT NOT NULL CHECK (rights_sha256 ~ '^[0-9a-f]{64}$'),
    offer_sha256 TEXT NOT NULL CHECK (offer_sha256 ~ '^[0-9a-f]{64}$'),
    uf_counts JSONB NOT NULL CHECK (jsonb_typeof(uf_counts) = 'object')
);

CREATE TABLE charging_catalog_current (
    singleton BOOLEAN PRIMARY KEY CHECK (singleton),
    batch_id UUID REFERENCES charging_catalog_generations(batch_id) ON DELETE RESTRICT,
    held BOOLEAN NOT NULL DEFAULT FALSE
);
INSERT INTO charging_catalog_current(singleton) VALUES (TRUE);

CREATE TABLE charging_catalog_activations (
    id BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    batch_id UUID NOT NULL REFERENCES charging_catalog_generations(batch_id) ON DELETE RESTRICT,
    previous_batch_id UUID REFERENCES charging_catalog_generations(batch_id) ON DELETE RESTRICT,
    override_actor TEXT,
    override_reason TEXT,
    activated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    CHECK ((override_actor IS NULL AND override_reason IS NULL) OR
           (previous_batch_id IS NOT NULL AND override_actor IS NOT NULL AND override_reason IS NOT NULL
            AND octet_length(override_actor) BETWEEN 1 AND 128
            AND octet_length(override_reason) BETWEEN 1 AND 500))
);

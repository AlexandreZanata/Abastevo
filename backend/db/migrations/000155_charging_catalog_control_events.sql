CREATE TABLE charging_catalog_control_events (
    id BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    action TEXT NOT NULL CHECK (action IN ('hold', 'resume', 'rollback')),
    previous_batch_id UUID REFERENCES charging_catalog_generations(batch_id) ON DELETE RESTRICT,
    batch_id UUID REFERENCES charging_catalog_generations(batch_id) ON DELETE RESTRICT,
    actor TEXT NOT NULL CHECK (octet_length(actor) BETWEEN 1 AND 128),
    reason TEXT NOT NULL CHECK (octet_length(reason) BETWEEN 1 AND 500),
    recorded_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    CHECK (
        (action IN ('hold', 'resume') AND previous_batch_id IS NOT DISTINCT FROM batch_id)
        OR (action = 'rollback' AND previous_batch_id IS NOT NULL AND previous_batch_id IS DISTINCT FROM batch_id)
    )
);

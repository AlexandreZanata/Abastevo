-- 000015: community price projection (P06-T05). Mutable current
-- prices per exact price key plus the restricted audit inputs behind
-- each version. Writers serialize on a transactional advisory lock per
-- key before loading votes, so a stale worker can never overwrite a
-- fresher projection: versions only move forward under the lock.
-- Reads never replay history; query-time expiry and HIGH decay apply
-- at read, independent of worker health (B-BR-008).
CREATE TABLE community_current_prices (
    station_id UUID NOT NULL REFERENCES directory_stations (id) ON DELETE RESTRICT,
    fuel_product TEXT NOT NULL,
    unit TEXT NOT NULL,
    condition_kind TEXT NOT NULL,
    qualifier_key TEXT NOT NULL,
    amount_milli_brl BIGINT CHECK (amount_milli_brl >= 1),
    availability TEXT NOT NULL CONSTRAINT community_current_prices_availability_check CHECK (availability IN
        ('AVAILABLE', 'DISPUTED', 'UNKNOWN')),
    confidence TEXT NOT NULL DEFAULT '',
    representative_observation_id UUID REFERENCES community_observations (id) ON DELETE RESTRICT,
    independent_supporters INTEGER NOT NULL DEFAULT 0 CHECK (independent_supporters >= 0),
    confirmation_count INTEGER NOT NULL DEFAULT 0 CHECK (confirmation_count >= 0),
    anchor_received_at TIMESTAMPTZ,
    expires_at TIMESTAMPTZ,
    next_recompute_at TIMESTAMPTZ,
    computed_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    projection_version BIGINT NOT NULL DEFAULT 1 CHECK (projection_version >= 1),
    algorithm_version TEXT NOT NULL,
    policy_config_version TEXT NOT NULL,
    PRIMARY KEY (station_id, fuel_product, unit, condition_kind, qualifier_key)
);
CREATE INDEX community_current_prices_expiry_idx
    ON community_current_prices (expires_at);
CREATE INDEX community_current_prices_station_fuel_idx
    ON community_current_prices (station_id, fuel_product);
CREATE INDEX community_current_prices_recompute_idx
    ON community_current_prices (next_recompute_at)
    WHERE next_recompute_at IS NOT NULL;
CREATE TABLE community_projection_inputs (
    projection_key TEXT PRIMARY KEY,
    version BIGINT NOT NULL,
    input_cutoff TIMESTAMPTZ NOT NULL,
    supporting_event_ids TEXT[] NOT NULL DEFAULT '{}',
    reason_codes TEXT[] NOT NULL DEFAULT '{}',
    computed_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

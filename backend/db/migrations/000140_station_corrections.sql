-- 000140: community station corrections (MC-B2, B-BR-COR-01..08).
-- Append-only: corrections never overwrite official facts; a decision
-- creates a new community assertion or a reviewed location revision. Rows
-- stay private to the owner and authorized reviewers, and the decision audit
-- is append-only. No price, trust, ownership or payment output here.
CREATE TABLE station_corrections (
    id UUID PRIMARY KEY,
    station_id UUID NOT NULL REFERENCES directory_stations (id) ON DELETE RESTRICT,
    account_id UUID NOT NULL,
    field TEXT NOT NULL,
    proposed_value JSONB NOT NULL,
    note TEXT NOT NULL DEFAULT '',
    evidence_ref TEXT NOT NULL DEFAULT '',
    proximity_bucket TEXT NOT NULL DEFAULT '',
    client_submission_id TEXT NOT NULL,
    status TEXT NOT NULL DEFAULT 'pending',
    reason_codes TEXT[] NOT NULL DEFAULT '{}',
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    decided_at TIMESTAMPTZ,
    CONSTRAINT station_corrections_field_check
        CHECK (field IN ('location', 'display_name', 'brand', 'address', 'operating_status')),
    CONSTRAINT station_corrections_status_check
        CHECK (status IN ('pending', 'auto_verified', 'accepted', 'rejected',
                          'expired', 'cancelled', 'superseded', 'frozen')),
    CONSTRAINT station_corrections_open_decided_check
        CHECK ((status IN ('pending', 'auto_verified') AND decided_at IS NULL)
            OR (status NOT IN ('pending', 'auto_verified') AND decided_at IS NOT NULL)),
    CONSTRAINT station_corrections_proximity_check
        CHECK (proximity_bucket IN ('', 'same_municipality', 'within_1km',
                                   'within_250m', 'within_50m'))
);
CREATE UNIQUE INDEX station_corrections_idempotency_unique
    ON station_corrections (account_id, client_submission_id);
CREATE UNIQUE INDEX station_corrections_one_open_unique
    ON station_corrections (account_id, station_id, field)
    WHERE status IN ('pending', 'auto_verified');
CREATE INDEX station_corrections_owner_idx
    ON station_corrections (account_id, created_at DESC);
CREATE INDEX station_corrections_pending_idx
    ON station_corrections (station_id, field, created_at)
    WHERE status = 'pending';
CREATE INDEX station_corrections_queue_idx
    ON station_corrections (created_at)
    WHERE status IN ('pending', 'auto_verified');

CREATE TABLE station_correction_decisions (
    id UUID PRIMARY KEY,
    correction_id UUID NOT NULL REFERENCES station_corrections (id) ON DELETE RESTRICT,
    decision TEXT NOT NULL,
    reason TEXT NOT NULL,
    decider_kind TEXT NOT NULL,
    decider_id TEXT NOT NULL DEFAULT '',
    evidence JSONB NOT NULL DEFAULT '{}'::jsonb,
    decided_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT station_correction_decisions_decision_check
        CHECK (decision IN ('auto_verified', 'accepted', 'rejected', 'expired',
                            'superseded', 'frozen')),
    CONSTRAINT station_correction_decisions_decider_check
        CHECK (decider_kind IN ('system', 'staff'))
);
CREATE INDEX station_correction_decisions_correction_idx
    ON station_correction_decisions (correction_id);

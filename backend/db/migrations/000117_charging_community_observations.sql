-- Community EV observations are account-scoped charging reports, separate
-- from fuel observations and official source facts. Existing community,
-- charging and account rows survive; no cross-module writes. Reporters
-- stay anonymous in reads (account IDs never leave the write path).
CREATE TABLE charging_observations (
 id UUID PRIMARY KEY,
 account_id UUID NOT NULL REFERENCES accounts(id) ON DELETE CASCADE,
 place_id UUID REFERENCES charging_places(id) ON DELETE RESTRICT,
 connector_code TEXT NOT NULL CHECK (connector_code IN ('CCS2','TYPE2','CHADEMO','NACS','UNKNOWN')),
 price_milli_brl_per_kwh INTEGER CHECK (price_milli_brl_per_kwh BETWEEN 1 AND 10000000),
 condition TEXT NOT NULL CHECK (condition IN ('STANDARD','OFF_PEAK','PEAK','MEMBER','UNKNOWN')),
 observed_day DATE NOT NULL CHECK (observed_day BETWEEN DATE '2020-01-01' AND DATE '9999-12-31'),
 note TEXT NOT NULL CHECK (octet_length(note) <= 1024),
 revision BIGINT NOT NULL DEFAULT 1 CHECK (revision > 0),
 deleted BOOLEAN NOT NULL DEFAULT false,
 updated_at TIMESTAMPTZ NOT NULL DEFAULT clock_timestamp(),
 CHECK ((NOT deleted) OR (deleted)),
 UNIQUE(account_id, id)
);
CREATE INDEX charging_observations_place_idx ON charging_observations(place_id) WHERE NOT deleted;
CREATE INDEX charging_observations_account_idx ON charging_observations(account_id) WHERE NOT deleted;

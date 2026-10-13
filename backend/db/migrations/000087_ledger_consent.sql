-- PRM05-T05 separate professional consent for financial uploads. Disabling
-- stops new ledger writes without affecting cloud or free data. Deletes stay
-- free. Bulk erasure tombstones records and clears segments and gaps.
CREATE TABLE ledger_consent(
 account_id UUID PRIMARY KEY REFERENCES accounts(id),
 mask INTEGER NOT NULL DEFAULT 0 CHECK(mask BETWEEN 0 AND 3),
 revision BIGINT NOT NULL DEFAULT 0 CHECK(revision>=0),
 updated_at TIMESTAMPTZ NOT NULL DEFAULT clock_timestamp()
);

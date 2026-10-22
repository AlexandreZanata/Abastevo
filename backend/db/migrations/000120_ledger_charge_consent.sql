-- Charge consent uses bit 4 alongside refuel (1) and expense (2).
-- Existing masks survive; no consent row is modified by this migration.
ALTER TABLE ledger_consent DROP CONSTRAINT IF EXISTS ledger_consent_mask_check;
ALTER TABLE ledger_consent ADD CONSTRAINT ledger_consent_mask_check CHECK (mask BETWEEN 0 AND 7);

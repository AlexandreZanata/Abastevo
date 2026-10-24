-- Private electric sessions share the owned ledger with fuel refuels
-- (PHEV/fuel coexistence: one vehicle holds both datasets without
-- interference). Existing refuel/expense rows, revisions and erasure
-- survive; no payload shape changes. Null measurements stay null (no
-- invented zeros); tariff references pin revisions (later changes never
-- rewrite history); sessions never become public observations.
ALTER TABLE ledger_records DROP CONSTRAINT IF EXISTS ledger_records_dataset_check;
ALTER TABLE ledger_records ADD CONSTRAINT ledger_records_dataset_check CHECK (dataset IN ('refuel','expense','charge'));

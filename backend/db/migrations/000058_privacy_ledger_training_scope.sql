-- 000058: admit the TRAINING erasure scope in the deletion replay
-- ledger (CS10D). Append-only constraint evolution: drop the frozen
-- scope list and re-add it with TRAINING between EVIDENCE and TRUST,
-- matching the AllScopes execution order. Existing rows are
-- unaffected (no backfill: restores before this migration never
-- recorded training rows, and the replay runner only replays scopes
-- present in the ledger).
ALTER TABLE privacy_deletion_ledger
    DROP CONSTRAINT privacy_deletion_ledger_scope_check;
ALTER TABLE privacy_deletion_ledger
    ADD CONSTRAINT privacy_deletion_ledger_scope_check CHECK (scope IN
        ('IDENTITY', 'COMMUNITY', 'EVIDENCE', 'TRAINING', 'TRUST', 'EXPORTS'));

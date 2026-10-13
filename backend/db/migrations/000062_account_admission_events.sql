-- RVA02: budget is independent of credential/account erasure. No owner linkage.
CREATE TABLE account_key_admission_events (
 id BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
 admitted_at BIGINT NOT NULL
);
CREATE INDEX account_key_admission_events_time_idx ON account_key_admission_events(admitted_at, id);
-- Conservative transition: legacy erased credentials cannot reconstruct their
-- type; preserve the hourly budget from all recent account audit creations.
INSERT INTO account_key_admission_events(admitted_at)
SELECT extract(epoch FROM created_at)::bigint FROM accounts
WHERE created_at >= now() - interval '1 hour'
ORDER BY created_at DESC LIMIT 100;

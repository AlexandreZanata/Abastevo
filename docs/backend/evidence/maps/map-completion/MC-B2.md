# MC-B2 correction storage

Status: LOCAL_DONE. Date: 2026-10-27. Base: `5db9765e` plus this task.
Time: ~25 minutes.

## Delivered

- Append-only migration `000140_station_corrections.sql`:
  - `station_corrections` with station/account/field, structured JSONB value,
    optional note/evidence/coarse proximity bucket, idempotency key, the eight
    frozen statuses, reason codes and timestamps;
  - unique `(account_id, client_submission_id)` and a partial unique index
    enforcing one open correction per account+station+field;
  - queue/owner/pending indexes for bounded review and owner reads;
  - `station_correction_decisions` with append-only audit fields (decision,
    mandatory reason, decider kind/id, evidence JSONB, timestamp);
  - check constraints coupling open statuses to `decided_at`.
- `sqlc.yaml` directory schema includes `000140`; `sqlc generate` produced the
  typed models and queries (create/get/by-key/owned/count/open/cancel/pending/
  decide/decision).
- No official/ANP fact is updated by these tables.

## Verification

- `cd backend && sqlc diff` empty; `sqlc generate` clean.
- `go build ./...` exit 0.
- `git diff --check` clean.
- Integration coverage for the migration and constraints is in MC-B4.

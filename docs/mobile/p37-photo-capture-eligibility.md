# P37-PC01 camera authorization checkpoint

Status: LOCAL_DONE
Validation: PASS

Parent: a20d484, maintained dev. B-BR-PC01 / BUC-PC01 are recorded in
[the contribution plan](../planning/PHOTO_PRICE_CONTRIBUTION_PLAN.md).
The [capture contract](../backend/PHOTO_CAPTURE_CONTRACT.md) defines signed,
owner/key/station-bound receipts. Coordinates remain transient. Camera access
expires after two minutes; submission expires after 24 hours. Replay preserves
the original receipt. An atomic binding permits one evidence session and stable
retries. Existing metadata-only submission behavior is unchanged.

Validation (2026-10-07):

- `go test ./internal/modules/community/... ./cmd/api ./internal/platform/apicontract/...`
- `go test -race -count=1 -tags=integration ./internal/modules/community/adapters -run '^TestPhotoCapture'`
- Real PostGIS isolated database: reviewed station inside/outside, owner isolation,
  key mismatch, expired camera/submission, unchanged replay, concurrent binding,
  migration ledger recovery. New migrations are append-only.
- `sqlc generate`, `sqlc vet`, OpenAPI vacuum lint (zero errors),
  `git diff --check`, tracked secret scan.

This checkpoint is source integration only. Android client, evidence reservation
binding, operational expiry sweep and device acceptance continue in subsequent
plan slices; this does not certify staging or production.

# MC-B3 correction API and review port

Status: LOCAL_DONE. Date: 2026-10-27. Base: `96952e07` plus this task.
Time: ~40 minutes.

## Delivered

- `application.CorrectionService`: validated one-field submissions, 20/day
  quota, one open per account+station+field, semantic idempotent replay,
  changed-body conflict, owner-only read/cancel, pending list and audited
  decisions. Validation covers location (Brazil bounds, null-island refusal),
  display name/brand, structured address, operating status, note/PII rejection,
  evidence-ref bound and coarse proximity buckets.
- Adapter `CorrectionStore`: generated queries, PostgreSQL unique-violation
  mapping, a single transaction for the decision plus its audit row, and
  `ApplyAcceptedLocation` appending a `reviewed` revision through the existing
  repository ladder (highest pin rank).
- HTTP handler with the four signed routes:
  - `POST /v1/stations/{id}/corrections`
  - `POST /v1/stations/corrections/mine`
  - `POST /v1/stations/corrections/{id}/status`
  - `POST /v1/stations/corrections/{id}/cancel`
  Session in the JSON body, `no-store`, one 404 for foreign/unknown ids.
- Composition root wires the account session validator; suspended/deleted
  accounts are forbidden.
- `station-review` operator CLI gains `-corrections-list` and
  `-corrections-decide` with mandatory `-reason`, `-decider` and JSON
  `-evidence`; decisions use the same port and never edit rows directly.
- OpenAPI paths and `StationCorrection`/`StationCorrectionValue` schemas added
  before implementation was exposed.

## Verification

- `cd backend && go build ./...` and `go vet ./...` exit 0.
- `go test -count=1 ./...` exit 0, 0 `FAIL`.
- Abuse/idempotency/concurrency and real PostGIS coverage is in MC-B4.
- `git diff --check` clean.

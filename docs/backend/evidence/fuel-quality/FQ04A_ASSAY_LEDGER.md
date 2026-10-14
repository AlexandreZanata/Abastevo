# FQ-04a — Assay ledger, migrations and atomic publication

Status: LOCAL_DONE / INTEGRATION_PENDING. Date: 2026-10-14.
Base: `codex/fuel-quality-plan` atop `origin/dev 08a54dc4`. Depends:
FQ-01/FQ-03 and app migration conventions. B-BR-FQ-04/05 (assay scope),
BUC-FQ-01/03 (assay scope). FQ-04 stays IN_PROGRESS: enforcement/sanction
fact tables and edition/period controls continue as FQ-04b.

## Behavior before implementation

No quality table, query or publisher existed: manifests, revisions and
links lived only as domain shapes with no durable ledger or lease.

## Delivered

- `backend/db/migrations/000093_quality_assay_ledger.sql` (new,
  append-only): `quality_editions` (unique source/stream/edition),
  `quality_resources` (URL + SHA-256 + bytes), `quality_manifests`
  (counts with a balance CHECK), `quality_revisions`
  (staging/published/superseded/withdrawn + supersession), `quality_assays`
  (verbatim native text, closed fuel/conformity enums, fingerprint
  idempotency, sample index), `quality_links` (nullable station FK to
  `directory_stations`, collision-tolerant uniqueness), and
  `quality_current_revisions` (the read lease), plus a partial unique
  index keeping exactly one published revision per edition.
- `backend/db/queries/quality/ledger.sql` + generated `quality` stanza in
  `sqlc.yaml` (schema: postgis + directory + 000093; `sqlc vet` and
  `generate` clean, generated code committed).
- `quality/application`: validated `Input` reusing the domain manifest
  rule, and the `Publisher` port (single-transaction publish, identical
  replay returns the existing revision).
- `quality/adapters/publisher.go`: edition/resources/manifest staging,
  fingerprint-idempotent assay and link staging, staged-count assertion,
  supersession, guarded publish, atomic current-pointer switch and
  rollback on any failure. Concurrent losers converge: edition-key
  conflicts retry into replay, published-index conflicts read the winner.
- `quality/application/publish_test.go` (pure contract cases) and
  `quality/adapters/publisher_integration_test.go` (real PostGIS,
  disposable database per test): publish + lease, replay stability,
  8-racer convergence to one revision, CHECK-violation and
  counts-mismatch rollback with nothing visible, correction supersession
  with retained history and moved pointer, staging invisibility to the
  lease, and migration recovery (000093 applied, re-apply no-op).

## Validation

RED→GREEN: first `-race` run failed `TestConcurrentReplayConverges` —
losers of the edition-key race surfaced unique-violation 23505 instead of
converging. Fixed with a bounded retry into the replay path; rerun green.

- `go test -race -count=1 -tags=integration ./internal/modules/quality/...`:
  adapters 7/7, application and domain suites PASS.
- `go build ./...`, `go vet`, `gofmt -l` clean; `kernel` suite PASS.
- Neighbor check: `official/...` integration suites PASS with the extended
  chain (migration + sqlc stanza are additive).
- `git diff --check` clean; new-file Markdown links resolve;
  scoped secret/PII scan clean (synthetic CNPJs/URLs only).

## Recovery

Retain the original publication: superseded revisions keep serving their
rows, and replaying the reconciled manifest returns the same revision id.
Drop `quality_*` tables only by a new append-only migration; never edit
000093. Next: FQ-04b enforcement/sanction ledgers and edition controls.

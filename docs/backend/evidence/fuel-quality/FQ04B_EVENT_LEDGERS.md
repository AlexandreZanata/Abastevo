# FQ-04b — Enforcement/sanction ledgers and edition controls

Status: LOCAL_DONE / INTEGRATION_PENDING. Date: 2026-10-14.
Base: `codex/fuel-quality-plan` atop `origin/dev 08a54dc4`. Depends:
FQ-01/FQ-03/FQ-04a and app migration conventions. B-BR-FQ-04/05/06
(event scope), BUC-FQ-01/03 (event scope). FQ-04 is now LOCAL_DONE;
FQ-06/FQ-07 own the adapter vocabularies built on these tables.

## Behavior before implementation

FQ-04a froze editions, manifests, revisions and the read lease for the
assay stream only: enforcement/sanction facts and claimed edition periods
had no ledger, and no scoped lease separated the streams.

## Delivered

- `backend/db/migrations/000094_quality_event_ledgers.sql` (new,
  append-only): `quality_enforcement_events` (native key, verbatim scope/
  status, subject CNPJ, occurred/as-of dates, evidence ref, per-revision
  native-key uniqueness + subject index), `quality_sanction_events`
  (proceeding ref, party CNPJ, nullable non-negative minor-unit amount,
  extraction-time status, decided/as-of dates + party index) and
  `quality_periods` (claimed coverage labels with ordered date bounds per
  edition, so missing/duplicated period links stay visible for FQ-05).
  Procedural scope/status stay source-verbatim; no guilt, finality or
  current-legal-status meaning is stored.
- Extended `db/queries/quality/ledger.sql` (stage/count/list per event
  table, period upsert/list) and the `quality` sqlc stanza schema;
  `sqlc vet`/`generate` clean, generated code committed.
- `quality/application/events.go`: `EventsInput` restricted to the
  enforcement/sanctions sources, reusing the domain manifest rule plus
  period-order and row-identity checks; pure contract tests.
- `quality/adapters/event_publisher.go`: `PublishEvents` reuses the 04a
  single-transaction machinery (extracted `editionRequest` + `stageFunc`:
  the assay publisher behavior is unchanged, verified by its untouched
  suite) with event/period staging and staged-count assertions.
- Integration tests (real PostGIS, disposable database per test):
  scoped per-stream lease (enforcement publication never moves the assay
  lease), verbatim procedural status and minor-unit amount, ordered
  period claims, replay stability, CHECK-violation rollback with nothing
  visible, and correction supersession with retained history.

## Validation

- `go test -race -count=1 -tags=integration ./internal/modules/quality/...`:
  all suites PASS (4 new event integration cases + 2 contract cases;
  04a suites untouched and green).
- `go build ./...`, `go vet -tags=integration`, `gofmt -l` clean.
- `git diff --check` clean; new-file Markdown links resolve;
  scoped secret/PII scan clean (synthetic CNPJs/URLs only).

## Recovery

Retain the original publication per stream: superseded event revisions
keep serving their rows, and replaying a manifest returns the same
revision id. Never edit 000093/000094. Next: FQ-05 historical backfill
and incremental discovery.

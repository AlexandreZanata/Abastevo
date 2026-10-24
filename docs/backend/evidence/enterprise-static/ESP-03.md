# ESP-03 — Freeze module-owned normalized export snapshots

Status: LOCAL_DONE
Validation: PASS
Integration: INTEGRATION_PENDING
Date: 2026-10-24
Branch: `codex/enterprise-static-portal`
Parent: ESP-02 `9c5e8d2c`.

Rules: B-BR-ESP-01/03–05/10/11; BUC-ESP-02.
[Plan](../../../planning/ENTERPRISE_DATA_STATIC_PORTAL_PLAN.md#esp-03--freeze-module-owned-normalized-export-snapshots).
[Admission register](../../../product/ENTERPRISE_SOURCE_ADMISSION.md).

## Before behavior

Directory and Official had public read ports for search/detail and per-station
prices, but no bounded bulk projection, no pinned-revision manifest and no way
to freeze one consistent input for the static builder. A caller could not prove
which Directory identity state or Official revision produced a page, and an
older correction could be read as current. The export also had to avoid becoming
a second station registry or price database.

## Delivered

- **Directory export port** (`applications.ExportReader` + read adapter):
  canonical-ID keyset pagination, full CNPJ only while the identifier is valid,
  source-reported `operation_status`, reviewed coordinates only, and an address
  field allowlist (`street`, `number`, `complement`, `district`, `postal_code`,
  `municipio`, `uf`; free-text `raw` is never exported). Suspended/revoked
  stations remain visible with their status instead of disappearing.
- **Official export port** (`application.ExportReader` + read adapter):
  current published revision per survey window in a selected range, with source
  URL/checksum/parser version/supersession pin; bounded price pages. Staging,
  failed, under-review and superseded revisions are excluded by the current
  pointer join.
- **Enterprise export builder** (`internal/modules/enterpriseexport`):
  deterministic `stations.jsonl`, `revisions.jsonl`, `prices.jsonl` and
  `manifest.json` in a private candidate directory. The manifest binds schema,
  public-field policy, survey scope, source pins, counts, per-file SHA-256/bytes/
  rows and a `content_sha256`. Operational `built_at`, the database snapshot
  identifier and the page/budget settings are excluded from the content hash, so
  identical inputs produce the same content identity. The builder streams with page/row budgets, refuses an
  existing output directory, writes to a temporary sibling directory and renames
  only after the manifest is complete. It performs no canonical write.
- **Consistency contract:** the caller supplies a read-only `REPEATABLE READ`
  transaction; the Directory and Official adapters run inside it. The manifest
  records `pg_current_snapshot()` as provenance. This is the bounded-consistent-
  read option allowed by the plan; a materialized projection is only warranted
  if later measurements require it.

No migration, dependency, source activation, analytics, public route or release
was added. Analytics start at ESP-04; Quality official records at ESP-11; the
production build composition at ESP-06; publication at ESP-13/14.

## Tests and actual results

Unit and race, from `backend/`:

```sh
go test -race -count=1 ./internal/modules/directory/... \
  ./internal/modules/official/... ./internal/modules/enterpriseexport
go build ./...
go vet ./internal/modules/directory/... ./internal/modules/official/... ./internal/modules/enterpriseexport
GOTOOLCHAIN=go1.27.1 staticcheck ./internal/modules/directory/... ./internal/modules/official/... ./internal/modules/enterpriseexport
sqlc vet
gofmt -l internal/modules/directory internal/modules/official internal/modules/enterpriseexport db/queries
git diff --check
```

All PASS. Builder unit tests cover pagination, status counts, address
allowlisting, exact milli-BRL/currency rows, deterministic content hash across
different `built_at`, changed-data hash, budget refusal with no residual output,
existing-output refusal, manifest round-trip and a non-advancing cursor.

Real PostGIS, disposable local database:

```sh
go test -race -count=1 -tags=integration \
  ./internal/modules/directory/adapters/read \
  ./internal/modules/official/adapters/read \
  ./internal/modules/enterpriseexport
```

All PASS. The Directory test proves two-page traversal, reviewed coordinates,
suspended status, invalidated CNPJ removal and the address allowlist. The
Official test proves two current weeks, same-week correction supersession,
superseded/staging price exclusion, price pagination and an empty unknown range.
The end-to-end test opens a repeatable-read/read-only transaction, establishes
its snapshot, publishes a correction from another connection, and proves the
in-transaction build still pins and emits the pre-correction revision/amounts;
a new transaction then pins the correction. `sqlc generate` is byte-stable.

## Limitations

- The builder does not open the transaction or choose the source adapters; the
  caller must use a read-only repeatable-read transaction. A production CLI/job
  composition remains for ESP-06, and the static pages remain for ESP-09.
- V1 export covers Directory stations and current Official price revisions. It
  does not yet export Quality enforcement/fine records (ESP-11) or derived
  analytics (ESP-04).
- Budgets are policy defaults (100k stations, 2k revisions, 5M prices); exceeding
  one fails the candidate rather than silently truncating. No full national
  backfill was measured.
- The tests use synthetic database rows and a disposable PostGIS database; no
  live ANP source, public artifact, deployment or capacity claim is made.
- Candidate files are private (0700 directory/0600 files) and are not served.
- Next: ESP-04 deterministic analytics over the pinned snapshot inputs, with
  denominator/period/tie/missing-data golden tests.

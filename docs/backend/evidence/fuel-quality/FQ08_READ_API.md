# FQ-08 — Official read API, timeline and coverage

Status: LOCAL_DONE / INTEGRATION_PENDING. Date: 2026-10-14.
Base: `codex/fuel-quality-plan` atop `origin/dev 08a54dc4`. Depends:
FQ-04/FQ-06/FQ-07. BUC-FQ-04/09. Anonymous public reads; no signed
writes, scores, migrations, VPS or Android in this task.

## Behavior before implementation

The ledger published but served nothing: no summary, pages, detail,
lease envelope, staleness rule or route existed.

## Delivered

- `db/queries/quality/reads.sql` (new, `sqlc vet`/`generate` clean):
  lease-by-stream, station/fuel summary, keyset assay/action/sanction
  pages and sample/event detail, every row scoped to one caller-resolved
  published revision. Assay pages key on observed date plus row id;
  event pages key on native key plus row id for complete frozen-edition
  traversal. Event pages are stream-scoped: station attribution for
  events needs resolved links, recorded owed.
- `quality/application/reads.go` (pure): closed-vocabulary fuel filter,
  UUID/text-filter validation, the frozen 180-day staleness rule
  (missing leases always stale, old leases served flagged, never
  dropped) and the deterministic denominator explanation builder.
- `quality/adapters/read`: frozen-lease Reader; source CNPJs and
  internal notes never leave the package (machine-checked in tests).
- `quality/adapters/http`: anonymous routes `GET /v1/quality/summary`,
  `/assays`, `/samples/{ref}`, `/actions`, `/actions/{id}`,
  `/sanctions`, `/sanctions/{id}`, every response carrying its lease
  (stream, edition, revision, denominators, periods, staleness,
  coverage note). Sealed filter-bound cursors fail closed across
  revisions; unknown stations/rows 404, unknown fuels/filters 400,
  storage failures 503 without internals. `ANPFUEL_QUALITY_READS_ENABLED`
  (default true) answers 404 without touching storage: the read recovery.
- Additive OpenAPI schemas/paths (`QualityLease`, `QualitySummary`,
  `QualityAssay(Page)`, `QualitySample`, `QualityAction(Page)`,
  `QualitySanction(Page)`); `apicontract` suite green.
- Wiring in `cmd/api/main.go` under the existing composition rule
  (station-exists closure, shared cursor secret); config flag validated
  at startup with the suite green.

## Validation

RED→GREEN: unknown sanction id on an unpublished stream answered 503
instead of 404 — detail routes now map missing leases to not-found;
rerun green.

- `go test -race -count=1 -tags=integration ./internal/modules/quality/...`:
  all suites PASS (read 7/7 incl. frozen-revision concurrency and
  identifier-leak checks; http 6/6 incl. anonymous reads, codes,
  traversal, kill switch, cross-revision cursor refusal).
- `go build ./...`, `go vet -tags=integration`, `gofmt -l` clean.
- Real SQL EXPLAIN on a migrated scratch database (chain 000001–000095
  applied clean, then dropped): summary and page plans use index scans
  on both join sides with no sequential scans — no denormalization.
- `git diff --check` clean; new-file Markdown links resolve;
  scoped secret/PII scan clean (synthetic fixtures only).

## Recovery

Turn quality reads off (`ANPFUEL_QUALITY_READS_ENABLED=false`); existing
station/price/community reads keep working, routes untouched otherwise.
Next: FQ-09 signed structured reports.

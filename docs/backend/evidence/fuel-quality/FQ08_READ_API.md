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

## VPS deployment attempt (abastevo-temp only, 2026-10-14)

Scope respected: only namespace `abastevo-temp` mutated (images,
migration job, one deployment's rollout); primary checkout, test-agent
branch and all other namespaces untouched (pods diff clean except one
external healthy restart of guia-cidade gc-api, unrelated).

- Built `abastevo-api:temp` + `abastevo-migrate:temp` + `abastevo-worker:temp`
  from `91ff1f93`, imported to k3s, migration job applied exactly
  `[000093 000094 000095]`, rollout restarted only
  `deployment/abastevo-temp-api`.
- RESULT: BLOCKED — pods run the new image (config-digest verified) but
  `/health/ready` stays 503 on the `schema` check while `db` passes.
- Ruled out with evidence: ledger chain (95/95 byte-identical checksums),
  all 11 tables present, embedded chain in the image (95 files, quality
  SQL present), credentials and DSN (migration job succeeds with them),
  DNS/service endpoints (single ready endpoint), locks (none), disk
  (42%), secret stability (unchanged since Oct 08). The same image
  reports READY against a freshly migrated scratch database, and
  `migrate.Apply` re-run reports "already current".
- Not yet determined: the exact `Check` error text inside the pod
  (distroless image has no shell; ephemeral debug containers share neither
  network namespace nor target-container access on this runtime).
- State left: deployment on `:temp` images, DB migrated (append-only,
  irreversible by policy), service degraded until the check passes.
  Rollback to an older image cannot pass either (chain older than DB).
- Resume: instrumented rebuild with the check error surfaced (prepared
  `/debugz/schema-check` draft, kept out of the branch), or compare a
  fresh pod's first-probe timing against pool setup; audit any concurrent
  writer to the image store (tag flapping observed on throwaway tags).

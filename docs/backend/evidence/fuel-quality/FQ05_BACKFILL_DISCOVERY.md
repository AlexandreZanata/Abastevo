# FQ-05 — Historical backfill and incremental discovery

Status: LOCAL_DONE / INTEGRATION_PENDING. Date: 2026-10-14.
Base: `codex/fuel-quality-plan` atop `origin/dev 08a54dc4`. Depends:
FQ-02/04 (assay path done); enforcement-content discovery additionally
needs the FQ-06 adapter contract and stays owed. B-BR-FQ-03–05,
BUC-FQ-01–03 (planner/discovery scope). No live fetch, VPS or scheduler
wiring in this task: listing, fetching and preparation stay injected.

## Behavior before implementation

Editions published only by direct publisher calls: no planner decided
replays, corrections, aliases or missing resources, and no bounded
scheduled run or watermark existed.

## Delivered

- `quality/application/backfill.go` (pure): `PlanBackfill` decides one
  bounded chunk, oldest labels first — import_new (unknown or late
  labels, never skipped), replay_stable (same content converges),
  correct (same label, new bytes, superseding revision), alias (known
  bytes under another label, e.g. the duplicated February/July monthly
  resource — one import, no double support), skip_missing (unpublished
  resources retried by later runs).
- `backend/db/migrations/000095_quality_watermarks.sql` (new,
  append-only): one cursor per source/stream recording the last
  completed edition import; failures never move it. Queries
  `ListStreamResourceHashes`/`GetWatermark`/`SetWatermark` added to the
  `quality` stanza (`sqlc vet`/`generate` clean).
- `quality/adapters/jobs/discovery.go`: Discovery handler (injected
  lister, bounded MaxEditions, converging dedupe keys) and Import
  handler (hash-pinned fetch with byte budget, injected preparation,
  atomic publish per stream, watermark advance only on success). The
  ledger itself is the resume state; the watermark records completion
  and the predecessor chain. Shared publisher machinery extracted to
  `editionRequest`/`stageFunc` (assay behavior unchanged, suite green);
  ID makers are fallible end to end.
- Integration tests (real PostGIS, disposable database per test):
  bounded oldest-first chunk, four overlapping discoveries converging on
  five distinct import jobs via the real platform queue, late February
  resource importing behind the watermark, publish + watermark advance,
  hash-mismatch and preparation failures leaving edition, publication
  and watermark untouched.

## Validation

RED→GREEN: `-race` runs exposed a fallible-ID mismatch at the jobs
boundary (makers now return errors), a redundant watermark-cursor filter
in Discovery (removed; the ledger decides, the watermark records), and
four test-setup defects (non-hex seeds, shared-hash fixtures colliding
with alias detection, per-test ID sequences colliding across seeds) —
all fixed without weakening assertions.

- `go test -race -count=1 -tags=integration ./internal/modules/quality/...`:
  all suites PASS (6 jobs integration + 3 planner + contract suites).
- `go build ./...`, `go vet -tags=integration`, `gofmt -l` clean.
- `git diff --check` clean; new-file Markdown links resolve;
  scoped secret/PII scan clean (synthetic CNPJs/URLs only).

## Recovery

Retain the last completed watermark or manifest: failed, skipped or
partial runs resume behind the cursor with the current publication
preserved; replaying a manifest returns the same revision id. Never edit
000093–000095. Next: FQ-06 enforcement adapter and legally accurate
events (which also unlocks enforcement-content discovery).

# FQ-13 — Deterministic projection engine and fenced recomputation

2026-10-18 review addendum: affected acceptance claims are superseded by
[the independent review](FQ_REVIEW_20261018.md), including full sample scope,
methodology version, projection fencing and operator-only dead-letter recovery.

Status: LOCAL_DONE
Validation: PASS

Completed 2026-10-18 on maintained dev, parent 07a6319c (FQ-12). The tested
behavior is the projection store, recomputation job, publication handoff and
fencing in this atomic checkpoint; no HTTP surface or Android consumption was
added (FQ-14). Delivery remains INTEGRATION_PENDING.

Before behavior: B-BR-FQ-08/09/13/16, BUC-FQ-03/05/09. The FQ-12 judgment
contract existed, but nothing derived station/fuel dimensions, nothing
recomputed after corrections, and no derived row was bound to its input or
protected from stale writers.

## Delivered and verified

- Migration 105 (`quality_station_fuel_projections`): derived, versioned rows
  keyed by (station, fuel, methodology) with policy id
  `pmqc-dimensions-v2026-10a`, the frozen basis (source/stream/revision +
  published_at), the canonical input digest, the FQ-12 dimension counts,
  observation-day bounds, `UNSCORED` score state with a NULL score, a
  monotonic generation and `computed_at`. A CHECK enforces
  conforming+nonconforming+unresolved = samples.
- `ProjectionStore.RecomputeRevision` judges every active linked sample of
  one published revision in a single transaction, groups per station and
  fuel, digests the canonical judged input (order-independent) and upserts
  through the current-pointer fence: a basis that is not (or stopped being)
  the stream's current published revision writes nothing
  (`ErrProjectionBasisStale`); identical digests are true no-ops. Community
  observations, private reports, moderation and payments never participate.
- The import path enqueues `quality-recompute` for the published revision
  with dedupe identity `quality-recompute:<revision>` after the watermark
  advance; the durable handler validates the versioned envelope, applies the
  recomputation and treats a stale basis as a completed no-op, so a late
  worker can never overwrite newer derived rows and no manual reset is
  needed. The kind is reserved from general claims and composed with the
  native handlers only when imports are enabled.
- Real-PostGIS acceptance: deterministic dimensions for the FQ-12 case
  matrix (gasoline aspect failure, conforming sample, confirmed diesel
  turbidity, diesel cleared by a conforming complementary, replica conflict
  → UNRESOLVED; an unlinked sample never projects); identical replay rewrites
  nothing and does not bump the generation; a correction moves the
  pointer, the new basis reads as unknown until recomputed, then the old
  basis is refused and the stale row set is gone; four concurrent
  recomputations of one revision converge to two rows with one digest and no
  fork; report erasure leaves official rows byte-identical. The real
  `Import` handler test proves publication enqueues exactly one recompute
  intent whose revision equals the current pointer, replay converges, the
  handler applies it twice idempotently, a non-current basis completes as a
  no-op and a malformed envelope fails.

Named limits: the projection is anchored to the stream's current edition
(matching the existing FQ-08 lease semantics); an explicit historical
revision request reads as unknown unless a future policy stores per-basis
history, and no numeric score is produced while calibration is unfrozen.
Payment, community and private signals never influence these rows.

Commands from backend/:

```sh
ANPFUEL_TEST_QUALITY_PREPARER_PATH=/tmp/fq-native-rust/debug/prepare_quality ANPFUEL_TEST_PREPARER_PATH=/tmp/fq-native-rust/debug/prepare_registry GOCACHE=/tmp/fq-completion-go-cache go test -count=1 -race -tags=integration ./cmd/worker ./internal/modules/quality/... ./internal/modules/directory/adapters
GOCACHE=/tmp/fq-completion-go-cache go build ./...
XDG_CACHE_HOME=/tmp/fq-completion-staticcheck GOTOOLCHAIN=go1.27.1 GOCACHE=/tmp/fq-completion-static-cache staticcheck ./cmd/worker ./internal/modules/quality/...
sqlc vet
sqlc generate
```

PASS: quality adapters (including the four new projection cases and the
enqueue-hook case) green with race detection; cmd/worker, quality http/read/
domain/jobs, discovery/revalidation suites and Directory adapters green;
build/staticcheck/sqlc stable; four migration-head assertions updated for
000105. gofmt, `git diff --check` and the scoped secret scan PASS. No new
dependency.

## Explicit next obligations

FQ-14 consumes the projections for the guest Android/API experience; FQ-15
reports, FQ-16 offline, FQ-17 scoped acceptance and the final protected
dev→main batch remain. Numeric scoring stays blocked until calibration is
accepted; D is NOT_AUTHORIZED.

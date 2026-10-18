# FQ-05W6 — Durable Directory revalidation for completed publications

2026-10-18 review addendum: affected acceptance claims are superseded by
[the independent review](FQ_REVIEW_20261018.md), including full sample scope,
methodology version, projection fencing and operator-only dead-letter recovery.

Status: LOCAL_DONE
Validation: PASS

Completed 2026-10-17 on maintained dev, parent b7aac422 (FQ-05W5). The tested
behavior is the W6 platform/Directory/quality/sweep changes in this atomic
checkpoint; delivery remains INTEGRATION_PENDING. No remote checks,
deployment, import activation or schedule registration claimed.

Before behavior: B-BR-FQ-01/02/03/04/05/07/13/16, BUC-FQ-01/02/03/04.
After a successfully completed publication, a later Directory binding move,
retirement or station deactivation must not leave stale active associations
visible until someone retries by hand. A bounded sweep compares every
currently published link with live Directory evidence, enqueues exactly one
frozen-provenance fresh import per divergent edition, and lets the existing
atomic publisher supersede with a correction. Historical revisions stay
frozen and reachable; no business validity window, lease/watermark/ETag
meaning or association decision is invented, and the sweep performs no
source I/O.

Platform enqueue now returns an owned `EnqueueResult{ID, Created, Status}`:
converged dedupe replays are no longer counted as fresh work, concurrent
callers converge on one stored row, and a converged dead refresh is replayed
once with an audited reason instead of forking duplicates.

## Delivered and verified

- `jobs.Enqueue` returns the created/existing/status outcome; ON CONFLICT
  DO NOTHING waits for the competing transaction, so a converged lookup
  observes a committed row. Eight concurrent callers of one dedupe key in
  real PostgreSQL create exactly one job and exactly one caller reports
  creation. Existing call sites keep their behavior; the two callers that
  consumed the identifier read `EnqueueResult.ID`.
- Directory owns the bounded set-based evidence read
  (`SourceEvidenceBatch`, 512 identifiers per statement) with exactly the
  two-candidate semantics of the single lookup; invalid identifiers refuse
  the whole batch before any query. Real-PostGIS test mirrors
  retained-history ambiguity and current/active flags.
- Migration 103 adds `quality_revalidation_cursors`: operational sweep
  progress separate from import watermarks. The cursor advances whole
  editions, wraps after the full stream, and never changes completed-import,
  lease or historical source clocks.
- `quality-revalidate` is reserved from general claims in every
  configuration and gets its own fixed slot only when
  `ANPFUEL_QUALITY_IMPORTS_ENABLED` is true; disabled native jobs remain
  queued at zero attempts while ordinary/safety work completes.
- The sweep judges only active associations: a bound record whose current
  Directory binding no longer names the same active station diverges
  (retired, moved, station missing/deactivated). Divergence is fingerprinted
  canonically per frozen revision; the enqueue dedupe identity is
  revision+fingerprint, so unchanged divergence converges on one job and a
  later change on the corrected revision gets a distinct one. Source bytes
  are re-fetched only when a queued import executes, never on every sweep.
- Real end-to-end acceptance with the pinned Rust preparer: retire binding →
  one refresh job reusing the frozen URL/hash/member → repeated sweeps
  converge → three failed attempts park dead → the sweep replays it once
  with `directory divergence persists; fresh source retry requested`
  (status queued, attempts 0, reason recorded) → the fresh import publishes
  the correction, the unrelated station keeps its fact, the superseded
  revision stays reachable with its frozen association, no station is
  created, and a later consistent sweep enqueues nothing. Deterministic
  synthetic-edition test proves the one-edition-per-run cursor window
  advances durably and reaches the divergent second edition instead of
  re-checking the first forever.

Named boundary: a record that was unbound and later becomes bindable again
is not auto-republished. Its manifest is frozen under the superseded
revision's publication key, which FQ-04c deliberately makes structurally
un-repeatable, and the conservative failure mode is a missing association,
never a wrong one. Historical explicit-revision access keeps the earlier
association; a later genuine source edition picks up current Directory.
This is recorded in code and not inferred as acceptance.

Commands from backend/:

```sh
ANPFUEL_TEST_QUALITY_PREPARER_PATH=/tmp/fq-native-rust/debug/prepare_quality ANPFUEL_TEST_PREPARER_PATH=/tmp/fq-native-rust/debug/prepare_registry GOCACHE=/tmp/fq-completion-go-cache go test -count=1 -race -tags=integration ./cmd/worker ./internal/modules/quality/... ./internal/platform/config ./internal/modules/official/adapters/source ./internal/platform/jobs ./internal/modules/directory/...
GOCACHE=/tmp/fq-completion-go-cache go vet ./cmd/worker ./internal/modules/quality/adapters/... ./internal/modules/directory/adapters/... ./internal/platform/jobs
GOCACHE=/tmp/fq-completion-go-cache go build ./...
XDG_CACHE_HOME=/tmp/fq-completion-staticcheck GOTOOLCHAIN=go1.27.1 GOCACHE=/tmp/fq-completion-static-cache staticcheck ./cmd/worker ./internal/modules/quality/... ./internal/modules/directory/... ./internal/platform/jobs
sqlc vet
sqlc generate
```

PASS: cmd/worker 12.5s, quality adapters 91.1s, quality http 41.3s, quality
jobs 35.1s, native 1.2s, prepared 1.1s, quality read 35.1s, quality
application/domain ~1.0s, config 1.0s, official source 5.7s, platform jobs
39.8s, Directory adapters 73.4s, dou 4.0s, geocoder 3.8s, directory http
11.9s, intake 1.0s, directory jobs 18.6s, directory read 97.3s, registry
109.2s, registrysync 2.4s, directory application/domain ~1.0s — all with
race detection. vet/build/staticcheck (unused helper removed) PASS.
Generated files hashed before/after `sqlc generate`: no drift. Five
integration tests that pin migration-head deltas were updated for 000103
(000101–103, 000102–103, 000099–103, 000100–103 and the dynamic 000097+
list). gofmt, `git diff --check` and `scripts/scan-secrets.sh` PASS; the
changed diff contains only synthetic fixtures, no real identifiers, PII or
binary outputs. No new dependency.

## Explicit next obligations

Keep imports OFF and no schedule registered. FQ-05W still owes bounded live
PMQC discovery with durable scan progress, non-starving enqueue, static
Rust worker image/container proof, explicit source scheduling and FQ-06/07
runtime reconciliation. Then FQ-12–17 and the final protected dev→main
batch; D is NOT_AUTHORIZED.

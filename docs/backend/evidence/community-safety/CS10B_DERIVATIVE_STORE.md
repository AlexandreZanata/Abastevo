# CS10B — Derivative persistence and sanitizer adapter

Date: 2026-10-11. Status: LOCAL_DONE. Scope: migration, sqlc
queries, pg inventory store, sanitizer adapter and real-PostGIS
tests only — no job handler/worker scheduling (no queue producer
exists yet; offers arrive via client review in CS11), no byte
namespace, no wall-clock soak (CS10C). Head: maintained `dev` +
this task.
Plan: [phased plan](../../../planning/COMMUNITY_SAFETY_EXECUTION_PLAN.md)
(CS10, AT03/AT05), [CS10A contract](CS10A_PROMOTION_CONTRACT.md).
Rules: B-BR-CS06, BUC-CS05.

Validation: PASS

## Schema (append-only `000057_privacy_training_derivatives.sql`)

- `privacy_training_derivatives`: PK (image_id, consent_revision),
  owner binding, source/derivative hashes, capture instant, frozen
  terms, state `CHECK IN ('ACTIVE','REVOKED')`, promoted/revoked
  instants, plus an owner-state index. No DELETE path; revoked
  receipts stay for the restore filter while physical bytes purge
  in CS10C.

## Store (`privacy/adapters/promotion_store.go`, sqlc `derivatives.sql`)

- Separate `PromotionStore` struct implementing
  `application.PromotionInventory` (compile-time asserted):
  insert-once put converging identical re-proposals and
  conflicting divergent hashes on the natural key (cross-owner
  image collisions conflict, never leak), owner-scoped active
  reads, and the guarded ACTIVE-to-REVOKED mark (repeats converge,
  unknown rows read as not found).
- Regenerated with pinned `sqlc v1.31.1` (`vet` clean, `generate`
  idempotent); stanza lists `000057` explicitly per ownership rules.

## Sanitizer (`privacy/adapters/promotion_sanitize.go`)

- Maps the evidence media pipeline onto the promotion Sanitizer
  port: bounded JPEG validation plus EXIF-free re-encode. Success
  minimizes (PASS); every pipeline refusal maps to FAIL —
  fail-closed with no fallback and no original retention. UNCERTAIN
  stays reserved for a future content classifier; nothing here
  invents one. The media package is stateless and stdlib-only, so
  sharing it crosses no ledger or query ownership.

## Validation (actual, disposable local PostGIS on 127.0.0.1:5434)

- `TestDerivativePutPersistsAndReadsBack`: exact round trip plus
  owner isolation of the list read.
- `TestDerivativePutConvergesAndConflicts`: identical replays,
  divergent hashes conflict.
- `TestDerivativeConcurrentPutsAgree` (10 writers): exactly one
  converged ACTIVE row.
- `TestDerivativeRevokeFlow`: two rows → mark one → active lists
  one; repeat converges; ghost reads as not found.
- `TestDerivativeUpgradeAppliesOnly000057`: chain without 000057,
  seed one consent row, then exactly `["000057"]` applies; table
  present, consent intact; replay idempotent.
- `TestDerivativeQueriesStayNonDestructive`: one converged INSERT,
  one guarded UPDATE, zero DELETE/TRUNCATE/DROP/ALTER, no
  cross-table references, state guard pinned.
- `TestSanitizePassesValidJPEG` + `TestSanitizeFailsClosed`
  (stdlib-generated JPEG; not-an-image/empty/truncated/trailing):
  PASS minimizes, all refusals FAIL with error.
- All 8 pass with `-count=1` under `-race`. Full
  `./internal/modules/privacy/...` unit + integration suites green
  (no regressions). `go vet` (both tag sets), `gofmt -l`, `sqlc
  vet`, `git diff --check` clean; `scan-secrets.sh` passed. Only
  disposable local test databases used.

## Limits / owed (CS10C and later)

- Job handler + worker scheduling + queue producer (offers arrive
  via client review/CS11 or a sweeper): CS10C. Registering a
  handler nothing can enqueue yet would be dead code, so it stays
  out deliberately.
- Private-namespace byte storage, 24-hour physical-deletion proof
  (controlled real soak, periodic inventory, restore drill, SLO
  measure): CS10C — no wall-clock claim here.
- Erasure fan-out into derivatives: CS10C. Collection stays OFF.
- Restricted DB roles: P08 provisioning (query-level owner scoping
  done here).

Next microtask: CS10C — worker, byte namespace, 24 h soak and restore drill.

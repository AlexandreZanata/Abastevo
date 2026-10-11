# CS02A — Helpfulness atomic store (migration + PostGIS)

Date: 2026-10-11. Status: LOCAL_DONE. Scope: migration, sqlc queries,
pg store and real-PostGIS tests only — no HTTP/routes/OpenAPI (CS02B),
no VPS/APK/dependency change. Head: maintained `dev`.
Plan: [phased plan](../../../planning/COMMUNITY_SAFETY_EXECUTION_PLAN.md),
[CS01 contract](CS01_HELPFULNESS_CONTRACT.md). Rules: B-BR-CS02, BUC-CS02.

## Schema (append-only `000054_community_helpfulness.sql`)

- `community_helpfulness_votes`: UUID PK, contributor/observation/
  sequence/choice (`HELPFUL`/`NOT_HELPFUL` check), submission id,
  created/deleted tombstone. Unique live
  (contributor, observation, sequence) partial, unique
  (contributor, submission), live-target index. FK RESTRICT.
- `community_helpfulness_tallies`: PK (observation, sequence),
  non-negative `helpful`/`not_helpful` checks, updated_at.
- `community_observations` gains nullable `hidden_at`/`deleted_at`
  (writers land in CS05/CS09; voting only reads them).

## Store (`community/adapters/pghelpfulness.go`, sqlc `helpfulness.sql`)

Implements the CS01 `HelpfulnessStore` port (compile-time asserted):
target resolution (author/state/visibility/latest sequence),
lock-or-insert cast with same-submission replay, in-place choice
change, tombstone removal, exact tally reads, owner-choice reads,
tally rebuild from live rows, per-contributor erasure. Regenerated
with pinned `sqlc v1.31.1` (`vet` + `generate` clean); generated
`GetObservationRow` widening adapted in `store.go` (`mapObservation`
+ explicit owner-history projection, no behavior change).

Key finding: PostgreSQL evaluates CHECK constraints on the proposed
row **before** `ON CONFLICT` resolution, so a delta upsert (`helpful
+ (-1)`) rejects against the non-negative check even with sufficient
balance — the integration flip test caught this deterministically.
Tallies now move with a plain `UPDATE` (`BumpHelpfulnessTally`); a
missing tally row rebuilds exactly from live rows instead of clamping,
and concurrent first-cast rebuild races retry the whole cast. The
neighbouring `feedback_tallies` table has no non-negative check,
which is why the identical delta-upsert pattern never exploded there
(latent note, not in scope).

## Validation (actual, disposable local PostGIS via `infra/compose.dev.yml`)

- `TestHelpfulnessConcurrentCastsAgree` (20 voters): exact 20/0,
  stored == live == rebuilt.
- `TestHelpfulnessConcurrentFlipsKeepOneRow` (10 flips): one live row,
  stored == live rows (this test exposed the upsert/CHECK bug above).
- `TestHelpfulnessConcurrentRemovalsIdempotent` (5 votes, 10 removes):
  zero totals, absent remove is no-op success.
- `TestHelpfulnessRetryAfterSuccessReplays`: same submission replays
  the committed vote, no inflation (crash-after-commit).
- `TestHelpfulnessSubmissionScopedPerContributor`: a foreign
  submission identity opens a separate vote (no cross-account replay).
- `TestHelpfulnessRebuildRepairsCorruptTally`: corrupted 999/999
  restored to exact counts, stored == rebuild.
- `TestHelpfulnessTargetOwnershipAndEligibility`: unknown fails,
  author resolves with sequence 2, owner cast refuses at app layer,
  hidden resolves ineligible.
- `TestHelpfulnessEraseTombstonesAndAllowsRecast`: erase tombstones
  one vote, tally 1, recast inserts (restore path).
- `TestHelpfulnessUpgradePreservesFacts`: chain without 000054, seed,
  then exactly `["000054"]` applies; tables/columns present; replay
  idempotent.
- `TestHelpfulnessQueriesStayNonDestructive`: new query file holds no
  destructive path (mirrors repo guard style).
- All 10 pass with `-count=1`; concurrent trio also passes under
  `-race`. Full `./internal/modules/community/...` unit green.
- `go vet`, `gofmt -l`, `git diff --check` clean; `scan-secrets.sh`
  passed. `staticcheck` local binary predates go1.27 export format
  (pre-existing env gap; CI runs the pinned version).

## Limits / owed (CS02B and later)

- HTTP routes, signed-proof negatives, OpenAPI schema + apicontract
  vectors: CS02B.
- `CONTRIBUTION` moderation target, hide/delete writers: CS05.
- Erasure footprint wiring into privacy: CS09 path (store method
  ready). Restricted DB roles: P08 provisioning (query-level guard
  done here).
- Helpfulness never touches price confidence/consensus/ranking:
  separate types/tables; feed projection untouched.

Next microtask: CS02B — helpfulness HTTP + OpenAPI.

# CS01 — Helpfulness domain and contract

Date: 2026-10-11. Status: LOCAL_DONE. Scope: pure domain + contract
only — no migration, store, HTTP, route, VPS, APK or dependency change
(CS02 owns those). Head: maintained `dev` at `61f7c1b`.
Plan: [phased plan](../../../planning/COMMUNITY_SAFETY_EXECUTION_PLAN.md),
[CS00 audit](CS00_AUDIT.md). Rules: B-BR-CS01, BUC-CS02.

## Target decision (CS00 owed row, frozen here)

Votes bind to **observation ID + latest decision sequence**
(`community_observations` + `community_observation_decisions_sequence_unique`
from `000008`). Observations are immutable facts with no revision
counter, so the decision sequence is the immutable revision the voter
saw. A vote whose sequence differs from the target's latest refuses
with `ErrStaleSequence`; old-sequence votes stay audit history and
never transfer. Aggregated price cards are never vote targets.

## Added files (owning module `community`, no cross-module imports)

- `backend/internal/modules/community/domain/helpfulness.go`:
  frozen `HELPFUL`/`NOT_HELPFUL` (VALID/INVALID and any other string
  refuse), `StoredVote` with `ContributorRef` binding + `DeletedAt`
  tombstone + `Live()`, `Tally` folding through `ComputeShare`
  (`floor(10000*helpful/total)`, null share on zero, negatives
  refuse), `HelpfulnessRecorded` event. The basis-point math duplicates
  the feedback convention in ~10 lines to preserve module ownership;
  shared vectors are pinned in tests on both sides.
- `backend/internal/modules/community/application/helpfulness.go`:
  `HelpfulnessStore` port (target/cast/remove/tally/my-vote),
  `Cast`/`Remove`/`TallyFor` reusing `Caller`, `ErrUnauthorized`,
  `ErrAccountBlocked`, `ErrTargetNotFound`, `ErrIneligibleTarget`.
  Eligibility = VALIDATED + visible + not deleted. Self-votes refuse
  before any mutation; inactive accounts map onto `ErrAccountBlocked`;
  choice changes rewrite in place; repeats and same-submission retries
  converge without inflating totals; removals are idempotent no-ops.
  `CastResult`/`TallyResult` carry exact counts plus the private
  `MyChoice` overlay; public shapes expose no voter list.
- `contracts/testdata/community-helpfulness/*.json` (4 golden vectors:
  valid cast, invalid VALID choice, tally 2/1 = 6666bp with overlay,
  empty tally with no percentage) pinned by
  `application/helpfulness_fixture_test.go` through the domain
  constructors. Money/product vocabularies stay pinned by the existing
  consensus and apicontract goldens.

## Frozen effects

- Public reference: observation UUID + decision sequence (wire shape in
  the valid cast vector). Owner overlay travels only the caller-bound
  read path, never public caches.
- Privacy/export/erase: votes store the server-resolved
  `ContributorRef`, so account erasure tombstones live votes (store +
  privacy footprint implement this in CS02; no shadow copies).
- Helpfulness never touches confirmations, disputes, VALID/INVALID
  votes, price confidence, consensus, ranking or star ratings
  (separate types, separate errors, `VALID` explicitly refused).

## Validation (actual)

- `go test ./internal/modules/community/... -count=1`: all packages
  ok, including 16 new helpfulness tests (domain 6, application 9,
  fixture 1), 0 failures. RED observed before GREEN (undefined-type
  build failures on both new test files).
- `go vet ./internal/modules/community/...`: clean.
  `gofmt -l`: clean. `git diff --check`: clean.
- `bash scripts/scan-secrets.sh`: passed. No PII/GPS/secrets in
  code, fixtures (synthetic UUIDs only) or this file.
- No PostGIS run: pure phase, no SQL touched. Concurrency proof is
  CS02 work (real-PostGIS races); the store contract documents the
  one-live-row invariant it must hold.

## Limits / owed

- Store atomicity under concurrent casts/flips/removals: CS02
  (migration + real PostGIS + crash/recovery/role/erase tests).
- HTTP routes + OpenAPI schema wiring for these shapes: CS02.
- `CONTRIBUTION` moderation target + report taxonomy: CS05.
- Android strings.xml already labels VALID/INVALID as Helpful/Not
  helpful: CS08/CS11 must give helpfulness cards distinct context.

Next microtask: CS02 — atomic votes, HTTP and projections.

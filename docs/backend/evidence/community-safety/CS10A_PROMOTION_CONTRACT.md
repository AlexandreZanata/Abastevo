# CS10A — Promotion domain and worker contract

Date: 2026-10-11. Status: LOCAL_DONE. Scope: pure domain +
application contract only — no migration, inventory SQL, sanitizer
adapter, worker scheduling, storage purge, soak or APK (CS10B owns
persistence/wiring, CS10C the wall-clock 24 h proof). Head:
maintained `dev` + this task.
Plan: [phased plan](../../../planning/COMMUNITY_SAFETY_EXECUTION_PLAN.md)
(CS10, AT03), [consent contract](../community-safety/CS09A_CONSENT_CONTRACT.md).
Rules: B-BR-CS06, BUC-CS05.

Validation: PASS

## Frozen decisions

- Promotion re-checks the current consent at commit time: a
  revocation landing between queueing and commit refuses with no
  inventory write; stale offline grants never resurrect selection.
- Future-only per-image scope carries over: pre-grant captures and
  opt-out images refuse (`ErrImageNotCovered`); grants never
  authorize old photos retroactively, including across a
  revoke→fresh-grant cycle.
- Minimization verdict is authoritative and fail-closed: FAIL and
  UNCERTAIN refuse identically (`ErrNotSanitizable`), no
  original-photo fallback. Sanitizer outages refuse with no write.
- Derivative rows stay identifier-only (reflect pins): references
  plus source/derivative hashes that bind bytes, never pixels, GPS,
  keys or labels. Source bytes never persist in the promotion path;
  hashes bind them at offer time.
- Inventory converges on the image-plus-revision key: identical
  re-proposals replay, divergent hashes conflict. Owner revocation
  flips ACTIVE→REVOKED tombstones (restore filter reads them) and
  repeats converge to zero; cross-owner rows are untouched.
- `SelectableForTraining` is the single shared filter for worker
  commits and restore replay: active receipt plus currently granted
  consent on current terms, plain denial otherwise.

## Added files (owning module `privacy`, no cross-module imports)

- `backend/internal/modules/privacy/domain/promotion.go`:
  `PromotionCandidate`, `Derivative` (ACTIVE/REVOKED),
  `PromotionReceipt`, frozen `PASS/FAIL/UNCERTAIN` verdicts,
  `ProposeDerivative` / `RevokeDerivative` /
  `SelectableForTraining`, `ErrConsentRequired` /
  `ErrImageNotCovered` / `ErrNotSanitizable`. The sanitizer port
  stays an interface so the real adapter (evidence media) wires in
  CS10B without an import inversion.
- `backend/internal/modules/privacy/application/promote.go`:
  `Sanitizer` + `PromotionInventory` ports, `PromotePorts`
  (embeds the consent ports), `Propose` (commit-time consent
  re-read → fail-closed minimize → converged write) and
  `RevokeOwner` (per-owner flip count for the purge signal).
- `contracts/testdata/privacy-consent/promotion-*.json` (4 golden
  vectors: eligible candidate, pre-grant refusal, uncertain
  refusal, active receipt) pinned by
  `application/promote_fixture_test.go` through the domain.
- Incidental: the CS09A fixture glob narrowed to `consent-*.json`
  so phase vectors coexist in one directory.

## Validation (actual)

- `go test -race -count=1 ./internal/modules/privacy/...`: all
  packages green, including 15 new promotion tests (domain 8,
  application 6, fixture 1), 0 failures. RED observed before GREEN
  (undefined-type build failures plus two contract mismatches the
  tests forced: three-value vs receipt return, embedded ports
  literal).
- `go build ./...`, `go vet`, `gofmt -l`, `git diff --check`
  clean; `scan-secrets.sh` passed. No PII/GPS/secrets in code,
  fixtures (synthetic IDs/hashes only) or this file.
- No PostGIS run: pure phase, no SQL touched. Persistence +
  worker wiring proofs are CS10B work; the wall-clock soak is CS10C.

## Limits / owed

- Derivative inventory migration + guarded store + real-PostGIS
  revision/ownership/replay/concurrency proofs: CS10B.
- Sanitizer adapter over the evidence media pipeline, worker
  scheduling/job kinds, private-namespace storage: CS10B.
- Physical 24-hour deletion proof (controlled real soak, periodic
  inventory, restore drill, SLO measure): CS10C — a virtual-clock
  test is explicitly not claimed as wall-clock evidence here.
- Erasure fan-out into derivatives: CS10B/C. Collection stays OFF.

Next microtask: CS10B — derivative persistence + worker wiring.

# CS09A — Training-consent domain and server contract

Date: 2026-10-11. Status: LOCAL_DONE. Scope: pure domain +
application contract only — no migration, store SQL, HTTP routes,
OpenAPI, worker, S3, APK or dependency change (CS09B owns those).
Head: maintained `dev` at `d7cc5c9` + this task.
Plan: [phased plan](../../../planning/COMMUNITY_SAFETY_EXECUTION_PLAN.md)
(CS09, B-BR-CS06), [consent amendment](../../../security/AI_TRAINING_IMAGE_CONSENT_PLAN.md)
(AT01/AT02/AT05 slice). Rules: B-BR-CS06, BUC-CS05.

Validation: PASS

## Frozen decisions

- Purpose frozen as `TRAINING`; current terms `training-terms-v1`
  with a bounded recorded digest. Rotation invalidates old grants
  until a fresh explicit grant lands (`ErrTermsOutdated`); stale
  digests refuse the same way.
- Absent consent is default OFF (denies selection, no error). There
  is no stored OFF state; `REVOKED` is the explicit OFF with history.
- Revision is the per-owner monotonic operation identity (singletons
  need ordering, not the multi-row submission keys of
  export/deletion intents): strictly greater moves state, equal +
  identical converges (`Replayed`), equal + divergent conflicts
  (`ErrConflict`), smaller is stale (`ErrStaleConsent`).
- Grant covers only future per-image selections: pre-grant captures
  are never retroactively authorized, even with the per-image flag.
  `OFF` stops selection at the revocation receipt and signals owned
  training-asset deletion (CS10 owns the purge + 24 h proof).
- Owner binding is `ContributorID + ContributorRef` throughout;
  mismatch reads as not found (no ownership oracle), anonymous as
  unauthorized. Another account/key/origin can never act as owner.
- Consent and image-selection rows stay identifier-only (reflect
  pins): no pixels, GPS, keys or image bytes.

## Added files (owning module `privacy`, no cross-module imports)

- `backend/internal/modules/privacy/domain/consent.go`:
  `GrantConsent` / `RevokeConsent` / `AuthorizesImage` plus
  `Consent`, `ImageSelection`, `ConsentReceipt`,
  `ErrTermsOutdated`, `ErrStaleConsent`. Revoking absent consent
  establishes the `REVOKED` baseline so a later stale grant cannot
  slip through; re-grant after revoke needs a fresh higher revision
  on current terms.
- `backend/internal/modules/privacy/application/consent.go`:
  `ConsentStore` (revision-guarded singleton port),
  `TermsProvider`, `GrantConsent` / `RevokeConsent` /
  `ReadConsent` / `CanSelectTraining`. Fresh revocation returns
  `DeleteOwnedTrainingAssets`; repeats converge without a duplicate
  signal. `CanSelectTraining` is the promotion gate CS10 consumes;
  denial is `(false, nil)`, never an error.
- `contracts/testdata/privacy-consent/*.json` (4 golden vectors:
  valid grant, stale-terms refusal, granted status, default-OFF
  status) pinned by `application/consent_fixture_test.go` through
  the domain constructors.

## Validation (actual)

- `go test ./internal/modules/privacy/... -count=1`: all packages
  ok, including 20 new consent tests (domain 10, application 9,
  fixture 1), 0 failures. RED observed before GREEN (undefined-type
  build failures on both new test files).
- `go test -race -count=1 ./internal/modules/privacy/...`: green,
  including concurrent revokes converging under `-race`.
- `go vet ./internal/modules/privacy/...`: clean. `gofmt -l`:
  clean. `go build ./...`: ok.
- `git diff --check`: clean. `bash scripts/scan-secrets.sh`:
  passed. No PII/GPS/secrets in code, fixtures (synthetic IDs only)
  or this file.
- No PostGIS run: pure phase, no SQL touched. Migration + real-DB
  revision/ownership/replay tests are CS09B work; HTTP/OpenAPI +
  signed owner routes likewise.

## Limits / owed

- Store SQL (append-only receipt/inventory rows, upgrade/recovery,
  least-privilege) + real-PostGIS revision/ownership/replay proofs:
  CS09B (server persistence).
- Signed owner grant/revoke/read/delete HTTP + OpenAPI: CS09B.
- Promotion worker, 24-hour physical-deletion SLO proof, restore
  drill, erasure integration: CS10. Collection stays OFF until then.
- Settings/terms wiring (CS11) and device matrix (CS14) unchanged.

Next microtask: CS09B — consent persistence (migration + guarded
store) and signed HTTP + OpenAPI.

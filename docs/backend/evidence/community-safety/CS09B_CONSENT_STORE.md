# CS09B — Training-consent persistence (migration + guarded store)

Date: 2026-10-11. Status: LOCAL_DONE. Scope: migration, sqlc
queries, pg store and real-PostGIS tests only — no HTTP/routes/
OpenAPI (CS09C), no worker/S3 purge (CS10), no VPS/APK/dependency
change. Head: maintained `dev` + this task.
Plan: [phased plan](../../../planning/COMMUNITY_SAFETY_EXECUTION_PLAN.md)
(CS09, B-BR-CS06), [CS09A contract](CS09A_CONSENT_CONTRACT.md). Rules:
B-BR-CS06, BUC-CS05.

Validation: PASS

## Schema (append-only `000056_privacy_training_consent.sql`)

- `privacy_training_consents`: `contributor_id` TEXT PK (singleton
  per owner), `contributor_ref`, purpose `CHECK = 'TRAINING'`,
  `terms_version`/`terms_digest` (digest 1–128 chars), state
  `CHECK IN ('GRANTED','REVOKED')`, `revision BIGINT CHECK >= 1`,
  `granted_at`/`revoked_at`/`decided_at`. No DELETE path, no sibling
  table touched; absent rows deny by default at the application layer.

## Store (`privacy/adapters/consent_store.go`, sqlc `consent.sql`)

- Separate `ConsentStore` struct (own pool wiring, like
  `HelpfulnessStore`): the shared `Store` already owns a conflicting
  `Get` signature, so the singleton gets its own type. Compile-time
  asserts `application.ConsentStore`.
- `GetTrainingConsent` + `UpsertTrainingConsent` (single guarded
  `ON CONFLICT ... DO UPDATE ... WHERE excluded.revision >
  stored.revision`). A no-row upsert resolves against the stored row:
  identical retries converge (`replayed`), divergent same-revision
  writes conflict, smaller revisions refuse stale — a racing stale
  offline ON can never undo a revocation. Owner reads share one
  not-found shape (no oracle).
- Regenerated with pinned `sqlc v1.31.1` (`vet` clean, `generate`
  idempotent); stanza lists `000056` explicitly per ownership rules.

## Validation (actual, disposable local PostGIS on 127.0.0.1:5434)

- `TestConsentGrantPersistsAndReadsBack`: exact round trip.
- `TestConsentRevisionGuardConverges`: identical replays, divergent
  conflicts, older is stale, greater revocation wins (state REVOKED
  rev 3).
- `TestConsentConcurrentUpsertsAgree` (10 revisions): surviving row
  keeps an exact revision/instant pair.
- `TestConsentOwnerIsolation`: absent/foreign read as not found.
- `TestConsentApplicationFlowOnPostgres`: real-store application
  flow — grant 1 → status granted → revoke 2 (purge signal) →
  stale grant at 2 refuses → selection denied.
- `TestConsentUpgradeAppliesOnly000056`: chain without 000056, seed
  one export request, then exactly `["000056"]` applies; table
  present, history intact; replay idempotent.
- `TestConsentQueriesStayNonDestructive`: one guarded upsert, zero
  DELETE/TRUNCATE/DROP/ALTER, no cross-table references, revision
  guard pinned.
- All 7 pass with `-count=1` under `-race`. Full
  `./internal/modules/privacy/...` unit + integration suites green
  (no regressions). `go vet` (both tag sets), `gofmt -l`,
  `git diff --check` clean; `scan-secrets.sh` passed. Only
  disposable local test databases used.

## Limits / owed (CS09C and later)

- Signed owner grant/revoke/read HTTP + OpenAPI/apicontract: CS09C.
- Promotion gate worker, 24-hour physical-deletion proof, restore
  drill, erasure fan-out: CS10. Collection stays OFF.
- Restricted DB roles: P08 provisioning (query-level owner scoping
  done here, same reservation as CS02A).
- Per-image inventory rows arrive with promotion (CS10); no stored
  image scope in this task by design.

Next microtask: CS09C — consent signed HTTP + OpenAPI.

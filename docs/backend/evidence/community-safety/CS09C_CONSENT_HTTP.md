# CS09C — Training-consent signed HTTP and OpenAPI

Date: 2026-10-11. Status: LOCAL_DONE. Scope: owner HTTP routes,
composition wiring, config digest, OpenAPI schemas and contract
vectors only — no migration/store change (CS09B), no worker/S3 purge
(CS10), no VPS/APK/dependency change. Head: maintained `dev` + this
task.
Plan: [phased plan](../../../planning/COMMUNITY_SAFETY_EXECUTION_PLAN.md)
(CS09, B-BR-CS06), [CS09A contract](CS09A_CONSENT_CONTRACT.md),
[CS09B store](CS09B_CONSENT_STORE.md). Rules: B-BR-CS06, BUC-CS05.

Validation: PASS

## Routes (`privacy/adapters/http`, same proof/envelope conventions)

- `POST /v1/privacy/training-consent/grant` → 200 with decision
  receipt; auth required. Identical retries converge with
  `replayed: true`.
- `POST /v1/privacy/training-consent/revoke` → 200 with receipt plus
  `delete_owned_training_assets` (true only on the fresh revocation
  that requests the purge); auth required.
- `GET /v1/privacy/training-consent` → 200 owner-only view
  (`granted/state/terms_version/terms_digest/revision/decided_at`,
  `needs_regrant` after rotation; absent consent reports
  `granted: false` with no state); auth required.
- Bodies: 64 KiB cap, strict decode (unknown/duplicate/trailing
  rejected), UTF-8 enforced, revision in canonical JSON number form
  (strings/floats/exponents refuse even when numerically equal),
  frozen terms pair checked at the wire and again in
  domain/application.
- Errors: `privacy.auth-required` 401 (anonymous, before any body
  work), `privacy.bad-body` 400, `privacy.terms-outdated` 400,
  `privacy.stale-revision` 409, `privacy.conflict` 409. Responses
  carry consent fields only — no contributor ID, token, key, device
  material — and are `no-store`.
- Wiring: `cmd/api/main.go` mounts the real commands over
  `ConsentPorts` (guarded `ConsentStore`, server clock, registrar
  attribution, configured terms pair) with the same
  verifier-derived caller shape as community writes. Config gains
  `ANPFUEL_TRAINING_TERMS_DIGEST` (validated lowercase hex,
  provisional default binding `training-terms-v1` until the verified
  terms text lands in CS11).

## Live-clock correction (found by the e2e test, fixed here)

The first e2e run returned 409 on the identical grant replay: each
attempt stamps a fresh server `DecidedAt`, so equal-revision retries
diverged under a live clock (frozen-clock unit tests never saw it).
The revision is the decision identity; attempt instants are server
stamps. `GrantConsent` now converges same-revision/same-state/same-
terms retries regardless of instant (stored receipt returned), and
the pg resolve compares decisions modulo instants. CS09A wording is
superseded accordingly: divergent means different state/terms, not a
new instant. `ConsentView` additionally carries `terms_digest` (found
missing while wiring the status response) so clients can present the
current pair back.

## Validation (actual)

- Unit `adapters/http`: 8 tests green (3 happy paths, anonymous
  401 ×3 with frozen code, 12 malformed bodies incl. non-UTF8,
  oversize and string revision, 4×2 error mappings, failure-shape
  guard). String-revision refusal and the live-clock replay were RED
  before GREEN.
- E2E `adapters/http` (router + real application + pg store on
  disposable PostGIS): grant 200 → replay converges → owner status
  shows grant → anonymous status 401 → revoke 200 with purge signal
  → stale grant 409 → old terms 400 with revocation intact;
  no-store asserted. Full package green under `-race`.
- CS09A/CS09B suites re-green after the replay refinement
  (domain/application unit + full privacy integration, `-race`).
- `apicontract`: 4 new vectors validate against the new
  `TrainingConsent*` schemas (spec self-check + local refs ok).
- `go build ./...`, `go vet` (both tag sets), `gofmt -l`,
  `git diff --check` clean; `scan-secrets.sh` passed. Only
  disposable local test databases used. Local staticcheck predates
  the go1.27 export format (pre-existing env gap, CI pinned).

## Limits / owed

- Promotion worker, 24-hour physical-deletion proof, restore drill,
  erasure fan-out: CS10. Collection stays OFF.
- Verified terms text + Settings wiring: CS11 (replaces the
  provisional digest default with the published digest).
- Unknown-key/tampered-signature proof negatives: generic
  cmd/api verifier path, unchanged here (community signed-proof
  suites own that coverage).
- Restricted DB roles: P08 provisioning.

Next microtask: CS10 — promotion, 24-hour expiry and physical deletion.

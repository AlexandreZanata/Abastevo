# P36-T01 — Staging account and provider integration

Status: LOCAL_DONE on `codex/phase-36-live-community`. Task: P36-T01.
Binds P13 free-account rules (no paid registration gate) and B-BR-011
(no provider credentials in Git). Verification slice: P34-T02 already
wired auth/anonymous/community/feedback/contribution DI to the shared
staging origin; this task locks that wiring and records the owed
provider/device proof. No credential, deployment or real-user action.

## Behavior (locked, not rebuilt)

- `NetworkModule.provideApiEnvironment()` returns `STAGING`
  (`https://teste.abastevo.com.br`); release stays explicit and
  separately certified.
- All six backend client bindings (auth, anonymous, community prices +
  votes, feedback, contribution, directory) resolve their base URL from
  the injected `@Named("apiOrigin")` environment. New regression guard
  `di/StagingOriginWiringTest` fails any future binding that drops the
  shared origin, and fails any placeholder that stops being
  non-resolving RFC 2606 `.invalid`.
- `AuthViewModel` keeps safe failure presentation for staging: invalid /
  wrong / expired / locked codes, session expiry, suspension, deletion,
  offline, provider denial and login-first states (existing
  `AuthUiError` mapping, untouched).
- Provider checklist (local ownership, nothing in Git): no
  `google-services.json` / `GoogleService-Info.plist` in tree (both
  absent; `google-services.json` gitignored); `git grep` for
  client-secret/API-key/private-key patterns finds nothing;
  `scan-secrets.sh` PASS. Real provider delivery (email-code SMTP,
  Google/Apple client IDs, redirects, cert/signature config) is
  explicitly OWED — mocks and unit suites are insufficient proof per
  plan, so no provider acceptance is claimed.

## Validation

- NEW guard: `./gradlew :data:testDebugUnitTest --tests
  ...StagingOriginWiringTest` → BUILD SUCCESSFUL (3/3: construction
  origin, every binding takes shared origin, placeholders
  non-resolving). No RED: guard locks existing P34-T02 behavior by
  design (recorded honestly, not a behavior change).
- Regression: `:app:testDebugUnitTest --tests ...AuthViewModelTest`
  PASS (existing 13); backend `go test ./internal/modules/account/...
  ./internal/modules/identity/...` PASS (11 pkgs incl. http/oidc/
  keyprover/application); `vacuum lint` + `apicontract` PASS
  (unchanged contracts).
- Live (no bypass): `curl` 60 unchanged (Fortinet middlebox CA);
  staging auth/provider JSON UNVERIFIED, no `-k`/trust-all/HTTP.
- `git diff --check` PASS; secret-surface review PASS.

## Limits and next

- BLOCKED_LIVE: staging account/provider reads/writes unverified from
  this runner (TLS trust gap). OWED: real provider delivery/client
  IDs/redirects + device callback proof at the end manual/device batch.
- OWED: end manual/device batch, media proof. No PR/CI/merge/wiki per
  ADR-018. iOS archived. G09 UNCERTIFIED.
- Next: P36-T02 station/fuel feedback screens (canonical UUID wiring)
  on this branch.

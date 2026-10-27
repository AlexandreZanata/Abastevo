# C3 — App: claim and profile self-service

Date: 2026-10-27. Branch `codex/adm-c3-app-contributor-self` stacked on C2
(`codex/adm-c2-landing-contributors`, PR #174), which stacks on C1 (PR #173
to `main`). Merge order: #173 → #174 → C3.

## Delivered

- Domain (`domain/contributor`): `Claim`, `ClaimState`/`ClaimProof` with
  `UNKNOWN` fallback, `IssuedClaim`, `ProfileEdit`/`StoredProfile`/
  `ProfileRead`, `ProfileVisibility`; validators mirroring the server
  (`isValidGithubLogin` CTB-03, `isValidGistId`, `displayNameError`,
  `isValidProfileLink` against the CTB-08 host allowlist, `profileEditError`:
  ≤2 links, one per platform, PUBLIC requires consent).
  `ContributorException` gains `Forbidden`/`Conflict`/`Quota`.
  Port `ContributorSelfGateway`.
- Data: `ContributorSelfHttpClient` against the A3 user routes
  (`POST /v1/contributor/claims`, `/claims/read`, `/claims/{id}/proof`,
  `/claims/{id}/withdraw`, `/profile/read`, `/profile`), every body carries
  `family_id` + `access_token`; claim create sends a fresh `operation_id`.
  No redirects, no retries, 20 s call timeout, 256 KiB body budget, claim ids
  must be canonical UUIDs. Status mapping: 400 invalid, 401 auth-required,
  403 forbidden, 404 not-found, 409 conflict (server code kept, e.g.
  `contributor.claim-state`), 429 quota, else unavailable. Hilt providers in
  `ContributorsModule` + six use-case providers in `UseCaseModule`.
- Application: six use cases. Each refuses before any network call when
  signed out (`contributors.signed-out`) and when the input fails the client
  validator (`contributors.invalid-login|gist|display-name|links|consent`).
- App: `ClaimScreen` (login → one-time code card with copy button and gist
  instructions → gist ID proof → status list, withdraw while pending) and
  `ProfileEditScreen` (name, two platform/handle rows, visibility switch,
  explicit consent checkbox, show-roles switch, current roles). Entry button
  "I am a contributor" on the contributors list; routes
  `contributor-claim` and `contributor-profile`. Strings in 8 locales
  (`strings_contributor_self.xml`).

## Server shapes used (A3, not invented)

Claim create → `201 {claim_id, code}`; claim object `{claim_id,
github_login, state, proof_status, gist_id?, override, created_at,
expires_at, decided_at?}`; profile read/upsert → `{profile|null, roles}` with
`profile = {display_name, links[{platform, handle, url}], visibility,
public_consent, show_roles_publicly}`.

Known quirk (documented, not changed here): admin approve/reject/hide
responses use capitalized Go-default JSON keys while list views use
lowercase. The B2 web panel normalizes both; the app only calls the user
routes, which use the lowercase shapes above.

## Fixes made while closing the gate

- `ClaimContent` had no `onNavigateBack` parameter (compile error).
- `ProfileEditScreen` save button passed `modifier` twice (compile error)
  and two LazyColumn items shared the key `roles` (runtime crash whenever
  the account has roles) — keys are now `show-roles` and `roles-list`.
- After a successful "request code" the state was rebuilt with the default
  `loading = true`, leaving the screen on the skeleton; now `loading = false`
  (asserted in `open issues the code and reloads`).
- Client validators existed but were never called; the use cases now refuse
  invalid input before any private call (`invalid input refuses before any
  private call`), the ViewModel maps it to its own notice, and the profile
  form shows localized errors instead of raw `name`/`consent` tokens.
- The server returns the one-time code on own reads while a claim is
  PENDING, but the client dropped it: after dismissing the code card the user
  could not see it again and a new claim hits 409 (one open claim). The
  pending row now re-displays the code with a copy button.
- Signed-out users now see "Sign in to …" instead of a bare "Access
  refused."; the display-name hint shows the straight apostrophe the server
  actually accepts.

## Tests

- Domain `ContributorSelfValidationTest` (6): login, gist id, display name
  bounds/charset by code point (combining marks and supplementary letters
  accepted like the server), link allowlist, edit rules, state parsing.
- Data `ContributorSelfHttpClientTest` (6): claim proof body + code parse,
  own-claims read keeps the pending code,
  proof/withdraw routes, profile upsert body (links, consent, show-roles),
  401 denied / 403 forbidden / 409 conflict / 429 quota mapping, 409 keeps
  the server code.
- Application `ContributorSelfUseCasesTest` (4): signed-out refusal before
  any call, claim and profile flows carry the session proof, invalid input
  refused before any call.
- App unit `ClaimViewModelTest` (6), `ProfileViewModelTest` (3): load, open
  + reload, quota/conflict/offline/forbidden/invalid mapping, denied,
  proof/withdraw reload, saved flag.
- androidTest `ClaimScreenTest` (5), `ProfileEditScreenTest` (3): copy
  button, login capture, proof/withdraw, decided claim hides the form,
  pending row re-displays its code,
  prefill + roles, public without consent never saves, consented save.
  Pure `*Content` functions with hand fakes; no new dependencies.

## Commands and results

```sh
./gradlew :domain:test :data:testDebugUnitTest :application:test \
  :app:testDebugUnitTest :app:lintDebug :app:assembleDebug \
  :app:assembleDebugAndroidTest --offline
# BUILD SUCCESSFUL
# domain 787 tests, 0 failures; data 635, 0 failures (4 pre-existing skips);
# application 488, 0 failures; app 570, 0 failures
# lintDebug: no issue in any C3 file (284 pre-existing warnings elsewhere)
# app-debug.apk and app-debug-androidTest.apk assembled
git diff --check            # empty
```

The first gate run exposed two failures in the then-uncommitted C3 tests:
the HTTP test still expected 403 → `Denied` after `Forbidden` was introduced
(the plan requires 403 to map distinctly), and the display-name validator
counted UTF-16 chars instead of code points and rejected combining marks the
server accepts. Both were fixed in the code/test pair, not by weakening.

## Not done

- androidTest compiled only; the attached device refuses installs
  (`INSTALL_FAILED_USER_RESTRICTED`), so device runs happen in CI.
- C4 staging check: blocked (staging is k3s without A1–A4; the admin key
  stays with the user).

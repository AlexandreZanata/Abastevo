# P22-T01 — Progressive free account journey

Status: LOCAL_DONE on `codex/phase-22-social-moderation`. Issue: #86. Entry G21 satisfied (P21 merged `f24afd1`, issues #81–#84 CLOSED, wiki `9fbaf9b`); P14/P17 contracts integrated. Binds B-BR-C05 and BUC-C01/C03 to existing contracts. No backend change, no migration, no new permission.

## What this task adds

- `AuthViewModel.onDeleteAccount()`: self-deletion via the real `POST /v1/accounts/deletion` plus local storage wipe inside `AuthFlow.deleteAccount()`. Success returns to email entry with the Deleted notice; failure keeps the session and maps the verdict (suspended stays suspended, never silently logged out).
- `AuthScreen`: explicit two-step delete (button → `AlertDialog` confirm → delete), en + pt-BR copy stating reported prices stay anonymous (consistent with B-BR-011: payloads carry no contributor id).
- `ProfileScreen`: session-aware account card reusing the shared `AuthViewModel` store (guest card vs signed-in card + Manage account → AUTH). No ceremony duplication; rehydrate runs once per Perfil instance.

## Reused and verified (not rebuilt)

Email-code/Google/Apple attempt + `anpfuel://auth/callback` completion, session refresh/rotation, rate-limit/quota, suspension/deletion verdicts, `Keystore` session store — all P14-owned and covered below.

## Auth-critical gate (real PostGIS, `-race`)

- Backend `account` + `identity` modules, unit + `-tags=integration` vs tmpfs PostGIS 18-3.6 (disposable container, removed): all packages ok — registration/deletion/rotation/quota/retention/oidc/keyprover/profile. No product backend change; the gate proves the deletion/sessions this UX drives.

## Explicitly deferred (not waived)

- Data export: no export endpoint exists; it needs a backend extension (API + privacy review) before any consumer — deferred, not silently dropped.
- Real Google/Apple provider proof: needs supported device + Play Services/Apple SDK; mocks insufficient by the plan's own rule → P24-owned device evidence. Provider denial paths stay unit-tested (`oidc-*`/`link-*` → ProviderDenied).
- Strings en + pt-BR; other locales fall back to en.

## Validation

- `AuthViewModelTest` 13/0-fail (2 new: delete ok/err).
- Backend account/identity suites above, 0 failures.
- `./gradlew :app:testDebugUnitTest :app:assembleDebug` (affected suites).
- `git diff --check` clean; scoped secret review (no secrets/PII/GPS/photo content).

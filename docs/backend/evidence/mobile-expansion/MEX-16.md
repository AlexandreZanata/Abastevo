# MEX-16 — Premium status, acquisition boundaries and provider handoff

Status: LOCAL_DONE
Validation: PASS
Integration: INTEGRATION_PENDING

Parent: `a5c4ed37` (maintained dev, MEX-15). Date: 2026-10-23.
Depends MEX-08/15; owns PRM09-T02/T08 and PRM10 handoff, not sales
activation. Rules B-BR-MEX-01/02/05/20; BUC-MEX-01/04/10. Risk:
entitlements/privacy.

Before behavior: a signed-in owner sees the server-verified Premium
state, capabilities, paid-through date and retained-data guarantee.
The app never derives an entitlement from local state, payment flags,
account text or a previous screen. Manage opens the provider's
subscription page. Restore is declared unavailable in this build
because no Play Billing token source is shipped; no purchase, token
or local grant is fabricated. Missing provider configuration and
outages are explicit, and a late response cannot render another
account's status.

## Delivered

- `domain/premium/PremiumStatus.kt`: closed state vocabulary with
  explicit UNKNOWN for forward-compatible values, bounded capability
  set and the frozen management URL.
- `GetPremiumStatusUseCase`: refuses without a session before any wire
  call and binds the result to the session account.
- `PremiumStatusHttpClient`: POST `/v1/premium/status` with only
  family/access proof, `no-store`, no redirects/retry, 64 KiB budget,
  strict policy/state/capability/expiry/retention validation and typed
  denied/unavailable/invalid failures.
- Account screen Premium card: verified state, paid-through date,
  retained-data copy, manage action, honest restore-unavailable
  notice, offline/denied/error states and manual refresh. 18 strings
  in all eight locales.
- No backend, schema, dependency, purchase or provider activation.

## Actual checks

- RED: corrupt JSON escaped as `JSONException` instead of the typed
  invalid failure; the client test reproduced it before the fix.
- Data tests (5): session-only request, ACTIVE capabilities, UNKNOWN
  retention, expired/null expiry, 401/503, retained-data/management/
  capability/oversize/corrupt refusals.
- Application tests (2): signed-out no-call and owner-bound result.
- App tests (5): signed-out no-call, active render, offline/denied
  without stale grant, account switch discards the old status,
  restore stays unavailable while keeping the verified status.
- `:domain:test :application:test --offline` targeted Premium PASS.
- `:data:testDebugUnitTest` Premium (5) and `:app:testDebugUnitTest`
  account Premium (5) PASS.
- `:app:assembleDebug :app:lintDebug --offline` PASS (zero lint
  errors).
- `git diff --check` and secret scan PASS; no PII/GPS/tokens in
  source, tests or evidence.

## Limits

- Play Billing acquisition/token retrieval is not implemented. The
  backend verify/restore contract exists, but this build cannot obtain
  a purchase token, so restore is explicitly unavailable. Paid
  acquisition remains OFF.
- Sandbox purchase/renewal/lapse/provider-outage proofs are
  EXTERNAL_PROOF_OWED and require separate authority. No purchase was
  made and no production grant exists.
- Device/native rendering remains OWED under the standing deferral.

Next: EE-24 EV sync/backup/privacy fields from this checkpoint.

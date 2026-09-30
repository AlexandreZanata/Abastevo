# P13-T05B — Android Keystore store + HTTP API adapter + Hilt

Status: LOCAL_DONE on `codex/phase-13-free-accounts` (phase PR #27,
draft). Issue: #26 (slice 2 of P13-T05). Device evidence stays
deferred to release per owner decision. No live backend contact
(the preview base URL never resolves).

## Slice acceptance (frozen before coding)

- T05B (this commit) — native Android boundary: AES/GCM session
  envelope (pure, JVM-tested), Keystore-backed `AuthSessionStore`
  with injectable key seam, OkHttp `AuthAccountApi` over the frozen
  backend paths with verbatim verdict mapping, Hilt `AuthModule`
  bindings. Zero new dependencies (framework KeyStore, pre-existing
  okhttp/org.json/mockwebserver/datastore) — nothing to
  license-review because nothing was introduced.
- T05C (next) — screens + app navigation wiring.
- Later — Keychain adapter + macOS run (release horizon).

## What changed

- `data/.../local/auth/SessionEnvelope.kt` — envelope
  `{"v":1,"iv","ct"}` (12-byte IV, 128-bit tag, 64KiB cap);
  tamper/wrong-key/malformed/version/oversize all decode to null,
  never throw, so rehydrate lands logged-out.
- `data/.../local/auth/KeystoreSessionStore.kt` — AES key in
  AndroidKeyStore (API 26+, no user-auth gating that would strand
  background refresh), sealed blob in private prefs;
  `SessionKeyProvider` seam keeps JVM suites real (in-memory AES).
  The ~30 KeyStore boilerplate lines cannot run on JVM: statically
  reviewed + lint-guarded, behavior suites cover the rest.
- `data/.../local/auth/AccountHttpApi.kt` — blocking OkHttp over
  email/consume/refresh/revoke/link/deletion; `account.` prefix
  stripped to portable verdicts; transport/parse failures and
  non-envelope errors map to `UNAVAILABLE` (retryable, never an
  auth refusal); RFC3339 converted to epoch at the boundary.
- `domain/.../portable/PortableAuth.kt` — additive `UNAVAILABLE`
  const only.
- `data/.../di/AuthModule.kt` — Hilt singletons (prefs, keys,
  store, api with 10s timeouts, flow with wall clock + UUID
  nonces); base URL is an explicit `.invalid` preview placeholder.
- Tests: `SessionEnvelopeTest` (5: roundtrip, IV uniqueness,
  tamper, wrong key, malformed matrix), `KeystoreSessionStoreTest`
  (5: roundtrip, clear, rotation unreadable, corrupt, unavailable
  keystore), `AccountHttpApiTest` (7: request/consume/refresh/
  revoke/delete/link shapes, 6-vector envelope matrix,
  transport+protocol failures).

## Dependency review (recorded need/license/security)

- Added: none. okhttp 4.12.0 (Apache-2.0), org.json test stub and
  mockwebserver (same pins) pre-date this slice; AndroidKeyStore,
  `java.time`, `javax.crypto` are platform APIs (minSdk 26).
- Secret surface: `scan-secrets.sh` + PR-diff scan clean; no token
  or key material in logs, tests, or fixtures (synthetic values).

## RED → GREEN

- RED proven by dropping the `account.` prefix strip: the
  6-vector envelope matrix fails (full codes leak to UI mapping);
  GREEN on restore.

## Validation (exact commands, this host)

```sh
./gradlew :domain:test :application:test :data:testDebugUnitTest :app:testDebugUnitTest :app:assembleDebug --no-daemon
make quick-verify
git diff --check
```

Outcome: 17 new data suites green (plus 16 T05A, baseline
intact); full Android baseline + `assembleDebug` SUCCESS;
`quick-verify` selection=full ok; `diff --check` clean; secrets
scan clean.

## Limits (not claimed)

- KeyStore boilerplate runs on device only (reviewed, not
  executed here); Keychain adapter + macOS run stay release.
- Preview base URL resolves nowhere; no screens yet (T05C).
- Live providers stay P13-T05/G13 device evidence (deferred).

## Rollback

Pure addition under `data/.../local/auth` + `data/.../di`:
deleting those files plus the `UNAVAILABLE` const restores the
tree. No migration, no flag, no manifest change.

## Next

P13-T05C screens + navigation (#26, same branch). Issue #26 stays
open until the phase PR merges.

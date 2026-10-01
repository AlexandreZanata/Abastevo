# P10-T03 — Device keys and anonymous protocol

Status: LOCAL_DONE on `codex/phase-10-functional-integration` (phase PR
#54 pending, third task push). Issue: #47 (slice 3 of 8 local;
P10-T09 pilot stays deferred until G18/G09 and is not part of G10-LOCAL).
No ANP path touched, no legacy enum rename, no backend change.

## Slice acceptance (frozen before coding)

- T03 (this commit) — Keystore P-256 keys and the agreed frozen proof on
  supported devices: portable `PortableAnonymousProof` (exact 8/10-line
  covered set/order, REGISTER/SIGN, `fp:<hex>` shape, nonce binding,
  300s window, canonical rotation intent + thumbprint inputs) +
  `AnonymousContributionConfig` default OFF; application
  `AnonymousDeviceFlow` (Disabled / MissingKey-honest-notice /
  Ready-public-only, nonce bind + single-spend replay guard, rotation
  intent, `markKeyLost` unrecoverable) behind
  `AnonymousDeviceKeyPort`; data `AnonymousProofCrypto` (SHA-256/SHA-512,
  raw `R || S` over the digest via `NONEwithECDSA`, DER refused by
  length, on-curve P-256 check, frozen-vector verifier) +
  `AndroidAnonymousDeviceKeys` (AndroidKeyStore `secp256r1`, PURPOSE_SIGN,
  no auth gate, no export/backup, fails closed) + preview `.invalid`
  `AnonymousProofHttpClient` (challenge/register/rotate, 64 KiB cap,
  network-only); `AnonymousContributionFlagStore` default OFF; DI via
  `AnonymousModule` + `RepositoryModule`/`UseCaseModule` bindings.

## What changed

- `domain/.../portable/PortableAnonymousProof.kt` — pure covered-set,
  shape, window, intent and thumbprint rules (no `java.*`).
- `domain/.../feature/AnonymousContributionConfig.kt` — DISABLED default.
- `application/.../port/AnonymousContributionFlagProvider.kt` +
  `port/AnonymousDeviceKeyPort.kt` — flag + custody/signing boundary.
- `application/.../usecase/identity/AnonymousDeviceFlow.kt` + outcome
  (Disabled/MissingKey/Ready).
- `data/.../local/auth/AnonymousProofCrypto.kt` — JVM-testable frozen
  verifier, fingerprint, DER converters, content-digest helper.
- `data/.../local/auth/AnonymousDeviceKeys.kt` — Keystore custody
  (lint-guarded, device-tested on CI).
- `data/.../local/preferences/AnonymousContributionFlagStore.kt` —
  default OFF.
- `data/.../remote/AnonymousProofHttpClient.kt` — bounded
  challenge/register/rotate envelopes.
- `data/.../di/AnonymousModule.kt`, `RepositoryModule` binds,
  `UseCaseModule` flow wiring.
- Tests: 4 portable shape/window + 4 flow (disabled/missing/ready/
  rotation-loss) + 4 crypto (11 frozen vectors, fingerprint/DER,
  generated-key round-trip + reinstall refusal, DER round-trip) + 3 HTTP
  (challenge/register/rotate paths, down/empty) + 1 androidTest device
  sign/verify (Keystore P-256 on API 26+).

## RED → GREEN

- RED proven by new symbols before implementation (same pattern as
  P10-T01/T02): `PortableAnonymousProof`, `AnonymousDeviceFlow`,
  `AnonymousProofCrypto`, `AnonymousProofHttpClient` did not exist;
  new tests referenced them. Interop RED: the verifier first ran
  against the 11 frozen vectors and only passed after the exact
  covered-order, window, keyid-binding and raw-signature rules matched
  the Go profile.

## Validation (exact commands, this host)

```sh
./gradlew :domain:test :application:test :data:testDebugUnitTest --no-daemon --rerun-tasks
./gradlew :app:testDebugUnitTest :app:assembleDebug --no-daemon
bash scripts/check-mobile.sh --static-only
git diff --check
bash scripts/scan-secrets.sh
```

Outcome: `:domain:test` + `:application:test` + `:data:testDebugUnitTest`
green (incl. 15 new unit suites listed above: crypto replays all 11
frozen vectors, 2 valid pass / 9 tamper fail); `:app:testDebugUnitTest`
+ `assembleDebug` BUILD SUCCESSFUL (ANP screens unchanged);
`check-mobile --static-only` ok; `diff --check`/secrets clean.
Limits: `:data:connectedDebugAndroidTest` NOT run locally (no
emulator); Keystore device sign/verify test added for CI/phase exit.
No backend change; no production URL invented.

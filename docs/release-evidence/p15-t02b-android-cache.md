# P15-T02B — Android sealed transient cache + Hilt

Status: LOCAL_DONE on `codex/phase-15-lightweight-photos` (phase PR
#39, draft). Issue: #35 (slice 2 of P15-T02; issue stays open
until the phase PR merges). No camera capture, backend, migration
or live-provider contact in this slice.

## Slice acceptance (frozen before coding)

- T02B (this commit) — native Android boundary: `PhotoBlobSeal`
  AES/GCM envelope (`{"v":1,"iv","ct"}`, 12-byte IV, 128-bit tag,
  400 KiB cap; tamper/wrong-key/malformed/version/oversize all
  decode to null, never throw) + `AndroidPhotoCache` implementing
  portable `PhotoCache` (sealed envelopes in private
  `anpfuel_photos/` cache files, never Gallery/backups/logs;
  reads fail closed with delete-on-read for expired/corrupt;
  atomic tmp+rename writes; id allowlist blocks path escape;
  missing keystore fails writes closed) + `AndroidPhotoCodec`
  (header-only probe, sample-plan decode, metadata-stripping
  JPEG re-encode at 85/70/55 quality schedule, null-never-throw)
  + Hilt `MediaModule`. Zero new dependencies (AndroidKeyStore,
  BitmapFactory, `javax.crypto` are platform APIs / pre-existing;
  minSdk 26) — nothing to license-review because nothing was
  introduced.
- Later — camera capture wiring + macOS/Keychain run (release
  horizon).

## What changed

- `data/.../local/media/PhotoBlobSeal.kt` — seal/open envelope
  with capture-stamp prefix (8 big-endian millis + JPEG bytes).
- `data/.../local/media/PhotoCacheKeys.kt` — `PhotoKeyProvider`
  seam + `AndroidPhotoKeystoreKeys` (API 26+, no user-auth gating
  that would strand launch/resume sweeps).
- `data/.../local/media/AndroidPhotoCache.kt` — sealed
  put/get/delete/sweepExpired over private files.
- `data/.../local/media/AndroidPhotoCodec.kt` — probe/encode thin
  wiring + pure `qualityForAttempt`.
- `data/.../di/MediaModule.kt` — Hilt singletons (keys, cache
  dir, cache, codec, decoder/encoder, `PhotoFlow` with wall clock
  + UUID ids).
- `application/.../portable/PortablePhotoPorts.kt` — single-method
  ports are now `fun interface` (SAM wiring for Hilt providers).
- Tests: `PhotoBlobSealTest` (7: roundtrip, IV uniqueness,
  tamper, wrong key, malformed matrix, empty, oversize) +
  `AndroidPhotoCacheTest` (9: roundtrip, expiry read+delete,
  tamper read+delete, id-escape matrix, sweep expired+corrupt,
  missing-keystore closed, idempotent delete, quality schedule,
  private-subdir containment).

## Dependency review (recorded need/license/security)

- Added: none. `javax.crypto`/`org.json` pre-date this slice;
  AndroidKeyStore/BitmapFactory are platform APIs (minSdk 26).
- Secret surface: `scan-secrets.sh` + PR-diff scan clean; no key
  material in logs, tests or fixtures (synthetic AES keys).

## RED → GREEN

- RED proven by dropping the `put` id-allowlist guard:
  `badIdsRefuseEveryPath` fails (`../escape` writes escape the
  subdir); GREEN on restore.
- (Also verified: neutering the envelope version check alone does
  NOT break the suite — the iv-size check still refuses — so the
  id-guard RED above is the recorded proof, not the version one.)
- Incidental fixes in-task: single-method ports to `fun
  interface`; `File(dir)` → `dir` in three test assertions.

## Validation (exact commands, this host)

```sh
./gradlew :domain:test :application:test :data:testDebugUnitTest :app:testDebugUnitTest :app:assembleDebug --no-daemon
bash scripts/check-mobile.sh
git diff --check
bash scripts/scan-secrets.sh
```

Outcome: Android baseline BUILD SUCCESSFUL (16 new media suites
green: 7 seal + 9 cache); `check-mobile` full ok (static guards
+ Android green; Swift SKIP recorded outstanding, never green);
`diff --check` clean; secrets scan clean. The ~70 Keystore/
BitmapFactory lines run only on device: statically reviewed +
lint-guarded, behavior suites cover the rest.

## Limits (not claimed)

- No camera capture UI, Keychain adapter or macOS run (release
  horizon); 32 MiB hypothesis still unmeasured on device.
- Sweep cadence (WorkManager periodic) belongs to app wiring
  (P17); the launch/resume/read hooks call `sweepExpired`.

## Rollback

Code rollback = revert media package, module, ports tweak and
tests; portable flow returns to T02A. No migration, no flag.

## Next

P15-T03 bounded backend media processing (#36, same branch).
Issue #35 stays open until the phase PR merges.

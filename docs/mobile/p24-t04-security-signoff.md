# P24-T04 — Security compatibility (code-only audit slice)

Status: IN_PROGRESS on `codex/phase-24-android-acceptance`. Issue: #99 OPEN (not done).
Entry: P24-T03 code slice landed; user directive 2026-10-02 applies (no
emulator/device runs — code tests only; device/signature proofs deferred to the
post-all-phases manual batch). Binds B-BR-C01–C06 / BUC-C01–C05. No migration, no
backend change, no iOS work, no production code changed in this slice.

## Audited surface (this tree, static inspection)

- Transport: `usesCleartextTraffic="false"` (manifest) + `network_security_config`
  `cleartextTrafficPermitted="false"` (base-config). No `http://` exception.
- Permissions (manifest, all install/request-time, none background): INTERNET,
  ACCESS_NETWORK_STATE, ACCESS_COARSE_LOCATION + ACCESS_FINE_LOCATION (requested
  only when useful per BUC-C01/C02/C04 journeys, never up front),
  POST_NOTIFICATIONS (opt-in alerts, P23-T01), CAMERA (system intent path, not
  required: `android.hardware.camera` `required="false"`).
- Client logging: zero `Log.*`/`Timber`/`println` in `app/src/main` and
  `data/src/main` — no GPS/PII/photo payload can leak through platform logs.
- Location failure matrix (code-pinned, no device needed): `PortableLocation`
  verdicts DENIED / UNKNOWN / SIMULATED / DEGRADED never authorize proximity
  claims (`allowsClaim` true only for VERIFIED); precedence manual > denial >
  missing-fix > source-info gate > simulated > clock/stale/coarse is covered by
  `PortableLocationTest`. `DiscoveryQuery` carries no precise location by
  construction (P20-T01).
- Photo privacy: 24 h TTL pinned (`PortablePhoto.TRANSIENT_TTL_MILLIS`,
  `PortablePhotoTest`); backend `SanitizeForward` strips EXIF/thumbnails and
  refuses > 256 KiB (P15-T01/P21-T02, unchanged).
- Auth deep link: `anpfuel://auth/callback` handled in `AnpNavGraph` +
  `AuthViewModel`; no other exported entry point besides `MainActivity` launcher.

## Open items (not waived, not green — manual/G09 batch)

- `allowBackup="true"` with account/session DataStores: needs a G09 backup-restore
  privacy review (no change in this slice; restore drill P08-T04/P21-T04 covers local).
- Real-provider callbacks, TLS pinning decision, signature/compatibility matrix on
  the frozen device rows and adversarial replay: device/manual proofs deferred per
  user directive. Phase integration never certifies real production.

## Validation (code tests only, per directive)

- `./gradlew :domain:test :application:test :data:testDebugUnitTest
  :app:testDebugUnitTest :app:assembleDebug --no-daemon` with `--rerun-tasks`
  (record counts in commit evidence). No `connectedAndroidTest`, no emulator boot.
- `git diff --check` clean; scoped secret review (no secrets/PII/GPS/photo content).

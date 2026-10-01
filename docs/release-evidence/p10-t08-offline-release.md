# P10-T08 — Integrated offline and release checks

Status: LOCAL_DONE on `codex/phase-10-functional-integration` (phase PR
#54 pending, eighth and last local task; P10-T09 pilot stays deferred
until G18/G09 and is not part of G10-LOCAL). Issue: #52. No ANP
behavior touched, no backend change, no existing screen redesigned.

## Slice acceptance (frozen before coding)

- T08 (this commit) — Prove upgrades and failure behavior on actual
  Android (TEST_STRATEGY scoped checks): full `./gradlew test` green;
  i18n backfill of all 21 P10 community keys into de/es/fr/ja/ru/zh-rCN
  (P10 MissingTranslation debt zeroed); CaptureScreen copy extracted
  from hardcoded English into resources (9 keys, 8 locales);
  two `community_privacy_*` notices describing actual P10 collection,
  rendered in the vote panel; heading semantics on community panel
  titles; v4→v6 upgrade chain test with representative
  vehicle/survey/average rows; V5→V6 strengthened with vehicle/survey
  asserts; seeder registers all five migrations; outage-with-cache
  portable test; additive-field tolerance test (fail-closed vocab
  kept); two real on-device bugs fixed (below). Rollback: stop rollout,
  community flags OFF, tested previous compatible release.

## What changed

- `app/.../res/values*/strings.xml` — 21 community keys backfilled in
  6 locales; 9 `capture_*` + 2 `community_privacy_*` keys in all
  8 locales (placeholders preserved; UNKNOWN/STALE/ANP/P04 stay
  untranslated per pt-BR precedent). No existing key touched.
- `app/.../capture/CaptureScreen.kt` — stringResource only.
- `app/.../community/CommunityVotePanel.kt` — privacy footnote +
  heading semantics; `CommunityPricePanel.kt` — heading semantics.
- `data/.../local/AnpFuelDatabaseMigrations.kt` — MIGRATION_4_5 no
  longer creates the undeclared `station_id` index (no DAO reads by
  station_id; every cache access is by primary key; the extra index
  drifted migrated installs from the entity schema and failed Room
  validation on device; phase unmerged/unreleased, nothing in the wild).
- `data/build.gradle.kts` — `androidTestImplementation(coroutines.test)`:
  the instrumented suite did not compile without it (connected tests
  never run in CI, so nobody noticed).
- Tests: new `V4ToV6DatabaseMigrationTest` (androidTest); `V5ToV6`
  + vehicle/survey rows and asserts; `InstrumentedAppDataSeeder`
  registers MIGRATION_4_5/5_6; `VehicleRepositoryImplTest` compares
  ids/names (`Vehicle` has no equals — test-only fix, no domain
  change); `Migration4To5UnitTest` asserts no secondary index;
  `PortableSyncStateTest` outage-with-cache falls back to CACHE;
  `BackendPriceGroupsJsonCodecTest` unknown additive fields tolerated,
  unknown vocab still refused.

## Collection → notice mapping (privacy review)

- Backend price reads (station_id/fuel, 60s cache): source/stale
  labels (`community_source_version`, `community_stale_label`).
- Keystore P-256 key + loss: `community_privacy_keys` (code-only
  `LOST_KEY_NOTICE` now user-visible).
- Photo (sealed 24h cache, EXIF stripped, reserve/PUT/complete) +
  outbox ids/nonces: `community_privacy_footnote` + capture copy.
- Votes (6 reasons, 500-char private detail, replacement id, no
  amplification): vote panel + `community_vote_private_note`.
- Full legal-grade policy stays deferred per `PRIVACY_NOTICE` draft
  and `SECURITY_PRIVACY` ("not legal compliance"); verified before
  G09/P10-T09, never claimed here.

## RED → GREEN (on actual Android 2311DRK48G, API 16)

- First device run: 44 tests, 4 failed — V4ToV5 + V4ToV6
  (`Migration didn't properly handle: backend_price_cache`, extra
  index) and VehicleRepository ×2 (instance comparison without
  equals). Pre-existing failures, never executed before (suite did
  not compile + CI never runs connected tests). Fixed as above.
- Second device run: `:data:connectedDebugAndroidTest` GREEN —
  44 tests, 0 failed (2 live-network tests SKIPPED by their own
  gate), including V1→V2, V4→V5, V4→V6, V5→V6 and the strengthened
  V5→V6. Wrong-clock stays covered by
  `clockSkewNeverMarksFreshCacheStale` (negative age → CACHE).

## Validation (exact commands, this host + device)

```sh
./gradlew test --no-daemon
./gradlew :app:lintDebug --no-daemon
./gradlew :app:assembleDebug --no-daemon
./gradlew :data:connectedDebugAndroidTest --no-daemon
./gradlew :app:connectedDebugAndroidTest --no-daemon
./gradlew :app:compileDebugAndroidTestKotlin --no-daemon
bash scripts/check-mobile.sh --static-only
git diff --check
bash scripts/scan-secrets.sh
```

Outcome: `./gradlew test` (all JVM suites incl. release unit)
green; `assembleDebug` green; data connected green on device (above);
`check-mobile --static-only` ok; `diff --check`/secrets clean.
Limits: `:app:lintDebug` still fails with 23 errors — 22 `auth_*`
MissingTranslation (P13 scope, untouched) + 1 `MissingPermission`
(pre-existing on main); zero community/capture findings, zero
findings in new/edited Kotlin. `:app:connectedDebugAndroidTest`
compiles but installs 0 tests: MIUI on the attached device demands a
per-APK user tap (`INSTALL_FAILED_USER_RESTRICTED`, retried twice);
app device suite stays CI-bound. No backend change; no production URL
invented. B-BR data rules untouched; migration stays additive (no DROP).

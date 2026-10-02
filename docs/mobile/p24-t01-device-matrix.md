# P24-T01 — Android candidate and device matrix (partial)

Status: IN_PROGRESS on `codex/phase-24-android-acceptance`. Issue: #96 OPEN. Entry G23 satisfied (P23 PR #95 merged `b4f664e`). Binds B-BR-C01–C06 and BUC-C01–C05. No migration, no backend change, no iOS work.

## Device rows

- Xiaomi 2311DRK48G (Redmi 13C 5G), Android 16 (API 36), arm64-v8a, heap 512 MB, en-US: first run 18/21 green (connected suites had never run in CI).
- Emulator `anpfuel-low26`, API 26 (SQLite 3.18.2), x86_64, 2 GB: local low-end row for the minSdk floor (signal, not a hardware replacement).
- Required but missing rows (block acceptance until proven): real low-end API 26–30 hardware (encode/perf, owned by P24-T03); second OEM skin.

## Fixes landed from device evidence (this task)

- FTS API-26 crash FIXED: `remove_diacritics=2` needs SQLite 3.20+/ICU (probed on-device); schema v7 uses plain `unicode61` plus the pre-normalized `normalized_name` column, so "SAO" (normalized) and "SÃO" (raw) both match on every API level. `MIGRATION_6_7` rebuilds the derived index (catalog rows untouched); `BigInteger.TWO` (Java 9+) replaced with `BigInteger("2")` after `AnonymousDeviceKeysDeviceTest` proved it missing on API 26. Manually verified E2E on the API 26 emulator: fresh install → auto-sync (~20k rows) → location → FTS "sao" matches → home with data.
- FreshInstall device test rewritten to the real auto-download flow (was asserting the manual picker against default auto-download).
- HomeScreenTest now asserts the merged accessibility announcement (descendants are intentionally merged).
- Restored the Search entry chip on home (`Routes.SEARCH` had zero callers — dead route); manually verified home → SearchScreen on API 26.
- Historical migration tests (V1–V6 from-states contain `remove_diacritics=2`, which cannot exist on API 26) self-skip there via a shared SQLite probe and run fully where supported.
- Worker device tests are mockk-free (hand fakes run the real use cases): mockk cannot intercept final use-case methods on API 26 ART (real method ran → NPE).
- PostSync journey: entry fixed via the Search chip; full script re-run pending.
- Device caveat: MIUI cancels installs intermittently (`INSTALL_FAILED_USER_RESTRICTED`) even with Install-via-USB enabled — each reinstall may need on-screen confirmation.

## Feature matrix (proof pointers, this tree)

- Anonymous lookup/offline/sync: JVM `:domain` 408 + `:data` 207 + `:app` 147 suites green; device AppendixA2 + startup rows green.
- Accounts/providers: `AuthViewModelTest`, backend account/identity integration (P22 evidence); real provider proof stays P24-owned (no device account run yet).
- Contribution/correction/outbox/expiry: outbox + edit + expiry suites green (P21–P23 evidence).
- Social/moderation/ratings: feedback/moderation suites + device community row green.
- Alerts/vehicles: alert suites incl. dedupe green (P23-T01).
- Privacy/erasure: footprint/erase suites green; 24 h photo expiry covered (P21-T04).

## Candidate

Tested tree: branch head at device run (`bd6dcdd` + this commit). No candidate SHA is frozen while proofs are missing.

## Validation (device proofs, this turn)

- `:data:connectedDebugAndroidTest` FULL green both rows: API-26 emulator 43 tests 0-fail (7 skipped: 5 migration self-skips where v6 history cannot exist + 2 live `@Ignore`); Xiaomi API 36 43 tests 0-fail (2 skipped: live `@Ignore` only). `V6ToV7DatabaseMigrationTest` executed + PASS on Xiaomi newer SQLite.
- `AnpScaffoldTest`: PASS on Xiaomi API 36 → the inset failure is an API-26-emulator-only row (pre-existing, env).
- `TempSeedDumpTest` R3.1.2/R3.1.3 replication: `R313_REPLICA_ROW=true` twice on the API-26 emulator; probe fulfilled and removed, flow stays covered by `AppendixA2PostSyncTest`.
- `AppendixA2PostSyncTest` R3.1.9 expectation corrected (test-only): the fuel icon is decorative by design and the FilterChip carries the text label, so the operability contract is label + click action, not a duplicate content-description node. End-to-end green still unproven (blocked by R3.1.3 flake below).
- `AppendixA2PostSyncTest` R3.1.3 search is FLAKY across rows, not env-specific: Xiaomi 1 pass (green through R3.1.8) + 2 fails at the 120s row wait; API-26 emulator stalls identically; `TempSeedDumpTest` identical-query replication stayed green 2x on the emulator. When stuck, the screen shows title + field (query committed) + footer only — no rows, no empty-state, no min-chars hint — i.e. the search flow never emits. Prime suspects for the next instrumented slice: first-query cost chain (`seedIfEmpty` → FTS → fuzzy `findAll` over 5571 rows + per-result `findCatalogEntry`), cross-process DataStore prefs access (seeder writes `UserPreferencesDataStore`/`SyncStateDataStore` from the test process, app reads them), background worker contention. Next: temporary timing instrumentation in the search path to identify, then fix; no thresholds weakened.
- `AppendixA2FreshInstallTest`: red on the offline emulator (no network route; auto-download needs real network) — networked-device proof pending (not attempted this turn; device time went to R3.1.3 triage).
- `git diff --check` clean; secret scan PASS. T01 stays IN_PROGRESS: R3.1.3 flake diagnosis + fix, R3.1.9 end-to-end green, FreshInstall networked proof, real low-end hardware row.

- FreshInstall page-count fix (test-only change for the 4-page onboarding).
- JVM `OnboardingViewModelTest` pager test still green (4-page walk).
- `git diff --check` clean; secret scan PASS. T01 stays IN_PROGRESS: missing/failed proof blocks acceptance by task rule.

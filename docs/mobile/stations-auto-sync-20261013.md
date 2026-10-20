# Station directory auto sync and list summary

Historical checkpoint. The unconditional sync and single-page behavior below are superseded by the [2026-10-20 pagination/optional-sync review](pagination-sync-review-20261019.md); its candidate still owes native execution. The earlier device results do not certify the new candidate.

Task: P35-T03 follow-up / MOBILE-SYNC-01. Status: LOCAL_DONE.
Validation: PASS.

## Authorized behavior before implementation

- B-BR-MSYNC-01 / BUC-MSYNC-01: while Stations is visible, read the VPS directory on entry/resume, immediately after validated internet returns, and at bounded 60-second intervals while online. Refresh an open canonical detail as well. Stop foreground polling when the screen stops. Preserve fuel, search and selected survey week; never manufacture prices or reviewed coordinates.
- B-BR-MSYNC-02 / BUC-MSYNC-02: a unique connected-network WorkManager job refreshes the existing bounded directory page every 15 minutes when Android permits, including after process restart. Failed requests retry with backoff and retain the last good cache. The community-read flag remains authoritative. Weekly ANP downloads retain their existing preferences/policy. This is read synchronization, not national data download or contributor upload.
- B-BR-MSYNC-03: cancellation is never an outage/cache success; canceled requests cannot publish old results. Concurrent reads serialize and Room replaces page rows plus metadata in one transaction. A successful empty response replaces old entries and remains a valid offline snapshot.
- B-BR-MSYNC-04 / BUC-MSYNC-03: show city, fuel and ordering inside a tonal, non-clickable “List summary” surface, with explicit labels and no input outline, arrow, button semantics or ripple. The top fuel selector and station search remain interactive.
- B-BR-MSYNC-05: real VPS probing exposed that the old list reader requests a national first page. Automatic UI/background reads must use the selected municipality code; missing scope cannot fall back nationally. Scoped cache keys reject another municipality and legacy unscoped snapshots.

Scope: Android/application/data only; existing CNPJ presentation fix from ING-AUDIT-01 retained. No VPS rollout, backend changes, new dependency, schema migration or iOS work. Physical-device installation and radio testing were explicitly authorized by the user after this record was opened (`adb install -r`, no data clear, no account/consent/contribution changes). The occupied dev checkout and its unrelated backend edit are preserved in a separate checkout and excluded from this delivery.

## Acceptance and risks

Prove reconnect, online periodic refresh, resume, offline cache, outage recovery, cancellation, no overlapping refreshes, flag OFF and preserved filter/selection. Prove real Room transaction rollback/empty snapshots and worker constraints/retry with isolated emulator tests. Run affected JVM suites, lint, APK compilation and diff/secret review. Android scheduling is inexact and can be delayed by Doze/battery restrictions; no instantaneous server push is claimed. City scope uses the existing IBGE catalog, never geocoding guesses. A bounded page of up to 100 stations is refreshed; a non-null server cursor is retained, not claimed as a complete large-city traversal. National browsing/pagination expansion remains separate.

Primary Android references: [Work requests and constraints](https://developer.android.com/develop/background-work/background-tasks/persistent/getting-started/define-work), [Lifecycle-aware coroutines](https://developer.android.com/topic/libraries/architecture/coroutines#restart).

## Local evidence 2026-10-13

Base: maintained `dev` at `e22824db` plus a normal merge of `codex/station-reliability-audit` at `8fefff7d` (task dependency, already pushed; its PR #133 remains unmerged). All new mobile changes below were developed and tested uncommitted in that isolated worktree.

Implementation:

- `StationsViewModel.syncServerStationsWhileVisible`, driven by `repeatOnLifecycle(RESUMED)`: refreshes on entry/return, on validated reconnect and every 60 s while online; cancelled on pause; refreshes an open detail too. Offline reads cache without HTTP. Search, fuel and week are preserved. Cancelled results cannot publish and a dismissed detail is never reopened.
- `DirectorySyncWorker`: unique periodic work `station_directory_sync`, 15 min, `NetworkType.CONNECTED`, `KEEP`, exponential 30 s backoff, bounded attempts; scheduled in `AnpFuelApplication`. Directory reads only; the weekly ANP download policy is untouched.
- Municipal scope: `StationDirectoryScopeProvider` resolves IBGE from preferences plus the existing catalog with mandatory DI in production. UI and worker request a bounded page of up to 100 for that city only; no national fallback. Responses from another city are rejected. The cursor is retained; pages beyond 100 are out of scope (Sorriso has 25 and fits one page).
- Cache: `ServerStationCache.saveScopedPage/loadScopedPage` keyed `city:IBGE`; a single active page snapshot per city, never a global or foreign-city snapshot. `DAO.replacePage` and the page snapshot are transactional; ad-hoc details survive with `position=Int.MAX_VALUE`. A valid empty snapshot replaces stale rows. No migration or schema change.
- HTTP: `awaitList/awaitCityList/awaitDetail` use OkHttp `enqueue` with `suspendCancellableCoroutine` and cancel the socket. The gateway runs IO off main. `GetServerStationsUseCase` serializes foreground/background reads with a Mutex; `CancellationException` never becomes cache or outage.
- JSON: `optNullableString` handles a JSON null `next_cursor` that the device parser surfaced as the string `"null"` while JVM tests passed. List reads require the `items` array; an invalid envelope never erases a good cache. The native `assertNull` was not weakened.
- Connectivity: `registerDefaultNetworkCallback` requires INTERNET plus VALIDATED; `onCapabilitiesChanged` uses the received snapshot; `onLost` only affects the matching default network.
- UI: `StationsListSummary.kt`, a tonal rounded non-clickable Surface titled “Resumo da lista” showing city, `FuelProductLabel` and “Ordenação: Menor preço primeiro”. No outline, arrow, input or ripple. Rendered in empty and search states too. All 8 locale string files updated.
- Two pre-existing instrumented tests did not compile against the changed APIs and were adapted without weakening assertions: `AnonymousDeviceKeysDeviceTest` now supplies synthetic `AuthFlow`/`ApiEnvironment`; `PhotoReviewOutboxDeviceTest` uses `findDispatchable`/`listOwnedPage`.

Validation:

- 62 scoped JVM tests, zero failures/errors/skips: domain 3 (`StationDirectoryVisibilityRuleTest`), application 13 (`GetServerStationsUseCaseTest`: cancellation, concurrency, scope, cache), app 25 (`*Stations*Test`: reconnect, polling, pause/return, search, detail, no overlap), data 21 (`*DirectoryStation*Test`: real HTTP via MockWebServer, socket cancellation, strict JSON, cache).
- RED→GREEN (JVM): initial RED reproduced a consumed `CancellationException`; GREEN after Mutex serialization plus cancellation passthrough. RED→GREEN (device): native RED reproduced the `"null"` cursor string; GREEN after `optNullableString`.
- Final `:app:lintDebug` plus data lint: 0 errors (230/18 pre-existing warnings). `:app:assembleDebug`, `:app:assembleDebugAndroidTest` and the data androidTest assembly compile.
- Physical device (Android API36, `com.anpfuel.app.debug`, `adb install -r`, matching prior signature, no clear/uninstall, no account/consent/contribution/GPS/photo actions): 18 native cases PASS. Data module 13 (`DirectoryCacheTransactionTest` 5: real trigger-ABORT rollback, empty snapshot, concurrent read/write, ad-hoc detail preservation, municipal isolation; `DirectorySyncWorkerTest` 4: success/cache, no-network OFF, bounded retry, constraints/interval/backoff; `AnonymousDeviceKeysDeviceTest` 1; `PhotoReviewOutboxDeviceTest` 3). App module 5 (`StationsListSummaryTest` 1: no click action/input plus capture; `StagingDirectorySyncDeviceTest` 4: unique job registration, validated internet observed, shell-level physical Wi-Fi/data loss and return observed `true→false→true` on the real connectivity observer with settings restored, live staging HTTPS Sorriso `5107925` page-100 `Fresh` with unique UUIDs/correct city/real null cursor, simulated outage served only in-memory cache as `StaleCache` with identical ids, reconnect `Fresh`).
- The radio test initially failed only on an immediate post-restore settings read (`svc data enable` is asynchronous); the connectivity-observer contract itself already passed. Fixed with bounded polling for the restored settings; the restore requirement was kept, not weakened. A no-flag control run is instant-skipped, confirming the flagged runs above genuinely executed.
- Earlier `uiautomator` focus attempts during `svc wifi/data disable` showed launcher/Home instead of Postos with no crash and no APK change; those attempts are NOT claimed as reconnect evidence and were replaced by the shell-level radio test.
- UI on device (pt-BR): summary card content confirmed; tapping fuel inside the summary opens no dialog while the top selector opens; GLP and common gasoline exercised, common gasoline restored; Sorriso and the 4–10 Oct week preserved.
- Installed product APK byte-verified by pull: `05d6abc4b2af62379ec11f7c739b595c4d81f7db2776a9790c53f06a215cb912` (`app/build/outputs/apk/debug/app-debug.apk`). Only test sources changed after that install; product bytes are unchanged. Wi-Fi/mobile settings restored to `1`/`1`; the temporary `stay_on_while_plugged_in` override was restored; `screen_off_timeout` was never touched. No device capture with personal content is versioned; the device serial stays in `/tmp` only.
- `git diff --check` clean; secret-surface review of the diff found no secrets, PII, precise contributor GPS or production photos.

Limits and next step:

- Background work stays inexact under Doze/battery policy; foreground polling runs only while RESUMED and online. Page bound is 100 with cursor retention; Sorriso coordinates remain unknown, so no minimum-distance validation is claimed. Weekly ANP sampling (459 localities) is distinct from the national registry. This checkpoint certifies neither the whole P35 phase nor any release (G24/P09 gates unchanged).
- Next: one atomic English commit of this task only, then scoped GitHub delivery that excludes the unrelated Home commit `e22824db` and the other checkout's backend `store.go` edit — fresh branch from `origin/dev`, normal merge of the audit dependency, cherry-pick of the mobile commit, and a dev-targeted integration PR. No main merge without Quick verification; no CI polling during construction.

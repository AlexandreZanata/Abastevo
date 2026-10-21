# OPT home/community feed — independent delivery review

Scope: review the five local commits `f60bb3e2` through `8bb652fa`, reconcile
maintained dev `a2f8925c`, correct reproducible defects, and deliver through
the protected dev → main PR. Original phase evidence remains historical.

## Behavior and acceptance corrections (before implementation)

- BUC-OPT01: returning to Home retains non-expired, same-city memory content
  while the local scope/snapshot lookup is pending. An obsolete lifecycle
  lease cannot stop the active session.
- B-BR-OPT02: every scheduled delay is positive, including a missing city
  and suspended city/network lookup; reconnect queries require an active
  Home session. Returning after retry exhaustion reopens the attempt budget.
- BUC-OPT02: periodic evaluations that skip the network preserve the last
  failure/offline indication until a successful check. Cursor loads make no
  request offline or during a first-page request.
- B-BR-OPT03: a full cached first page acquires a live continuation promptly
  while retaining its cards; persisted cursors remain excluded. A cancelled
  snapshot must not paint into a newer filter scope. Malformed JSON snapshots
  fail softly so they cannot block a valid live response.
- Room acceptance requires actual SQLite/Room 16→17 migration, retained
  existing rows, DAO bounds/isolation, and close/reopen persistence. Mocked
  emitted SQL alone does not prove migration/reopen behavior.

The original virtual-time call counts are scenarios, not production
measurements. The removed scenario is not reproducible evidence of bytes;
call-count reduction alone does not establish transferred bytes or latency.

## Validation and delivery

Reviewed original behavior and the reconciliation at `438833a1`; final code
and test changes are committed with this record. Delivery head and exact
current-head/base remote results belong to PR #140 metadata.

- RED: eight isolated Home/feed regressions failed on the submitted code
  (missing-city/pending-lookup delay, return retention, off-screen reconnect,
  failure indication, retry-budget return, offline cursor, full cached-page
  continuation, cancelled-snapshot isolation). A ninth data regression
  reproduced `JSONException` from balanced-invalid JSON/missing fields.
- GREEN: all nine regressions pass after the scoped corrections. A separate
  concurrency case proves first-page and cursor work cannot overlap.
- Full JVM suites: domain 549, application 387, data 449 (two existing opt-in
  skips), app 391; zero failures/errors. Exact commands:

  ```sh
  ./gradlew --no-daemon --max-workers=2 :domain:test :application:test :data:testDebugUnitTest :app:testDebugUnitTest
  ./gradlew --no-daemon --max-workers=2 :app:assembleDebug :app:lintDebug :data:assembleDebugAndroidTest :app:assembleDebugAndroidTest
  ```

  The aggregate initially exposed a compile error in the new native test
  (`RoomDatabase` is not `Closeable`); explicit close/finally corrected it.
  Final affected checks pass, lint reports zero errors. No test was weakened.
- Physical Android device: actual 16→17 SQLite/Room validation, retained ANP
  fixture rows, generated DAO age/64-entry bounds and close/reopen PASS (1).
  Community Compose class plus two Home community cases PASS (7, no skips),
  including stale hint, standard-price label, failure retention and retry.
  Synthetic screenshot inspected. APKs installed with `adb install -r`;
  direct instrumentation used `-e clearPackageData false`, no app-data reset.

  ```sh
  adb shell am instrument -w -r -e clearPackageData false -e class com.anpfuel.data.local.CommunityCacheMigration16To17Test com.anpfuel.data.test/androidx.test.runner.AndroidJUnitRunner
  adb shell am instrument -w -r -e clearPackageData false -e class 'com.anpfuel.app.ui.community.CommunityFeedScreenTest,com.anpfuel.app.ui.home.HomeScreenTest#communityFailureRetainsPriceAndOffersRetry,com.anpfuel.app.ui.home.HomeScreenTest#communitySourceWorksWithoutANPCacheAndKeepsThreePriceFilters' com.anpfuel.app.debug.test/androidx.test.runner.AndroidJUnitRunner
  ```

- Backend community HTTP contract PASS (`go test
  ./internal/modules/community/adapters/http -count=1`). The previously pending
  dev Directory correction also passes its actual disposable-PostGIS test
  three times with `-race -tags=integration -run
  '^TestFaultWorkerKillMidLoad$' -count=3` (no skips).
- Public staging reads on 2026-10-21: feed (MT/5107925/ETHANOL/recent/20) and
  summary both HTTP 200, valid-empty items, `Cache-Control: no-store`, hint 15.
  This proves read connectivity/empty contract, not contribution visibility.
- `git diff --check`, tracked secret scan and changed-file secret review PASS
  (45 changed/new files, zero high-confidence matches); no dependencies added.
- Main protection verified remotely: strict `Quick verification` from GitHub
  Actions app 15368; admins enforced, force pushes/deletion disabled. Connector
  lacked administration-read access and later failed transport; CLI provided
  the actual protection/PR evidence.

## Reproducible efficiency evidence and remaining limits

Three permanent feed JVM tests drive wall time from the coroutine scheduler:
300 s fresh foreground produces exactly 6 calls; offline with a snapshot 0;
outage 5 then silence through a further 300 s. Original fixed-15 s baseline
21 is analytic. Full cached pages may acquire a live cursor immediately;
smaller fresh snapshots paint with zero requests. These counts do not prove
network bytes, production latency, battery or capacity.

Device migration/reopen and the selected rendering paths are now proved.
Signed contribution → live projection → feed staging E2E, broader manual app
acceptance and production latency remain OWED. No deploy/tag/release claim.

Status: LOCAL_DONE
Validation: PASS
Integration: INTEGRATION_PENDING (protected PR #140; required current-head
checks and guarded finish still required).

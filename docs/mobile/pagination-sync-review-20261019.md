# Pagination and optional catalog synchronization review

Status: LOCAL_DONE
Validation: PASS
Integration: INTEGRATION_PENDING
Native execution: scoped PASS on 2026-10-20; see [device follow-up](pagination-sync-device-20261020.md). Whole-app/release acceptance remains OWED.

Opened 2026-10-19; completed locally 2026-10-20. Review input: ten commits ending at `28a3de0a` (`79b6a7bd` first), reconciled with maintained dev `cb10cc5b` in `codex/mobile-pagination-review`. Both parents and the resolution are retained by the local merge commit containing this record. The occupied original checkout, its unrelated evidence-store edit and the separate quality construction checkout were preserved. Dev subsequently advanced to `08bef884`; integration with that newer head still requires affected revalidation. No remote PR/check/merge, staging probe, device installation, deployment or release is claimed.

## Correctness contracts

- B-BR-PAGE-REVIEW-01: a retired query cannot change rows, selected fuel, loading state or errors of the current query. Polling must not erase manually loaded pages.
- B-BR-PAGE-REVIEW-02: catalog continuation must advance; repeated cursors and nonprogressing pages preserve the prior snapshot and report a retryable failure. Search, city and backend origin are isolated.
- B-BR-PAGE-REVIEW-03: background work pins its city and opt-in revision, rechecks before committing each page and records completion only through an atomic current-preference update. Disabling cannot be undone by an old worker. Bounded runs resume partial traversals rather than downloading the same first hundred rows forever.
- B-BR-PAGE-REVIEW-04: Room upgrades preserve the existing quality 13→14→15 chain. Catalog changes append at 15→16; exported historical schemas remain unchanged.
- BUC-PAGE-REVIEW-01: browse, continue, change filters, interrupt/retry, go offline and restart with honest source/staleness and no lost or foreign rows.

## Findings and corrections

The submitted branch had no final validation commit or current progress entry. Its migration 13→14 collided with the maintained quality migration. The first combined build also exposed obsolete native-test scheduler calls. Independent RED runs reproduced a repeated Directory cursor being accepted, completion after opt-out, and a retired ANP response replacing the current query. Regression tests passed after correction.

- **Backend:** retained and tested the submitted Nearby batching: one bounded page query plus one reference query, replacing per-row reference reads while preserving identifier/official-source semantics. Existing list/search keyset contracts remain bounded; no backend migration or new dependency was needed.
- **Android station lists:** retained a single virtualized list with stable keys. ANP loads cancel obsolete jobs, debounce search and guard generations, including noncooperative late responses. Directory manual search reaches the server and partitions its cache. Refresh/reconnect polling preserves loaded continuation rows instead of shrinking the screen to page one. Failed continuations retain displayed rows and retry cursors; expired cursors recover explicitly.
- **Local ANP data:** SQL filters/order/limit execute before mapping. Derived Unicode-lowercase fields repair SQLite NOCASE's failure on Portuguese uppercase accents; literal wildcard escaping is retained. A matching price index supports stable price/id ordering. Migration backfills the derived fields without changing original ANP values.
- **Cache:** origin/city/query isolation; transactional append preserves row positions. Saved detail rows cannot acquire page positions. Normal offline reads fetch twenty rows plus one lookahead, with SQL continuation offsets and JSON mapping on IO; they can run while background network work is pending. Internal complete snapshots are bounded by 10,000 rows/scope, eight scopes, 200 details and a 32 MiB estimate; admission failures roll back instead of silently truncating a complete snapshot. Settings displays estimated public-cache size and offers scoped clearing.
- **Optional synchronization:** opt-in stays off by default and is separate from weekly ANP downloads. A persisted preference observer cancels/reschedules unique work on opt-in, state/city or network-policy changes, including restart. Each run admits at most five twenty-row requests, 512 KiB/response and 45 seconds; incomplete staging resumes next run. A previous complete download remains usable during partial refresh/outage. Completion uses an atomic generation/scope comparison in DataStore, preserving unrelated settings and rejecting old off→on workers. Clearing serializes with catalog writes and fences the completion marker.
- **Other collections:** owner contributions now expose twenty-row continuation with scope checks and a virtualized list. Community feed can continue beyond its former 200-item dead end using a bounded 200-item window and an explicit return-to-start action; polling preserves that window. History rows are lazy and retired fuel requests cannot publish cancellation as an error. Comment continuation remains virtualized, preserves rows on failure and cancels retired reads. Directory and public feedback HTTP reads cancel the actual socket, enforce deadlines/body bounds and reject excessive JSON depth. Existing cities/weeks already use lazy presentation. No new reply-reading feature or unrelated private transport redesign is claimed.

## Validation

Final Android command (repository root):

```sh
./gradlew :domain:test :application:test :data:testDebugUnitTest :app:testDebugUnitTest \
  :app:lintDebug :app:assembleDebug :data:assembleDebugAndroidTest :app:assembleDebugAndroidTest \
  --console=plain --continue
```

PASS: 1,728 discovered JVM cases, 1,726 executed, zero failures/errors, two existing opt-in skips. Module totals: domain543, application387, data434 (two skipped), app364. The skipped cases are `TrainingConsentBackendInteropTest` and `AnpListingLiveCatalogValidationTest`; neither is evidence of a live run. Real DataStore file tests cover atomic completion, unrelated preferences, opt-out/off→on fencing and city/cache-clear resets. MockWebServer tests cover actual request paths, response refusal and socket cancellation. App/data native test APKs compile, including renamed 15→16 migration tests and updated worker contracts. Lint passes with zero errors, 240 warnings and two informational findings; this is not a warning-free build.

Actual SQLite checks (not Android Room runtime):

```sh
python3 scripts/tests/test-mobile-pagination-sql.py
python3 scripts/tests/test-quality-outbox-sql.py
```

PASS: six cases each. The new harness executes SQL extracted from production DAO/migrations against exported schemas: 15→16 and 13→14→15→16 preservation/schema parity, metadata-trigger rollback, twelve concurrent writers/reopen, twenty-row windows and public-clear/detail isolation, Unicode/literal-wildcard filtering and stable price pagination. Its migration oracle emulates the Kotlin lowercase backfill; native execution remains owed. A synthetic 10,001-row equal-price fixture returned twenty rows using `idx_station_discovery_price`, without a temporary sort; median0.069ms/p950.073ms in the final isolated run. These timings are a local SQL diagnostic, not device frame-time, VPS latency or a capacity claim.

Backend checks (from `backend/`):

```sh
ANPFUEL_TEST_DATABASE_URL='<dedicated disposable local PostGIS DSN>' \
  go test -race -count=1 -tags=integration \
  ./internal/modules/directory/adapters/read ./internal/modules/directory/adapters/http \
  ./internal/modules/directory/application ./internal/platform/httpapi
go vet ./internal/modules/directory/... ./internal/platform/httpapi
go build ./internal/modules/directory/... ./internal/platform/httpapi
GOTOOLCHAIN=go1.27.1 staticcheck ./internal/modules/directory/... ./internal/platform/httpapi
```

PASS: all four race/PostGIS packages, including Nearby batch parity/query counts and existing cursor/failure/concurrency contracts; scoped build/vet/staticcheck and explicit empty gofmt listing. The database was a separately named tmpfs PostGIS container owned by this review; shared databases were not selected. Final whitespace/conflict-marker and tracked/changed-file secret-surface checks pass; no dependency, secret, production photo or contributor GPS was added.

Local logs: `/tmp/mobile-pagination-review-{red,backend,final-gradle,sqlite-final,quality-sql}.log` (ephemeral). Canonical reproducible SQL harnesses and regression tests are committed with this record.

## Remaining acceptance and integration

This source review is locally complete; the whole P35/G24/G09 is not certified. The subsequent [device follow-up](pagination-sync-device-20261020.md) records38 native passes, real staging reads/network recovery and the physical frame diagnostic; it supersedes the native deferral for those named scenarios. Execute the deferred Android batch against this candidate: Room migration/transaction/worker tests, process restart and offline browsing, rapid query/fuel/city changes, opt-out during requests, cache clear, network restoration, and scroll/frame-time checks with a dense-city catalog. Run scoped staging-origin checks separately; no production load campaign is authorized. Then reconcile the latest maintained dev with affected revalidation and use the protected dev→main delivery with current required `Quick verification`. No remote issue/PR was created for this local review; wiki/release evidence remains unchanged.

Rollback is the existing community-read flag OFF plus public-sync opt-out. Keep the applied 15→16 schema append-only; do not replace historical migrations or reset user data to roll back UI behavior.

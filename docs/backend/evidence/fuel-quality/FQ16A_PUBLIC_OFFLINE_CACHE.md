# FQ-16A — Bounded public offline history

Status: LOCAL_DONE
Validation: PASS

Parent d50b6b10, maintained dev. B-BR-FQ-07/12/13/16, BUC-FQ-03/04.
Cache only accepted anonymous official JSON, keyed by complete origin/path/query;
never private report/session/proof data. Persist across process death under the
platform-excluded cache directory, with atomic writes and independent bounds.
Network-first conditional ETags reuse a validated body on 304. Connectivity/5xx
fallback is explicitly stale; malformed/scope-mismatched/4xx responses never
silently fall back. A disabled/not-found read purges this origin’s owned cache.

Operational budgets (not trust/score rules): 64 entries, 16MiB total, 1MiB body,
seven days retained; backend source staleness and max-age=60 remain independent.
Persist a clock watermark; rollback/corrupt/missing clock metadata refuses old
entries. Never extend freshness from restore or failed refresh. Public files
contain official published facts only; no media, GPS, account or commands.

Acceptance: real files/restart, TTL/rollback/corruption/eviction/origin separation,
actual HTTP 200→304, disconnect/5xx stale fallback, no fallback for invalid body,
foreign selection or kill switch, cancellation, affected JVM/lint/APK. Private
stable outbox/erasure remains next, D and native execution restrictions unchanged.

## Acceptance, 2026-10-18

Implemented file-backed public cache only after complete typed parsing. IO stays
off the main dispatcher; content hash, version, complete URL hash, retained-at
and persisted clock are checked before use. Atomic fsync+rename and interrupted
file cleanup survive restart. Entries evict to both budgets. `qualityCacheEnabled`
independently disables cache construction; Android cacheDir is excluded from OS
backup. Conditional ETags validate via 304; no-store removes this origin’s old
representations. Invalid/foreign/4xx bodies neither fall back nor replace the
last accepted body. 404 purges the origin. Retryable failures reread cache at
failure time, so request-time expiry/clock rollback cannot slip through.

Offline lease staleness propagates from an assay page to the entire pinned
snapshot and is visible in sample/provenance details. No cached response is
promoted to current guaranteed quality, and no numeric score is created.

`./gradlew --offline :data:testDebugUnitTest --tests '*QualityPublicCacheTest'
--tests '*QualityOfflineHttpTest' --tests '*QualityHttpClientTest'
:application:test --tests '*ReadQualityTest' :app:testDebugUnitTest --tests
'*QualityViewModelTest' :app:lintDebug :app:assembleDebug
:app:assembleDebugAndroidTest`: PASS. Three real-file cases, three HTTP cases,
five existing HTTP cases, four application and four ViewModel cases (19) pass.
The initial cancellation harness blocked the test scheduler before IO returned;
waiting for its socket on Dispatchers.IO fixes the harness without weakening the
actual cancel/close assertion. No-cache construction also avoids unnecessary IO.

Added explicit no-store refusal: affected `QualityOfflineHttpTest`, lint and both
APK builds rerun PASS (39s), now four HTTP cases / 20 affected JVM cases total.
Zero quality lint findings; whitespace/secret/link surfaces reviewed. Logs
/tmp/fq16a-final.log and /tmp/fq16a-no-store-final.log remain local. No schema,
dependency, private file, device execution or deployment. Next: encrypted owner
stable command persistence/dispatch/erasure and remaining FQ-16 compatibility.

# MAP-14 bounded continuation after release-build recovery

Date: 2026-10-24. Phase: IN_PROGRESS / ENGINE_GATE_PARTIAL.
Worktree: `/data/dev/worktrees/abastevo-map-ev`,
branch `codex/map-ev-implementation`, base `721582b8`, with uncommitted work.

Resume the existing work; preserve its dirty tree and do not restart MAP-00.
Read current PROGRESS, MAP-14 and MAP-14_BUILD_RECOVERY evidence, then the
selected MAP-14 task. This continuation does not authorize push/merge/deploy,
real-phone installation, a different engine or a false gate declaration.

## Decisions already resolved

1. The user accepted the engine APK budget of 80 MiB. Do not request that
   authorization again or raise the threshold further to pass a failed build.
2. Build release classes and native code together with the recorded source
   patch. The trailing gold option came from CMake IPO; it has been corrected
   without disabling ThinLTO or editing generated Ninja files. The real arm64
   release candidate is `/tmp/abastevo-comaps-release-arm64`; regenerate it with
   `scripts/maps/build-comaps-aar.sh` if the temporary directory is absent.
3. Do not delete or regenerate the SDK BuildConfig. `maps/engine` consumes the
   original release class and the recorded JNI consumer rules.
4. MAP-15 Ed25519 implementation direction: use the already pinned native
   `platform::crypto::VerifyEd25519`/Monocypher behind an injected Abastevo
   verification port. No new provider dependency or handwritten crypto is
   needed. Audit the BSD-2-Clause source/notices and prove exact Go-signed bytes,
   RFC vectors, bad lengths/message/key/signature, trust and rotation on API 26
   before accepting that bridge. No implementation is claimed by this choice.
5. Reuse existing Room/WorkManager/file-storage patterns. Allocate the next
   append-only schema version from the actual head. Normal migration/download
   implementation choices do not require a new owner decision.

## Work still required, one bounded task at a time

- Reconcile the MAP-12/MAP-13 review obligations before certifying DATA_GATE:
  reject non-finite viewport coordinates before storage; prevent serving changed
  package bytes under a previously verified digest and immutable URL; and feed
  actual API response bytes into the Kotlin HTTP decoder. The old corpus codec
  expects `format/version/generation/assertions`, whereas the endpoint returns
  `generation/items/next_cursor/generated_at`. Preserve exact identity/price
  continuity from preparation; adjacent fixture tests are not one end-to-end
  proof. The prior review reproduced the first two failures in an isolated copy.
- MAP-14 T06: implement and prove canonical UUID hit-testing, independent
  overlay generations, correct fuel/electric/co-located symbols, clustering
  and the accessible equivalent. A KML title or rendered pin alone is not
  selection acceptance. Use the actual SDK APIs, not a mocked map.
- T04/T02: complete cancellation, explicit lifecycle/teardown and stale-callback
  handling, permission denial/double-init/rotation/process-death/leak checks.
- T01/T03/T07: clean-source rebuild comparison, the remaining supported ABI
  matrix, admitted minimal resources, complete transitive audit/SBOM/notices,
  endpoint/secret scan and source/update/rollback recipe. The local arm64 build
  does not close all of these tasks.
- T08: verify the packaged APK and actual 16 KB runtime, measure the remaining
  declared costs, and close ENGINE_GATE only with the required real evidence.
  Earlier x86_64/API 26 render/search/route results remain probe evidence; they
  do not demonstrate execution of the new arm64 candidate.

After DATA_GATE and ENGINE_GATE actually pass, continue MAP-15, then MAP-16
and the later map tab. Do not move the full gate into a documentation waiver.
Keep MAP-10 corridor/query-plan and MAP-12 rotation/recovery obligations visible
until implemented or explicitly reassigned with their acceptance preserved.

Run meaningful affected checks immediately; use dedicated disposable PostGIS
for SQL/concurrency changes and existing authorized emulator surfaces for native
proof. Do not touch the physical phone or staging. Leave truthful evidence and
atomic checkpoints after each bounded task; preserve previous uncommitted work
and do not invent a tested commit SHA.

## Progress since this resume (2026-10-24)

- MAP-10 review fixed: `NaN`/`Inf` viewport coordinates are rejected before
  storage; `capabilities.price_known` is now projected from the selected
  source/product price array, so a hidden community price is not reported as a
  known ANP price.
- MAP-12 review fixed: package bytes are re-read and SHA-256-verified before
  every response; a changed file returns `409 map.package-changed` instead of
  serving under the old immutable URL/ETag.
- MAP-13 review fixed: the actual API response is captured in
  `contracts/testdata/map/assertions-response-v1.json` and decoded by
  `MapDiscoveryResponseCodec`; the test cross-checks the API item against the
  prepared corpus (station UUID and exact `5890` price).
- MAP-14 T04 adapter guards added: serialized concurrent init, active search
  listener/routing container tracking, stale search/route cancellation and
  place-page listener removal. Manual double-init/rotation/relaunch/permission-
  denied browsing passed. Instrumented leak/process-death tests remain open.
- Debug probe fixture note: use an unpacked AAR that retains
  `app.organicmaps.sdk.BuildConfig` (for example a fresh `sdk-debug.aar` with
  only R classes removed). Removing BuildConfig reproduces a
  `NoClassDefFoundError`; do not delete it.
- Still open: T06 hit-test/custom symbols/clustering/accessibility, T07
  transitive license/SBOM/endpoint audit, T08 arm64/16 KB runtime and full
  ENGINE_GATE evidence, then MAP-15/16.

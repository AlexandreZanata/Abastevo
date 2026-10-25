# MAP-14 — Accepted native engine checkpoint

Status: LOCAL_DONE
Validation: PASS
Integration: INTEGRATION_PENDING
Gate: ENGINE_GATE PASS (local extraction; not production certification)
Measurements: 2026-10-24
Checkpoint review: 2026-10-25

Behavior revision: `8e707b62` plus the source changes in the commit containing
this record. The receipt pins the exact artifacts and source files. This record
supersedes the PARTIAL/deferred/legal-owner summaries in the historical records;
it does not erase their observations or accept the later map UI phases.

## What removed the blockers

- Release code and native libraries come from the pinned Gradle SDK build,
  not a debug JAR with a replacement library. The source patch removes the
  obsolete CMake IPO gold option; NDK LLD and ThinLTO remain enabled. Release
  linking strips unshipped symbol/debug metadata before computing the build ID.
- Independent root/submodule checkouts produced byte-identical ARM64 and ARM32 AARs,
  classes and native library. The clone shares immutable Git objects and the
  pinned toolchain, not native build directories. Prepared public style/atlas
  inputs are hash-pinned in `map14-final/prepared-inputs.json`; they are explicit
  inputs, not a claim that every generator was rerun from raw OSM.
- `resource-admission.json` admits 193 exact assets and excludes two fonts.
  Code2000 declares shareware registration; KhmerOS lacks an established
  preferred-source offer for this build. The other 11 fonts are unchanged.
  Eight application-language glyph samples pass; this is a bounded coverage
  check, not a claim that every Unicode character has a glyph.
- The admitted AAR embeds 231 retained source license files, upstream full
  copyright/license texts, ODbL and the source-offer recipe in a deterministic
  notices archive. Recursive nested licenses are all retained, in stable order.
  The 215-component source inventory is conservative, including pinned build
  inputs; it is not a claim that all checkout components are linked at runtime.
- Exact asset hashes, unknown resources, unsafe archive paths, changed source
  pins, missing license evidence, unclassified endpoint strings and secret
  patterns fail closed. ABI merge requires matching code/resources/notices.
- Existing SDK translations are retained; missing entries explicitly inherit
  their base-language/default fallback. They are not invented translations.
  SDK lint passes without disabling MissingTranslation.
- The APK advertises only ABIs containing `liborganicmaps.so`. Without this
  filter, auxiliary AndroidX x86 libraries could advertise an ABI with no engine.
- Release invalidates even queued readiness/overlay generations, cancels
  search/routing listeners, stops location polling and removes place listeners.
  Disposed hosts cannot revive on late callbacks. Native search IDs are unique
  per generation; stale route failures cannot call a retired owner.
- Compose waits for native initialization before creating the surface. The
  asynchronous-initialization instrumentation test failed before this fix.
- Overlay ownership uses a versioned category namespace and recognizes native
  decimal duplicate-name suffixes. Replacement deletes only prior owned
  categories after commit. Process restart/rotation no longer leaves an
  unconfirmed overlay because the SDK renamed the new category. An unrelated
  category is never adopted by a loose `Abastevo` name prefix.

The SDK surface now receives create/start/resume/pause/stop callbacks from an
AndroidX lifecycle host and pauses rendering before SurfaceView buffer teardown.
Native four-rotation tests pass on ARM64/16 KB and ARM32/Vulkan. Search updates
replace complete SDK snapshots rather than concatenating intermediate results.

## Exact artifacts and checks

Upstream `4587b8ad4d12c9f59982709fdaf03cc925948827`; 193 clean pinned
submodules; JDK 21; NDK `28.2.13676358`; CMake `3.22.1`; minSdk 26.
The source/tool/resource hashes and final measured APK are in
[acceptance.json](map14-final/acceptance.json),
[APK verification](map14-final/apk-verification.json) and
[fresh comparison](map14-final/fresh-comparison.json).
[Repeat the exact runtime proof](map14-final/REPEAT_RUNTIME.md).

The accepted local candidate is recorded in the receipt, outside Git and `/tmp`.
Consume it with `-PcomapsSdkDir=<candidate>`. Rebuild with
`scripts/maps/build-comaps-aar.sh` for both ARM ABIs, then
`scripts/maps/merge-comaps-aars.py`; see the upstream README. Static builders do
not run Android and deliberately do not declare runtime acceptance. The receipt
and this versioned evidence own the local phase gate.

- arm64 AAR: 34,349,010 bytes, SHA-256
  `43eb683f2d5ae880dd133d6ea1d1bbe312dd5ecaf0ef3536a6518ae938084db0`.
- arm64 core: 14,675,096 bytes, SHA-256
  `df51c422e3e6163e7079a53875933232577bb939396738cf05c0edb8cc509f21`.
- armv7 AAR SHA-256
  `a3bdb7da7dabd3cb408ad6ee08053d1ebb00eccda3ca18325d0b066a603fdce6`;
  core 10,499,236 bytes, SHA-256
  `c911390d5906f97f82ec4876e56f90dbe05d9d24487897f81907a40b89dc1788`.
- Merged AAR: 40,064,390 bytes, SHA-256
  `d2ec37881ede4e2288e1a929f01a1eabab193e7a6d518c76ff4342f085f2b10c`.
- Release APK: 77,794,496 bytes / 74.19 MiB, versus the previous universal
  80,407,806 bytes. Reduction 2,613,310 bytes. Existing 80 MiB ceiling unchanged.
  All packaged ARM64 LOAD segments and ZIP native entries pass 16 KB alignment;
  no Qt runtime, excluded font or unsupported advertised x86 ABI is packaged.
- Build baselines: clean armv7 Gradle native build 3m35s; arm64 relinks with
  release strip-before-ID 1m38s/independent 1m37s; final resource-only packaging
  8–13s cached. Final full affected Android assembly took about 2–3 minutes.
  Fresh prepared checkout was 3.4 GiB; reference SDK native cache 2.9 GiB.
  These are observations, not CI time or peak compiler-RSS guarantees.
- Runtime baseline after recreation: translated arm64/16 KB PSS about 341 MiB;
  translated armv7/Vulkan about 239 MiB, one Activity in each process. Native
  translation/software graphics overhead is included. Performance/battery and
  low-end native-device budgets are still owned by MAP-29; no new threshold was
  invented to turn those baselines into a performance acceptance.

## Runtime matrix and limits of the measurements

The exact tested probe uses the accepted release SDK/core inside an isolated
Abastevo application variant. It is not the fourth production tab.

- API 35 official `google_apis_ps16k/x86_64`, actual kernel page size 16,384;
  automatically selected app ABI `arm64-v8a`, executed through Android's native
  bridge. Actual light and dark Curitiba rendering, pan/zoom, 29-result offline
  search, road-following 1.6 km / 288 s route, two committed overlay generations
  and cluster hit returning all three canonical UUID members passed offline.
  This is execution on a 16 KB Android emulator, not a physical ARM64 16 KB phone.
- API 30 official `google_apis/x86_64`, explicit installed app ABI
  `armeabi-v7a` through the native bridge; isolated official emulator 37.2.12,
  Vulkan renderer with the Mesa Lavapipe backend. Actual render/search/route,
  repeated initialization, four native Activity rotations and three process
  restarts passed, including a clean-data run. Weak-reference observation
  after recreation reported `previous_host_checked=true released=true`; memory
  inventory reported one Activity. This is bounded leak evidence, not an
  exhaustive heap proof or physical ARMv7 certification.
- The historical physical arm64/4 KB proof remains in MAP-14. This continuation
  did not install anything on the physical phone.
- Compose readiness/recreation/accessibility tests use a controlled engine so
  initialization can be held pending. They are separate from the real native
  probes above. The explicit empty-before-ready regression is part of the
  final instrumentation result.

Failed environment attempts are retained: SwiftShader/Vulkan initially gave a
blank surface; the OpenGLES3 path rendered. The API 35 system_server crashed
in `android.display` during an earlier translated instrumentation attempt
(backtrace in ART/services.jar). The ARM32 OpenGL path also terminated the
host emulator process during
recreation across the attempted Android 9/11 images and graphics backends.
Completing the SDK lifecycle callbacks was necessary; it did not cure that
virtual OpenGL configuration. ARM32 Vulkan/Lavapipe passed render, four
rotations and three restarts. This accepts the recorded Vulkan configuration,
not ARM32 OpenGL-emulator stability; retain physical/driver QA in MAP-29.
These attempts are not
passing tests and do not establish an engine crash root cause. The final
accepted environment/results are recorded in the receipt; do not reuse the
unstable launch recipe and claim it passed. No Vulkan-wide or all-device
acceptance follows from this local gate.

## Rights and distribution

The MIT grant for pinned `just_gtfs` exists in ancestor `083d98e` (2020-03-28),
not only in a later tree. Full retained terms and copyright are bundled. The
upstream Freepik credit is not an admission basis: upstream issue
[3311](https://github.com/organicmaps/organicmaps/issues/3311) records replacement
of the proposed Flaticon icons before merge; source SVG metadata was checked.
Per-asset provenance/selected licenses are frozen in the resource profile.

The regional `.mwm` files and OSM-derived geometry/reference assertions retain
ODbL obligations. Separate rendering/storage alone does not make derived OSM
facts independent. Independently obtained ANP/community records retain their
own source terms. Upstream full notices and the retained source/derived-data
recipe satisfy this local source checkpoint. Publishing the durable source and
derived-data offer URL is mandatory before public distribution and remains a
release/deployment task; no distribution is authorized or claimed here.

36 combined runtime-code endpoint hosts are classified after expanding JAR/AAR
class/native entries, none unknown;
`ge0.me` is a legacy parsed URL prefix, not a required outbound service. No
secret-pattern hits or required private upstream service were found. The
native service strings remain documented unused; they were not all removed.

## Validation and continuation

- Eight Python admission/inventory tests, including negative/failure cases.
- Nineteen engine JVM tests; affected app unit tests passed earlier in this
  checkpoint; engine lint, Android test APK, normal debug, engine probe and
  release assembly/size/alignment checks pass. Final instrumentation is pinned
  in the receipt. No backend/SQL behavior changed; no redundant PostGIS run.
- Scoped source/diff/secret review and `git diff --check` pass.

Continue local construction from this checkpoint. MAP-15/16 already have local
records; do not restart them or ask again about the APK budget, linker waiver,
source-license owner stamp or permission to implement normal crypto/storage
choices. Close their retained integration items as actual engineering work:
API-26 Ed25519 verification, active-package-to-native layout and download
retry/cancel/migration cases. Then implement MAP-17's real fourth-tab route and
native Compose host, and MAP-18/19/20 markers/quick prices/minimum-map acceptance.

The dense OSM-vs-bookmark ambiguity, final native pictograms/48 dp marker
geometry, brand palette/theme control, permanent attribution, viewport update
policy and localized result list belong to that real UI acceptance. They are
explicit obligations, not accepted features or circular prerequisites requiring
finished MAP-17/18 UI before those phases can start. Full guidance/trips/EV and
performance follow their original later phases. No public endpoint activation,
production certification, push, PR, merge, wiki publication or deployment here.

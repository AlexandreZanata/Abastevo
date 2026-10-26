# Pinned CoMaps release builder

This is the owned MAP-14 builder. Local extraction acceptance is recorded in
[MAP-14_ENGINE_GATE](../../../docs/backend/evidence/maps/MAP-14_ENGINE_GATE.md)
and its exact receipt; static build tools do not run or certify Android runtime.
Upstream: `4587b8ad4d12c9f59982709fdaf03cc925948827` (Apache-2.0 core).
Preserve the upstream LICENSE/NOTICE and the separate transitive/asset audit.

## Why the source patch is necessary

The pinned SDK CMake 3.22.1 Clang module appends `-fuse-ld=gold` for IPO when
the legacy NDK toolchain leaves `CMAKE_ANDROID_NDK_VERSION` unset. NDK r28c
uses LLD and does not ship LLVMgold.so. Adding an earlier `-fuse-ld=lld` does
not fix the trailing IPO option. `AbastevoAndroidLld.cmake` removes only the
obsolete gold IPO option for NDK >=22 and retains ThinLTO for C and C++.
Release linking strips unshipped debug/symbol metadata before computing its
build ID, allowing identical native bytes across independent source paths.

`ndk-r28-lld.patch` records the root fix and the earlier explicit LLD/16 KB
SDK settings. It never modifies generated Ninja files or the diagnostic map
checker. Apply it only to the exact pin; preserve unrelated source edits.

## Build and consume

Use a prepared local source checkout with every pinned submodule and generated
map/style assets, the installed SDK/NDK, and a complete JDK 21 (a JRE alone is
insufficient). The builder is offline and refuses wrong pins or patch drift.

```sh
JAVA_HOME=/data/dev/java/candidates/java/current \
  bash scripts/maps/build-comaps-aar.sh \
    --source /data/dev/src/comaps-4587b8ad \
    --sdk /data/dev/android/sdk/Sdk \
    --output /tmp/abastevo-comaps-release-arm64 --abi arm64-v8a

JAVA_HOME=/data/dev/java/candidates/java/current \
  ./gradlew :app:assembleDebug :app:verifyReleaseApkSize \
    -PcomapsSdkDir=/tmp/abastevo-comaps-release-arm64 --offline --console=plain
```

For the all-device release, build the second ABI and merge the two verified
candidates (the merge script checks identical classes/manifest and writes
`multi-build.json`):

```sh
JAVA_HOME=/data/dev/java/candidates/java/current \
  bash scripts/maps/build-comaps-aar.sh \
    --source /data/dev/src/comaps-4587b8ad \
    --sdk /data/dev/android/sdk/Sdk \
    --output /tmp/abastevo-comaps-release-armv7 --abi armeabi-v7a

python3 scripts/maps/merge-comaps-aars.py \
  --arm64 /tmp/abastevo-comaps-release-arm64 \
  --armv7 /tmp/abastevo-comaps-release-armv7 \
  --output /tmp/abastevo-comaps-release-multi
```

The output contains the actual `sdk-release.aar`, its verified unpacked
contents, `build.json`, the source patch, runtime diff, submodule pins and build
log. The verifier refuses debug/hybrid classes, incorrect minSdk/ABI,
unstripped native libraries and misaligned 64-bit LOAD segments. The adapter
consumes the upstream release BuildConfig; it must not generate a duplicate
class or remove that class from the AAR. Consumer keep rules preserve JNI
class/member names while the final symbol-specific audit remains pending.

No patch to files under `.cxx`/`build.ninja`, debug-JAR/release-.so mixing,
ignored linker failure, algorithm substitution or changed size gate is needed.
The recorded user-approved APK budget remains 80 MiB.

## Patch reapplication and rollback

The builder accepts only the exact upstream pin. It applies the patch when the
source is unpatched, accepts it when the reverse check succeeds, and refuses
patch drift. To roll back the patch without touching unrelated source:

```sh
git -C /data/dev/src/comaps-4587b8ad apply --reverse --check \
  /data/dev/worktrees/abastevo-map-ev/maps/engine/upstream/ndk-r28-lld.patch
git -C /data/dev/src/comaps-4587b8ad apply --reverse \
  /data/dev/worktrees/abastevo-map-ev/maps/engine/upstream/ndk-r28-lld.patch
```

A changed upstream SHA or ABI set invalidates the audit, notices, SBOM and T08
runtime evidence; rebuild and re-audit from the new pin.

## Notices, SBOM and endpoint audit

```sh
python3 scripts/maps/audit-comaps-release.py \
  --source /data/dev/src/comaps-4587b8ad \
  --candidate /tmp/abastevo-comaps-release-arm64 \
  --out /tmp/abastevo-comaps-release-arm64/audit.json \
  --notices-dir /tmp/abastevo-comaps-release-arm64/notices \
  --sbom /tmp/abastevo-comaps-release-arm64/sbom.cdx.json
```

The audit covers 193 submodules and 22 vendored `3party` directories, resolves
`glm` (MIT), `just_gtfs` (MIT upstream) and the source-header/vendored licenses,
writes a CycloneDX 1.5 SBOM and a 231-license-file notice bundle, and classifies the
native endpoint strings. Resource admission is hash-pinned in `resource-admission.json`: 193 admitted
assets, Code2000/KhmerOS excluded, full notices embedded. Missing SDK localized
entries inherit explicit base/default fallbacks; existing translations remain.
The runtime scan expands compressed Java/JNI entries and fails on unknown
endpoints, credential patterns or license gaps. The distributed build input now carries a [source-access offer](../vendor/SOURCE_OFFER.md),
exact submodule pins, notices and SBOM. Any later public app/map distribution
must preserve and fulfill its own corresponding source/data obligations.

## Accepted artifact and test matrix

Persistent merged candidate:
`/data/dev/artifacts/abastevo-map-engine/d2ec37881ede4e22`.
SHA-256 `d2ec37881ede4e2288e1a929f01a1eabab193e7a6d518c76ff4342f085f2b10c`.
Both ARM AAR/classes/core comparisons pass in independent checkouts. Prepared
style/atlas inputs are explicit, pinned inputs in the evidence manifest.
Use the official source preparation scripts with its compatible Python venv
(protobuf 3.20.3), Boost bootstrap/headers and Serbian/desktop string generation;
never copy native `.cxx` or build outputs to impersonate a clean source build.
The prepared asset hashes are verified before admission.

Accepted native environments: ARM64 on an actual 16 KB API35 emulator through
the Android native bridge/OpenGLES3, and ARM32 on API30 through the native
bridge/Vulkan with Mesa Lavapipe. Four native rotations and Activity collection
pass on each; three ARM32 process restarts pass. ARM32 OpenGL recreation caused
host-emulator crashes in attempted configurations and is not accepted. The
receipt retains the precise environments; do not infer all-device certification.
The isolated official 37.2.12 emulator download/hash is recorded for repetition;
no global emulator upgrade or proprietary test-image component enters the APK.

The SDK surface must use `MapViewHelper.create` with an AndroidX lifecycle
owner, after readiness. It forwards create/start/resume/pause/stop and pauses
before buffer teardown. The adapter keeps the application core across surfaces;
construct one engine per application process. Future map UI and retained
MAP-15 integration work are listed in the current resume, not new owner gates.

For Android Ed25519, the pinned core already compiles
`libs/platform/crypto/ed25519_monocypher.cpp` and
`3party/monocypher/monocypher-ed25519.c`. The implementation route for MAP-15
is to expose that verifier behind an injected Abastevo port, without requiring
the platform Java Ed25519 provider or reimplementing cryptography. Its pinned
Monocypher source offers BSD-2-Clause (or CC0); retain the chosen BSD notices
and audit the shipped files. Before accepting the bridge, test exact Go-signed
manifest bytes, RFC 8032 vectors, altered message/key/signature, wrong lengths,
untrusted key IDs and key rotation on API 26. Do not use Java provider presence
as proof or treat this design choice as an already implemented verifier.

Room/download implementation is also an engineering task, not a pending human
choice: reuse the current database and add the next append-only migration after
checking the actual schema head, with crash/disk-full/cancellation tests.

## MAP-22 privacy revision

The MAP-22 build input was `9cdd3cda00e1025a4ba986bd385f07cf013b0584b41c9c14091ba2b4277c352c`.
It retained the same source/submodule/resource pins and added
[private-diagnostics.patch](private-diagnostics.patch): native and Java SDK
log sinks suppress GPS, route and exception payloads, including native assertion
fallbacks. Generic SDK diagnostics, native assertion source points and route error codes remain available.
See MAP-22 evidence for that revision's actual runtime matrix.

## MAP-23 explicit-guidance revision

The current build input is `7ad828883c4aca8afc46601ea98ed340db5056cbc61df2e1896d8916d98d6837`
(40,063,612 bytes; `classes.jar`
`071120e96b1c27be14d166183dd96b37751119fc4da45c74cb76a3647e0ee66a`;
`arm64-v8a` `bb2aed7bdbc26e52fc90d0cfc0cf05e01b2e7aab96119b7f2e1d01533ba1c45b`;
`armeabi-v7a` `7d61b573c6a15bcb1bd7de463ec420a02b716f88c872099ff7d8e901f3fba842`).
It retains the same source/submodule/resource pins and adds
[explicit-guidance-voice.patch](explicit-guidance-voice.patch): the SDK no
longer auto-initializes Android TTS at startup and exposes
`TtsPlayer.configureNativeNotifications` so the host enables native turn
instruction text only during explicit foreground guidance. No cloud voice or
background tracking is added.
The builder applies all three recorded patches; actual release AARs for both ABIs are
verified/admitted before merge. No resource/endpoint/license gate is bypassed.
See MAP-23 evidence for this revision's actual runtime matrix; the earlier MAP-14
comparison and acceptance above describe the prior artifact, not a new clean-clone
comparison or production certification.

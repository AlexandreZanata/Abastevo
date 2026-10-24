# Pinned CoMaps release builder

This is the MAP-14 T03 linker/build unblock, not ENGINE_GATE acceptance.
Upstream: `4587b8ad4d12c9f59982709fdaf03cc925948827` (Apache-2.0 core).
Preserve the upstream LICENSE/NOTICE and the separate transitive/asset audit.

## Why the source patch is necessary

The pinned SDK CMake 3.22.1 Clang module appends `-fuse-ld=gold` for IPO when
the legacy NDK toolchain leaves `CMAKE_ANDROID_NDK_VERSION` unset. NDK r28c
uses LLD and does not ship LLVMgold.so. Adding an earlier `-fuse-ld=lld` does
not fix the trailing IPO option. `AbastevoAndroidLld.cmake` removes only the
obsolete gold IPO option for NDK >=22 and retains ThinLTO for C and C++.

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
writes a CycloneDX 1.5 SBOM and a 219-license-file notice bundle, and classifies the
native endpoint strings. It is a technical inventory, not a legal opinion; the
human license/ODbL sign-off remains an owner obligation.

## Still required

- Fresh-checkout double-build comparison (clean-build double-build PASS) and
  final per-asset font/symbol/style admission.
- Armv7 runtime acceptance, packaged APK/16 KB runtime checks, native
  device/emulator behavior and the full lifecycle/selection matrix.
- Overlay native cluster hit-test and final Abastevo artwork; MAP-15/16 are
  LOCAL_DONE but remain dependent on ENGINE_GATE.

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

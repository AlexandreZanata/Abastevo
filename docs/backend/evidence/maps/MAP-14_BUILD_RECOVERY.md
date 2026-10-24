# MAP-14 T03 — Release linker/build recovery

Date: 2026-10-24. Status: LOCAL_DONE / INTEGRATION_PENDING (bounded linker/build task only).
Validation: PASS (source fix, real arm64 release AAR, debug/release app assembly and size/alignment checks).
Phase: ENGINE_GATE_PARTIAL. No phase closure, integration, deployment or release certification.

Worktree: `/data/dev/worktrees/abastevo-map-ev`, `codex/map-ev-implementation`,
base `721582b8` plus existing uncommitted MAP changes and this recovery task.
No tested commit is invented. Upstream: `4587b8ad4d12c9f59982709fdaf03cc925948827`.

## Failure and correction

The exact NDK 28.2.13676358/CMake 3.22.1 C/C++ ThinLTO smoke failed before the
fix: CMake appended `-fuse-ld=gold`, which tried to load absent `LLVMgold.so`.
The legacy NDK toolchain leaves `CMAKE_ANDROID_NDK_VERSION` unset; CMake's
Clang IPO compatibility branch therefore selects the obsolete linker.

The recorded source patch removes only this obsolete IPO option for modern
Android NDKs. ThinLTO remains enabled. The same C/C++ smoke then linked with
NDK LLD and `LOAD` alignment `0x4000`; no generated Ninja file was edited.
The real SDK release build subsequently passed in 3m31s, including a new
API-26 native configuration, release Java classes and Gradle AAR packaging.

Files: [source patch](../../../../maps/engine/upstream/ndk-r28-lld.patch),
[builder](../../../../scripts/maps/build-comaps-aar.sh),
[verifier](../../../../scripts/maps/verify-comaps-aar.py),
[build notes](../../../../maps/engine/upstream/README.md),
[continuation](../../../planning/MAP_EV_RESUME.md).

The adapter now consumes the original release BuildConfig instead of generating
a duplicate or removing it from classes.jar. Conservative SDK consumer rules
retain JNI names through R8. Actual mapping inspection preserved Framework,
OrganicMaps and MapObject names; narrowing the keep set remains tied to the
native callback/symbol audit.

## Artifacts actually produced

- Candidate directory: `/tmp/abastevo-comaps-release-arm64`.
- Actual Gradle `sdk-release.aar`: 35,160,271 bytes,
  SHA-256 `1fe196f928f94128f27898ace124c33fa788859bfe89111b19e1bb89848abed5`.
- Original SDK `BuildConfig.DEBUG=false`, minSdk **26**, arm64-v8a only.
- Stripped `liborganicmaps.so`: 14,675,096 bytes,
  SHA-256 `08342926e3799d29851baf093e535c3ca107dd19e218eb69b3e1447e7d155ccb`; all three LOAD segments 16 KB aligned.
- App debug with this release candidate: **91,811,421 bytes**, assembly PASS.
- App release: **69,881,875 bytes (66.64 MiB)**,
  SHA-256 `79a3117eed91aa5dffb455f0ccafacb2cc6e2223ab37d8af8511ffe50e98e678`.
- `verifyReleaseApkSize`: PASS under the already user-authorized 80 MiB limit.
- APK `zipalign -c -P 16 4`: PASS. All five packaged 64-bit native libraries
  (core plus AndroidX arm64/x86_64 libraries) have 16 KB LOAD alignment. The
  packaged core digest equals the verified AAR core digest.

The initial `/usr/lib/jvm` path supplied only a JRE; the build was rerun with
complete Temurin JDK 21 at `/data/dev/java/candidates/java/current`. The SDK
wrapper uses Gradle 8.14.4; the Abastevo wrapper uses Gradle 8.10.2.

## Commands and negative cases

```sh
JAVA_HOME=/data/dev/java/candidates/java/current \
  bash scripts/maps/build-comaps-aar.sh \
    --source /data/dev/src/comaps-4587b8ad \
    --sdk /data/dev/android/sdk/Sdk \
    --output /tmp/abastevo-comaps-release-arm64 --abi arm64-v8a

JAVA_HOME=/data/dev/java/candidates/java/current \
ANDROID_HOME=/data/dev/android/sdk/Sdk \
ANDROID_SDK_ROOT=/data/dev/android/sdk/Sdk \
  ./gradlew :app:assembleDebug :app:verifyReleaseApkSize \
    -PcomapsSdkDir=/tmp/abastevo-comaps-release-arm64 \
    --offline --no-daemon --max-workers=2 --console=plain
# BUILD SUCCESSFUL in 2m55s; 222 tasks, 77 executed, 8 from cache.

/data/dev/android/sdk/Sdk/build-tools/36.0.0/zipalign -v -c -P 16 4 \
  app/build/outputs/apk/release/app-release-unsigned.apk
```

Verifier regressions actually exercised: an original debug AAR with its manifest
changed to API 26 was refused for debug classes; the actual arm64 release AAR
was refused when x86_64 was requested; an existing valid candidate was refused
without altering its build.json. Failed candidates left no output directory.
Bash/Python syntax, patch application/reversal, diff/whitespace and scoped
secret-surface checks passed. No backend behavior changed; no repeat PostGIS
or aggregate JVM suite is claimed for this build-only task.

Local detailed logs: `/tmp/abastevo-map-unblock/` (red/green CMake smoke,
AAR/app build logs, packaging check and artifact JSON). The builder leaves the
source patch, submodule pins, runtime diff and log alongside the candidate.

## Decisions and remaining obligations

The size and manual-linker decisions no longer block continuation. For the
future MAP-15 verifier, the pinned native core already contains Monocypher
Ed25519 behind `platform::crypto::VerifyEd25519`; use an injected bridge and
prove exact Go-signature/RFC/negative/rotation/API-26 behavior rather than
relying on a Java provider. Its selected BSD-2-Clause notices/admission and JNI
bridge acceptance remain required. Room uses the next verified append-only
migration; no owner decision is needed for routine implementation.

This is an incremental local source build, not two clean-checkout reproducible
builds or a completed minimal-asset/transitive audit. The diagnostic generator
source edit is preserved outside this runtime patch. Other supported ABIs,
actual 16 KB execution, release runtime/lifecycle, custom overlay hit-test and
accessibility, source/SBOM/license/update audits and remaining budgets stay
open under MAP-14. No APK was installed, no emulator/phone was mutated, and no
commit/push/merge/deployment took place in this recovery task. The earlier
render/search/route evidence belongs to the earlier x86_64 probe, not this new
arm64 candidate. MAP-15/16 still depend on real ENGINE_GATE acceptance.

## Later update (2026-10-24)

The arm64 build above remains the historical T03 recovery record. A separate
`armeabi-v7a` candidate was subsequently built and merged with it:
[multi-ABI evidence](MAP-14_MULTI_ABI.md) records both AAR/core hashes and the
80,407,806-byte universal APK under the 80 MiB gate. Armv7 runtime and actual
16 KB execution remain open. T07 technical audit is complete with legal
sign-off pending; T06 remains partial for native cluster hit-test/final artwork.

A clean-build double-build was also run: generated `android/sdk/build` and
`android/sdk/.cxx` were removed, the arm64 AAR was rebuilt, and the AAR,
`classes.jar` and stripped core were byte-identical. See
[MAP-14_REPRODUCIBILITY](MAP-14_REPRODUCIBILITY.md). A fresh-checkout clone
comparison remains.

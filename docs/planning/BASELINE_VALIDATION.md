# Baseline validation — 2026-09-28

Source commit: `b8a52049e0294cd2d07612cedcc52b3017c5271e`.

## Completed

- Full upstream clone: successful; branch main, clean checkout, 670 tracked files.
- Source file inventory: 330 production Kotlin files, 134 unit-test files and 24 instrumentation-test files across four modules. These are file counts only.
- `bash scripts/validate-repo-baseline.sh`: exit 0 on imported destination. Checks ignore/rule/convention files; tracked-only secret scan is not effective on untracked files.
- `bash scripts/scan-secrets.sh`: exit 0 in the tracked upstream clone. Scanner exclusions and pattern limits remain as audited.

## Attempted Android baseline

```sh
GRADLE_USER_HOME=/tmp/brazil-fuel-gradle-baseline ./gradlew \
  :domain:test :application:test :data:testDebugUnitTest \
  :app:testDebugUnitTest :app:assembleDebug --no-daemon
```

Executed in the isolated upstream clone. Initial sandbox attempt could not open network sockets to download Gradle. Authorized network retry downloaded Gradle 8.10.2, configured the build and failed before tests with exit 1:

```text
Cannot find a Java installation ... {languageVersion=17, vendor=any vendor,
implementation=vendor-specific} for LINUX on x86_64.
No locally installed toolchains match and toolchain download repositories
have not been configured.
BUILD FAILED in 1m 25s
```

Active Java is Temurin 21. Inspected Java locations contain no JDK 17. The configured Android SDK symlink has no available platforms directory, so Android SDK 35 also needs verification/provisioning. No test count, coverage or APK success is asserted. No source build versions were changed, and no SDK/JDK installation was performed by this planning task.

## Next validation

## P01-T01 re-validation — 2026-09-28

Toolchain observed in the execution environment (no Android versions changed):

- `java -version` → Eclipse Temurin 21+35-LTS (Gradle launcher JVM).
  `./gradlew -q javaToolchains --no-daemon` shows Gradle auto-provisioned
  Eclipse Temurin JDK 17.0.19+10, satisfying `jvmToolchain(17)`.
- `go version` → `go1.22.2 linux/amd64` (out of support; plan-only — P01-T02
  requires Go ≥1.26, preferred 1.27.x; see `docs/backend/TOOLCHAIN.md`).
- `docker version` → Server 29.1.3 reachable; `docker compose version` →
  v2.27.0.
- `ANDROID_HOME`/`ANDROID_SDK_ROOT` = `/data/dev/android/sdk/Sdk` contains
  `platforms/android-34, android-35, android-36, android-36.1` and
  `build-tools/35.0.0`. Correction: the "no platforms directory" note above
  inspected the parent symlink `/data/dev/android/sdk/platforms` instead of
  `$ANDROID_HOME`; SDK 35 was present.
- `bash scripts/validate-repo-baseline.sh` → exit 0.
- `./gradlew :domain:test :application:test :data:testDebugUnitTest :app:testDebugUnitTest :app:assembleDebug --no-daemon` → **BUILD SUCCESSFUL
  in ~1m55s** (88 tasks: 81 executed, 2 from cache, 5 up-to-date).
  The earlier JDK-17 blocker no longer reproduces here because Gradle
  auto-downloaded the 17 toolchain over the network.

Pinned backend choices (D01) and A01/A02/A11 correction records:
`docs/backend/TOOLCHAIN.md`. Scoped prose fixes: `docs/architecture.md`
(R-ARCH-03, FTS4 labels, schema-v4 note) and `README.md` (UC-001…UC-015,
release v3.1.0, `.local` legacy note). No Android source touched.

Not executed here: instrumentation tests, coverage/lint gates, live ANP
import, Play publishing, production security.

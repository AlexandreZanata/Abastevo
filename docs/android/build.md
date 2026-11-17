# Local Android build

Instructions for developers to build the Android app on a workstation, following [AGENTS.md](../../AGENTS.md).

For **signed release** artifacts (APK/AAB, Play Store), see [release-build.md](release.md).

## Prerequisites

| Requirement | Version / notes |
|-------------|-----------------|
| JDK | **17** (Temurin recommended) |
| Android SDK | **API 36** (`compileSdk` / `targetSdk`) — see `gradle/libs.versions.toml` |
| `minSdk` | **26** (Android 8.0) |
| Gradle | Wrapper included — run `./gradlew`, do not install Gradle globally |

### Android SDK setup

Create `local.properties` at the repository root (gitignored):

```properties
sdk.dir=/path/to/Android/Sdk
```

Alternatively, set `ANDROID_HOME` (or `ANDROID_SDK_ROOT`) and ensure the SDK platform **android-36** and **build-tools** are installed (Android Studio SDK Manager or `sdkmanager`).

Do **not** commit `local.properties`, keystores, `signing.properties`, or `.env`.

## First-time clone

```bash
git clone https://github.com/AlexandreZanata/Abastevo.git
cd Abastevo
```

Run the full JVM unit test suite (same gate as CI):

```bash
./gradlew test
```

## Compile the app (debug)

The standard local compile command for day-to-day development:

```bash
./gradlew :app:assembleDebug
```

**Output:** `app/build/outputs/apk/debug/app-debug.apk`

### Install on a device or emulator

With USB debugging enabled or an emulator running:

```bash
./gradlew :app:installDebug
```

Then launch **ANP Fuel Prices** from the launcher, or:

```bash
adb shell am start -n com.anpfuel.app/.MainActivity
```

### Faster iteration (compile + install)

```bash
./gradlew :app:installDebug --no-daemon
```

Use `--no-daemon` when you want Gradle to exit after the build (useful in scripts).

## Compile all modules

| Goal | Command |
|------|---------|
| Debug APK only | `./gradlew :app:assembleDebug` |
| All variants (debug + release) | `./gradlew assemble` |
| Full build + unit tests | `./gradlew build` |
| Clean outputs | `./gradlew clean` |

### Per-module compile (no APK)

Useful when working on a single layer:

```bash
./gradlew :domain:compileKotlin
./gradlew :application:compileKotlin
./gradlew :data:compileDebugKotlin
./gradlew :app:compileDebugKotlin
```

Layer boundaries are enforced by Gradle dependencies — see [architecture.md](../architecture/android.md) and [AGENTS.md](../../AGENTS.md).

## Checks before opening a PR

Follow the same workflow as agents and contributors:

2. **Domain terms** — [glossary.md](../product/glossary.md)
3. **Domain tests first** (TDD) — `./gradlew :domain:test`
4. **Full unit suite** — `./gradlew test`
5. **Security scan** — `./gradlew securityCheck`
6. **i18n keys** for every user-visible string (`values/strings.xml`, locale variants)

Optional coverage report for `:domain`:

```bash
./gradlew :domain:jacocoTestReport
# Report: domain/build/reports/jacoco/jacocoTestReport/html/index.html
```

CI on GitHub runs `./gradlew test` on every push and pull request to `main`.

## Release build (signed)

Debug builds are unsigned and suitable for local testing. Release builds require a keystore and signing config.

```bash
./gradlew assembleRelease bundleRelease   # after signing.properties or env vars
```

Full steps: [release-build.md](release.md).

## Troubleshooting

| Symptom | Fix |
|---------|-----|
| `SDK location not found` | Create `local.properties` with `sdk.dir=...` |
| `Failed to install` | Enable USB debugging; run `adb devices` |
| `compileSdk 35` missing | Install Android SDK Platform 35 in SDK Manager |
| Wrong Java version | `java -version` must report 17; set `JAVA_HOME` |
| Stale build artifacts | `./gradlew clean :app:assembleDebug` |

## Related docs

| Document | Purpose |
|----------|---------|
| [CONTRIBUTING.md](../../CONTRIBUTING.md) | Contributor guide — TDD, layers, i18n, PRs |
| [tech-stack.md](../architecture/android-stack.md) | Libraries and versions (do not substitute) |
| [architecture.md](../architecture/android.md) | Module layout and data flow |
| [release-build.md](release.md) | Signed APK/AAB for distribution |

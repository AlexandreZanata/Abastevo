# Repeat the accepted local native proof

This is a test-only probe, not the product map downloader or fourth tab.
Use only the explicit emulator serial below; never apply these commands to a
phone or the production package. The owned disposable AVDs were stopped after
validation and retained for subsequent integration work.

## Build the probe from the accepted engine

```sh
JAVA_HOME=/data/dev/java/candidates/java/current \
ANDROID_HOME=/data/dev/android/sdk/Sdk \
./gradlew :app:assembleMapProbe \
  -PcomapsSdkDir=/data/dev/artifacts/abastevo-map-engine/d2ec37881ede4e22 \
  --offline --no-daemon --max-workers=2 --console=plain
```

## ARM32 accepted graphics environment

Official system image: `system-images;android-30;google_apis;x86_64`.
Owned AVD: `abastevo-map14-api30-x64`. Official emulator 37.2.12 download,
size and hashes are in [test-emulator.json](test-emulator.json). Its isolated
installation was `/tmp/map14-gate-sdk-tools/emulator/`; restore it from that
verified official archive if the temporary tool directory is absent.

```sh
ANDROID_SDK_ROOT=/data/dev/android/sdk/Sdk \
ANDROID_HOME=/data/dev/android/sdk/Sdk \
/tmp/map14-gate-sdk-tools/emulator/emulator \
  -avd abastevo-map14-api30-x64 -port 5584 -no-snapshot -no-window \
  -gpu lavapipe -no-audio -no-boot-anim \
  -camera-back none -camera-front none -memory 3072 -cores 2 -skin 720x1280
```

After Android boot completes, use the SDK's `platform-tools/adb` and serial
`emulator-5584`. Install the probe with `install --abi armeabi-v7a -r` to test
ARM32 explicitly on this mixed-ABI image; verify `primaryCpuAbi` with `dumpsys
package com.anpfuel.app.mapprobe`.

Before launching, seed public test maps into the probe's private
`files/261004/` using `adb push` to `/data/local/tmp/` then `shell run-as
com.anpfuel.app.mapprobe cp ...`. Original local sources were:

- `/data/dev/src/comaps-gen4/mwm/Brazil_Parana_East.mwm`;
- `/data/dev/src/comaps-4587b8ad/data/world_mwm/261004/World.mwm`;
- `/data/dev/src/comaps-4587b8ad/data/world_mwm/261004/WorldCoasts.mwm`.

Their exact hashes are in [runtime-map-inputs.json](runtime-map-inputs.json).
These are local proof inputs, not permission to use an upstream CDN for product
distribution. Set the probe's `files/settings.ini` before first launch:

```ini
PreferredGraphicsAPI=Vulkan
StoragePath=/data/data/com.anpfuel.app.mapprobe/files/
TtsEnabled=false
```

Disable Wi-Fi and cellular data on this emulator with `shell svc wifi disable`
and `shell svc data disable`. Start the explicit component:

```sh
/data/dev/android/sdk/Sdk/platform-tools/adb -s emulator-5584 shell am start \
  -n com.anpfuel.app.mapprobe/com.anpfuel.app.map.MapProbeActivity
```

Logs tagged `MapProbe`/`ComapsMapEngine` report readiness, overlay commit,
search and road route. Renderer logs must confirm `Api = Vulkan` and Mesa
Lavapipe/llvmpipe. `--ez overlayOnly true` exercises two overlay generations;
a cluster tap must return its three canonical member UUIDs. This fixture is
never a source of product station prices or geometry.

For native recreation, set `accelerometer_rotation=0` and alternate system
`user_rotation=1/0` four times, allowing each surface to settle. At least 12s
after the last creation, the bounded leak probe reports
`previous_host_checked=true released=true`; inspect `dumpsys meminfo` for one
Activity. Repeat force-stop/start for process recreation. Do not substitute a
mock renderer or count an emulator crash as a passing test.

## ARM64 and actual 16 KB kernel

Owned invocation: existing `anpfuel-16k` AVD, read-only, serial
`emulator-5580`, official API35 `google_apis_ps16k/x86_64`. The installed
emulator was 36.5.11, launched with `-read-only -no-snapshot -no-window
-gpu swiftshader_indirect -feature -Vulkan -memory 4096 -cores 2 -skin
720x1280`, no audio/cameras. Seed the same maps into the separate probe package
and choose `PreferredGraphicsAPI=OpenGLES3`. A normal install of the ARM-only
probe selects ARM64 through Android's native bridge. Verify kernel page size
with `shell getconf PAGE_SIZE` (16384), not only ELF alignment.

The recorded light/dark native routes, four rotations and Activity collection
are separate from the controlled Compose instrumentation tests. ARM32 OpenGL
attempts on the tested emulator configurations remain failed/unaccepted; use
the Vulkan recipe for the accepted ARM32 repetition and retain broader
physical/driver QA before product certification.

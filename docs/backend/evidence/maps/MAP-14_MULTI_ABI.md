# MAP-14 T08/T03 — Multi-ABI release artifact

Status: **MULTI_ABI_RELEASE_ARTIFACT_PASS / ARMV7_RUNTIME_UNPROVEN / 16KB_RUNTIME_DEFERRED**
Date: 2026-10-24. Upstream: `4587b8ad4d12c9f59982709fdaf03cc925948827`.

This records the all-device ABI extension requested by the owner: the release
now carries both `arm64-v8a` and `armeabi-v7a`, not only a specific
architecture. It is an artifact/verifier proof; it does not close
ENGINE_GATE and does not prove armv7 runtime.

## ABI candidates

| ABI | AAR bytes | AAR SHA-256 | Core bytes | Core SHA-256 | LOAD alignment |
| --- | ---: | --- | ---: | --- | --- |
| `arm64-v8a` | 35,160,271 | `1fe196f928f94128f27898ace124c33fa788859bfe89111b19e1bb89848abed5` | 14,675,096 | `08342926e3799d29851baf093e535c3ca107dd19e218eb69b3e1447e7d155ccb` | 0x4000 |
| `armeabi-v7a` | 34,517,731 | `1ac7ba2935063de63ecee104216eae07b78442487ae5bafd3985290f3fe315ec` | 10,499,236 | `4b69c2aa8c9e4c33da0278994e01d49187afd359c270559bb74a753f9fa0b28e` | 0x4000 |

Both candidates were built by `scripts/maps/build-comaps-aar.sh` from the exact
pin and verified with `verify-comaps-aar.py` (minSdk 26, release BuildConfig,
no debug/Qt, stripped, 64-bit LOAD >= 16 KB; the armv7 verifier does not require
16 KB but the library is 0x4000 aligned). Machine-readable reports:
[arm64-build.json](map14-multi-abi/arm64-build.json),
[armv7-build.json](map14-multi-abi/armv7-build.json).

## Combined candidate and release APK

`scripts/maps/merge-comaps-aars.py` copies the arm64 candidate, adds the
`jni/armeabi-v7a` core, writes a deterministic combined AAR and verifies that
`classes.jar` and `AndroidManifest.xml` are identical between ABIs.

- Combined AAR SHA-256: `1b96a8467a37a86b9aeacbe9a515d9266d3d2e663d85e65df45b99b45d64b0a4`.
- Release APK: `app-release-unsigned.apk`, **80,407,806 bytes (76.68 MB)**,
  SHA-256 `b2ede0a65b819ce59e227b5ece67bfbe555e07afeda654d15bbcac78e364b97b`.
- Contains `lib/arm64-v8a/liborganicmaps.so` and
  `lib/armeabi-v7a/liborganicmaps.so` with the hashes above.
- `:app:verifyReleaseApkSize` PASS: 80,407,806 <= 83,886,080 bytes (80 MiB),
  leaving 3,478,274 bytes (~3.32 MiB) headroom.
- `zipalign -c -P 16 4` PASS; both cores have 0x4000 LOAD alignment.
- Machine-readable combined report:
  [multi-build.json](map14-multi-abi/multi-build.json).

This rebuild also includes the two T06 accessible-list drawables
(`ic_map_station_combustion`/`ic_map_station_electric`), a 6,736-byte increase
from the prior multi-ABI artifact. The engine adapter code is still stripped by
R8 because no production screen references it; the two native cores are
packaged. MAP-17 will add the adapter and UI, so the 3.32 MiB headroom must be
rechecked then.

## Commands

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

JAVA_HOME=/data/dev/java/candidates/java/current \
  ./gradlew :app:assembleRelease :app:verifyReleaseApkSize \
    -PcomapsSdkDir=/tmp/abastevo-comaps-release-multi \
    --offline --no-daemon --max-workers=2 --console=plain

unzip -l app/build/outputs/apk/release/app-release-unsigned.apk \
  | grep 'lib/.*/liborganicmaps.so'
sha256sum app/build/outputs/apk/release/app-release-unsigned.apk
```

## Limits

- **No armv7 runtime execution**: no 32-bit device was available. The armv7
  artifact is verifier/static proof only; do not treat it as a 32-bit render
  acceptance.
- **16 KB runtime still deferred**: the arm64 phone has 4 KB pages. Static
  alignment and the T08 arm64 4 KB runtime pass do not prove a 16 KB kernel.
- The combined AAR is a merge of two independently verified single-ABI
  candidates, not a builder-native multi-ABI output. A changed upstream SHA,
  ABI, patch or native core invalidates this evidence; rebuild both candidates
  and re-merge.
- The universal APK is under the current 80 MiB gate. If MAP-17/18 adds enough
  code/assets to exceed it, use per-ABI splits or a new owner-authorized budget;
  do not silently raise the gate.
- This does not close T06 native cluster hit-test, T07 legal sign-off, 16 KB
  runtime or ENGINE_GATE.

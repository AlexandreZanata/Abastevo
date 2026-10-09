# P37-T01 split board price recognition

B-BR-PC02 / BUC-PC02 refinement, 2026-10-08: local OCR may return the integer
and decimal digits of one board price in separate tokens. Recover a suggestion
only when one nonzero integer digit (with an optional decimal separator) and
one two/three-digit fraction have comparable heights, closely aligned centers,
bounded horizontal separation and a recognized fuel label on the same row.
Require a unique fragment partner; never combine across rows or infer a fuel
from bare numeric fragments. Existing complete-price tokens take precedence.
Suggestions still require human review; this changes no submission or trust rule.

The connected POCO reproduced the current review photo: only ethanol 4.44 was
offered. The other three prices were split into integer/fraction tokens and
discarded before fuel association. Photos and raw diagnostics remain private
under the ignored corpus directory; no image, GPS or raw output enters Git.

Status: LOCAL_DONE
Validation: PASS

Scope: pure spatial parser correction, targeted regression and genuine local
device re-evaluation. No backend, dependency, deployment or remote publication.

Opening reconciliation: maintained dev was clean at `5bfd214`, with origin/main
already an ancestor. Earlier current progress entries predate subsequent actual
commits; preserve their pending obligations rather than infer remote acceptance.
Source/test fingerprint (sorted repository paths + NUL + bytes + NUL, SHA-256):
`acce722ac56b2d4df17c7c377962d90d4e53cc41706ce610603f7fb13296ccd6`.

Domain RED: the split-row regression failed with the original parser (only one
of four rows). GREEN and refactor: 17 spatial parser cases passed, including
cross-row/distant/small/zero fragments, competing integer/fraction partners,
complete-price precedence and three fractional digits without duplicate prices.

Validation commands:

```sh
./gradlew :domain:test :application:test :data:testDebugUnitTest :app:testDebugUnitTest :app:lintDebug :app:assembleDebug :data:assembleDebugAndroidTest --no-parallel --max-workers=2 --console=plain
bash scripts/check-mobile.sh --static-only
bash scripts/scan-secrets.sh
git diff --check
adb install -r -t data/build/outputs/apk/androidTest/debug/data-debug-androidTest.apk
adb shell am instrument -w -e class com.anpfuel.data.ocr.RealImageOcrDeviceTest com.anpfuel.data.test/androidx.test.runner.AndroidJUnitRunner
```

Gradle BUILD SUCCESSFUL (2m 26s): domain 464, application 307, data 282 and app
244 cases; 1,296 executed cases passed, no failures/errors. One existing opt-in
live ANP catalog test was skipped, outside this OCR scope; it is not acceptance
evidence. Lint: zero errors, 204 warnings and two informational findings.
Static/import/secret/whitespace checks passed. No formatter task is configured.

Connected POCO 2311DRK48G/API36: genuine ML Kit corpus plus corrupt/synthetic
pixel tests `OK (2 tests)`. Exact original review photo now offers ethanol
4.440, regular gasoline 6.910, diesel S500 6.350 and diesel S10 6.450 in 354ms,
with no unresolved/conditional flag. The twelve-sample private corpus changed
only the matching duplicate board: the same three missing rows were recovered;
all other suggestions stayed unchanged. This is a scoped reproduction, not
statistical OCR acceptance. Diagnostics/photos remain ignored and private.

Corrected app APK installed with `adb install -r` (`Success`), without clearing
account/app data. The added duplicate diagnostic input and shared-storage UI
dump were removed from the device; the original app review photo is preserved.

Next: re-read the existing photo with the updated APK or take another photo;
all offered values still need human confirmation. Source integration remains
INTEGRATION_PENDING; broader PC05/PC06 and release obligations remain owed.

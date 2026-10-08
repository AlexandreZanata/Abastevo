# P37-T01 expanded OCR format and scenario validation

B-BR-PC02 / BUC-PC02 refinement, 2026-10-08: recognition must preserve exact
milli-BRL and product association over board decimal/separator/layout variants.
Unknown diesel specifications, true-premium variants, competing prices for one
normalized product and ambiguous geometry require manual review. Never infer
missing digits, select the cheapest condition or turn totals, volume, dates,
addresses or zero displays into fuel suggestions. Conflicting valid prices
remain available for manual selection; pixel ambiguity is not resolved by a
parser guessing digits. Cropping must retain the fuel labels.

One bounded local acceptance task on maintained dev parent `82aaa22`: a
repeatable manually checked twenty-photo corpus, stratified outcome metrics,
portable negative/layout regressions and actual-device synthetic pixels. Private
images/raw OCR stay outside Git. Annotated smoke/diagnostic images are a targeted
regression corpus, not an independent statistical sample or national acceptance.
No training collection, dependency, new backend submission or model replacement.

Bounded pixel refinement: at most three on-device views (upright original,
label/price-derived board crop enlarged to at most 2048px, grayscale enlarged
crop). A unique reading stays a human-review suggestion. If views disagree on
one product, require a two-view exact agreement; tied amounts stay manual.
Never average, edit digits or select a minimum. Conditions remain conservative.
These correlated views are not calibrated statistical confidence. Original
32MiB/100MP input, EXIF correction and fifteen-second capture timeout remain.

Status: LOCAL_DONE
Validation: PASS

Behavior/evaluation fingerprint (sorted twelve changed code/test/manifest/report
paths, NUL separators and bytes): `a54106d83784dca4c460576e4cd1a032695fcd9be01ce84c091cc4832e5ebf35`. Source parent: `82aaa22`.

[Anonymous ground truth](ocr-scenarios.json) and [sanitized exact-value results](ocr-evaluation-20261008.json)
cover twenty unique, hash-identified real photos. Initial same-corpus reading:
51/76 exact product-and-value pairs, 25 misses and three incorrect associations.
Final: 60/76 exact (78.95% recall), sixteen misses and zero incorrect associated
rows (100% precision on this targeted corpus). Required cash-condition detection
was preserved. Four of five explicitly manual prices were recovered. Unassigned
numbers remain a separate limitation: 27 match visible prices and seven do not;
these are manual candidates, excluded from associated-row precision, and must
never be described as correct automatic prices. No population accuracy claim.

The actual three-view pipeline took p50 983ms / nearest-rank p95 1289ms on the
POCO API36. Timing excludes the separate raw-token diagnostic pass. All twenty
inputs recognized within the production fifteen-second timeout; SHA-256 is
measured on-device and checked before scoring. No trained model or digit repair.
Any view's priced low-confidence label can withhold a product across views but
never assign it. Cross-view same-price/different-fuel associations require
co-observation in a view; genuine equal-price rows remain supported. Explicit
conflicting products cannot be hidden by another view missing a row.

Validation:

- RED: unknown diesel spec, offset multi-line S10, missing manual conflicts,
  total/volume children, E/GRID heading, uncertain-label conflict and cross-view
  grade ambiguity exposed failures before their fixes. Existing smoke tests were
  corrected to assert the now-recovered S10 and the standing bare-Diesel policy,
  while unknown explicit specs remain manual.
- GREEN: 479 domain (including 32 spatial/view cases), 307 application, 281 data
  and 246 app unit cases executed without failure/error. One unrelated opt-in
  live ANP catalog case remains skipped (282 discovered data cases).
- POCO: `OcrFormatMatrixDeviceTest,RealImageOcrDeviceTest` → `OK (5 tests)`,
  44.485s: eighteen decimal/currency/color/layout combinations, all eight EXIF
  orientations, zero/total/volume/date and empty/corrupt/over-limit inputs, native
  twenty-photo evaluation and prior synthetic two-fuel pixels.
- `:domain:test :application:test :data:testDebugUnitTest :app:testDebugUnitTest
  :data:lintDebug :app:lintDebug :data:assembleDebugAndroidTest :app:assembleDebug`
  passed on final production code. Both lints have zero errors. App/data have
  204/18 pre-existing warning totals respectively; no lint suppression/baseline.
  The affected data-module standalone check exposed undeclared existing app
  permissions and two permission guards opaque to lint. Its manifest now declares
  the same four permissions already in the app; location propagates an explicit
  SecurityException and notification denial/revocation fails closed. No new app
  permission or prompt was added.
- Five scorer cases verify exact milli/product mismatches, missing conditions,
  absent/duplicate/extra samples, device hash mismatch, privacy minimization and
  explicit failed budgets. `--require-device-hash --minimum-exact 51
  --maximum-wrong 0` passed. The minimum retains the measured baseline coverage;
  it is a regression gate, not a commercial release threshold.
- Static mobile, secret-surface and diff checks passed. No dependency or backend
  mutation; compiled app installation is combined with the active owner-scope
  send recovery. Temporary native diagnostics are excluded from source delivery.

Reproduction (private inputs are supplied only in the device test package):

```sh
adb shell am instrument -w -e class com.anpfuel.data.ocr.OcrFormatMatrixDeviceTest,com.anpfuel.data.ocr.RealImageOcrDeviceTest com.anpfuel.data.test/androidx.test.runner.AndroidJUnitRunner
python3 scripts/score-ocr.py docs/mobile/ocr-scenarios.json PRIVATE_RESULTS.json --output PRIVATE_SCORE.json --require-device-hash --minimum-exact 51 --maximum-wrong 0
PYTHONDONTWRITEBYTECODE=1 python3 -m unittest discover -s scripts/tests -p test_score_ocr.py
```

PC06 independent expanded acceptance remains OWED: the regression set was used
for diagnosis and is not a holdout. Distant/worn/pump digits and raw unassigned
noise still require crop/retake/manual confirmation. Human review and existing
condition/auth/media rules remain essential. No statistical national acceptance,
G09/G24 certification, training collection, external publication or universal
image-recognition guarantee follows from these results.

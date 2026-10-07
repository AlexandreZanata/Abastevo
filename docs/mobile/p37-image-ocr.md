# P37-PC02 real image OCR

B-BR-PC02 / BUC-PC02: bounded local pixel recognition produces suggestions for
explicit human confirmation. Spatial proximity connects fuel labels and prices.
Conflicting prices, generic diesel and unsupported true premium are not guessed.
Zero values and pump totals do not become prices. Conditional boards require
explicit review; absence of payment inputs does not mean an unconditional price.

Dependency review (2026-10-07): final Latin recognizer is the on-device
Play-Services `com.google.android.gms:play-services-mlkit-text-recognition:19.0.1`.
Initial comparison used bundled `com.google.mlkit:text-recognition:16.0.1`
for immediate offline recognition including first use. The prior comment incorrectly described this
artifact as unbundled and Apache-2.0. The SDK and models are governed by the
[ML Kit terms](https://developers.google.com/ml-kit/terms), incorporating Google
API terms. Apache-2.0 refers to their sample code, not the proprietary SDK.
[Official integration guide](https://developers.google.com/ml-kit/vision/text-recognition/v2/android).
No new capture permissions, no server recognition. SDK usage/performance metrics
may contact Google; input images and recognition output are processed on-device.
This disclosure must appear in app privacy text. Transitives are resolved from
Google Maven; no dynamic versions. Release size must remain below 15 MB.

Decoder probes headers first, rejects >32 MiB encoded input and >100MP bounds,
samples to bounded dimensions and normalizes JPEG EXIF orientation, then releases
native bitmap/recognizer resources. Work runs off the UI thread; cancellation
suppresses publication of stale analysis. Private device corpus is loaded only
at runtime by instrumentation; no real photos enter Git or APK assets.

Status: LOCAL_DONE / INTEGRATION_PENDING. Native adapter benchmark is complete; production review integration and statistical acceptance continue in PC03/PC06.

Initial genuine POCO run (2026-10-07), model 2311DRK48G / API 36:
12 private images recognized, synthetic pixel/invalid-input instrumentation
passed (2 tests). Cold first image 404ms, remaining 163–282ms. Baseline association
is NOT_ACCEPTED: 13/40 supported rows matched, three false suggestions (two variants and one LED digit).
Raw text/geometry and results stay in ignored `combustiveis/teste_ocr/device/`.
Android refused installation until the owner enabled USB installation; no app
user data was erased. ExifInterface 1.3.7 (AndroidX, Apache-2.0) normalizes rotations.

Model choice revised after measurement: bundled native libraries alone compress
to 16.16 MiB across required Android ABIs, exceeding the entire 15 MB release
budget before app/model assets. Production now uses pinned
`com.google.android.gms:play-services-mlkit-text-recognition:19.0.1` and
`play-services-base:18.11.0` per the
[official module-install guide](https://developers.google.com/android/guides/module-install-apis).
The API checks availability before recognizing; a missing model requests a
model-only download and returns a stable pending outcome for retry/manual entry.
Photos are never sent with that request. Offline first use without an installed
model cannot recognize and must not pretend otherwise. Once installed, pixels
remain local. The private harness waits at most 45 seconds for model readiness.


Measured model comparison: bundled release APK 46,454,168 bytes; final chosen
Play-Services release APK 4,048,037 bytes, below 15 MB. The size build passed
release compilation/R8/vital lint. Subsequent pure association corrections are
covered by the final native debug run; their next release build is part of PC03.

Selected regression validation: 34 unit tests passed across spatial association,
portable money/parser and fuel-wire mapping; native private-corpus and synthetic
pixel/corrupt tests passed on POCO. Mobile static/import/no-background-tracking
checks, `git diff --check` and tracked secret scan passed. No real photographs,
raw OCR or coordinates enter Git. Native raw diagnostics and per-image scores
stay private in ignored device artifacts.

Limits: a twelve-image challenge smoke is not statistical accuracy acceptance.
Generic diesel, true premium, absent labels, worn digits and conditional prices
require explicit manual review/crop. One LED digit remains wrong in the smoke;
no automatic submission is permitted. Training collection is not involved.

A verified-TLS staging probe from this host failed because the network presents
a Fortinet-issued certificate outside the local CA trust. TLS was not disabled;
this is an integration limitation, not proof of a backend outage or deployment.

Final genuine POCO run: 12/12 images processed, 21/40 supported exact rows,
19 misses and one incorrect LED digit on the explicitly conditional board.
Precision 21/22 (95.45%), recall 21/40 (52.5%); p50 331.5ms, p95/cold 769ms.
All other offered rows matched the manual smoke labels. This challenge corpus
makes crop, omission and human correction essential; no broad accuracy claim.

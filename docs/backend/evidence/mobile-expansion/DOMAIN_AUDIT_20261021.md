# Existing domain boundary audit

Status: LOCAL_DONE
Validation: PASS
Date: 2026-10-21. Parent: d9b31fe4, with EE-11 corrections under review.

The user explicitly requested a broad audit before four more construction
units. Actual `:domain:jacocoTestCoverageVerification` reproduces the existing
0.87/0.90 failure; `/tmp/bfp-ee11-acceptance-jvm.log`. Do not lower the limit,
exclude more packages or assert DTO getters merely to improve a percentage.

Test existing observable contracts: exact portable money/capacity grammar and
bounds (BR-002/023), financial matched windows and raw command refusals
(B-BR-MEX-03–06), official/community source separation and honest unknown/read
boundaries (P10/P35), quality revision selection and private report validation
(FQ-14/15), and existing signed-claim export refusals (P32). No new activation,
product policy or application feature is authorized by these tests.

Acceptance: meaningful positive/negative/boundary tests of those existing
rules, complete domain suite and unchanged 90% gate, existing Go/Kotlin/Rust
financial parity, affected client compilation and source/diff/secret checks.
Raw/unsupported/provider/native/production obligations stay separately named.

Actual result: unchanged90% gate PASS;24,474 covered/2,701 missed instructions
in its existing scope (90.0607%). No threshold, exclusions, production policy
or build setting changed. Tests cover actual refusals and results: malformed
money/capacity precision, matched financial windows/amounts, invalid outbox
intents, immutable price snapshots, distinct unknown/available community data,
station identity/location, private Unicode/date/reference claims, selectable
official revisions, unsupported claim-export scopes, selected-station fallback,
country-suffix normalization and OCR currency hints preserving exact milli-BRL.
No tests were added merely to assert generated getters/copy/hashCode behavior.

The first meaningful boundary batch passed all behavior tests but left the
global gate at0.89; missing fallback/OCR/navigation contract cases completed
the declared scope. Failed gate logs are retained. Final full command:

```sh
./gradlew --offline --no-daemon --max-workers=2 :domain:test :domain:jacocoTestCoverageVerification :application:test :data:testDebugUnitTest :app:testDebugUnitTest :app:assembleDebug :app:assembleDebugAndroidTest :app:lintDebug
```

PASS in1m23s; domain632/application390/data450(two existing opt-in skips)/
app375, zero failures/errors. Debug app/test APK compilation and lint pass;
this does not claim device execution. Final log:
`/tmp/bfp-domain-audit-complete.log`. Earlier RED coverage results remain
in `/tmp/bfp-ee11-acceptance-jvm.log` and `/tmp/bfp-domain-audit-*.log`.
Integration pending; no source/provider feature is activated by these tests.

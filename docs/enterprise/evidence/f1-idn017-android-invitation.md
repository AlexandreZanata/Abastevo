# EMP-IDN-017 — Android existing-account invitation acceptance

Date: 2026-11-13.
Status: LOCAL_DONE
Validation: PASS
Integration: INTEGRATION_PENDING

Construction source: tested016 checkpoint
`ae5defcd4446c3f76f60dd21ac83e32978a2deb9`.
Owned branch `codex/enterprise-idn016-018`; [task213](https://github.com/AlexandreZanata/Abastevo/issues/213)
and cumulative [PR203](https://github.com/AlexandreZanata/Abastevo/pull/203).
B-BR-EMP-92/93, BUC-EMP-47 and EDR-058 precede behavior. No publication or
remote-check wait separates this task from its tested016 parent.

## Recommended decisions

Use the existing personal account/family proof and the actual company-cookie
invitation API. The native client keeps credentials only in memory and cannot
send business authority through the personal signed/bearer plane. Verify the
known `enterprise-invitation-v1` notice and its source text before presenting
its localized equivalent. Unknown semantics fail closed. Acknowledgement state
and membership state are checked together; ACCEPTED/INVITED stays pending.

Fresh proof has a deliberate existing-account key-entry route; no forced logout
or personal-family revocation is introduced. Acceptance does not enrol a device,
unlock a private vault, activate managed access or grant offline authority.
Observable local fence epochs invalidate the UI without waiting for network or
filesystem I/O. Invitation input is never saved by navigation/SavedState.

## Physical test isolation and dependency review

The user explicitly requires the connected physical ADB phone and forbids an
emulator. The phone already has normal debug/map-probe apps. A strict opt-in
`enterpriseDeviceValidation=true` uses a separate `.enterprisevalidation` package
and dedicated test sources/manifest/runner. Existing applications and data are
preserved; the existing clear-package-data runner setting applies only to this
new disposable validation package. No device identifiers or personal screenshots
are recorded.

Conventional Hilt test bindings are needed to drive actual MainActivity navigation
without putting synthetic account proofs into live staging requests. The
test-only `com.google.dagger:hilt-android-testing:2.53.1` and matching compiler
reuse the repository's pinned runtime/compiler version. Its locally cached Maven
POM declares Apache-2.0 and Google/Dagger ownership; transitive injection artifacts
match the existing family. It is confined to the opt-in instrumentation classpath,
introduces no production dependency or network service and allows redirect/error/
identity transport tests separately. No independent advisory scan is claimed.

## Validation

Domain RED precedes implementation. The new domain's4 tests pass; its four
invitation classes have100% line and92.9% instruction coverage in the focused
JaCoCo report. This is scoped new-code evidence, not the historical global
Kotlin coverage gate. The final focused debug run passes domain enterprise228,
application enterprise15 and data enterprise49 tests with zero failures/errors/
skips, including17 invitation HTTP cases. Earlier notes claiming80/81 data tests
incorrectly counted32 stale release XML results alongside the selected debug
results; they are not current acceptance evidence. Data Android compilation/KSP/
Hilt pass. The affected app selection covers invitation, reauthentication and
the existing AuthViewModel tests:33 pass. The final focused build, both APKs
and full app lint pass in2m04s.

Transport review exposed and corrected nominal repeated-preview session
exhaustion, body reads on the UI thread, cancellation/resource leaks and a
concurrent-open generation race. A single account/origin-bound memory retirement
hint is used only for login replacement, never business authority. Same-family
token refresh remains valid; family/epoch changes during open require a new
preview. Invalidation cancels owned calls, including streaming bodies. The
entire HTTP body read/close is on IO; prompt cancellation closes an undelivered
response. Atomic generation capture allows only the latest concurrent open.
The source still enforces freshness/idle/expiry; no phone clock grants authority.
Source comparison also corrected a timestamp assumption: the server's eight-hour
lifetime starts at cookie creation, not the personal proof authentication time.
The response omits creation time, so the client parses timestamps and checks
expiry after authentication without inventing an upper bound from the wrong
anchor. A five-minute-old valid proof remains usable through the actual source
response shape.

Actual TLS/ordinary PostGIS/race regression
`TestNativeCompanyCookieRotationKeepsAdmissionBoundedAndAccountsIndependent`
passes in3.344s and again in1.937s during finalization: twelve old-cookie replacements keep live/total sessions bounded,
old cookie reuse fails, an independent web cookie and personal families remain
valid, and foreign account proof cannot retire the owner's cookie. Native data
tests separately use actual OkHttp callbacks with synthetic HTTPS interceptors;
they are not represented as a deployed native/backend integration.

Both normal and opt-in Android builds compile. The final isolated build passes
in49s. Manifest/DEX inspection proves application
`com.anpfuel.app.enterprisevalidation`, test
`com.anpfuel.app.enterprisevalidation.test`, the exact target and Hilt runner,
and absence of legacy instrumentation fixtures/provider authority. The isolated
package was absent before installation. Existing normal-debug metadata is
checked before/after without reading app data. All45 new strings exist in each
of eight locale directories, with matching placeholders and no duplicate keys.
Twenty Android release-script and four enterprise-protection checks pass.

The first physical installation was refused by the phone before any test ran.
The maintainer explicitly requested a retry and accepted its installer prompt.
The first Android16/API36 run executed six journeys: four passed and two timed
out during return-after-login/re-preview-after-background. The harness now waits
for exact masked input and actual foreground/window readiness. The unchanged
candidate subsequently passed all six journeys; this diagnostic pass did not
close the task because independent bytecode review found a separate privacy
defect. Delegated Compose epoch capture let an old callback read a newer epoch
while retaining the preceding private input holder. The rendered epoch is now
an immutable primitive per composition; the compiled callback captures a long,
not State<Long>. Both ViewModel guards reject already stale or queued intent.
The corrected candidate passes all six final physical journeys with zero
failures/errors/skips. A further diagnostic run had5 passes and one IME/bounds
timeout; the helper now closes the keyboard through standard Activity insets/
InputMethodManager APIs, preserving exact masked-input, enabled/displayed,
stable-bounds and five-second assertions. Only the isolated test APK changed.
An internal Kotlin FIR lint crash passed on an unchanged single-worker retry;
no suppression or check was removed.

Final native report:
`app/build/reports/enterprise-device-validation/final-all-20261113T104359/TEST-EnterpriseInvitationDeviceTest.xml`.
Each method runs in a fresh instrumentation process and must execute exactly
one test; zero-test output is rejected. Final application APK SHA256:
`847bd7490016232e73f1a25713deb4b4955f63750a54b1e84c0095107d13e087`;
test APK:
`dcb4e0fa923d3c9fafc6b170887525cd31cd53d5bb1e7034a647c2856bd43c2d`.
The normal-debug package metadata remains unchanged. Only the newly isolated
validation package is cleared between methods. All six native cases cover
consent/pending confirmation, ACTIVE, offline/invalid/decline, guest/fresh key
entry, immutable uncertain-decision retry and background/late acknowledgement.
No emulator, existing app data clearing, security-setting change, release,
deployment or readiness claim is involved.

Focused commands use the pinned JDK17/Android SDK and admitted local map engine;
the integration command targets only the owned disposable PostGIS database:

```sh
./gradlew --offline --no-daemon --no-parallel --max-workers=2 -PenterpriseDeviceValidation=true -PcomapsSdkDir=build/comaps/7ad828883c4aca8a :domain:test --tests 'com.anpfuel.domain.enterprise.*' :application:test --tests 'com.anpfuel.application.enterprise.*' :data:testDebugUnitTest --tests 'com.anpfuel.data.enterprise.*' :app:testDebugUnitTest --tests 'com.anpfuel.app.ui.enterprise.EnterpriseInvitationViewModelTest' --tests 'com.anpfuel.app.ui.auth.AuthViewModelTest' --tests '*AuthReauthentication*' :app:assembleDebug :app:assembleDebugAndroidTest :app:lintDebug
go test -tags=integration -race -count=1 -timeout=20m ./internal/modules/companysession/adapters -run '^TestNativeCompanyCookieRotationKeepsAdmissionBoundedAndAccountsIndependent$'
git diff --cached --check
git diff --cached --binary | gitleaks stdin --redact --no-banner
```

Full Quick verification and the batch union regression run once after018. This
task checkpoint does not close PRIV-003, managed access, F1/M1 or deployment.

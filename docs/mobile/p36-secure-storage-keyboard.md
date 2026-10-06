# P36-T01 Android key custody and keyboard regression repair

Status: LOCAL_DONE
Validation: PASS

Scope: current key-account onboarding and every Android screen using AnpScaffold; portable account orchestration remains shared-domain compatible. Base: d0a3565 on dev. An isolated managed worktree preserves another contributor's uncommitted UI/crypto work. No iOS native changes, dependency upgrades or backend contract changes.

## Behavior before implementation

- B-BR-AK06: AndroidKeyStore randomized AES/GCM encryption must let Cipher generate its IV. Keep encryption authenticated, keys non-exportable, per-environment isolation and existing envelope versions/decryption compatible. No plaintext fallback.
- B-BR-AK07: account creation immediately attempts login and stores the issued recovery key plus session locally. Persistence success must acknowledge the disk write, not only SharedPreferences memory. A storage failure remains visible and the issued key remains available for manual recovery/retry; do not navigate as authenticated on failure.
- B-BR-AK08: closing/reopening, backgrounding, transient network failure and ordinary token/family expiry retain credentials and use existing verified renewal. Explicit logout/deletion clears credentials. Server revocation, reuse, suspension/deletion and invalid keys still deny access; local persistence never overrides server authority.
- B-BR-UI06: reserve keyboard/system insets once across shell, screen Scaffold and nested inputs. Consume the supplied content padding before nested inset modifiers, request adjustResize, retain native IME animation/scroll-to-focused-field behavior and adapt to system-reported sizes rather than fixed keyboard heights. Modal sheets/dialogs retain their platform-owned insets.
- BUC-AK06: signup → show/export optional backup while already signed in → Continue → authenticated navigation; restart reads sealed credentials without asking for a key again.
- BUC-UI06: focus any screen input → native keyboard → usable scroll viewport without duplicate keyboard-height blank space → dismissal restores safe bars.

## Research and diagnosis

Android [KeyGenParameterSpec.Builder](https://developer.android.com/reference/android/security/keystore/KeyGenParameterSpec.Builder#setRandomizedEncryptionRequired(boolean)) rejects caller IVs for randomized encryption; initialize Cipher with ENCRYPT_MODE and key only, then persist cipher.iv. Current source supplies GCMParameterSpec on encryption, causing device-only storage failures concealed by software-key unit tests.

Android [inset consumption](https://developer.android.com/develop/ui/compose/system/insets-ui#inset-consumption) requires consumption of PaddingValues when ordinary padding has already reserved insets. Current safeDrawing includes the IME, while AuthScreen and StationPageScreen additionally use imePadding without consuming Scaffold padding. The Activity also omits adjustResize.

[Kotlin Multiplatform platform behavior](https://kotlinlang.org/docs/multiplatform/compose-platform-specifics.html) is platform dependent. ADR-015 keeps Android Compose presentation and native iOS adapters; this repair does not pretend the repository already uses shared Compose UI or certify every phone/iOS.

## Required evidence

Real emulator Keystore regression (non-exportable randomized keys, recreation, distinct IVs, wrong-key/tampering/unavailable storage), deterministic persistence failure cases, portable expiry/logout/offline and serialized concurrency suites, ViewModel signup/continue, synthetic inset geometry and focus tests, affected unit suites, lint/APK/instrumentation compilation, diff/static/secret review. Physical-device and exhaustive OEM/navigation/font/rotation matrix remain explicit limitations. No real user account/key/GPS/media enters tests or logs.

## Validation fixture and base repairs

- The native-IME test uses Activity.setContent to avoid the rule's fake platform input session. Restore adjustResize after host attachment/focus; observed test-host adjustPan on API 26 displayed a keyboard but reported no resize inset. The production Activity requests adjustResize in its manifest. The existing system-bar mock attached a listener before content and dropped its event on API 26; inject the same explicit bar insets at the Scaffold boundary and assert exact pixel geometry instead of merely positive padding.
- The base d0a3565 failed Android lint for feed_best/feed_worst/feed_rating_value missing in six supported locales. Added those exact translations to remove the known baseline errors; no lint suppression/baseline or unrelated feature expansion.

## Final local evidence (2026-10-06)

- RED: new application storage-error regressions failed (2/28), durable preference regressions failed (2/12); real API-26 Keystore failed with `InvalidAlgorithmParameterException: Caller-provided IV not permitted`. The nested inset geometry test failed with expected viewport 1487px versus actual 1243px (the extra 24px top + 220px bottom reservation). Failures were corrected without suppressions or skipped tests.
- GREEN unit/security/concurrency: `./gradlew :application:test --tests '*PortableAuthFlowTest' :data:testDebugUnitTest --tests '*KeystoreAccountKeyTest' --tests '*KeystoreSessionStoreTest' --tests '*SessionEnvelopeTest' --tests '*SerializedAuthOperationsTest' --tests '*AccountHttpApiTest' :app:testDebugUnitTest --tests '*AuthViewModelTest' --tests '*SecurityConfigurationTest' --tests '*AuthDeepLinkManifestTest' :app:assembleDebugAndroidTest :app:lintDebug :app:assembleDebug --console=plain`. 29 application + 35 data + 28 app = 92 tests, zero failures/errors/skips. Lint and debug APK passed. Three baseline translation errors repaired, with unchanged format placeholders.
- Final instrumentation compile after host-policy adjustment: `./gradlew :app:assembleDebugAndroidTest --console=plain` passed. Direct emulator invocation below intentionally selects emulator-5554 instead of the attached physical phone and uses isolated synthetic credentials/aliases only.
- Real Keystore: `adb -s emulator-5554 install -r data/build/outputs/apk/androidTest/debug/data-debug-androidTest.apk`; `adb -s emulator-5554 shell am instrument -w -r -e class com.anpfuel.data.local.auth.KeystoreAccountStorageDeviceTest com.anpfuel.data.test/androidx.test.runner.AndroidJUnitRunner` — 1 passed.
- Real Compose screen/native IME: install debug app and app test APK with `adb -s emulator-5554 install -r`, then `adb -s emulator-5554 shell am instrument -w -r -e class com.anpfuel.app.ui.components.AnpScaffoldTest,com.anpfuel.app.ui.auth.AuthAccountContinuityDeviceTest com.anpfuel.app.debug.test/androidx.test.runner.AndroidJUnitRunner` — 4 passed. Exact system bar geometry; 220/360/0px keyboard reservations; focused field above native IME; create via keyboard Done → automatic login → Continue without second login → fresh flow rehydration → explicit offline logout → fresh flow logged out. Server port is synthetic, not evidence of a live staging registration.
- `bash scripts/check-mobile.sh --static-only`, `bash scripts/check-security.sh --static-only`, `bash scripts/scan-secrets.sh`, locale XML/placeholder uniqueness review and `git diff --check` passed. New files were reviewed separately and staged for the final tracked-secret scan. No dependencies, SQL, migrations, backend contracts, native iOS code or user data changed.
- Environment: configured JDK 21/Gradle 8.10.2/Android SDK 35; instrumentation on the existing API-26 x86_64 emulator. Physical phone untouched. No all-OEM/iOS/landscape/font-scale/device certification claimed; hardware and live provider/staging matrix remain OWED.

## Delivery and recovery

Status: LOCAL_DONE / INTEGRATION_PENDING. Temporary branch `codex/account-keyboard-fix` descends from occupied dev head d0a3565; original dev files and all other contributor changes were preserved. No push, main merge, PR mutation, wiki sync, deployment or release was attempted. Integrate this atomic commit into dev when its owner releases the checkout, reconcile any UI overlap while retaining the other work, revalidate affected combined inputs and use protected dev → main delivery with current-head `Quick verification`.

Existing v1 envelopes remain readable (explicit legacy caller-IV envelope test). Rollback is a new revert commit; keep existing encrypted preferences/Keystore aliases. Never clear user app data or downgrade custody to plaintext. Normal process exit, offline use and ordinary expiry preserve account continuity; explicit logout/deletion clears it, and server security denial still takes precedence.

Behavior/test/resource source fingerprint (sorted path + NUL + bytes + NUL): `198c8f5b476359a562591a6c246aeb69ab6275ba720bbe81d82ccf48f2114cd6`.

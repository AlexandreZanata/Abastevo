# P36 secure storage and keyboard integration with branding

Status: LOCAL_DONE / INTEGRATION_PENDING
Validation: PASS
Date: 2026-10-06

The checkout owner completed and committed the separate branding task before integration. The clean maintained dev head `550958c` contains the shared README-style login/station banner, default station artwork and username-taken field feedback. Merge the tested isolated repair `4dca43c` without rewriting either history. Both descend from `d0a3565`; fetched main `a424688` remains an ancestor. The user authorized sending the combined work to GitHub.

## Preserved behavior and conflict resolution

The [repair contract](p36-secure-storage-keyboard.md) remains authoritative for B-BR-AK06–08, B-BR-UI06 and BUC-AK06/BUC-UI06. Resolve the three overlapping files by retaining the shared banner and username validation, adding keyboard Done/focus handling, keeping provider-generated AES/GCM IVs plus length validation, and preserving durable preference writes and existing envelope compatibility. The other task's photo encryption fix remains intact.

The combined app creates an account and signs in immediately, stores sealed key/session credentials with disk acknowledgement, and continues without a second login. A fresh flow reloads those credentials; explicit offline logout clears them. Existing server revocation/security denial and recovery rules remain enforced. Scaffold consumes already-reserved keyboard/system padding once, using native sizes and adjustResize. No new dependency, backend behavior, migration or native iOS change is introduced by this integration.

## Combined validation

- Unit/security/concurrency: `./gradlew :application:test --tests '*PortableAuthFlowTest' :data:testDebugUnitTest --tests '*KeystoreAccountKeyTest' --tests '*KeystoreSessionStoreTest' --tests '*SessionEnvelopeTest' --tests '*SerializedAuthOperationsTest' --tests '*AccountHttpApiTest' --tests '*PhotoBlobSealTest' --tests '*AndroidPhotoCacheTest' :app:testDebugUnitTest :data:assembleDebugAndroidTest :app:assembleDebugAndroidTest :app:lintDebug :app:assembleDebug --console=plain`. 29 application + 51 data + 223 app = 303 tests, zero failures/errors/skips. APK compilation passed. This first combined invocation exposed missing accessibility translations for auth_brand_wordmark in the separate branding task.
- Added actual logo descriptions in all six affected locales (de/es/fr/ja/ru/zh-rCN), with unique XML keys. No lint suppression/baseline or translatable=false shortcut. Final `./gradlew :app:lintDebug :app:assembleDebug :app:assembleDebugAndroidTest --console=plain` passed. Only translations changed after the successful combined tests; no application behavior or test code changed.
- Real API-26 Keystore: install the data test APK and run `adb -s emulator-5554 shell am instrument -w -r -e class com.anpfuel.data.local.auth.KeystoreAccountStorageDeviceTest com.anpfuel.data.test/androidx.test.runner.AndroidJUnitRunner`: OK (1 test).
- Combined branded screen/native IME: install debug app and app test APK, then `adb -s emulator-5554 shell am instrument -w -r -e class com.anpfuel.app.ui.components.AnpScaffoldTest,com.anpfuel.app.ui.auth.AuthAccountContinuityDeviceTest com.anpfuel.app.debug.test/androidx.test.runner.AndroidJUnitRunner`: OK (4 tests). Verified exact bar geometry, 220/360/0px keyboard reservation, focused scroll input visibility, signup via Done, Continue, reload and explicit offline logout.
- `bash scripts/scan-secrets.sh`, `bash scripts/check-security.sh --static-only`, `bash scripts/check-mobile.sh --static-only`, staged/unstaged `git diff --check`: passed. Test credentials, backend port and aliases are synthetic. Physical phone untouched.

Behavior/test/resource source fingerprint (changed app/application/data files relative to d0a3565; sorted path + NUL + bytes + NUL): `3e630c9a4602fc9304adbab201edd63a4a6bbea1e72bdc1eede740352c239cae`.

The earlier cumulative backend/PostGIS evidence is retained for unchanged backend inputs. Emulator API 26 does not certify staging registration, every OEM/navigation/font/rotation variant, iOS or production. Those runtime obligations remain owed.

## Delivery

Reuse ready dev → main PR #124 with a cumulative description and normal dev push. Required Quick verification must pass on the new exact head; use `bash scripts/git-flow.sh finish --base origin/main --pr 124 --required "Quick verification"` at batch closure. Missing/pending/failed checks block integration. Only publish the owned wiki snapshot after verifying the merged SHA, once for the combined batch. Record actual remote outcomes in PR metadata/local notes and the next useful commit; this local checkpoint does not assert a merge, wiki sync, deployment or release.

# P36-T01 — Branded account journey and persistent key login

Authorized scope: Android signup, key login, account backup and session continuity.
Risk: critical authentication/privacy; no backend schema, endpoint or token TTL changes.

## Behavior contract

- B-BR-AK01: signup retains the issued key in Android Keystore-sealed storage and signs in automatically; the backup step stays visible before entering the app. Closing/restarting the app preserves the account.
- B-BR-AK02: access tokens keep their server TTL. Rotate expired access through the existing refresh route. Only an explicit server `session-expired` may trigger one key login, after checking the old family; never bypass revocation, reuse, suspension or deletion. A replacement session must belong to the same account.
- B-BR-AK03: transport failure preserves local credentials and displays an offline account state. Expired tokens never authorize private reads/writes. Logout clears session and key even offline and prevents an in-flight refresh from restoring them. Mutating auth operations are serialized on Android.
- B-BR-AK04: export is user initiated, masked by default and confirmed before copying/sharing. Explain that the recipient gains account access; recommend a password manager/private destination. No automatic messages, secret logs, saved UI state or public upload. Mark clipboard content sensitive, clear owned clipboard content after a bounded interval, hide secrets on background, protect sensitive windows from screenshots and exclude auth preferences from OS backup/transfer.
- B-BR-AK05: use the existing SVG-derived Android vector, lowercase abastevo wordmark, approved blue/green palette, Material typography, localized text, scrollable layouts and full-width accessible controls. No new dependency.

- B-BR-AK06: account deletion renews expired access before submitting proof and clears local credentials only after server success or a definitive auth denial; a transport failure preserves the account and recovery key. Legacy email login cannot overwrite an existing key-account backup.

## Use cases

- BUC-AK01: create username → receive key → sealed backup + automatic login → save/export deliberately → continue; retry login without recreating the account on transient failure.
- BUC-AK02: restart/resume → inspect sealed session → rotate or reauthenticate only after verified expiry → retain offline state on outage; invalid/revoked/suspended/deleted proof clears local auth.
- BUC-AK03: profile → masked backup → reveal/copy/export confirmation → system chooser → hide on background. Paste the raw or grouped key into another installation of the same environment.
- BUC-AK04: explicit logout → clear local key/session/UI secrets → attempt existing revoke-all; no automatic relogin on this device.

## Acceptance evidence

Status: LOCAL_DONE
Validation: PASS
Integration: INTEGRATION_PENDING

Tested on 2026-10-06, maintained `dev`, parent `2891df9`. Behavior-source SHA256
`3e94cb49ff7d697961a8a95aaa315817ae222274834023a5b5a8917f6944063b`:
SHA256 over sorted changed/new `app/`, `application/`, `data/` paths, each path,
NUL, file bytes, NUL. Documentation is excluded to avoid self-referential evidence.

### Immediate critical checks

- RED: session continuity/offline/refusal cases exposed three failures before implementation. HTTP redirect, oversized valid JSON and malformed session cases exposed three failures. Deletion transport failure exposed credential loss; expired-access deletion exposed two additional failures. Each was corrected before acceptance.
- GREEN: `./gradlew :application:test --tests '*PortableAuthFlowTest' :data:testDebugUnitTest --tests '*SerializedAuthOperationsTest' :app:testDebugUnitTest --tests '*AuthViewModelTest' :app:lintDebug :app:assembleDebug --console=plain` passed. Portable flow: 26; serialized rotation/logout: 2; auth view model: 21. Concurrent rotations send one refresh; logout queued behind refresh leaves no resurrected local session/key.
- Final affected security/write suite: `./gradlew :data:testDebugUnitTest --tests '*AccountHttpApiTest' --tests '*KeystoreAccountKeyTest' --tests '*KeystoreSessionStoreTest' --tests '*FeedbackHttpClientTest' --tests '*SerializedAuthOperationsTest' :app:testDebugUnitTest --tests '*SecurityConfigurationTest' --tests '*FeedbackViewModelTest' --tests '*AuthViewModelTest' --console=plain`. HTTP: 13; sealed key: 5; sealed session: 5; authenticated feedback HTTP: 11; lock: 2; security configuration: 4; feedback view model: 16; auth view model: 21. No auth key follows an HTTP redirect; unavailable/revoked renewal sends no private write.
- `cd backend && go test -race ./internal/modules/account/...` passed.
- Disposable `infra/compose.validation.yml` PostGIS fixture: applied all 42 existing migrations using `go run ./cmd/migrate`; `go test -race -count=1 -tags=integration ./internal/modules/account/adapters` passed. Separate unique validation project; no live database touched; fixture and network removed. Initial fixture startup failed because first image health preceded final TCP readiness; fixed the readiness check and absolute cleanup path, then reran successfully.
- Android compile/debug APK and lint passed: 0 lint errors, 185 warnings. Restored missing auth/capture translations in all six additional supported locales instead of suppressing MissingTranslation. XML parsing, unique resource names and format placeholders verified. APK: `app/build/outputs/apk/debug/app-debug.apk` (debug artifact, not a release-size certification).
- `git diff --check`, `bash scripts/check-mobile.sh --static-only`, `bash scripts/scan-secrets.sh`, `bash scripts/check-security.sh --static-only` passed. Manual secret-surface review: synthetic fixture credentials only, no contributor data, precise GPS, photos, keys or token logging added. No dependency additions.

### Limits and recovery

Device credential confirmation, share chooser, SAF file export, clipboard lifecycle,
screenshot protection, TalkBack, large fonts, light/dark visual QA and process restart
on hardware remain OWED under the standing device deferral. JVM/static checks do not
prove those OS behaviors. Staging runtime account/provider evidence remains OWED;
no live account or external message was created. iOS remains archived; G09 stays
UNCERTIFIED. This checkpoint accepts the scoped source behavior, not all P36,
provider/device acceptance, deployment or a public pilot.

An exported file/message contains the bearer account key and is outside the app's
storage boundary; the warning and OS credential confirmation make that transfer
explicit. The app cannot revoke a recipient's saved copy. Existing server expiry,
revocation, reuse detection, suspension and deletion rules remain authoritative.
Transient renewal/deletion failures preserve sealed recovery data without granting
expired authority. Ordinary app restart/expiry does not require manual key entry.

Rollback: revert this atomic source commit through protected `dev` → `main`
delivery; preserve existing backend migrations/endpoints and sealed key format.
Never clear account backups to roll back styling/session scheduling. Next action:
publish the cumulative dev head as one protected PR, require current-head/base
Quick verification, guarded merge, then mirror the merged documentation once.

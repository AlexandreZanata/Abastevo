# Premium area organization

Parent: `b90a0cd1` (maintained dev). Date: 2026-10-24.
Scope: Android presentation/navigation only. No backend, billing, entitlement,
purchase, permission or commercial-rule change.

## Before behavior

The signed-in account screen owned the complete Premium card (status, manage,
restore, notices) and there was no dedicated Premium destination. Profile showed
only the account header and tools. This mixed subscription management with
account/backup actions and made a single, consistent Premium visual language
impossible.

## Delivered

- Routes `premium` and `premium-management` with no navigation arguments.
- `ui/premium/PremiumScreen.kt`: Premium title/back, verified status, paid-through
  date only for states that keep paid access, known-capability labels, retained
  data notice, refresh, manage-subscription and honest restore-unavailable action.
- `ui/premium/PremiumManagementScreen.kt`: subscription state, store guidance,
  fixed Play subscription destination and post-store refresh through a
  resume-scoped refresh flow. No cancellation or renewal is simulated.
- `ui/premium/PremiumEntry.kt`: one gold/crown entry component, full in Profile
  and compact in Manage account. It uses the shared container shape rather than a
  one-off radius.
- Profile: Premium category immediately below the signed-in account card, shown
  only for `AUTHENTICATED` or `OFFLINE_ACCOUNT` sessions.
- Manage account: old Premium section and Premium query removed; one compact
  shortcut remains; account/key/backup/logout/delete actions preserved.
- `PremiumViewModel` moved to `ui/premium`; generation and owner fencing preserved
  and strengthened so a refresh for a new owner clears the previous status before
  the response. Both pages refresh once on initial resume and again on each
  return to resumed, including after the store.
- `ui/premium/PremiumStatusCache.kt`: 60-second, account-scoped display cache.
  Re-entering within the TTL reuses the last verified status without a private
  call; explicit refresh bypasses it; logout, errors, account switches and store
  handoff clear it. It never creates or extends an entitlement.
- Premium and Manage-subscription loading states now render a structural
  skeleton preview: section titles, crown, default "access until" placeholder,
  retained-data text, capability placeholder rows and the real action labels.
  Only the verified status/capability values are replaced when the server
  responds.
- Profile loading renders a full-page skeleton and the signed-in account,
  Premium, reference and tools sections no longer mount with crossfade/expand
  animations. The login step crossfade was not changed.
- Fixed light/dark Premium palette in `ui/theme/PremiumColors.kt`, selected by the
  app theme via `LocalAnpDarkTheme`; the active color scheme still drives general surfaces.
- Local crown vector, shared `ProfileActionRow`, 25 new strings in eight locales.
- Follow-up container-shape pass: `AnpContainerDefaults.containerShape()` (the
  active Material medium shape) now covers the Premium entry, the private-reports
  button, Manage-account guest/status cards and the account-key card. The nested
  key panel uses `MaterialTheme.shapes.small`. This matches the more rectangular
  Perfil tools and signed-in cards instead of 18/22/24dp one-offs.
- Navigation bottom bar keeps three tabs. Premium and premium-management keep
  Profile selected; tapping Profile still opens the base page with
  `navigateToTabBase` and no child restore.

## State and navigation coverage

- JVM PremiumViewModel/cache tests: signed-out no private call, active render,
  offline/denied without stale grant, account switch during request, new-owner
  refresh clears old status, logout clears verified status, restore unavailable,
  store link failure recoverable, TTL hit avoids a second private call, explicit
  refresh bypasses the cache, logout clears cached data.
- Presentation tests: distinct labels for all ten states (including HOLD, PAUSED,
  NONE, UNKNOWN), access date only for ACTIVE/CANCELED/GRACE, known capabilities
  only, deterministic order, store-failure message.
- Navigation tests: premium routes carry no account/purchase arguments; Premium
  pages keep Profile selected.
- Color tests: light/dark text >= 4.5:1; accent and control boundary >= 3:1.

## Actual checks

- `./gradlew :app:testDebugUnitTest :app:lintDebug :app:assembleDebug --console=plain`
  PASS: 490 tests, 0 failures/errors/skips; lint 0 errors, 257 warnings (none in
  the new Premium files); debug APK produced (28,272,706 bytes).
- Focused Premium/navigation/color tests PASS before the union.
- `git diff --check` clean; secret-surface review found no credentials/PII added.
- Debug APK SHA-256 `f2869b784b0fe65ee1e1032b46873300c2384bb73bd493fd2eec174768b1596b`.
- Device update: `adb install -r app/build/outputs/apk/debug/app-debug.apk` succeeded
  on 2311DRK48G; package `com.anpfuel.app.debug`, version `3.1.0-debug`,
  lastUpdateTime `2026-10-24 08:46:13`. firstInstallTime remained
  `2026-10-05 16:00:02` (no data clear). No settings change or UI automation.

## Limits and manual review

- Play Billing acquisition/token retrieval is still absent. Restore remains
  explicitly unavailable; no purchase or local entitlement is created. The
  management page opens only the frozen Play subscription URL and does not
  confirm cancellation, renewal or restoration.
- No provider sandbox or live store proof was run. No release or production
  certification is claimed.
- Rendered screenshots were not captured in this environment. Previews exist for
  Premium, management and both entry variants; rendered light/dark/dynamic,
  enlarged-font, narrow-screen and TalkBack checks remain for the owner.
- Manual script: Profile -> Premium -> Manage subscription -> back -> Premium ->
  back -> Profile; Manage account -> compact Premium -> back -> Manage account;
  switch light/dark and app theme; test offline/error; switch account and confirm
  no previous status is shown. Cold-open Premium to see the field skeleton, then
  leave and re-enter within 60s to confirm the verified preview is reused. On
  Profile, confirm the full loading skeleton and no mount animation; confirm the
  login step crossfade still works.

## Dependency review: Google Play Billing Library

AGENTS rule 11 review for the store billing contract
(`docs/product/payments/store-billing.md`, B-BR-STORE-01).

- **Need:** Android sells Premium only through Google Play Billing. The Play
  Billing Library is the supported client for Play subscriptions; there is no
  web checkout in the app.
- **Artifact:** `com.android.billingclient:billing:9.1.0`, the latest stable
  release on Google Maven when added, pinned in `gradle/libs.versions.toml`
  and used only by `:data` (`PlayBillingGateway`). No dynamic versions.
  `billing-ktx` is not used: since 8.1.0 it depends on `kotlin-stdlib` 2.2.10,
  which the project's Kotlin 2.0.21 compiler cannot read. The gateway wraps
  the three callbacks it needs with `suspendCancellableCoroutine` instead.
- **Transitives (POM):** `androidx.activity` 1.2.3, `androidx.core` 1.15.0,
  `play-services-base` 18.5.0, `-basement` 18.9.0 and `-tasks` 18.2.0 are
  floors the app already meets. New: `play-services-location` 19.0.0 (its
  manifest declares no permission) and `com.google.android.datatransport`
  `transport-api` 3.0.0, `transport-backend-cct` 3.1.8 and
  `transport-runtime` 3.1.8, which raise the 2.x versions ML Kit pulled.
  Check OCR on a device after the first build with them.
- **License:** the POM declares the Android Software Development Kit License
  (`https://developer.android.com/studio/terms.html`), proprietary Google
  terms rather than an open-source license. The AAR carries
  `third_party_licenses.txt` for its own dependencies. Listed in
  `THIRD_PARTY_NOTICES.md`.
- **Security and privacy:** published by Google on Google Maven and fetched
  over HTTPS. The merged manifest gains `com.android.vending.BILLING`,
  `<queries>` for the Play billing service and its test companion, and two
  non-exported proxy activities. `INTERNET` and `ACCESS_NETWORK_STATE` were
  already declared; no location permission is added. The library talks to
  the Play Store app over IPC; Play may log library usage through the
  datatransport stack, as ML Kit already does. The app gives Play only the
  server-derived account binding (`obfuscatedAccountId`, 64 hexadecimal
  characters; no account ID or e-mail). Purchase tokens go only to the
  signed backend route and are never logged, stored or printed. The app
  never acknowledges or consumes a purchase. Library minSdk 23 is below the
  app's 26; consumer R8 rules ship in the AAR.
- **Build:** the artifacts are not in the offline Gradle cache; the first
  build after this change downloads them.

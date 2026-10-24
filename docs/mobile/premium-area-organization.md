# Premium area organization — Android

Status: LOCAL_DONE
Validation: PASS
Visual validation: VISUAL_VALIDATION_PENDING
Integration: INTEGRATION_PENDING

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
  and compact in Manage account.
- Profile: Premium category immediately below the signed-in account card, shown
  only for `AUTHENTICATED` or `OFFLINE_ACCOUNT` sessions.
- Manage account: old Premium section and Premium query removed; one compact
  shortcut remains; account/key/backup/logout/delete actions preserved.
- `PremiumViewModel` moved to `ui/premium`; generation and owner fencing preserved
  and strengthened so a refresh for a new owner clears the previous status before
  the response. Both pages refresh once on initial resume and again on each
  return to resumed, including after the store.
- Fixed light/dark Premium palette in `ui/theme/PremiumColors.kt`, selected by the
  app theme via `LocalAnpDarkTheme`; dynamic colors still drive general surfaces.
- Local crown vector, shared `ProfileActionRow`, 25 new strings in eight locales.
- Navigation bottom bar keeps three tabs. Premium and premium-management keep
  Profile selected; tapping Profile still opens the base page with
  `navigateToTabBase` and no child restore.

## State and navigation coverage

- JVM PremiumViewModel tests: signed-out no private call, active render, offline/
  denied without stale grant, account switch during request, new-owner refresh
  clears old status, logout clears verified status, restore unavailable, store
  link failure recoverable.
- Presentation tests: distinct labels for all ten states (including HOLD, PAUSED,
  NONE, UNKNOWN), access date only for ACTIVE/CANCELED/GRACE, known capabilities
  only, deterministic order, store-failure message.
- Navigation tests: premium routes carry no account/purchase arguments; Premium
  pages keep Profile selected.
- Color tests: light/dark text >= 4.5:1; accent and control boundary >= 3:1.

## Actual checks

- `./gradlew :app:testDebugUnitTest :app:lintDebug :app:assembleDebug --console=plain`
  PASS: 483 tests, 0 failures/errors/skips; lint 0 errors, 257 warnings (none in
  the new Premium files); debug APK produced (28,272,706 bytes).
- Focused Premium/navigation/color tests PASS before the union.
- `git diff --check` clean; secret-surface review found no credentials/PII added.
- Debug APK SHA-256 `48f1558ef6f4c3d269ece1ffe2ebf47b1544bc4f9de9489b10a464f03e2e31a8`.
- Device update: `adb install -r app/build/outputs/apk/debug/app-debug.apk` succeeded
  on 2311DRK48G; package `com.anpfuel.app.debug`, version `3.1.0-debug`,
  lastUpdateTime `2026-10-24 08:22:04`. firstInstallTime remained
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
  no previous status is shown.

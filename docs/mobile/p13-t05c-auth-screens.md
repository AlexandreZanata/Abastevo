# P13-T05C — Auth screens + navigation + deep link

Status: LOCAL_DONE on `codex/phase-13-free-accounts` (phase PR #27,
draft). Issue: #26 (slice 3 of P13-T05). Device evidence stays
deferred to release per owner decision. Free browsing stays
logged-out: no start destination changed.

## Slice acceptance (frozen before coding)

- T05C (this commit) — login UI: `AuthViewModel` (Hilt shell over
  `AuthFlow`: rehydrate/refresh on start, email request/consume,
  provider attempt/complete, logout, locale-free error mapping,
  back-navigation event), `AuthScreen` (Material3 email/code/
  provider/logout, tokens never render, previews), `Routes.AUTH`
  + graph entry with `anpfuel://auth/callback` deep link
  (provider/id_token/nonce/state args), Settings "Conta" row,
  manifest intent-filter. Native provider SDK minting real id
  tokens is device-gated (release horizon).
- Later — Keychain adapter + macOS run (release horizon).

## What changed

- `app/.../ui/auth/AuthViewModel.kt` — step machine
  (Checking/EmailEntry/CodeSent/Busy/ProviderPending/
  Authenticated), attempt custody for callbacks, verdict→UI
  mapping (suspended/deleted/offline/provider-denied distinct;
  oidc-/link- prefixes share one denial).
- `app/.../ui/auth/AuthScreen.kt` — email/code fields (digit
  filter, 6-take), provider buttons (authenticated only, matching
  the backend link-only contract), pending/cancel, logout,
  error text + dismiss, back affordance, two previews.
- `navigation/Routes.kt` (`AUTH`) + `AnpNavGraph.kt` (route with
  defaulted deep-link args + `navDeepLink` pattern; `AuthRoute`
  forwards non-empty callbacks to the VM).
- `SettingsScreen.kt` (+1 defaulted callback, +1 row, +1 string
  pair) wired to `Routes.AUTH` in the graph.
- `AndroidManifest.xml` — VIEW/DEFAULT/BROWSABLE intent-filter
  for `anpfuel://auth/*callback*` on MainActivity.
- `strings.xml` + `values-pt-rBR` — 22 auth keys each (EN+PT).
- Tests: `AuthViewModelTest` (11, Turbine+mockk: init paths,
  login+back-nav, wrong/suspended codes, provider
  pending/complete/ignored-callback, logout, verdict map),
  `RoutesTest` auth const, `AuthDeepLinkManifestTest`
  (scheme/host/path/browsable as manifest text).

## RED → GREEN

- RED proven by dropping the post-login `NavigateBack` emit:
  `emailLoginNavigatesBack` fails (state flips, event missing);
  GREEN on restore.

## Validation (exact commands, this host)

```sh
./gradlew :domain:test :application:test :data:testDebugUnitTest :app:testDebugUnitTest :app:assembleDebug --no-daemon
make quick-verify
git diff --check
```

Outcome: app 74 tests (13 new auth/nav), 0 failures; full
baseline + `assembleDebug` SUCCESS; `quick-verify`
selection=full ok; `diff --check` clean; secrets scan clean.
Compose rendering and deep-link routing are device-gated
(verified by build + lint here, not pixels).

## Limits (not claimed)

- No native provider SDK (no id-token minting on device yet);
  pending attempt + deep-link parse path is real and tested.
- No Keychain/macOS run (release horizon, per owner decision).
- Preview base URL still `.invalid` (T05B).

## Rollback

Additive `ui/auth` + route + row + filter + strings: reverting
those hunks restores the tree. No migration, no manifest
permission change (no new `<uses-permission>`).

## Next

P13 phase closure review (all T01–T05 slices), then `finish`
→ P13 INTEGRATED. Issue #26 stays open until the phase PR
merges.

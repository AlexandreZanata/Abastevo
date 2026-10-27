# C1 — App: public contributors list

Date: 2026-10-27. Branch `codex/adm-c1-app-contributors` from `origin/main`
`d4dcf716`. PR targets `main`. Front C starts after the A3 PR exists (#167).

## Delivered

- `domain/contributor/`: `Contributor`/`ContributorLink`/`ContributorPage`,
  `ContributorException` (Invalid/Denied/Unavailable/Offline),
  `ContributorLinkHosts` (CTB-08 host table + `isAllowedUrl`).
- `domain/repository/ContributorsGateway` + `data` OkHttp client
  (`GET /v1/contributors?cursor=`, anonymous, 20 s timeout, 256 KiB budget,
  strict decode), `ContributorsMapper` (unknown platforms, non-https urls
  and wrong hosts dropped; blank names dropped; malformed `since` degrades
  to blank), Hilt `ContributorsModule`, `GetContributorsUseCase`.
- App: `ContributorsViewModel` (loading/items/empty/error/offline states,
  manual cursor paging with generation fencing), `ContributorsScreen` +
  pure `ContributorsContent`, `ProfileContributorsEntry` (Premium-accent
  card, last Profile item), `Routes.CONTRIBUTORS` + nav wiring.
- `ProfileScreen.kt`: one `onOpenContributors` param, one privacy-item
  testTag, one appended card; no reformatting (open PR #152 also touches
  this file).
- `strings_contributors.xml` in all 8 locales (values, -pt-rBR, -es, -fr,
  -de, -ja, -ru, -zh-rCN).
- Links open only the server `url` after re-checking scheme `https` and the
  CTB-08 host allowlist, via `ACTION_VIEW`; no WebView.

## Tests

- `ContributorLinkHostsTest` (4): all 7 server urls pass; non-https, wrong
  hosts and unknown platforms dropped.
- `ContributorsMapperTest` (8): valid page, three link-drop cases,
  blank-name drop, since degradation, missing-items invalid, null login.
- `ContributorsHttpClientTest` (5): no session in wire body, cursor query,
  denied explicit, 5xx unavailable, unreachable host offline (127.0.0.1:9,
  loopback only).
- `GetContributorsUseCaseTest` (3): two-page passthrough, oversized cursor
  refused without a call, gateway failure propagates.
- `ContributorsViewModelTest` (6): items, empty, offline+retry, denied vs
  error, cursor append/stop, late-response fencing.
- `ProfileContributorsCardTest` (androidTest, real `ProfileScreen` with a
  hand-built signed-out `AuthViewModel`, zero new deps): entry displayed
  last after the privacy item (bounds order), click opens contributors.
- `ContributorsScreenTest` (androidTest, pure content): row content
  (name/badge/roles/link → captured url), empty state, error retry.

## Commands and results

```sh
./gradlew :domain:test :data:testDebugUnitTest :application:test :app:testDebugUnitTest --offline
# domain 781, data 629 (4 pre-existing skips), application 484, app 561 — 0 failures/errors
./gradlew :app:lintDebug :app:assembleDebug :app:assembleDebugAndroidTest --offline
# BUILD SUCCESSFUL (lint clean, androidTest compiles)
```

Instrumented tests could not execute here: the attached device refuses
installs (`INSTALL_FAILED_USER_RESTRICTED`, needs a physical tap), and no
emulator was started. They compile and run in CI.

## Not done

- C2 (landing `/contribuidores/`), C3 (claim/profile self-service), C4
  (staging check). No staging touched.

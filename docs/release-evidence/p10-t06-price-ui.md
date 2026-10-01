# P10-T06 — Official and community price UI

Status: LOCAL_DONE on `codex/phase-10-functional-integration` (phase PR
#54 pending, sixth task push). Issue: #50 (slice 6 of 8 local;
P10-T09 pilot stays deferred until G18/G09 and is not part of G10-LOCAL).
No ANP path touched, no legacy enum rename, no backend change, no
existing screen modified.

## Slice acceptance (frozen before coding)

- T06 (this commit) — Display source, condition, uncertainty and age
  separately: app `CommunityPriceDisplay` (pure `AVAILABLE/DISPUTED/UNKNOWN`
  + `FRESH/AGING/STALE/UNKNOWN`, exact milli-BRL 3-decimal, `formatCondition`
  with APP/LOYALTY qualifier rules, confidence note as support-never-
  guarantee) + `CommunityPricesViewModel` (Disabled without IO / Loading /
  Content / Stale-explicit / Unavailable, flag OFF pauses) +
  `CommunityPricePanel` (official card separate from community beta card,
  source+condition+unit+age on every row, community UNKNOWN until P04,
  stale labelled STALE, flag OFF renders nothing) + `community_*`
  strings en + pt-BR. ANP Prices/Stations/Home byte-identical; rollback
  is flag OFF.

## What changed

- `app/.../community/CommunityPriceDisplay.kt` — pure presentation,
  B-BR-001/002/008 (no `android.*` in logic).
- `app/.../community/CommunityPricesViewModel.kt` — flag-gated fetch
  over `GetCommunityPriceGroupsUseCase` (T02, unchanged).
- `app/.../community/CommunityPricePanel.kt` — distinct sections,
  accessible labels, units always.
- `app/.../res/values/strings.xml` + `values-pt-rBR/strings.xml` —
  9 new `community_*` keys each, no existing key touched.
- Tests: 7 display (UNKNOWN/STALE/DISPUTED/differing-sources/APP
  qualifier/milli-exact/confidence-note) + 4 viewmodel
  (disabled-no-IO/fresh/stale/unavailable) + 1 androidTest device
  replay (CI).

## RED → GREEN

- RED proven by new symbols before implementation (same pattern as
  P10-T01…T05): `CommunityPriceDisplay`, `CommunityPricesUiState`,
  `CommunityPricesViewModel`, `CommunityPricePanel` did not exist; new
  tests referenced them. GREEN after adding bounded panel only.

## Validation (exact commands, this host)

```sh
./gradlew :app:testDebugUnitTest --no-daemon --rerun-tasks
./gradlew :app:lintDebug --no-daemon
./gradlew :app:assembleDebug --no-daemon
bash scripts/check-mobile.sh
bash scripts/check-mobile.sh --static-only
git diff --check
bash scripts/scan-secrets.sh
```

Outcome: `:app:testDebugUnitTest` green (incl. 11 new suites listed
above); `assembleDebug` BUILD SUCCESSFUL (ANP screens unchanged);
`check-mobile` full ok (Swift SKIP recorded); `diff --check`/secrets
clean.
Limits: `:app:lintDebug` FAILS with 32 errors — 23 pre-existing on
main (incl. `MissingPermission` in `LocationPermissionHandler` plus
older `MissingTranslation` in es/fr/de/ja/zh/ru) + 9 new
`MissingTranslation` for the `community_*` keys in de/es/fr/ja/zh/ru
(en + pt-BR complete); zero Kotlin/Compose errors in the new files.
Lint is not part of required Quick verification
(`scripts/quick-verify.sh` gates backend/contracts only); translation
backfill for all locales belongs to the P10-T08 device/i18n pass, never
claimed here. `:app:connectedDebugAndroidTest` +
`:data:connectedDebugAndroidTest` NOT run locally (no emulator);
`CommunityPriceDisplayDeviceTest` added for CI/phase exit. No backend
change; no production URL invented. B-BR-001/002/008: sources never
blended, milli exact, STALE/UNKNOWN out of cheapest ranking.

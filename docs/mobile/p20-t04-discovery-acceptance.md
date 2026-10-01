# P20-T04 — Discovery acceptance and legacy regression

Status: LOCAL_DONE on `codex/phase-20-community-discovery`. Issue: #79. Phase P20 exit candidate: G20 scope (real community current-price browsing and station detail against local backend, with source/freshness/condition/accessibility/offline truth) is demonstrated at contract + journey level; backend community projection stays UNKNOWN until P04 (no fake coverage).

## Regression evidence (rerun, current head `9a40d32`)

- `./gradlew :domain:test :application:test :app:testDebugUnitTest :app:assembleDebug --no-daemon --rerun-tasks` → BUILD SUCCESSFUL.
- `:domain:test` 397/0-fail · `:application:test` 238/0-fail · `:app:testDebugUnitTest` 128/0-fail (763 total, 0 failures/errors/skips).
- Targeted acceptance classes, all green: `DiscoveryQueryTest` 6, `DiscoveryStationsRuleTest` 7, `StationDetailRuleTest` 6, `StationsViewModelTest` 9 (search/cache/detail), `CommunityPriceDisplayTest` 7 (conditional prices: STANDARD needs no qualifier, APP/LOYALTY without qualifier stay pending, OTHER excluded from ranking), `CommunityPricesViewModelTest` 4, `HomeViewModelTest` 4.
- Accessibility: `ColorContrastTest` 2, `FuelProductTintContrastTest` 2, `SourceTimeBadgeTest` 3 (text + icon + screen-reader labels per source kind).
- Legacy/expert tools preserved: `RoutesTest` 6 + `NavigationTabTest` 6 green; nav graph still wires onboarding, home, community, profile, search, location, prices, history, stations (+fuel arg), vehicles, week picker, capture, auth deep link, settings — none removed by P20.
- `git diff --check` clean; scoped secret review (no secrets/PII/GPS/photo content).

## Source/recency/price identification (contract level)

- Primary price is never an ANP substitution (`BackendPriceGroup.create` rejects non-null community; P20 detail shows explicit no-coverage + dated ANP reference with stale/unknown states).
- Guest explores without account/GPS; offline keeps cached lists with explicit banners; failed refresh preserves cache with retry.

## Honest limits (not waived)

- No novice comprehension campaign ran here: P19-T02 recorded it pending and P24-T02 owns the usability/accessibility study with real participants.
- No device matrix, cold-start, heap/battery profiling: owned by P24-T03 on frozen devices.
- No map: deferred until provider/dependency/privacy audit (frozen IA).
- Community projection content (P04), photo journey (P21), social/moderation (P22): out of scope.
- Data module and backend suites untouched by P20 (no changes there); full production certification stays G09-deferred.

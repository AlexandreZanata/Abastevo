# Skeleton loading plan — staged rollout

Status: PLANNED. No skeleton/shimmer exists today; loading is either a fullscreen
`LoadingState` spinner or a bare `CircularProgressIndicator`. This plan adds
layout-faithful skeleton placeholders to every page with an async state, in
stages. No behavior change, no new strings, no new dependencies.

## Foundation (E0)

- New `app/src/main/kotlin/com/anpfuel/app/ui/components/Skeleton.kt`:
  `Modifier.skeleton()` shimmer built on `rememberInfiniteTransition` plus a
  linear-gradient brush over theme `surfaceContainerHighest` colors (light/dark
  automatic). No third-party shimmer artifact: dependency additions require a
  recorded need/license/security review, and the built-in animation suffices.
- Primitives: `SkeletonLine`, `SkeletonCard`, `SkeletonButton`, mirroring the
  app's 20–24dp card language and real paddings so content arrival causes no
  layout jump.
- Accessibility: the skeleton container carries the existing `a11y_loading`
  description; the shimmer itself is decorative. Reuse `state_loading` text
  where a label is needed.
- Light/dark `@Preview` for every primitive.

Exit: `compileDebugKotlin` passes, previews render, `lintDebug` zero errors.

## Stage E1 — high-traffic list pages

Home (`home`), Community (`community`), Prices (`prices`), History (`history`),
Stations (`stations`), Search (`search`).

- Home: hero banner block + two reference-price rows + actions row.
- Community: three feed-card placeholders; keep the small `loadingMore` spinner.
- Prices: week header + four price rows. History: selector + four series rows.
- Stations: four station cards (replaces empty-list spinner only; inline
  download progress stays truthful). Search: search field + four result rows.

Exit per page: affected unit tests pass, `lintDebug` zero errors,
`assembleDebug` installs on POCO with manual navigation showing the skeleton
before content.

## Stage E2 — detail and flow pages

Station page (`station/{key}`), Station profile (`station-profile/{id}`),
Station claim (`station-claim/{id}`), Station management
(`station-management/{id}`), Location (`location`), Week picker (`week_picker`),
Onboarding catalog (`onboarding`), Settings (`settings`).

- Detail headers plus their row/button blocks; keep disabled-button semantics
  on claim/management while busy. Location keeps its overlay pattern but with
  skeleton rows underneath.

Same exit as E1.

## Stage E3 — account, capture and embedded panels

Account (`account`), Auth (`auth`), Profile status (`profile`),
Vehicles (`vehicles`), embedded `CommunityPricePanel`/`CommunityVotePanel`,
Capture/OCR analyzing states (`capture`). Help (`help`) and Suggest (`suggest`)
only if an async state is found during implementation.

- Auth keeps its button-spinner honesty (no fake form submit); skeleton covers
  only initial form load. Capture skeleton covers OCR analysis only, never the
  camera preview.

Same exit as E1.

## Delivery rules

One atomic commit per stage on maintained `dev`; per-stage compile, affected
tests, lint, `git diff --check`, secret-surface review and POCO install. No
backend changes, no PR/merge per stage — integration follows the batch
delivery workflow at construction end.

# MEX-31 — Master discovery Android price/record journeys

Status: LOCAL_DONE
Validation: PASS
Integration: INTEGRATION_PENDING

Parent: `d1e1b4d6` (maintained dev, EE-23 commit; PR #147 head stays
open per batch rule, no merge yet). Date: 2026-10-26. Depends MEX-26/
27/29/30, EE-25. Rules B-BR-MEX-08–18; BUC-MEX-06–09. Risk: Android
query meaning/performance.

Before behavior: owners ask in Portuguese (typed questions or frozen
suggestions), see the typed interpretation with visible source/
conditions, execute price rankings server-filtered with stable order,
or open official records with context preserved on return. Ambiguous
inputs become labelled choices; adversarial or municipal-unknown
inputs refuse before any wire call. No whole-national download, no
per-keystroke refresh, no phone-only subsets.

## Delivered

- `domain/exploration/MasterQuestions.kt`: on-device port of the
  Go interpreter (same vocabularies, same precedence, same unknown
  reasons; deterministic longest-match UF instead of map order).
  Mirrored golden vectors pass on both sides.
- `PriceRankingHttpClient` (MEX-29, anonymous, typed, no-store,
  cancellable, 1 MB budget, repeat-cursor/corrupt/foreign refusal)
  plus `MunicipalityIbgeLookup` (exact IBGE or refuse, ambiguous
  refused) and `GetPriceRankingUseCase` wiring interpretation to
  bounded server pages.
- `MasterExploreScreen/ViewModel` + `master-explore` route with an
  Explore entry point: frozen PT-BR suggestions, typed question box,
  visible interpretation, NEEDS_CHOICE chips that adopt concrete
  queries, ranking results with revision badges, load-more
  continuations, offline/error/empty states, occurrence deep-link.
  11 new strings in all 8 locales.

## Actual checks

From this worktree (2026-10-26, parent `d1e1b4d6`):

- RED: client/use-case/VM/domain tests failed to compile or diverged
  before each unit (unknown `station_id` key, eager vs lazy
  cancellation, duplicated DI bindings, mockk `eq` absence,
  ambiguous-map resolution and error-state retention fixed without
  weakening tests).
- Domain parity vectors (liquid/EV/unknown/adversarial),
  MockWebServer client tests (typed order, revision binding,
  no-store request, corrupt/oversize/private/unknown-entity refusal,
  cancellation, pre-wire municipal validation), use-case tests
  (interpret-and-execute, unknown-IBGE refusal without wire,
  exact-match execution); VM tests (frozen suggestions, ambiguous
  refinement, server-filtered execution, official routing, failure
  context preservation).
- Full `:domain:test :data:testDebugUnitTest :application:test
  :app:testDebugUnitTest --offline` → 2029 tests, 0 failures;
  `:app:lintDebug :data:lintDebug` green (8 locales);
  Go exploration + apicontract green (EE-25 batch);
  `git diff --check` + secret scan clean; no OpenAPI/SQL edits.
  Device reopen stays deferred per standing directive. No PII/GPS in
  Git/logs.

## Limits

- Scenario/energy intents show their interpretation without a
  dedicated execution surface (no invented comparison); EV place
  intents route to charging discovery without a vehicle context.
- No device, no production, no deploy and no release acceptance is
  claimed.

Next: MEX-14/15 (sync transport/backup) once authorized; they carry
queued ledger commands with these same guards.

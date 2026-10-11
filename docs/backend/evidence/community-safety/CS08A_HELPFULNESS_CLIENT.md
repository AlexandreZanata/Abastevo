# CS08A — Helpfulness client layers

Date: 2026-10-11. Status: LOCAL_DONE. Scope: domain ports, use
case, flag store, HTTP client + unit tests — no DI, no ViewModel,
no panel (CS08B), no VPS/APK. Head: maintained `dev`.
Plan: [phased plan](../../../planning/COMMUNITY_SAFETY_EXECUTION_PLAN.md).
Rules: B-BR-CS01, BUC-CS02.

## Added (mirrors the vote stack, frozen wire from CS02B)

- `CommunityHelpfulnessPorts.kt`: `HELPFUL`/`NOT_HELPFUL` frozen
  set, reject kinds (SELF_VOTE, STALE_REVISION, INELIGIBLE_TARGET,
  QUOTA_EXCEEDED, INVALID_VOTE, CONFLICT, TRANSPORT), exact
  receipt/tally shapes (basis points and choice nullable),
  `CommunityHelpfulnessGateway`.
- `SubmitHelpfulnessVoteUseCase`: flag gate (Disabled, no IO),
  blank/choice/sequence validation throwing `DomainException`,
  cast/change/remove/tally with fixed-kind sanitized rejections.
- `CommunityHelpfulnessFlagStore`: default OFF SharedPreferences.
- `CommunityHelpfulnessHttpClient`: cast/remove/tally routes with
  stable submission ids, Idempotency-Key, exact parsing (percentage
  and choice absent on zero/anonymous), backend-code → kind mapping,
  fixed error text only.

## Validation (actual)

- Application: 6/6 green (disabled silence, exact tally, blank/
  choice/sequence refusal, self/stale kinds, transport honesty,
  remove/tally).
- Data (MockWebServer): 4/4 green (tally parse + overlay, empty
  tally omits percentage, 7 backend codes, remove posts sequence
  only). Full `:application:test` green.
- `git diff --check` clean; CI Android runs `./gradlew test`
  (no lint gate configured).

## Limits / owed (CS08B)

- DI bindings, ViewModel + card/panel, guest-login explanation,
  Denunciar reuse of the dispute flow, TalkBack/large-text checks,
  device matrix (CS14-owned).

Next microtask: CS08B — ViewModel, panel, DI, guest.

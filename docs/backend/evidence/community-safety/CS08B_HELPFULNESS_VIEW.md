# CS08B — Helpfulness ViewModel, panel, DI, guest

Date: 2026-10-11. Status: LOCAL_DONE. Scope: ViewModel, panel,
strings, DI bindings, guest — no feed wiring, no VPS/APK. Head:
maintained `dev`.
Plan: [phased plan](../../../planning/COMMUNITY_SAFETY_EXECUTION_PLAN.md),
[CS08A client](CS08A_HELPFULNESS_CLIENT.md). Rules: B-BR-CS01.

## Added (mirrors the vote stack)

- `CommunityHelpfulnessViewModel`: Disabled/Guest/Idle/Loading/
  Content/Submitting/Rejected. Flag OFF and blank account perform
  no network IO; stable submission id minted at prepare; single
  flight (second tap ignored); counts + choice + review notice from
  exact tallies; fixed-kind sanitized rejections; retry reloads.
- `CommunityHelpfulnessPanel`: counts, selected choice, review
  notice, vote/remove buttons, `Denunciar` delegating to the host
  dispute flow (never reports itself), guest free-login card.
  TalkBack via a pure description helper (counts + choice + notice);
  MaterialTheme type scales for large text, no fixed heights.
- 15 English strings; `under_review` added to the tally DTO with a
  default (backwards compatible) and parsed from `under_review`.
- DI: client provider, flag/gateway binds, use-case provider.

## Validation (actual)

- ViewModel 6/6 (disabled/guest silence, tally content, stable id,
  single flight, explicit rejection) with `runTest(dispatcher)`.
- A11y 4/4 (counts/choice/notice composition, blank ignored).
- Hilt bindings proven by the compiling test run (MissingBinding
  seen pre-DI, green after); full debug compile successful.
- `git diff --check` clean.

## Limits / owed

- Card placement in feed/detail screens and Denunciar host wiring:
  screen work, not started (no per-card network calls added).
- Portuguese product copy: values/ is English-only; translation
  pass later. Device matrix: CS14-owned.

Next microtask: CS09 — consent/inventory/rights (server).

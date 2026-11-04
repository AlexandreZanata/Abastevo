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

## Contribution ownership and reporting

B-BR-CS01-OWNER: The signed helpfulness tally may tell the caller that the
observation is theirs. Compare the same stored attribution reference used by
self-vote protection. Never include attribution identifiers or the ownership
indicator in anonymous or other callers' responses.

BUC-CS02-OWNER: In the community prices list, the author sees the passive
ownership label in place of evaluation and confirmation. Another signed-in
viewer keeps the evaluation actions and a full-width error-colored report
button below the two vote buttons. Guest sign-in and report reasons are
preserved.

### Verification — 2026-11-04

The helpfulness GET returns `own_contribution: true` to the verified author
and omits it for other viewers. The signed PostGIS HTTP test verifies the
owner and another account before any vote, plus anonymous reads and refused
self-votes. The public feed remains cacheable without identity data. Android
also maps the existing self-vote refusal to the ownership label during a
rolling backend update.

The shared Android panel shows the localized passive label for the author.
The report action uses the theme error color and spans both vote buttons.
Two dark-theme emulator tests verified the author-only label, report bounds,
report callback and removal action for a viewer with a vote.

Debug and instrumentation APK builds, lint, 53 focused JVM tests, Go race
and vet, PostGIS ownership/proof/concurrency tests, mobile static checks and
OpenAPI lint passed. This commit carries the tested source from `dc332b80`
for integration through protected PR #188.

### Ownership during backend rollout

B-BR-CS01-OWNER compatibility: a signed tally from an older backend may
omit ownership. Before offering actions, resolve that case through the
existing signed observation status endpoint. Only a matching observation
returned by that owner-only endpoint proves ownership. An explicit
not-found response leaves the normal actions available. Other failures
leave the actions unresolved and allow a retry. New backends return a
boolean for verified viewers and omit the field for anonymous reads.

The debug APK was updated over ADB and verified on the connected device
against the staging backend. The author's card changed from the voting
panel to the ownership label without a vote. Android build, lint and 23
focused JVM tests passed. Go race and vet checks and PostGIS tests covered
signed owner and other-viewer reads, anonymous reads, invalid proofs and
the private observation status endpoint.

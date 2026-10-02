# Community moderation policy — user-facing rules

State: **ADOPTED target**, 2026-10-01. Owner phase P22-T03 (issue #88).
This is the user-facing policy; the operator procedure stays in
[feedback-moderation](../operator/feedback-moderation.md) and
[access](../operator/access.md). Existing backend/account/social modules
are locally integrated; the new commercial UX is not. No future phase is
implemented by this document.

## Reporting policy (B-BR-F07, BUC-C03/C05)

- Any active free account may report any comment or reply: one report
  reason, 1–200 characters, plain text. Reporting is rate-limited;
  when the quota is exhausted the app says so with a retry delay
  (429 + `Retry-After`), never with silence.
- Duplicate reports converge on the single open case for that target
  instead of flooding the queue; the comment flags once.
- **Reports never delete content and never grant moderation power.**
  Filing against another author's comment changes nothing but the
  review queue. Removal happens only through a reviewed operator
  decision below — never automatically, never by vote count.
- Cases store identifiers and the reason only: never reporter
  identity, media, GPS or URLs. Deleting your account cannot cascade
  into the queue, because there is nothing to cascade.

## What moderators can and cannot do

- Authority: restricted operators only, through the audited `ops`
  CLI. There is no public admin route and no in-app moderator role.
  Every decision appends actor/action/reason audit rows; raw facts
  are never edited.
- Visibility moves: `visible` (default) → `flagged` (under review,
  still reads normally) → `hidden` (vanishes from public reads, kept
  for audit). Author deletion always wins over later moves.
- A suspended or deleted account cannot write; past ratings and
  comments stay counted and readable until moderated or erased
  through the explicit erasure command.
- Payment never buys moderation immunity, ranking, trust or
  placement (B-BR-C05).

## Appeal and support route

- A disputed decision is appealed by a **new** reviewed case or fact;
  closed cases never reopen and history is never edited. In the app
  this means: re-report with the new reason, or contact support with
  the comment reference. The support channel itself (in-app help /
  pilot contact) is staffed at the bounded-city pilot (P23); until
  then the Comunidade tab states the re-report route honestly and
  invents no unavailable contact.
- Operator mistakes are corrected the same way: a new audited
  `DISMISS`/`RESOLVE` action, never a silent edit.

## Staffing for the pilot (no invented headcount)

- No city opens without named moderation coverage and a response
  target recorded in its pilot plan (P23). Coverage, backlog and
  response time are measured aggregates (P23-T03), not promises in
  this document.
- Closed cases purge after 12 months per SECURITY_PRIVACY; audit
  rows go first.

## Block/mute contract (defined before code, P22-T03)

- **Decision: no user-level mute or block in v1.** Users cannot mute
  or block each other; there is no per-user hidden-author list, no
  shadow state, and no client-side filter to test. This keeps the
  abuse surface to exactly one reviewed path: the operator-owned
  contributor block (`ops moderation block`, trust `BLOCKED` tier),
  which already exists, is audited per case, time-boxed and
  appealable through the route above.
- If a user-level mute is ever proposed, it must first freeze —
  before any code — its visibility scope (who stops seeing whom,
  and what moderators/audit still see), its rights (who may mute,
  whether muting affects the muted account's reach or only the
  muter's view), its deletion (what happens on erase/export), and
  its API with authorization tests. Until that contract lands in
  this file, mute code must not exist.

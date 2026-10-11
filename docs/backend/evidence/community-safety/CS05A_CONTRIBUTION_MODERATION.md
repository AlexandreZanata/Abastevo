# CS05A — Contribution moderation target

Date: 2026-10-11. Status: LOCAL_DONE. Scope: moderation target,
reasons, one-open-case, invalidate wiring — no routes, no VPS/APK.
Head: maintained `dev`.
Plan: [phased plan](../../../planning/COMMUNITY_SAFETY_EXECUTION_PLAN.md).
Rules: B-BR-CS05.

## Added

- `TargetContribution = "CONTRIBUTION"` (observation UUID) with P2
  default: social-abuse reports on contributions; price-accuracy
  reports keep OBSERVATION with dispute reasons.
- Frozen reason set, enforced for CONTRIBUTION only (other targets
  keep free text): SPAM, HARASSMENT, VOTE_GAMING, OFF_TOPIC, OTHER.
- `AllowedForTarget`: REVIEW/INVALIDATE/RESOLVE/DISMISS accept
  CONTRIBUTION; BLOCK stays contributor-only.
- `actionJob`: INVALIDATE on CONTRIBUTION enqueues
  `community-consensus` with the observation id, like OBSERVATION.
- Migration `000055` widens `moderation_cases_target_check`
  (additive, following the 000026 COMMENT precedent). The existing
  partial unique index gives one-open-case-per-target with no change.
- Ops CLI `moderationInvalidate` accepts OBSERVATION/CONTRIBUTION.

## Hide semantics frozen (compensation)

Hide-from-public is the REJECTED decision plus consensus recompute
through existing machinery — no new action kind, no new writer.
Proven end to end: open CONTRIBUTION case → INVALIDATE act
(`community-consensus` job) → worker records the decision →
helpfulness cast and tally refuse (`ErrIneligibleTarget`) while the
pre-hide vote row and its exact 1/0 tally survive (audit intact,
aggregates accurate). `hidden_at`/`deleted_at` stay NULL, reserved
for author/privacy-led visibility (CS09); the Target reads both as
defense in depth.

## Validation (actual)

- Unit moderation: target accept/UUID-shape, frozen reasons
  (accept + refuse + free-text untouched elsewhere), action
  eligibility matrix.
- Integration moderation: duplicate CONTRIBUTION reports converge
  onto one open case (new test; existing suite green).
- Integration community: full invalidate flow above, incl. audit and
  rebuild exactness.
- `go build`, `go vet` (incl. integration tags), `gofmt -l`,
  `sqlc vet`, `git diff --check` clean; `scan-secrets.sh` passed.

## Limits / owed (CS05B)

- Public tally annotation for targets under open-case review.
- Staging-only local adversarial pass over the helpfulness command
  set (awaits the authorized staging window; e2e negatives already
  cover tamper/unknown-key/stale/oversize/anon locally).
- 15-minute cursor decisions already hold (feed cursor TTL, cited).

Next microtask: CS05B — tally review annotation + adversarial pass.

# P31-T03 — Restricted case review and atomic scoped grants

Status: LOCAL_DONE on `codex/phase-31-verified-representation`. Task: P31-T03.
Binds B-BR-P07/P09/P10/P14/P16 and BUC-P04/P07. Independent operator
decisions with atomic grant writes; no public admin route, no
moderator role from representation.

## Behavior (TDD, critical-first)

- Migration `000039_claim_decisions_grants.sql` (append-only):
  audited `claim_decisions` + `representation_grants` with one-active-
  grant-per-account/station; grants confer representation only.
- Application `ReviewClaim`: reviewer identity required, self-review
  refused, open-state/account-live/operator-current rechecked at
  commit, approval needs valid signature + sufficient authority +
  unchanged operator, denial needs a reason. No public admin route.
- Adapters `ReviewDecisions.DecideAtomically` (single transaction:
  decision + grant + state transition): concurrent approvals
  converge on the open-state guard and the active-grant key (duplicate
  grant writes map to convergence); the loser fails closed.
- Moderation: `STATION_CLAIM` target added to the explicit enum with
  review/resolve/dismiss allowed and invalidate/block refused —
  reports route through the ordinary restricted path and never
  auto-decide.

## Validation

- Unit (`-race`): approve-grants-narrowly, self/stale/denied/reason/
  reviewer refusals, concurrent convergence, moderation target matrix
  — PASS.
- Integration (real PostGIS, `-race`): approve→active grant, self-
  review refusal, 2-way race converging on exactly 1 grant with 1
  closed loser, denial granting nothing. Fresh disposable DBs
  (migrations incl. 000039).
- Fixed from real failures (not weakened): missing sentinels,
  unguarded transitions (added `SetClaimReviewState`), test races
  (shared ports, unlocked maps), raw `no rows` on grant conflicts
  (mapped to convergence), non-UUID test ids.
- Regression: full `stationprofile` + `moderation` + `directory`
  unit + integration PASS (zero failures); `go vet` clean;
  `sqlc vet` + `generate` clean; `go build ./...` PASS.
- `git diff --check` PASS; `scan-secrets.sh` PASS.

## Limits and next

- Human review console/CLI consumes the service ports when
  authorized; reviewer role provisioning is operator-owned.
- Next: P31-T04 business edits and official reply permissions.

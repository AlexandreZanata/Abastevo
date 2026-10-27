# CD-04 staging continuous deploy — what remains (OWED acceptance)

Status: INTEGRATION_PENDING (2026-10-27). Code, harness (166 cases) and Part
B on staging are done: RBAC in `abastevo-temp`, mode-600 kubeconfig, service
user, persistent unit/timer installed, re-seeded baseline (DB `000139`,
floor `000132`, `release_match: false`), one plan-only run. The timer is
disabled; `ABASTEVO_APPLY=0`; no apply has run.

## Context

- Evidence: `docs/backend/evidence/cd/CD-04.md` ("Live staging status and
  remaining acceptance"); plan: `docs/planning/STAGING_CONTINUOUS_DEPLOY_PLAN.md`
  Phases 3–4; agent: `infra/deploy/agent/README.md`.
- Lesson encoded: the 2026-10-26 incident (images rolled before migrating,
  ~6 min outage) in `docs/backend/evidence/STAGING_LANDING_PORTAL_2026-10-26.md`.

## What changed since CD-04 was written

- CD-04 deferred apply because staging runs the maps branch schema `000139`
  while `main` ended at `000132`. `origin/main` (`39a3e6e6`) now has
  `000133`–`000138` and `000140`–`000143` but still not `000139`;
  `000139` arrives with this integration (#162). Once the integration
  merges, `main` contains the map code and the "maps branch blocks apply"
  reason is resolved.
- The staging images (`stg-7c7d776f`) carry `000132` + `000139` only, so the
  staging ledger lacks `000133`–`000138` (verify against the live ledger).
  The first apply will run `000133`–`000138` and `000140`–`000143` after
  `000139`, out of numeric order; `migrate.Apply` applies every missing chain
  entry, but the order change is untested on real data.

## Remaining work

1. Pre-apply rehearsal. Restore the latest verified staging dump into a
   disposable PostGIS, run the integrated `migrate` image, then
   `migrate -check`. Acceptance: all missing migrations apply, check passes,
   no checksum mismatch; result recorded. Structural rehearsal done
   2026-10-27 on a disposable empty PostGIS: chain up to `000132` plus
   `000139` (the staging state), then `000133`–`000138` and `000140`–`000143`
   applied cleanly out of numeric order. The runner applies by version set,
   so the order is supported; the data rehearsal on a restored dump remains.
2. Re-baseline if needed. Acceptance: `current.json` still matches the
   running images and floor `000132`; plan-only run against the merged
   integration SHA lists api, worker, landing and the expected migrations.
3. First supervised apply (`ABASTEVO_APPLY=1`, timer still off), owner
   present. Acceptance: backup Job with TOC + SHA-256, migration Job, one
   shared `set image`, smoke (`/health/ready`, `/`, `/consultar/`, station
   list, portal manifest, search shard), landing switch, `current.json`
   last, receipt; map pins and overlay still served (see
   `MAP_NATIONAL_REMAINING.md` item 3).
4. Enable the timer with apply. Acceptance: owner authorization recorded;
   `abastevo-deploy-agent.timer` active, `ABASTEVO_APPLY=1`; pause runbook
   (`systemctl stop abastevo-deploy-agent.timer`) verified.
5. Two consecutive `main` merges deploy without a human. Acceptance: two
   receipts with distinct SHAs, no manual step between merge and smoke PASS.
6. Broken-image drill. Acceptance: a deliberately broken release is rolled
   back automatically (`rollout undo` for api and worker), site stays up,
   failure receipt written, `current.json` unchanged. The agent only accepts
   signed releases that descend from `main`, so the drill needs an
   owner-approved way to produce one (verify: a reverted drill commit on
   `main`, or another reviewed mechanism).
7. Contract-floor refusal. Acceptance: a release whose
   `schema_min_supported` rises above the previous floor is refused before
   backup/migration; deployment untouched; receipt records the refusal.
   Same production question as item 6.
8. Landing switch and rollback drill. Acceptance: smoke failure after the
   landing switch restores the previous `current` symlink; previous release
   plus last 5 retained.
9. Surge measurement (Part C). Acceptance: with `ABASTEVO_ALLOW_SURGE=1` and
   quota headroom ≥ `ABASTEVO_SURGE_CPU_MILLI`, a rollout records zero
   unavailable replicas; only then may surge become the default (owner).
   Fix the stale README line that still says the R5 overlap proof is OWED
   (CD-04 records it as passing).
10. Plan gaps to verify. Plan Phase 3 step 5 also updates the worker
    search-script env and enqueues one portal refresh; `apply.sh` has no such
    step (verify and implement or amend the plan). Phase 4 operations
    (status/receipt endpoint, owner failure notification, disk retention for
    images/releases/backups) are not started.

## Blockers and owner decisions

- Integration PR merged to `main` (unblocks items 1–3).
- Owner presence/authorization for the first apply and timer enable.
- Drill mechanism for items 6–7 on a signed `main` lineage.
- Surge default and quota headroom (item 9).
- Production stays manual behind G09 (plan Phase 5); nothing here touches it.

## Delivery: one branch, one PR

Items 1–9 are staging operations; their receipts go in
`docs/backend/evidence/cd/CD-04.md` (or a dated CD-05 file). Code/doc changes
(item 9 README fix, item 10 portal-refresh step and Phase 4 operations, any
fix found by the drills) plus the acceptance evidence ship on one branch
`codex/cd-staging-acceptance`, one PR to `main`. Drill commits for items 6–7,
if that mechanism is chosen, are a documented exception (they must reach
`main` separately to be deployable).

# P25 — National ANP registry: execution record

## Current state

- Definition commit and task range: plan `0067e86`; P25-T01 → P25-T02 → P25-T03 → P25-T04 → P25-T05 on `codex/phase-25-national-registry`.
- Publication scope: LOCAL_ONLY. Branch backup pushes only; no PR, CI wait, merge, deploy or wiki at this boundary (ADR-018).
- Repository / base SHA / phase branch / worktree: `brazil-fuel-prices` / base `f005f9f` (P38 checkpoint) / `codex/phase-25-national-registry` / `.worktrees/abastevo-project-batch-flow`.
- Milestone / task issue IDs / optional final batch PR: task IDs P25-T01…T05 per ROADMAP/STATION_CATALOG_PLAN; no per-phase PR by ADR-018 (cumulative batch PR only at final closure).
- Next task and blockers: P26 (`codex/phase-26-regulatory-discovery` via `--from-checkpoint`); no live ANP fetch performed (sandbox egress + reconfirm-at-opening); national calibration P29-owned.
- Integration status and tested head/base: INTEGRATION_PENDING after checkpoint; tested head `731e68e` / base `f005f9f`.
- Wiki source SHA, owned-manifest version, remote commit/status: no wiki sync at phase boundary (final closure only).
- Release status (not implied by phase integration): G09 UNCERTIFIED; no production deployment/tag/pilot; iOS archived.

## Task evidence

- P25-T01 (`f252970`): source contracts frozen (policy + executable package 6/6 + v1 fixtures). Record: `docs/release-evidence/p25-t01-source-contracts.md`.
- P25-T02 (`95ce3d7`): bounded CSV staging (migration 000031 + sqlc + run ledger; unit 11/11 + PostGIS PASS). Record: `docs/release-evidence/p25-t02-csv-staging.md`.
- P25-T03 (`7120575`): API discovery (guarded transport + coordinate honesty; unit 8/8 + PostGIS PASS, no live fetch). Record: `docs/release-evidence/p25-t03-api-discovery.md`.
- P25-T04 (`84822f0`): canonical reconciliation (stable UUIDs, revocation sticks, reviewed points; unit 4/4 + PostGIS e2e/race + zero-price reads PASS). Record: `docs/release-evidence/p25-t04-reconciliation.md`.
- P25-T05 (`731e68e`): reconcile job + disabled schedule + runbook + freshness (handler 3/3 + platform/directory suites PASS, worker builds). Record: `docs/release-evidence/p25-t05-jobs-acceptance.md`.

## Local phase checkpoint

Use only after required local task/specialized exit acceptance passes:

Status: LOCAL_DONE
Validation: PASS

- Tested behavior revision/tree and actual commands/results: head `731e68e` (clean). `go test -race ./internal/modules/directory/...` PASS; real-PostGIS `-race -tags=integration` directory + platform/jobs PASS; `sqlc vet` + `generate` clean; `go build ./cmd/worker` PASS; `vacuum lint` + `apicontract` PASS; `git diff --check` PASS; `scan-secrets.sh` PASS.
- Checkpoint head (private helper state; do not require this file to contain its own commit SHA):
- Parent phase checkpoint / next authorized phase: base P38 checkpoint `f005f9f` / P26 `codex/phase-26-regulatory-discovery`.
- Integration: PENDING until actual final batch merge.
- Deferred manual/device/production obligations: end manual/device batch OWED; provider/media/export OWED; live source fetch + national calibration OWED (P29); G09 production certification deferred — never fabricated PASS.

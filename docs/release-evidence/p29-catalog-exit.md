# P29 — Catalog capacity and refreshed Android acceptance: execution record

## Current state

- Definition commit and task range: plan `0067e86`; P29-T01 → P29-T02 → P29-T03 on `codex/phase-29-catalog-acceptance`.
- Publication scope: LOCAL_ONLY. Branch backup pushes only; no PR, CI wait, merge, deploy or wiki at this boundary (ADR-018).
- Repository / base SHA / phase branch / worktree: `brazil-fuel-prices` / base `dd0579f` (P27 checkpoint) / `codex/phase-29-catalog-acceptance` / `.worktrees/abastevo-project-batch-flow`.
- Milestone / task issue IDs / optional final batch PR: task IDs P29-T01…T03 per ROADMAP/STATION_CATALOG_PLAN; P28 unselected; no per-phase PR by ADR-018 (cumulative batch PR only at final closure).
- Next task and blockers: P30 (`codex/phase-30-station-profiles` via `--from-checkpoint`); full-scale national calibration + live/device rows owed downstream.
- Integration status and tested head/base: INTEGRATION_PENDING after checkpoint; tested head `541c657` / base `dd0579f`.
- Wiki source SHA, owned-manifest version, remote commit/status: no wiki sync at phase boundary (final closure only).
- Release status (not implied by phase integration): G09 UNCERTIFIED; no production deployment/tag/pilot; iOS archived.

## Task evidence

- P29-T01 (`1304399`): capacity campaign with frozen budgets (simulation, no SLA). Record: `docs/release-evidence/p29-t01-capacity.md`.
- P29-T02 (`0622cf8`): source-to-app lifecycle + reacceptance mapping (device rows owed). Record: `docs/release-evidence/p29-t02-lifecycle.md`.
- P29-T03 (`541c657`): pinned candidate + production handoff. Record: `docs/release-evidence/p29-t03-candidate.md`.

## Local phase checkpoint

Use only after required local task/specialized exit acceptance passes:

Status: LOCAL_DONE
Validation: PASS

- Tested behavior revision/tree and actual commands/results: head `541c657` (clean). Backend directory + account/feedback/moderation/evidence/privacy unit+`-race` PASS; real-PostGIS `-race -tags=integration` directory + privacy PASS; `sqlc vet` + `generate` clean; `go build ./...` PASS; Android `:domain :application :data :app` + `:app:assembleDebug` PASS; `vacuum lint` + `apicontract` + `check-compat.sh` PASS; `git diff --check` PASS; `scan-secrets.sh` PASS.
- Checkpoint head (private helper state; do not require this file to contain its own commit SHA):
- Parent phase checkpoint / next authorized phase: base P27 checkpoint `dd0579f` / P30 `codex/phase-30-station-profiles`.
- Integration: PENDING until actual final batch merge.
- Deferred manual/device/production obligations: end manual/device batch OWED (P38-T02 matrix + catalog rows); provider/media/export-implementation OWED; full-scale national calibration OWED; G09 production certification deferred — never fabricated PASS.

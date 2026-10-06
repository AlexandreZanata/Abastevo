# P38 — Source regression and end acceptance handoff: execution record

## Current state

- Definition commit and task range: plan `0067e86`; P38-T01 → P38-T02 → P38-T03 on `codex/phase-38-app-acceptance`.
- Publication scope: LOCAL_ONLY. Branch backup pushes only; no PR, CI wait, merge, deploy or wiki at this boundary (ADR-018).
- Repository / base SHA / phase branch / worktree: `brazil-fuel-prices` / base `72cdc24` (P37 checkpoint) / `codex/phase-38-app-acceptance` / `.worktrees/abastevo-project-batch-flow`.
- Milestone / task issue IDs / optional final batch PR: task IDs P38-T01…T03 per ROADMAP/ANDROID_VPS_PLAN; no per-phase PR by ADR-018 (cumulative batch PR only at final closure).
- Next task and blockers: P25 (`codex/phase-25-national-registry` via `--from-checkpoint`); BLOCKED_LIVE staging reads/writes/uploads + BLOCKED_MEDIA photo path + OWED provider delivery/export implementation/end manual-device batch.
- Integration status and tested head/base: INTEGRATION_PENDING after checkpoint; tested head `f1cf3ac` / base `72cdc24`.
- Wiki source SHA, owned-manifest version, remote commit/status: no wiki sync at phase boundary (final closure only).
- Release status (not implied by phase integration): G09 UNCERTIFIED; no production deployment/tag/pilot; iOS archived.

## Task evidence

- P38-T01 (`7e274d3`): integrated code regression (backend unit+PostGIS + full Android suites + assemble + contracts, 0 failures). Record: `docs/mobile/p38-t01-code-regression.md`.
- P38-T02 (`7a7b9dc`): consolidated end manual/device matrix, plan only, execution OWED. Record: `docs/mobile/p38-t02-manual-matrix.md`.
- P38-T03 (`f1cf3ac`): candidate/handoff (scope, trust/blockers, P25 entry, final-closure plan). Record: `docs/mobile/p38-t03-candidate-handoff.md`.

## Local phase checkpoint

Use only after required local task/specialized exit acceptance passes:

Status: LOCAL_DONE
Validation: PASS

- Tested behavior revision/tree and actual commands/results: head `f1cf3ac` (clean). P38-T01 sweep green (see its record); `git diff --check` PASS; `scan-secrets.sh` PASS; no trust-all/`-k`/HTTP fallback in `data/src/main` (no matches).
- Checkpoint head (private helper state; do not require this file to contain its own commit SHA):
- Parent phase checkpoint / next authorized phase: base P37 checkpoint `72cdc24` / P25 `codex/phase-25-national-registry`.
- Integration: PENDING until actual final batch merge.
- Deferred manual/device/production obligations: end manual/device batch OWED (P38-T02 matrix); provider/media/export OWED; staging live + photo path BLOCKED; catalog/profile P25–P33 PLANNED; G09 production certification deferred — never fabricated PASS.

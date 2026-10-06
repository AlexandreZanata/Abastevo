# P30 — Station profile and private claim foundations: execution record

## Current state

- Definition commit and task range: plan `0067e86`; P30-T01 → P30-T02 → P30-T03 → P30-T04 on `codex/phase-30-station-profiles`.
- Publication scope: LOCAL_ONLY. Branch backup pushes only; no PR, CI wait, merge, deploy or wiki at this boundary (ADR-018).
- Repository / base SHA / phase branch / worktree: `brazil-fuel-prices` / base `de3d207` (P29 checkpoint) / `codex/phase-30-station-profiles` / `.worktrees/abastevo-project-batch-flow`.
- Milestone / task issue IDs / optional final batch PR: task IDs P30-T01…T04 per ROADMAP/STATION_PROFILE_PLAN; no per-phase PR by ADR-018 (cumulative batch PR only at final closure).
- Next task and blockers: P31 (`codex/phase-31-verified-representation` via `--from-checkpoint`); verifier selection + Receita/QSA access + staffing + storage provisioning explicitly OPEN.
- Integration status and tested head/base: INTEGRATION_PENDING after checkpoint; tested head `c548997` / base `de3d207`.
- Wiki source SHA, owned-manifest version, remote commit/status: no wiki sync at phase boundary (final closure only).
- Release status (not implied by phase integration): G09 UNCERTIFIED; no production deployment/tag/pilot; iOS archived.

## Task evidence

- P30-T01 (`ec8eabf`): contract freeze with 4 OPEN items (docs-only). Record: `docs/release-evidence/p30-t01-contract-freeze.md`.
- P30-T02 (`b72766f`): operator link + public unclaimed profile (module, migration 000036, handler + OpenAPI/wiring). Record: `docs/release-evidence/p30-t02-operator-profile.md`.
- P30-T03 (`c4a7eea`): claims + one-use declarations (migration 000037 + routes + OpenAPI/wiring). Record: `docs/release-evidence/p30-t03-claims.md`.
- P30-T04 (`c548997`): proof intake + expiry (migration 000038 + validation + 503-without-storage). Record: `docs/release-evidence/p30-t04-proof-intake.md`.

## Local phase checkpoint

Use only after required local task/specialized exit acceptance passes:

Status: LOCAL_DONE
Validation: PASS

- Tested behavior revision/tree and actual commands/results: head `c548997` (clean). `go test -race ./internal/modules/stationprofile/... ./internal/modules/directory/...` PASS; real-PostGIS `-race -tags=integration` stationprofile + directory PASS; `sqlc vet` + `generate` clean; `go build ./...` PASS; `vacuum lint` + `apicontract` + `check-compat.sh` PASS; `git diff --check` PASS; `scan-secrets.sh` PASS.
- Checkpoint head (private helper state; do not require this file to contain its own commit SHA):
- Parent phase checkpoint / next authorized phase: base P29 checkpoint `de3d207` / P31 `codex/phase-31-verified-representation`.
- Integration: PENDING until actual final batch merge.
- Deferred manual/device/production obligations: end manual/device batch OWED; provider/media/export-implementation OWED; verifier selection + Receita/QSA access + staffing + storage provisioning OPEN; G09 production certification deferred — never fabricated PASS.

# P27 — Account station intake and Android catalog: execution record

## Current state

- Definition commit and task range: plan `0067e86`; P27-T01 → P27-T02 → P27-T03 → P27-T04 → P27-T05 on `codex/phase-27-station-intake`.
- Publication scope: LOCAL_ONLY. Branch backup pushes only; no PR, CI wait, merge, deploy or wiki at this boundary (ADR-018).
- Repository / base SHA / phase branch / worktree: `brazil-fuel-prices` / base `d873c4f` (P26 checkpoint) / `codex/phase-27-station-intake` / `.worktrees/abastevo-project-batch-flow`.
- Milestone / task issue IDs / optional final batch PR: task IDs P27-T01…T05 per ROADMAP/STATION_CATALOG_PLAN; no per-phase PR by ADR-018 (cumulative batch PR only at final closure).
- Next task and blockers: P29 (`codex/phase-29-catalog-acceptance` via `--from-checkpoint`); P28 optional (selected source only); live-source/device rows owed downstream.
- Integration status and tested head/base: INTEGRATION_PENDING after checkpoint; tested head `58fa437` / base `d873c4f`.
- Wiki source SHA, owned-manifest version, remote commit/status: no wiki sync at phase boundary (final closure only).
- Release status (not implied by phase integration): G09 UNCERTIFIED; no production deployment/tag/pilot; iOS archived.

## Task evidence

- P27-T01 (`49623a4`): signed intake + private status (migration 000034 + service/handler/OpenAPI/wiring; unit + PostGIS PASS). Record: `docs/release-evidence/p27-t01-signed-intake.md`.
- P27-T02 (`4b44f29`): exact-match verification + audited review + unknown pins (unit + PostGIS + worker PASS). Record: `docs/release-evidence/p27-t02-verified-decisions.md`.
- P27-T03 (`6658af4`): Room v8 catalog cache + offline replay (Room 4/4 + migration guard PASS; full Android suites green). Record: `docs/release-evidence/p27-t03-catalog-cache.md`.
- P27-T04 (`464d05e`): suggest/correct/status journey (10 new tests PASS; full suites + assemble green). Record: `docs/release-evidence/p27-t04-suggest-journey.md`.
- P27-T05 (`58fa437`): liveness gate + abuse/privacy/lifecycle (quota/races + cross-module suites green). Record: `docs/release-evidence/p27-t05-abuse-privacy.md`.

## Local phase checkpoint

Use only after required local task/specialized exit acceptance passes:

Status: LOCAL_DONE
Validation: PASS

- Tested behavior revision/tree and actual commands/results: head `58fa437` (clean). Backend `go test -race ./internal/modules/directory/...` + account/moderation/evidence/privacy PASS; real-PostGIS `-race -tags=integration` directory PASS; `sqlc vet` + `generate` clean; `go build ./...` PASS; Android `:domain :application :data :app` tests + `:app:assembleDebug` PASS; `vacuum lint` + `apicontract` + `check-compat.sh` PASS; `git diff --check` PASS; `scan-secrets.sh` PASS.
- Checkpoint head (private helper state; do not require this file to contain its own commit SHA):
- Parent phase checkpoint / next authorized phase: base P26 checkpoint `d873c4f` / P29 `codex/phase-29-catalog-acceptance`.
- Integration: PENDING until actual final batch merge.
- Deferred manual/device/production obligations: end manual/device batch OWED; provider/media/export-implementation OWED; retention sweeper + review UI deferred explicit; national calibration OWED (P29); G09 production certification deferred — never fabricated PASS.

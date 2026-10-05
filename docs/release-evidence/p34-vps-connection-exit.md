# P34 — Staging connection and a useful test catalog: execution record

## Current state

- Definition commit and task range: plan `0067e86`; P34-T01 → P34-T02 → P34-T03 on `codex/phase-34-vps-connection`.
- Publication scope: LOCAL_ONLY. Branch backup pushes only; no PR, CI wait, merge, deploy or wiki at this boundary (ADR-018).
- Repository / base SHA / phase branch / worktree: `brazil-fuel-prices` / base `0067e86` (plan head over `b4754b2`) / `codex/phase-34-vps-connection` / `.worktrees/abastevo-project-batch-flow`.
- Milestone / task issue IDs / optional final batch PR: task IDs P34-T01…T03 per ROADMAP/ANDROID_VPS_PLAN; no per-phase PR by ADR-018 (cumulative batch PR only at final closure).
- Next task and blockers: P35 (`codex/phase-35-live-discovery` via `--from-checkpoint`); BLOCKED_LIVE staging reads (TLS trust gap, `curl` 60, Fortinet middlebox CA) — not a VPS outage diagnosis.
- Integration status and tested head/base: INTEGRATION_PENDING after checkpoint; tested head `d05c7d3` / base `0067e86`.
- Wiki source SHA, owned-manifest version, remote commit/status: no wiki sync at phase boundary (final closure only).
- Release status (not implied by phase integration): G09 UNCERTIFIED; no production deployment/tag/pilot; iOS archived.

## Task evidence

- P34-T01 (`9b4740c`): client/flag/contract inventory + TLS verification without bypass. Live `https://teste.abastevo.com.br` UNVERIFIED (issuer `FG6H0FTB23902129`, `curl` 60 on 4 probes). Local vacuum/apicontract/health/directory-PostGIS + Android HTTPS tests PASS. Record: `docs/mobile/p34-t01-staging-inventory.md`.
- P34-T02 (`40e7a9f`): shared `ApiEnvironment` staging origin wired into auth/identity/community/feedback/contribution DI. RED→GREEN `ApiEnvironmentTest` + client/session regression + Hilt KSP + `:app:assembleDebug` PASS; vacuum/apicontract/health PASS; live still UNVERIFIED, no bypass. Record: `docs/mobile/p34-t02-environment.md`.
- P34-T03 (`d05c7d3`): bounded synthetic catalog spec (`contracts/testdata/p34/catalog.json` v1, 3 owned `[P34-TEST]` stations + 9 exercise cases). RED→GREEN `TestP34Catalog*` + full apicontract + vacuum + real-PostGIS directory PASS. NOT SEEDED, population unclaimed. Record: `docs/mobile/p34-t03-test-catalog.md`.

## Local phase checkpoint

Use only after required local task/specialized exit acceptance passes:

Status: LOCAL_DONE
Validation: PASS

- Tested behavior revision/tree and actual commands/results: head `d05c7d3` (clean). `go test -count=1 ./internal/platform/apicontract/...` PASS; `vacuum lint` PASS (0 errors, 27 informs); `go test -count=1 -tags=integration ./internal/modules/directory/... ./internal/platform/health/...` PASS (real PostGIS `127.0.0.1:5434`, incl. 400 invalid-lat / 404 unknown-UUID honesty); `./gradlew :data:testDebugUnitTest --tests ...ApiEnvironmentTest --rerun-tasks` BUILD SUCCESSFUL (26 executed); `git diff --check` PASS; `scan-secrets.sh` PASS; no trust-all/`-k`/HTTP fallback in `data/src/main` (no matches).
- Checkpoint head (private helper state; do not require this file to contain its own commit SHA):
- Parent phase checkpoint / next authorized phase: base plan `0067e86` / P35 `codex/phase-35-live-discovery`.
- Integration: PENDING until actual final batch merge.
- Deferred manual/device/production obligations: end manual/device batch OWED (2026-10-02 directive); provider/media proof OWED; staging live reads BLOCKED_LIVE; G09 production certification deferred — never fabricated PASS.

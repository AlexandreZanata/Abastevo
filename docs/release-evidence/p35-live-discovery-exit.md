# P35 — Live discovery and community prices: execution record

## Current state

- Definition commit and task range: plan `0067e86`; P35-T01 → P35-T02 → P35-T03 on `codex/phase-35-live-discovery`.
- Publication scope: LOCAL_ONLY. Branch backup pushes only; no PR, CI wait, merge, deploy or wiki at this boundary (ADR-018).
- Repository / base SHA / phase branch / worktree: `brazil-fuel-prices` / base `6e032d7` (P34 checkpoint) / `codex/phase-35-live-discovery` / `.worktrees/abastevo-project-batch-flow`.
- Milestone / task issue IDs / optional final batch PR: task IDs P35-T01…T03 per ROADMAP/ANDROID_VPS_PLAN; no per-phase PR by ADR-018 (cumulative batch PR only at final closure).
- Next task and blockers: P36 (`codex/phase-36-live-community` via `--from-checkpoint`); BLOCKED_LIVE staging reads (TLS trust gap, `curl` 60, Fortinet middlebox CA) — not a VPS outage diagnosis.
- Integration status and tested head/base: INTEGRATION_PENDING after checkpoint; tested head `567f9c0` / base `6e032d7`.
- Wiki source SHA, owned-manifest version, remote commit/status: no wiki sync at phase boundary (final closure only).
- Release status (not implied by phase integration): G09 UNCERTIFIED; no production deployment/tag/pilot; iOS archived.

## Task evidence

- P35-T01 (`48f456b`): canonical Directory UUID ports/cache (domain `ServerStation` + gateway/cache ports, data HTTP client/codec/gateway/memory-cache + `DirectoryModule`, no Room migration, legacy CNPJ untouched). RED→GREEN domain/data suites + Hilt KSP + `:app:assembleDebug` PASS; vacuum/apicontract/PostGIS PASS; live UNVERIFIED, no bypass. Record: `docs/mobile/p35-t01-directory-ports.md`.
- P35-T02 (`b284ca2`): flag-gated server use cases + ViewModel UUID state + row/detail sheet + en/pt-BR strings; application 5/5 + app discovery 5/5 + untouched `StationsViewModelTest` PASS; assemble PASS; live UNVERIFIED. Record: `docs/mobile/p35-t02-explore-detail.md`.
- P35-T03 (`567f9c0`): transient nearby use case, cancellable loads, refresh-preserving selection, GPS-denied honesty; application 7/7 + app discovery 8/8 + full domain/application/data/app suites + assemble + KSP PASS; live UNVERIFIED. Record: `docs/mobile/p35-t03-search-regression.md`.

## Local phase checkpoint

Use only after required local task/specialized exit acceptance passes:

Status: LOCAL_DONE
Validation: PASS

- Tested behavior revision/tree and actual commands/results: head `567f9c0` (clean). `./gradlew :domain:test :application:test :data:testDebugUnitTest` PASS; `:app:testDebugUnitTest` PASS (incl. `StationsServerDiscoveryTest` 8/8 + `StationsViewModelTest` untouched); `:app:assembleDebug` PASS; `vacuum lint` PASS (0 errors, 27 informs); `go test ./internal/platform/apicontract/...` PASS; real-PostGIS `directory/... + health` PASS (reused baseline, no SQL touched); `git diff --check` PASS; `scan-secrets.sh` PASS; no trust-all/`-k`/HTTP fallback in `data/src/main` (no matches).
- Checkpoint head (private helper state; do not require this file to contain its own commit SHA):
- Parent phase checkpoint / next authorized phase: base P34 checkpoint `6e032d7` / P36 `codex/phase-36-live-community`.
- Integration: PENDING until actual final batch merge.
- Deferred manual/device/production obligations: end manual/device batch OWED (2026-10-02 directive); provider/media proof OWED; staging live reads BLOCKED_LIVE; G09 production certification deferred — never fabricated PASS.

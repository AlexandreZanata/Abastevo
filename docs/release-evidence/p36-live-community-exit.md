# P36 — Free account and social journeys: execution record

## Current state

- Definition commit and task range: plan `0067e86`; P36-T01 → P36-T02 → P36-T03 on `codex/phase-36-live-community`.
- Publication scope: LOCAL_ONLY. Branch backup pushes only; no PR, CI wait, merge, deploy or wiki at this boundary (ADR-018).
- Repository / base SHA / phase branch / worktree: `brazil-fuel-prices` / base `c5da7f6` (P35 checkpoint) / `codex/phase-36-live-community` / `.worktrees/abastevo-project-batch-flow`.
- Milestone / task issue IDs / optional final batch PR: task IDs P36-T01…T03 per ROADMAP/ANDROID_VPS_PLAN; no per-phase PR by ADR-018 (cumulative batch PR only at final closure).
- Next task and blockers: P37 (`codex/phase-37-live-contributions` via `--from-checkpoint`); BLOCKED_LIVE staging reads/writes + provider delivery (TLS trust gap, `curl` 60) — not a VPS outage diagnosis.
- Integration status and tested head/base: INTEGRATION_PENDING after checkpoint; tested head `c04fb0d` / base `c5da7f6`.
- Wiki source SHA, owned-manifest version, remote commit/status: no wiki sync at phase boundary (final closure only).
- Release status (not implied by phase integration): G09 UNCERTIFIED; no production deployment/tag/pilot; iOS archived.

## Task evidence

- P36-T01 (`9efd2d5`): staging origin wiring locked (guard 3/3, auth/account/identity suites PASS, no creds in Git); provider delivery/device proof OWED. Record: `docs/mobile/p36-t01-staging-accounts.md`.
- P36-T02 (`0ed3d17`): canonical-UUID feedback screens (`FeedbackTarget` 3/3 + section in server detail, guest reads/signed writes); stations+community suites + assemble PASS. Record: `docs/mobile/p36-t02-feedback-screens.md`.
- P36-T03 (`c04fb0d`): session→discussion binding + deletion/session-revocation verified, export contract frozen NOT IMPLEMENTED; app 10/10 + backend race+PostGIS PASS. Record: `docs/mobile/p36-t03-activity-rights.md`.

## Local phase checkpoint

Use only after required local task/specialized exit acceptance passes:

Status: LOCAL_DONE
Validation: PASS

- Tested behavior revision/tree and actual commands/results: head `c04fb0d` (clean). `./gradlew :domain:test :application:test :data:testDebugUnitTest` PASS; `:app:testDebugUnitTest` PASS (incl. `StationsServerDiscoveryTest` 10/10 + untouched auth/stations/feedback suites); `:app:assembleDebug` PASS; backend `go test -race` account/identity/feedback/moderation PASS + real-PostGIS `-tags=integration` account + `-race` feedback/moderation PASS; `vacuum lint` PASS; `apicontract` PASS; `git diff --check` PASS; `scan-secrets.sh` PASS; no trust-all/`-k`/HTTP fallback in `data/src/main` (no matches).
- Checkpoint head (private helper state; do not require this file to contain its own commit SHA):
- Parent phase checkpoint / next authorized phase: base P35 checkpoint `c5da7f6` / P37 `codex/phase-37-live-contributions`.
- Integration: PENDING until actual final batch merge.
- Deferred manual/device/production obligations: end manual/device batch OWED (2026-10-02 directive); provider/media proof OWED; staging live reads/writes BLOCKED_LIVE; export endpoint implementation OWED per frozen decision; G09 production certification deferred — never fabricated PASS.

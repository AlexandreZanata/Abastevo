# P37 — Capture and durable contributions: execution record

## Current state

- Definition commit and task range: plan `0067e86`; P37-T01 → P37-T02 → P37-T03 on `codex/phase-37-live-contributions`.
- Publication scope: LOCAL_ONLY. Branch backup pushes only; no PR, CI wait, merge, deploy or wiki at this boundary (ADR-018).
- Repository / base SHA / phase branch / worktree: `brazil-fuel-prices` / base `fd5cb7a` (P36 checkpoint) / `codex/phase-37-live-contributions` / `.worktrees/abastevo-project-batch-flow`.
- Milestone / task issue IDs / optional final batch PR: task IDs P37-T01…T03 per ROADMAP/ANDROID_VPS_PLAN; no per-phase PR by ADR-018 (cumulative batch PR only at final closure).
- Next task and blockers: P38 (`codex/phase-38-app-acceptance` via `--from-checkpoint`); BLOCKED_LIVE staging upload + BLOCKED_MEDIA photo path (TLS trust gap + no attached VPS private storage) — not VPS outage diagnoses.
- Integration status and tested head/base: INTEGRATION_PENDING after checkpoint; tested head `368597f` / base `fd5cb7a`.
- Wiki source SHA, owned-manifest version, remote commit/status: no wiki sync at phase boundary (final closure only).
- Release status (not implied by phase integration): G09 UNCERTIFIED; no production deployment/tag/pilot; iOS archived.

## Task evidence

- P37-T01 (`8545440`): contextual capture/review (`ContributionTarget` RED→GREEN + ViewModel bind + target-aware nav, FAB flow kept); capture/nav + photo/OCR + evidence suites PASS. Record: `docs/mobile/p37-t01-contextual-capture.md`.
- P37-T02 (`75e0c01`): confirmed→outbox submit (target+fuel-match guards, worker/gateway verified, no backend defect); capture + contribution/outbox suites + assemble PASS. Record: `docs/mobile/p37-t02-upload-outbox.md`.
- P37-T03 (`368597f`): storage/expiry verification (24h all-copy matrix PASS unit+PostGIS, docs-only); live photo BLOCKED_MEDIA. Record: `docs/mobile/p37-t03-storage-expiry.md`.

## Local phase checkpoint

Use only after required local task/specialized exit acceptance passes:

Status: LOCAL_DONE
Validation: PASS

- Tested behavior revision/tree and actual commands/results: head `368597f` (clean). `./gradlew :domain:test :application:test :data:testDebugUnitTest` PASS; `:app:testDebugUnitTest` PASS (incl. capture/discovery suites); `:app:assembleDebug` PASS; backend `go test` evidence/privacy unit + real-PostGIS `-tags=integration` evidence/privacy PASS; `vacuum lint` PASS; `apicontract` PASS; `git diff --check` PASS; `scan-secrets.sh` PASS; no trust-all/`-k`/HTTP fallback in `data/src/main` (no matches).
- Checkpoint head (private helper state; do not require this file to contain its own commit SHA):
- Parent phase checkpoint / next authorized phase: base P36 checkpoint `fd5cb7a` / P38 `codex/phase-38-app-acceptance`.
- Integration: PENDING until actual final batch merge.
- Deferred manual/device/production obligations: end manual/device batch OWED (2026-10-02 directive, incl. low-end encode + camera proof); provider/media proof OWED; staging live upload BLOCKED_LIVE + photo path BLOCKED_MEDIA; G09 production certification deferred — never fabricated PASS.

# P18-T02 — Performance and memory baselines

Status: LOCAL_VALIDATION_IMPLEMENTED / FULL_ACCEPTANCE_BLOCKED on `codex/phase-18-functional-acceptance` (second
task; PR #67). Issue: #61. Docs only: every measured budget met,
so per "profile before optimization" no hot path was touched.

## Baselines (this host, 2026-10-01)

- Backend load smoke (`test-load.sh`, disposable dev PostGIS,
  500 seeded stations, 20 s at concurrency 8): **52,636 requests,
  p95 15.1 ms (budget 300 ms), 5xx rate 0.0000 (budget 0.01)** —
  20× latency headroom. Fault suite 7/7 (outage refuses/fails
  closed, recovery, no unauthorized upload under storage outage,
  capped disk pressure).
- Photo memory (`TestSanitizeForwardMeasuredMemory`): 2 MP
  sanitize emits **71,731 wire bytes** (target 150 KiB, cap
  256 KiB) with **3,329,392 bytes heap growth vs 32 MiB
  hypothesis** — unchanged from P15, 10× headroom, no regression.
- Android battery + `assembleDebug`: BUILD SUCCESSFUL (no
  main-thread/decode changes in this phase to profile further
  on-host).

## Optimization decision

None: no budget breached, no new allocation hotspot found, so no
code changed. Inventing an optimization without a violating
profile would be speculative work against the task outline.

## Validation (exact commands, this host)

```sh
bash scripts/tests/test-load.sh   # EXIT=0, log /tmp/p18-t02-load.log
go test -count=1 -v -run TestSanitizeForwardMeasuredMemory ./internal/modules/evidence/adapters/media/
./gradlew :domain:test :application:test :data:testDebugUnitTest :app:testDebugUnitTest :app:assembleDebug --no-daemon
git diff --check
bash scripts/scan-secrets.sh
```

Dev DB stopped after the run (named volume retained by the dev
topology, no seed residue outside `b000…` rows it already owns).

## Limits (not claimed)

- Physical Android follow-up: existing cached-home readiness test passed at 382 ms vs 2000 ms budget (P18 local exit). Complete cold-process/frame/heap/battery and iOS device matrix remains unproven; no macOS/iOS device.
- 30-minute staging matrix (100 k stations, 1 M rows): runs on
  provisioned staging per harness design, never here.
- No release claim by these baselines.


Publication reconciliation: PR #67 integrates the validated local slice only. The original task acceptance remains unmet; see [current exit and blockers](p18-local-integration-exit.md). Historical command results above are retained and are not relabeled device proof.

# P18-T01 — Feature matrix and final local acceptance

Status: LOCAL_VALIDATION_IMPLEMENTED / FULL_ACCEPTANCE_BLOCKED on `codex/phase-18-functional-acceptance` (first
task; PR #67). Issue: #60. No production change; one
integration-test harness fix; otherwise docs only.

## Slice acceptance (frozen before coding)

- T01 (this commit) — Freeze the expected Android/iOS/account/
  feedback/photo/location matrix with actual local-service proof:
  Android battery with exact counts, backend fast gate, backend
  units + real-PostGIS integration under `-race`, static scans,
  and explicit BLOCKED rows (S3-emulator registry leg, macOS
  device leg) that gate G18. No silent skips.

## What changed

- `backend/.../account/adapters/pglinks_integration_test.go` —
  mutex around the four stub generators (`CodeGen/TokenGen/
  AliasGen/IDGen`) in `freshLinkService`. The 16-goroutine
  `TestPGBindRaceAdmitsExactlyOneOwner` raced the harness's own
  counters under `-race` (2 warnings → FAIL in 3 runs);
  `-race -count=5` clean after the fix, full integration sweep
  zero FAIL / zero warnings. `//go:build integration` file only:
  production code untouched.
- `docs/mobile/p18-t01-acceptance-matrix.md` — frozen GREEN/
  BLOCKED matrix, candidate artifacts (not a release) and the
  three G18 blockers.

## RED → GREEN

- RED: `TestPGBindRaceAdmitsExactlyOneOwner` FAIL under `-race`
  (harness counters, reproduced before the fix).
- GREEN: same test `-race -count=5` clean; backend units zero
  FAIL; full `-tags=integration` sweep zero FAIL; Android
  battery `--rerun-tasks` (domain 73/378, application 44/238,
  data 52/198 + 1 pre-existing skip, app 28/112), `assembleDebug`
  SUCCESS; `check-backend-fast.sh`, `vet`, `staticcheck`,
  `sqlc vet`, `govulncheck`, `check-mobile --static-only`,
  `diff --check`, secrets all clean.

## Validation (exact commands, this host)

```sh
./gradlew :domain:test :application:test --rerun-tasks --no-daemon
./gradlew :data:testDebugUnitTest :app:testDebugUnitTest :app:assembleDebug --no-daemon
bash scripts/check-backend-fast.sh
# disposable digest-pinned PostGIS tmpfs DB on 127.0.0.1:32768:
go run ./cmd/migrate && go test -race -count=1 ./...
go test -race -count=1 -p 2 -timeout 10m -tags=integration ./...
go vet ./... && staticcheck ./... && sqlc vet && govulncheck ./...
bash scripts/check-mobile.sh --static-only
git diff --check
bash scripts/scan-secrets.sh
```

`test-local-backend.sh` attempted as the full-matrix harness:
BLOCKED at startup — `minio/minio` digest pull denied by the
registry on this host (`startup.log` kept in `/tmp`). PostGIS
leg executed standalone instead (recorded, never substituted).

## Limits (not claimed)

- S3-emulator/backup/restore/load/edge legs stay BLOCKED until
  registry access (or a reviewed image decision) exists.
- Swift/device/performance/security rows stay open for the Mac
  run and P18-T02/T03. No candidate selected, no release claim.
- Disposable validation DB removed (`down --volumes`); tmpfs
  data never persisted.


Publication reconciliation: PR #67 integrates the validated local slice only. The original task acceptance remains unmet; see [current exit and blockers](p18-local-integration-exit.md). Historical command results above are retained and are not relabeled device proof.

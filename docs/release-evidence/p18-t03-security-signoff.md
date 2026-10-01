# P18-T03 — Security compatibility and functional sign-off

Status: LOCAL_DONE on `codex/phase-18-functional-acceptance` (exit
task; no PR yet). Issue: #62. Docs only; no production change.

## Candidate reconciliation (exact)

- Base: `origin/main 047a347` (P17 merge). Phase commits: `6ff7145`
  (open), `3b302e7` (T01 matrix + bind-race harness fix),
  `cf452b1` (T02 baselines). No candidate selected: selection is a
  G18 act, not this task.
- Dependency drift vs base and vs `origin/main`: EMPTY
  (`backend/go.mod`, `go.sum`, `gradle/libs.versions.toml`
  untouched). No new permission, capability or network client.
- Migrations: 30 frozen files untouched; empty→30 applies clean,
  re-run idempotent ("schema already current") on disposable
  digest-pinned PostGIS.
- API compatibility: `check-compat.sh` ok (OpenAPI lint 0 errors,
  frozen deltas fixture, wire enums, runbooks present).

## Security gates (this host)

- `test-security.sh`: **10 passed, 0 failed** (reviewed tree
  accepted; public port / floating image / open proxies / dev
  secret / unpinned builder / root runtime / go.mod replace all
  refused; govulncheck clean; adversarial selection executes).
- `scan-secrets.sh` + PR-diff surface: clean.

## Adversarial coverage (executed, not claimed by suite name)

- Auth/recovery/abuse: account application + adapters suites
  (bind idempotent/stolen/fresh-proof/dead-account, link cross-
  account/nonce-replay/unlink-last, code replay/expiry/quota,
  refresh rotation/reuse-revokes-family, revoke-all) — units and
  real-PostGIS integration under `-race`, zero FAIL/warnings.
- Unicode: 280-scalar/emoji/combining portable + viewmodel
  subsets green (domain/application/app community filters).
- Expiry: 24 h transient + audit-expiry suites inside the
  evidence/privacy sweep, green.
- GPS: simulated/denied/unknown portable verdicts inside the
  community sweep, green.
- Upgrades: migration idempotence proven above; no destructive
  rewrite anywhere (append-only history preserved).

## Support matrix (frozen)

| Platform | Support | Proof |
|---|---|---|
| Android (min 26 / target 35, Kotlin 2.0.21 / AGP 8.7.3) | Supported locally | Unit batteries + assemble green (P18-T01 counts) |
| Backend (Go pinned toolchain, PostGIS 18/3.6) | Supported locally | Race units + integration, load p95 15.1 ms, faults 7/7 |
| iPhone (iOS 17 / macOS 14 per SPM) | NOT PROVEN | No Xcode/macOS on this host — BLOCKS G18 |
| Staging 30-min matrix / S3-emulator legs | NOT RUN | Registry-blocked + provisioned-infra-only — BLOCK G18 |

## Sign-off (explicit non-acceptance)

- No critical findings open: the one real find of this phase
  (bind-race harness, T01) is fixed and regressed.
- **G18 is NOT accepted by this task**: real Android/iOS device
  evidence is missing per the standing blocker list in P18-T01.
- **G09 remains uncertified** until its separate production
  matrix runs after G18. No deploy, tag, pilot or release claim.

## Validation (exact commands, this host)

```sh
bash scripts/tests/test-security.sh        # 10 passed, 0 failed
bash scripts/check-compat.sh              # compat ok
bash scripts/scan-secrets.sh              # clean
# disposable digest-pinned PostGIS tmpfs DB (removed afterwards):
go run ./cmd/migrate && go run ./cmd/migrate   # 30 applied, idempotent
go test -race -count=1 ./internal/modules/account/... ./internal/modules/feedback/... ./internal/modules/community/... ./internal/modules/evidence/... ./internal/modules/privacy/...
go test -race -count=1 -p 2 -timeout 10m -tags=integration ./internal/modules/account/... ./internal/modules/feedback/... ./internal/modules/community/... ./internal/modules/evidence/... ./internal/modules/privacy/...
./gradlew :domain:test :application:test --tests "com.anpfuel.domain.portable.*" --tests "com.anpfuel.application.usecase.feedback.*" --tests "com.anpfuel.application.portable.*" --rerun-tasks --no-daemon
./gradlew :app:testDebugUnitTest --tests "com.anpfuel.app.community.*" --no-daemon
git diff --check
```

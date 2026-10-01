# P18-T01 — Frozen functional acceptance matrix

Status: LOCAL_VALIDATION_IMPLEMENTED / G18_ACCEPTANCE_BLOCKED on `codex/phase-18-functional-acceptance` (first
task; no PR yet). Issue: #60. Entry: G17 + G10-LOCAL. One
test-harness fix (no production change, below); otherwise docs only.

Frozen 2026-10-01 on `origin/main 047a347` plus this task's commit.
GREEN means executed on this host with the exact command in the
task record. BLOCKED means missing result that blocks G18 — never
a silent skip, never green by mock.

## Result matrix

| Requirement | State | Evidence |
|---|---|---|
| Backend units under `-race` (all packages) | GREEN | `go test -race -count=1 ./...`: zero FAIL; `p18-t01` task record |
| Backend integration on real PostGIS under `-race` | GREEN | Disposable `postgis/postgis:18-3.6` digest-pinned tmpfs DB, 30 migrations from empty, full `-tags=integration` sweep zero FAIL / zero race warnings after harness fix |
| Account bind race admits exactly one owner | GREEN | `TestPGBindRaceAdmitsExactlyOneOwner -race -count=5` clean; harness-only mutex fix (stub generators), production untouched |
| Backend static (`vet`, `staticcheck`, `sqlc vet`, `govulncheck`) | GREEN | All clean, 0 vulnerabilities |
| Backend fast gate | GREEN | `check-backend-fast.sh` passed (gofmt/build/vet/units/static/sqlc/vacuum/secrets) |
| Android regression (`domain` 73 suites / 378 tests) | GREEN | `--rerun-tasks`, 0 failures |
| Android regression (`application` 44 / 238) | GREEN | `--rerun-tasks`, 0 failures, incl. `FeedbackLifecycleExitTest` 6/6 |
| Android regression (`data` 52 / 198, 1 pre-existing skip) | GREEN | `--rerun-tasks`, 0 failures (one stale XML from an up-to-date run initially suggested a failure; rerun disproved it) |
| Android regression (`app` 28 / 112) + `assembleDebug` | GREEN | `--rerun-tasks` SUCCESS |
| Account/social/media/location portable exits | GREEN JVM-side | P13–P17 exit suites green; matching XCTest vectors ship Mac-gated (P17 record) |
| Full `test-local-backend.sh` (S3 emulator, backup/restore/load/edge) | BLOCKED (environment) | `minio/minio` digest pull denied by registry (`pull access denied`) on this host; PostGIS leg ran standalone instead. Capacity blocker, not product verdict |
| iPhone `swift build/test`, simulator/device, lifecycle | BLOCKED | No Xcode/macOS on this host (standing precedent since P12-T04) |
| Backend/photo baselines and security/compatibility review | GREEN within recorded local scope | P18-T02 and P18-T03 records; device performance and native sign-off remain unproven |

## Candidate artifacts (frozen, NOT a release)

- Backend + app source: phase head after this task's commit (exact
  SHA in git log); only change vs `047a347` is the race-harness
  mutex in `pglinks_integration_test.go` (`//go:build integration`).
- No release candidate selected: G18 selects it only after T02/T03
  and the BLOCKED rows above resolve with actual evidence.
- No deploy, tag, public pilot or production claim by this matrix.

## G18 blockers (must resolve before G18)

1. S3-emulator leg of the local matrix (registry access or vendored
   image decision with license/provenance review).
2. macOS build/test/simulator/device run with synthetic server.
3. Actual Android/iOS device performance and integrated functional acceptance. Backend/photo baselines and security/compatibility review are implemented; they do not satisfy missing device rows.


## Publication reconciliation (2026-10-01)

The maintainer confirmed no Mac is available. A physical Android was detected, the first startup/community device attempt was INSTALL_FAILED_USER_RESTRICTED. After maintainer authorization/releasing installation, the same command passed 2/2 with no failures/errors/skips; startup test recorded 382 ms vs its 2000 ms limit. This proves only the two existing test cases, not the full device matrix. The corrected account-bind race test passed five times with real disposable PostGIS and `-race` on the reconciled branch; mobile static/compatibility checks passed. [Local integration exit](../release-evidence/p18-local-integration-exit.md) records the exact scope. P18 local work may integrate, while G18 acceptance, issues #60–#62 and their milestone remain open until actual required proof exists.

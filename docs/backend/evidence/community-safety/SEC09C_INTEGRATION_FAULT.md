# SEC09-C — Local integration and fault/recovery evidence

Date: 2026-10-11. Status: LOCAL_DONE. Scope: affected checks once +
release-scoped fault/recovery at this head (no product change). Head:
maintained `dev` + this task (tested head recorded below).
Plan: [remediation plan](../../../planning/SECURITY_REMEDIATION_PLAN_2026-10-11.md)
(SEC09). Contract: migration append-only discipline; Android
regression suites; backend gates. Constraint: prior evidence does not
cover later commits — this doc pins the tested head; business-logic
regressions are not replaced by clean scans.

Validation: PASS

## Tested head

`375e213` (SEC09-B) + this task's test file. Commit hash of this
task's merge into the record: see PROGRESS top entry.

## Validation (actual)

- **Migration fault/recovery** (new
  `sec09c_upgrade_test.go`, real PostGIS): 000060 applies alone over
  a 000059 world, exactly `["000060"]`; a live pre-upgrade family
  survives byte-for-byte; history writable immediately; chain replay
  empty. PASS (0.45 s).
- **Android immediate checks** (current head): `:domain:test`,
  `:application:test`, `:data:testDebugUnitTest`,
  `:app:testDebugUnitTest` + `:app:assembleDebug` (Hilt check) green.
- **Backend immediate checks** (current head): `go build ./...`,
  `go vet ./...`, `gofmt -l` (clean), `sqlc vet`,
  `git diff --check` clean, secret scan passed.
- ** inherits SEC09-A**: 71 unit packages + race integration across
  account/community/privacy/feedback/identity incl. real-HTTP suites
  and the MaxConns=2 saturation proof — all green at the parent head;
  this task adds the upgrade proof and the Android re-green above
  (domain/application changed since SEC07-C).

## Limits / owed

- Crash-after-commit coverage rides existing replay suites
  (helpfulness crash-replay, erasure-ledger replay, outbox lost-ACK);
  no new fault injection in this task.
- Edge Caddy test still env-blocked (SEC09-A record); phone matrix
  and soaks still owed. No production claim.

Next microtask: SEC09-D staging runbook and rollback.

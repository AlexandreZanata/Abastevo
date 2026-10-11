# CS10D — Training scope in account erasure

Date: 2026-10-11. Status: LOCAL_DONE. Scope: TRAINING erasure
scope only — consent revocation, derivative tombstones, immediate
byte purge, ledger row, replay coverage and composition wiring. No
soak claim (SOAK_RUNNING separately), no VPS/APK. Head: maintained
`dev` + this task.
Plan: [phased plan](../../../planning/COMMUNITY_SAFETY_EXECUTION_PLAN.md)
(CS10, AT02), [consent amendment](../../../security/AI_TRAINING_IMAGE_CONSENT_PLAN.md).
Rules: B-BR-CS06, B-BR-016, BUC-CS05.

Validation: PASS

## Scope (`TRAINING`, after `EVIDENCE` in execution order)

- `domain.ScopeTraining` joins `AllScopes` between `EVIDENCE` and
  `TRUST` (derivatives purge right after their source images);
  `NewLedgerEntry` accepts it. Migration `000058` evolves the
  ledger CHECK append-only (drop + re-add with TRAINING);
  existing rows unaffected, replay only replays recorded scopes.
- `application.EraseTrainingContributor`: consent ends REVOKED
  (past the stored revision, baseline when absent), every owned
  derivative flips to its tombstone, and every owned copy deletes
  immediately — erasure never waits for the 24 h sweeper.
  Re-runs converge to zero; byte failures error for the FAILED
  receipt and audited retry. Receipts stay for the restore filter
  in all cases.
- `runScopes` + `ReplayLedger` run the scope with its ledger row;
  report gains `TrainingRevoked`/`TrainingPurged`. New
  `ListDerivativesByOwner` query feeds erasure (sqlc regenerated,
  vet clean); owner key scopes the read.
- Composition: `adapters.TrainingErasure` (compile-time port
  assertion) wired into `cmd/worker` erasure ports (refusing
  objects when storage is unconfigured, same convention as
  erasure deletes) and `cmd/ops` replay (with operator-visible
  training counts and the same unconfigured-storage warning).

## Validation (actual)

- Unit: 3 new training-erasure tests (revoke-all + purge-all
  with selection stopped everywhere, empty + repeat convergence
  with baseline, byte-outage failure) plus scope-failure-marks-
  FAILED and 5→6 ledger assertions in the erasure suite. RED
  before GREEN (undefined ports/scope).
- Real PostGIS: the erasure journey now seeds a granted consent,
  an ACTIVE derivative and its namespace bytes (fake S3 speaking
  real SigV4 transfers) and proves consent REVOKED, derivative
  tombstoned, bytes gone, 6 ledger rows and converged repeat;
  restore replay re-stamps all 6 scopes. Full
  `./internal/modules/privacy/...` unit + integration suites
  green under `-race`.
- `go build ./...`, `go vet` (plain/integration), `gofmt -l`,
  `sqlc vet`, `git diff --check` clean; `scan-secrets.sh`
  passed. Only disposable local databases used.

## Limits / owed

- Soak SLO proof after 2026-10-12T16:11:11Z (SOAK_RUNNING,
  unchanged by this task — erasure tests never touch soak rows).
- Promotion queue producer (client review opt-in): CS11.
- Restricted DB roles: P08 provisioning.

Next microtask: soak SLO proof after the wall clock (then CS11 Settings/terms/UX).

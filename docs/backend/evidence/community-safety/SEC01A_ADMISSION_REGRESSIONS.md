# SEC01-A — Admission regressions (pending SEC01-C bounds)

Date: 2026-10-11. Status: LOCAL_DONE. Scope: six skipped
regression tests only — no behavior change (bounds land in
SEC01-C). Head: maintained `dev` + this task.
Plan: [remediation plan](../../../planning/SECURITY_REMEDIATION_PLAN_2026-10-11.md)
(SEC01, AUD-01). Constraint: free registration and social
participation stay free; floods cannot bypass aggregate
ceilings.

Validation: PASS (suites green; bodies proven failing unskipped)

## Regressions (all `t.Skip("SEC01-C: ...")`, repo Skip precedent)

Account (`account/application/admission_regression_test.go`):

- `TestDistinctAddressesExceedAggregateBudget`: 100 distinct
  `RequestCode` all accepted; asserts a future aggregate mail
  bound. Per-address quota exists — aggregate does not.
- `TestDistinctKeyUsernamesUnbounded`: 101 distinct key
  usernames accepted; asserts a future aggregate bound.
- `TestSessionsMultiplyWithoutBound`: 50 logins for one
  account accepted; asserts a future per-account session cap.
- `TestInvalidAddressCostsStoreAccess`: malformed addresses
  cost a store lookup before refusing (counting decorator);
  asserts zero store IO for invalid input.

Feedback (`feedback/application/admission_regression_test.go`):

- `TestSingleAccountCommentFloodUnbounded`: 101 comments for
  one account accepted; asserts a future per-account budget.
- `TestInvalidCommentCostsPipelineAccess`: oversized text
  costs authorize + station lookups before the text refusal
  (wrapped `StationExists` counter); asserts zero lookups.

Each body was verified to FAIL with skips stripped (true
regressions, not blessings), then restored skipped so CI stays
green. Bounds, production wiring changes and SMTP decisions
stay SEC01-B/C/D-owned.

## Validation (actual)

- `go test ./internal/modules/account/application/
  ./internal/modules/feedback/application/`: green with 6
  skips. `go vet`, `gofmt -l`, `git diff --check` clean;
  `scan-secrets.sh` passed (synthetic addresses only).

## Limits / owed

- Aggregate admission, invalid-first validation order, steady
  budgets, real-SQL flood/rollback/retry proofs: SEC01-B/C/D.
- Nothing here changes runtime behavior or claims a bound.

Next microtask: SEC01-B mailbox bound (or SEC01-C with it).

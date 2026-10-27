# DRV-10 — Signed commands, capability and reconciliation

Status: LOCAL_DONE
Validation: PASS (local application/storage; not HTTP, identity-provider or
commercial acceptance)
Integration: INTEGRATION_PENDING
Parent: fd176a8a. Behavior: 0c98c0b4. Contract: [DRV-10](../../../product/DRV_10_CONTRACT.md).

T01: `application.CommandService` requires a verified principal from the
`Authority` port and rejects missing/failed signatures, owner mismatch (IDOR),
foreign origin and missing contributor/key before storage. Hosted mutations also
recheck the live professional capability inside the same transaction through
`Store.SetPaid`; the check runs after the transaction's writes and on exact
replays. Reads and erasure require signed owner proof but not Premium, so
retained data remains reachable after a lapse. The real identity verifier,
subscription adapter composition and HTTP status mapping remain DRV-11/12.

T02: append-only migration 000136 adds versioned reconciliation decisions,
counterpart record references and operation receipts. SUMMARY_DETAIL requires a
counterpart and both DETAIL/SUMMARY coverage; PAYOUT_EARNINGS requires an opaque
payout source hash and never turns the payout into earnings. Referenced records
must belong to the owner, match platform/revision and fit inside the decision
window. Identical retries converge; changed bodies, stale versions and foreign/
stale records fail. Decisions share the earnings owner epoch and are removed by
owner erasure/soft account deletion. Report/import consumption is not claimed
here; DRV-11/19 must consume the decisions before asserting non-duplicated totals.

T03: real disposable PostGIS proves concurrent duplicate saves converge to one
receipt, stale adjustments are refused without a row, cancellation after writes
rolls back completely, and an old epoch cannot recreate a record or decision.
The existing owner advisory lock serializes driver mutations; no partial row was
observed in any failure path.

## Actual validation

Real owned disposable PostGIS (`abastevo-pr150-postgis-20261025`,
127.0.0.1:55439): **24 driver adapter tests under `-race` PASS**, including the
three new command tests:

- signed proof/owner/origin negatives, capability lapse with retained earning
  read, suspended-account write refusal and foreign vehicle refusal;
- reconciliation summary/detail and payout decisions, retry, changed body,
  stale version/revision, foreign record, missing coverage kind, owner erase and
  stale epoch;
- concurrent duplicate earning save, stale correction, cancellation after
  writes and stale-epoch recreation refusal.

The command package adds one top-level unit test group (five subtests); the
driver domain suite contributes 19 top-level tests. The `-race` run reports
**44 top-level PASS** (24 adapter + 19 domain + 1 application command group).

```
cd backend
go test -count=1 ./...                                   # PASS
ANPFUEL_TEST_DATABASE_URL=postgres://anpfuel:anpfuel@127.0.0.1:55439/anpfuel?sslmode=disable \
  go test -race -count=1 -v -tags=integration ./internal/modules/driver/...  # PASS, 44 checks
go vet ./... && go vet -tags=integration ./internal/modules/driver/...
GOTOOLCHAIN=go1.27.1 staticcheck ./internal/modules/driver/...
sqlc vet && sqlc diff && gofmt -l . && git diff --check  # PASS
```

Logs: `/tmp/drv10-final-integration.log`, `/tmp/drv10-final-unit.log`.

## Deliberate limits

No HTTP route, OpenAPI document, real identity-key verifier composition,
subscription-service wiring, mobile UI, cloud opt-in, deployment or production
claim. `SetPaid` is a transaction-bound port; the integration test uses a
synthetic capability decision, not a real purchase/provider proof. Reconciliation
decisions are persisted for future report/import consumption; no double-count
prevention in reports is claimed until that consumer exists. The inherited
global Android coverage gate is unchanged and not re-run here.

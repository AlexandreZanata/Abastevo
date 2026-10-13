# Premium next-ten execution evidence

Date: 2026-10-13. Branch codex/premium-backend; parent 8553b7a; PR131 to main. User authorized the next ten tasks: PRM01-T07, PRM02-T01..T05 and PRM03-T01..T04. One commit per task, no merge/deploy/mobile edits. Real provider/payment module remains deferred; paid production ports stay disabled. The occupied dev checkout is excluded.

## PRM01-T07 — Opening

Rules PR02–PR05: a missed notification must not permanently prevent canonical reconciliation. Select at most 25 indexed due purchases per worker tick (maximum source batch 100), checkpoint next attempt durably, serialize owner then token locks, enqueue digest-only refresh atomically. Five-minute due interval with worker minute ticks; only configured payment ports enable it. Canonical refresh, not scheduler output, grants rights. Existing durable queue provides retries/dead work and audited replay. Pending operation token material expires at 30 days; completed command/provider receipts at 90 days. Retired ownership fences are retained separately until an explicit audited ownership-retention migration; they are not counted as exported financial history. No indefinite cloud data retention or paid activation is claimed.

Status: LOCAL_DONE / INTEGRATION_PENDING (source)
Validation: PASS
Checks: subscription unit/race plus cmd/ops and worker compilation; real PostGIS TestReconciliation*, TestTerminalRetention*, TestSubscriptionUpgrade* (2.996s); affected vet/staticcheck; sqlc vet/diff; diff/secret review. Twelve parallel ticks produce one job; failed enqueue leaves due checkpoint intact; restart recovers; deleted owners excluded; bounded cleanup removes expired pending/terminal receipts and terminal purchase material while retaining opaque retirement fence; restricted operator replay refuses anonymous/nondead work. Real provider sandbox remains PROOF_OWED. No production paid scheduler runs with nil ports. Next PRM02-T01.

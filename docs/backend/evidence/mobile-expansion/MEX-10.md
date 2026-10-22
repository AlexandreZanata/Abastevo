# MEX-10 — Ledger/report reacceptance

Status: LOCAL_DONE
Validation: PASS

Parent: `597f5e95`, maintained dev. B-BR-MEX-03–06; BUC-MEX-02–04.
Before behavior: authoritative centavos remain raw; monthly category/vehicle
values are monetary sums, not occurrence counts. Only complete full-to-full,
same-segment, gap-free observations with all odometers produce consumption.
Corrections/deletions/gaps invalidate projections; erasure removes derived
private reports as well as raw values. Report/read/write concurrency must
serialize on the existing owner's ledger consent row, without paid gating of
free held-data reads/deletes. All supplied JSON must be consumed exactly.
Comparison must use its bounded projection port rather than a 1000-row request
to the public 101-row page. Truncation must refuse, never yield partial metrics.
No new ledger, dependency, billing activation or source/provider activation.


## Delivered and proved

Actual pre-fix PostGIS RED reproduced category counts, erased/stale reports,
missing odometer, ignored gap and ignored reset (six assertions). A second RED
proved that an unrelated future timestamp hid a deletion. GREEN fixes monetary
sums with checked addition, all-odometer/gap/segment eligibility, private cached
projection version2 and SHA256 of ordered record revisions/gap/segment identities.
No wall-clock maximum decides freshness. Old cached methods refuse/rebuild
without changing public DTOs or raw entries. EraseAll clears derived reports.
All commands, reports, gap/reset changes and erasure share an owner row lock.

Observable real PostGIS writer waits prove correction cannot pass a report
transaction; both commit and cancellation release it, with exact corrected totals
and no partial publication. The dedicated consumption projection accepts up to
1000 complete rows, refuses limit+1, filters void/deleted/foreign rows and carries
private gap/segment evidence through root wiring. The former production root
asked a public max101-row reader for1000 and therefore could never compare.
The integration journey used a masking100-row clamp; it now uses the same
complete projection as production. Unimplemented matched-window analysis refuses
rather than returning a fuel-only result under that name. Strict JSON EOF and
arithmetic overflow negatives are covered; no schema/dependency was added.

Validation on the final inputs: `go test -race -count=1 -tags=integration`
ledger/all, metrics/all, cloudsync/adapters and cmd/api PASS, including existing
correction/conflict/idempotency, timezone/leap/DST/half-open month, raw export,
erasure/lapse and actual signed private journey cases. Pure frozen financial
vectors remain green. `go vet`, staticcheck(v0.8.1 analysis-only Go1.27.1),
`sqlc vet/diff`, API contract race, gofmt, diff-check and changed-file secret
review PASS. Runtime/test toolchain1.27.3; no lowered thresholds or skipped tests.
Logs: `/tmp/bfp-mex10-{red,clock-red,green,acceptance,final,journey,contracts}.log`.
Native/app inputs unchanged since the reconciled v18 acceptance. Source/provider
activation, broad manual and production certification remain separate.

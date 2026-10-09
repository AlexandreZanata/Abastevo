# P37-T02 visible submission feedback and scoped recovery

B-BR-PC04 / BUC-PC04 refinement, 2026-10-08: a durable local enqueue is not
server receipt or validation. Immediately show a visible confirmation after
enqueue, state that sending continues automatically, and offer return to the
community without waiting for validation. A matching server RECEIVED/VALIDATING
receipt is truthful sent feedback; VALIDATED remains the separate accepted fact.
Offline/retrying commands must never show sent. Rejections and partial outcomes
must remain explicit. Reopening the review must recover the same command IDs,
and no notification, retry or return action may create a duplicate submission.

Connected POCO diagnosis: five local commands were retrying without a receipt;
the signed upload status returned VERIFYING, proving server connectivity. The
matching staging verify-evidence job exhausted its five attempts with a duplicate
key conflict before the existing content convergence correction was deployed.
Current API/worker already contain that correction; old DEAD jobs are not
automatically replayed. Recovery is restricted to the job for the current
capture, through the existing audited ReplayDead operation, after immediate
real PostGIS recovery/convergence tests. No other stuck session is in scope.

Status: LOCAL_DONE
Validation: PASS

Source parent: `1896cf2`; behavior fingerprint (sorted fourteen changed app
source/resource/test paths, NUL separator and bytes):
`7d6c7548d8dc1312bbe1b6141fe19336620eb1c91bf2d4bdcd65e1e5db656aa7`.

- RED: restored RECEIVED feedback regression failed before the change.
- GREEN: 246 app unit cases, zero failures/errors/skips. Immediate receipt,
  retrying/missing/foreign owner states and no duplicate enqueue are covered.
- POCO API36: two native dialog cases passed (`OK (2 tests)`, 4.099s), including
  queued/retrying/received-pending-validation, partial refusal and visible return
  without scrolling. APKs compiled; debug lint has zero errors. Static mobile,
  secret scan and diff checks passed. Corrected app and clean test APK installed
  with `-r`; no account/data wipe.
- Existing backend convergence and replay were tested immediately with real
  disposable PostGIS databases: `go test -race -count=1 -tags=integration
  ./internal/platform/jobs ./internal/modules/evidence/adapters` passed against
  the local PostGIS test endpoint. No backend source/deployment change.

Authorized runtime recovery: the guarded first check found the matching
quarantine object missing (404), so replay refused before mutation. The owner
explicitly authorized restoring the same queued photo and only its job. Native
Keystore recovery checked the original hash, 231,563-byte size and unchanged
retention deadline without exporting keys. A bounded staging-only dry run
verified exactly one eligible job and original bytes. Exact-key restoration and
existing audited ReplayDead then succeeded. Subsequent sanitized counts show
READY sessions 5→6 and DEAD jobs 3→2; the recovered job is DONE on its first
attempt. The other two old jobs remain untouched. Temporary server photo and
operator binary were removed after verification; no new receipt/session or
retention extension was created. Android still had five retrying commands at
this checkpoint; their existing WorkManager backoff is retained, so final mobile
server receipts remain pending and must not be reported as sent yet.

Return navigation is available immediately after local enqueue. All owner-scoped
RECEIVED/VALIDATING receipts count as sent; acceptance remains separate. Polling
reads the local outbox immediately and every two seconds for at most ten minutes,
without new network polling. Returning does not cancel durable upload workers.

No new photo submission, backend deployment, secret/quota change, public pilot
or remote Git publication follows from this repair. Private diagnostics remain
local; no photo, raw database/log export, credential or contributor GPS enters Git.

# FQ-09 — Signed structured reports

Status: LOCAL_DONE
Validation: PASS
Integration: INTEGRATION_PENDING.

Behavior contract before implementation: B-BR-FQ-07/08/09/14/15 and
BUC-FQ-05/08. Account plus current signed contributor binding is mandatory for
all writes and private reads. The server derives ownership; no owner field is
accepted. Report commands carry a UUID idempotency key and optimistic revision.
One command ID cannot be reused with different content. Relating a report to
a PMQC sample requires the exact published revision, station and fuel; expired
revision associations fail closed. Reports remain private pending moderation.

The first document-free slice accepts EXPERIENCE and LINK_REVIEW, date-only
incident time, a closed symptom code (STARTING, PERFORMANCE, CONSUMPTION, OTHER)
and up to 1,000 Unicode text characters with no invalid encoding/control bytes.
These are resource bounds and user claims, never a chemical diagnosis. Dates
cannot be in the future; no invented maximum age. Reports contain no protocol,
receipt, media, GPS, checked-purchase state or business privilege fields.
LINK_REVIEW requires a sample target; event linkage remains refused until its
station/fuel attribution contract is accepted. Edits preserve the target and
require the current revision. Withdrawal clears text and leaves a minimal
command receipt/audit. New/edited reports atomically enter restricted review.

Immediate acceptance: replay/content conflict, optimistic edit race, ownership,
guest/invalid/revoked/unbound proof, forged/stale target, rollback of failed
review intent, account deletion and no public disclosure. D scope remains held.

Validation (local disposable PostGIS, never staging):

- Domain RED→GREEN: invalid IDs, target pairing, future/invalid dates, UTF-8,
  controls, text bounds, unsupported symptoms/checked-purchase claims, edit and
  withdrawal envelopes and canonical payload binding.
- `go test -race -count=1 -tags=integration ./internal/modules/quality/...
  ./internal/platform/migrate ./internal/modules/moderation/domain
  ./internal/modules/account/adapters ./internal/modules/identity/adapters`:
  all ten affected packages PASS. New report SQL tests: six top-level cases
  (eight concurrent replays, one of eight optimistic edits, IDOR, changed
  idempotency payload, stale/forged official target, failed queue transaction,
  suspended/deleted account, revoked/rotated family/key, unbinding, concurrent
  account deletion lock, private erasure/replay). Existing chain/recovery,
  quality publication/read/jobs and account/identity regression suites PASS.
- HTTP: eight auth/body/intake cases and both signed owner-read routes PASS;
  exact bytes restored for signature verification, forbidden owner/document
  fields rejected, no-store and credential/owner-error leakage assertions.
- Worker: real SQL review outage/replay convergence, stale/withdrawn/erased
  report refusal and unknown payload version PASS. Job payloads contain only
  report ID/revision, never body, owner or credentials.
- API/worker build, affected vet/staticcheck, sqlc generate/vet and OpenAPI
  contract vectors PASS. Fixed the prior FQ-08 OpenAPI 3.0 `nullable` spelling
  to 3.1 unions; strict vacuum now reports zero errors without relaxing rules. Removed the unused prior publisher UUID helper found
  by staticcheck; injected production ID generation remains unchanged.
- gofmt, `git diff --check` and scoped tracked/new-file secret review PASS.

Recovery: `ANPFUEL_QUALITY_REPORTS_ENABLED=false` stops new/edited intake while
preserving signed owner reads and withdrawal. The existing account-erasure
worker also removes bounded report/command batches; anonymous export, media,
training or billing never grants report access. Detailed FQ-16 export/restore
acceptance remains separate; reviewed visibility and operator action semantics
are owned by FQ-11. No new migration/deployment was applied to staging here.
FQ-10 remains DEFERRED_FUTURE, and no purchase/document state was implemented.

Newly verified official inputs revealed an FQ-02 native-layout gap (actual
19-column PMQC versus internal eight-column contract) and sample Aspecto rules.
Resolve this compatibility prerequisite before FQ-12/live import. Retain the
existing internal format as a versioned prepared contract, not a native claim.

# F1 EMP-PLAT-010 transactional enterprise events

Status: LOCAL_DONE
Validation: PASS

Date 2026-11-11. Scoped task validation passes. Delivery is
INTEGRATION_PENDING through [PR202](https://github.com/AlexandreZanata/Abastevo/pull/202).
F1/M1 remain open.

## Entry and behavior

The owner authorized the next bounded enterprise task and GitHub delivery.
Started from clean `codex/enterprise-f0-f1` at `3f7a25b8`, matching
origin/dev and PR202, with main `ef96cd13` already an ancestor. The occupied
local dev checkout and unrelated primary-checkout work are preserved.
Claude's corrections and activation obligations in the
[review record](f1-pr202-review.md) remain intact.

B-BR-EMP-23, B-BR-EMP-24 and BUC-EMP-13 were documented before behavior.
The new orgevent module uses the existing orgaudit application runner,
WithOrgTx, PostgreSQL queue and Dispatcher. No dependency, queue service,
scheduler, business workflow, provider call or runtime activation is added.

Source modules register event/target/consumer codes and check current write
authority before the producer. Service.Record is default off. Its callback
shares the audit transaction; source change, audit, immutable intent and all
required consumer jobs commit together. Event identity and target match the
original audit operation/target, with a positive source revision. The digest
binds the original command, organization, event metadata and canonical sorted
fan-out. Identical command replay skips work/jobs; changed intent is reuse.
Pre-existing dedupe rows with unknown provenance cannot be adopted.

The flat version-1 payload contains eight typed reference/code/revision
fields only. Strict decoding rejects duplicate/aliased/unknown/missing/null
fields, fractional revision, oversize and trailing JSON. Catalog validation
rejects unknown event/target/consumer/version, malformed/noncanonical/nil
UUIDs and invalid revisions. Event/envelope default and structured logging
redact private references. SQL/provider/callback errors are mapped to static
errors before the existing Dispatcher can persist last_error.

NewHandler requires a database-only effect for each registered consumer.
The owning module registers the returned jobs.Handler with the existing
Dispatcher. Each delivery opens the original ordinary-login scoped
transaction, locks the exact claimed queue row and checks token, worker,
expiry, payload, dedupe, kind and attempt budget. The immutable intent binds
the job ID and matching original audit digest/metadata. Effect and receipt
commit together under (org_id,event_id,consumer). Receipt replay skips the
effect; queue acknowledgement remains outside this transaction.

Lease validity is checked after a lock wait and after the effect. An expired
or superseded lease, callback failure, cancellation or failed commit leaves
no effect/receipt. A locked job cannot be superseded mid-effect by another
claim. The existing five-claim budget is enforced even after crash/expiry
loops; the next claim is parked without another effect. Failure retry keeps
the existing fixed minute, and dead replay uses the existing reason-required
queue API. Operator authorization/audit UI and external delivery retain
their own tasks. No external operation is allowed inside these callbacks.

## Migration and compatibility

Additive migration 000168 creates forced-RLS append-only intent/receipt
tables. Intent has the composite original-audit foreign key; receipt has
the composite intent foreign key. The exact job ID is immutable intent
metadata, without imposing deletion/retention changes on the existing queue.
No activation or work is seeded. Existing migrations and compatibility
floor `000132` are unchanged. Rollback stops producer/handler composition
and preserves expanded schema, jobs, original facts and deduplication data.

The audit regression initially failed because the new foreign key prevents
plain TRUNCATE before its immutable trigger is reached. The test now grants
the related truncate permissions and uses CASCADE, still requiring the exact
55000 append-only trigger error. No rejection assertion was weakened.

Migration recovery tests populate the previous chain through 000167 with
catalog and job rows, inject a failure after the new DDL and verify absence
of both tables, function and migration ledger entry. Retry and repeated apply
converge; old rows survive and the previous binary's schema chain remains
compatible.

The actual previous API was built from `git archive 3f7a25b8 backend`
with `-buildvcs=false`. Binary SHA-256:
`0dda45a2977b5393879f3c9d4802d7308817e060f36085ca14a83925c547357a`.
Against the current chain through 000168 in a separate disposable database,
it returned HTTP 200 `{"status":"ready"}` and stopped with exit 0.
Inherited ANPFUEL configuration was cleared and discovery disabled.

## Executed verification

Linux amd64, Go 1.27.4, sqlc 1.31.1, pinned staticcheck v0.8.1 with the
documented go1.27.1 compiler workaround. Real PostGIS 3.6 / PostgreSQL 18
ran in the unique disposable Compose project
`enterprise-plat010-20261111`, using tmpfs and loopback-only exposure.
Fixtures create their own databases and ordinary non-owner/non-BYPASSRLS
runtime logins. Concurrent cases use a separate 12-connection runtime pool.
No staging/production/provider credentials, receipts or private data are used.

- RED: initial domain tests failed with no implementation. GREEN: catalog,
  plan/digest, immutable registration and privacy cases pass.
- `go test -race -tags=integration -coverprofile=/tmp/enterprise-plat010-coverage.out ./internal/modules/orgevent/... ./internal/modules/orgaudit/... -count=1`
  passed. New coverage: domain 98.1%, application 92.6%, adapters 93.0%.
- Real publication tests prove identical replay, changed fan-out/revision
  denial, colliding event IDs in two organizations and twelve concurrent
  identical commands with one source change/job.
- Failure injection covers business callback, queue admission, second
  consumer, intent insert and deferred audit commit. All roll back source,
  audit, intents and jobs, then recover on the same operation.
- Actual queue claims prove acknowledgement loss, changed token/stale ack,
  twelve concurrent deliveries with one effect, receipt/commit failure,
  cancellation, expiry during effect and recovery. A barrier retains an
  executing effect across actual database-clock expiry; a competing claim
  skips the locked row, the original rolls back and a new token succeeds.
- Payload tampering includes stored queue changes as well as envelope
  changes, so checks prove immutable intent binding rather than trusting
  the claim payload alone. Foreign organization, target, revision and
  registered-but-wrong consumer apply no effect. Both forced-RLS tables
  hide unscoped/foreign rows and reject mutation.
- The real Dispatcher exhausts five failures, persists only a static error,
  refuses automatic dead reclaim and recovers via explicit dead replay.
  Five abandoned leases also exhaust the effect budget without a receipt.
- `go test -race -tags=integration ./internal/modules/orgaudit/... ./internal/platform/jobs ./internal/platform/database ./internal/platform/migrate ./internal/modules/kernel -count=1`
  ran. Jobs, database, migrate and kernel passed; the audit truncate
  regression above was corrected and its full package rerun passed.
- Affected `go vet -tags=integration` and pinned staticcheck passed.
  `sqlc generate`, `sqlc vet`, `sqlc diff` and API/worker/ops/migrator builds
  passed. The three behavioral packages join the quick manifest.
- Enterprise verifier/generator regressions, explicit gofmt, relative-link,
  whitespace and changed/new-file secret reviews passed. Plan counts stay
  348 tasks, 545 rules, 167 use cases, 145 screens, zero structural problems.

Tested implementation blobs, in order:

- Migration: `97105f3df695f9eaccc066497cefb97402202838`.
- Domain: `abc7882bedbecfbf3f949d767955e775045378d7`.
- Application: `680bcf1515b5c40755af955a0623efe6ec4c0897`.
- Store: `c1c70bafadd32179601890161962aedb6a146130`.

Source, SQL, tests and evidence travel in the same atomic task commit.

## Retained obligations

No concrete business catalog, effect or production worker registration is
activated. Each owning module must compose its current authority/state
checks and all required consumers before enabling a producer. The existing
ordinary runtime-role/grant and backup/restore gates remain mandatory.
No retention purge is added: receipts/intents must survive every supported
replay window; retention changes require their owning review.

Claude's AUTH, single-family revocation, transfer limits, private-original
bucket lifecycle, composed route gate and other activation obligations stay
open. The unavailable online npm advisory audit and Rust worker integration
environment are not reclassified. The pre-existing aggregate Kotlin coverage
failure remains FAILED/PENDING as recorded in
[EMP-PLAT-009 evidence](f1-plat009-exact-values.md); no Kotlin source changed
or aggregate test was repeated here. No fully green aggregate or merge
readiness is claimed.

F1/M1 remain open. Next bounded task is EMP-PLAT-011 upon authorization.
New remote checks/review must apply to the new PR head. No merge, deployment,
wiki sync or release certification is performed or inferred.

## Second PR202 review corrections (2026-11-11)

`SafeError` turned every work-callback refusal into a retryable 503. It now
returns a registered orgapi code (CONFLICT, VALIDATION_FAILED,
PERIOD_LOCKED, ...) bare, after the rollback, and keeps every other error
opaque. The append-only tests claimed more than they proved: the audit
TRUNCATE accepted the event tables' 55000, and intent TRUNCATE failed on its
receipt FK first. Each table's trigger is now proved alone, with the other
triggers disabled and the exact message required. The statement above that
no rejection assertion was weakened did not hold for the audit TRUNCATE.
[Review record](f1-pr202-review.md#second-review-head-1f99b018).

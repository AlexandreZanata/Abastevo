# EMP-OFFL-002 — Scoped server operation intake

Date: 2026-11-13.
Status: LOCAL_DONE
Validation: PASS
Integration: INTEGRATION_PENDING

Continue on owned codex/enterprise-f2 from tested/published PRIV-003 51c25146.
That checkpoint's source/host/actual API36 emulator acceptance is complete and
backed up at origin/codex/enterprise-f2. Its cumulative PR203 dev head is still
the earlier batch; final phase publication remains dev → main. No merge,
deployment, wiki or CI wait is introduced. The owned emulator was shut down
after acceptance; its AVD/private synthetic credential remain available for the
next Android task, with no other AVD/device/application changed.

OFFL-001, AUTH-003 and PLAT-010 are local checkpoints. The source task is the
intake/ledger boundary, not feature business logic or F4 late-intake. Read-only
protocol, SQL/privacy and HTTP/auth audits preceded B-BR-EMP-108..114,
BUC-EMP-56/57 and EDR-071..075. Those rules are recorded before behavior.

## Frozen decisions

- Reuse complete Candidate.Parse with its captured limits for every item;
  compare all immutable ownership tuples before any intake write/budget. No
  partial-correlation parser or changed frozen wire is needed.
- Use separate claims, immutable final ACKs and real applied-effect readiness.
  Missing owning producer/history/review/admission/follow-up is unavailable.
  Authorized dependency waiting can retain a claim without a final ACK/effect.
  EDR-075 requires owning current parent-record coverage; an explicit coverage
  refusal rolls back the operation and returns the same DEPENDENCY_PENDING as
  a missing parent, without a new claim. Covered cross-device parents are valid.
- Each operation owns an ordinary READ COMMITTED scoped transaction. Source
  proof/record/role/capability and DB time are rechecked after waits/projection.
  Earlier commits survive later failure; a late source denial withholds private
  response data without pretending earlier effects rolled back.
  Unverifiable current source also withholds the response using bounded503;
  ordinary feature/storage unavailability remains an independent item retry.
- Replay requires current original-owner/member/enrollment and owning read
  coverage. Never substitute an old write decision, claimed digest or current
  declaration for source authority. Revalidate stored ACK bytes before exposure.
- Audit/outbox target offl.operation/opID once; receipt revision1 is not a
  business revision. Borrow existing transaction-aware audit/writer/limiter.
  Meter max(1,N) operations through explicit configured opaque axes; no invented
  permission, unmeasured runtime budget or probe/no-op production producer.
  EDR-074 maps the logical needs-review follow-up to offl.operation.review,
  preserving applied migrations165/168 identifier checks and frozen wire codes.
- Migration196 is additive and preserves previous-image compatibility/minimum
  floor. Keep company provenance through existing account-deletion unlink;
  no raw business payload/token/family/signature, new live-account FK, personal
  export or arbitrary receipt-pruning TTL is introduced.

## Immediate acceptance

Capture REDs for malformed trailing item/no prefix, unsupported installed
schema independence, changed intent, retry then acceptance, review not ready,
soft orphan with actual held data, twelve-way duplicates/target contention,
failure after effect/audit/outbox/ACK and observed source/role/proof expiry waits.
Use actual ordinary-role PostGIS, signatures/nonces/current source and normal
route/gate/inventory, plus populated195→196 failure/recovery/noop, retained
history on personal deletion and previous-image compatibility. No known failure
or missing critical check is deferred to the final phase aggregate.

## Construction observations

The actual signed normal-router RED used an ordinary non-bypass PostGIS source:
the newly specified path returned404 where unavailable503 is required. Log:
/tmp/enterprise-offl002-http-red.log. The test's initial helper also used its
parent testing context from a subtest; this diagnostic-only defect was corrected
by passing the subtest context explicitly, preserving every assertion.

The transport response is the source-owned EnterpriseSyncBatchResponse wrapper:
schema_version1 plus complete canonical acknowledgements in request order.
EDR-074 records the inspected165/168 identifier constraint and logical review
event mapping before production catalog behavior. No historical migration edits
or frozen result/code changes are authorized by that mapping.

The three initial real signed HTTP/race cases pass3.655s after core construction:
unconfigured availability and exclusive source boundary; independent six-item
dependency/malformed/accepted/held/schema outcomes, immutable resend/reuse and
six actual SQL Dispatcher follow-ups; and a real runtime family revocation
observed waiting behind the committed prefix's source locks. The latter refuses
the buffered response after an unregistered suffix while preserving the prefix.
Log: /tmp/enterprise-offl002-http-green.log. Subsequent source fixes require
affected revalidation before this task can close.

Independent review also captured and corrected real counterexamples before
closure:

- Fresh received_at is constructed after the audit/outbox waits, then authority
  is confirmed again after ACK/effect projection and before commit.
- A canonical, schema-valid corrupted ACK cannot masquerade as replay: compare
  its complete typed receipt mirrors and bound audit/source metadata, as well
  as exact original canonical bytes. Do not repair retained data into success.
- Typed-nil providers fail closed before source work/claims. A fatal current
  source refusal outranks a joined/wrapped dependency retry. Only a private
  rollback marker from the owning dependency guard can become the nonfinal
  pending response; unrelated Apply/writer failures cannot impersonate it.
- PostgreSQL own-claim row locking needs only UPDATE(operation_id), in addition
  to SELECT/INSERT. The statement-level append-only guards still reject every
  update/delete/truncate, including no-op statements. No runtime grant is
  installed by migration196.

Completed scoped checks so far: all260 frozen vectors retained, protocol race
93.9% and application13 tests/race93.5%, vet/static/format pass; actual weighted
budget/race3.069s and joined current-source/authority2.602s pass; populated
ordinary-role migration failure/recovery/noop/previous embedded chain plus
personal erasure/export boundary pass2 cases/3.195s. Full affected cmd/api
race regression passes18.477s. Existing seven affected transport packages,
focused route gate/inventory, vacuum, integration-tag vet/static and plan check
pass. No dependency or Android source change requires a new device run here.

The complete API-contract run found one stale OFFL-001 component-only assertion
after334.471s: it prohibited every organization sync route. Preserve all shared
payload/hash/ACK checks and allow only the newly delivered exact route, still
rejecting all other organization sync paths. Focused proof and the complete
affected rerun passed after that correction: isolated legacy RED6.943s,
legacy/new-boundary GREEN9.729s and complete apicontract race327.733s. Logs:
/tmp/enterprise-offl002-apicontract-full-final-race.log and
/tmp/enterprise-offl002-cmdapi-full-race.log. The failed334.471s run remains a
diagnostic, not acceptance. No further API aggregate is needed without changed
inputs or another failure.

The completed ordinary PostGIS matrix has13 top-level/30 named passes,
25 leaf cases and zero failures/skips in30.811s, including the two migration/
privacy cases. Its authoritative artifact is
/tmp/enterprise-offl002-sql-final.jsonl, SHA256
7f4820adf243076fcd1b5ffcea3189b1d91f9a5b2e8083538998a705a3c04777.
An initial /tmp log-name collision was corrected: use this SQL filename and
the separate core approved-final log for protocol/application evidence.
The deferred commit fault proves all six projections and final source proof
were reached before commit fails, then checks target/audit/receipt/effect/
outbox/job rollback and recovery. Parent visibility tests prove SELF denial,
indistinguishable missing/hidden dependency retries, actual held soft orphan
facts without a linked parent and a covered other-member/device effect.

## Final acceptance and source

The final affected ordinary PostGIS/race union passes24 top-level/46 named cases,
40 leaves, zero failures/skips. Package times: ledger schema1.975s,
organization/source/actual signed HTTP32.735s and transport quota3.723s.
Artifact: /tmp/enterprise-offl002-union-final.jsonl with its derived
enterprise-offl002-union-final.summary.json. The same command compiled the
transport-only HTTP package without matching its names; its three actual
transport unit tests were separately run in full and pass with race, recorded
in /tmp/enterprise-offl002-http-unit-final.jsonl. No compile-only result is
counted as a behavioral test.

The new populated isolation test independently passes2.228s and is included
in the final union. Same account/key and deliberately colliding operation and
target UUIDs have separate real memberships/enrollments in two organizations.
All three ledgers refuse foreign and unscoped reads. A foreign-only effect
does not release a dependency. Actual other enrollment/key and other member/
account cannot adopt an original receipt; current SELF/read revocation refuses
replay even while the original signed device proof remains active. Restored
actual read coverage recovers the same original bytes without rewriting data.

The actual signed route also proves full-body failures spend no intake budget,
empty bodies spend one unit on all three opaque axes, every replay is metered,
nonce reuse is refused even after unavailable intake, and a modified signed
body is refused. Mixed malformed/schema/accepted/held/dependency outcomes
remain independent. A26-year delivery delay does not produce CLOCK_SKEW.
Six actual SQL Dispatcher effects occur once through lost-answer recovery;
the current-source revocation barrier withholds the entire private response
after a committed prefix and unknown suffix.

Reproducible scoped commands from backend/ with pinned Go1.27.4,
GOCACHE=/tmp/enterprise-go-build and the owned disposable PostGIS3.6 DSN:

```sh
go test -race ./internal/modules/orgsync/protocol ./internal/modules/orgsync/application -count=1
go test -race -tags=integration -json -timeout=10m ./internal/modules/orgsync/adapters ./internal/modules/organization/adapters ./internal/platform/orgquota ./internal/modules/orgsync/adapters/http -run '^Test(Intake|SyncIntake|OrganizationSync|SyncCurrentDevice|SyncAuthority)' -count=1
go test -race ./internal/modules/orgsync/adapters/http -count=1
go test -race ./internal/platform/apicontract -timeout=15m -count=1
go test -race ./cmd/api -timeout=15m -count=1
sqlc vet
sqlc generate
```

Generation stays byte-identical in all three new generated files. Integration-
tag vet/static, pinned gofmt, vacuum, all route/inventory guards, plan consistency,
staged secret scan and diff checks pass. Staticcheck uses the already compatible
Go1.27.1 toolchain and /tmp cache; no dependency, migration rewrite or personal
database schema change is introduced. Deliberate corruption fixtures alter
only their disposable admin-owned row and restore its immutable trigger before
the ordinary runtime replay; normal acceptance never bypasses its constraints.

The [frozen source manifest](f2-offl002-source-manifest.sha256) has42 files,
rooted at the worktree and based on51c25146. It includes changed code,
contracts/tests and the two unchanged260-vector corpora. Manifest SHA256:
4662130f78a9039eb1f9cc84a930ae252418ec9e04080eb43efc94567ffce0af.
No required local check or known failure is
deferred. Protected PR/CI integration remains pending; no construction polling,
merge, deployment, wiki or real pilot occurred.

Next: atomic commit/owned branch backup, then EMP-OFFL-003's durable Android
queue on the accepted protected store. The user authorizes the owned emulator
for native tests. Production registry still has no business handlers or measured
budget until actual owning integrations arrive. C1/M1/M2, managed/provider,
legal/staging and release gates stay distinct. Six local checkpoints do not
close the remaining58 audited nodes or the full F2.

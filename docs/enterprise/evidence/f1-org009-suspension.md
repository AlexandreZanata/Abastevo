# F1 operator suspension and reactivation — EMP-ORG-009

Status: LOCAL_DONE / INTEGRATION_PENDING
Validation: PASS
Date: 2026-11-12
Source parent: main/dev `6a19bcc799f958adbf9252e956ec6f66ec2bca5a`, the actual
merge of PR202 after Claude's corrections through `69d02109`.
Owned worktree: `.worktrees/enterprise-suite-execution`; isolated local branch
`codex/enterprise-org009`, publishing to maintained remote dev. Other occupied
checkouts are preserved. F1/M1, external activation and release gates stay open.

Tested backend tree: `487c9d9003ea055b7ec8b622d319dad8715f01ad`.
Tested contract Git blob: `e69bc7b7f4a25a4277c5c9e444e7399cb4ec048c`.
The enclosing task commit pins these runtime/test inputs; delivery documentation
does not change them.

## Contract and authority

B-BR-EMP-69/70 and BUC-EMP-35 precede behavior. EDR-045 records the existing
ADR-023 administrative credential plane as the actual platform-operator source.
Company OWNER/ADMIN roles and public staff badges confer no operator power.
Subscription automation remains its owner's work; this task adds no billing
provider, credential or automatic cause resolution.

Two POST routes require both the enterprise flag and configured administrative
surface, with the dedicated admin cookie, exact configured administrative Origin,
X-Admin-CSRF, Idempotency-Key UUID, schema version, root revision, reason category
and a 10–500-character reason:

- `/v1/admin/organizations/{org_id}/suspend`
- `/v1/admin/organizations/{org_id}/reactivate`

New commands and stored retries recheck actual DB session liveness and a fresh
15-minute window. The existing admin mutation helper adds the explicit
`MutateFresh` transaction port while preserving existing callers. It locks the
session before root work and rechecks after root waits. Its mutation/replay
transactions use READ COMMITTED, matching Claude's integrated organization
transaction correction. Key configuration/rotation, cookie and CSRF remain the
existing administrative HTTP source; no account identity is fabricated for an
operator audit.

The new routes reject ambiguous security headers/cookies, Authorization bearer
substitution, query strings and unexpected/private authority fields. A RED
transport test reproduced acceptance of duplicate Origin; the route now refuses
it. Missing idempotency remains the existing 400 protocol refusal. Errors are
static and responses are no-store without ETag.

## Persistence and lifecycle

Additive migration `000184_organization_suspensions.sql` adds two forced-RLS
scoped tables: retained suspension periods and durable pending owner notices.
At most one period is open per organization. Start identity/cause/time are
immutable; resolution closes once, retaining actual operator sessions and root
revisions. Invoker guards pin their search path, validate scope/root/current
proof, reject regressed clocks, and prevent deletion/truncation/history rewrite.
No prior migration, unscoped inventory, RLS exception, runtime login, external
service or dependency is introduced.

Only ACTIVE roots become SUSPENDED. Reactivation resolves the matching cause
and returns SUSPENDED to ACTIVE. On ARCHIVED roots, opening/resolving a cause
keeps ARCHIVED. An open cause cannot be silently replaced; stale revisions and
mismatched causes conflict. PENDING_SETUP and CLOSED roots cannot be suspended.
Metadata anchors and unknown legacy suspension history are not assigned invented
periods or operator provenance.

Root state/revision, period, `CAUSE_OPENED`/`CAUSE_RESOLVED` pending owner notice,
admin audit and original acknowledgement commit atomically. Notices say
`PENDING_NOTIFICATION_CORE`, never delivered. Their cause-based names also avoid
claiming that an archived root was reopened. Notification delivery remains
NOTIF-001. Runtime grants still require the actual reviewed ordinary-role
inventory; local fixture ACLs do not provision staging or production.

Exact retries preserve their original body/time/period even after later
reactivation. Changed intent conflicts. A revoked or no-longer-fresh operator
cannot replay. A late failure returns unavailable with no provisional body;
root, history, notices, audit and idempotency all roll back.

## Write gate and field work

Existing current authorization and company-store guards continue to block
configuration/operational writes in SUSPENDED. Authorized reads, exports,
access reductions and plan recovery retain their existing scoped effects;
reactivation creates no role/device or retrospective permission. Actual company
settings tests prove reads remain available and writes stop/resume with state.
No future export, device, subscription or employee-management endpoint is claimed
implemented by this task.

The explicit owning-transaction assessment port requires an ordinary scoped
READ COMMITTED transaction, locks the root and captures server receipt/history.
It classifies only the suspension condition; the business owner still verifies
device, enrollment/revocation, membership/permission, schema, replay, dependencies
and its own intake window. No public intake bypass is mounted.

A server-recorded entity opened strictly before suspension can finish for
review. A fact declared strictly before suspension and received within seven
days (inclusive) is review-only. New work during suspension is refused. Closed
intervals retain review implications after reactivation, including facts declared
during a now-resolved interval; no held record is promoted automatically.
Invalid/future/overlapping/unproven server history fails closed, and at most 100
relevant periods are assessed; a larger result is unavailable rather than
silently truncated. Device time never proves authority. Synthetic owning-record
SQL tests persist the classification and prove reactivation preserves REVIEW;
they are not a claim that the later OFFL/trip/fuel/expense ingress exists.

## Validation

Go 1.27.4; sqlc 1.31.1; staticcheck 0.8.1 with Go 1.27.1; vacuum 0.30.6.
Real tests use the owned disposable local PG18/PostGIS 3.6 fixture. No persistent
or deployed environment is targeted.

- Domain RED → GREEN covers archived causes, open/mismatched/stale/terminal
  changes; grace boundaries, existing completion, historical review and malformed
  history/server-clock cases. Composed organization domain coverage: 91.84%.
- Twelve competing CAS requests admit one change; twelve identical retries
  preserve one resolution/result. Actual owning intake holds the root lock
  through commit and serializes with the operator change.
- Direct ordinary-runtime SQL negatives prove period/cause/time immutability,
  notice immutability, deletion/truncation refusal, fake operator rejection and
  refusal to invent a legacy missing period. These additional cases pass after
  the aggregate run without rerunning its unchanged runtime/contract inputs.
- Real operator login/proof, revoked/stale replay, changed intent, late deferred
  failure/rollback/recovery, metadata/foreign scope and GUC restoration pass.
- Composed HTTP tests cover genuine admin credentials, company-cookie/bearer
  refusal, CSRF/origin/header ambiguity, strict DTOs, idempotency, actual company
  read/write gates and default-off combinations.
- Populated additive upgrade as an ordinary non-login/NOBYPASSRLS owner reaches
  an injected failure, rolls back completely, recovers, preserves prior rows,
  repeats without changes and passes previous-image schema compatibility.
  Actual upgraded operator commands run through the separate ordinary runtime.
- Full affected organization, admin, orgaccess, membership, companysession,
  orgapi, enterprisesuite and OpenAPI real/race suites pass. Full OpenAPI suite
  took 404.8 seconds with composed coverage. The final transport correction is
  additionally verified by the real HTTP test, HTTP unit package and full
  cmd/api unit suite, including actual production operator and company gates.
- OpenAPI carries strict schemas/examples and the actual administrative guard
  metadata. The dedicated operator route gate rejects cookie/fresh/header/raw-
  handler/missing-route drift, separate from company AUTH.
- Scoped vet/staticcheck/build, sqlc vet/diff pass. Canonical vacuum reports zero
  errors, 799 warnings and 383 informational findings; it is not warning-free.
- Plan checker: 348 tasks, 545 rules, 167 use cases, 145 screens; zero structural
  problems and three checker tests. Changed Go formatting, diff, added links and
  staged secret review accompany the atomic task checkpoint.

Representative scoped command, with the explicit disposable PostGIS DSN set:

```sh
GOCACHE=/tmp/enterprise-go-build go -C backend test -race -tags=integration -p 2 \
  -coverpkg=./internal/modules/organization/... \
  -coverprofile=/tmp/enterprise-org009-coverage.out \
  ./internal/modules/organization/... ./internal/modules/admin/... \
  ./internal/modules/orgaccess/... ./internal/modules/membership/... \
  ./internal/modules/companysession/... ./internal/platform/orgapi \
  ./internal/platform/enterprisesuite ./internal/platform/apicontract
TZ=America/Cuiaba python3 docs/enterprise/tools/check_plan.py --write
python3 -m unittest discover -s docs/enterprise/tools -p 'test_*.py'
```

## Delivery and remaining work

Publish through a new cumulative dev → main PR, since PR202 is merged. This
request authorizes one next task and GitHub delivery, not another autonomous
F0–F8 run, merge or deployment. Current-head/base remote CI is separate from
local acceptance. No wiki publication of unmerged code occurs.

Preserve Claude's integrated activation blockers: creator lock-in until transfer/
abandoned-setup closure; ordinary runtime grants; early pending-setup check;
keyed replay fingerprints; actual AUTH/IDN/device providers and revocation;
vault lifecycle/native acceptance; DOC-014; staging/backup evidence;
PLAT-012/GOV-014; aggregate Kotlin coverage and legal receipt/purpose gates.
No Android source changes require emulator work in this task. No production,
release, delivery-provider or real-data acceptance is asserted.

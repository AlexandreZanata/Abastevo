# F1 EMP-ORG-003 atomic synthetic organization onboarding

Status: LOCAL_DONE
Validation: PASS

2026-11-11. LOCAL_DONE / INTEGRATION_PENDING above AUTH-004 49048734 in the
owned enterprise execution worktree, targeting [PR202](https://github.com/AlexandreZanata/Abastevo/pull/202).
F1/M1 remain open. Publication follows the authorized completed-phase cadence;
current-head/base remote CI is still due at the cumulative batch exit.

## Contract and recorded choices

B-BR-EMP-53/54 and BUC-EMP-27/28 preceded behavior. The selected task owns
creation and setup APIs, not public enrollment or permission for real data.
The autonomy authorization allows the following recommended choices, recorded
also as EDR-033..036 in [the decision register](../16-decisions.md):

- Creation and activation are PANEL operations through the existing dedicated
  company cookie, CSRF and exact configured HTTPS Origin/Host. Normal personal
  account proof is exchanged at company login. Business DTOs contain no account
  or credential fields. Freshness is 15 minutes, inherited from that plane.
- Creation accepts the explicitly synthetic source terms fixture, version
  enterprise-synthetic-dpa-v1 and SHA-256
  590cc4cf31e1909ac9baacb76092897f3dba07b95719953b0f4588b51348f8b3.
  This records a synthetic declaration, never actual DPA/legal acceptance.
- Managed provenance belongs to the actual account and is immutable from
  insertion. Existing accounts remain personal; a descriptive membership model
  cannot convert them. Managed provisioning still belongs to IDN's owning task.
- Pending OWNER slots are serialized on the personal account. Future and
  invited/suspended, non-ended potentially effective OWNER periods reserve the
  slot too; counting active authority only would permit scheduled bypass.
- The synthetic org.setup admission class explicitly inherits identity v1's
  200 writes/hour for each org/account/web-or-device axis. This is a bounded
  synthetic setting, not measured production capacity. Live measurement,
  stable replica policy/key and quota cleanup activation remain prerequisites.

No dependency, external provider, production credential, grant or flag is added.
No payment tier or public/community source grants organization access.

## Atomic command and current authority

POST /v1/org-access/organizations/create is a distinct account-scoped bootstrap
entry, before the organization/membership exist. Five identifiers are selected
on the server. In one ordinary-role transaction the command locks the account,
rechecks current cookie/family/freshness/deletion fence, resolves an earlier
account-owned acknowledgement, then writes the pending root, active MAIN unit
with the organization's zone, employee, bound active membership, OWNER period,
synthetic terms receipt and audit event. Any failure leaves none of that group.
A matching operation returns its original tuple even when new IDs were generated
for the retry; changed normalized intent returns OPERATION_ID_REUSED. Names,
kinds and the digest are explicit mappings, not domain redacted JSON.

The existing audit runner gains a borrowed-transaction port that validates the
ordinary role and installed scope and never commits. The old runner retains its
own transaction behavior. Both callers and tests distinguish provisional work
from committed results. No speculative downstream consumer is introduced.

POST /v1/orgs/{org_id}/organization/activate uses AUTH's current role snapshot,
request-bound decision, org.settings.manage, fresh company proof, independent
organizations pilot flag, setup record policy and orgquota admission. The write
locks and rechecks the account and original cookie/family again, then the flag
(shared), root, current membership and role. It derives checklist facts from
actual stored units, owners and the pinned terms receipt. It never trusts a
client checklist or earlier authority alone. Exact retries return the original
ACTIVE/revision acknowledgement; stale distinct changes conflict. A deferred
commit failure returns no provisional value or replay success.

Incomplete setup returns only copied allowlisted checklist keys with REQUIRED
details. A reproduced 409-with-empty-details regression is fixed and verified
through the real company HTTP flow after removing/restoring a pending name.
Unknown/duplicate keys and caller mutation cannot add private details.

Source fields and request values redact through fmt (including invalid verbs),
JSON and slog where used. Responses map only the intended identifiers/state,
revision, replay marker and SYNTHETIC mode; errors use bounded registered codes.
All protected responses are no-store without ETag. The global surface remains
off by default, and creation never provisions a per-org capability.

## Schema, limits and safe upgrade

Additive migration 000172 adds the account managed-origin marker, forced-RLS
org_units and immutable org_onboarding_commands. Composite FKs retain scope for
unit/employee/membership/assignment references; deferred audit and composition
checks require the complete committed setup. SQL rejects missing/null or changed
synthetic terms, immutable-history mutation and last-active-unit archival.
Pending-to-active SQL checks actual units/terms/continuous owner coverage, so a
supplied pure-domain checklist cannot bypass persisted facts.

A trigger-only SECURITY DEFINER inventories pending ownership under an account
FOR UPDATE lock, verifies coherent ownership of the four relevant tables, and
has no PUBLIC execution. Narrow owner-only inventory policies use invoker
PL/pgSQL helpers that short-circuit for runtime readers. Inline subqueries had
imposed unrelated membership grants on ordinary root reads; that reproduced
regression is fixed without expanding runtime visibility. Runtime GUC spoofing
cannot obtain the owner inventory. OWNER adapters acquire the exclusive account
lock before the root, avoiding a SHARE-to-UPDATE upgrade race between grants.

The populated upgrade includes an initialized prior root, account, active member,
employee, OWNER period and public station. An ordinary NOSUPERUSER/NOBYPASSRLS
migration owner applies it. Injected SQLSTATE 22012 rolls back new tables,
marker/functions/triggers and ledger; retry applies once, then no-ops. Prior rows
are unchanged (the new account marker is separately null), the previous image's
migration check succeeds, and actual post-upgrade creation/limit checks pass
under ordinary ownership. Recovery keeps the additive schema/history and turns
off the entry, then uses the compatible previous image. No destructive down or
applied migration edit is used.

The audit TRUNCATE CASCADE regression fixture explicitly grants truncate on the
new referencing receipt table and disables only dependent append-only triggers
for that check. It still requires the audit trigger's exact 55000 and message;
the assertion was not weakened to accept a permission failure.

## Validation

Disposable PostgreSQL 18/PostGIS 3.6 uses the pinned repository image on loopback
32771, with separate ordinary runtime logins and synthetic records. Privileged
pools are used only to prepare/rehearse fixtures, never as accepted application
roles. This is not evidence of production login/grants. Go 1.27.4, sqlc 1.31.1,
staticcheck v0.8.1 (Go 1.27.1 toolchain) and vacuum 0.30.6 were used.

Meaningful real PostGIS/race cases pass:

- Twelve distinct creations: exactly three successes/nine bounded refusals,
  with no orphan root/unit/member/role/receipt/audit rows. Twelve equal retries:
  one creation/eleven matching acknowledgements. Changed intent refuses.
- Four concurrent future OWNER grants across roots reserve three slots; ending
  a reservation permits the refused grant. The adapter lock order converges.
- Late membership failure and deferred creation/activation commit failures roll
  back the entire transaction and return zero values. Twelve identical
  activations produce one write/eleven replays; stale/reused intent refuses.
- Real fresh managed-account cookie cannot create or receive OWNER. Revoked
  cookies/families, elapsed freshness and inactive account cannot create;
  activation independently rechecks revoked/stale proof inside its transaction.
- Two-organization/unscoped unit and receipt isolation, runtime inventory spoof
  refusal, null/different terms constraints, immutable history and last unit.
- Real account-key/family/company login through the actual composition, cookie
  and CSRF refusal, wrong Origin/admin/bearer plane refusal, create/replay,
  unprovisioned activation denial, explicit fixture flag provision,
  activation/replay, changed intent and identical private unknown/foreign scope.
- The fully composed production router (over 100 routes), before listening,
  passes the bidirectional OpenAPI gate when enabled. Adding an unguarded private
  route fails it. Default-off composition exposes no creation entry. Cookie,
  CSRF, freshness/channel metadata and new schema/example drift are checked.

Commands from backend/, with ANPFUEL_TEST_DATABASE_URL set only to the fixture:

```sh
go test -race -tags=integration ./internal/modules/organization/... ./internal/modules/membership/... ./internal/modules/orgaccess/... ./internal/modules/orgaudit/... ./internal/modules/orgcapability/... ./internal/modules/companysession/... ./internal/platform/orgapi ./internal/platform/orgquota ./internal/platform/apicontract ./internal/platform/enterprisesuite ./cmd/api -count=1 -coverpkg=./internal/modules/organization/... -coverprofile=/tmp/enterprise-org003-coverage.out
go test -race -tags=integration ./internal/modules/account/... ./internal/modules/payments/... -count=1
sqlc vet
sqlc diff
go vet ./internal/modules/organization/... ./internal/modules/orgaccess/... ./internal/modules/orgaudit/... ./internal/modules/companysession/... ./internal/platform/enterprisesuite ./internal/platform/orgapi ./internal/platform/apicontract ./cmd/api
GOTOOLCHAIN=go1.27.1 XDG_CACHE_HOME=/tmp/enterprise-staticcheck staticcheck ./internal/modules/organization/... ./internal/modules/orgaccess/... ./internal/modules/orgaudit/... ./internal/modules/companysession/... ./internal/platform/enterprisesuite ./internal/platform/orgapi ./internal/platform/apicontract ./cmd/api
go build ./cmd/api ./cmd/worker ./cmd/ops
```

All pass. The coverage profile before the final request-redaction follow-up
records domain 97.83%, application 83.93%, adapters 78.06%, HTTP 75.53%; uncovered
paths remain, and percentage does not replace the critical cases above. Final
request-redaction, schema examples and safe checklist details have additional
focused race/real-HTTP/static/build checks. No full aggregate rerun is claimed after documentation/example-only edits.
Plan checker/three regressions, final OpenAPI validation (no added warnings),
strict formatting, added links, whitespace and secret-surface checks accompany
the task commit. No Android behavior changes, so native acceptance is not claimed.


Tested runtime Git blobs in this task commit: migration 000172
b8ccc9acd71ee4b5bac6418de5a00be8587fbda3; application onboarding
cc8ed0a1b9225ffd90edea4ad65761732588efe5; persistence composer
0670d3754b9c7f0639b53becec8b75f26eb3567c; HTTP entry
62116ace0c5d8c59b0c5c803af376220103ef690; production enterprise composition
5df06a8923484d771d7f38b5de1e71aaa2356014.

## Next and independent gates

ORG-005 operating units is the next C1 organization task; settings and identity,
invitation/managed provisioning, native context and offline tasks remain. The
first composed-business-route contract gate is now exercised. Actual IDN/device
providers, ordinary production runtime grants, quota cleanup/measurement, vault
expiry/device cleanup, native/emulator acceptance, real legal terms/receipts,
staging/bucket/backup, Kotlin coverage and PLAT-012/GOV-014 obligations remain.
Nothing here certifies M1, real-data activation, a deployment or a release.

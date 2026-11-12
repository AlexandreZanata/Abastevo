# F1 EMP-IDN-001 private identity and lifecycle domain

Status: LOCAL_DONE
Validation: PASS

2026-11-11. Pure-domain local acceptance, INTEGRATION_PENDING through
[PR202](https://github.com/AlexandreZanata/Abastevo/pull/202). F1/M1 remain open.

## Entry and scope

The owner requested the next enterprise task and GitHub publication. The
clean isolated codex/enterprise-f0-f1 worktree entered at published e087b944
(ORG-002), matching remote dev and PR202; main ef96cd13 is its ancestor.
Occupied local dev and unrelated primary-checkout work are preserved.

ORG-003 still depends on IDN-003/AUTH-002. IDN-001 is the first eligible
identity foundation and depends only on accepted PLAT-001. Read current
progress/fast card, its task, identity entities/rules/state graph, permissions
§4, scope, existing account identities and contributor fingerprint parsing.
B-BR-EMP-40/41/42 and BUC-EMP-21 precede behavior. The 29-row oracle was fixed
from the three source state graphs before implementation; its initial RED
run failed on the then-absent domain API, followed by the passing implementation.

The confirmed package is membership/domain, separate from contributor
identity/domain. Scope is pure identities, associations and lifecycle only.
No employee descriptive register, licence content/eligibility, storage,
token/session issuance, runtime providers, actual revocation or routes are
implemented. No new dependency, SQL migration or personal-account change.

## Implemented model

- There is no platform-wide Person record. PersonalAccount references an
  existing nonzero UUID without owning, discovering or provisioning it.
  ManagedAccount also binds the reference to its original organization.
  Constructors canonicalize UUIDs using the existing parser for shape only.
- Employee is an organization-owned identity/lifecycle value, initially
  ACTIVE/revision one. It requires no membership, account or driver data.
  Register attributes remain IDN-002. The optional DriverProfile association
  references the employee; licence values and eligibility remain IDN-014/F2.
- Membership selects exactly one existing-account/managed access model.
  NewInvited has no account; BindAccount remains INVITED, including while
  confirmation is pending. Activation requires an active matching employee,
  bound matching account, confirmed admission/confirmation and positive
  role-assignment facts. A reference/credential supplies no permission.
- ValidateLiveMemberships detects duplicate non-ENDED employee/account links
  within an explicitly supplied scoped set. Unknown pending accounts do not
  collide. It is not an authoritative query or proof that unseen rows do
  not exist; IDN-003 must enforce concurrent uniqueness in PostgreSQL.
- Employee and membership transitions follow only the documented graph.
  Suspension/end require the relevant reason, ownership and handover facts.
  Inactivity-driven suspension records its origin. Automatic employee return
  cannot lift a managerial suspension. Rehire restores the employee identity
  but never an ENDED membership; a merged-away employee cannot be rehired.
- Credential descriptors contain account/source kind only. They never take
  secrets, verify a credential or expose grant/authorization methods. Managed
  descriptors allow only enrolled device keys; personal account lifecycles
  and sessions are untouched.
- Device enrolment binds an existing device UUID and canonical fingerprint
  to an ACTIVE membership after confirmed proof/count/limit facts. Personal
  explicit limits are 1–5 and managed is exactly 1. No default is inferred
  from absent facts. The existing identity fingerprint parser is reused.
  Revocation records LOST_OR_COMPROMISED or ACCESS_ENDED and is terminal.
  Later enrolment needs a fresh persisted record; no remote erase is claimed.

Every private value read/change requires its original kernel.OrgScope;
mutations check expected positive revision and refuse overflow. Values and
snapshots copy by value and are safe for independent concurrent evaluation.
Formatting with every fmt verb, slog and nested JSON prints redactions.
Storage must map fields explicitly. Errors are bounded and contain no input.
Rehydrate functions accept trusted storage only, never client command DTOs.

Conditions are server-resolved facts, not permission booleans sent by a
client. Current actor proof, last-owner locking, real role assignment,
admission/token validation, period timestamps, history/tombstones, uniqueness,
audit and all-or-nothing effects remain persistence/application obligations.
The core is not wired into API/worker production composition.

## Validation

Go module toolchain 1.27.4; staticcheck v0.8.1 uses its Go 1.27.1 toolchain.
Commands from backend/:

```sh
go test -race ./internal/modules/membership/domain ./internal/modules/identity/domain ./internal/modules/account/domain ./internal/modules/kernel ./internal/modules/organization/domain -count=1
go test -race ./internal/modules/membership/domain -coverprofile=/tmp/enterprise-idn001-coverage.out -count=1
go vet ./internal/modules/membership/domain ./internal/modules/identity/domain ./internal/modules/account/domain ./internal/modules/kernel ./internal/modules/organization/domain
staticcheck ./internal/modules/membership/domain ./internal/modules/identity/domain ./internal/modules/account/domain ./internal/modules/kernel ./internal/modules/organization/domain
go build ./cmd/api ./cmd/worker ./cmd/ops
```

All pass, no skipped cases. New domain statement coverage: **99.6%**. The
[29 vectors](../../../contracts/testdata/enterprise/README.md#identity-lifecycle-v1)
cover all 9 employee, 16 membership and 4 device state pairs. Nine additional
invariant suites cover account/model/scope independence, known-set live-link
uniqueness, no implicit access on binding/rehire, activation fact combinations,
employee-state provenance, both revocation classes and device-limit edges,
absent/foreign scope, invalid hydrated values, stale/exhausted revisions,
immutable sharing with 32 goroutines and nested logging/JSON redaction.

The initial malformed-account negative used an already-bound fixture; its
expected conflict was correct. The test now uses a genuinely unbound period
to exercise invalid association, retaining the separate rebind-conflict test.
No production behavior or assertion was weakened.

Changed Go formatting, scoped static/build checks, git diff --check, added
document links, plan verifier and its three regression tests, and staged
secret-surface review pass. The verifier retains 348 tasks, 545 rules,
167 use cases and 145 screens with zero problems. The new package is in the
quick behavioral manifest. No aggregate Android/native or database run is
claimed for a pure Go change without SQL/transaction changes.

## Next and retained gates

IDN-003 is now eligible after IDN-001/ORG-002; AUTH-001 remains needed before
AUTH-002 and the eventual ORG-003 onboarding transaction. Neither those tasks
nor F1/M1 is marked complete by this source checkpoint.

Previously recorded AUTH/IDN single-family/provider decisions, production
non-bypass login/grants, composed-router gate, vault inventory/sweep and
device-revoked cleanup, native acceptance, EMP-DOC-014, staging lifecycle/
backup roles, PLAT-012/GOV-014, aggregate Kotlin coverage and legal receipt
gates remain owed. Publication updates the existing cumulative PR; no merge,
VPS deployment or production certification is inferred.

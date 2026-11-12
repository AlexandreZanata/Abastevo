# F1 EMP-AUTH-003 current request authorization

Status: LOCAL_DONE
Validation: PASS

2026-11-11. Source acceptance above AUTH-002 commit 10edd3c4, on the owned
enterprise worktree; INTEGRATION_PENDING through cumulative
[PR202](https://github.com/AlexandreZanata/Abastevo/pull/202). F1/M1 remains open.
No business route, production runtime grant or organization is activated here.

## Contract and decisions

Read the selected task, current progress/fast execution, permissions §§4–5/10,
organization suspension/setup rules and existing gateway/company proof.
B-BR-EMP-49/50 and BUC-EMP-25 precede implementation. Domain and application
REDs failed on missing coverage/authorization APIs. The scoped source RED
failed on missing LoadAuthority. Android fresh-route RED failed on the
missing explicit session confirmation capability.

Recommended implementation decisions adopted under the maintainer's
autonomous-construction authorization:

- Load membership, organization, employee/account availability, deletion
  fence and effective periods in one current scoped SQL statement. Do not
  cache permission or infer it from a client context, cookie or paid tier.
- Separate preparation from completion so that current role/target checks
  precede proof freshness, capabilities/limits and record policy. Each gate
  is explicit; missing providers fail closed. Sensitive role/delegation,
  ownership, credential and Data-key policies require fresh authentication.
- Bind the result to a fresh request lease, account, organization, permission
  and server-resolved target. Completion is single-use; finishing the request
  invalidates retained contexts/decisions. Verified contributor/device-key
  attribution is carried explicitly to record policies and handlers.
- Classify source-owned effects for setup, normal work, reads, access
  reduction, plan regularization and restoration. Existing organization
  rules govern suspension/archive exceptions; none bypass roles or plan/
  record gates. Operational setup refusal is the documented 409
  ORG_SETUP_INCOMPLETE. Missing permission exposes only its registered key.
- Keep application record/capability ports mandatory. The concrete pilot
  entitlement adapter belongs to AUTH-004; business record rules belong to
  their modules. No permissive production fallback is supplied here.

## Source and boundaries

orgaccess/domain checks effective periods, ORG/SELF scope and every catalog
restriction over server-owned resource facts. OWNER's S cells remain own
records; driver GPS remains own trips; auditor tracks are recorded-only;
ADMIN cannot assign OWNER; agreement parties stay separate. All 23 registered
but unassigned permission keys remain denied. No catalog grant/version changes.

orgaccess/application loads actual current authority, checks membership/model
and organization state before resource resolution, then prepares covering
rules. Source errors are bounded. Prepare never accepts cached authority.
Complete confirms proof, checks capabilities/limits and record policy, and
produces a request-bound decision only after all succeed. Concurrent attempts
to complete one preparation execute the proof/capability/record chain once.
Private actors, snapshots, subjects and decisions redact fmt/JSON/slog views.

The owned sqlc CurrentAuthority query runs under the ordinary forced-RLS
runtime transaction. Ended memberships, unavailable accounts and account-exit
reservations disappear; expired/ended/future roles are excluded. No migration
is added. Existing schema 000171 and its owner/account-erasure guards remain.

orgaccess/adapters/http.Register binds a policy, resource resolver and service
to the existing orgapi gateway. It preserves normal strict decoding, signature/
cookie/CSRF/origin checks, no-store/error behavior and write admission. It
refuses missing gates, rule/policy disagreement, an unconfirmed Android channel
and conflicting pre-existing authority callbacks. Each request gets fresh
private state. The handler receives the completed decision and normal principal.

The gateway now supports explicit Android session confirmation after current
scope/permission; fresh Android routes refuse registration without it.
AccountSessions reuses the existing live AccountProof verifier and freshness
window without retaining credentials or minting a new family. The existing
company confirmation still checks current proof/freshness before completion.
The bounded permission detail and setup error are documented in OpenAPI.

## Validation

Race-tested domain/application cases cover catalog restrictions, half-open
periods, revoked/missing authority, stale request-context reuse, malformed
proof metadata, managed web refusal, unavailable/missing gates, organization
state exceptions, proof/freshness/capability/limit/record denials, private
error stripping, attribution and fmt/JSON/slog redaction. Twelve concurrent
completions have one proof/capability/record execution.

Actual PostGIS/current source and normal HTTP entry prove:

- Real account family, enrolled ECDSA signer, current account/key binding and
  nonce proof plus real company cookie/CSRF session; invalid signature stops
  before resource/handler access.
- Ordinary forced-RLS role/membership reads; foreign organization refuses
  before resource lookup. A foreign record and unknown record return identical
  404 bodies, no foreign metadata or ETag.
- A role end affects the next Android and cookie request. Offboarding removes
  both channels. A newly assigned DRIVER/SELF role reads only a record whose
  author is the current member; another member's record stays forbidden.
- Freshness precedes capability/record execution; missing capability and
  self-approval refusal never dispatch a handler. Finished context reuse fails.
- Shared account/company, membership, owner/history and gateway regressions
  pass, preserving the earlier migration/concurrency/erasure safeguards.

The resource table and allow/deny capability/record probes in the integration
test are explicitly disposable fixtures, not new production modules or
substitutes for AUTH-004/business policies.

Commands from backend/ (explicit disposable PostGIS DSN):

```sh
go test -race ./internal/modules/orgaccess/... ./internal/platform/orgapi ./internal/platform/apicontract
go test -race -tags=integration ./internal/platform/orgapi ./internal/modules/orgaccess/... ./internal/modules/membership/... ./internal/modules/companysession/... -count=1 -coverpkg=./internal/modules/orgaccess/... -coverprofile=/tmp/enterprise-auth003-final-coverage.out
go vet ./internal/modules/orgaccess/... ./internal/platform/orgapi
GOTOOLCHAIN=go1.27.1 XDG_CACHE_HOME=/tmp/enterprise-staticcheck staticcheck ./internal/modules/orgaccess/... ./internal/platform/orgapi
go build ./cmd/api ./cmd/worker ./cmd/ops
sqlc vet
sqlc generate
```

All pass. Cross-package profile: domain 92.96%, application 89.74%, storage
adapters 83.59%, HTTP adapter 77.19%, aggregate 88.2%. Additional HTTP
configuration/missing-provider tests pass after that profile. Percentages do
not replace the critical cases above or establish production readiness.
OpenAPI lint passes with 0 errors and the existing 785 warnings/179 informs.
Strict format, added links, plan checker/three regressions, whitespace and
secret review accompany the commit. Plan consistency remains 348 tasks,
545 rules, 167 use cases, 145 screens and zero problems.

## Next

AUTH-004 supplies the concrete disabled-by-default pilot entitlement adapter.
ORG-003 and subsequent business handlers consume the bound authorization
entry and implement their actual record rules/audit/transactions. The first
business route still owes the composed production-router gate. Enterprise
device enrollment/revocation, actual IDN providers, production runtime grants,
vault/native/legal/staging and aggregate Kotlin gates remain named obligations.
F1/M1 and final current-head/base CI are open. Rollback keeps the enterprise
surface disabled; no merge, deployment, release or real-data acceptance occurs.

## Final PR202 review corrections (2026-11-12)

Fresh proof is now also required for `rpt.export.run`, `org.lifecycle.manage`
and `org.group.manage` (BR-AUTH-010). The full-router gate requires every
non-session organization route to complete AUTH authority, so a raw
`orgapi.Register` route fails it, and the disabled-mode test proves through
`chi.Walk` that no organization route is mounted. [Review record](f1-pr202-review.md#final-review-head-45230e28).

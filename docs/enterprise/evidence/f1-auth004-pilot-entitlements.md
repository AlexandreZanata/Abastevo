# F1 EMP-AUTH-004 pilot entitlement adapter

Status: LOCAL_DONE
Validation: PASS

2026-11-11. Local source acceptance above AUTH-003 4e32bfae;
INTEGRATION_PENDING in cumulative [PR202](https://github.com/AlexandreZanata/Abastevo/pull/202).
F1/M1 remains open. The occupied dev checkout and other work are preserved.

## Contract and interpretation

B-BR-EMP-51/52 and BUC-EMP-26 precede behavior. Read AUTH-004, permission
conditions, activation §10 and PLAT-005's actual switch/transaction contracts.
Initial RED failed on missing PilotEntitlements. No billing, account tier,
subscription, role grant or new capability registration is introduced.

Recommended interpretation under the maintainer's autonomous authorization:
the adapter consumes the existing scoped flag repository directly, rather
than calling the authorizing capability Service recursively. Its default-off
global activation matches the private gateway's existing configuration.
Only source-registered capabilities are accepted; no client flag is read.

An absent/off organization flag denies new operational/configuration work.
Authorized existing reads/exports remain available per PLAT-005. Existing
access-reduction, plan-regularization and restoration safety paths remain
available per BR-ORG-032/033; their route effect is server-owned. These paths
still require the current proof/roles/scope, available state provider and
independent record gate. The global surface being off still denies every
path. Future paid-plan billing remains a separate adapter/task.

## Implementation

orgaccess/adapters.PilotEntitlements implements the capability port already
required by AUTH-003. Each call validates scope/subject/policy/catalog and
reads the selected organization's current flag. Missing/unavailable/corrupt
state and mismatched scope fail closed. No cache, account/payment input or
authority mutation exists; enabled state never creates a covering role.

The entry check does not hold a business transaction lock. Owning commands
must still use PLAT-005's GuardWrite/Run in their actual scoped transaction;
its shared row lock serializes with disable. The adapter does not nest that
transaction or weaken the existing guard.

A missing production reader stored as a typed-nil interface initially
panicked in orgcapability.Store.validate. The regression reproduced it;
the common store guard now returns its existing ErrUnavailable for a nil
receiver. The pilot boundary translates that failure without private detail.
No query, migration or normal flag behavior changes in that correction.

## Validation

Race-tested matrix: global default off; absent/off/enabled organization
state; retained read/safety effects; unknown catalog; missing/unavailable
and typed-nil production reader; corrupt revisions; cancellation and foreign
scope refusal. Role denial precedes any flag read, and flag state never
creates or expands an assignment.

Actual ordinary-login PostGIS tests use the existing audited flag repository
and current AUTH storage: B's enabled flag does not admit A; enabling A
admits its current owner; DRIVER gains no management permission; disabling A
stops its next new action, preserves current reads and changes neither B nor
role history; re-enabling resolves current state. A disable after entry
acceptance still refuses the actual transaction before its callback. The
fixture calls the storage Set boundary explicitly; no production operator
authorization/entry point is invented.

Commands from backend/ (explicit disposable PostGIS DSN):

```sh
go test -race ./internal/modules/orgaccess/... ./internal/modules/orgcapability/...
go test -race -tags=integration ./internal/modules/orgaccess/... ./internal/modules/orgcapability/... ./internal/platform/orgapi -count=1 -coverpkg=./internal/modules/orgaccess/... -coverprofile=/tmp/enterprise-auth004-coverage.out
go vet ./internal/modules/orgaccess/... ./internal/modules/orgcapability/...
GOTOOLCHAIN=go1.27.1 XDG_CACHE_HOME=/tmp/enterprise-staticcheck staticcheck ./internal/modules/orgaccess/... ./internal/modules/orgcapability/...
go build ./cmd/api ./cmd/worker ./cmd/ops
```

All pass, including PLAT-005's existing real lock/rollback/race suites and
the normal real-proof AUTH-003 gateway regressions. Pilot Check coverage is
95%; aggregate orgaccess profile 89.3%, domain 92.96%. The additional typed-nil
RED/GREEN and affected unit/race/static checks pass after that profile.
Strict format, plan checker/three regressions, added links, whitespace and
secret checks accompany the commit. No Android, applied SQL or production
configuration changes; no broad native/production acceptance is claimed.

## Next

ORG-003 is the next core F1 step: compose pending root, default unit and
creator employee/membership/OWNER atomically, with normal account proof and
the first composed business-router gate. Actual business record policies,
operator provisioning authority, IDN/device/runtime/native/legal/staging and
Kotlin coverage obligations remain tracked. Phase publication and final
current-head/base CI remain due; no merge/deployment/release is performed.

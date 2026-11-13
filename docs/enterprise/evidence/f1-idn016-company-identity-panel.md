# EMP-IDN-016 — Company employee and invitation panel

Date: 2026-11-13.
Status: LOCAL_DONE
Validation: PASS
Integration: INTEGRATION_PENDING

Construction source: `6b0921ce512358569b690defeca4dd9e52206cbc`.
Owned branch: `codex/enterprise-idn016-018`; final batch publication targets
maintained dev in [PR203](https://github.com/AlexandreZanata/Abastevo/pull/203).
[Task212](https://github.com/AlexandreZanata/Abastevo/issues/212) stays open until
actual protected integration. B-BR-EMP-90/91, BUC-EMP-46 and EDR-057 precede
behavior. Tasks017 and018 follow only after this tested local checkpoint.

## Source decisions

PLAT-008 intentionally shipped an unavailable production membership provider.
This task must install an account-bound current source instead of guessing an
endpoint or treating browser selection as authority. A narrow SELECT-only
current-account membership discovery policy and per-organization source checks
are the recommended dependency; there is no global organization directory.
The migration estimate is updated from none to an additive source policy with
ordinary-role recovery/isolation tests. Business routes retain their existing
authorization, fresh proof and idempotent command contracts.

The C1 panel covers existing-account invitations. One-time delivery uses code
and fragment-only link without a new QR dependency. Managed activation remains
behind GOV-013 and its owning APIs. No personal account sessions are revoked by
switching organization. The separate encrypted Android vault remains closed
without its own current device/contact proof.

## Validation

Backend critical PostGIS/race checks pass in6.676s: two-organization and
cross-account discovery, filtered-empty pagination, twelve concurrent selections,
observed root-lock waits followed by freshness expiry, actual membership
suspension/reactivation, storage-failure classification and strict real
cookie/Origin/CSRF HTTP. A populated ordinary194→195 rehearsal proves late-fault
rollback, recovery, idempotence and previous-image compatibility. Privileged
runtime roles remain refused. Private source JSON/fmt/slog redaction passes.
The actual route/OpenAPI gate passes after final source redaction in15.069s;
vet, staticcheck, sqlc vet/regeneration and compatibility checks pass.

Employee reads project the actual current scoped live membership separately
from the employee lifecycle. The real source test passes NONE→INVITED→NONE
after revocation, then ACTIVE after acceptance and SUSPENDED after manager
reduction; the employee/profile revisions remain unchanged. Lists, reads and
duplicate matches agree, foreign scope refuses. This dedicated test passes
in1.928s; the affected employee/profile/HTTP/migration regression passes in6.849s.
Membership domain coverage is91.3%. The new descriptive value is never an
authorization input; unhydrated private objects expose UNKNOWN rather than
inventing active access.

```sh
go -C backend test ./internal/modules/membership/domain ./internal/modules/membership/application ./internal/modules/membership/adapters/http
go -C backend test ./internal/modules/membership/domain -cover
go -C backend test -race -tags=integration -timeout=20m ./internal/modules/organization/adapters -run 'TestAccountContext' -count=1
go -C backend test -race -tags=integration -timeout=20m ./internal/modules/organization/adapters -run 'TestEmployee(Register|Profiles|AccessView)' -count=1
go -C backend test ./internal/platform/apicontract -run TestAccountContext -count=1
```

These commands use the explicit disposable ANPFUEL_TEST_DATABASE_URL on the
owned `emp-idn016-018-20261113` PostgreSQL18/PostGIS3.6 tmpfs instance and pinned
Go1.27.4. No external service or production database is targeted.

The actual Chromium/HTTPS/Go API browser flow passes using a distinct ordinary
runtime (NOSUPERUSER/NOBYPASSRLS/NOINHERIT/non-owner) and explicit per-table C1
fixture grants. Migrations run separately as the disposable database operator.
The capability flag is seeded only for the exact synthetic local organization;
this does not activate any deployed service. Actual account, current context,
two organizations, employee creation, issue/resend/revoke, ADMIN confirmation
and rejection, metadata refresh, keyboard interaction and mobile/desktop layouts
pass. Thirty Node tests, typecheck, build and all22 artifact checks pass.

Independent review findings were corrected and tested: updated idle deadlines,
private directory cleanup on refusal, immediate outer login reset on401/fresh,
concealed404 membership denial, source history after resend, no stale CAS retry
and retention of first-only delivery only across transient follow-up failures.
Definitive authority refusal clears code/profile/list. Access metadata is UNKNOWN
until refreshed from the server after a mutation, never inferred from invitation
status. Mobile and desktop synthetic screenshots were visually inspected.

```sh
npm --prefix business-web run typecheck
npm --prefix business-web test
npm --prefix business-web run build
npm --prefix business-web run check
GOCACHE=/tmp/enterprise-go-build python3 business-web/tests/browser.py
python3 scripts/tests/test-enterprise-local-sql.py
python3 scripts/tests/test-enterprise-protection.py
python3 docs/enterprise/tools/check_plan.py
```

No new production or npm dependency is introduced. The local Python harness uses
already installed psycopg3.3.6 (LGPL-3.0-only; test process only), parameterized SQL
and quoted identifiers. No advisory scan is claimed for that existing test tool.
Basic C1 creation leaves the optional unit unset; a linked unit is described
without exposing opaque identifiers in the main flow. Unit selection/team
management retains the owning structure UI. Managed-code controls remain absent.

This tested local task checkpoint precedes017. Full local Quick and the union
regression run once after018. No runtime activation, production readiness, merge,
wiki or deployment is claimed.

Tested backend tree: `0c04125ffd69941cdc8a45d319bc44733edfdde6`.
Tested company web tree: `838492e46967e148a64e97956e817081ff25d7c9`.
OpenAPI Git blob: `44e16fe9003b1f4ac2fd73c994007ba92a2c28ea`.
The staged complete diff passes gitleaks and whitespace review.

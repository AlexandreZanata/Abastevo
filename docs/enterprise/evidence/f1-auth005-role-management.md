# F1 role management with last-owner and separation-of-duties guards — EMP-AUTH-005

**Historical implementation snapshot, superseded by the
[2026-11-12 source review](f1-antigravity-review.md).** The PASS/LOCAL_DONE
statements below describe the original author's checks, not accepted current
security evidence. Review reproduced missing transactional authority, mutable
replays and route/contract failures; the linked record states the corrected
behavior, actual commands, limitations and tested artifacts. Published source
migrations remain unchanged; corrections use forward migrations186/187.


Status: LOCAL_DONE / INTEGRATION_PENDING
Validation: PASS
Date: 2026-11-12
Source parent: `8a48c0a35b93d6bd9886e6bdb20de2caf407b5d6` (EMP-ORG-009 on `origin/dev`, head of PR #203).
Owned worktree: `.worktrees/enterprise-suite-execution`; isolated local branch `codex/enterprise-auth005`, publishing to maintained remote `dev` (updating PR #203).

Tested backend tree: `github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgaccess`
Tested OpenAPI contract: `contracts/openapi/v1.yaml` (validated with `vacuum` 0.30.6 and `scripts/check-compat.sh`).

---

## 1. Objectives & Delivered Scope

Task **EMP-AUTH-005** implements audited, scoped role management for Abastevo Empresas organizations under `/v1/orgs/{org_id}/roles`, enforcing strict separation of duties, immediate effect, and protection of the last-owner invariant.

### Endpoints Implemented

1. **`POST /v1/orgs/{org_id}/roles/assign`**
   - Assigns a system role (`OWNER`, `ADMIN`, `FLEET_MANAGER`, `SUPERVISOR`, `FINANCE`, `APPROVER`, `DRIVER`, `OPERATIONS_STAFF`, `AUDITOR`, `STATION_ADMIN`, `STATION_MANAGER`, `STATION_ATTENDANT`, `DATA_MANAGER`) to an active or invited member.
   - Requires permission `auth.role.assign` and fresh authentication (`actor.RequireFresh()`).
   - Emits audit event `auth.assignment.created` (`target_type: "assignment"`, reason: `"granted"`).

2. **`POST /v1/orgs/{org_id}/roles/{assignment_id}/end`**
   - Ends an active role assignment immediately.
   - Requires permission `auth.role.end` and fresh authentication.
   - Rejects removal of the last active owner (mapped from database trigger `SQLSTATE P7501` to `kernel.ErrLastOrgOwner`).
   - Emits audit event `auth.assignment.ended` (`target_type: "assignment"`, reason: `"ended"`).

3. **`POST /v1/orgs/{org_id}/roles/{assignment_id}/change`**
   - Atomically ends the existing assignment and creates a new assignment for the target member in a single database transaction.
   - Requires permission `auth.role.assign` and fresh authentication.
   - Prevents changing the role of the last active owner away from `OWNER` if no other owner exists.
   - Emits exactly one audit event `auth.assignment.created` (`target_type: "assignment"`, reason: `"changed"`), recording the previous assignment ID and prior role in audit payload.

4. **`GET /v1/orgs/{org_id}/roles/{assignment_id}/read`**
   - Retrieves assignment details by ID within the organization scope.
   - Requires permission `auth.role.read`.

5. **`POST /v1/orgs/{org_id}/roles/list`**
   - Lists role assignments within the organization with optional filtering by role name, membership ID, active-only status, and keyset cursor pagination (`limit`, `cursor`).
   - Requires permission `auth.role.list`.

---

## 2. Invariants & Business Rules Enforced

- **BR-AUTH-003 (Role Assignment Lifecycle & Scope):**
  - Role assignments must specify an authorized role and target membership within the organization scope.
  - Target membership must be in `ACTIVE` or `INVITED` state, and the target employee record must be `ACTIVE`.
- **BR-AUTH-006 (Last-Owner Invariant):**
  - An organization must maintain at least one active `OWNER` assignment.
  - Ending or changing an `OWNER` assignment when only one active owner exists is strictly prohibited.
  - Backed by PostgreSQL trigger `org_role_assignment_preserve_owner` raising `P7501` (handled in store and mapped to `kernel.ErrLastOrgOwner`), serialized with `SELECT ... FOR UPDATE` row locks on the organization root.
  - Concurrent race tests verify that two concurrent owner reductions leave exactly one active owner and fail the conflicting attempt.
- **BR-AUTH-007 (Separation of Duties & No Escalation):**
  - Only an `OWNER` can assign the `OWNER` role (`ErrCannotAssignOwner`). An `ADMIN` cannot grant `OWNER`.
  - Self-assignment and self-modification are strictly forbidden (`ErrSelfAssignmentForbidden`, `ErrSelfModificationForbidden`).
  - Self-reduction: An `OWNER` may end their own owner role only if another active owner exists for the organization. Non-owners cannot end their own roles.
- **BR-AUTH-010 (Fresh Authentication):**
  - Every mutating role operation (`assign`, `end`, `change`) enforces fresh authentication within the last 15 minutes (`actor.RequireFresh()`).
- **BR-AUTH-014 (Immediate Effect):**
  - Changes take effect immediately upon transaction commit without cache lag; subsequent authorization queries reflect the new authority.
- **Organization Root Lifecycle Guards:**
  - `ARCHIVED` organizations reject all role modifications (`ErrOrganizationArchived`).
  - `SUSPENDED` organizations reject role additions and role changes (`ErrOrganizationSuspended`), but permit role endings/reductions to allow emergency offboarding.
- **Audit & Replay Idempotency:**
  - Every command accepts an `operation_id` (UUID). Exact replays return `replayed: true` and the previously generated assignment ID without re-executing or creating duplicate audit rows.
  - Reusing an `operation_id` with different parameters is detected and rejected with `ErrRoleOperationReused`.
  - Role change executes in a single transaction producing exactly one audit event with complete lineage.

---

## 3. OpenAPI Contract

- Updated `contracts/openapi/v1.yaml` under tag `enterprise-access`.
- Added paths:
  - `/v1/orgs/{org_id}/roles/assign`
  - `/v1/orgs/{org_id}/roles/{assignment_id}/end`
  - `/v1/orgs/{org_id}/roles/{assignment_id}/change`
  - `/v1/orgs/{org_id}/roles/{assignment_id}/read`
  - `/v1/orgs/{org_id}/roles/list`
- Added schemas:
  - `EnterpriseRoleAssignRequest`
  - `EnterpriseRoleEndRequest`
  - `EnterpriseRoleChangeRequest`
  - `EnterpriseRoleAssignmentView`
  - `EnterpriseRoleResult`
  - `EnterpriseRoleChangeResult`
  - `EnterpriseRolePageInput`
  - `EnterpriseRoleListRequest`
  - `EnterpriseRolePage`
- Validation:
  - `vacuum lint -r contracts/openapi/vacuum-rules.yaml contracts/openapi/v1.yaml --no-update-check` -> 0 errors.
  - `bash scripts/check-compat.sh` -> 0 errors.

---

## 4. Test Evidence & Validation

Real tests executed against PostgreSQL 18 / PostGIS 3.6 fixture:

1. **Domain Tests (`internal/modules/orgaccess/domain`):**
   - `TestCanAssignRoleEscalation`: Verifies Admin cannot assign Owner; Owner can assign Owner; Admin can assign Fleet Manager.
   - `TestCanModifyAssignmentSelf`: Verifies self-assignment, self-change, and self-end rules.
   - Result: `PASS` (0.019s).

2. **Application Tests (`internal/modules/orgaccess/application`):**
   - `TestRoleService_PolicyChecks`: Verifies fresh authentication requirements, permission checks, and root/member validation.
   - Result: `PASS`.

3. **Store Integration Tests (`internal/modules/orgaccess/adapters`):**
   - `TestRoleStore_AssignRole_PositiveAndReplay`: Assigns role, verifies audit event emission, verifies exact replay idempotency (`replayed: true`), detects operation ID reuse conflict.
   - `TestRoleStore_AssignRole_NegativeGuards`: Verifies rejection on archived org, rejection on suspended org, rejection on ended/inactive member, escalation prevention (Admin -> Owner), self-assignment prevention.
   - `TestRoleStore_EndRole_PositiveAndLastOwner`: Ends role successfully, verifies reduction allowed even on suspended org, verifies last-owner protection returning `ErrLastOrgOwner`.
   - `TestRoleStore_ConcurrentLastOwnerContention`: Concurrent race between two active owners attempting to end their roles simultaneously. Exactly one succeeds, one fails with `ErrLastOrgOwner`. Exactly one owner remains.
   - `TestRoleStore_ChangeRole_Atomicity`: Atomically changes role in single transaction, verifies single audit event emission with lineage, verifies last-owner protection on role downgrade.
   - `TestRoleStore_ReadAndListRoles`: Reads assignment by ID, lists roles with filtering and cursor pagination.
   - Result: `PASS` (3.96s).

4. **HTTP Integration Tests (`internal/modules/orgaccess/adapters/http`):**
   - `TestRoles_ComposedHTTP_Lifecycle`: End-to-end HTTP lifecycle using real company web session cookies, CSRF tokens, origin headers, fresh authentication, role assignment, role change, role ending, and error cases (forbidden, unauthenticated, validation).
   - Result: `PASS` (0.74s).

5. **Full Module Test Suite:**
   - Ran `go test -v -tags=integration ./internal/modules/orgaccess/...` -> ALL PASS.

6. **Plan Consistency:**
   - `python3 docs/enterprise/tools/check_plan.py` -> `tasks 348; rules 545; use cases 167; screens 145; problems 0`.

7. **Git Whitespace:**
   - `git diff --check` -> Clean, 0 whitespace errors.

---

## 5. Delivery Checklist

- [x] Bounded task EMP-AUTH-005 implemented in isolated worktree `.worktrees/enterprise-suite-execution`.
- [x] PostGIS and unit tests passing.
- [x] OpenAPI specification updated and validated (`vacuum lint` & `check-compat.sh`).
- [x] Evidence documented in `docs/enterprise/evidence/f1-auth005-role-management.md`.
- [x] Decisions documented in `docs/enterprise/16-decisions.md` (EDR-046).
- [x] Backlog updated in `docs/enterprise/backlog/ep03-identity-access.md`.
- [x] Progress updated in `docs/planning/PROGRESS.md`.
- [x] Ready for commit and push to `origin/dev` (updating PR #203).

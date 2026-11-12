# F1 organization ownership transfer and last-owner invariant — EMP-ORG-010

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
Source parent: `845d1d94` (EMP-AUTH-005, PR #203)  
Owned worktree: `.worktrees/enterprise-suite-execution`; isolated local branch `codex/enterprise-org010`, publishing to maintained remote `dev` (updating PR #203).  
Main workspace and checkouts preserved.  

## 1. Summary

EMP-ORG-010 implements the two-party organization ownership transfer protocol, enforcing the legal handover of organization ownership while preserving the core invariant that an organization is never left ownerless (BR-AUTH-006).

Rules and use cases governed:
- `BR-ORG-026`: An organization owner must hold a personal account; enterprise-managed accounts (`accounts.enterprise_managed_org_id IS NOT NULL`) cannot hold `OWNER` or receive an ownership transfer proposal.
- `BR-ORG-034`: Ownership transfer proposal requires initiator to hold `OWNER` role, fresh authentication, and target member to be in `ACTIVE` state with an active employee identity. Only one pending transfer may exist per organization. Validity is 7 days.
- `BR-ORG-035`: Ownership transfer acceptance requires the target member to authenticate with a fresh session and explicitly accept. The initiator may cancel, or the target may decline, transitioning the transfer to `CANCELLED` or `DECLINED`. Stale or expired transfers transition to `EXPIRED`.
- `BR-AUTH-006`: Last-owner invariant. An organization must have at least one active owner at all times. During acceptance, the target member is granted `OWNER` before the initiator's role is ended or converted to `ADMIN`.
- `UC-ORG-09`: Safe transfer of organization ownership with voluntary step-down to administrator.
- `EDR-047`: Two-party ownership transfer protocol specification, 7-day validity, and automated cancellation triggers.

## 2. API Surface and Routing

Mounted under `/v1/orgs/{org_id}/ownership-transfer` with dedicated `__Host-abv_company` cookie, exact HTTPS Origin/Host, CSRF token, and fresh session re-verification:

1. `POST /v1/orgs/{org_id}/ownership-transfer/propose`:
   - Guarded by permission `org.ownership.transfer`, write: true, fresh: true, admission class `org.ownership_transfer`.
   - Payload: `schema_version` (1), `operation_id` (UUID), `target_membership_id` (UUID), `step_down_to_admin` (bool).
   - Validates target membership exists, is `ACTIVE`, employee is `ACTIVE`, target account is personal (not managed), and target does not already hold `OWNER`.
   - Returns 200 with `EnterpriseOwnershipTransferResult` (`transfer` view and `replayed` boolean).
   - Replay idempotency via partial unique index on `(org_id, operation_id)`.

2. `POST /v1/orgs/{org_id}/ownership-transfer/read`:
   - Guarded by permission `org.ownership.transfer`, write: false, fresh: true (per BR-AUTH-010).
   - Returns 200 with `EnterpriseOwnershipTransferView` for the current pending transfer if any.

3. `POST /v1/orgs/{org_id}/ownership-transfer/accept`:
   - Requires fresh session from the target member.
   - Payload: `transfer_id` (UUID), `operation_id` (UUID).
   - Atomically grants `OWNER` role to target member (`org_role_assignments`).
   - If `step_down_to_admin` is true, ends initiator's `OWNER` role and assigns `ADMIN` role.
   - Emits audit events `org.ownership_transfer.accepted` and corresponding role assignment events.
   - Returns 200 with `EnterpriseOwnershipTransferResult`.

4. `POST /v1/orgs/{org_id}/ownership-transfer/cancel`:
   - Requires fresh session from initiator (to cancel) or target member (to decline).
   - Payload: `transfer_id` (UUID), `operation_id` (UUID).
   - Transitions state to `CANCELLED` (if initiator) or `DECLINED` (if target).
   - Emits audit event `org.ownership_transfer.cancelled` or `org.ownership_transfer.declined`.
   - Returns 200 with `EnterpriseOwnershipTransferResult`.

## 3. Database Migration and Integrity

Additive migration `000185_ownership_transfers.sql`:
- Table `org_ownership_transfers` with forced Row Level Security (RLS) and strict tenant scope policy (`org_ownership_transfers_tenant_isolation`).
- Partial unique index `org_ownership_transfers_one_pending`: enforces at most one `PENDING` transfer per organization.
- Partial unique index `org_ownership_transfers_operation_id_key`: enforces idempotency on `(org_id, operation_id)`.
- Foreign keys with composite tenant scope: `(org_id, initiator_membership_id)` and `(org_id, target_membership_id)` referencing `org_memberships(org_id, id)`.
- `org_ownership_transfer_guard`: trigger enforcing root organization is `ACTIVE`, fields `(initiator_membership_id, target_membership_id, step_down_to_admin, created_at, expires_at)` are immutable, and terminal states (`ACCEPTED`, `CANCELLED`, `DECLINED`, `EXPIRED`) cannot be mutated.
- `org_ownership_transfer_consume_membership`: trigger on `org_memberships` that automatically transitions an open `PENDING` transfer to `CANCELLED` when the target membership transitions to `SUSPENDED` or `ENDED`.
- `org_ownership_transfer_consume_organization`: trigger on `organizations` that automatically transitions an open `PENDING` transfer to `CANCELLED` when the organization transitions to `ARCHIVED` or `CLOSED`.
- `org_identity_keep_history`: trigger preventing `DELETE` and `TRUNCATE`.

## 4. Test Evidence

### Real PostGIS Integration Tests
Database container: `postgis/postgis:18-3.6` on port `32771`.

1. `TestOwnershipTransfer_Propose_PositiveAndReplay` (PASS, 0.77s):
   - Verified proposal creation, field retention, 7-day expiry calculation, and exact replay idempotency.
2. `TestOwnershipTransfer_Propose_NegativeGuards` (PASS, 0.78s):
   - Self-transfer rejected (`ErrSelfTransferForbidden`).
   - Managed account target rejected (`ErrOwnerRequiresPersonalAccount`).
   - Target already owner rejected (`ErrTargetAlreadyOwner`).
   - Target suspended/ended rejected (`ErrTargetNotActive`).
   - Second concurrent proposal rejected (`ErrTransferPending`).
3. `TestOwnershipTransfer_Accept_PositiveWithStepDown` (PASS, 0.71s):
   - Target accepts transfer. Target becomes `OWNER`. Initiator steps down to `ADMIN`.
   - Last-owner trigger `org_role_assignment_preserve_owner` verified intact throughout execution.
   - Replay idempotency returns `replayed: true`.
4. `TestOwnershipTransfer_Accept_WithoutStepDown` (PASS, 0.86s):
   - Co-ownership transfer: target becomes `OWNER` while initiator remains `OWNER`.
5. `TestOwnershipTransfer_CancelAndDecline` (PASS, 0.86s):
   - Initiator cancellation transitions to `CANCELLED`.
   - Target declination transitions to `DECLINED`.
   - Non-participant attempts rejected with `ErrNotTransferParticipant`.
6. `TestOwnershipTransfer_MembershipEndTrigger` (PASS, 0.86s):
   - When target membership transitions to `ENDED`, database trigger automatically cancels pending transfer.
   - Subsequent accept attempt rejected with `ErrTransferNotPending`.
7. `TestOwnershipTransferThroughComposedCompanyHTTP` (PASS, 0.74s):
   - End-to-end HTTP pipeline via `enterprisesuite.Register` router.
   - Verified company session cookies, CSRF tokens, origin verification, and fresh authentication.
   - Verified propose, duplicate propose rejection, managed account rejection, self-transfer rejection, read, target member accept, and target cancel.

### Static & Compatibility Checks
- OpenAPI v1.yaml lint: `vacuum lint -r contracts/openapi/vacuum-rules.yaml contracts/openapi/v1.yaml --no-update-check` -> 0 errors.
- Schema & Runbook compatibility: `bash scripts/check-compat.sh` -> compat ok (0 errors).
- Enterprise Plan Checker: `python3 docs/enterprise/tools/check_plan.py` -> tasks 348; rules 545; use cases 167; screens 145; problems 0.
- Whitespace & Diff check: `git diff --check` -> clean (0 issues).

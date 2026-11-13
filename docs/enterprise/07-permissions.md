# Permissions, roles, scopes and active context

Status: **EMP-AUTH-001/002/003/004 LOCAL_DONE / INTEGRATION_PENDING**, 2026-11-11; shared current-request authorization implemented; remaining module features PLANNED. Module code `AUTH`. This file is both the module specification for authorization ([00-conventions](00-conventions.md) §4) and the single registry of roles and permission keys for every other module. Tasks: `EMP-AUTH-001…011` in [backlog/ep03-identity-access](backlog/ep03-identity-access.md).

Evidence: EMP-AUTH-001 adds the code-frozen `orgaccess/domain` catalog and [matrix evidence](evidence/f1-auth001-role-permission-catalog.md), without assigning roles or authorizing requests. Station representation (`administrator`, `manager` with four scopes, `backend/internal/modules/stationprofile/domain/claim.go`) and the admin-suite staff catalog (ADR-023, `backend/internal/modules/staff`) remain separate. EMP-AUTH-002 adds [assignment persistence and locked membership/owner SQL policy](evidence/f1-auth002-role-assignment-persistence.md); EMP-AUTH-003 adds the [current authority service and guarded request binding](evidence/f1-auth003-current-authorization.md). EMP-AUTH-004 adds the [default-off pilot adapter](evidence/f1-auth004-pilot-entitlements.md); business record adapters and production routes remain owning-task work.

## 1. Purpose

Decide, on the server and for every action, whether this account may do this to this record now. The organization's managers need to give each employee exactly the access the job needs; employees need to see only what concerns them; the platform needs one rule that every module calls instead of ad-hoc checks.

## 2. Scope

- **Basic (F1):** fixed system roles, role assignments with organization and own-records scopes, the authorization service, organization context in every request, an entitlement gate, a conformance suite generated from the matrix in §7.3.
- **Next (F2–F3):** scopes by unit, cost centre, team and asset; temporary delegation and substitutes; auditor role and export gating.
- **Later (F6):** custom roles as named bundles of the same permission keys. Justification: fixed roles keep the authorization surface testable while the product is piloted; custom bundles are added when at least three pilot organizations need a combination the fixed roles cannot express, and they never introduce new permission keys.
- **Out of scope:** attribute-based policies written by customers, per-field permissions, and any permission decided on the client.

## 3. Entities and ownership

| Entity | Owner | Content | Invariants |
|---|---|---|---|
| Role | Platform (code-defined catalog, versioned) | Key, name, permission keys | A role's permissions change only with a new catalog version. |
| Permission key | Platform (code-defined) | `<module>.<resource>.<action>` | Defined once in §7.2. |
| Role assignment | Organization | Membership, role, scope type, scope target, `valid_from`, `valid_to`, assigned by, reason | Belongs to the membership's organization; scope target belongs to the same organization; periods never overlap for the same (membership, role, scope). |
| Delegation | Organization | A role assignment flagged as delegated, with delegator and end date | End date mandatory; never wider than the delegator's rights at creation. |
| Active context | Device and web session | Personal, or one organization | Chosen by the user; never inferred from a record. |

Relations: an account has zero or more memberships; a membership has one or more role assignments; a role assignment points to one scope target. A station authority ([modules/station-claims](modules/station-claims.md)) is the scope target of station roles.

## 4. Business rules

- **BR-AUTH-001 — Four conditions.** An action is allowed only when all hold at the server's time: (1) the account has an active membership in the organization; (2) a valid role assignment of that membership contains the permission and its scope covers the target record; (3) the organization's state and plan allow the action; (4) the record's own rules (state, lock, separation of duties) allow it.
- **BR-AUTH-002 — Client identifiers are not proof.** Organization, membership, role or record identifiers sent by a client only select; the server recomputes every condition (BR-TEN-003).
- **BR-AUTH-003 — Permissions add up; restrictions do not disappear.** A member with several roles has the union of their permissions in each scope. Separation-of-duties rules still apply to each record (BR-REV-003).
- **BR-AUTH-004 — Scope coverage.** `ORG` covers every record of the organization. `UNIT` and `COST_CENTRE` cover records carrying that unit or cost centre. `TEAM` covers records authored by or assigned to an employee of that team. `ASSET` covers records of that asset. `STATION` covers one station under an active station authority. `SELF` covers records the member authored or is assigned to.
- **BR-AUTH-005 — Historical scope.** Coverage uses the unit, cost centre and asset stored on the record when it was created, not the current allocation of the asset or person.
- **BR-AUTH-006 — Last organization owner.** An organization always has at least one active `OWNER`. The last one cannot be removed, demoted, suspended or offboarded; ownership is transferred first ([modules/organizations](modules/organizations.md)).
- **BR-AUTH-007 — No escalation.** A member assigns only roles whose permissions are all within the member's own effective permissions for that scope. Only an `OWNER` assigns `OWNER`. Nobody changes their own assignments except an `OWNER` reducing their own, subject to BR-AUTH-006.
- **BR-AUTH-008 — Delegation is bounded.** A delegation has a mandatory end (default maximum 90 days, organization-configurable up to 180), cannot exceed the delegator's rights, ends when the delegator's right ends, and every action under it records the delegation (BR-AUD-005).
- **BR-AUTH-009 — Plans do not grant roles.** A subscription enables capabilities and limits for the organization. It never gives a person a role, and losing a capability never changes role assignments.
- **BR-AUTH-010 — Sensitive actions need fresh authentication.** Role and ownership changes, credential resets, full exports and API key issue require a session authenticated within the fresh window (existing convention: access token issued less than 15 minutes ago).
- **BR-AUTH-011 — Location and document content are separate permissions.** Seeing a live position, a recorded track or a document's content each needs its own permission, is not implied by reading the trip or expense, and is audited (BR-AUD-003).
- **BR-AUTH-012 — Context is explicit.** The app and the web always show whether the user is in personal mode or in a named organization. Changing context is a user action. A request names its organization in the path.
- **BR-AUTH-013 — Denials reveal nothing.** A caller without membership receives not-found (BR-TEN-006). A member without the permission receives `FORBIDDEN` with the missing permission key and no record data.
- **BR-AUTH-014 — Changes take effect at once.** Ending or changing an assignment or membership applies to the next request. Cached sessions, queued operations and open screens do not extend a removed right ([10-offline-sync](10-offline-sync.md) defines what happens to operations recorded offline before the change).
- **BR-AUTH-015 — Station roles need a live authority.** Station permissions are evaluated against the station authority's current state; a suspended, expired or revoked authority denies them whatever the role assignment says.

## 5. Use cases

**UC-AUTH-01 — Assign a role.** Actor: member with `auth.role.assign` on the target scope. Preconditions: target membership active; fresh authentication. Main flow: choose member, role, scope and optional end date → server checks BR-AUTH-007 and scope ownership → assignment created → audit event → the member's next request sees it. Alternatives: role not grantable by the actor → `FORBIDDEN`; overlapping identical assignment → `CONFLICT`; scope target archived → `VALIDATION_FAILED`. Result: assignment active from `valid_from`.

**UC-AUTH-02 — End or change an assignment.** Actor: as UC-AUTH-01. Main flow: set `valid_to` now (or create a new assignment with the new scope and end the old one in one transaction) with a reason. Alternatives: would remove the last `OWNER` → `LAST_OWNER`; actor changing own rights upward → `FORBIDDEN`. Result: effective on the next request (BR-AUTH-014).

**UC-AUTH-03 — Delegate temporarily.** Actor: member with `auth.delegation.manage` who holds the role being delegated. Main flow: choose substitute, role, scope, start and end → server checks BR-AUTH-008 → delegated assignment created → both parties notified. Alternatives: end missing or beyond the maximum → `VALIDATION_FAILED`; substitute would approve their own records → allowed to hold the role, still blocked per record by BR-REV-003. Result: substitute acts until the end date; the delegation ends automatically.

**UC-AUTH-04 — Switch context.** Actor: any signed-in user. Main flow: open the context switcher → the app lists personal mode and each organization with an active membership → user picks one → the app shows that organization's enterprise home. Alternatives: membership suspended → listed as unavailable with the reason; no connection → last known list, marked "not confirmed"; pending operations of another context stay queued untouched (BR-TEN-004). Result: context changed on this device only.

**UC-AUTH-05 — Authorize a request (system).** Actor: authorization service. Flow: authenticate session (and device proof for app writes) → resolve organization and membership → check organization state → find a covering assignment → check entitlement → run record rules → allow, or answer with the first failing condition's error (§10). Result: decision plus audit event when the action is a write or a sensitive read.

**UC-AUTH-06 — See my access.** Actor: any member. Flow: open "My access" → list of roles, scopes, end dates and delegations received. Result: the member understands why something is or is not available.

## 6. States and transitions

Role assignment:

| From | To | Actor | Validation | Effect | Reversal |
|---|---|---|---|---|---|
| — | `SCHEDULED` | Assigner | BR-AUTH-007; `valid_from` in the future | None until start | End before start |
| — / `SCHEDULED` | `ACTIVE` | Assigner or time | Membership active | Permissions apply | End the assignment |
| `ACTIVE` | `ENDED` | Assigner, time (`valid_to`), offboarding | BR-AUTH-006 | Permissions stop on next request | New assignment; history kept |
| `ACTIVE` | `INEFFECTIVE` | System | Membership suspended, organization suspended, or station authority not active | Permissions denied while the cause lasts | Automatic when the cause ends |

`INEFFECTIVE` is derived at evaluation time, not stored. Assignments are never deleted.

## 7. Roles and permissions

### 7.1 System roles (catalog `org-roles-v1`)

| Role | Interface (pt-BR) | Intent | Typical scope |
|---|---|---|---|
| `OWNER` | Proprietário | Final authority over the organization, its plan and its owners | ORG |
| `ADMIN` | Administrador | Runs the organization: structure, people, access | ORG |
| `FLEET_MANAGER` | Gestor de frota | Assets, assignments, missions, maintenance | ORG or UNIT |
| `SUPERVISOR` | Supervisor | Follows operations and reviews trips of a unit or team | UNIT or TEAM |
| `FINANCE` | Financeiro | Expenses, documents, budgets, payables, periods | ORG, UNIT or COST_CENTRE |
| `APPROVER` | Aprovador | Decides on submitted records within a scope | UNIT, COST_CENTRE or TEAM |
| `DRIVER` | Motorista | Drives and records own operations | SELF (plus assigned assets) |
| `OPERATIONS_STAFF` | Funcionário operacional | Records fuelings, expenses, checklists and stock movements without driving | SELF or UNIT |
| `AUDITOR` | Auditor | Reads everything in scope; changes nothing | ORG or UNIT |
| `STATION_ADMIN` | Administrador do posto | Manages a station's business data and team | STATION |
| `STATION_MANAGER` | Gerente do posto | Day-to-day station data: prices, promotions, replies | STATION |
| `STATION_ATTENDANT` | Atendente | Confirms voucher redemptions | STATION |
| `DATA_MANAGER` | Gestor de dados | Manages Data API keys and reads usage | ORG |

`STATION_ADMIN` and `STATION_MANAGER` map to the existing representation roles `administrator` and `manager` (STATION_PROFILE_CONTRACT_FREEZE). No generic account role `OWNER` is created outside an organization; the existing freeze ("No `OWNER` role exists anywhere in this scope") keeps applying to station representation.

### 7.2 Permission registry (`org-permissions-v4`)

Every permission used by any module is listed here. A module that needs a new key adds it here first.

| Key | Allows |
|---|---|
| `org.settings.read` / `org.settings.manage` | Read / change organization settings and policies |
| `org.structure.manage` | Create and archive units, cost centres, teams |
| `org.legal_entity.manage` | Declare legal entities and request verification |
| `org.ownership.transfer` | Start an ownership transfer |
| `org.ownership.respond` | Attempt a private ownership proposal read, acceptance or cancellation; actual participant check is mandatory |
| `org.lifecycle.manage` | Request archive or closure |
| `org.group.manage` | Propose and accept consolidation grants |
| `idn.employee.read` / `idn.employee.manage` | Read / create and edit employee records |
| `idn.invitation.manage` | Issue, resend, revoke invitations |
| `idn.credential.manage` | Issue activation codes, reset managed credentials, revoke devices |
| `idn.membership.manage` | Suspend, reactivate, offboard, rehire |
| `auth.role.read` / `auth.role.assign` | Read / change role assignments |
| `auth.delegation.manage` | Create and end delegations of own roles |
| `fleet.asset.read` / `fleet.asset.manage` | Read / register and change assets |
| `fleet.asset.assign` | Assign assets to employees |
| `fleet.reservation.manage` | Reserve and block assets |
| `fleet.supplier.manage` | Maintain suppliers |
| `meter.reading.record` / `meter.reading.correct` | Record a reading / correct or void one |
| `trip.mission.manage` | Plan and assign missions |
| `trip.trip.start` | Start and record own trips |
| `trip.trip.read` | Read trips in scope |
| `trip.trip.close_for_other` | Close another person's trip with a reason |
| `trip.trip.review` | Approve, return or reject trips |
| `check.checklist.execute` / `check.template.manage` | Fill a checklist / maintain templates |
| `check.incident.report` / `check.incident.manage` | Report / handle incidents and damages |
| `route.plan.manage` | Create and change route plans |
| `gps.location.read_live` | See live positions of trips in progress |
| `gps.track.read` | See recorded tracks |
| `gps.geofence.manage` | Maintain geofences and location alerts |
| `fuel.fueling.record` | Record own fuelings |
| `fuel.fueling.read` | Read fuelings in scope |
| `fuel.fueling.review` | Approve, return or reject fuelings |
| `fuel.fueling.amend` | Amend or void approved fuelings |
| `doc.document.attach` | Upload and link documents to own records |
| `doc.document.read` | See that documents exist and their metadata |
| `doc.document.view_content` | Open a document's content |
| `doc.document.confer` | Record a conference result |
| `doc.document.allocate` | Allocate document amounts |
| `exp.expense.record` | Record own expenses |
| `exp.expense.read` | Read expenses in scope |
| `exp.expense.review` | Approve, return or reject expenses |
| `exp.category.manage` | Maintain expense categories |
| `appr.policy.manage` | Maintain approval policies |
| `budg.budget.read` / `budg.budget.manage` | Read / maintain budgets and holds |
| `fin.payable.read` | Read payables and payments |
| `fin.payment.record` | Record payments, reimbursements and advances |
| `fin.period.close` / `fin.period.reopen` | Lock / reopen a period |
| `maint.request.create` | Ask for maintenance |
| `maint.workorder.manage` / `maint.workorder.review` | Run / approve work orders |
| `maint.plan.manage` | Maintain preventive plans and expiries |
| `stock.tank.manage` | Maintain internal tanks |
| `stock.movement.record` | Record receipts, dispensing, transfers |
| `stock.reconcile` | Record measurements and adjustments |
| `kpi.dashboard.read` | Read indicators |
| `find.finding.read` / `find.finding.resolve` | Read / decide findings |
| `rpt.export.run` | Run exports and reports |
| `audit.event.read` | Read the audit trail |
| `import.batch.run` | Import files |
| `notif.rule.manage` | Maintain organization alert rules |
| `integ.connection.manage` | Maintain integrations and their secrets |
| `subs.plan.read` / `subs.plan.manage` | Read / change plan and billing |
| `data.key.manage` / `data.usage.read` | Manage API keys / read usage |
| `claim.claim.submit` | Submit a station claim for the organization |
| `stn.team.manage` | Manage a station's team |
| `stn.profile.edit` | Edit a station's business profile |
| `stn.price.publish` | Publish announced prices and promotions |
| `stn.reply.official` | Reply to reviews as the business |
| `stn.metrics.read` | Read station interaction metrics |
| `agree.agreement.manage` | Propose, accept and end agreements |
| `agree.authorization.issue` | Issue fueling authorizations (fleet side) |
| `agree.redemption.confirm` | Confirm redemptions (station side) |
| `back.support.approve` | Approve a platform support session |

### 7.3 Matrix: action × role × scope

Cell values: `O` allowed in the whole assigned scope; `S` allowed on own records and assigned assets only; `—` not allowed. A role assigned to a unit, cost centre or team applies `O` inside that scope only. Station roles apply inside their station.

| Action (permission) | OWNER | ADMIN | FLEET_MGR | SUPERVISOR | FINANCE | APPROVER | DRIVER | OPS_STAFF | AUDITOR |
|---|---|---|---|---|---|---|---|---|---|
| Invite (`idn.invitation.manage`) | O | O | — | — | — | — | — | — | — |
| Manage employees (`idn.employee.manage`) | O | O | — | — | — | — | — | — | — |
| Read employees (`idn.employee.read`) | O | O | O | O | O | O | — | — | O |
| Reset corporate access (`idn.credential.manage`) | O | O | — | — | — | — | — | — | — |
| Offboard (`idn.membership.manage`) | O | O | — | — | — | — | — | — | — |
| Assign roles (`auth.role.assign`) | O | O¹ | — | — | — | — | — | — | — |
| Delegate own role (`auth.delegation.manage`) | O | O | O | O | O | O | — | — | — |
| Read settings (`org.settings.read`) | O | O | O | O | O | O | — | — | O |
| Change settings (`org.settings.manage`) | O | O | — | — | — | — | — | — | — |
| Manage structure (`org.structure.manage`) | O | O | — | — | — | — | — | — | — |
| Declare legal entities (`org.legal_entity.manage`) | O | O | — | — | — | — | — | — | — |
| Transfer ownership (`org.ownership.transfer`) | O | — | — | — | — | — | — | — | — |
| Respond to ownership proposal (`org.ownership.respond`) | O | O | O | O | O | O | O | O | O |
| Manage assets (`fleet.asset.manage`) | O | O | O | — | — | — | — | — | — |
| Read assets (`fleet.asset.read`) | O | O | O | O | O | O | S | S | O |
| Assign vehicle (`fleet.asset.assign`) | O | O | O | O | — | — | — | — | — |
| Record reading (`meter.reading.record`) | O | O | O | O | — | — | S | S | — |
| Correct reading (`meter.reading.correct`) | O | O | O | O | — | — | — | — | — |
| Plan missions (`trip.mission.manage`) | O | O | O | O | — | — | — | — | — |
| Start trip (`trip.trip.start`) | S | S | S | S | — | — | S | — | — |
| Read trips (`trip.trip.read`) | O | O | O | O | O | O | S | S | O |
| Close for another (`trip.trip.close_for_other`) | O | O | O | O | — | — | — | — | — |
| Review trips (`trip.trip.review`) | O | O | O | O | — | O | — | — | — |
| Fill checklist (`check.checklist.execute`) | S | S | S | S | — | — | S | S | — |
| Report incident (`check.incident.report`) | S | S | S | S | — | — | S | S | — |
| See live location (`gps.location.read_live`) | O | O | O | O | — | — | S² | — | — |
| See recorded track (`gps.track.read`) | O | O | O | O | — | — | S² | — | O³ |
| Record fueling (`fuel.fueling.record`) | S | S | S | S | — | — | S | S | — |
| Read fuelings (`fuel.fueling.read`) | O | O | O | O | O | O | S | S | O |
| Review fuelings (`fuel.fueling.review`) | O | O | O | — | O | O | — | — | — |
| Amend approved fueling (`fuel.fueling.amend`) | O | O | O | — | O | — | — | — | — |
| Attach document (`doc.document.attach`) | S | S | S | S | O | — | S | S | — |
| See documents (`doc.document.view_content`) | O | O | O | — | O | O | S | S | O |
| Confer document (`doc.document.confer`) | O | O | — | — | O | — | — | — | — |
| Register expense (`exp.expense.record`) | S | S | S | S | S | — | S | S | — |
| Read expenses (`exp.expense.read`) | O | O | O | O | O | O | S | S | O |
| Approve (`exp.expense.review`) | O | O | — | — | O | O | — | — | — |
| Manage budgets (`budg.budget.manage`) | O | O | — | — | O | — | — | — | — |
| Record payment (`fin.payment.record`) | O | — | — | — | O | — | — | — | — |
| Close / reopen period (`fin.period.close`, `.reopen`) | O | — | — | — | O | — | — | — | — |
| Manage work orders (`maint.workorder.manage`) | O | O | O | — | — | — | — | — | — |
| Ask for maintenance (`maint.request.create`) | S | S | O | O | — | — | S | S | — |
| Record stock movement (`stock.movement.record`) | O | O | O | — | — | — | — | O | — |
| Read indicators (`kpi.dashboard.read`) | O | O | O | O | O | — | — | — | O |
| Resolve findings (`find.finding.resolve`) | O | O | O | — | O | — | — | — | — |
| Export (`rpt.export.run`) | O | O | O | — | O | — | — | — | O |
| Read audit trail (`audit.event.read`) | O | O | — | — | — | — | — | — | O |
| Import files (`import.batch.run`) | O | O | O | — | O | — | — | — | — |
| Manage integrations (`integ.connection.manage`) | O | O | — | — | — | — | — | — | — |
| Manage plan (`subs.plan.manage`) | O | — | — | — | — | — | — | — | — |
| Read plan (`subs.plan.read`) | O | O | — | — | O | — | — | — | — |
| Approve support session (`back.support.approve`) | O | O | — | — | — | — | — | — | — |
| Submit station claim (`claim.claim.submit`) | O | O | — | — | — | — | — | — | — |
| Manage agreements (`agree.agreement.manage`) | O | O | O⁴ | — | O | — | — | — | — |
| Issue fueling authorization (`agree.authorization.issue`) | O | O | O | O | — | — | — | — | — |

¹ `ADMIN` cannot assign `OWNER` (BR-AUTH-007). ² A driver sees the track and position of their own trips only. ³ An auditor reads recorded tracks, never live positions. ⁴ Fleet side only.

Station and data roles:

| Action (permission) | STATION_ADMIN | STATION_MANAGER | STATION_ATTENDANT | DATA_MANAGER | OWNER / ADMIN |
|---|---|---|---|---|---|
| Represent a station: edit profile (`stn.profile.edit`) | O | O | — | — | by assignment only |
| Publish prices and promotions (`stn.price.publish`) | O | O | — | — | by assignment only |
| Official reply (`stn.reply.official`) | O | O | — | — | by assignment only |
| Manage station team (`stn.team.manage`) | O | — | — | — | by assignment only |
| Read station metrics (`stn.metrics.read`) | O | O | — | — | by assignment only |
| Station side of agreements (`agree.agreement.manage`) | O | — | — | — | by assignment only |
| Confirm redemption (`agree.redemption.confirm`) | O | O | O | — | by assignment only |
| Manage API keys (`data.key.manage`) | — | — | — | O | O |
| Read API usage (`data.usage.read`) | — | — | — | O | O |

An organization `OWNER` or `ADMIN` does not represent a station by being owner or admin. Station permissions always come from a station role assignment under an active station authority (BR-AUTH-015).

### 7.4 Initial source-catalog interpretation

EMP-AUTH-001 freezes the 13 canonical roles and 84 registry keys above.
The initial v1 matrix covers 61 keys; the other 23 remain registered but
unassigned, with denied coverage even for OWNER. Proposed role rows in
owning module documents do not silently extend this matrix. A later grant
needs an explicit matrix revision and catalog version change.
EMP-ORG-004 explicitly adopts the module's recommended legal-management row
for OWNER and ADMIN under the maintainer's autonomous decision authorization
(EDR-037). Version org-permissions-v2 covers 62 keys, leaving 22 unassigned.
These are exactly two new positive cells; all other 1,090 cells and the four
independent current-authority conditions remain unchanged. This row grants no
verification, station representation or external-registry authority.

The literal stock.reconcile key is retained as the registry's sole
two-segment compatibility exception; it stays unassigned. Other keys use
the documented three-segment format; no new alias is inferred.

O and S are static coverage descriptors, not a request decision. Every
positive descriptor retains the four conditions of BR-AUTH-001. The four
footnotes are explicit restrictions. For agree.agreement.manage, main-table
roles act on the fleet side, and STATION_ADMIN acts on the station side
with live station authority; the station table's "by assignment only" does
not turn an organization OWNER/ADMIN into a station representative. Fleet
and station views retain separate party scopes (station-commerce §7).

Catalog lookup does not persist assignments, interpret a subscription as a
role, evaluate current scope/time, waive self-review rules or enable the
membership database policy. Actual authority remains AUTH-002/003 work.

## 8. Offline, concurrency, idempotency and corrections

EMP-AUTH-002 persistence interpretation (2026-11-11): F1 stores ORG with
the organization's own ID and SELF with the assignment's membership ID.
OWNER uses ORG; station roles stay unavailable until STATION/live-authority
storage exists. A bound INVITED membership may hold staged periods for an
atomic admission, without becoming a current authorization candidate.
Managed memberships cannot receive OWNER. All intervals are half-open UTC;
PostgreSQL persists microsecond precision. Ending records a server timestamp
and revision, including cancellation before a scheduled start, without
rewriting the original validity or deleting history.

The shared organization lock protects assignment overlap and continuous
surviving-owner coverage through future expiry. The union of the surviving
active owners' periods must cover the remaining time without gaps; adjacent
finite and future intervals are valid. Account-deletion reservations do not
count. Membership end atomically ends every remaining assignment, and
personal-account status reductions use the same owner fence. These storage
guards do not establish an assigner's current permission or fresh session;
AUTH-003/009 and onboarding own those application checks and audit.

- **Offline:** the app caches the member's effective permissions per organization to shape the interface (hide what cannot be done). The cache never authorizes; the server decides when the operation arrives ([10-offline-sync](10-offline-sync.md)). The cached set carries the time it was confirmed and is shown as "not confirmed" after 24 hours offline.
- **Concurrency:** two administrators changing the same member's assignments act on a version of the member's assignment set; the second write on a stale version fails with `CONFLICT`. Removing the last owner is checked inside the transaction with a row lock on the organization.
- **Idempotency:** assignment commands carry an idempotency key; a repeat returns the first result.
- **Corrections:** an assignment made by mistake is ended, not deleted; the audit trail keeps both events.

## 9. Events, notifications and integrations

Produced: `auth.assignment.created`, `auth.assignment.ended`, `auth.delegation.created`, `auth.delegation.ended`, `auth.access.denied` (sampled, for security review). Consumed: `idn.membership.suspended`, `idn.membership.ended`, `org.organization.suspended`, `claim.authority.suspended` (to evaluate `INEFFECTIVE`). Notifications: the affected member is told when a role is given, changed, delegated or removed. No external integration.

## 10. Data, formulas and examples

Decision order and error codes (first failure wins):

| Step | Check | Error |
|---|---|---|
| 1 | Session valid; device proof valid for app writes | `UNAUTHENTICATED` |
| 2 | Active membership of the account in the path organization | `NOT_FOUND` |
| 3 | Membership not suspended | `MEMBERSHIP_SUSPENDED` |
| 4 | Organization state allows the action | `ORG_SUSPENDED`, `ORG_ARCHIVED` |
| 5 | A valid assignment has the permission and covers the record | `FORBIDDEN` (with the permission key) |
| 6 | Fresh authentication when required | `FRESH_AUTH_REQUIRED` |
| 7 | Capability enabled and limit not exceeded | `ENTITLEMENT_REQUIRED`, `LIMIT_REACHED` |
| 8 | Record rules | module error such as `SELF_APPROVAL`, `PERIOD_LOCKED`, `CONFLICT` |

Example 1. Ana is `SUPERVISOR` of unit Norte and `DRIVER`. Bruno, a driver of unit Sul, submits a trip. Ana opens it: step 5 fails, because her supervisor scope does not cover unit Sul and her driver scope covers only her own records. She receives `FORBIDDEN` with `trip.trip.read` and no trip data.

Example 2. Ana submits her own trip in unit Norte and then tries to approve it. Step 5 passes (`trip.trip.review` in unit Norte). Step 8 fails with `SELF_APPROVAL` (BR-REV-003).

Example 3. Vehicle V was in unit Norte in March and moved to unit Sul in April. A March fueling of V stays readable by the Norte finance role and not by the Sul finance role (BR-AUTH-005).

## 11. Screens

| ID | Surface | Objective | Actor | Data | Actions | Rules and permissions | Validations and states |
|---|---|---|---|---|---|---|---|
| SCR-AUTH-01 | PANEL | See who has which access | `auth.role.read` | Members, roles, scopes, end dates, delegations | Filter, open member | BR-AUTH-013 | Empty: "Only you have access so far" with an invite action. Forbidden: standard pattern. |
| SCR-AUTH-02 | PANEL | Give, change or end a role | `auth.role.assign` | Role catalog with plain descriptions, scope picker, end date, reason | Save, end | BR-AUTH-006, 007, 010 | Errors shown next to the field: `LAST_OWNER`, not grantable, overlap. Asks for fresh authentication when needed. |
| SCR-AUTH-03 | PANEL | Delegate to a substitute | `auth.delegation.manage` | Own roles, substitute, period | Create, end early | BR-AUTH-008 | End date required; maximum shown. |
| SCR-AUTH-04 | APP | Choose personal mode or an organization | Any signed-in user | Personal mode, organizations with state, pending operations per context | Switch | BR-AUTH-012, BR-TEN-004 | Offline: list marked "not confirmed". Suspended membership: disabled with reason. Screen-reader label announces the active context. |
| SCR-AUTH-05 | APP, PANEL | Understand own access | Any member | Roles, scopes, end dates | None | — | Empty state not possible (a member has at least one role). |
| SCR-AUTH-06 | APP, PANEL | Explain a denial | Any member | The action that was denied and who can grant it | Go back | BR-AUTH-013 | Shared "no permission" pattern used by every module. Never shows record data. |

## 12. Acceptance, dependencies, risks, phase and microtasks

Acceptance criteria:

1. Every row of §7.3 is covered by a generated test that passes for allowed cells and is denied for `—` cells, in a two-organization fixture.
2. A request with a valid session and another organization's identifier returns not-found with an empty body beyond the error envelope.
3. Ending an assignment denies the member's next request without any session change.
4. The last owner cannot be removed through any path (role end, membership suspension, offboarding, account deletion).
5. A delegated action shows actor and delegation in the audit trail and stops at the end date.
6. Examples 1 to 3 in §10 pass as written.

Dependencies: [modules/identity](modules/identity.md) (memberships), [modules/organizations](modules/organizations.md) (scope targets), [09-architecture](09-architecture.md) (request context), EMP-GOV-001 (tenancy decision).

Risks: role sprawl (kept in check by the fixed catalog); scope confusion when assets move (BR-AUTH-005 and example 3); interface hiding mistaken for authorization (BR-AUTH-002 and the conformance suite).

Phase: F1 for the core; F2 scopes; F3 delegation and auditor; F6 custom roles. Microtasks: EMP-AUTH-001 to EMP-AUTH-011.

Settings-read amendment (EMP-ORG-008, EDR-043): org-permissions-v3 adopts only
its proposed read row above. All other cells retain v2, including the explicit
legal-management row. Roles without a published grant remain denied.

Ownership-response amendment (PR #203 review, EDR-048):
`org-permissions-v4` adds only `org.ownership.respond` with ORG descriptor
coverage for all thirteen fixed roles, including the four station/data roles.
Every v3 cell is preserved. This is permission to attempt a fresh, scoped
response, not to inspect any proposal in the organization. Server-owned attempt
facts bind SELF coverage to the current member; the source transaction checks
actual participant identities under the retained root lock before returning
private state/receipts or changing anything. Station authority requirements are
unchanged. An ACTIVE membership with no live role remains denied until an
ordinary authorized role grant restores coverage. Finer UNIT/COST_CENTRE/TEAM/
ASSET grant management belongs to AUTH-006 (F2).

AUTH-005 review clarification: permission containment is literal, including
coverage and requirements. Neither OWNER nor ADMIN receives a wildcard. Some
ORG templates exceed a single OWNER/ADMIN template; the actor's current union
must actually cover every descriptor. A SELF-narrowed grant is compared at
SELF coverage. OWNER may reduce its own assignment only within its existing
permissions and while another valid owner remains. `auth.role.assign` owns the
five POST management/read routes; there are no `auth.role.end`, `auth.role.read`
or `auth.role.list` aliases. Archived reads and access reductions remain allowed
under current proof; new grants/changes do not.


Authorization-conformance checkpoint (EMP-AUTH-009, EDR-063): the current
independent v4 fixture covers85 keys × 13 roles (1,105 descriptor cells). The
[route inventory](../../contracts/testdata/enterprise/authorization-routes-v1.json)
and [current acceptance profile](15-validation.md#5-authorization-sampling)
distinguish current-source request coverage from future permissions/scopes.
The complete production-router gate already exists in cmd/api; this checkpoint
does not introduce a second runtime registry or expand the fixed policy.

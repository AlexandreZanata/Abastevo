# Enterprise web experience

Status: **PLANNED / RECONSTRUCTED**, 2026-11-11. This specification follows
the [approved company panel decision](evidence/f0-m0-decisions.md#company-web-surface),
the [permission matrix](07-permissions.md) and the module workflows.

## 1. Purpose

Managers need to review field records, manage scoped resources and understand
what the server accepted. Platform reviewers need a separate privileged
surface. Each screen shows its active organization or station and the
current authority for each action.

## 2. Surfaces and information architecture

The company panel lives at `/empresas/` on the configured API origin.
It uses a separate static TypeScript build, cookie session and middleware
from `admin-web/`. Authentication, exact-origin and CSRF checks precede
mutations. Private responses are `no-store`.

- Company setup leads to organizations, units, employees, invitations,
  assets and role scopes. Context switching is explicit.
- Operations include trips, fuelings, documents, expenses and review queues.
  Finance, budgets, maintenance, internal stock, indicators and exports
  appear only when their capability and permission permit them.
- A future station portal has its own station-scoped navigation under an
  admitted business surface. Company-panel approval grants no Stage D
  override.
- Internal staff continue to use the separate admin suite and its network,
  session and staff-role controls. Support content needs an organization-
  approved support session with the exact scope and validity.

## 3. Shared screen states and accessibility

Loading preserves the named context and gives a bounded progress indication.
Empty state explains the permitted next action without fabricating data.
Error state preserves entered data and names a safe retry or correction.
Unavailable state states the disabled capability or offline requirement.
Forbidden state reveals no foreign content or existence hint.

Use semantic labels, keyboard navigation, visible focus, associated field
errors and adequate contrast. State is text as well as color. Large text
must retain primary actions. Confirmation controls state the target,
scope and consequence. A pending request cannot appear successful before
the server acknowledges it.

## 4. Main flows

Company onboarding signs in through the approved session, creates one
organization and first unit, then shows server-confirmed setup state.
Invitations are issued under fresh current authority and their secret
appears once. Managed activation remains outside C1.

Fueling review opens the scoped record and warnings. Document content
requires its independent permission and audited capability. The decision
names the current revision. Returned work includes a reason and the author
sees it at the next sync.

Sync review separates held facts from ordinary approved records. A reviewer
can resolve only the owning record types and cannot approve their own
benefiting record. A competing decision produces a visible conflict.

Export begins with current scope and filters. The job state is visible.
Downloads recheck authority and use an expiring private capability. Full
organization export requires its stronger permissions and fresh proof.

Support approval shows staff actor, exact categories and time. The
organization approves explicitly. Access outside the session remains
closed and the resulting audit is visible to the organization.

## 5. Data and freshness

Lists and details use server pagination and revision. Changing organization
clears prior private results and cancels incompatible pending view work.
A late response must not render into a new context. Filters show time zone,
period and source. Indicator values include their method and basis.
Missing data remains insufficient, never a default zero.

Live operations show collection time, receipt age and quality. Polling
follows [architecture](09-architecture.md) and a stale location is visibly
stale. Navigation visibility never substitutes for API permission.

## 6. Action and failure boundaries

Server-dependent decisions, role changes, budget holds and voucher redemption
are online-only. Forms retain safe input across a retry. An identical
operation retries with its original identity and a correction follows
the owning revision contract. Conflicts present the current result and
preserve the user's proposal where allowed.

Receipt images, location, credentials and tokens stay out of client logs,
URLs, analytics and generic error text. A token intended for a fragment
never enters server request logs. A private capability is neither a
permanent URL nor authority to another document.

## 7. Screen indexes

The indexes below are generated from the canonical module screen rows.

### Company panel

<!-- SCREEN-INDEX:PANEL:BEGIN -->
| Screen | Objective | Specified in |
|---|---|---|
| SCR-AGREE-01 | Review own contract side | [modules/station-commerce.md](modules/station-commerce.md) |
| SCR-APPR-01 | Review the current submission | [modules/expenses-finance.md](modules/expenses-finance.md) |
| SCR-AUTH-01 | See who has which access | [07-permissions.md](07-permissions.md) |
| SCR-AUTH-02 | Give, change or end a role | [07-permissions.md](07-permissions.md) |
| SCR-AUTH-03 | Delegate to a substitute | [07-permissions.md](07-permissions.md) |
| SCR-AUTH-05 | Understand own access | [07-permissions.md](07-permissions.md) |
| SCR-AUTH-06 | Explain a denial | [07-permissions.md](07-permissions.md) |
| SCR-BACK-02 | Approve a support request | [modules/back-office.md](modules/back-office.md) |
| SCR-BUDG-01 | Manage budget basis | [modules/expenses-finance.md](modules/expenses-finance.md) |
| SCR-CHECK-04 | See checklist runs and their issues | [modules/maintenance.md](modules/maintenance.md) |
| SCR-CHECK-05 | Handle reports and release blocked assets | [modules/maintenance.md](modules/maintenance.md) |
| SCR-CHECK-06 | Maintain templates and the critical-item policy | [modules/maintenance.md](modules/maintenance.md) |
| SCR-DATA-01 | Manage authorized keys | [modules/data.md](modules/data.md) |
| SCR-DATA-02 | Inspect usage and methodology | [modules/data.md](modules/data.md) |
| SCR-DOC-02 | Inspect linked receipt | [modules/documents.md](modules/documents.md) |
| SCR-DOC-03 | Confer and allocate | [modules/documents.md](modules/documents.md) |
| SCR-FIN-01 | Distinguish cost and settlement | [modules/expenses-finance.md](modules/expenses-finance.md) |
| SCR-FIND-01 | Work the queue of findings | [modules/intelligence.md](modules/intelligence.md) |
| SCR-FIND-02 | Review one finding and decide | [modules/intelligence.md](modules/intelligence.md) |
| SCR-FIND-03 | Configure detection and reprocess | [modules/intelligence.md](modules/intelligence.md) |
| SCR-FLEET-01 | Find an asset and see the fleet at a glance | [modules/fleet.md](modules/fleet.md) |
| SCR-FLEET-02 | Understand one asset and its history | [modules/fleet.md](modules/fleet.md) |
| SCR-FLEET-03 | Register or edit an asset | [modules/fleet.md](modules/fleet.md) |
| SCR-FLEET-04 | Say who is responsible for and who may use an asset | [modules/fleet.md](modules/fleet.md) |
| SCR-FLEET-05 | Change lifecycle, tenure, unit or cost centre | [modules/fleet.md](modules/fleet.md) |
| SCR-FLEET-06 | Reserve and block assets | [modules/fleet.md](modules/fleet.md) |
| SCR-FLEET-07 | Maintain suppliers | [modules/fleet.md](modules/fleet.md) |
| SCR-FUEL-04 | Find fuelings | [modules/fuelings.md](modules/fuelings.md) |
| SCR-FUEL-05 | Review one fueling | [modules/fuelings.md](modules/fuelings.md) |
| SCR-FUEL-06 | Decide a suspected duplicate | [modules/fuelings.md](modules/fuelings.md) |
| SCR-FUEL-07 | Amend or void an approved fueling | [modules/fuelings.md](modules/fuelings.md) |
| SCR-FUEL-08 | Read the consumption of an asset | [modules/fuelings.md](modules/fuelings.md) |
| SCR-GPS-05 | Follow operations in progress | [modules/gps-navigation.md](modules/gps-navigation.md) |
| SCR-GPS-06 | Replay a trip with its logbook | [modules/gps-navigation.md](modules/gps-navigation.md) |
| SCR-GPS-07 | Compare the four distances of a trip | [modules/gps-navigation.md](modules/gps-navigation.md) |
| SCR-GPS-08 | Maintain geofences | [modules/gps-navigation.md](modules/gps-navigation.md) |
| SCR-GPS-09 | Set tracking policy and alert rules | [modules/gps-navigation.md](modules/gps-navigation.md) |
| SCR-IDN-01 | Find employees and see who has access | [modules/identity.md](modules/identity.md) |
| SCR-IDN-02 | Create or edit an employee record | [modules/identity.md](modules/identity.md) |
| SCR-IDN-03 | Give access by invitation | [modules/identity.md](modules/identity.md) |
| SCR-IDN-04 | Follow invitations and codes | [modules/identity.md](modules/identity.md) |
| SCR-IDN-05 | Give or recover corporate access | [modules/identity.md](modules/identity.md) |
| SCR-IDN-06 | Manage one member's access | [modules/identity.md](modules/identity.md) |
| SCR-IDN-07 | Offboard with handover | [modules/identity.md](modules/identity.md) |
| SCR-IDN-08 | Resolve duplicates | [modules/identity.md](modules/identity.md) |
| SCR-IDN-09 | Enter a company with a code | [modules/identity.md](modules/identity.md) |
| SCR-IDN-10 | Decide on an invitation | [modules/identity.md](modules/identity.md) |
| SCR-IDN-13 | See my access, devices and data in this company | [modules/identity.md](modules/identity.md) |
| SCR-IMPORT-01 | Preview and confirm mapped rows | [modules/imports-notifications-integrations.md](modules/imports-notifications-integrations.md) |
| SCR-INTEG-01 | Manage an admitted connection | [modules/imports-notifications-integrations.md](modules/imports-notifications-integrations.md) |
| SCR-KPI-01 | See the state of the fleet for a period | [modules/intelligence.md](modules/intelligence.md) |
| SCR-KPI-02 | Break a figure down by a dimension | [modules/intelligence.md](modules/intelligence.md) |
| SCR-KPI-03 | Understand how a figure was computed | [modules/intelligence.md](modules/intelligence.md) |
| SCR-KPI-04 | See a price comparison and its reference | [modules/intelligence.md](modules/intelligence.md) |
| SCR-MAINT-01 | See what is due, overdue and scheduled | [modules/maintenance.md](modules/maintenance.md) |
| SCR-MAINT-02 | Create and edit a preventive plan | [modules/maintenance.md](modules/maintenance.md) |
| SCR-MAINT-03 | Follow work orders by state | [modules/maintenance.md](modules/maintenance.md) |
| SCR-MAINT-04 | Run one work order from request to closing | [modules/maintenance.md](modules/maintenance.md) |
| SCR-MAINT-05 | Track expiries of asset documents and, when enabled, licences | [modules/maintenance.md](modules/maintenance.md) |
| SCR-MAINT-08 | Insurance policies and claim records (evolution) | [modules/maintenance.md](modules/maintenance.md) |
| SCR-MAINT-09 | Tyres, components and parts stock (evolution) | [modules/maintenance.md](modules/maintenance.md) |
| SCR-METER-02 | See what a meter said over time | [modules/fleet.md](modules/fleet.md) |
| SCR-METER-03 | Correct or void one reading | [modules/fleet.md](modules/fleet.md) |
| SCR-METER-04 | Replace or reset a meter, confirm a rollover | [modules/fleet.md](modules/fleet.md) |
| SCR-METER-05 | Decide every suspect reading | [modules/fleet.md](modules/fleet.md) |
| SCR-NOTIF-01 | Read own notices | [modules/imports-notifications-integrations.md](modules/imports-notifications-integrations.md) |
| SCR-OFFL-03 | Resolve what the server kept for review | [10-offline-sync.md](10-offline-sync.md) |
| SCR-ORG-01 | Create the organization and finish setup | [modules/organizations.md](modules/organizations.md) |
| SCR-ORG-02 | See and change organization settings | [modules/organizations.md](modules/organizations.md) |
| SCR-ORG-03 | Maintain units | [modules/organizations.md](modules/organizations.md) |
| SCR-ORG-04 | Maintain cost centres | [modules/organizations.md](modules/organizations.md) |
| SCR-ORG-05 | Maintain teams and their members | [modules/organizations.md](modules/organizations.md) |
| SCR-ORG-06 | Declare legal entities and establishments and see their status | [modules/organizations.md](modules/organizations.md) |
| SCR-ORG-07 | Request verification of a legal entity | [modules/organizations.md](modules/organizations.md) |
| SCR-ORG-09 | Transfer ownership | [modules/organizations.md](modules/organizations.md) |
| SCR-ORG-10 | Manage consolidation grants | [modules/organizations.md](modules/organizations.md) |
| SCR-ORG-11 | Archive, restore or close the organization | [modules/organizations.md](modules/organizations.md) |
| SCR-ORG-12 | Tell members the organization is suspended, archived or closing | [modules/organizations.md](modules/organizations.md) |
| SCR-PRIV-02 | Set retention inside limits and see the schedule | [11-privacy-security.md](11-privacy-security.md) |
| SCR-PRIV-03 | Handle a person's data request | [11-privacy-security.md](11-privacy-security.md) |
| SCR-PRIV-04 | Manage legal holds | [11-privacy-security.md](11-privacy-security.md) |
| SCR-PRIV-06 | See who opened sensitive items | [11-privacy-security.md](11-privacy-security.md) |
| SCR-ROUTE-01 | Plan the route of a mission or trip | [modules/gps-navigation.md](modules/gps-navigation.md) |
| SCR-ROUTE-05 | Choose where to fuel on the route | [modules/gps-navigation.md](modules/gps-navigation.md) |
| SCR-RPT-01 | Export the current list or table | [modules/intelligence.md](modules/intelligence.md) |
| SCR-RPT-02 | Follow own exports | [modules/intelligence.md](modules/intelligence.md) |
| SCR-RPT-03 | Generate and schedule reports | [modules/intelligence.md](modules/intelligence.md) |
| SCR-RPT-04 | Export all organization data | [modules/intelligence.md](modules/intelligence.md) |
| SCR-RPT-05 | Consult the audit trail | [modules/intelligence.md](modules/intelligence.md) |
| SCR-STOCK-01 | Inspect tank movements | [modules/fuel-stock.md](modules/fuel-stock.md) |
| SCR-SUBS-01 | Inspect own organization plan | [modules/subscriptions.md](modules/subscriptions.md) |
| SCR-SUBS-02 | Resolve authorized overage | [modules/subscriptions.md](modules/subscriptions.md) |
| SCR-TRIP-02 | Inspect and review | [modules/trips.md](modules/trips.md) |
| SCR-TRIP-03 | Plan and hand over work | [modules/trips.md](modules/trips.md) |
<!-- SCREEN-INDEX:PANEL:END -->

### Station portal

<!-- SCREEN-INDEX:PORTAL:BEGIN -->
| Screen | Objective | Specified in |
|---|---|---|
| SCR-AGREE-01 | Review own contract side | [modules/station-commerce.md](modules/station-commerce.md) |
| SCR-AGREE-02 | Redeem an eligible voucher | [modules/station-commerce.md](modules/station-commerce.md) |
| SCR-CLAIM-01 | Find the station in the catalog and start a claim | [modules/station-claims.md](modules/station-claims.md) |
| SCR-CLAIM-02 | Declare the relation and submit the evidence | [modules/station-claims.md](modules/station-claims.md) |
| SCR-CLAIM-03 | Follow a claim and answer an information request | [modules/station-claims.md](modules/station-claims.md) |
| SCR-CLAIM-04 | See every station of the organization and its representation state | [modules/station-claims.md](modules/station-claims.md) |
| SCR-CLAIM-05 | Manage the station team | [modules/station-claims.md](modules/station-claims.md) |
| SCR-CLAIM-06 | Contest a decision or report impersonation | [modules/station-claims.md](modules/station-claims.md) |
| SCR-STN-01 | Manage authorized station commerce | [modules/station-commerce.md](modules/station-commerce.md) |
<!-- SCREEN-INDEX:PORTAL:END -->

### Internal administration

<!-- SCREEN-INDEX:ADMIN:BEGIN -->
| Screen | Objective | Specified in |
|---|---|---|
| SCR-BACK-01 | Review a permitted case | [modules/back-office.md](modules/back-office.md) |
| SCR-BACK-03 | Act inside a support session | [modules/back-office.md](modules/back-office.md) |
| SCR-CLAIM-07 | Work the review queue | [modules/station-claims.md](modules/station-claims.md) |
| SCR-CLAIM-08 | Decide a claim or a concurrent set | [modules/station-claims.md](modules/station-claims.md) |
| SCR-CLAIM-09 | Act on authorities and contest cases | [modules/station-claims.md](modules/station-claims.md) |
| SCR-ORG-08 | Decide a verification request | [modules/organizations.md](modules/organizations.md) |
<!-- SCREEN-INDEX:ADMIN:END -->

## 8. Acceptance and dependencies

Each owning backlog specifies V-WEB, V-AUTHZ and V-ISO evidence for its
screens. Exercise keyboard and large-text flows, loading, empty, error,
unavailable and forbidden states. Test session confusion, context switches
with a late response, revoked permission, stale revision and expired files.

Backend behavior and authority must be locally proved before dependent
screen acceptance. The [F1 entry conditions](14-roadmap.md) remain M0
closure and protected integration of the current ADR-019 batch. Existing
real-data, station and release gates still govern activation.

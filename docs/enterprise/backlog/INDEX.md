# Backlog index

Status: **PLANNED**, 2026-11-10. Generated from the task blocks in `backlog/ep*.md` by [tools/check_plan.py](../tools/check_plan.py) ([00-conventions](../00-conventions.md) §7). Do not edit by hand: change the task block and regenerate.

Columns: phase (`F0`–`F8`), cut (`C1` = required for the first operational cut), type and size as defined in [00-conventions](../00-conventions.md) §5. Sizes are relative hypotheses, not dates.

## Summary

- Tasks: **348** in 21 epics. Cut C1: **80** tasks.
- Relative effort (XS 0.5, S 1, M 2, L 4 agent cycles): **600.5** in total, **142.5** for cut C1. Hypothesis only.

| Phase | Tasks | Relative effort |
|---|---|---|
| F0 | 16 | 18.5 |
| F1 | 62 | 101 |
| F2 | 34 | 60 |
| F3 | 67 | 113 |
| F4 | 38 | 65 |
| F5 | 16 | 30 |
| F6 | 50 | 90 |
| F7 | 36 | 65 |
| F8 | 29 | 58 |

| Epic | Title | File | Tasks |
|---|---|---|---|
| EP00 | Governance, decisions and discovery | [ep00-governance.md](ep00-governance.md) | 16 |
| EP01 | Platform foundations and privacy | [ep01-platform.md](ep01-platform.md) | 21 |
| EP02 | Organizations | [ep02-organizations.md](ep02-organizations.md) | 15 |
| EP03 | Identity, employee accounts and authorization | [ep03-identity-access.md](ep03-identity-access.md) | 30 |
| EP04 | Offline synchronization | [ep04-sync.md](ep04-sync.md) | 14 |
| EP05 | Fleet, assets and meters | [ep05-fleet-meters.md](ep05-fleet-meters.md) | 20 |
| EP06 | Operations, trips and logbook | [ep06-trips.md](ep06-trips.md) | 23 |
| EP07 | Tracking, routes and navigation | [ep07-gps-navigation.md](ep07-gps-navigation.md) | 25 |
| EP08 | Fuelings and consumption | [ep08-fuelings.md](ep08-fuelings.md) | 15 |
| EP09 | Receipts, fiscal documents and attachments | [ep09-documents.md](ep09-documents.md) | 15 |
| EP10 | Expenses, approvals, budgets and operational finance | [ep10-expenses-finance.md](ep10-expenses-finance.md) | 27 |
| EP11 | Maintenance, checklists and complementary fleet controls | [ep11-maintenance.md](ep11-maintenance.md) | 16 |
| EP12 | Internal fuel stock | [ep12-fuel-stock.md](ep12-fuel-stock.md) | 7 |
| EP13 | Intelligence, findings and reports | [ep13-intelligence.md](ep13-intelligence.md) | 17 |
| EP14 | Station claims and representation | [ep14-station-claims.md](ep14-station-claims.md) | 13 |
| EP15 | Station commerce and agreements | [ep15-station-commerce.md](ep15-station-commerce.md) | 19 |
| EP16 | Subscriptions, entitlements and public products | [ep16-subscriptions.md](ep16-subscriptions.md) | 14 |
| EP17 | Abastevo Data | [ep17-data.md](ep17-data.md) | 8 |
| EP18 | Imports, notifications and integrations | [ep18-imports-notifications-integrations.md](ep18-imports-notifications-integrations.md) | 17 |
| EP19 | Internal administration and support | [ep19-back-office.md](ep19-back-office.md) | 6 |
| EP20 | Release gates and validation | [ep20-release.md](ep20-release.md) | 10 |

## EP00 — Governance, decisions and discovery

Detail: [ep00-governance.md](ep00-governance.md).

| ID | Title | Phase | Cut | Type | Size | Priority | Depends on |
|---|---|---|---|---|---|---|---|
| EMP-GOV-001 | Record the enterprise scope decision and propose the organization-tenancy ADR with the list of amended contracts | F0 | C1 | DEC | S | P0 | none |
| EMP-GOV-002 | Decide enterprise document authorization and retention, separate from the 24 h photo cap and from Stage D purchase evidence | F0 | C1 | DEC | S | P0 | EMP-GOV-001 |
| EMP-GOV-003 | Decide the business web surface: stack, path or origin and session model, following the admin-web precedent | F0 | C1 | DEC | S | P0 | EMP-GOV-001 |
| EMP-GOV-004 | Decide how internal administration extends the ADR-023 admin suite and staff roles | F0 | - | DEC | S | P1 | EMP-GOV-001 |
| EMP-GOV-005 | Legal validation package: privacy roles, location, documents and labour boundary | F0 | C1 | DISC | M | P0 | EMP-GOV-001 |
| EMP-GOV-006 | Spike: organization trip tracking on top of the existing location services, with upload, on target devices | F0 | - | DISC | M | P1 | EMP-GOV-001 |
| EMP-GOV-007 | Decide the panel web map and whether server-side routing is needed, with a cost model | F0 | - | DEC | M | P1 | EMP-GOV-006 |
| EMP-GOV-008 | Decide the Android enterprise partition and local data protection per account, origin and organization | F0 | C1 | DEC | S | P0 | EMP-GOV-001 |
| EMP-GOV-009 | Decide organization billing: extend the Stripe/Asaas offer policy and confirm the store-policy position | F0 | - | DEC | S | P2 | EMP-GOV-001 |
| EMP-GOV-010 | Discovery protocol with pilot companies and stations, including pricing hypotheses | F0 | - | DISC | S | P1 | none |
| EMP-GOV-011 | Decide the viability of authorised fiscal document consultation | F0 | - | DEC | S | P2 | EMP-GOV-005 |
| EMP-GOV-012 | Register the enterprise program in ROADMAP and PROGRESS sequencing | F0 | C1 | DEC | XS | P0 | EMP-GOV-001 |
| EMP-GOV-013 | Security review of managed credentials: activation code, device binding, recovery | F0 | - | DEC | S | P1 | EMP-GOV-001 |
| EMP-GOV-014 | Decide external delivery channels: transactional e-mail and push for enterprise notices | F0 | - | DEC | XS | P1 | EMP-GOV-001 |
| EMP-GOV-015 | Record the scoped Stage D override for station claims, management and portal (OPT-D00) | F0 | - | DEC | XS | P1 | EMP-GOV-001 |
| EMP-GOV-016 | Establish redistribution rights for public source data in commercial Data products | F0 | - | DISC | M | P2 | EMP-GOV-001 |

## EP01 — Platform foundations and privacy

Detail: [ep01-platform.md](ep01-platform.md).

| ID | Title | Phase | Cut | Type | Size | Priority | Depends on |
|---|---|---|---|---|---|---|---|
| EMP-PLAT-001 | Tenancy kernel: OrgID, OrgScope, the scoped-repository convention and the module registration checklist | F1 | C1 | DOM | M | P0 | EMP-GOV-001 |
| EMP-PLAT-002 | Tenant schema conventions, row-level-security template and two-organization isolation harness | F1 | C1 | DB | M | P0 | EMP-PLAT-001 |
| EMP-PLAT-003 | Organization audit trail: append-only, per organization, following the admin audit pattern | F1 | C1 | DB | M | P0 | EMP-PLAT-002 |
| EMP-PLAT-004 | Organization API conventions: path scope, session and device proof, error catalog, pagination, OpenAPI section, route-versus-contract test | F1 | C1 | API | M | P0 | EMP-PLAT-001 |
| EMP-PLAT-005 | Per-organization capability flags for activation and rollback | F1 | C1 | DB | S | P0 | EMP-PLAT-002 |
| EMP-PLAT-006 | Private storage namespace for enterprise documents with short-lived access | F1 | C1 | INT | M | P0 | EMP-GOV-002, EMP-PLAT-002 |
| EMP-PLAT-007 | Account web session for the business web: cookie, CSRF, origin check and fresh authentication | F1 | C1 | API | M | P0 | EMP-GOV-003, EMP-PLAT-004 |
| EMP-PLAT-008 | Business web scaffold: build, routing, auth guard, organization switcher, design tokens, checks | F1 | C1 | WEB | M | P0 | EMP-GOV-003, EMP-PLAT-007 |
| EMP-PLAT-009 | Exact-value kernel for organization records: reuse centavo, milli-unit and milli-BRL types, add reconciliation and allocation vectors | F1 | C1 | DOM | S | P0 | EMP-GOV-001 |
| EMP-PLAT-010 | Transactional enterprise events on the existing job queue | F1 | C1 | DB | S | P0 | EMP-PLAT-002 |
| EMP-PLAT-011 | Android enterprise foundation: packages, local store partitioned by account, origin and organization, DI | F1 | C1 | APP | M | P0 | EMP-GOV-008 |
| EMP-PLAT-012 | Transactional e-mail port and adapter for enterprise notices | F1 | - | INT | S | P1 | EMP-GOV-014, EMP-PLAT-004 |
| EMP-PLAT-013 | Enterprise telemetry: per-route metrics, redaction rules and organization-safe logs | F1 | C1 | INT | S | P0 | EMP-PLAT-004 |
| EMP-PLAT-014 | Organization rate limits per account, device and organization on the existing limiter | F1 | - | API | S | P1 | EMP-PLAT-004 |
| EMP-PLAT-015 | Time kernel: instants, organization and unit time zones, reporting-day attribution | F1 | C1 | DOM | S | P0 | EMP-GOV-001 |
| EMP-PLAT-016 | Amendment kernel: append-only corrections for approved records | F3 | - | DOM | S | P1 | EMP-PLAT-003 |
| EMP-PRIV-001 | Enterprise data inventory and retention schedule enforcement | F6 | - | JOB | M | P1 | EMP-PLAT-010, EMP-GOV-005 |
| EMP-PRIV-002 | Data-subject request routing between the organization and the platform | F6 | - | API | M | P1 | EMP-IDN-003, EMP-GOV-005 |
| EMP-PRIV-003 | Device-side protection: encrypted store, logout cleanup and shared-device rules | F1 | C1 | APP | M | P0 | EMP-PLAT-011 |
| EMP-PRIV-004 | Backup, restore and deletion-ledger replay for enterprise data | F6 | - | OPS | M | P1 | EMP-PLAT-002 |
| EMP-PRIV-005 | Access log for sensitive reads: documents, location and exports | F3 | C1 | API | S | P0 | EMP-PLAT-003 |

## EP02 — Organizations

Detail: [ep02-organizations.md](ep02-organizations.md).

| ID | Title | Phase | Cut | Type | Size | Priority | Depends on |
|---|---|---|---|---|---|---|---|
| EMP-ORG-001 | Organization aggregate and lifecycle rules | F1 | C1 | DOM | S | P0 | EMP-PLAT-001 |
| EMP-ORG-002 | Organization persistence as the tenant root | F1 | C1 | DB | S | P0 | EMP-ORG-001, EMP-PLAT-002 |
| EMP-ORG-003 | Create-organization flow: the creator becomes owner and a default unit exists | F1 | C1 | API | M | P0 | EMP-ORG-002, EMP-IDN-003, EMP-AUTH-002, EMP-PLAT-004, EMP-PLAT-003 |
| EMP-ORG-004 | Legal entities with CNPJ and CPF validation, declared versus verified | F1 | - | API | M | P1 | EMP-ORG-003 |
| EMP-ORG-005 | Operating units | F1 | C1 | API | S | P0 | EMP-ORG-003, EMP-PLAT-015 |
| EMP-ORG-006 | Cost centres | F1 | - | API | S | P1 | EMP-ORG-003 |
| EMP-ORG-007 | Teams | F1 | - | API | S | P1 | EMP-ORG-003, EMP-IDN-002 |
| EMP-ORG-008 | Organization settings and policy holder: time zone, units, approval defaults | F1 | C1 | API | S | P0 | EMP-ORG-003, EMP-PLAT-015 |
| EMP-ORG-009 | Suspension and reactivation with write-block semantics | F1 | - | API | M | P1 | EMP-ORG-003 |
| EMP-ORG-010 | Ownership transfer and the last-owner invariant | F1 | - | API | S | P1 | EMP-ORG-003, EMP-AUTH-005 |
| EMP-ORG-011 | Legal entity verification workflow | F7 | - | API | M | P1 | EMP-ORG-004, EMP-BACK-002 |
| EMP-ORG-012 | Organization groups and consolidation grants | F6 | - | API | M | P2 | EMP-ORG-003, EMP-AUTH-006 |
| EMP-ORG-013 | Archive and closure: open operations, export and retention schedule | F8 | - | API | M | P2 | EMP-ORG-009, EMP-RPT-005, EMP-PRIV-001 |
| EMP-ORG-014 | Panel: onboarding wizard and organization settings | F1 | C1 | WEB | M | P0 | EMP-ORG-003, EMP-ORG-005, EMP-ORG-008, EMP-PLAT-008 |
| EMP-ORG-015 | Panel: units, cost centres and teams management | F1 | - | WEB | M | P1 | EMP-ORG-005, EMP-ORG-006, EMP-ORG-007, EMP-PLAT-008 |

## EP03 — Identity, employee accounts and authorization

Detail: [ep03-identity-access.md](ep03-identity-access.md).

| ID | Title | Phase | Cut | Type | Size | Priority | Depends on |
|---|---|---|---|---|---|---|---|
| EMP-IDN-001 | Identity domain: person, employee, driver profile, membership, credential, device | F1 | C1 | DOM | M | P0 | EMP-PLAT-001 |
| EMP-IDN-002 | Employee records that exist without a login | F1 | C1 | API | M | P0 | EMP-IDN-001, EMP-ORG-002, EMP-AUTH-003 |
| EMP-IDN-003 | Membership persistence and states | F1 | C1 | DB | S | P0 | EMP-IDN-001, EMP-ORG-002 |
| EMP-IDN-004 | Invitations: issue, expire, resend, revoke | F1 | C1 | API | M | P0 | EMP-IDN-002, EMP-IDN-003 |
| EMP-IDN-005 | Invitation acceptance by an existing Abastevo account | F1 | C1 | API | M | P0 | EMP-IDN-004 |
| EMP-IDN-006 | Managed corporate credential: activation code issuance | F1 | - | API | M | P1 | EMP-IDN-002, EMP-IDN-003, EMP-GOV-013 |
| EMP-IDN-007 | Managed credential activation on the device: key enrolment and local lock | F1 | - | API | M | P1 | EMP-IDN-006 |
| EMP-IDN-008 | Managed credential recovery, reset and device revocation | F1 | - | API | S | P1 | EMP-IDN-007 |
| EMP-IDN-009 | Membership suspension and reactivation | F1 | - | API | S | P1 | EMP-IDN-003, EMP-AUTH-003 |
| EMP-IDN-010 | Offboarding: end membership, revoke sessions, hand over responsibilities | F1 | - | API | M | P1 | EMP-IDN-009 |
| EMP-IDN-011 | Rehire: a new membership period on the same employee record | F1 | - | API | S | P2 | EMP-IDN-010 |
| EMP-IDN-012 | Multiple memberships and duplicate employee detection and merge | F1 | - | API | M | P2 | EMP-IDN-005 |
| EMP-IDN-013 | Contact change and invitation invalidation | F1 | - | API | S | P2 | EMP-IDN-004 |
| EMP-IDN-014 | Driver profile: licence category, expiry and eligibility | F2 | - | API | S | P1 | EMP-IDN-002 |
| EMP-IDN-015 | Device registry, session list and revocation | F1 | - | API | S | P1 | EMP-IDN-005 |
| EMP-IDN-016 | Panel: employees, invitations and activation codes | F1 | C1 | WEB | M | P0 | EMP-IDN-002, EMP-IDN-004, EMP-PLAT-008 |
| EMP-IDN-017 | App: invitation acceptance and managed activation | F1 | C1 | APP | M | P0 | EMP-IDN-005, EMP-PLAT-011 |
| EMP-IDN-018 | App: personal and enterprise context switch with the enterprise home shell | F1 | C1 | APP | M | P0 | EMP-AUTH-003, EMP-PLAT-011, EMP-IDN-017 |
| EMP-IDN-019 | Personal account erasure versus organization-owned history | F6 | - | API | M | P1 | EMP-IDN-010, EMP-PRIV-002 |
| EMP-AUTH-001 | Role and permission registry: system roles and permission keys | F1 | C1 | DOM | S | P0 | EMP-PLAT-001 |
| EMP-AUTH-002 | Role assignments with organization and self scopes | F1 | C1 | DB | S | P0 | EMP-AUTH-001, EMP-IDN-003 |
| EMP-AUTH-003 | Authorization service and organization-context middleware | F1 | C1 | API | M | P0 | EMP-AUTH-002, EMP-PLAT-004 |
| EMP-AUTH-004 | Entitlement gate port with a pilot adapter | F1 | C1 | DOM | S | P0 | EMP-AUTH-003, EMP-PLAT-005 |
| EMP-AUTH-005 | Role management with last-owner and separation-of-duties guards | F1 | - | API | M | P1 | EMP-AUTH-003, EMP-PLAT-003 |
| EMP-AUTH-006 | Unit, cost-centre, team and asset scopes with resource scope resolution | F2 | - | DB | M | P1 | EMP-AUTH-003, EMP-ORG-005, EMP-ORG-006, EMP-ORG-007, EMP-FLEET-002 |
| EMP-AUTH-007 | Temporary delegation and substitutes | F3 | - | API | S | P1 | EMP-AUTH-005 |
| EMP-AUTH-008 | Panel: roles and access management | F1 | - | WEB | S | P1 | EMP-AUTH-005, EMP-PLAT-008 |
| EMP-AUTH-009 | Authorization matrix conformance suite | F1 | C1 | TEST | M | P0 | EMP-AUTH-003 |
| EMP-AUTH-010 | Custom roles as named permission bundles | F6 | - | API | M | P3 | EMP-AUTH-005, EMP-AUTH-009 |
| EMP-AUTH-011 | Auditor read-only role and export permission gating | F3 | - | API | S | P1 | EMP-AUTH-003 |

## EP04 — Offline synchronization

Detail: [ep04-sync.md](ep04-sync.md).

| ID | Title | Phase | Cut | Type | Size | Priority | Depends on |
|---|---|---|---|---|---|---|---|
| EMP-OFFL-001 | Sync protocol contract: operation envelope, results, error codes, golden vectors | F1 | C1 | API | M | P0 | EMP-PLAT-004 |
| EMP-OFFL-002 | Server operation intake: receipt ledger, idempotency and dependency ordering | F1 | C1 | DB | L | P0 | EMP-OFFL-001, EMP-AUTH-003, EMP-PLAT-010 |
| EMP-OFFL-003 | Android persistent operation queue with immutable organization context | F1 | C1 | APP | L | P0 | EMP-OFFL-001, EMP-PLAT-011 |
| EMP-OFFL-004 | App: sync status model and the pending-sync screen | F1 | C1 | APP | M | P0 | EMP-OFFL-003 |
| EMP-OFFL-005 | Attachment upload: offline queue, retry and resume, linkage to operations | F3 | C1 | APP | M | P0 | EMP-OFFL-002, EMP-OFFL-003, EMP-PLAT-006 |
| EMP-OFFL-006 | Reference data pull for the active organization: versioned deltas | F2 | C1 | API | M | P0 | EMP-OFFL-002, EMP-FLEET-006 |
| EMP-OFFL-007 | Device clock capture, skew estimation and time fields | F1 | C1 | DOM | S | P0 | EMP-OFFL-002, EMP-PLAT-015 |
| EMP-OFFL-008 | Review queue for operations that need review, with resolution actions | F2 | C1 | API | M | P0 | EMP-OFFL-002 |
| EMP-OFFL-009 | Post-revocation intake window and quarantine | F4 | - | API | M | P1 | EMP-OFFL-008, EMP-IDN-010 |
| EMP-OFFL-010 | Context switch, logout and shared-device behaviour with a pending queue | F1 | C1 | APP | M | P0 | EMP-OFFL-003, EMP-IDN-018 |
| EMP-OFFL-011 | Cleanup of revoked sessions and devices on next contact | F4 | - | APP | S | P1 | EMP-OFFL-003, EMP-IDN-015 |
| EMP-OFFL-012 | Organization-state and plan-change handling for queued operations | F4 | - | API | S | P1 | EMP-OFFL-008, EMP-ORG-009 |
| EMP-OFFL-013 | Sync resilience suite: duplicates, reordering, partial uploads, process death | F4 | - | TEST | M | P1 | EMP-OFFL-002, EMP-OFFL-003, EMP-OFFL-005 |
| EMP-OFFL-014 | Panel: sync review queue | F2 | C1 | WEB | S | P0 | EMP-OFFL-008, EMP-PLAT-008 |

## EP05 — Fleet, assets and meters

Detail: [ep05-fleet-meters.md](ep05-fleet-meters.md).

| ID | Title | Phase | Cut | Type | Size | Priority | Depends on |
|---|---|---|---|---|---|---|---|
| EMP-FLEET-001 | Asset domain: vehicles, machines, implements, tanks and compatible products | F2 | C1 | DOM | M | P0 | EMP-PLAT-001, EMP-PLAT-009 |
| EMP-FLEET-002 | Asset persistence and management with per-organization identifier uniqueness | F2 | C1 | API | M | P0 | EMP-FLEET-001, EMP-PLAT-002, EMP-AUTH-003, EMP-FUEL-001 |
| EMP-FLEET-003 | Asset lifecycle: acquisition, active, sale, write-off, return, archive | F2 | - | API | S | P1 | EMP-FLEET-002 |
| EMP-FLEET-004 | Tenure kinds: owned, leased, rented, third-party, loaned | F2 | - | API | S | P1 | EMP-FLEET-002 |
| EMP-FLEET-005 | Unit and cost-centre allocation history | F2 | - | API | S | P1 | EMP-FLEET-002, EMP-ORG-005, EMP-ORG-006 |
| EMP-FLEET-006 | Driver assignments: primary, temporary and multiple drivers | F2 | C1 | API | M | P0 | EMP-FLEET-002, EMP-IDN-002 |
| EMP-FLEET-007 | Reservations, blocks and derived availability | F4 | - | API | M | P1 | EMP-FLEET-006 |
| EMP-FLEET-008 | Transfer between units with an effective date | F2 | - | API | S | P2 | EMP-FLEET-005 |
| EMP-FLEET-009 | Supplier registry | F3 | - | API | S | P1 | EMP-PLAT-002, EMP-AUTH-003 |
| EMP-FLEET-010 | Asset documents and expiry dates | F6 | - | API | S | P1 | EMP-FLEET-002, EMP-DOC-003 |
| EMP-FLEET-011 | Implements: attach to and detach from a powered asset | F4 | - | API | S | P2 | EMP-FLEET-002 |
| EMP-FLEET-012 | Panel: fleet list, detail and assignments | F2 | C1 | WEB | M | P0 | EMP-FLEET-002, EMP-FLEET-006, EMP-PLAT-008 |
| EMP-FLEET-013 | App: my assets and asset selection | F2 | C1 | APP | S | P0 | EMP-FLEET-006, EMP-OFFL-006, EMP-IDN-018 |
| EMP-METER-001 | Meter and reading domain: append-only evidence, sources and corrections | F2 | C1 | DOM | M | P0 | EMP-FLEET-001 |
| EMP-METER-002 | Meter persistence and reading intake | F2 | C1 | DB | M | P0 | EMP-METER-001, EMP-FLEET-002, EMP-OFFL-002 |
| EMP-METER-003 | Reading validation: monotonicity, retroactive inserts and suspect flags | F2 | C1 | DOM | S | P0 | EMP-METER-002 |
| EMP-METER-004 | Reading correction and void with audit | F2 | - | API | S | P1 | EMP-METER-003, EMP-PLAT-003 |
| EMP-METER-005 | Meter replacement, reset and rollover with cumulative usage | F2 | - | DOM | M | P1 | EMP-METER-003 |
| EMP-METER-006 | Current-value projection and usage-between-instants query | F3 | - | DB | S | P1 | EMP-METER-005 |
| EMP-METER-007 | Panel: meter history and corrections | F2 | - | WEB | S | P1 | EMP-METER-004, EMP-PLAT-008 |

## EP06 — Operations, trips and logbook

Detail: [ep06-trips.md](ep06-trips.md).

| ID | Title | Phase | Cut | Type | Size | Priority | Depends on |
|---|---|---|---|---|---|---|---|
| EMP-TRIP-001 | Trip domain core: operational states and the driver-initiated trip | F2 | C1 | DOM | M | P0 | EMP-FLEET-001, EMP-METER-001, EMP-PLAT-015 |
| EMP-TRIP-002 | Trip persistence and sync handlers with the one-active-trip-per-asset rule | F2 | C1 | DB | L | P0 | EMP-TRIP-001, EMP-OFFL-002, EMP-METER-002, EMP-FLEET-006 |
| EMP-TRIP-003 | Logbook events: the typed timeline | F2 | C1 | DB | M | P0 | EMP-TRIP-002 |
| EMP-TRIP-004 | Trip submission and administrative review | F2 | C1 | API | M | P0 | EMP-TRIP-002, EMP-APPR-001 |
| EMP-TRIP-005 | App: start a trip, log events and finish, offline first | F2 | C1 | APP | L | P0 | EMP-TRIP-002, EMP-TRIP-003, EMP-OFFL-003, EMP-FLEET-013 |
| EMP-TRIP-006 | Panel: trips list, timeline detail and review | F2 | C1 | WEB | M | P0 | EMP-TRIP-004, EMP-PLAT-008 |
| EMP-TRIP-007 | Asset-overlap conflict resolution | F4 | - | API | M | P1 | EMP-TRIP-002, EMP-OFFL-008 |
| EMP-TRIP-008 | Missions: create, assign, accept and decline | F4 | - | API | M | P1 | EMP-TRIP-002 |
| EMP-TRIP-009 | Manager-planned trips: destination, reason, customer or project, cargo or passengers | F4 | - | API | M | P1 | EMP-TRIP-008 |
| EMP-TRIP-010 | Stops and legs: planned versus actual | F4 | - | DB | M | P1 | EMP-TRIP-003 |
| EMP-TRIP-011 | Pause, resume and cancellation | F4 | - | DOM | S | P1 | EMP-TRIP-002 |
| EMP-TRIP-012 | Driver handover during a trip | F4 | - | API | M | P1 | EMP-TRIP-002 |
| EMP-TRIP-013 | Vehicle change within a mission | F4 | - | DOM | S | P2 | EMP-TRIP-008 |
| EMP-TRIP-014 | Missing final reading and supervisor closure | F4 | - | API | M | P1 | EMP-TRIP-004, EMP-METER-004 |
| EMP-TRIP-015 | Late and duplicate events, and amendments after closure | F4 | - | API | M | P1 | EMP-TRIP-004, EMP-PLAT-016 |
| EMP-TRIP-016 | Period lock and authorised reopening for trips | F4 | - | API | S | P1 | EMP-TRIP-004, EMP-APPR-006 |
| EMP-TRIP-017 | Midnight and time-zone crossing rules | F4 | - | DOM | S | P1 | EMP-TRIP-001 |
| EMP-TRIP-018 | Open trips when a membership ends | F4 | - | API | S | P1 | EMP-TRIP-014, EMP-IDN-010 |
| EMP-TRIP-019 | Checklist gate at trip start and finish | F4 | - | API | S | P1 | EMP-TRIP-002, EMP-CHECK-002 |
| EMP-TRIP-020 | Trip closing conference: expenses and documents before submission | F4 | - | APP | M | P1 | EMP-TRIP-005, EMP-FUEL-011, EMP-EXP-004 |
| EMP-TRIP-021 | App: missions inbox and planned-trip execution | F4 | - | APP | M | P1 | EMP-TRIP-008, EMP-TRIP-009, EMP-TRIP-011, EMP-TRIP-005 |
| EMP-TRIP-022 | Panel: mission planning and assignment board | F4 | - | WEB | M | P1 | EMP-TRIP-009, EMP-FLEET-007, EMP-PLAT-008 |
| EMP-TRIP-023 | Reservation-to-trip integration and no-show release | F4 | - | DOM | S | P2 | EMP-FLEET-007, EMP-TRIP-008 |

## EP07 — Tracking, routes and navigation

Detail: [ep07-gps-navigation.md](ep07-gps-navigation.md).

| ID | Title | Phase | Cut | Type | Size | Priority | Depends on |
|---|---|---|---|---|---|---|---|
| EMP-GPS-001 | Tracking session domain: trip-bound lifecycle, permission and quality states | F4 | - | DOM | M | P1 | EMP-TRIP-001, EMP-GOV-005, EMP-GOV-006 |
| EMP-GPS-002 | Organization trip tracking service: explicit start from the trip screen, visible notification, location-guard update | F4 | - | APP | L | P1 | EMP-GPS-001, EMP-TRIP-005 |
| EMP-GPS-003 | Adaptive sampling, local buffer and batch envelope | F4 | - | APP | M | P1 | EMP-GPS-002 |
| EMP-GPS-004 | Track intake: batch endpoint, de-duplication and segment storage | F4 | - | DB | L | P1 | EMP-GPS-001, EMP-OFFL-002 |
| EMP-GPS-005 | Track processing: filtering, gaps, GPS distance and simplified geometry | F4 | - | JOB | M | P1 | EMP-GPS-004 |
| EMP-GPS-006 | Distance sources on a trip and the divergence signal | F4 | - | DOM | S | P1 | EMP-GPS-005, EMP-METER-006 |
| EMP-GPS-007 | Live position projection with staleness rules | F5 | - | API | M | P1 | EMP-GPS-004 |
| EMP-GPS-008 | Panel: live operations map | F5 | - | WEB | M | P1 | EMP-GPS-007, EMP-ROUTE-001, EMP-GPS-012 |
| EMP-GPS-009 | Panel: trip track replay aligned with logbook events | F5 | - | WEB | M | P1 | EMP-GPS-005, EMP-TRIP-006, EMP-ROUTE-001 |
| EMP-GPS-010 | Tracking interruptions: permission, approximate-only, reboot, process death | F4 | - | APP | M | P1 | EMP-GPS-002 |
| EMP-GPS-011 | Track retention: raw-point purge and summary retention | F5 | - | JOB | S | P1 | EMP-GPS-005 |
| EMP-GPS-012 | Location access permission and access audit | F4 | - | API | S | P1 | EMP-GPS-004, EMP-PRIV-005 |
| EMP-GPS-013 | Geofences with server-side evaluation and alerts | F5 | - | JOB | M | P2 | EMP-GPS-004, EMP-NOTIF-001 |
| EMP-GPS-014 | Operational alerts: stale tracker and long stop | F5 | - | JOB | S | P2 | EMP-GPS-007, EMP-NOTIF-001 |
| EMP-GPS-015 | App: tracking transparency (indicator, what is collected, session history) | F4 | - | APP | S | P1 | EMP-GPS-002 |
| EMP-ROUTE-001 | Map components: reuse the Android map engine port and add the panel web map | F5 | - | INT | M | P1 | EMP-GOV-007, EMP-PLAT-008, EMP-PLAT-011 |
| EMP-ROUTE-002 | Routing port: on-device route computation through the map engine, with a server-side adapter slot | F5 | - | INT | M | P1 | EMP-GOV-007 |
| EMP-ROUTE-003 | Route plan on missions and trips | F5 | - | API | M | P1 | EMP-ROUTE-002, EMP-TRIP-009, EMP-TRIP-010 |
| EMP-ROUTE-004 | Navigation phase 1: open the existing in-app route and guidance for a trip destination | F4 | - | APP | S | P1 | EMP-TRIP-005 |
| EMP-ROUTE-005 | In-app route preview | F5 | - | APP | M | P1 | EMP-ROUTE-001, EMP-ROUTE-003 |
| EMP-ROUTE-006 | Trip-aware turn-by-turn navigation: guidance and tracking as coordinated location consumers | F5 | - | APP | L | P1 | EMP-ROUTE-005, EMP-GPS-002 |
| EMP-ROUTE-007 | Driving-mode experience: quick actions and stop-only detailed entry | F5 | - | APP | M | P1 | EMP-ROUTE-006 |
| EMP-ROUTE-008 | Station suggestions along the route | F5 | - | API | M | P2 | EMP-ROUTE-002, EMP-FUEL-014 |
| EMP-ROUTE-009 | Navigation degraded and offline behaviour | F5 | - | APP | S | P2 | EMP-ROUTE-006 |
| EMP-ROUTE-010 | Engine and adapter replacement drill against the routing and map ports | F5 | - | TEST | S | P2 | EMP-ROUTE-002, EMP-ROUTE-006 |

## EP08 — Fuelings and consumption

Detail: [ep08-fuelings.md](ep08-fuelings.md).

| ID | Title | Phase | Cut | Type | Size | Priority | Depends on |
|---|---|---|---|---|---|---|---|
| EMP-FUEL-001 | Fueling domain: products, units, payer and supplier roles | F3 | C1 | DOM | M | P0 | EMP-PLAT-009, EMP-FLEET-001, EMP-METER-001 |
| EMP-FUEL-002 | Fueling persistence, sync handlers and arithmetic consistency | F3 | C1 | DB | L | P0 | EMP-FUEL-001, EMP-OFFL-002, EMP-METER-002 |
| EMP-FUEL-003 | Fueling review: check, approve, reject, return, cancel | F3 | C1 | API | M | P0 | EMP-FUEL-002, EMP-APPR-001 |
| EMP-FUEL-004 | Supplier linkage: canonical station, organization supplier or not found | F3 | - | API | M | P1 | EMP-FUEL-002, EMP-FLEET-009 |
| EMP-FUEL-005 | Capacity tolerance and product compatibility validation | F3 | - | DOM | S | P1 | EMP-FUEL-002 |
| EMP-FUEL-006 | Suspected duplicate versus technical retry | F3 | - | DOM | S | P1 | EMP-FUEL-002 |
| EMP-FUEL-007 | Consumption engine: full-to-full intervals with partial fuelings | F3 | - | DOM | L | P1 | EMP-FUEL-002, EMP-METER-006 |
| EMP-FUEL-008 | Consumption projections and recalculation after corrections | F3 | - | JOB | M | P1 | EMP-FUEL-007 |
| EMP-FUEL-009 | Multi-tank, flex, CNG and energy category rules | F6 | - | DOM | M | P2 | EMP-FUEL-007 |
| EMP-FUEL-010 | Per-trip fuel allocation and cost per distance or hour | F6 | - | DOM | M | P1 | EMP-FUEL-008, EMP-TRIP-004 |
| EMP-FUEL-011 | App: add a fueling offline with receipt and meter reading | F3 | C1 | APP | M | P0 | EMP-FUEL-002, EMP-OFFL-005, EMP-FLEET-013, EMP-DOC-004 |
| EMP-FUEL-012 | Panel: fuelings list, detail and review | F3 | C1 | WEB | M | P0 | EMP-FUEL-003, EMP-PLAT-008 |
| EMP-FUEL-013 | Retroactive fuelings and their effect on intervals | F3 | - | DOM | S | P1 | EMP-FUEL-007 |
| EMP-FUEL-014 | Price reference snapshot at fueling time | F3 | - | API | M | P2 | EMP-FUEL-004 |
| EMP-FUEL-015 | Fueling amendment after approval and period lock | F3 | - | API | S | P1 | EMP-FUEL-003, EMP-PLAT-016, EMP-APPR-006 |

## EP09 — Receipts, fiscal documents and attachments

Detail: [ep09-documents.md](ep09-documents.md).

| ID | Title | Phase | Cut | Type | Size | Priority | Depends on |
|---|---|---|---|---|---|---|---|
| EMP-DOC-001 | Document domain: container, independent statuses, links and allocations | F3 | C1 | DOM | M | P0 | EMP-PLAT-001 |
| EMP-DOC-002 | Document persistence, upload session and audited private access | F3 | C1 | API | M | P0 | EMP-DOC-001, EMP-PLAT-006, EMP-AUTH-003, EMP-PRIV-005 |
| EMP-DOC-003 | Links between documents and operations | F3 | C1 | API | S | P0 | EMP-DOC-002 |
| EMP-DOC-004 | App: capture and import with offline queue and legibility check | F3 | C1 | APP | M | P0 | EMP-DOC-002, EMP-OFFL-005 |
| EMP-DOC-005 | Conference workflow | F3 | - | API | S | P1 | EMP-DOC-003 |
| EMP-DOC-006 | Fiscal structure: access key, NFC-e QR code and XML structural validation | F3 | - | DOM | M | P1 | EMP-DOC-002 |
| EMP-DOC-007 | Fiscal items and allocations without double counting | F3 | - | DB | M | P1 | EMP-DOC-006, EMP-EXP-002 |
| EMP-DOC-008 | Duplicate detection by access key and content hash | F3 | - | DB | S | P1 | EMP-DOC-002 |
| EMP-DOC-009 | Optional on-device extraction with per-field confidence | F3 | - | APP | M | P2 | EMP-DOC-004 |
| EMP-DOC-010 | Reconciliation status between documents and operations | F3 | - | JOB | M | P1 | EMP-DOC-007 |
| EMP-DOC-011 | Replacement and cancellation of documents | F3 | - | API | S | P1 | EMP-DOC-005 |
| EMP-DOC-012 | Retention, deletion and legal hold by category | F6 | - | JOB | M | P1 | EMP-DOC-002, EMP-GOV-002 |
| EMP-DOC-013 | Panel: document inbox, viewer, conference and allocation | F3 | - | WEB | M | P1 | EMP-DOC-005, EMP-DOC-007, EMP-PLAT-008 |
| EMP-DOC-014 | Upload safety checks: type, size and active content | F3 | C1 | JOB | S | P0 | EMP-DOC-002 |
| EMP-DOC-015 | Panel: receipt viewer inside fueling review | F3 | C1 | WEB | S | P0 | EMP-DOC-003, EMP-FUEL-012 |

## EP10 — Expenses, approvals, budgets and operational finance

Detail: [ep10-expenses-finance.md](ep10-expenses-finance.md).

| ID | Title | Phase | Cut | Type | Size | Priority | Depends on |
|---|---|---|---|---|---|---|---|
| EMP-APPR-001 | Review lifecycle component: submit, approve, return, reject, no self-approval | F3 | C1 | DOM | M | P0 | EMP-AUTH-003, EMP-PLAT-003 |
| EMP-APPR-002 | Approval policies: thresholds, levels and routing by scope | F3 | - | API | M | P1 | EMP-APPR-001, EMP-AUTH-006 |
| EMP-APPR-003 | Substitutes and delegation in approval routing | F3 | - | DOM | S | P1 | EMP-APPR-002, EMP-AUTH-007 |
| EMP-APPR-004 | Partial approval and the return-for-correction loop | F3 | - | DOM | S | P1 | EMP-APPR-002 |
| EMP-APPR-005 | Panel: approvals inbox | F3 | C1 | WEB | M | P0 | EMP-APPR-001, EMP-PLAT-008 |
| EMP-APPR-006 | Period closing, lock and reopening | F3 | - | API | M | P1 | EMP-APPR-001 |
| EMP-APPR-007 | App: my submissions and returned items | F3 | - | APP | S | P1 | EMP-APPR-001, EMP-OFFL-004 |
| EMP-EXP-001 | Expense domain: categories, origin, competence date and company amount | F3 | - | DOM | M | P1 | EMP-PLAT-009, EMP-PLAT-015 |
| EMP-EXP-002 | Expense persistence, sync handlers and the fueling-to-expense projection | F3 | - | DB | M | P1 | EMP-EXP-001, EMP-FUEL-002, EMP-OFFL-002 |
| EMP-EXP-003 | Expense categories configuration | F3 | - | API | S | P1 | EMP-EXP-002 |
| EMP-EXP-004 | App: add an expense with receipt | F3 | - | APP | M | P1 | EMP-EXP-002, EMP-DOC-004 |
| EMP-EXP-005 | Panel: expenses list, detail and review | F3 | - | WEB | M | P1 | EMP-EXP-002, EMP-APPR-001, EMP-PLAT-008 |
| EMP-EXP-006 | Cost allocation across cost centres and assets | F3 | - | DB | M | P1 | EMP-EXP-002, EMP-ORG-006 |
| EMP-EXP-007 | Fines and recurring costs such as lease and insurance | F6 | - | API | S | P2 | EMP-EXP-002, EMP-FLEET-004 |
| EMP-FIN-001 | Payables domain: supplier, reimbursement and card statement obligations | F3 | - | DOM | S | P1 | EMP-EXP-001 |
| EMP-FIN-002 | Reimbursements for out-of-pocket expenses | F3 | - | API | M | P1 | EMP-FIN-001, EMP-EXP-002, EMP-APPR-001 |
| EMP-FIN-003 | Advances and settlement against expenses | F3 | - | API | M | P2 | EMP-FIN-002 |
| EMP-FIN-004 | Payment records and the single-cost invariant | F3 | - | DB | M | P1 | EMP-FIN-001, EMP-EXP-002 |
| EMP-FIN-005 | Statements: card and supplier invoices grouping payables | F6 | - | API | M | P2 | EMP-FIN-004 |
| EMP-FIN-006 | Accounting export: competence and cash views | F6 | - | JOB | M | P1 | EMP-FIN-004, EMP-RPT-001 |
| EMP-FIN-007 | Panel: payables, reimbursements and payments | F3 | - | WEB | M | P1 | EMP-FIN-002, EMP-FIN-004, EMP-PLAT-008 |
| EMP-BUDG-001 | Budget domain: lines, periods, dimensions and amounts | F3 | - | DOM | S | P1 | EMP-EXP-001 |
| EMP-BUDG-002 | Budget persistence and concurrent consumption posting | F3 | - | DB | M | P1 | EMP-BUDG-001, EMP-EXP-002 |
| EMP-BUDG-003 | Reservations: pre-authorisation, release and expiry | F3 | - | API | M | P1 | EMP-BUDG-002 |
| EMP-BUDG-004 | Overrun policies and exception approval | F3 | - | API | S | P1 | EMP-BUDG-002, EMP-APPR-002 |
| EMP-BUDG-005 | Panel: budgets setup and monitoring | F3 | - | WEB | M | P1 | EMP-BUDG-002, EMP-PLAT-008 |
| EMP-BUDG-006 | Driver and asset limits at submission and after sync | F3 | - | DOM | S | P2 | EMP-BUDG-002 |

## EP11 — Maintenance, checklists and complementary fleet controls

Detail: [ep11-maintenance.md](ep11-maintenance.md).

| ID | Title | Phase | Cut | Type | Size | Priority | Depends on |
|---|---|---|---|---|---|---|---|
| EMP-CHECK-001 | Checklist templates per asset category | F4 | - | API | S | P1 | EMP-FLEET-002 |
| EMP-CHECK-002 | Checklist execution offline with the critical-item policy | F4 | - | APP | M | P1 | EMP-CHECK-001, EMP-OFFL-002, EMP-OFFL-003 |
| EMP-CHECK-003 | Damage and incident reports with photos | F4 | - | API | M | P1 | EMP-DOC-003, EMP-TRIP-003 |
| EMP-CHECK-004 | Panel: checklists, damages and incidents | F4 | - | WEB | M | P1 | EMP-CHECK-002, EMP-CHECK-003, EMP-PLAT-008 |
| EMP-MAINT-001 | Maintenance domain: plans, triggers and work orders | F6 | - | DOM | M | P1 | EMP-FLEET-001, EMP-METER-001 |
| EMP-MAINT-002 | Preventive plans and due computation from meters and dates | F6 | - | JOB | M | P1 | EMP-MAINT-001, EMP-METER-006 |
| EMP-MAINT-003 | Corrective requests from checklists, incidents and drivers | F6 | - | API | S | P1 | EMP-MAINT-001, EMP-CHECK-003 |
| EMP-MAINT-004 | Work-order lifecycle with quote, approval, parts and services | F6 | - | API | L | P1 | EMP-MAINT-001, EMP-FLEET-009, EMP-APPR-002 |
| EMP-MAINT-005 | Unavailability during maintenance and return to operation | F6 | - | DOM | S | P1 | EMP-MAINT-004, EMP-FLEET-007 |
| EMP-MAINT-006 | Maintenance cost posted once as an expense, with attachments | F6 | - | DOM | S | P1 | EMP-MAINT-004, EMP-EXP-002, EMP-DOC-003 |
| EMP-MAINT-007 | Expiry tracking and alerts for asset and driver documents | F6 | - | JOB | M | P1 | EMP-FLEET-010, EMP-IDN-014, EMP-NOTIF-001 |
| EMP-MAINT-008 | Panel: maintenance calendar, plans and work orders | F6 | - | WEB | M | P1 | EMP-MAINT-002, EMP-MAINT-004, EMP-PLAT-008 |
| EMP-MAINT-009 | App: report a problem and see asset restrictions | F6 | - | APP | S | P1 | EMP-MAINT-003, EMP-MAINT-005 |
| EMP-MAINT-010 | Tyres and component history | F6 | - | API | M | P3 | EMP-MAINT-004 |
| EMP-MAINT-011 | Parts inventory | F6 | - | API | M | P3 | EMP-MAINT-004 |
| EMP-MAINT-012 | Insurance policies and claim records | F6 | - | API | S | P2 | EMP-FLEET-010, EMP-CHECK-003 |

## EP12 — Internal fuel stock

Detail: [ep12-fuel-stock.md](ep12-fuel-stock.md).

| ID | Title | Phase | Cut | Type | Size | Priority | Depends on |
|---|---|---|---|---|---|---|---|
| EMP-STOCK-001 | Internal tank domain: movements and moving-average valuation | F6 | - | DOM | M | P1 | EMP-PLAT-009, EMP-FUEL-001 |
| EMP-STOCK-002 | Tanks and receipts of purchased fuel into stock | F6 | - | API | M | P1 | EMP-STOCK-001, EMP-FLEET-009, EMP-DOC-003 |
| EMP-STOCK-003 | Dispensing to assets as internal fuelings at average cost | F6 | - | DB | M | P1 | EMP-STOCK-002, EMP-FUEL-002, EMP-EXP-002 |
| EMP-STOCK-004 | Transfers, adjustments, measurements and reconciliation | F6 | - | API | M | P1 | EMP-STOCK-002 |
| EMP-STOCK-005 | Single-count guard between stock purchase and consumption | F6 | - | TEST | S | P1 | EMP-STOCK-003 |
| EMP-STOCK-006 | Panel: tanks, movements and reconciliation | F6 | - | WEB | M | P1 | EMP-STOCK-004, EMP-PLAT-008 |
| EMP-STOCK-007 | App: dispense from an internal tank | F6 | - | APP | S | P1 | EMP-STOCK-003, EMP-FUEL-011 |

## EP13 — Intelligence, findings and reports

Detail: [ep13-intelligence.md](ep13-intelligence.md).

| ID | Title | Phase | Cut | Type | Size | Priority | Depends on |
|---|---|---|---|---|---|---|---|
| EMP-KPI-001 | Indicator registry: formula, inputs, version and insufficiency states | F6 | - | DOM | M | P1 | EMP-PLAT-009 |
| EMP-KPI-002 | Spend and volume read models by dimension and period | F6 | - | DB | M | P1 | EMP-KPI-001, EMP-EXP-002, EMP-FUEL-002 |
| EMP-KPI-003 | Consumption and cost-per-distance or hour indicators | F6 | - | DB | M | P1 | EMP-KPI-001, EMP-FUEL-008, EMP-FUEL-010 |
| EMP-KPI-004 | Total operating cost, maintenance and availability indicators | F6 | - | DB | M | P2 | EMP-KPI-002, EMP-MAINT-006, EMP-FLEET-007 |
| EMP-KPI-005 | Price comparison engine with a methodology snapshot | F6 | - | DOM | M | P1 | EMP-KPI-001, EMP-FUEL-014 |
| EMP-KPI-006 | Panel: overview dashboard and drill-downs | F6 | - | WEB | M | P1 | EMP-KPI-002, EMP-KPI-003, EMP-PLAT-008 |
| EMP-FIND-001 | Findings domain: types, severity, evidence and lifecycle | F6 | - | DB | M | P1 | EMP-PLAT-003, EMP-AUTH-003 |
| EMP-FIND-002 | Detection batch 1: duplicate fueling, incoherent reading, tank over capacity | F6 | - | JOB | M | P1 | EMP-FIND-001, EMP-FUEL-006, EMP-METER-003, EMP-FUEL-005 |
| EMP-FIND-003 | Detection batch 2: consumption outlier, document divergence, price above reference | F6 | - | JOB | M | P1 | EMP-FIND-001, EMP-FUEL-008, EMP-DOC-010, EMP-KPI-005 |
| EMP-FIND-004 | Findings review workflow and panel | F6 | - | WEB | M | P1 | EMP-FIND-001, EMP-PLAT-008 |
| EMP-FIND-005 | Reprocessing and analysis versioning | F6 | - | JOB | S | P1 | EMP-FIND-002, EMP-FIND-003 |
| EMP-RPT-001 | Asynchronous export infrastructure: job, private file and expiring link | F3 | - | JOB | M | P1 | EMP-PLAT-006, EMP-PLAT-010, EMP-PRIV-005 |
| EMP-RPT-002 | CSV and spreadsheet exports for operational lists | F3 | - | API | M | P1 | EMP-RPT-001, EMP-AUTH-011 |
| EMP-RPT-003 | PDF reports: trip logbook, fueling statement and period summary | F6 | - | JOB | M | P1 | EMP-RPT-001, EMP-TRIP-004, EMP-FUEL-003 |
| EMP-RPT-004 | Scheduled reports and delivery | F6 | - | JOB | S | P2 | EMP-RPT-002, EMP-NOTIF-001 |
| EMP-RPT-005 | Full organization data export | F6 | - | JOB | M | P1 | EMP-RPT-001 |
| EMP-RPT-006 | Audit trail viewer | F6 | - | WEB | S | P1 | EMP-PLAT-003, EMP-AUTH-011, EMP-PLAT-008 |

## EP14 — Station claims and representation

Detail: [ep14-station-claims.md](ep14-station-claims.md).

| ID | Title | Phase | Cut | Type | Size | Priority | Depends on |
|---|---|---|---|---|---|---|---|
| EMP-CLAIM-001 | Decide organization-held station authority, reconciled with ADR-017 and the contract freeze | F7 | - | DEC | S | P1 | EMP-GOV-015 |
| EMP-CLAIM-002 | Claim on behalf of an organization, bound to the requesting member, on the existing claim model | F7 | - | API | M | P1 | EMP-CLAIM-001, EMP-ORG-011, EMP-AUTH-003 |
| EMP-CLAIM-003 | Station search and duplicate prevention at claim intake | F7 | - | API | S | P1 | EMP-CLAIM-002 |
| EMP-CLAIM-004 | Proportional evidence intake and the information-request loop | F7 | - | API | M | P1 | EMP-CLAIM-002 |
| EMP-CLAIM-005 | Review decisions, including concurrent claims | F7 | - | INT | M | P1 | EMP-CLAIM-004, EMP-BACK-002 |
| EMP-CLAIM-006 | Contest and appeal cases | F7 | - | API | M | P1 | EMP-CLAIM-005 |
| EMP-CLAIM-007 | Authority lifecycle: suspend, expire, revoke, revalidate | F7 | - | JOB | M | P1 | EMP-CLAIM-005 |
| EMP-CLAIM-008 | Station teams: representatives, managers and station scopes | F7 | - | API | M | P1 | EMP-CLAIM-007, EMP-AUTH-006 |
| EMP-CLAIM-009 | Networks: several stations under one organization | F7 | - | API | M | P1 | EMP-CLAIM-007 |
| EMP-CLAIM-010 | Operator succession, CNPJ change, closure and catalog merge effects | F7 | - | JOB | M | P1 | EMP-CLAIM-007 |
| EMP-CLAIM-011 | Representation badge read model and wording | F7 | - | API | S | P1 | EMP-CLAIM-007 |
| EMP-CLAIM-012 | Portal: claim wizard and status tracking | F7 | - | WEB | M | P1 | EMP-CLAIM-004, EMP-PLAT-008 |
| EMP-CLAIM-013 | Representative exit and last-administrator recovery | F7 | - | API | S | P1 | EMP-CLAIM-008 |

## EP15 — Station commerce and agreements

Detail: [ep15-station-commerce.md](ep15-station-commerce.md).

| ID | Title | Phase | Cut | Type | Size | Priority | Depends on |
|---|---|---|---|---|---|---|---|
| EMP-STN-001 | Commercial profile fields and publication through profile revisions | F7 | - | API | M | P1 | EMP-CLAIM-007 |
| EMP-STN-002 | Station-announced prices: own source, validity, conditions and history | F7 | - | DB | M | P1 | EMP-CLAIM-007, EMP-PLAT-009 |
| EMP-STN-003 | Price display rules across announced, community, official and private prices | F7 | - | API | S | P1 | EMP-STN-002 |
| EMP-STN-004 | Promotions: labelled and time-bound | F7 | - | API | S | P2 | EMP-STN-002 |
| EMP-STN-005 | Official replies and moderation requests | F7 | - | API | S | P1 | EMP-CLAIM-008 |
| EMP-STN-006 | Catalog correction requests through directory governance | F7 | - | API | S | P1 | EMP-CLAIM-007 |
| EMP-STN-007 | Interaction metrics with aggregation thresholds | F7 | - | JOB | M | P2 | EMP-STN-001 |
| EMP-STN-008 | Portal: station dashboard, profile, prices, promotions and reviews | F7 | - | WEB | L | P1 | EMP-STN-001, EMP-STN-002, EMP-STN-004, EMP-STN-005, EMP-PLAT-008 |
| EMP-STN-009 | Portal: network overview | F7 | - | WEB | S | P2 | EMP-STN-008, EMP-CLAIM-009 |
| EMP-STN-010 | Ranking neutrality and trust-independence tests | F7 | - | TEST | S | P1 | EMP-STN-002 |
| EMP-AGREE-001 | Agreement domain: parties, scope, price terms and validity | F7 | - | DOM | M | P1 | EMP-PLAT-009 |
| EMP-AGREE-002 | Agreement lifecycle: propose, accept, suspend, end | F7 | - | API | M | P1 | EMP-AGREE-001, EMP-CLAIM-007, EMP-ORG-011 |
| EMP-AGREE-003 | Private negotiated price tables | F7 | - | DB | M | P1 | EMP-AGREE-002 |
| EMP-AGREE-004 | Authorisations and vouchers: limits, expiry, single use and cancellation | F7 | - | API | M | P1 | EMP-AGREE-002, EMP-BUDG-003 |
| EMP-AGREE-005 | Redemption at the station: validation, attempt limit and idempotent confirmation | F7 | - | API | M | P1 | EMP-AGREE-004 |
| EMP-AGREE-006 | Transaction to fleet fueling and station receivable with minimal sharing | F7 | - | DOM | M | P1 | EMP-AGREE-005, EMP-FUEL-002 |
| EMP-AGREE-007 | Statements and invoice reconciliation | F7 | - | API | M | P2 | EMP-AGREE-006, EMP-FIN-005 |
| EMP-AGREE-008 | Panel and portal: agreements, vouchers and redemption | F7 | - | WEB | L | P1 | EMP-AGREE-004, EMP-AGREE-005, EMP-PLAT-008 |
| EMP-AGREE-009 | Cross-organization sharing guard tests | F7 | - | TEST | S | P1 | EMP-AGREE-006 |

## EP16 — Subscriptions, entitlements and public products

Detail: [ep16-subscriptions.md](ep16-subscriptions.md).

| ID | Title | Phase | Cut | Type | Size | Priority | Depends on |
|---|---|---|---|---|---|---|---|
| EMP-SUBS-001 | Organization product, plan and entitlement catalog as data, beside the account tier catalog | F8 | - | DB | M | P1 | EMP-PLAT-005 |
| EMP-SUBS-002 | Subscription lifecycle state machine | F8 | - | DOM | M | P1 | EMP-SUBS-001 |
| EMP-SUBS-003 | Organization entitlement evaluation: capability gates and measurable limits as a new entitlement source | F8 | - | DOM | M | P1 | EMP-SUBS-001, EMP-AUTH-004 |
| EMP-SUBS-004 | Organization billing on the existing payment providers: offers, idempotent event intake and reconciliation | F8 | - | INT | L | P1 | EMP-GOV-009, EMP-SUBS-002 |
| EMP-SUBS-005 | Organization effects per subscription state, including queued offline operations | F8 | - | API | M | P1 | EMP-SUBS-002, EMP-ORG-009, EMP-OFFL-012 |
| EMP-SUBS-006 | Upgrade, downgrade, overage and resource archive or reactivation | F8 | - | API | M | P1 | EMP-SUBS-003 |
| EMP-SUBS-007 | Trial and activation | F8 | - | API | S | P1 | EMP-SUBS-002 |
| EMP-SUBS-008 | Panel: plan and billing | F8 | - | WEB | M | P1 | EMP-SUBS-006, EMP-SUBS-007, EMP-PLAT-008 |
| EMP-SUBS-009 | Usage metering: active vehicles, stations, storage and API volume | F8 | - | JOB | M | P1 | EMP-SUBS-001 |
| EMP-SUBS-010 | Individual Premium kept separate from organization plans in evaluation and interface | F8 | - | DOM | M | P2 | EMP-SUBS-001 |
| EMP-SUBS-011 | Invoices and the service tax document boundary | F8 | - | INT | S | P2 | EMP-SUBS-004 |
| EMP-PUB-001 | Personal and enterprise separation in app storage and interface | F1 | C1 | APP | M | P0 | EMP-PLAT-011 |
| EMP-PUB-002 | Voluntary price publication from an enterprise fueling | F6 | - | API | M | P2 | EMP-FUEL-003 |
| EMP-PUB-003 | Personal vehicles and history stay personal, with explicit copy only | F2 | - | DOM | S | P1 | EMP-PUB-001, EMP-FLEET-002 |

## EP17 — Abastevo Data

Detail: [ep17-data.md](ep17-data.md).

| ID | Title | Phase | Cut | Type | Size | Priority | Depends on |
|---|---|---|---|---|---|---|---|
| EMP-DATA-001 | Dataset catalog and methodology records on top of the enterprise export module | F8 | - | DOM | M | P1 | EMP-GOV-016 |
| EMP-DATA-002 | Organization API keys: issue, rotate, revoke and scopes | F8 | - | API | M | P1 | EMP-ORG-003, EMP-AUTH-003 |
| EMP-DATA-003 | Metering, quotas and rate limits per key | F8 | - | API | M | P1 | EMP-DATA-002, EMP-SUBS-009 |
| EMP-DATA-004 | Regional reports and historical series endpoints | F8 | - | API | M | P1 | EMP-DATA-001, EMP-DATA-002 |
| EMP-DATA-005 | Station monitoring: watchlists and change notifications | F8 | - | API | M | P2 | EMP-DATA-004, EMP-NOTIF-001 |
| EMP-DATA-006 | Market comparison and custom report requests | F8 | - | JOB | M | P2 | EMP-DATA-004, EMP-RPT-001 |
| EMP-DATA-007 | Privacy guard: no private enterprise data and aggregation thresholds | F8 | - | TEST | S | P1 | EMP-DATA-004 |
| EMP-DATA-008 | Portal: keys, usage and documentation | F8 | - | WEB | M | P1 | EMP-DATA-002, EMP-DATA-003, EMP-PLAT-008 |

## EP18 — Imports, notifications and integrations

Detail: [ep18-imports-notifications-integrations.md](ep18-imports-notifications-integrations.md).

| ID | Title | Phase | Cut | Type | Size | Priority | Depends on |
|---|---|---|---|---|---|---|---|
| EMP-IMPORT-001 | Import pipeline core: upload, parse, map, preview and validate rows | F2 | - | API | L | P1 | EMP-PLAT-006, EMP-AUTH-003, EMP-PLAT-010 |
| EMP-IMPORT-002 | Import employees | F2 | - | API | S | P1 | EMP-IMPORT-001, EMP-IDN-002 |
| EMP-IMPORT-003 | Import assets | F2 | - | API | S | P1 | EMP-IMPORT-001, EMP-FLEET-002 |
| EMP-IMPORT-004 | Import historical fuelings and meter readings | F3 | - | API | M | P2 | EMP-IMPORT-001, EMP-FUEL-002, EMP-METER-002 |
| EMP-IMPORT-005 | Error report, correction and undo strategy | F2 | - | API | M | P1 | EMP-IMPORT-001 |
| EMP-IMPORT-006 | Panel: import wizard | F2 | - | WEB | M | P1 | EMP-IMPORT-001, EMP-IMPORT-005, EMP-PLAT-008 |
| EMP-NOTIF-001 | Notification core: inbox, preferences, de-duplication, limits and history | F3 | - | DB | M | P1 | EMP-PLAT-010, EMP-AUTH-003 |
| EMP-NOTIF-002 | Delivery channels: Android local and push, and e-mail, with retries | F3 | - | INT | M | P1 | EMP-NOTIF-001, EMP-PLAT-012 |
| EMP-NOTIF-003 | Operational alert subscriptions | F3 | - | API | S | P2 | EMP-NOTIF-001 |
| EMP-NOTIF-004 | App and panel notification centres | F3 | - | WEB | M | P1 | EMP-NOTIF-001, EMP-PLAT-008, EMP-IDN-018 |
| EMP-INTEG-001 | Integration registry: connections, secrets and health | F8 | - | DB | M | P1 | EMP-PLAT-002, EMP-AUTH-003 |
| EMP-INTEG-002 | Telematics ingestion port | F8 | - | INT | M | P3 | EMP-INTEG-001, EMP-GPS-004 |
| EMP-INTEG-003 | Fuel card statement import | F8 | - | INT | M | P2 | EMP-INTEG-001, EMP-FIN-005, EMP-IMPORT-001 |
| EMP-INTEG-004 | Authorised fiscal document retrieval with the organization's certificate | F8 | - | INT | L | P2 | EMP-INTEG-001, EMP-GOV-011, EMP-DOC-006 |
| EMP-INTEG-005 | Optional server-side extraction provider | F8 | - | INT | M | P3 | EMP-INTEG-001, EMP-DOC-009 |
| EMP-INTEG-006 | Outbound webhooks for organization systems | F8 | - | INT | M | P2 | EMP-INTEG-001, EMP-PLAT-010 |
| EMP-INTEG-007 | Accounting system connectors on top of the export layouts | F8 | - | INT | M | P3 | EMP-INTEG-001, EMP-FIN-006 |

## EP19 — Internal administration and support

Detail: [ep19-back-office.md](ep19-back-office.md).

| ID | Title | Phase | Cut | Type | Size | Priority | Depends on |
|---|---|---|---|---|---|---|---|
| EMP-BACK-001 | Platform operator model for enterprise: staff roles, admin sessions and audit reused from the admin suite | F1 | - | DOM | M | P1 | EMP-GOV-004, EMP-PLAT-003 |
| EMP-BACK-002 | Review queues in the admin suite: claims, legal-entity verification and moderation hand-off | F7 | - | INT | M | P1 | EMP-BACK-001 |
| EMP-BACK-003 | Support access: time-boxed, organization-approved and audited | F3 | - | API | M | P1 | EMP-BACK-001, EMP-AUTH-003 |
| EMP-BACK-004 | Organization administration commands: suspend, reactivate and quality tools | F1 | - | INT | S | P1 | EMP-BACK-001, EMP-ORG-009 |
| EMP-BACK-005 | Admin-web screens for enterprise administration and support | F7 | - | WEB | M | P2 | EMP-BACK-002, EMP-BACK-003 |
| EMP-BACK-006 | Subscription administration: credits and manual adjustments | F8 | - | INT | S | P2 | EMP-BACK-001, EMP-SUBS-002 |

## EP20 — Release gates and validation

Detail: [ep20-release.md](ep20-release.md).

| ID | Title | Phase | Cut | Type | Size | Priority | Depends on |
|---|---|---|---|---|---|---|---|
| EMP-REL-001 | Cut C1 end-to-end scenario and isolation gate | F2 | C1 | TEST | M | P0 | EMP-TRIP-006, EMP-TRIP-005, EMP-FUEL-012, EMP-FUEL-011, EMP-DOC-015, EMP-DOC-014, EMP-APPR-005, EMP-OFFL-014, EMP-OFFL-010, EMP-OFFL-007, EMP-OFFL-004, EMP-ORG-014, EMP-IDN-016, EMP-FLEET-012, EMP-METER-003, EMP-AUTH-009, EMP-AUTH-004, EMP-PUB-001, EMP-PRIV-003, EMP-PLAT-013 |
| EMP-REL-002 | Cut C1 pilot activation, runbook and rollback | F2 | C1 | REL | S | P0 | EMP-REL-001, EMP-PLAT-005, EMP-GOV-005, EMP-GOV-012 |
| EMP-REL-003 | M1 gate: access and isolation acceptance | F1 | - | TEST | S | P1 | EMP-AUTH-009, EMP-IDN-008, EMP-IDN-010, EMP-ORG-009 |
| EMP-REL-004 | M3 gate: fuelings, documents, expenses and approvals acceptance | F3 | - | TEST | M | P1 | EMP-DOC-010, EMP-FIN-004, EMP-BUDG-004, EMP-APPR-004, EMP-FUEL-008 |
| EMP-REL-005 | M4 gate: trips, logbook, sync and tracking fundamentals acceptance | F4 | - | TEST | M | P1 | EMP-OFFL-013, EMP-TRIP-015, EMP-GPS-006, EMP-GPS-010 |
| EMP-REL-006 | M5 gate: navigation and live operations acceptance | F5 | - | TEST | M | P1 | EMP-ROUTE-006, EMP-GPS-008, EMP-GPS-013 |
| EMP-REL-007 | M6 gate: intelligence, maintenance and stock acceptance | F6 | - | TEST | M | P1 | EMP-KPI-006, EMP-FIND-004, EMP-MAINT-008, EMP-STOCK-005 |
| EMP-REL-008 | M7 gate: station representation and commerce acceptance | F7 | - | TEST | M | P1 | EMP-CLAIM-006, EMP-STN-010, EMP-AGREE-009 |
| EMP-REL-009 | M8 gate: subscriptions, Data and integrations acceptance | F8 | - | TEST | M | P1 | EMP-SUBS-005, EMP-DATA-007, EMP-SUBS-004 |
| EMP-REL-010 | Capacity benchmark with a synthetic fleet fixture | F6 | - | TEST | M | P1 | EMP-GPS-004, EMP-KPI-002 |

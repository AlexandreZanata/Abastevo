# Abastevo enterprise suite — planning package

Status: **F0 LOCAL_DONE / INTEGRATION_PENDING**, 2026-11-11.
[The local checkpoint](evidence/f0-local-checkpoint.md) records the recovered
sources, accepted owner decisions and passing validation. Local F1 enters
from synchronized main/dev `d8bdc642` after verified integration of the
ADR-019 source batch. [ADR-025](../adr/025-private-abastevo-empresas-organization-boundary.md)
governs private organization data. Real-data, station, provider and release
gates remain recorded in [the decision register](16-decisions.md).

F1 is **IN_PROGRESS**. EMP-PLAT-001 through EMP-PLAT-011 and EMP-PLAT-013/014/015 are
LOCAL_DONE / INTEGRATION_PENDING with organization ID/scope values,
repository conventions, scoped database transactions, the PostGIS isolation
harness, atomic organization audit storage, shared API conventions,
[audited capability switches](evidence/f1-plat005-capability-flags.md),
[private original transfers](evidence/f1-plat006-private-document-storage.md),
[isolated company web sessions](evidence/f1-plat007-company-web-session.md),
the [company web shell](evidence/f1-plat008-business-web-shell.md),
[exact reconciliation/allocation values](evidence/f1-plat009-exact-values.md),
[transactional enterprise events](evidence/f1-plat010-transactional-events.md),
the [partitioned Android foundation](evidence/f1-plat011-android-foundation.md),
[private enterprise telemetry](evidence/f1-plat013-private-telemetry.md),
[scoped write-rate admission](evidence/f1-plat014-scoped-rate-admission.md),
and the [operation-time kernel](evidence/f1-plat015-operation-time.md).
The [IDN-016 company panel](evidence/f1-idn016-company-identity-panel.md) now
supplies narrow current-account context discovery and actual employee/invitation
composition under server authority. [IDN-017 native invitations](evidence/f1-idn017-android-invitation.md)
are LOCAL_DONE / INTEGRATION_PENDING, including six actual physical-device
journeys. [IDN-018 context selection](evidence/f1-idn018-android-contexts.md) is
also LOCAL_DONE / INTEGRATION_PENDING, including twelve final physical tests
covering invitation regression, context switching and actual cold-process entry.
Android real-data persistence remains unavailable
until EMP-PRIV-003 supplies and proves the protected store. EMP-PLAT-012
awaits the EMP-GOV-014 channel decision. EMP-PRIV-003 has its F1 source
implementation, with acceptance IN_PROGRESS:
[protected device store](evidence/f1-priv003-protected-device-store.md).
Native device/backup/unlock proof and current-server composition remain
owed, so protected persistence stays unavailable in the application graph.
The [organization aggregate](evidence/f1-org001-organization-domain.md) and
[organization persistence](evidence/f1-org002-organization-persistence.md) have
tested local checkpoints, followed by the structure/access tasks recorded in
the current backlog and PROGRESS. EMP-PLAT-016 belongs to F3. Domain aggregate coverage,
native protected-store proof and the existing activation gates remain owed.
[Kernel evidence](evidence/f1-plat001-tenancy-kernel.md) and
[database evidence](evidence/f1-plat002-database-isolation.md), with
[audit evidence](evidence/f1-plat003-organization-audit.md) and
[API evidence](evidence/f1-plat004-organization-api.md)

The package specifies four fronts — **Abastevo Motoristas** (kept as it is), **Abastevo Empresas (Frotas)**, **Abastevo Postos** and **Abastevo Data** — from business rules down to microtasks.

## Reading order

Business logic comes first; technical design, interfaces and tasks follow.

| # | Document | What it answers |
|---|---|---|
| 00 | [Conventions](00-conventions.md) | Identifier formats, the module and task templates, validation profiles, the consistency check |
| 01 | [Diagnosis](01-diagnosis.md) | What exists in the repository today, with paths and evidence labels; where to extend and where to separate; conflicts found |
| 02 | [Product vision](02-product-vision.md) | The four fronts, audiences, problems, scope by module and phase, what the suite is not, commercial hypotheses |
| 03 | [Glossary](03-glossary.md) | One meaning per term; status words with a fixed meaning |
| 04 | [Shared business rules](04-business-rules.md) | Tenancy, source separation, honest status, exact values, time, audit, review lifecycle, amendments; generated rule index |
| 05 | [Journeys](05-journeys.md) | Thirteen end-to-end journeys with rules and exceptions |
| 06 | [Domain model](06-domain-model.md) | Contexts, aggregates and owners, relations, constraints |
| 07 | [Permissions](07-permissions.md) | Roles, scopes, permission registry, the action × role × scope matrix, active context |
| 08 | [State model](08-state-model.md) | Operational, sync and review states kept apart; catalog of every state machine |
| 09 | [Architecture](09-architecture.md) | Modules, request pipeline, API conventions, persistence, events, panel refresh, storage, capacity hypotheses, activation and rollback |
| 10 | [Offline and sync](10-offline-sync.md) | Operation queue, acknowledgements, intake decisions, review queue, the offline matrix |
| 11 | [Privacy and security](11-privacy-security.md) | Isolation, minimisation, retention schedule, devices, points that need legal validation |
| 12 | [App experience](12-ux-app.md) | Enterprise mode: information architecture, shared states, flows, screen index |
| 13 | [Web experience](13-ux-web.md) | Company panel, station portal, internal administration |
| 14 | [Roadmap](14-roadmap.md) | Phases F0–F8, milestones, the first operational cut, critical path, parallel work, effort hypotheses |
| 15 | [Validation](15-validation.md) | Risks and how each is proven, fixtures, scenarios with exact numbers, exit criteria, check result |
| 16 | [Decisions](16-decisions.md) | Decisions proposed, conflicts with existing rules, assumptions, open questions, what is actually blocking |
| 17 | [Traceability](17-traceability.md) | Every requirement of the request → module → rule → use case → entity → task → acceptance |
| 18 | [External references](18-references.md) | Facts taken from outside the repository, with source and date; unverified items marked |

## Module specifications

Each follows the same twelve sections ([00-conventions](00-conventions.md) §4).

| Front | Module | Codes | Backlog |
|---|---|---|---|
| Empresas | [Organizations](modules/organizations.md) | ORG | [EP02](backlog/ep02-organizations.md) |
| Empresas | [Identity and employee accounts](modules/identity.md) | IDN | [EP03](backlog/ep03-identity-access.md) |
| Empresas | [Permissions](07-permissions.md) | AUTH | [EP03](backlog/ep03-identity-access.md) |
| Empresas | [Offline and sync](10-offline-sync.md) | OFFL | [EP04](backlog/ep04-sync.md) |
| Empresas | [Fleet and meters](modules/fleet.md) | FLEET, METER | [EP05](backlog/ep05-fleet-meters.md) |
| Empresas | [Trips and logbook](modules/trips.md) | TRIP | [EP06](backlog/ep06-trips.md) |
| Empresas | [Tracking, routes and navigation](modules/gps-navigation.md) | GPS, ROUTE | [EP07](backlog/ep07-gps-navigation.md) |
| Empresas | [Fuelings and consumption](modules/fuelings.md) | FUEL | [EP08](backlog/ep08-fuelings.md) |
| Empresas | [Documents](modules/documents.md) | DOC | [EP09](backlog/ep09-documents.md) |
| Empresas | [Expenses, approvals, budgets, finance](modules/expenses-finance.md) | EXP, APPR, BUDG, FIN | [EP10](backlog/ep10-expenses-finance.md) |
| Empresas | [Maintenance and checklists](modules/maintenance.md) | MAINT, CHECK | [EP11](backlog/ep11-maintenance.md) |
| Empresas | [Internal fuel stock](modules/fuel-stock.md) | STOCK | [EP12](backlog/ep12-fuel-stock.md) |
| Empresas | [Intelligence, findings, reports](modules/intelligence.md) | KPI, FIND, RPT | [EP13](backlog/ep13-intelligence.md) |
| Empresas | [Imports, notifications, integrations](modules/imports-notifications-integrations.md) | IMPORT, NOTIF, INTEG | [EP18](backlog/ep18-imports-notifications-integrations.md) |
| Postos | [Station claims and representation](modules/station-claims.md) | CLAIM | [EP14](backlog/ep14-station-claims.md) |
| Postos | [Station commerce and agreements](modules/station-commerce.md) | STN, AGREE | [EP15](backlog/ep15-station-commerce.md) |
| Data, Motoristas | [Subscriptions and public products](modules/subscriptions.md) | SUBS, PUB | [EP16](backlog/ep16-subscriptions.md) |
| Data | [Abastevo Data](modules/data.md) | DATA | [EP17](backlog/ep17-data.md) |
| Platform | [Back office](modules/back-office.md) | BACK | [EP19](backlog/ep19-back-office.md) |
| Platform | [Architecture](09-architecture.md), [privacy and security](11-privacy-security.md) | PLAT, PRIV | [EP01](backlog/ep01-platform.md) |
| Programme | Governance and discovery; release gates | GOV, REL | [EP00](backlog/ep00-governance.md), [EP20](backlog/ep20-release.md) |

The full task list with phase, cut, type, size and dependencies is [backlog/INDEX](backlog/INDEX.md) (generated).

## How to use this package

- **To decide:** start at [16-decisions](16-decisions.md) §5 (what is blocking) and §2 (conflicts with existing rules), then [backlog/ep00-governance](backlog/ep00-governance.md).
- **To understand the product:** [02-product-vision](02-product-vision.md), [05-journeys](05-journeys.md), then the module of interest.
- **To implement a task:** open its block in `backlog/`, read the documents listed under "Read first", follow the repository's own workflow (`AGENTS.md`), and treat the cited rules as the contract. A task never overrides a rule.
- **To change the plan:** edit the canonical source (a rule in its module, a task in its epic file) and run the check below; never edit a generated block.

## Consistency check

```bash
python3 docs/enterprise/tools/check_plan.py --write
```

The script ([tools/check_plan.py](tools/check_plan.py), standard library only) verifies task identifiers, dependencies, cycles, the cut C1 closure, phase order, required task fields, that every referenced rule, use case, screen, decision and scenario is defined once, and that relative links resolve. With `--write` it regenerates [backlog/INDEX](backlog/INDEX.md), the rule index in [04-business-rules](04-business-rules.md) §9, the screen indexes in [12-ux-app](12-ux-app.md) and [13-ux-web](13-ux-web.md), the lists in [14-roadmap](14-roadmap.md) and the result block in [15-validation](15-validation.md) §9.

## Limits of this package

- The accepted organization boundary is documented in ADR-025. Product behavior and production or legal acceptance still require evidence in their owning tasks.
- It was written against one commit. Facts about the repository must be rechecked when a task starts.
- External facts in [18-references](18-references.md) were read on one date and include items marked unverified.
- Sizes and totals are relative hypotheses, not a schedule.

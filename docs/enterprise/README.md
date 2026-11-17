# Abastevo Empresas

Private fleet, fueling, expense, document and trip management for companies,
built on the same backend and Android app as the public product. Organization
data is isolated by the boundary in
[decision 025](../decisions/025-private-abastevo-empresas-organization-boundary.md):
row-level security, explicit permissions and an active organization context.
The suite is disabled in staging until the activation runbook is applied
([operations](../operations/enterprise-activation.md)). Open work is listed in
the [backlog](backlog/INDEX.md) and summarized in [ROADMAP.md](../../ROADMAP.md).

## Reading order

Business rules first, then design and interfaces.

| Document | What it answers |
|---|---|
| [Conventions](conventions.md) | Identifier formats, the module and task templates, validation profiles |
| [Product vision](product-vision.md) | The four fronts, audiences, problems, scope by module and phase, what the suite is not, commercial hypotheses |
| [Glossary](glossary.md) | One meaning per term; status words with a fixed meaning |
| [Shared business rules](business-rules.md) | Tenancy, source separation, honest status, exact values, time, audit, review lifecycle, amendments; generated rule index |
| [Journeys](journeys.md) | Thirteen end-to-end journeys with rules and exceptions |
| [Domain model](domain-model.md) | Contexts, aggregates and owners, relations, constraints |
| [Permissions](permissions.md) | Roles, scopes, permission registry, the action × role × scope matrix, active context |
| [State model](state-model.md) | Operational, sync and review states kept apart; catalog of every state machine |
| [Architecture](architecture.md) | Modules, request pipeline, API conventions, persistence, events, panel refresh, storage, capacity hypotheses, activation and rollback |
| [Offline and sync](offline-sync.md) | Operation queue, acknowledgements, intake decisions, review queue, the offline matrix |
| [Privacy and security](privacy-security.md) | Isolation, minimisation, retention schedule, devices, points that need legal validation |
| [App experience](ux-app.md) | Enterprise mode: information architecture, shared states, flows, screen index |
| [Web experience](ux-web.md) | Company panel, station portal, internal administration |
| [Validation](testing.md) | Risks and how each is proven, fixtures, scenarios with exact numbers, exit criteria |
| [Decisions](decisions.md) | Decisions proposed, conflicts with existing rules, assumptions, open questions, what is actually blocking |

## Module specifications

Each follows the same twelve sections ([00-conventions](conventions.md) §4).

| Front | Module | Codes | Backlog |
|---|---|---|---|
| Empresas | [Organizations](modules/organizations.md) | ORG | [EP02](backlog/ep02-organizations.md) |
| Empresas | [Identity and employee accounts](modules/identity.md) | IDN | [EP03](backlog/ep03-identity-access.md) |
| Empresas | [Permissions](permissions.md) | AUTH | [EP03](backlog/ep03-identity-access.md) |
| Empresas | [Offline and sync](offline-sync.md) | OFFL | [EP04](backlog/ep04-sync.md) |
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
| Platform | [Architecture](architecture.md), [privacy and security](privacy-security.md) | PLAT, PRIV | [EP01](backlog/ep01-platform.md) |
| Programme | Governance and discovery; release gates | GOV, REL | [EP00](backlog/ep00-governance.md), [EP20](backlog/ep20-release.md) |

The full task list with phase, cut, type, size and dependencies is [backlog/INDEX](backlog/INDEX.md) (generated).

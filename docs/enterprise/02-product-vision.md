# Product vision and scope

Status: **PLANNED**, 2026-11-10. Authority: owner request of 2026-11-10 to plan the complete enterprise evolution of Abastevo. This document records the product intention; it changes no runtime behaviour and lifts no existing gate. Conflicts with existing contracts are listed in [16-decisions](16-decisions.md) §2.

## 1. Decision recorded

The owner asked for Abastevo to become an ecosystem that keeps the public driver and community experience and adds a complete suite for companies with fleets, employees and fuel stations. The request states that it **explicitly widens** the earlier proposal, which centred the first business product on fuel only. GPS, logbook, employee accounts and complete company management are in the scope to be planned now.

Earlier documents defer or exclude these capabilities (for example the Premium contract excludes "employees, shared fleet access, business tenants", and AGENTS rule 12 says "no tenant model"). This plan does not delete those statements. It records the new intention, specifies every requested module, and names the decisions the owner must accept before each part can be built ([16-decisions](16-decisions.md)). Planning the whole suite and building it gradually are compatible: one usable first flow is defined (cut C1 in [14-roadmap](14-roadmap.md)), and every later module is still specified and decomposed.

## 2. Four fronts

| Front | Who uses it | What it is | Relation to what exists |
|---|---|---|---|
| **Abastevo Motoristas** | Drivers and the community | Public app: prices, map, community, personal vehicles, history, alerts, individual Premium | Exists and stays as it is. Free guarantees (B-BR-PR01) are preserved. |
| **Abastevo Empresas (Frotas)** | Companies with vehicles and employees | Organizations, employees, assets, operations, GPS, logbook, fuelings, documents, expenses, maintenance, controls, intelligence | New. Reuses exact-value engines, the command protocol, the map engine and the design system. |
| **Abastevo Postos** | Fuel station operators and networks | Claim of establishments, verified representatives, teams, commercial profile, announced prices, relationship, agreements with fleets | Extends the existing `stationprofile` design (ADR-017). Implementation is under the Stage D hold until the owner lifts it for this scope. |
| **Abastevo Data** | Analysts and systems | Market reports, monitoring, exports and a commercial API with explicit origin and quality | Extends the enterprise data and static portal plan (ESP). Commercial redistribution rights are still pending. |

Naming note. Three different things already use the word "enterprise" in this repository: the account tier `ENTERPRISE` of ADR-023 (a label with the Premium bundle and no organization right), the "Enterprise data offering" of ESP (one regional price brief), and the single-payer Enterprise candidate in the payment offer policy. None of them is the company suite planned here. To avoid a fourth meaning, this plan calls the customer entity an **organization**, the fleet product **Abastevo Empresas**, and never uses a tier named `ENTERPRISE` for it ([16-decisions](16-decisions.md) EDR-002).

## 3. Audiences and problems

| Audience | Problem today | Result wanted |
|---|---|---|
| Owner or manager of a small or mid-size fleet (delivery, services, transport, farm) | Fuel and trip data live on paper, in chat messages and in spreadsheets. Receipts arrive late or never. Nobody knows the real cost per kilometre of a vehicle. | One place where each employee records what happened, with evidence, and the manager reviews it. |
| Driver or field employee | Must keep receipts, fill forms after the fact and explain differences from memory. Often works without signal. | Record a trip, a fueling or an expense in seconds, offline, and see whether the company received it. |
| Finance or administrative staff | Reimbursements and card statements are reconciled by hand; the same cost is counted twice or not at all. | Each cost appears once, with its document, its approval and its payment status. |
| Supervisor | Does not know which vehicles are out, who has them, or whether a trip was closed. | A current view of operations with honest freshness, and a queue of what needs attention. |
| Station operator or network | Public profile is out of date; cannot answer reviews as the business; cannot offer conditions to fleets. | Prove representation once, manage the commercial profile, announce prices under its own label, and serve fleets under an agreement. |
| Analyst at a distributor, association or fleet | Public price data is hard to consume and compare. | Documented datasets and an API with origin, coverage and method. |
| Abastevo operator | Reviews and support have no structured queues. | Review queues, audited support access and plan administration, without broad access to customer documents. |

Target organizations in the first phases are companies that use vehicles to work but are not built around telematics hardware: delivery companies, service providers, small carriers, farms, and businesses with own, rented or third-party vehicles. Large carriers with dedicated tracking hardware are served later through integrations ([modules/imports-notifications-integrations](modules/imports-notifications-integrations.md)).

## 4. The central experience

1. A company signs up and creates its **organization**. It configures units and, when useful, cost centres and teams.
2. It registers or invites **employees**. An employee can exist in the register before having a login. The company gives each employee an individual access with specific permissions: either the employee's own Abastevo account joins the organization by invitation, or the company provisions a managed corporate access that the employee activates on the device.
3. It registers **vehicles and machines** and assigns them to people and operations.
4. The employee opens the app and enters **enterprise mode**, which always shows which company is active. There the employee receives activities, uses navigation, records trips and logbook events, informs kilometres or hours, fuelings, expenses and receipts.
5. Everything recorded belongs to the company that was active when it was created. It reaches the **panel** of the people responsible, even after periods without internet, and the app always says whether a record is saved on the device, sent, accepted by the company or needs review.
6. Managers review, approve or return for correction. Costs, consumption and findings are computed from the reviewed facts, and insufficient data is shown as insufficient.

## 5. Product principles

These hold for every module. They restate existing project rules where those exist.

1. **Business logic first.** Behaviour, rules and states are specified before screens, tables or endpoints.
2. **The organization owns its records.** An operation belongs to the organization chosen when it was created, and that never changes ([04-business-rules](04-business-rules.md) BR-TEN-004).
3. **Personal stays personal.** Personal vehicles, history and community identity never move into an organization on their own, and a private fueling never becomes a public contribution on its own (BR-SRC-003).
4. **Sources stay distinct.** Official ANP price, community observation, station-announced price and privately negotiated price are four different things and are never merged (BR-SRC-001).
5. **Payment never buys trust.** No plan, agreement or advertisement changes confidence, ranking, moderation or reviews (BR-SRC-002; existing B-BR-009, ADR-012).
6. **Honest status.** "Validated", "verified" and "approved" always say what was checked. Missing data is shown as missing, never as zero (BR-TRUTH-001…004).
7. **Offline is normal.** Field work continues without signal; nothing recorded is dropped silently ([10-offline-sync](10-offline-sync.md)).
8. **Location only for authorized work.** Tracking exists only during an operation the employee started in enterprise mode, with a visible indicator. No personal trips are collected ([modules/gps-navigation](modules/gps-navigation.md)).
9. **Manual first.** Every flow works with manual entry. OCR, fiscal consultation, payment and hardware integrations are optional layers with a fallback.
10. **Reuse before adding.** The existing Go modular monolith, PostgreSQL/PostGIS, job queue, exact-value engines, command protocol, map engine and design system are extended; a new service or paid dependency needs a measured reason and a recorded decision.

## 6. Scope by front

Each line links to its specification. Phases are defined in [14-roadmap](14-roadmap.md).

### 6.1 Abastevo Empresas (Frotas)

| Module | Content | Phase |
|---|---|---|
| [Organizations](modules/organizations.md) | Organization, legal entities, units, teams, cost centres, groups, lifecycle | F1 (groups F6, closure F8) |
| [Identity](modules/identity.md) | Account, person, employee, driver, membership, credentials, devices, invitations, offboarding | F1 |
| [Permissions](07-permissions.md) | Roles, scopes, active context, entitlement gate | F1 (fine scopes F2, custom roles F6) |
| [Offline sync](10-offline-sync.md) | Operation queue, acknowledgements, review queue, attachments | F1–F4 |
| [Fleet and meters](modules/fleet.md) | Assets, tenure, assignments, availability, odometer and hour-meter readings | F2 |
| [Trips and logbook](modules/trips.md) | Missions, trips, stops, timeline events, closing and review | basic F2, complete F4 |
| [Tracking and navigation](modules/gps-navigation.md) | Route plans, in-app navigation, recorded track, live operations view | F4–F5 |
| [Fuelings and consumption](modules/fuelings.md) | Fueling records, review, consumption and cost methods | basic F3 (in cut C1), complete F3–F6 |
| [Documents](modules/documents.md) | Receipts, fiscal documents, extraction, conference, reconciliation | basic F3 (in cut C1), complete F3–F6 |
| [Expenses and finance](modules/expenses-finance.md) | Expenses, approvals, budgets, payables, reimbursements, advances | F3 |
| [Maintenance](modules/maintenance.md) | Checklists, incidents, preventive and corrective maintenance, expiries | checklists F4, maintenance F6 |
| [Internal fuel stock](modules/fuel-stock.md) | Tanks, purchases, dispensing, reconciliation | F6 |
| [Intelligence](modules/intelligence.md) | Indicators, findings, reports, exports, audit view | exports F3, rest F6 |
| [Imports, notifications, integrations](modules/imports-notifications-integrations.md) | CSV/XLSX import, in-app and external notifications, optional connectors | F2–F8 |

### 6.2 Abastevo Postos

| Module | Content | Phase |
|---|---|---|
| [Station claims](modules/station-claims.md) | Claim, evidence, review, contest, appeal, authority lifecycle, teams, networks | F7 |
| [Station commerce](modules/station-commerce.md) | Commercial profile, announced prices, promotions, replies, metrics, agreements and vouchers with fleets | F7 |

### 6.3 Abastevo Data and public products

| Module | Content | Phase |
|---|---|---|
| [Data](modules/data.md) | Datasets, methodology, API keys, quotas, monitoring | F8 |
| [Subscriptions and public products](modules/subscriptions.md) | Organization plans and entitlements, individual Premium kept separate, voluntary publication | separation F1, plans F8 |

### 6.4 Platform

| Module | Content | Phase |
|---|---|---|
| [Back office](modules/back-office.md) | Internal administration, review queues, audited support access | F1–F8 |
| [Architecture](09-architecture.md), [privacy and security](11-privacy-security.md) | Tenancy, audit, storage, events, retention, isolation | F1 onward |

## 7. What this suite is not

Each boundary has an owner rule so that it cannot be crossed by accident.

| Not this | Why | Rule |
|---|---|---|
| An electronic time clock or legal journey record | Lei 13.103/2015 names "diário de bordo" as one of the instruments for recording a professional driver's journey, and Portaria MTP 671/2021 regulates electronic time registration (REP). The operational logbook planned here produces no AFD/AEJ and is not validated for either purpose. Any labour control is a separate module with its own legal validation. | BR-TRIP (logbook is operational), OQ in [16-decisions](16-decisions.md) |
| A fiscal emission platform | The suite stores, reads and reconciles documents it receives. It does not issue NF-e, NFC-e, CT-e or MDF-e. | [modules/documents](modules/documents.md) |
| Proof of fiscal authenticity | A photo, an extracted text, a well-formed XML or a valid access key do not prove that a document is authorized and not cancelled. | BR-TRUTH-002 |
| A payment institution | Recording a payment made elsewhere works without any integration. Cards, payments and financial services are a separate module with their own dependencies. | [modules/expenses-finance](modules/expenses-finance.md) |
| A fraud detector | Findings point to differences that need review. They never state fraud. | [modules/intelligence](modules/intelligence.md) |
| Dedicated telematics | Tracking uses the employee's phone during authorized operations. Hardware trackers are a later integration. | [modules/gps-navigation](modules/gps-navigation.md) |
| A way to buy reputation | Plans and agreements never change trust, ranking or reviews. | BR-SRC-002 |

## 8. Commercial hypotheses

All figures in this section are **hypotheses from the initial material**. None is approved, none is to be written in code, and the catalog is data, not constants ([modules/subscriptions](modules/subscriptions.md)).

| Product | Hypothesis | Charging unit under evaluation |
|---|---|---|
| Frotas Starter | R$ 149 per month, up to 10 vehicles | active vehicles |
| Frotas Pro | R$ 349 per month, up to 30 vehicles | active vehicles |
| Frotas Business | on request | active vehicles and storage |
| Postos Pro | R$ 99 per month | managed stations |
| Data | from R$ 199 per month | API volume and datasets |
| Premium individual | to validate | per account (existing PRM scope; sale disabled) |

Before any value is recommended, [modules/subscriptions](modules/subscriptions.md) §10 compares charging units and estimates the variable cost per vehicle (storage of documents and tracks, map distribution, support). Interviews and pilots with companies ([backlog/ep00-governance](backlog/ep00-governance.md), EMP-GOV-010) test priorities and prices; they inform packaging and do not replace this specification.

Public position found for one global reference only: Fleetio lists USD 4 to 10 per vehicle per month (annual billing), checked 2026-11-10 at fleetio.com/pricing. Brazilian fleet platforms checked the same day (Ticket Log, Cobli, Infleet, Gestran) publish no prices.

## 9. Measures of success

Measures are aggregate and collected only with a documented purpose.

| Stage | Measure |
|---|---|
| Cut C1 pilot | Share of fuelings that arrive with a legible receipt; median time from fueling to manager review; share of operations recorded offline that are accepted without manual repair; zero cross-organization access in the isolation suite. |
| Fleet product | Active vehicles with at least one closed consumption interval; share of expenses with a reconciled document; findings resolved with a recorded outcome. |
| Station product | Claims decided within the staffed target; announced prices renewed before they expire; no change in community trust metrics attributable to plan status. |
| Data product | Datasets with published method and coverage; API calls served within quota; no private organization data in any dataset. |

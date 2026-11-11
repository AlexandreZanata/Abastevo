# Enterprise plan conventions

Status: **PLANNED**, 2026-11-10. This file defines how every document in `docs/enterprise/` is written, identified and checked. It is the only place where identifier formats, templates and validation profiles are defined; other files reference it.

Nothing in `docs/enterprise/` is runtime proof. A capability exists only when its task is done and its tests pass (AGENTS rule 12: "no target is runtime-complete without tests").

## 1. Language and authority

- Documents, code and commits are in English (AGENTS rule 14). User-facing product copy is specified in Brazilian Portuguese where the wording itself is a rule (badge text, status labels).
- Order of authority inside this plan: [16-decisions](16-decisions.md) (decisions and conflicts) → [04-business-rules](04-business-rules.md) (shared rules) → module specifications in `modules/` → backlog tasks in `backlog/`. A task never overrides a rule; a module never overrides a shared rule.
- Existing repository rules (AGENTS, ADRs, product contracts) govern this plan. Accepted changes are recorded in [16-decisions](16-decisions.md) and the corresponding ADR and contracts. Proposed changes are listed there as conflicts (`CX-nn`).

## 2. Identifier namespaces

New identifiers never reuse an existing namespace. Existing namespaces (checked on `origin/dev` at `a97ba3b3`) are: `BR-001…BR-028` and `UC-001…UC-015` (legacy Android); `B-BR-<NS>-…` and `BUC-<NS>-…` (backend and product contracts, about fifty `<NS>` codes including `DRV`, `MAP`, `MEX`, `PAY`, `ESP`, `PR`, `ENT`, `ADM`, `STF`, `SYNC`, `NAV`, `CF`); task namespaces `P##-T##`, `PAY`, `ESP`, `DRV`, `FOR`, `FQ`, `OPT`, `MEX`, `EE`, `MAP`, `PRM`, `RST`, `CS`, `SEC`; gates `G##`; `ADR-001…023`; `D01…D18`.

The request that originated this plan asked for rule identifiers shaped like `BR-ORG-001`. They are kept, and they differ from every existing rule by having no `B-` prefix and a three-digit number. Module codes that would read like an existing `B-BR-<NS>` code are avoided: offline sync is `OFFL` (not `SYNC`), routes and navigation are `ROUTE` (not `NAV`), internal administration is `BACK` (not `ADM`). The task prefix is `EMP` (Empresas), because `ENT` already names the granted-tier rules of ADR-023.

| Kind | Format | Example | Defined in |
|---|---|---|---|
| Request requirement | `REQ-<section>.<n>` | `REQ-05.3` | [17-traceability](17-traceability.md) |
| Business rule | `BR-<MODULE>-<nnn>` | `BR-ORG-001` | shared: [04-business-rules](04-business-rules.md); module: `modules/*.md` §4 |
| Use case | `UC-<MODULE>-<nn>` | `UC-TRIP-03` | `modules/*.md` §5 |
| Screen | `SCR-<MODULE>-<nn>` | `SCR-FUEL-02` | `modules/*.md` §11 |
| Permission key | `<module>.<resource>.<action>` | `fleet.asset.assign` | [07-permissions](07-permissions.md) |
| Event | `<module>.<entity>.<past-tense>` | `trip.trip.completed` | `modules/*.md` §9 |
| Error code | `UPPER_SNAKE` | `MEMBERSHIP_ENDED` | [09-architecture](09-architecture.md) §6 |
| Indicator | `IND-<nnn>` | `IND-004` | [modules/intelligence](modules/intelligence.md) |
| Finding type | `FT-<nnn>` | `FT-002` | [modules/intelligence](modules/intelligence.md) |
| Microtask | `EMP-<MODULE>-<nnn>` | `EMP-ORG-003` | `backlog/ep*.md` |
| Epic | `EP<nn>` | `EP06` | [backlog/INDEX](backlog/INDEX.md) |
| Phase | `F0…F8` | `F3` | [14-roadmap](14-roadmap.md) |
| Milestone | `M0…M8`, cut `C1` | `C1` | [14-roadmap](14-roadmap.md) |
| Decision | `EDR-<nnn>` | `EDR-004` | [16-decisions](16-decisions.md) |
| Conflict with an existing rule | `CX-<nn>` | `CX-01` | [16-decisions](16-decisions.md) |
| Assumption | `AS-<nn>` | `AS-07` | [16-decisions](16-decisions.md) |
| Open question | `OQ-<nn>` | `OQ-03` | [16-decisions](16-decisions.md) |
| Validation scenario | `VAL-<nn>` | `VAL-05` | [15-validation](15-validation.md) |
| Journey | `JRN-<nn>` | `JRN-03` | [05-journeys](05-journeys.md) |
| Legal validation point | `LV-<nn>` | `LV-05` | [11-privacy-security](11-privacy-security.md) §10.4 |

Module codes used in rule, use-case, screen and task identifiers:

| Code | Scope | Specification |
|---|---|---|
| `TEN`, `SRC`, `TRUTH`, `MONEY`, `TIME`, `AUD`, `REV`, `AMD` | Cross-cutting: tenancy, source separation, honest status wording, exact values, time, audit, review lifecycle, amendments | [04-business-rules](04-business-rules.md) |
| `PLAT`, `PRIV` | Platform foundations, privacy and security | [09-architecture](09-architecture.md), [11-privacy-security](11-privacy-security.md) |
| `ORG` | Organizations, legal entities, units, teams, cost centres | [modules/organizations](modules/organizations.md) |
| `IDN` | Accounts, people, employees, memberships, credentials, devices | [modules/identity](modules/identity.md) |
| `AUTH` | Roles, scopes, permissions, active context | [07-permissions](07-permissions.md) |
| `OFFL` | Offline operation queue and intake | [10-offline-sync](10-offline-sync.md) |
| `FLEET`, `METER` | Assets, assignments, meters | [modules/fleet](modules/fleet.md) |
| `TRIP` | Missions, trips, logbook | [modules/trips](modules/trips.md) |
| `GPS`, `ROUTE` | Tracking, live view, routes, navigation | [modules/gps-navigation](modules/gps-navigation.md) |
| `FUEL` | Fuelings and consumption | [modules/fuelings](modules/fuelings.md) |
| `DOC` | Receipts, fiscal documents, attachments | [modules/documents](modules/documents.md) |
| `EXP`, `APPR`, `BUDG`, `FIN` | Expenses, approvals, budgets, payables | [modules/expenses-finance](modules/expenses-finance.md) |
| `MAINT`, `CHECK` | Maintenance, checklists, incidents | [modules/maintenance](modules/maintenance.md) |
| `STOCK` | Internal fuel tanks | [modules/fuel-stock](modules/fuel-stock.md) |
| `KPI`, `FIND`, `RPT` | Indicators, findings, reports and exports | [modules/intelligence](modules/intelligence.md) |
| `CLAIM` | Station claims and representation | [modules/station-claims](modules/station-claims.md) |
| `STN`, `AGREE` | Station commerce, agreements, vouchers | [modules/station-commerce](modules/station-commerce.md) |
| `SUBS`, `PUB` | Subscriptions, entitlements, public products | [modules/subscriptions](modules/subscriptions.md) |
| `DATA` | Abastevo Data | [modules/data](modules/data.md) |
| `IMPORT`, `NOTIF`, `INTEG` | Imports, notifications, integrations | [modules/imports-notifications-integrations](modules/imports-notifications-integrations.md) |
| `BACK` | Internal administration and support (back office) | [modules/back-office](modules/back-office.md) |
| `GOV`, `REL` | Decisions and discovery; release gates (tasks only) | [backlog/ep00-governance](backlog/ep00-governance.md), [backlog/ep20-release](backlog/ep20-release.md) |

Identifiers are stable. A retired identifier is marked `RETIRED` with its replacement and is never reused.

## 3. Evidence labels

Every statement about the current repository carries one of these labels:

| Label | Meaning |
|---|---|
| `IMPLEMENTED` | Code and tests exist; the path is cited. |
| `PARTIAL` | Code exists, but a listed part is missing or not reachable in production. |
| `DOCUMENTED` | Described in an existing document only; no code. |
| `PROPOSED` | New in this plan. |
| `UNVERIFIED` | Could not be confirmed in this analysis; the limitation is stated. |

Paths to existing files are written as real repository paths. Paths that do not exist yet are always marked `(proposed)`.

## 4. Module specification template

Every file in `modules/` uses these twelve sections, in this order, with these headings. A section that does not apply states why in one line; it is never omitted.

1. **Purpose** — problem solved, users, expected result.
2. **Scope** — basic capabilities, later evolutions, out of scope.
3. **Entities and ownership** — entities, relations, data owner, invariants.
4. **Business rules** — `BR-<MODULE>-nnn`, one rule per line, each testable.
5. **Use cases** — `UC-<MODULE>-nn` with preconditions, main flow, alternatives, result.
6. **States and transitions** — per state machine: actor, validation, effects, reversal.
7. **Roles and permissions** — permission keys used, allowed roles, scopes.
8. **Offline, concurrency, idempotency and corrections**.
9. **Events, notifications and integrations** — produced, consumed.
10. **Data, formulas and examples** — inputs, validations, formulas, outputs, worked numbers.
11. **Screens** — `SCR-<MODULE>-nn` with surface, objective, actor, data, actions, rules, permissions, accessibility, validations and the five states (loading, empty, error, unavailable, forbidden).
12. **Acceptance, dependencies, risks, phase and microtasks**.

Screen surfaces: `APP` (Android enterprise mode), `PANEL` (company panel in the business web), `PORTAL` (station portal in the business web), `ADMIN` (internal administration).

Shared screen states are defined once in [12-ux-app](12-ux-app.md) §3 and [13-ux-web](13-ux-web.md) §3; a screen lists only what differs from those defaults.

## 5. Microtask template

Each task in `backlog/ep*.md` uses this block. Field names are fixed so that the consistency check can parse them.

```markdown
### EMP-XXX-000 — Title
- **Meta:** module XXX · phase F0 · cut C1|- · priority P0|P1|P2|P3 · type T · size XS|S|M|L · state TODO
- **Objective:** what changes for a user or for the system, and which problem it removes.
- **Rules and use cases:** BR-…, UC-…
- **Depends on:** EMP-…, EMP-… (or `none`)
- **Read first:** the minimum documents and sections.
- **Scope:** in: … · out: …
- **Areas:** existing: real paths · new (proposed): paths
- **Steps:** 1) … 2) … 3) …
- **Deliverable:** the observable result.
- **Acceptance:** AC1 …; AC2 …; AC3 …
- **Negative cases:** authorization, isolation, concurrency and error cases that must be tested.
- **Validation:** profiles from §6 · migration: … · rollback: …
- **Risks and assumptions:** …
- **Done when:** the single condition that closes the task.
```

Rules for tasks:

- One task produces one verifiable behaviour in one short working cycle of one agent. "Implement fleets" is an epic, not a task.
- `Depends on` lists task identifiers only. The graph has no cycles; [backlog/INDEX](backlog/INDEX.md) is generated from the task blocks and checked.
- A dependency may sit in a later phase only when that dependency is tagged `cut C1` (pulled forward for the first operational cut).
- `Areas` separates existing paths from proposed ones. A proposed path follows the conventions in [09-architecture](09-architecture.md) §2.
- A field that does not apply says `n/a — <reason>`.
- Task types: `DEC` decision, `DISC` discovery or spike, `DOM` pure domain, `DB` persistence and transactions, `API` use case plus HTTP contract, `APP` Android, `WEB` business web or internal admin, `JOB` background job, `INT` integration or adapter, `OPS` operations, `TEST` cross-cutting validation, `REL` release.
- Sizes are relative and are hypotheses, not a calendar: `XS` about half a cycle, `S` one cycle, `M` two to three cycles, `L` four to six cycles. A cycle is one focused agent session ending in tested, reviewable work. A task that grows past `L` is split with letter suffixes (`EMP-TRIP-002a`), keeping the original identifier as the parent.
- Priorities: `P0` required for cut C1; `P1` core of its phase; `P2` completes its phase; `P3` evolution with no committed phase exit.
- States follow the repository vocabulary: `TODO`, `READY` (dependencies done), `IN_PROGRESS`, `LOCAL_DONE`, `INTEGRATED`, `BLOCKED` (names the blocking decision).

## 6. Validation profiles

A task names the profiles that apply to it. Every profile adds to the baseline in AGENTS rule 5: targeted meaningful tests, affected compilation, formatting and static checks, `git diff --check`, and a secret-surface review. Compile-only is never behaviour evidence. Commands are the evidence-backed ones in [backend/README.md](../../backend/README.md) and [TEST_STRATEGY](../backend/TEST_STRATEGY.md); a profile does not invent command names.

| Profile | Proves | Minimum content |
|---|---|---|
| `V-DOC` | A decision or discovery task is complete | Decision record updated, links resolve, affected identifiers consistent, plan check passes (§7). |
| `V-DOM` | Pure rules | RED → GREEN → REFACTOR unit tests; Go domain packages stay standard-library only; Kotlin `:domain` tests. |
| `V-SQL` | Schema, queries, transactions | Integration tests against real PostGIS (`-tags=integration`), migration applied forward on a populated database, recovery note. |
| `V-ISO` | Tenant isolation | Two-organization fixture: every read, list, write, export, job and file access for organization A is attempted with organization B's identity and returns not-found or forbidden with no data in the error. |
| `V-AUTHZ` | Permissions | For each permission touched: allowed role and scope succeeds; wrong role, wrong scope, ended or suspended membership, suspended organization and missing entitlement are denied. |
| `V-IDEM` | Replay and concurrency | Same operation twice returns the first result; two different operations racing on one resource end in one consistent state; no lost update. |
| `V-MONEY` | Decimal behaviour | Golden vectors shared by Go and Kotlin (proposed `contracts/testdata/enterprise/`), including rounding boundaries. |
| `V-API` | Wire contract | OpenAPI updated, golden vectors validate against it, error envelope and codes match [09-architecture](09-architecture.md) §6. |
| `V-SYNC` | Offline behaviour | Queue survives process death; duplicate, reordered and partial deliveries converge; nothing is dropped silently. |
| `V-APP` | Android behaviour | ViewModel and state tests, Compose state rendering for the five screen states, affected module compilation. Instrumented tests run on an emulator, never on the owner's device. |
| `V-WEB` | Web behaviour | Type check, component or state tests for the five screen states, keyboard and screen-reader checks for the changed screens. |
| `V-JOB` | Background work | Handler is safe to re-run; lease loss, retry and dead-letter paths tested; de-duplication key defined. |
| `V-PRIV` | Privacy | No personal data, location or document content in logs, events or public DTOs; retention behaviour tested with a controlled clock. |
| `V-PERF` | Capacity | Benchmark on a synthetic fixture with the measured numbers recorded; no claim without a measurement. |
| `V-E2E` | End-to-end | The named `VAL-nn` scenario in [15-validation](15-validation.md) passes. |

## 7. Plan consistency check

The plan is checked mechanically whenever it changes. The check verifies:

1. every `EMP-` identifier is defined exactly once, and every dependency exists;
2. the dependency graph has no cycle;
3. every cut C1 task depends only on cut C1 tasks;
4. no task depends on a later-phase task unless that task is cut C1;
5. every `BR-`, `UC-` and `SCR-` identifier referenced anywhere is defined exactly once;
6. every task references at least one rule or use case, or states why not;
7. every `REQ-` row in [17-traceability](17-traceability.md) points to existing identifiers;
8. every relative link between plan documents resolves.

The check script and its last result are recorded in [15-validation](15-validation.md) §9. [backlog/INDEX](backlog/INDEX.md) and the rule index at the end of [04-business-rules](04-business-rules.md) are generated by that script from the canonical sources and are never edited by hand.

## 8. Writing rules

- State behaviour before structure: a rule says what must be true, then a table or contract may show how.
- One fact has one home. Other documents link to it instead of restating it.
- Numbers that are product defaults are written as defaults with their configuration range; numbers that are hypotheses are labelled `(hypothesis)`.
- Examples use synthetic data only. No real CNPJ, plate, person, document or location appears in this plan.
- "Verified", "validated" and "approved" are never used without saying what was checked, by whom and against which source.

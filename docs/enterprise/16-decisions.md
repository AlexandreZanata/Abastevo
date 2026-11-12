# Decisions, conflicts, assumptions and open questions

Status: **M0 LOCAL_DONE / INTEGRATION_PENDING**, 2026-11-11. [ADR-025](../adr/025-private-abastevo-empresas-organization-boundary.md) records the private Abastevo Empresas organization boundary. The [M0 decision record](evidence/f0-m0-decisions.md) records the accepted panel, Android partition and synthetic receipt scope, and the remaining gates. See the [governance backlog](backlog/ep00-governance.md).

## 1. Decisions

| ID | Decision | Why | Alternatives considered | Accepted through |
|---|---|---|---|---|
| EDR-001 | The **organization** is the isolation boundary and the owner of private Abastevo Empresas records. A CNPJ, an establishment and a unit are within it. | One company may have several CNPJs and units; one CNPJ root may belong to unrelated franchises; access must follow an explicit relation, not a registry coincidence. | Tenant = CNPJ root (breaks groups and farms run by natural persons); tenant = unit (no consolidated view). | EMP-GOV-001, accepted in ADR-025 |
| EDR-002 | The customer entity is called **organization**; the fleet product **Abastevo Empresas**. No plan or tier of this suite is named `ENTERPRISE`. | The word already names an account tier (ADR-023), a data offering (ESP) and a payment candidate, each explicitly without organization rights. | Reuse `ENTERPRISE` and redefine it (would contradict three accepted texts and migration 000130). | EMP-GOV-001 |
| EDR-003 | Extend the existing Go modular monolith and PostgreSQL/PostGIS. No new service, broker, cache or database for this suite. | Matches ADR-005 and the measured-need rule; every requirement here fits the existing job queue, storage and API. | Separate enterprise service (operational cost, second deployment, no measured need). | EMP-GOV-001 |
| EDR-004 | The plan lives in `docs/enterprise/` on a branch created from `origin/dev`, with its own identifier namespaces ([00-conventions](00-conventions.md)). | The request suggested the folder; the wiki mirror picks up any `docs/**/*.md`; the primary checkout was 1 069 commits stale. | Spread across `docs/planning`, `docs/product`, `docs/backend` as other fronts do (harder to keep one canonical source for a suite this size). | EMP-GOV-012 |
| EDR-005 | Tenant isolation uses four layers: an organization scope required by every repository method; `org_id` with composite foreign keys; PostgreSQL row-level security as defence in depth; and a two-organization test harness on every surface. | A missed filter must not leak data. Row-level security catches what a code review misses; the harness proves each surface. | Scoped queries only (one mistake leaks); schema or database per organization (migrations and pool cost grow with customers). | EMP-GOV-001, built in EMP-PLAT-001/002 |
| EDR-006 | Organization routes are path-scoped: `/v1/orgs/{org_id}/…`. The app authenticates with the existing account session plus device proof. The business web uses a separate cookie session (HttpOnly, `__Host-`, CSRF token, origin check). | The path makes the organization explicit and loggable without body parsing; the app keeps its existing proof; cookies keep tokens out of page scripts. | Organization in a header (invisible in logs and caches); body tokens in the browser (exposed to scripts). | EMP-GOV-003, company panel accepted; EMP-PLAT-004/007 implement |
| EDR-007 | Two ways to give an employee access. **(a) Invitation:** an existing Abastevo account accepts a single-use invitation. **(b) Managed access:** the organization issues a single-use activation code; the employee activates on a device, which enrols a device key and creates an account usable only in that organization. No password is ever known to a manager; recovery is a new activation code. | Many field employees have no corporate e-mail; shared or known passwords cannot be audited; device keys already exist in the product. | Manager sets a password (shared secret); login by registration number and company code (guessable). | EMP-GOV-013 |
| EDR-008 | Fixed system roles and permission keys first; custom roles as bundles of the same keys later. | A fixed matrix can be tested exhaustively while the product is piloted. | Customer-defined permissions from day one (untestable surface). | EMP-GOV-001 |
| EDR-009 | Organization operations reuse the existing protocol's identifiers, replay and acknowledgement rules, but each operation is its own transaction and field facts are kept for review instead of refused. | One conflicting record must not block the rest of a day's field work; a fact recorded in the field has evidential value even when its authority is in doubt. | Reuse the atomic personal batch as is (one conflict blocks all); last-write-wins (silent loss). | EMP-GOV-001, [10-offline-sync](10-offline-sync.md) |
| EDR-010 | Adopt the existing exact-value conventions: centavos, milli-units, metres, seconds, milli-BRL unit price, half-up rounding at display, shared vectors. | Personal and organization figures must agree; the conventions are implemented and tested in two languages. | Decimal columns (two rounding behaviours in one product). | EMP-PLAT-009 |
| EDR-011 | Consumption uses the existing full-to-full method. When its conditions are not met the result is `INSUFFICIENT_DATA`. | Dividing distance by a partial fueling is wrong; the method and vectors already exist. | Distance since last fueling over litres (misleading). | [modules/fuelings](modules/fuelings.md) |
| EDR-012 | One review lifecycle (BR-REV) and one approval engine serve trips, fuelings, expenses and work orders. | Managers learn one flow; rules such as no self-approval are enforced once. | A lifecycle per module (drift). | [modules/expenses-finance](modules/expenses-finance.md) |
| EDR-013 | One economic fact, one cost. A fueling creates exactly one expense. Payments, reimbursements, card statements and stock purchases settle or move money; they never create a second cost. | Prevents the double counting the request names. | Treat each payment as an expense (double counting). | [modules/expenses-finance](modules/expenses-finance.md) |
| EDR-014 | Private C1 receipts have separate receipt, human review and fiscal status. The [document design](modules/documents.md) describes additional processing stages. Real-receipt retention and original preservation require category-specific review. | A single "validated" flag would claim more than was checked. | One status field (false assurance). | EMP-GOV-002, C1 synthetic path accepted |
| EDR-015 | Location is recorded only inside a tracking session of an organization trip, started from the visible trip screen in a location foreground service with a persistent notification. `ACCESS_BACKGROUND_LOCATION` is not requested. | Android treats a foreground service started from a visible activity as foreground location (developer.android.com, location permissions and foreground-service restrictions, read 2026-11-10); this avoids the background-location review and matches the existing guard. | Background location (store review, weaker privacy position); always-on tracking (collects personal trips). | EMP-GOV-005, EMP-GOV-006 |
| EDR-016 | In-app navigation for trips reuses the existing CoMaps-derived engine: route computation and voice guidance on the device, offline. The server stores the route plan and the distance the device computed. | The engine, routing and guidance already exist (ADR-022); no per-request provider cost; works offline. | Google Navigation SDK (paid per destination, cannot be combined with other maps, 30-day storage limits per its service terms read 2026-11-10); Mapbox Navigation (in-vehicle use needs a separate commercial licence per its terms). | EMP-GOV-007 |
| EDR-017 | The panel shows maps with an open web map library and vector tiles hosted by the project, built from the same OpenStreetMap basis as the app's packages. Recommended: MapLibre GL JS with a single-file Brazil tile archive in the project's storage. This is a new dependency and needs the recorded review. | The native engine does not run in a browser; the public OpenStreetMap tile service forbids heavy and commercial-critical use (operations.osmfoundation.org tile policy); self-hosting matches ADR-022's rule of no upstream CDN dependency. | Google Maps JavaScript (terms forbid storing and mixing with other maps); no map in the panel (list only, kept as the fallback). | EMP-GOV-007 |
| EDR-018 | Tracks are stored as one row per uploaded batch with arrays of fixes, plus one live-position row per trip in progress and one simplified geometry per finished trip. | Far fewer rows and index entries than one row per fix; retention is a delete by trip; no new database. | One row per fix (about five to seven times the storage with indexes); a time-series extension (new dependency, no measured need). | [modules/gps-navigation](modules/gps-navigation.md) |
| EDR-019 | The company panel is a static, framework-free TypeScript site served by Caddy at `/empresas/` on the configured API origin. It has its own build and session. Any station portal needs separate approval. | Same-origin company requests use the existing build and security patterns. Admin sessions remain isolated. | A framework single-page app or a separate rendering path. | EMP-GOV-003, company panel accepted |
| EDR-020 | The back office extends the admin suite: staff roles `station_reviewer` and `support_agent` get their first enforced powers; review queues and support sessions are added; `ops` remains for repair. | The session, CSRF, idempotency and audit machinery exists; the role catalog already names these roles. | A separate internal tool (duplicate machinery). | EMP-GOV-004 |
| EDR-021 | A station authority may be held by an organization. The claim keeps its frozen states and ceremony; the requesting member acts for the organization; station roles are role assignments scoped to that authority. | Networks and teams need shared, revocable, auditable access that survives a person leaving. | Keep per-account grants only (authority leaves with the person). | EMP-GOV-015, EMP-CLAIM-001 |
| EDR-022 | A station-announced price is its own source with author, validity and conditions. It never feeds community consensus and never changes ranking by plan. | Keeps sources distinct and payment neutral. | Treat as a community observation with extra weight (purchased trust). | EMP-CLAIM-001 |
| EDR-023 | An agreement shares only the fields of each contracted transaction between the two organizations. | The request forbids general access to the fleet's data. | Shared workspace (leaks routes and people). | [modules/station-commerce](modules/station-commerce.md) |
| EDR-024 | Organization plans are separate from individual Premium. Entitlements are capabilities plus measurable limits, evaluated on the server as a new source beside Play, web and admin grants. Plans are sold on the web by contract or invoice; the Play build shows no price and no purchase link for them. | Google Play's payments policy lists cloud and business software among services that must use Play billing when sold in the app, has no enterprise exemption, and allows consumption-only apps (support.google.com/googleplay/android-developer/answer/9858738, read 2026-11-10). | Sell in the app (store fee and policy exposure); reuse account tiers (cannot express per-organization limits). | EMP-GOV-009 |
| EDR-025 | Data products build on the enterprise export module and are sold only for sources with recorded redistribution rights. No private organization data enters any dataset. | Source admission is already `RIGHTS_PENDING` for prices and registry; the ANP site states a no-derivatives licence for its content, and no dataset-specific licence was found (gov.br/anp open-data page, read 2026-11-10). | Sell first and clarify later (legal exposure). | EMP-GOV-016 |
| EDR-026 | The panel refreshes by polling: 15 seconds for the live map, 60 seconds for queues, on demand elsewhere. Server-sent events are considered only when measurements show polling is the bottleneck. | Tracks arrive in batches every 30 to 60 seconds, so a faster channel would show nothing sooner; polling needs no connection state on one host. | WebSocket (connection state, proxy configuration, no user-visible gain at this freshness). | [09-architecture](09-architecture.md) |
| EDR-027 | Nine phases F0–F8 and one first operational cut C1 that pulls a minimal trip, fueling and receipt forward. | A complete, reviewable flow early; later modules still specified. | Module by module (no usable flow until late). | EMP-GOV-012 |
| EDR-028 | The logbook is an operational record. It computes no working hours and produces no labour file. | Lei 13.103/2015 names "diário de bordo" among journey-control instruments and Portaria MTP 671/2021 regulates electronic time registration (planalto.gov.br and gov.br/trabalho-e-emprego, read 2026-11-10). | Offer journey control (needs its own legal validation and product). | EMP-GOV-005 |
| EDR-029 | Internal fuel stock is valued at moving weighted average cost; cost is recognised when fuel is dispensed to an asset. | Simple, deterministic, and avoids counting the purchase and the consumption. | FIFO layers (more state for little gain at this scale). | [modules/fuel-stock](modules/fuel-stock.md) |
| EDR-030 | Findings state a difference and its evidence, never fraud. Every price comparison stores the reference and method version it used. | The request's own example: a difference to review, not proven loss. | Risk scores without evidence (not reviewable). | [modules/intelligence](modules/intelligence.md) |
| EDR-031 | A managed account has no personal mode content: no personal vehicles, Premium, or community posting. It can read public data. | The organization provisioned it and can reset it; mixing personal data into it would give the employer a path to personal data. | Full account (ownership confusion at offboarding). | EMP-GOV-013 |
| EDR-032 | Notifications are in-app first (inbox plus local notifications at sync). E-mail and push are added only after a delivery decision. | No e-mail or push provider is configured today. | Build on a provider now (unapproved dependency). | EMP-GOV-014 |


| EDR-033 | Managed-account origin is immutable account metadata set at insertion; existing accounts stay personal. Membership descriptors cannot convert that origin. | Creation and OWNER eligibility must use an authoritative fact rather than a caller-selected descriptor. | Infer origin from any membership (would convert legitimate personal accounts); writable origin flag (would let managed access shed its boundary). | EMP-ORG-003, recommended choice adopted under autonomous construction authorization; [evidence](evidence/f1-org003-atomic-onboarding.md) |
| EDR-034 | Synthetic organization creation and setup activation use PANEL's dedicated fresh cookie/CSRF plane and the source-pinned synthetic terms version/hash. All new IDs are server-selected and returned in a durable acknowledgement. | UC-ORG-01/02 are PANEL flows; no new login/provider or actual legal approval is available. | Account IDs/credentials in the business DTO; silently accept unreviewed real terms. | EMP-ORG-003, same authorization; real-data/legal gates retained |
| EDR-035 | Serialize pending ownership on the account and reserve slots for every non-ended potentially effective OWNER period, including future/invited/suspended reservations. | Counting current authority alone would let scheduled grants bypass the three-pending cap. | Active-only count; unlocked per-request count (both race or postpone the bypass). | EMP-ORG-003, same authorization; concurrent PostGIS evidence |
| EDR-036 | Synthetic org.setup admission explicitly inherits identity v1's 200/hour on each organization/account/web-or-device axis. Live use still requires measured source budgets and quota-cleanup activation. | Bounds the first synthetic configuration command using an existing versioned allowance without claiming production capacity. | Unbounded activation; invent a measured production default. | EMP-ORG-003, same authorization; source decision only |
| EDR-037 | Adopt only the proposed OWNER/ADMIN `org.legal_entity.manage` row as explicit ORG-scoped coverage in org-permissions-v2. All other matrix cells, roles, scopes and restrictions retain their previous values. | The declaration task needs a real guarded entry; the v1 catalog deliberately denied this unassigned key. This is the module's recommended row, adopted under the unattended decision authorization. | Implicit OWNER inheritance; ship an unusable always-denied declaration route; grant verification or station powers. | EMP-ORG-004; B-BR-EMP-55..58; autonomous construction authorization |
| EDR-038 | Permit lower-case snake_case segments in the bounded, source-registered audit/event vocabulary so specified names such as org.legal_entity.declared are representable. Keep exact source registration, immutable events and all scope/proof checks. Retained revoked declarations may coexist with a new explicit current declaration of the same identifier, as §6 requires. | The first legal producer exposed an existing grammar mismatch; historical references need stable identities while correction can create a new record. | Invent an action alias; silently weaken registration; forbid the specified correction flow. | EMP-ORG-004; B-BR-EMP-58; autonomous construction authorization |
| EDR-039 | Unit metadata may be edited while archived to resolve restoration collisions; only active names/codes are unique. An omitted zone captures the current organization zone at the new revision. New establishment links require a current declared parent; retained historical links survive withdrawal. Archive uses a scoped source-owned SQL usage guard under the root lock; unknown referencing consumers fail closed until their owning migration installs actual counts/admission guards. | Supports the specified restoration and prospective reporting rules without inventing current operational records or silently assuming an unknown source is empty. Current consumers are immutable onboarding/command receipts, not active work. | Delete archived units; dynamically inherit zones; count all historical references as active; install a speculative usage ledger or ignore future consumers. | EMP-ORG-005; B-BR-EMP-59/60; autonomous construction authorization |
| EDR-040 | A cost-centre code correction may select a new current code, while every old normalized code remains an immutable scoped reservation of the same centre. Historical lookup resolves the stable ID; names need not be unique. Current-usage/admission guards follow the root-locking structure contract and reject unknown consumers. | UC-ORG-03 permits code changes while BR-ORG-007 and historical exports require permanent code meaning, including archived records. This preserves both requirements instead of freezing corrections or reassigning old codes. | Make all codes immutable; release a previous/archived code; remap prior exports; infer empty usage for a new source. | EMP-ORG-006; B-BR-EMP-61/62; autonomous construction authorization |
| EDR-041 | Keep employee descriptive profiles/revisions separate from identity lifecycle and access. Existing identities have an explicit pending profile until a manager supplies it. Compare trimmed case-insensitive registration across all states; warn only on normalized name plus a matching nonempty work contact and require explicit create-anyway acknowledgement. Use bounded unverified work contacts, never infer account ownership. Home-unit archival clears current nonactive links with causal immutable evidence. | IDN-002 precedes ORG-007 and must provide a real register without provisioning access or weakening last-owner/offboarding guards. The pending creator cannot be assigned an invented name. | Placeholder employee rows for teams; copy account aliases; automatic merge/account linking; conceal archived home links and silently restore them later. | EMP-IDN-002; B-BR-EMP-63/64; BUC-EMP-32; autonomous construction authorization |
| EDR-042 | Model team membership as retained link periods with at most one current employee/team link, independent of account access. Consume employee inactivation/termination and membership suspension/end atomically, retaining causal source facts and never restoring links on reactivation. Count distinct active employees across home-unit and team-unit assignments. Retain an empty team's archived unit reference, but require an active unit for new links or restoration; archived metadata may be corrected. | BR-ORG-008/009/011/012 and the consumed IDN events require history, immediate reductions and no implicit grants. Empty structural labels are not active employees. Existing pending employee profiles are honest references, not fabricated names. | Delete links; dynamically hide links then resurrect them; infer team roles; count empty teams as employees; double count one employee across teams; attribute a synthetic manager audit to an unknown actor. | EMP-ORG-007; B-BR-EMP-65/66; BUC-EMP-33; autonomous construction authorization |
| EDR-043 | Adopt the proposed org.settings.read row for OWNER, ADMIN, FLEET_MANAGER, SUPERVISOR, FINANCE, APPROVER and AUDITOR as explicit ORG coverage in org-permissions-v3. Preserve every other matrix cell, including the recorded legal-management amendment. | ORG-008 must provide the readable settings contract to the roles named by the module; the prior catalog deliberately denied unassigned keys. | Infer read inheritance from management; grant DRIVER/OPS access to the private settings document; leave the required read endpoint always denied. | EMP-ORG-008; B-BR-EMP-67/68; autonomous construction authorization |
| EDR-044 | Capture immutable typed settings at serialized server intake under the root lock. Initialize older authentic roots lazily at the actual system bootstrap time, without an unscoped migration inventory. Keep initialization/cleanup sources honest, preserve unaffected substitute slots and store pending notification facts until NOTIF-001 consumes them. Settings owns the root default zone; captured unit zones stay explicit. Store only frozen source policy parameters, with real-retention admission left to its owner. | This implements prospective revisions, safe defaults and routing without inventing past authoring, a delivery provider or authority from designation. | Device-selected policy revision; mutate old settings/records; silently update every unit; fabricate a manager for cleanup or delivered notices; accept arbitrary policy JSON. | EMP-ORG-008; B-BR-EMP-67/68; BUC-EMP-34; autonomous construction authorization |
| EDR-045 | Implement ORG-009 through the existing ADR-023 admin cookie/key plane with fresh transactional proof on new commands and replays, separate from company roles and public staff badges. Retain actual suspension periods and pending owner notices, including archived causes. Use seven-day inclusive server-received grace and an explicit owning-transaction assessment port; keep field facts under review after reactivation. | BR-ORG-029..032 and UC-ORG-08 require actual operator authority and historical intake without inventing subscription/device/notification providers. | Give OWNER/ADMIN self-suspension power; fabricate an account for an operator audit; trust device time or silently promote held work; activate billing or delivery without its owning workflow. | EMP-ORG-009; B-BR-EMP-69/70; BUC-EMP-35; resumed task authorization 2026-11-12 |





ORG-004 is executed before ORG-005 to supply actual scoped establishment
references for the optional unit link. This changes construction order only;
legal declarations remain optional for fleet operation. CPF/CNPJ validation
checks format, never tax status, representation or legal consent. The default
legal list masks CPF even for managers; explicit fresh management reveal is
separately access-audited. Synthetic-only construction does not satisfy the
legal-purpose gate for actual CPF collection.

EMP-ORG-005 uses the independent org.structure admission class, inheriting
identity v1's synthetic 200/hour on each organization/account/web-or-device
axis. Actual production measurement and quota cleanup remain required.
Structure usage errors expose four category counts as exact decimal strings;
existing error details omit the optional count field. A source refusal without
validated category counts is unavailable rather than an invented empty result.

## 2. Conflicts with existing rules

### Next-task resumption, 2026-11-12

After Claude's corrections and merge of PR202 as main/dev `6a19bcc7`, the
maintainer authorized the next bounded enterprise task and GitHub publication.
Resume EMP-ORG-009 only from that integrated baseline; preserve review fixes,
other occupied worktrees and disabled runtime gates. No new merge/deployment
or completion of all phases is implied. This supersedes the earlier stop only
for the newly requested task.

### Construction stop and review handoff, 2026-11-12

The maintainer ended the autonomous goal and limited the remaining scope to
finishing the in-progress EMP-ORG-008, publishing the existing authorized
checkpoint to PR202 and providing a cumulative summary/review prompt. Do not
start ORG-009 or any later task. Existing recommended decisions stay recorded
for review. No merge, deployment, release or external acceptance is authorized.

### Autonomous construction authorization, 2026-11-11

The maintainer authorized completion of the whole F0–F8 enterprise plan,
with atomic task commits and publication to the existing cumulative PR202
after each completed phase. Final validation includes current-head/base CI
and review triage. The occupied local dev checkout stays preserved; the
owned execution worktree continues to publish its commits to remote dev.
No PR merge, production deployment or public release is implied.

Recommended engineering and product decisions are adopted and recorded
with their owning task, rationale and alternatives for later review,
without pausing for routine choices. This authorization does not fabricate
legal review, source rights, provider credentials or external acceptance.
Where those are unavailable, construction uses the existing synthetic or
disabled path and records the remaining evidence accurately.

The Android emulator is now authorized whenever functional/native testing
requires it, superseding the earlier emulator deferral for this enterprise
batch. Emulator results do not establish physical-device, manufacturer or
production behavior. The cumulative PR body is shortened by linking to its
history archive (final blank line normalized) and canonical task evidence, preserving prior
review history while staying below GitHub's description limit.

The resolution column records accepted amendments or proposals. For a
proposal, the existing rule governs the dependent tasks until acceptance.

| ID | Existing rule and source | What the request needs | Proposed resolution | Decided in |
|---|---|---|---|---|
| CX-01 | Historical tenant exclusion in `AGENTS.md` rule 12 and ADR-004; current bounded station and personal Premium scopes under ADR-017 and `docs/backend/PREMIUM_CLOUD_PROTOCOL.md` | Organizations as tenants | ADR-025 authorizes the exception only for private Abastevo Empresas data (BR-TEN-010). AGENTS and ADR-004 record it. | EMP-GOV-001, accepted |
| CX-02 | Premium excludes employees, shared fleet access and business tenants. Public product plans defer fleets. | Fleet product with employees | The [Abastevo Empresas contract](../product/ABASTEVO_EMPRESAS_CONTRACT.md) governs the private fleet context. Individual product and community rules keep their existing scope. | EMP-GOV-001, accepted |
| CX-03 | Tier `ENTERPRISE`: "no organization, team, seat, station or cross-account right" — ADR-023, `ADMIN_TIERS_CONTRACT.md`, migration 000130; payment code refuses enterprise sale, a payer other than the account, and quantity above one | Plans that grant an organization capabilities and limits | Leave the account tier untouched. Add an organization entitlement source and a new offer family with its own contract (EDR-024). | EMP-GOV-009 |
| CX-04 | ESP excludes "Private delivery, API access, tenant isolation, personalization and billing" — `docs/product/ENTERPRISE_DATA_OFFERING.md` | Data API with keys, quotas and plans | A new contract for Abastevo Data on top of ESP, as that document itself requires. | EMP-GOV-016 |
| CX-05 | Stage D: do not implement purchase documents, business claims, management or portal until maturity and thousands of real clients, unless the maintainer explicitly requests the named scope — `docs/planning/FUTURE_EVIDENCE_BUSINESS_PLAN.md`; private profile routes always 503 — `backend/cmd/api/profile_containment.go` | Station front; receipts | This plan is the design that Stage D allows now. Implementation of the station front needs the recorded scoped override (OPT-D00). Organization receipts are private business records, not public purchase evidence; they need their own explicit decision and do not lift D1. | EMP-GOV-015, EMP-GOV-002 |
| CX-06 | "No location-history endpoint", exact GPS capped at 24 h — `docs/security/SECURITY_PRIVACY.md`, ADR-008; raw traces local — B-BR-DRV-09, B-BR-PR06; guard limits continuous location to three services — `scripts/check-mobile.sh` | Trip tracks uploaded to the organization and kept for a period | New rule set for organization tracking with its own purpose, indicator, retention and access audit (EDR-015); personal and community rules unchanged; guard script updated in the same change that adds the consumer; privacy page updated before any pilot. | EMP-GOV-005, EMP-GOV-006 |
| CX-07 | Every photo and scan copy at most 24 h — B-BR-M01, ADR-008 amendment, ADR-017 item 5; Premium excludes "receipt OCR, permanent receipt photos" | A private receipt with an organization retention policy | Synthetic C1 receipt design accepted. Real-receipt intake and retention remain subject to legal review. The existing 24-hour rule continues to apply to community photos and claim scans. | EMP-GOV-002, real-data retention pending |
| CX-08 | "No public admin HTTP endpoints in MVP" — `docs/backend/API_PLAN.md`; BUC-006; B-BR-P16. ADR-023 (`PROPOSED`) adds a narrow admin surface | Review queues, support sessions | Build only on ADR-023 once accepted; staff-role powers are enforced per endpoint with tests; nothing is added to user-facing routes. | EMP-GOV-004 |
| CX-09 | ADR-019 governs current source priority. G09 governs real production. | Starting enterprise source work and later using real data | Local F1 starts after M0 closes and the current ADR-019 construction batch is integrated through protected `main`. Real-data use requires G09, the applicable pilot gates and legal review. | EMP-GOV-012, accepted |
| CX-10 | No new service or dependency without recorded review — ADR-005, `TARGET_ARCHITECTURE.md`, AGENTS rule 11 | Web map library; PDF report generation; XML parsing; spreadsheet export | Each addition is its own reviewed decision inside the task that needs it; standard-library options are preferred (as D06 did for XLSX). | EMP-GOV-007 and the owning tasks |
| CX-11 | Grant is "not ownership of the establishment or an organization-wide tenant role"; "No `OWNER` role exists anywhere in this scope" — `STATION_PROFILE_REPRESENTATION.md`, contract freeze | Organization-held authority with teams | The authority stays station-scoped and narrow; holding it by an organization does not make it an organization-wide role (BR-AUTH-015). The freeze text is amended to name the holder kind. | EMP-CLAIM-001 |
| CX-12 | All sale channels OFF; Android purchase interface disabled; provider notifications off — `STRIPE_ASAAS_OFFER_POLICY.md` | Selling plans; notifying people | Plans stay unsold until the catalog, fiscal treatment and channel decisions are made; pilots run on administrative activation. | EMP-GOV-009, EMP-GOV-014 |

Not conflicts, but boundaries to respect: the personal vehicle ceiling of three (`VEHICLE_REGISTRATION_POLICY.md`) does not apply to organization assets, which are a different entity; the one-shift rule of the driver suite does not apply to organization trips.

## 3. Assumptions

| ID | Assumption | If wrong |
|---|---|---|
| AS-01 | First organizations run 5 to 100 vehicles and do not depend on dedicated tracking hardware. | Larger fleets need the telematics integration earlier. |
| AS-02 | Employees use Android phones at API level 26 or higher; iOS stays deferred. | An iOS client becomes a prerequisite for some customers. |
| AS-03 | One host with PostgreSQL remains enough through cut C1; capacity numbers in [09-architecture](09-architecture.md) §9 are hypotheses. | The measured triggers there decide the next step. |
| AS-04 | Managers can sign in with username and account key until e-mail delivery exists. | E-mail delivery moves into cut C1. |
| AS-05 | Storage stays S3-compatible; cost examples use Cloudflare R2 public prices read on 2026-11-10. | Recalculate with the chosen provider. |
| AS-06 | Device clocks are usually within minutes of real time. | More operations are flagged for review. |
| AS-07 | Pilot organizations accept manual entry with optional extraction. | Extraction quality becomes a priority. |
| AS-08 | Task sizes and totals are relative hypotheses for one agent cycle, not dates. | Re-estimate after the first milestone. |
| AS-09 | Pump prices are displayed with two decimals (ANP FAQ citing Resolução ANP 948/2023, read 2026-11-10); the unit-price type keeps three. | No change; three decimals cover both. |
| AS-10 | CNPJ may be alphanumeric (Receita Federal, in production since July 2026); the existing kernel type is already alphanumeric-aware. | — |
| AS-11 | Platform staff can review station claims and support requests at pilot volume. | Station front waits for staffing evidence, as the existing freeze already requires. |

## 4. Open questions

| ID | Question | Needed by | Blocking? |
|---|---|---|---|
| OQ-01 | Organization isolation for private Abastevo Empresas data | Everything | Resolved by the maintainer on 2026-11-10, recorded in ADR-025 |
| OQ-02 | Legal validation of the points in [11-privacy-security](11-privacy-security.md) §10.4 | Any pilot with real data; tracking | Yes — pilot and F4 tracking |
| OQ-03 | Where does F1 sit relative to the current ADR-019 batch? | Start of F1 | Resolved by the maintainer on 2026-11-10. Local F1 starts after M0 closes and the current ADR-019 construction batch is integrated through protected `main`. |
| OQ-04 | Private C1 receipts and retention | Documents; cut C1 receipts | Synthetic receipt scope accepted on 2026-11-10. Real receipts require legal approval by category. |
| OQ-05 | Company panel path, session and stack | Panel tasks | Company panel at `/empresas/` accepted on 2026-11-10. |
| OQ-06 | Android local partition and protection | App enterprise mode | Account, server origin and organization partition accepted on 2026-11-10. |
| OQ-07 | Scoped Stage D override for the station front | F7 | Yes — F7 only |
| OQ-08 | Redistribution rights for public source data | Data products | Yes — Data sale only |
| OQ-09 | E-mail and push delivery providers | Invitations by e-mail, external notices | No — in-app path exists |
| OQ-10 | Panel map option and whether server-side routing is needed | F5 | No — list views work without a map |
| OQ-11 | Billing provider, offer catalog, fiscal treatment and prices for organization plans | F8 | No — pilots use administrative activation |
| OQ-12 | Whether the interface should say "diário de bordo" or a neutral name such as "registro de viagem" | F2 | No — wording only |
| OQ-13 | Is a server-verified PIN needed for managed access, beyond device key and app lock? | F1 managed access | No — invitation path is in cut C1 |
| OQ-14 | Storage region for documents and tracks | Before real data | With OQ-02 |
| OQ-15 | Which companies and stations take part in discovery and pilots | Prioritisation, prices | No |
| OQ-16 | Play distribution status and target API level | Any Play release | No for sideload pilots |

## 5. What is actually blocking

The [M0 review package](evidence/f0-m0-decisions.md) records the proposals,
legal questions and sequencing. These gates apply to the corresponding work.

| Blocker | Stops | Unblocked by |
|---|---|---|
| M0 closure and protected integration of the current ADR-019 construction batch (OQ-03, CX-09) | Start of local F1 | Full plan consistency check and verified integration evidence |
| Approved retention and legal review for real receipts (OQ-04, CX-07) | Real-receipt intake and retention | EMP-GOV-002, EMP-GOV-005 |
| Legal validation (OQ-02) | Any pilot with real data; tracking | EMP-GOV-005 |
| Stage D override (OQ-07, CX-05) | Station front (F7) | EMP-GOV-015 |
| Source rights (OQ-08) | Sale of Data products (F8) | EMP-GOV-016 |

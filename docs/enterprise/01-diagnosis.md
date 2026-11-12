# Diagnosis of the repository

Status: analysis of 2026-11-10. Evidence labels are defined in [00-conventions](00-conventions.md) §3. Paths are relative to the repository root.

## 1. What was inspected, and its limits

- **Baseline:** `origin/dev` at `a97ba3b3` (2026-11-10), 11 commits ahead of `origin/main` (`e0519257`). This plan lives in a worktree created from that commit (`.worktrees/enterprise-suite-plan`, local branch `enterprise-suite-plan`, nothing committed).
- **Why not the primary checkout:** the primary checkout is a detached HEAD at `e22824db` (2026-10-13) with uncommitted Android edits. It is 1 069 commits behind `origin/dev` and lacks the map engine, driver suite, ledger, payments, admin suite and enterprise export. Planning against it would have produced a wrong diagnosis. Its files were not touched.
- **Method:** documents, migrations (000001–000163), composition roots, module sources, Android modules and scripts were read; nothing was executed against a database, device or network service. Statements about behaviour are therefore statements about source, not about a running system.
- **Not verified:** anything on the staging host; whether the app is currently published on Google Play; the content of branches other than `origin/dev`; external provider accounts. Where a fact came from an external page, the page and date are given in the document that uses it.
- **Reference proposal check:** the proposal mentions a modular Go backend (confirmed), PostgreSQL/PostGIS (confirmed), `admin-web` (confirmed on `origin/dev`: a static owner admin panel, not a customer panel), public domains (confirmed) and `stationprofile` (confirmed, with its private routes disabled).

## 2. Stack actually found

| Layer | Found | Evidence |
|---|---|---|
| Backend | Go 1.27 modular monolith; chi, pgx, sqlc, slog; API, worker, migrator and `ops` CLI from one module | `backend/cmd/{api,worker,migrate,ops}`, `backend/go.mod` |
| Database | PostgreSQL 18 + PostGIS 3.6; 163 append-only migrations with a checksum ledger and a minimum-supported floor (`000132`) | `backend/db/migrations/`, `backend/db/migrations/schema_min_supported` |
| Jobs | PostgreSQL queue with lease and fencing token; fixed one-minute retry; per-kind serial dispatchers | `backend/internal/platform/jobs/`, `backend/cmd/worker/dispatchers.go` |
| Storage | Private S3-compatible bucket; client uploads through an API ingress capability, never a storage URL | `backend/internal/modules/evidence/` |
| Android | Kotlin 2.0.21, Compose, Hilt, Room (schema 37, 46 entities), WorkManager; modules `:app`, `:domain`, `:application`, `:data`, `:maps:engine`; minSdk 26, targetSdk 35 | `settings.gradle.kts`, `gradle/libs.versions.toml` |
| Map | CoMaps-derived native engine behind an Abastevo port; offline region packages signed and distributed by the project | `maps/engine/`, ADR-022, `backend/internal/modules/mapdist/` |
| Web | Static, framework-free TypeScript sites: `landing/` (public) and `admin-web/` (owner admin at `/admin/` on the API origin) | `landing/`, `admin-web/`, `infra/caddy/` |
| Deployment | One host; staging runs on k3s (namespace `abastevo-temp`); Compose files for development and validation; Caddy | `infra/k8s/staging/`, `infra/compose.*.yml` |
| iOS | Archived; explicit resumption only | ADR-016 |

## 3. Capability matrix

Columns: capability; evidence and real path; current state; reuse; gap; impact of the expansion.

### 3.1 Identity, accounts and access

| Capability | Evidence | State | Reuse | Gap | Impact |
|---|---|---|---|---|---|
| Anonymous device identity with proof of possession | `backend/internal/modules/identity/`; signed-request headers in `identity/adapters/auth/auth.go` | `IMPLEMENTED` | Device proof on every organization write from the app | Keyed to a contributor, not to an organization | Organization operations add account session and membership on top of the proof |
| Accounts and sessions | `backend/internal/modules/account/`; opaque tokens sent as `family_id` + `access_token` in the JSON body; access 900 s, refresh 30 days | `IMPLEMENTED` | Account is the login identity of members | No header or cookie transport; private reads are POST | A web session for the business web is new (EMP-PLAT-007) |
| Login methods | Username + account key (`account/application/keyaccounts.go`); e-mail code only with `ANPFUEL_MAIL_MODE=memory`; Google/Apple can be linked to an existing account, not used to sign in | `PARTIAL` | Username + key works today for managers and employees | No production e-mail delivery (no SMTP adapter) | Invitations cannot be e-mailed until a delivery decision (EMP-GOV-014); cut C1 uses codes and links shown to the manager |
| Device binding | `account_contributor_bindings` (migration 000022): one account per contributor key; cloud devices bind account, session family, key and origin, at most three (`cloudsync/adapters/device.go`) | `IMPLEMENTED` | Device enrolment for managed access | Three-device ceiling is personal-cloud policy | Organization device enrolment is its own record |
| Roles, organizations, memberships | No table or column for tenant, organization, membership, seat or team | `PROPOSED` | — | Everything | New modules: organization, membership, access |
| Station representation roles | `stationprofile/domain/claim.go`: `administrator`, `manager`, four scopes | `PARTIAL` | Scope vocabulary for station roles | Two of four scopes have no code; routes disabled | See §3.6 |
| Staff roles | `backend/internal/modules/staff/`; catalog `contracts/admin/staff-roles-v2.json` includes `station_reviewer` and `support_agent`; roles grant nothing yet | `PARTIAL` | Reviewer and support identities for the back office | No endpoint enforces a role | Back office gives these roles their first enforced powers |
| Owner administration | ADR-023 (`PROPOSED`); `backend/internal/modules/admin/`; owner key → cookie session `__Host-abv_admin` + CSRF + origin check + `Idempotency-Key`; append-only `admin_audit_events` | `IMPLEMENTED`, mounted only when a key hash is configured | Session, CSRF, idempotency and audit patterns; admin-web shell | One owner key, no per-person identity | Customer web sessions copy the pattern, not the key |

### 3.2 Private data, sync and money

| Capability | Evidence | State | Reuse | Gap | Impact |
|---|---|---|---|---|---|
| Private command protocol | `docs/backend/PREMIUM_CLOUD_PROTOCOL.md`; `backend/internal/modules/cloudsync/`; `ledger/adapters/store.go` (operation identifier, payload hash, stored result, expected revision) | `IMPLEMENTED` on the server | Identifiers, replay and acknowledgement rules for organization operations | Owner is one account; batch is atomic; "No tenant/fleet ownership" | New organization intake with per-operation transactions ([10-offline-sync](10-offline-sync.md)) |
| Android offline queues | Separate outboxes: contributions, ledger, quality, charging; a driver outbox exists with no transport | `PARTIAL` | Retry and owner-scope patterns | No shared queue; no organization partition | One shared queue component for enterprise mode |
| Exact values | Centavos, milli-litres, metres, milli-BRL; shared Go and Kotlin vectors in `contracts/testdata/premium/` | `IMPLEMENTED` | Adopted as BR-MONEY | Allocation and reconciliation vectors do not exist | Extended vectors |
| Personal ledger | `backend/internal/modules/ledger/`: refuel, expense, charge records per account; Android `ui/ledger`; sync opt-in and off by default | `IMPLEMENTED` (sale disabled) | Field vocabulary; nothing is moved | Per account, per personal vehicle | Organization fuelings are a new module; personal records stay personal |
| Consumption and cost engine | `backend/internal/modules/metrics/` (pure); Kotlin `domain/premium/PremiumConsumption.kt`; method in `docs/product/PREMIUM_FINANCIAL_METHOD.md` | `IMPLEMENTED` | Full-to-full method and its vectors | Works on one account's ledger | Called through a port by the fueling module |
| Individual driver suite | `backend/internal/modules/driver/` (shifts, earnings, distance, costs); disabled by flag and its capability is unreachable while payments are unconfigured; no Android transport | `PARTIAL` | Time-truth and distance-basis rules (B-BR-DRV-06 to 10) | Individual, not organizational; stores no coordinates by test | Trips are a new organization concept; no reuse of shift tables |
| Personal vehicles | Android `vehicle` table is device-global, ceiling of three; server `cloud_records` dataset `vehicles` | `IMPLEMENTED` | None for fleets | No owner column on Android; ceiling | Organization assets are a separate entity; personal vehicles untouched |
| Entitlements | `backend/internal/modules/entitlement/` and `subscription/`: eight capability keys; tiers `PRO`, `CONTRIBUTOR`, `ENTERPRISE` (label only); `RequireCapability` evaluated in the caller's transaction | `IMPLEMENTED` | Evaluation pattern and admin grants | Account-scoped; `ENTERPRISE` grants no organization right | Organization entitlements are a new source |
| Payments | `backend/internal/modules/payments/`: Stripe card and Asaas Pix adapters, idempotent webhook inbox, refunds and disputes; production catalog has zero offers; all sales flags off | `PARTIAL` (maintenance only) | Provider intake, inbox and dedupe | `Offer.Validate` refuses any enterprise sale; payer must equal account and quantity must be one | Organization billing needs a new offer family and contract |

### 3.3 Stations, prices and public data

| Capability | Evidence | State | Reuse | Gap | Impact |
|---|---|---|---|---|---|
| Canonical station catalog | `backend/internal/modules/directory/`: station UUID, CNPJ identifier, location revisions, registry sources; no merge of distinct CNPJs | `IMPLEMENTED` | Station reference for fuelings, claims and agreements | No duplicate-merge operation | Claim and agreement rules must handle a later merge |
| Official ANP prices | `backend/internal/modules/official/`: revisioned imports, immutable prices | `IMPLEMENTED` | Price reference for comparisons; Data products | Commercial redistribution rights pending (`docs/product/ENTERPRISE_SOURCE_ADMISSION.md`) | Data product blocked on rights (EMP-GOV-016) |
| Community prices | `backend/internal/modules/community/`: observations, consensus projection; trust written only by audited operator actions | `IMPLEMENTED` | Price reference | — | Must stay independent of plans (BR-SRC-002) |
| Station suggestions and corrections | `directory` intake routes; hourly auto-verify | `IMPLEMENTED` | Catalog correction path for station representatives | Manual decision has no production caller | Corrections go through this governance |
| Station feedback | `backend/internal/modules/feedback/`: ratings, comments, replies, business attribution columns (migration 000040) | `IMPLEMENTED` | Official replies | Reply route disabled with the profile routes | Enabled with station authority |
| Announced or negotiated prices | — | `PROPOSED` | — | Everything | New source, new module |
| Public data export and portal | `backend/internal/modules/enterpriseexport/` and `portalgen`: deterministic public snapshot and static portal; off by default; no routes, keys or quotas | `IMPLEMENTED` (flag off) | Snapshot builder for datasets | No API, no customers | Data API is new, on top of it |

### 3.4 Evidence, documents and attachments

| Capability | Evidence | State | Reuse | Gap | Impact |
|---|---|---|---|---|---|
| Photo evidence | `backend/internal/modules/evidence/`: JPEG only, up to 3 MiB, every copy deleted within 24 hours | `IMPLEMENTED` | Upload ingress, verification job and sweep patterns | Wrong type, size and retention for receipts | Documents are a new module with its own storage prefix and policy |
| Signed PDF proof | `stationprofile/domain/proof.go`: PDF up to 5 MB, original bytes preserved; intake disabled | `PARTIAL` | Exact-byte preservation, content-type by magic bytes | Disabled | Pattern reused for documents |
| On-device OCR | `data/.../local/ocr/MlKitImagePriceOcr.kt` (ML Kit, price boards) | `IMPLEMENTED` | Optional extraction for receipts | Tuned for price boards; no receipt fields | New extraction profile; manual entry stays the base |
| Receipts, fiscal documents | Not found; the Premium contract excludes "receipt OCR, permanent receipt photos" | `PROPOSED` | — | Everything | New module; needs the owner's decision (EMP-GOV-002) |

### 3.5 Maps, location and navigation

| Capability | Evidence | State | Reuse | Gap | Impact |
|---|---|---|---|---|---|
| Offline map, search and car routing | `maps/engine/` (`MapEngine` port, `ComapsRoutePlanner`); ADR-022 | `IMPLEMENTED` | Map, route computation and distance on the device | Android only; no web equivalent | Panel needs a web map (EMP-GOV-007) |
| Turn-by-turn guidance | `app/.../ui/map/GuidanceService.kt` (location foreground service, offline voice) | `IMPLEMENTED` | In-app navigation for trips | Not aware of an organization trip | Coordination between guidance and tracking |
| Trip recording (personal) | `app/.../ui/map/TripRecordingService.kt`; saved trips are local and owner-scoped | `IMPLEMENTED` | Recording policy and service pattern | Never uploaded; personal | Organization tracking is a separate consumer with upload |
| Driver distance (personal) | `app/.../ui/driver/work/DriverGpsService.kt`: aggregates only | `IMPLEMENTED` | Lifecycle lessons (DRV-21) | Stores no coordinates | — |
| Location guards | `scripts/check-mobile.sh` forbids `ACCESS_BACKGROUND_LOCATION` and continuous location outside three named services | `IMPLEMENTED` | Keeps tracking honest | Blocks a fourth consumer | Guard updated deliberately with the tracking task |
| Geocoding | Nominatim clients on Android at one request per second | `IMPLEMENTED` | Address lookup at low volume | Public service policy excludes heavy or tracking use | No reverse geocoding of tracks through it |
| Server-side routing, web map, geofences | Not found | `PROPOSED` | — | Everything | Decisions in EMP-GOV-007 |

### 3.6 Station representation

| Capability | Evidence | State | Reuse | Gap | Impact |
|---|---|---|---|---|---|
| Public station profile | `GET /v1/stations/{station_id}/profile`; badge only with a live grant | `IMPLEMENTED` | Badge semantics | — | Unchanged |
| Claims, proofs, decisions, grants | Migrations 000036–000039; `backend/internal/modules/stationprofile/`; private routes always answer 503 (`backend/cmd/api/profile_containment.go`); review use case has no production caller; no code changes a grant's status | `PARTIAL` | Data model, declaration ceremony, frozen states | No review path, revocation, invitation or contest; grants are per account | Extension to organization-held authority; lifting the containment is a deliberate step |
| Android claim screens | `app/.../ui/stationprofile/`; no navigation reaches them | `PARTIAL` | Screens as a starting point | Unreachable | Portal is planned on the web; app screens optional |

### 3.7 Notifications, imports, administration

| Capability | Evidence | State | Reuse | Gap | Impact |
|---|---|---|---|---|---|
| Notifications | Android local channels only; server `alert_inbox` and push-token enrolment with no provider | `PARTIAL` | Inbox table pattern; local notifications | No push or e-mail delivery | In-app inbox first; external channels need a decision |
| Imports | ANP XLSX streaming parser with caps (`docs/planning/DECISIONS.md` D06) | `IMPLEMENTED` for ANP files | Bounded parsing approach | No customer file import | New pipeline |
| Operator tooling | `backend/cmd/ops` (moderation, privacy, quality, subscription); admin suite routes under `/v1/admin` | `IMPLEMENTED` | Audit and mutation helpers | No support-session concept | New |
| Audit | Module-specific ledgers and `admin_audit_events`; no audit table for non-admin actors | `PARTIAL` | Append-only trigger pattern | No organization audit | New |

## 4. Where to extend and where to separate

| Decision | Reason |
|---|---|
| **Extend** `stationprofile` for organization-held authority, contest and revocation | It already owns claims, proofs, decisions and grants; a second claim system is forbidden by the Stage D text and would duplicate identity. |
| **Extend** `entitlement`, `subscription` and `payments` with an organization source | Capability evaluation, admin grants and provider intake exist and are tested. |
| **Extend** `admin`, `staff` and `admin-web` for the back office | Session, CSRF, idempotency and audit are in place; staff roles `station_reviewer` and `support_agent` exist and need their first enforced powers. |
| **Extend** `enterpriseexport` towards datasets | It already builds deterministic public snapshots with a rights gate. |
| **Reuse through ports** the `metrics` engine, the map engine, the evidence upload pattern, the job queue, cursors and the error envelope | Pure or platform pieces with no ownership conflict. |
| **Separate** organization records from `ledger`, `driver` and `cloudsync` | Those tables are keyed by account, erased with the account and bounded by personal ceilings. Mixing owners would break both products' rules. |
| **Separate** organization documents from `evidence` | Different purpose, formats and retention; the 24-hour rule must keep holding for community photos. |
| **Separate** organization tracking from personal recording | Different owner, consent basis and retention; the personal recorders stay local. |
| **Separate** the business web from `admin-web` | Different users, sessions and blast radius; the owner admin must never share a session with customers. |
| **Do not create** a second account system | Members authenticate with the existing account; a managed access is an account with an extra constraint. |

## 5. Conflicts between existing rules and this expansion

Summarised here; each has an entry with its proposed resolution in [16-decisions](16-decisions.md) §2.

| ID | Existing rule (source) | Conflict |
|---|---|---|
| CX-01 | "no tenant model" (`AGENTS.md` rule 12; ADR-004; ADR-017; `PREMIUM_CLOUD_PROTOCOL.md`) | The organization is a tenant. |
| CX-02 | Premium, driver and mobile-expansion contracts exclude employees, shared fleets and business tenants; `OPEN_SOURCE_BUSINESS.md` and `PRODUCT_CONTRACT.md` defer fleets | The fleet product is exactly that. |
| CX-03 | Tier `ENTERPRISE` grants "no organization, team, seat, station or cross-account right" (ADR-023; migration 000130; payment code refuses enterprise sale) | Organization plans grant organization rights. |
| CX-04 | Enterprise data offering excludes organization management, API access, private delivery and billing | The Data product adds them. |
| CX-05 | Stage D hold: no new claim, management, portal or purchase-document work until maturity or an explicit named request; private profile routes contained | Station front and document module. |
| CX-06 | No location history; exact GPS capped at 24 hours; raw traces stay local; guard script limits location consumers | Organization tracks are uploaded and kept for a period. |
| CX-07 | Every photo and scan copy deleted within 24 hours; no permanent receipt photos | Receipts are kept for years. |
| CX-08 | No public admin HTTP path (API plan, BUC-006, B-BR-P16), partly superseded by the proposed ADR-023 | Back office review queues and support sessions. |
| CX-09 | Release order: real-production certification (G09) is pending; G24 was accepted by decision with evidence owed; next source priority is P34–P38 | Any organization pilot with real data. |
| CX-10 | No new service, broker, database or dependency without a recorded decision | Web map library, document parsing, PDF generation. |
| CX-11 | Representation grant is per account, "not … an organization-wide tenant role"; "No `OWNER` role" | Organization-held station authority. |
| CX-12 | All sales channels off; Android purchase interface disabled; provider notifications off | Organization billing and external notices. |

Two inconsistencies found that this plan does not create and does not resolve: `LICENSE` on `origin/dev` is the Abastevo Visible Attribution License while `AGENTS.md` still says to preserve MIT notices; and the comment in `backend/cmd/ops/main.go` saying there is no admin HTTP path is stale since the admin suite. One risk found: the app targets API level 35, and Google Play's published requirement since 31 August 2026 is level 36 for new apps and updates (developer.android.com/google/play/requirements/target-sdk, read 2026-11-10); whether this affects current distribution was not verified.

## 6. What the diagnosis changes in the plan

1. The first usable flow can sign people in today with username and account key; e-mail invitations wait for a delivery decision.
2. In-app navigation does not need a provider purchase: the engine, routing and voice guidance exist. The open decisions are the panel's web map and whether the server needs routing at all.
3. Offline sync reuses an existing, tested protocol shape instead of inventing one.
4. Money and consumption rules are adopted from what already exists, so personal and organization figures agree.
5. Station representation is an extension of a half-built, deliberately contained feature, and needs an explicit owner decision before any code.
6. Organization billing reuses provider adapters but needs a new offer family, because the current code refuses exactly what a company plan needs.
7. The Data product is blocked on source rights, not on engineering.
8. The largest prerequisite is governance: twelve recorded conflicts must be decided by the owner before the corresponding implementation starts.

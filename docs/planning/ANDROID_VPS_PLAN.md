# Android-first VPS integration plan

State: PLANNED / LOCAL_ONLY, 2026-10-05. [ADR-019](../adr/019-android-vps-integration-first.md) owns sequencing; [ADR-018](../adr/018-project-batch-delivery.md) owns rapid delivery. This batch edits plans only. No app/backend runtime change, issue creation, VPS write/deploy, fixture seeding, GitHub publication or pilot occurs here.

## Goal and current evidence

Make the existing Android journeys work against `https://teste.abastevo.com.br` before national catalog expansion. Preserve `com.anpfuel`, MIT, KMP ports, existing expert/offline functions and current brand; focus on functional journeys, not visual redesign. iOS remains archived until a new explicit request.

The supplied nearby URL is a sample read, not the base origin or a fixed user location: `/v1/stations/nearby?lat=-23.55&lon=-46.63`. Freeze payloads and bounds against [OpenAPI](../../contracts/openapi/v1.yaml); use current backend Directory UUIDs, existing account/identity proof, feedback, price and media contracts. Never invent successful endpoints or trust an unverified client field.

Inspected current main `b4754b2` and the existing VPS [smoke record](../release-evidence/p10-t09-temp-smoke.md): API/PostGIS health and empty reads passed in that earlier slice, no ANP import/contributors/prices, private media not attached. Android DI has separate `.example.invalid` constants in AuthModule, AnonymousModule, CommunityModule and ContributionModule; FeedbackModule inherits CommunityModule. Community read/feedback/capture/outbox flags must be inventoried and deliberately enabled for a staging build, not assumed ON. StationDetailSheet still has a pending-community card; P22 records CNPJ-to-UUID screen integration outstanding. Account data export has no endpoint and requires a separate documented extension before a consumer.

Read-only probe 2026-10-05 from this agent: sandbox DNS unavailable; approved network retry with Python and curl found `unable to get local issuer certificate` for health/live, health/ready and the supplied nearby URL. Current responses could not be verified. This does not diagnose the VPS as down or prove its certificate invalid: distinguish runner CA/proxy trust and server chain in P34-T01. TLS verification was not disabled. Earlier smoke evidence remains historical, not today's live proof.

## Execution order

**Now: P34 → P35 → P36 → P37 → P38 code-ready → P25 → P26 → P27 → P29 code-ready → P30 → P31 → P32 → P33 code-ready → end Android manual/device acceptance → final cumulative PR/CI/merge/wiki → P09/G09 real certification → P10 public pilot.** P28 sources and P11 paid benefits remain optional. Exact source prerequisites consume tested local checkpoints under ADR-018; no deployment/release dependency is waived. Backend contract/risk tests precede each affected app consumer, even when the surrounding phase focuses on Android.

All phases remain PLANNED, not runtime-complete. The end manual/device batch remains OWED until performed. A P38/P29/P33 code checkpoint records only completed source acceptance and owed live/device obligations; their full acceptance gates cannot be labeled passed without the required results. Known local failures block dependent code. Unavailable live provider/storage access blocks the affected live flow, not unrelated source tasks.

## P34 — Staging connection and a useful test catalog

Entry: integrated current backend/Android source baseline and existing staging origin. Branch `codex/phase-34-vps-connection`. Code exit: deterministic environment/URL/proof selection and fixture strategy; live gate separately requires verified HTTPS and deployed contract/data availability.

- **P34-T01:** inventory all clients/flags, deployed API source/version, headers and actual endpoint shapes. Verify `/health/live`, `/health/ready`, `/v1/stations`, supplied nearby read, invalid lat and unknown UUID without collecting user GPS. Resolve trust with supported CA/server chain; no `-k`, trust-all client or HTTP fallback. A 200 empty page is valid; record missing data explicitly. No fake uptime/deployment revision inferred from a probe.
- **P34-T02:** introduce one explicit public origin/environment configuration shared by auth, anonymous identity, community, feedback and contribution DI. Staging selects `https://teste.abastevo.com.br`; release target remains explicit and separately certified. Keep bounded timeouts/retries, token/key isolation across environment changes and deliberate feature flags. Tests cover relative route joining, no double `/v1`, malformed/non-HTTPS origin refusal, stale session/environment switch, existing rollback flags and no secret/GPS logs. Compile affected modules and APK; no device run during construction.
- **P34-T03:** specify a small deterministic test catalog and source/community cases to exercise the app without national ingestion. When seeding is authorized, use existing validated import/operator mechanisms or narrowly justified test tooling, stable fixture ownership markers and only synthetic test accounts/contributions. No invented admin API, private photos, global reset or alteration of another VPS app. Test repeatability and cleanup restricted to owned fixture scope. Population is not claimed by this plan.

## P35 — Explore, station detail and actual community prices

Entry: P34 source configuration and known deployed contracts; live exercise requires HTTPS/data readiness. Branch `codex/phase-35-live-discovery`.

- **P35-T01:** implement/reuse bounded server Directory list/nearby/detail ports and Android cache models with stable `station_id` UUID. Resolve old full CNPJ identities explicitly; no fabricated UUID, ambiguous station selection or silent rewriting of saved data. Check actual contract support for city/name/fuel queries; add only needed bounded backend contract changes before their consumers. Freeze migrations and prove real PostGIS plus Room upgrade/recovery when touched.
- **P35-T02:** wire Explore/selected station to live community and dated official-price reads. Display fuel/unit/condition/source/time, explicit empty/disputed/stale/error states and last-known cache; community price is primary, ANP is dated reference. Server stations without price remain browsable. Do not present stale ANP rows as fresh community observations.
- **P35-T03:** correct existing search-emission failure using deterministic first-query/restart/cancellation/outage cases, not weakened timeout assertions. Test city/manual lookup with GPS denied, bounded nearby lookup with explicit permission, pagination/cache refresh and saved legacy functions. Nearby GPS remains transient; authenticated proximity claims still require P16 integrity proof. Live probes are limited and never a load campaign on official/provider services.

## P36 — Free accounts and station/fuel social participation

Entry: P35 canonical targets and existing account/feedback contracts. Branch `codex/phase-36-live-community`.

- **P36-T01:** connect email-code, Google/Apple login completion, account/session/key binding, refresh/revocation/deletion and safe failure presentation to the staging origin. Verify supported provider delivery/client IDs/redirects and certificate/signature configuration under local secret ownership; provider credentials are not added to Git. No paid registration gate. Deterministic denial/replay/session tests run immediately; actual provider acceptance stays explicitly owed when access/device prerequisites are missing.
- **P36-T02:** wire ratings, comments/replies, valid/invalid votes and reports into StationDetail/Comunidade with canonical UUID + fuel. Reuse 1–5 rating rules and 280 Unicode scalar limit, author ownership, moderation and rate limits. Present vote agreement separately from price confidence. Signed-in writes only; guest reads remain usable. No manual station-ID textbox as the normal journey, no new trust bonus or moderator powers.
- **P36-T03:** complete own activity/status/retry and account privacy journey. Implement export only after a scoped backend contract/privacy decision and owner-isolation tests; deletion must revoke sessions and clear appropriate local data. Test expired account, IDOR, concurrent retry/vote/report, offline replay and abuse denial with immediate backend PostGIS/race checks when affected. Operator actions remain restricted existing ports/CLI unless a measured app requirement owns a new contract.

## P37 — Capture, upload and durable contribution status

Entry: P35 targets and P36 signed account flows; private storage readiness is required for actual photo acceptance. Branch `codex/phase-37-live-contributions`.

- **P37-T01:** connect station/fuel context to lightweight camera/OCR/manual review, precise price and payment/loyalty conditions. One explicit submit, no silent publication, GPS simulation/denial/unknown follows existing integrity contracts. Keep established KiB limits/one-image processing; test huge/invalid images and bounded allocations locally before claiming low-end device performance.
- **P37-T02:** wire identity proof, upload negotiation/bytes/completion, observation/outbox, owner status/cancel, idempotent retry and process recovery. Use actual response/OpenAPI shapes; temporary network failure never becomes ACCEPTED. Fix only demonstrated backend/contract/job defects with security/concurrency/PostGIS/recovery tests before the app retries them.
- **P37-T03:** validate private storage/processing/deletion paths and all-copy 24h maximum across local cache/temp/outbox/server objects/backups/restore. The current VPS smoke has no private media attached; storage provisioning/deployment requires its recorded scope and is not done by this plan. Without it, only source/isolated storage tests pass; live photo journey is BLOCKED_MEDIA, never fabricated green. Use only synthetic images, restricted test namespace and recorded expiry outcomes.

## P38 — App regression, end validation and release handoff

Entry: P34–P37 implemented source tasks, explicit live obligations and reproducible staging configuration. Branch `codex/phase-38-app-acceptance`.

- **P38-T01:** code regression of Explore → station/community → free login → capture/review → queued/accepted status → comment/vote/report → deletion/export where implemented. Include empty city, no price, bad auth, offline/restart, revoked identity, media expiry, migration/search and server outage. Execute affected module tests/format/static/compile/APK once; retain failures, not just healthy probes.
- **P38-T02:** consolidate P24/P29/P33 device/manual/accessibility/performance/provider rows into the one end construction batch. Until all selected source phases complete, no emulator/device runs. At the end measure actual low-end/OEM/cold start/scroll/encode/OCR/memory, TalkBack/large font, novice journeys, provider callbacks and signed release behavior. Test proof and user acceptance decisions are separate fields; no blanket waiver.
- **P38-T03:** record exact source/backend build/config/fixture scope, private-storage/provider trust state and remaining launch blockers. Apply final integration only at complete project batch closure, with one required current Quick verification/reviews/guarded merge and owned wiki sync. G09 still owns real-production TLS/storage/off-host restore/load/privacy certification; the temporary staging domain does not itself release the app or authorize a public pilot.

## Later catalog and verified-business phases

[Catalog plan](STATION_CATALOG_PLAN.md): P25 registry CSV/API → P26 DOU → P27 verified suggestions and expansion of existing P35 UUID/catalog consumers → P29 capacity/freshness/affected acceptance. P27 must reuse P35/P36/P37, not rebuild discovery/social/outbox. Keep P28 source complements conditional.

[Profile plan](STATION_PROFILE_PLAN.md): P30 public profile/private claim schema → P31 identity/corporate-power review and scoped grants → P32 Android claim/status/management → P33 fraud/privacy/operations/affected acceptance. Reuse P35 station detail and P36 account/social journeys. Never ship unverified owner privileges to accelerate construction.

## Rapid delivery and authorization

One task/atomic commit at a time on a phase branch; local checkpoint pins clean tested head/evidence and next dependent branch. Immediate TDD/DDD/auth/privacy/SQL/migration/job tests stay mandatory. No per-phase PR/CI polling/wait loop/merge/wiki. Authorized branch pushes are backups; issue preparation failures record pending stable task IDs without halting unrelated code. After all selected phases, one acceptance union and final cumulative PR/CI/merge/wiki. No bookkeeping-only follow-up PRs. Keep PROGRESS concise and actual; no invented remote IDs.

This request authorizes the plan and read-only HTTPS probes, not future code execution, server writes, uploads, deployments or messages to real users. A future continuation request authorizes construction of the named source scope; continue independent local work when a live prerequisite is missing, record the exact blocked task, and preserve separately required deployment/publication scope. No credentials/private topology in docs/Git.

[Continuation prompt](CONTINUE_ANDROID_VPS_PROMPT.md) gives the bounded starting point. Start P34-T01, not a national importer or public pilot.

Scoped evidence: [planning revision validation](ANDROID_VPS_PLAN_VALIDATION.md).

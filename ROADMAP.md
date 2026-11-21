# Roadmap

Open work only. Finished work lives in git history and merged pull requests;
never record progress or completed items here. Each item says what remains and
why. An identifier in parentheses is the old task ID, kept so `git log --grep`
finds its history. Enterprise task specifications are in
[docs/enterprise/backlog/](docs/enterprise/backlog/INDEX.md).

## Release chain

Android device end-acceptance → Android commercial acceptance → first real
production release → controlled public pilot → optional paid services. The Play submission runs in parallel but needs the
owner-signed AAB, the Console declarations and the CoMaps legal sign-off.

## Backend

- **Free-account data export.** Add the account data export endpoint (access right); the contract was frozen but no route or OpenAPI entry exists (only premium/cloud export). (P36-T03)
- **Payments sandbox and test mode.** Run the gated sandbox harness with owner Stripe test-mode and Asaas sandbox credentials, then enable test mode on the VPS (real checkout off) and prove callbacks and order → access → renewal/cancel/refund. Live channels open one at a time after merchant approval and Pix Automático eligibility. (PAY-21, PAY-23)
- **Commercial inputs (owner).** Real BRL prices, the Enterprise offer and fiscal/invoice/legal terms are undecided, so every paid channel stays disabled; Google Play Billing is later. (PAY-01, PRM10)
- **Push delivery.** The alerts module enrolls tokens but no push provider or app SDK is integrated; implement per the notification content contract. (PRM11–PRM14)
- **Admin suite contract decisions, then code.** Decide and implement: account detail with roles and contributor link (code returns only account, grants, paid summary); `/contribuidores/` API origin, CSP and paging; signed-owner proof and a quota on contributor writes, gist visibility; admin keys in headers/cookies (ADM-03) and logout reason; role codes vs pt-BR labels. Then mark ADR-023 accepted.
- **Community reliability without GPS (stage B).** Media independence, abuse and price-context signals, contributor outcomes, a calibrated publication policy and its in-app explanation; never started. (OPT-B00–B08)
- **Cacheable community reads.** Conditional GET (ETag/304) or bounded public caching for the community summary and feed; the backend is still no-store while the app cache exists. (OPT-A02)
- **Economy (stage C).** Reconcile the existing alerts/history with the plan, then build the net-cost-after-detour engine, bounded route evidence (provider decision) and the app journeys. (OPT-C05–C07)
- **Fuel quality activation.** Compose the live enforcement-event source, revalidate fine ↔ station links live, then switch on quality imports on the VPS through scoped acceptance. (FQ-06, FQ-17)
- **DOU regulatory discovery.** The INLABS adapter exists but nothing calls it and there are no credentials: obtain access, reconfirm the live format, add the job and the review runbook.
- **Forum extensions.** Automated news ingestion stays blocked until a licensed source exists; follows, personalized feed, push alerts, deep links and images each need explicit scope; per-endpoint SQL/lock tracing and burst-delivery measurements are missing. (FOR-10, FOR-28)
- **EV results.** Reported-energy price ranking and complete scenario cost need place-level aggregates and provider tariffs. (EE-25, MEX-31)
- **Security candidate.** Reproduce AB-STATIC-001 (percent-encoded official-source URLs vs the allowlist) and fix only if confirmed; the anonymous-signup shared-capacity hypothesis stays inconclusive.
- **Security leftovers.** Build a real training-content minimizer before any image-training collection (RVA06); verify RVA10–RVA14 (outbox bounds, redirect/parser memory bounds, retention/proxy/quota wiring, final terms review); run the wall-clock all-copy deletion soak (SEC05-E).
- **Community-safety live evidence.** Guarded read/abuse performance runs and the consent-purge soak SLO on the current VPS. (CS07B, CS13)
- **CPF-name projection (owner).** Decide whether the stored Directory projection must also be minimized (public reads already strip CPF-shaped tokens); if yes, add the rule, migration and PostGIS tests.

## Android app

- **Device end-acceptance rerun.** Fix/triage the harness and HomeScreen failures; private OCR corpus on the device only (never Git); admitted regional map fixture; triage environment/Compose cases; owner decision on the 15 MB database budget; low-end ARM and signed-release rows; manual and novice/TalkBack journeys; live email/Google/Apple delivery and the 24-hour photo path on abastevo.com.br; P25–P33 rows. Then one complete matrix on a single revision. (P38-T02)
- **Owed device evidence for shipped features** (run with the rerun): native map reopen, two-device sync/restore on the live origin, MEX-35 performance campaign, community-first rollout (PRM21-T02), home-feed reopen, forum device/TalkBack/IME (FOR-28); run or retire the fake-GPS phone matrix (SEC08-D, ADR-021 dropped mandatory GPS).
- **Map performance and accessibility.** Performance gate (frame pacing, PostGIS load/EXPLAIN, API 26, ARMv7, physical battery, national capacity), live wire, two-device restore, TalkBack, live runbook acceptance. (MAP-29, MAP-31, MAP-32, MAP-20)
- **Map markers with Abastevo icon art.** The pinned engine cannot draw custom native icon art yet. (MAP-18)
- **National map in the app.** Read pins from the static national overlay (keep `/v1/stations/nearby` for taps) within offline/size budgets; confirm base tiles outside Paraná on a device; fix `MapRouteInstrumentedTest` and `NativeRouteScreenInstrumentedTest`.
- **Map completion residuals** (verify existing code first: a correction backend and dialog exist): street-segment pin tier, station dossier, "help locate" for ~5,300 L5 stations via the community location flow plus a published coverage list, correction auto-verification and admin review, Brazil coverage budgets, offline station points, airplane-mode device run.
- **Native navigation.** My-position modes, route from the current location, follow camera, rerouting, screen-off guidance; fix the suspected main-thread stall at TTS start-up.
- **Offline community.** Outbox for votes, comments and reports; city packs; revalidation and deletion propagation; offline media expiry; end-to-end acceptance. (CS16–CS25)
- **Driver suite (Premium, `ANPFUEL_DRIVER_ENABLED` off).** Owner decisions D1 shift checkpoints, D2 Play foreground-service location declaration, D3 session invalidation events; close DRV-18–25 (logout/Room races, permission/account changes, break-even and cost windows, Compose/a11y tests, field GPS/battery proof); DRV-26–35 (import, reports, privacy/offline/battery/security acceptance, pilot) not started.
- **Premium purchase/restore UI.** Disabled by design; wire it once the payment path is chosen. (PRM09-T02)
- **Fuel-quality journeys.** Enforcement/sanction screens once safe station links exist; check overlap with the existing occurrence explorer first. (FQ-14)
- **Optional: Android Auto.** Select and license-review the Car App Library; no adapter exists. (MAP-30)
- **Optional: games.** Backend ranking/leaderboard and the device/performance matrix. (GME-03, GME-05)

## Enterprise suite

Task specifications live in [docs/enterprise/backlog/](docs/enterprise/backlog/INDEX.md)
(134 indexed open tasks). Summary:

- **M4 acceptance (BLOCKED).** Native runs on the locked emulator, real-device tracking interruption matrix on three or more manufacturers, held fuelings, VAL-23(f) clock skew, `trip.start` through the suspension check, VAL-02 sweep, VAL-27 trip review panel. Tracking stays off for real employees until it passes or the owner records the accepted risk. (EMP-REL-005, FUEL-003, TRIP-006, EDR-362)
- **F5–F7 Android integration.** The open trip shows the enrolled driver's route, field-management and fueling-authorization cards after signed confirmation. Coordinates open a transient native route preview. Trip-aware guidance, arrival confirmation, the shared guidance/tracking location lease, protected offline plan use, and the cross-service revocation and organization-switch matrix remain open. (EDR-373, EDR-383, EDR-391)
- **Earlier gates never accepted.** C1 end-to-end scenario and pilot runbook/rollback, M1 access/isolation, M3 cost acceptance; each needs its gate run before a pilot with a real company. (REL-001–004)
- **Enable enterprise on the VPS (owner).** Apply the runtime role and DSN, set `ANPFUEL_ENTERPRISE_ENABLED=true`, run the capability command and a synthetic smoke; regenerate `runtime-role.sql` on every migration change; wire grant convergence into the release pipeline; schedule `identity_rate_windows` cleanup. (EDR-272, EDR-364)
- **Retained design obligations.** Per-operation authority denial must not become a whole-batch denial (L2), managed-account managers on acceptance (L5), DB defense for ACTIVE↔SUSPENDED (L6), per-reading time-zone scan (L10), synthetic intake budget measurement (L11), suspended/archived context notice (SCR-ORG-12), sign-in trigger for delivery.
- **F0 decisions open.** Admin-suite extension, tracking spike on target devices (now effectively the M4 matrix), organization billing (gates F8), fiscal-document consultation, managed-credential security review (gates M1), e-mail/push channels, stage-D activation of station claims and the station portal (the F7 commerce override itself is recorded, EDR-386), data redistribution rights. (GOV-004, GOV-006, GOV-009, GOV-011, GOV-013..016)
- **F1 remainder (12).** Managed corporate credentials (activation code, enrolment, recovery), transactional e-mail port, unit/cost-centre/team and role panels, clock-skew capture, context switch on shared devices, personal vs enterprise separation, back-office operator model.
- **F2 remainder (15).** Fleet, meter, trip and sync-review panels; tenure kinds and unit transfer; driver licence profile; personal-vehicle separation; employee/asset import pipeline and wizard.
- **F3 remainder (43).** Fueling review, supplier linkage, duplicates, consumption engine, amendments; document conference, fiscal items, on-device extraction, reconciliation; approval policies, delegation, inbox; expense categories/allocation, reimbursements, advances, payments, budgets; notification delivery; historical imports; supplier registry; support access.
- **F6 local checkpoint recorded (EMP-REL-007, EMP-REL-010, EDR-375..385; 0 open backlog tasks).** KPIs/dashboards (`IND-001..020`), price comparisons, findings detection/review/reprocessing, preventive plans/work orders/holds/expiries/components/parts/insurance, internal fuel stock (`VAL-32`), competence/cash CSV/PDF/ZIP reports and audit viewer, custom roles, consolidation grants, retention/legal holds/DSR/erasure tombstones, voluntary price publishing, panel & Android field slices, and synthetic host benchmark; M4 physical device matrix, VPS activation, and production `G09` remain owed before release.
- **F7 remainder (26; the maintainer recorded the stage-D commerce override, EDR-386).** Delivered: station-announced prices, labelled promotions, fleet agreements with private negotiated terms, bounded single-use fueling authorizations, once-only station redemption and the cross-organization sharing guards (EMP-STN-002..004, EMP-AGREE-001..006, EMP-AGREE-009, EDR-386..391). Open: station claims and contests, commercial profile revisions, official replies, catalog corrections, interaction metrics, station portal, ranking-neutrality proofs, statement reconciliation, panel/portal split, legal-entity verification, back-office review queues.
- **F8 (29, in local construction).** Organization subscriptions on Stripe/Asaas, Data API keys/datasets/metering, integrations (telematics, fuel cards, fiscal retrieval, webhooks, accounting), organization closure, M8 gate; needs billing and redistribution decisions.
- **Governance drift.** Reconcile GOV-006 with owner decision EDR-271. (GOV-006)
- **Regional Fuel Price Brief (B2B).** Validate demand (customer, price, contract) before building. (ESP-05)

## Web/landing/SEO

The organic growth and multilingual items follow
[decision 026](docs/decisions/026-multilingual-public-site.md). Their specifications are in the
[multilingual contract](docs/product/landing/organic-growth-i18n-plan.md), section 10.

- **Translation owner inputs (owner).** Name the `en` and `es` reviewers before the first locale page ships, and optionally a named editor with a bio. Until then, authorship stays "Equipe abastevo". (SEO-18)
- **Keyword map and cannibalization.** Add `primaryQuery` per landing page with a uniqueness check. Retarget the two editorial forum posts that duplicate `/guias/etanol-ou-gasolina/` and `/guias/pesquisa-anp/`. (SEO-20)
- **Head and template polish.** Add `max-image-preview:large` and `max-snippet:-1` in both generators. Use descriptive home, city and station H1s. Emit Article `citation` from the "Fontes" links. (SEO-21)
- **Feeds and IndexNow.** Atom feeds for guides and editorial posts. Submit changed URLs to IndexNow after each release smoke test, non-blocking. Bing, and ChatGPT search through Bing, otherwise wait for crawling. (SEO-22, SEO-23)
- **Multilingual landing.** A locale registry and URL vectors in `contracts/i18n/`, shared by Node and Go. Locale-aware landing build with hreflang groups, switcher, per-locale sitemaps and the reviewed-only and stale gates. Localized calculators with locale-aware number parsing. (SEO-25–SEO-27)
- **Multilingual portal.** A portalgen message catalog and `en`/`es` routes for price hubs, methodology, data and insights. Stations stay pt-BR. Per-locale indexation gates, sitemaps and link-graph checks. (SEO-28)
- **Fuel × state pages.** `/precos/{fuel}/{uf}/`, indexable only when the state has that fuel in the current survey. Add BRL per gallon on `en` pages; a US$ value waits for an admitted exchange-rate source. (SEO-29, SEO-30)
- **Editorial wave 2.** 24 pt-BR pieces at 2 per week, each with primary sources and ≥ 10 internal links to data pages and tools. (SEO-31)
- **International content.** About 20 reviewed `en`/`es` pages each: visitor and border guides, localized pillars and calculators. The tutorials follow with captures taken in the same locale. (SEO-32, SEO-33)
- **App share links and App Links.** A share action that produces canonical web URLs, `/.well-known/assetlinks.json` and verified intent filters, so shared links open the app. (SEO-34)
- **Social images, press page, store locales.** Build-time OG images per hub and guide, `/imprensa/` with a monthly data release, and `es` and Wave B Play listings. (SEO-35–SEO-37)
- **First-party aggregated stats.** Cookie-free daily counts from Caddy logs, with IP truncated and 30-day raw retention. Enabled only together with the privacy policy update. (SEO-38)
- **Portal source rights.** Portal indexing is on in staging (worker env set by hand) while registry, prices, enforcement and PMQC remain rights-pending (only fines admitted): record the owner decision or obtain ANP clarification, and make `ANPFUEL_ENTERPRISE_PORTAL_INDEXABLE` in `infra/k8s/staging/api-deployment.yaml` match the live value. (ESP-18, SEO-03/04)
- **Search engine onboarding (owner).** The Google verification TXT record is already on the apex. Still to do: confirm `sitemap.xml` and `sitemap-portal.xml` are submitted, import into Bing Webmaster, inspect sample URLs per page family, and export the first coverage and performance baseline. (SEO-16, SEO-19)
- **SEO measurement loop.** Monthly review by URL family and locale. Tune the station indexing window, prune pages that stay unindexed, and open Wave B locales only by the contract's demand rule. (SEO-17)
- **Weekly bulletin pages.** `/boletim/{date}/` from the accepted ANP survey, noindex until an editor adds a summary. (SEO-14)
- **Forum UGC indexing gate (later).** Index only moderated threads above a threshold, user links `rel="ugc nofollow"`; needs a privacy review. (SEO-13)
- **Navigation hubs.** A home entry for `/forum/`, a states grid, and shared footer hubs across the landing and the portal. (SEO-11, SEO-15, SEO-24)
- **Portal measurements.** Public capacity and browser campaign; live fine-link revalidation. (ESP-16, ESP-11)
- **Search shard size (optional).** Bound the largest search shard (São Paulo 461 KB gzip) while `portalcheck` stays strict.
- **Store-published switch.** When the Play listing goes live, change storeState/storeUrl, badge, MobileApplication JSON-LD, sameAs, sitemap and copy together, and recheck Play legal destinations. (D-L08)

## Infrastructure/CD

- **Deploy agent acceptance on the VPS.** Broken-image automatic rollback, contract-floor refusal and landing rollback drills; three measured zero-unavailable rollouts and a surge measurement before surge becomes default; migration rehearsal on a restored dump; confirm which zero-downtime pieces are installed. (ZD-01–ZD-07)
- **Deploy operations.** Status screen, owner failure notification, retention/GC of images, releases and backups, verified pause procedure; fix the `imagePullPolicy` drift. (ZD-08)
- **Hand-managed pieces into signed releases.** Web Caddy config, worker environment (forum, portal URLs and indexing; branch `forum-enable-staging` holds an unmerged manifest change), registry-sync tool releases, charging review timers. (EVC-08)
- **National overlay automation.** Rebuild when a new data generation or ANP week lands (today a manual `mapoverlay` run), switch atomically, keep the previous release; document the national location pipeline rerun cadence with byte-identical reruns.
- **Admin suite live checks.** After the owner sets the admin key hash, origin and allowed CIDRs: the 16-step admin script, the panel walkthrough and the app/landing contributors checks, synthetic accounts only. (A5b, B3, C4)
- **Capacity decisions (owner).** CPU quota or replicas for the nearby-stations API; optional batching of the registry publish step (164 s/day).
- **Station-prep performance (optional).** 24/48-hour soak and the final decision; streaming parse, COPY loading, durable reaping/retention, HTTP budget, per-state partitioning only as a matched trial. (RST-20, RST-21)

## Store/release

- **Play submission (verdict NOT READY).** Owner-signed AAB through Play App Signing with the new upload key (automate signed `bundleRelease` and run artifact checks on AAB-derived APKs); Console declarations (Data safety, foreground-service location videos, content rating, target audience, news/government/financial, ads, app access); developer verification and a public contact email.
- **CoMaps legal sign-off.** SRC-COMAPS is still ADMITTED_FOR_SPIKE ("do not ship an AAR/APK until accepted") while release APKs carry the engine: get legal sign-off and publish the source offer before the Play upload.
- **Play code follow-ups.** Round or proxy coordinates before Nominatim reverse geocoding; upgrade AGP to one that supports compileSdk 36 without warnings.
- **First production release (never done).** abastevo.com.br is served by the staging manifests (`infra/k8s/staging`); no production release or certification exists. Needs one immutable candidate and full release matrix; TLS, private storage, secret/dependency review; off-host encrypted backup restored into a new environment within accepted RPO/RTO with deletion-ledger replay; retention/export/erasure and legal review of the notice; load/fault/cache budgets against an accepted capacity; tested operator runbooks; production apply of the national map, overlay and portal. (P09-T03)
- **Controlled public pilot.** After the production release: choose city/cohort and capacity, rehearse rollback, watch aggregate freshness/dispute/cost metrics, write the pilot report. (P10-T09)
- **Optional paid services.** Only after the pilot, through the payments/Premium items above. (P11)
- **Stranded draft release.** Remove the asset-less draft `android-v3.1.18` left by the 2026-11-16 publish failure.

## Data sources

- **National EV charger catalog.** Run the owner-authorized import and activation on the VPS and verify app and portal; also open: CNEFE dataset licence, 30 imagery candidates to review, 11 sites without a state. (SNA-07, EVC-10)
- **Rights work before publishing derived map/EV data.** Per-asset font/style/symbol review and a reproducible regional map build; OSM ODbL derived-database offer; OSM/OCM eligibility and conflation; OCM per-provider attribution; community-data relicense review. (SRC-GEN)
- **Live charging status and tariffs.** No source exists; the optional operator adapter needs real credentials. (EE-17)
- **Optional licensed coverage.** Partner opening feeds or paid POI data only if a measured gap remains after the OSM/CNEFE/RFB pipeline. (P28)

## Deferred (start only on an explicit owner request)

- **Stage D.** Fuel-purchase receipts/NFC-e and business claim/management/portal, until the app is consolidated with thousands of real users; this also holds enterprise F7 and station-profile activation (code exists, routes contained with 503 until trust roots, revocation sources and proof storage are accepted).
- **Mandatory contribution proximity.** Only after more than 90% of stations have verified coordinates and the owner decides. (PRM22)
- **iOS.** Archived (ADR-016); on resumption: real Xcode target, native login/Keychain, camera and 24-hour expiry, Core Location, device acceptance, parity with the Android flows.

## Repository hygiene

- **EV prepared-contract test.** `scripts/tests/test-ev-prepared-contract.py` fails with `KeyError: 'provider_license_sha256'` against its sample fixture and no CI job runs it: fix the fixture or the test, then wire it into a gate.

# Enterprise data foundation and static public portal plan

Status: PLANNING_LOCAL_DONE / IMPLEMENTATION_PLANNED / LOCAL_ONLY.
Date: 2026-10-24. Task namespace: ESP-00–ESP-18.
Rules/use cases: [B-BR-ESP-01–16 / BUC-ESP-01–07](../product/ENTERPRISE_DATA_STATIC_PORTAL_CONTRACT.md).
Registry: [ROADMAP](../../ROADMAP.md#enterprise-data-and-static-public-portal).
Continuation: [bounded implementation prompt](ENTERPRISE_DATA_STATIC_PORTAL_PROMPT.md).

## Authority and actual baseline

The maintainer requested this plan after selecting a common ANP data foundation
and a static professional consultation portal. Scope is local documentation.
No source import, schedule activation, database change, dependency installation,
VPS configuration, load campaign, deployment, GitHub publication or billing
implementation follows from saving the plan. All ESP implementation tasks are
PLANNED; no runtime/capacity/production gate is accepted here.

The primary checkout was detached at `e22824db` with unrelated Android and
Evidence Store edits. Planning is isolated in `enterprise-static-plan`, based on
local dev `5c9f52046e45ffa3e470fc8ab04ca0966361fe2c`. This supersedes the older
primary checkout as the inspected planning baseline. Local dev and the cached
origin/dev ref differ by 3 local / 20 remote commits; no fetch, reconciliation or
claim of remote synchronization was made. ESP-00 must reconcile actual maintained
dev/main and owned work before implementation. Existing checkouts remain intact.

Reuse inspected baseline:

- `backend/internal/modules/official/`: source fetching, ANP parsing, staged
  price revisions and history. Current `adapters/jobs/discovery.go` probes two
  recent weeks and documents a missing-ETag correction-discovery limitation;
  audit/fix only still-existing gaps rather than copying that implementation.
- `backend/internal/modules/directory/` and `tools/station-prep/`: canonical
  identity and bounded Rust preparation. Go remains owner of canonical writes.
- `backend/internal/modules/quality/`: FQ monitoring/enforcement/fine source,
  historical publication and current-company association/read boundaries.
  [FQ plan](FUEL_QUALITY_IMPLEMENTATION_PLAN.md), current [PROGRESS](PROGRESS.md)
  and task evidence own their actual acceptance. Existing code is not proof of
  enabled schedules, deployed sources or complete national coverage.
- `landing/`: existing static HTML/CSS, plain TypeScript, fingerprinted assets
  and Node-based build/tests. Extend the existing landing; no framework/server
  rendering migration or new frontend runtime is selected.
- `infra/caddy/Caddyfile`: the inspected config proxies API/health routes and
  returns 404 otherwise. Static root/routing is new scoped hosting work, not
  an already deployed capability.

This overlay does not reorder ADR-019 app-first/safety work or accept G24/G09.
It reuses catalog/FQ/MEX owners instead of recreating their tasks. The request
names planning of this public portal and business data foundation only;
[Stage D](FUTURE_EVIDENCE_BUSINESS_PLAN.md) private evidence, organization/team
management and business participation implementation remain deferred. New
construction requires selection of the relevant ESP scope; public/VPS operations
need their concrete named operational scope at execution.

## Selected direction and alternatives

Recommended defaults, subject to evidence at ESP-00/01:

- One shared normalized foundation in the existing PostgreSQL/module ownership,
  plus original source artifacts and derived public snapshots. No separate
  enterprise database, data lake, broker, search server or always-on service.
- Existing ANP weekly files for recent prices; bounded historical resources for
  backfill. Existing admitted registry/FQ streams remain canonical. ANP API is a
  conditional alternative if it supplies a measured missing need with admitted
  access/rights/schema; never called by visitors.
- Static HTML pages plus small public search indexes split by region; optional
  compact national name/CNPJ locator only if its measured byte budget permits.
  Avoid shipping national history or enforcement detail to every browser.
- Existing landing stack and bounded Go export orchestration; reuse Rust only
  where the established source preparer owns a required format. No language
  rewrite based on unmeasured speed assumptions.
- Caddy serves files from the VPS. A CDN is the recommended growth option, but
  provider/procurement/DNS/cache behavior is an unresolved hosting decision.
- Begin with deterministic complete snapshot builds. Add incremental dependency
  rebuilds only when measured full-build time/disk cost justify the complexity.

Alternatives retained: HTML-only directory browsing is simplest but limits free
search; a whole-country browser database increases download/memory and is not
the default; per-request backend search conflicts with this public portal goal.
Search engines or analytical databases require a later recorded measured need.

## Front A — Ingestion, organization and enterprise-ready analytics

### Source register and source-specific clocks

References inspected during the planning conversation on 2026-10-24; this is not
a fresh byte/schema/license validation of every resource. ESP-01 must inspect
bounded actual files and reconcile existing FQ admission/evidence.

- **S-ESP-01 registry:** [ANP retailer dataset](https://www.gov.br/anp/pt-br/centrais-de-conteudo/dados-abertos/dados-cadastrais-dos-revendedores-varejistas-de-combustiveis-automotivos).
  Reuse RST/Directory current admitted editions and municipality resolution.
- **S-ESP-02 recent/historical prices:** [weekly research](https://www.gov.br/anp/pt-br/assuntos/precos-e-defesa-da-concorrencia/precos/levantamento-de-precos-de-combustiveis-ultimas-semanas-pesquisadas)
  and [historical files](https://www.gov.br/anp/pt-br/centrais-de-conteudo/dados-abertos/serie-historica-de-precos-de-combustiveis).
  Survey subset/collection dates are mandatory; stored coverage is measured.
- **S-ESP-03 enforcement:** [ANP actions](https://www.gov.br/anp/pt-br/centrais-de-conteudo/dados-abertos/acoes-de-fiscalizacao).
  FQ-06 owns native parsing/acceptance. Verify actual edition period, action/
  notice identifiers and publication delay; page labels are not coverage proof.
- **S-ESP-04 fines:** [ANP fine snapshots](https://www.gov.br/anp/pt-br/centrais-de-conteudo/dados-abertos/multas-aplicadas-com-vencimento-a-partir-de-2016).
  The source describes annual extraction. Reuse FQ-07 profiles, native clock
  precision, overlap/correction and association controls. A portal rebuild
  cannot make an annual legal-status snapshot current.
- **S-ESP-05 PMQC, optional portal extension:** [official monitoring](https://www.gov.br/anp/pt-br/centrais-de-conteudo/dados-abertos/pmqc-programa-de-monitoramento-da-qualidade-dos-combustiveis).
  Existing FQ owns sample/assay semantics; monitoring is not enforcement.
- **S-ESP-06 registry API, conditional:** [ANP API announcement](https://www.gov.br/anp/pt-br/centrais-de-conteudo/paineis-dinamicos-da-anp/paineis-dinamicos-do-abastecimento/painel-dinamico-de-relatorio-de-revenda).
  Do not assume uninterrupted access, complete coordinates or a contract identical
  to the CSV. No coordinate quality promotion from mere API availability.

Each admission record freezes exact resource identity, schema/parser version,
native keys, declared rights/attribution, safe public fields, source coverage,
time precision, validators, request quotas, polling/backfill window and failure
policy. Annual and monthly overlaps require source-specific reconciliation.
Paid/unknown monetary values and legal finality preserve their actual semantics.

### Pipeline and representations

1. Discover changed admitted resources with bounded periodic checks. Handle
   ETag/Last-Modified where supported; missing/unreliable validators require
   bounded content rechecks with persisted hashes, not permanent URL dedupe.
   Revisit configured past periods for corrections; old corrections outside
   that window use a documented controlled backfill.
2. Download to private temporary storage with existing origin/redirect/SSRF,
   byte/archive/parser/time bounds. A filename suffix is not a format guarantee.
3. Preserve source artifact/provenance under a reviewed per-source retention
   policy. Originals never reside under the public root or in Git. Reuse FQ
   source cleanup requirements; do not silently extend raw-data retention.
4. Normalize through existing domain owners, retain corrections and quarantine,
   resolve full establishment identity and account for every input disposition.
5. Publish normalized revisions using the owner's complete-run controls.
6. Freeze a public export manifest and derive metrics over those inputs only.
7. Record durable export intent, build candidate files and promote an accepted
   release. Lost notifications are recovered through bounded reconciliation.

Logical entities are SourceEdition, ImportRun, Station/OperatorRevision,
PriceObservation, OfficialRecord, AnalyticSnapshot and PublicRelease. These are
semantic names, not tables/endpoints claimed to exist. ESP-03 maps each to existing
contracts first; any new SQL requires append-only migration and recovery proof.

The export manifest binds source revisions/checksums, identity/association and
public-field-policy revisions, metric definitions, schema/toolchain version and
reference periods. Use bounded consistent reads or an immutable materialized
export owned by the modules; never hold an unbounded transaction over a full
national build. Identity/link invalidation must prevent exporting stale bindings.

### Analytics and business value

First outputs: survey-scoped cheapest lists; municipal/state fuel summaries;
price spread; dated price history; source coverage/freshness; safely associated
official-record counts and changes. Record denominator, missing values, ties,
geographic scope, fuel/unit/condition, period and method for every metric.

Use official published summary values with their methodology. Separately label
Abastevo calculations; never average municipal means as a national mean without
an accepted weighting method. Weekly change compares compatible observations
or reports changed sample composition; absent weeks are not zero-price weeks.
Fuel ratios require aligned periods and disclose consumption assumptions when
used for personal economy examples. No guaranteed savings or laboratory claims.

Enterprise candidates are regional price intelligence, monitored public station
reports and licensed standardized exports. ESP-05 validates one buyer/use case
and its value/cost boundaries. It does not build a tenant model, private customer
lists, fleet telemetry, automatic alerts, billing or paid-access protection.
Public portal content remains public; commercial differentiation needs a later
explicit contract for service, personalization or private delivery.

## Front B — Static consultation portal beside the landing

### Information architecture and feature slices

Proposed URL families, to freeze at ESP-08: `/consultar/`, `/postos/`,
`/precos/`, `/combustiveis/`, `/fiscalizacao/`, `/curiosidades/` and
`/metodologia/`. Preserve existing landing, guides, legal pages and verified
store state. Use a stable canonical station ID in detail routes; slugs/CNPJ/name
changes cannot break identity or silently redirect to a different operator.

- **V1 prices/catalog:** station name/CNPJ search with UF/city; station detail;
  dated ANP observations; fuel/city/state summary; survey-period cheapest lists;
  bounded history; coverage and methodology pages. Empty cities and unresearched
  stations remain honest no-data pages, without invented prices.
- **V1 official history, source-gated:** complete paginated public official
  event lists and available detail, source references, clocks, current-company
  versus proven historical scope and unresolved coverage. Do not advertise the
  slice as complete until the admitted source and all eligible records are
  reachable. Failure to admit a source blocks its slice, not unrelated prices.
- **V2 curated data questions:** price dispersion, comparable changes, counts
  by municipality/brand, source-reported action/notice/fine counts by period,
  optional accepted PMQC detail. Each page uses a frozen formula/template;
  no automatic generative article publication or numeric station-risk score.

Prefer titles such as “Lowest prices in the ANP survey, [period]” and “Most
source-reported notices in this dataset, [period]”. Preserve procedural wording;
no “worst stations”, automatic adulteration label or clean-record guarantee.
Public detail means every allowed available field, not private/raw documents.

### Search and browser boundary

Render meaningful lists/details/history summaries in HTML. JavaScript provides
accent-insensitive name/CNPJ search, sorting and bounded filters over public
static shards. Select UF/city before loading detailed regional indexes; a compact
national locator is conditional on ESP-10 measurements. Larger historical/event
detail remains in paginated files or versioned static detail payloads.

Every page pins its release/schema and shard names. A mid-session publication
cannot mix new data with old code. Bound results and downloaded bytes, cancel
obsolete work and provide useful unsupported/failed-shard fallbacks. Use a web
worker only if measured parsing/search blocks the UI. Map tiles and geocoding
are outside this initial consultation scope; reuse MAP planning if later selected.

Tests must inspect browser traffic: no Abastevo `/v1/*`, external ANP or third-party
search requests during the portal journey. Static same-origin file downloads are
allowed. CNPJ/search inputs are not placed in analytics or detailed access logs.

### Build, publishing and correction protocol

Build into a nonpublic candidate directory from a pinned manifest. Escape source
strings, sanitize links, enforce field allowlists, prohibit formula execution in
source parsing and neutralize spreadsheet formula injection in public CSV exports.
Generate deterministic HTML, JS/CSS references, JSON indexes, allowed CSV downloads,
split sitemaps and source attribution. Bound page counts and avoid generating
every possible filter combination or empty search query as an indexable URL.

Fingerprint immutable assets/data, prepare gzip/Brotli sidecars and verify all
references before publication. Upload/copy complete payloads before an atomic
release switch on the same filesystem (or an equivalently tested pointer).
A fenced publisher refuses obsolete revisions, including identity corrections.
Versioned dependencies survive the declared cache and open-session support window.
An ordinary release has rollback; withdrawn/incorrect records have an explicit
minimum safe revision and cannot be reintroduced by rolling back old assets.

Corrections enumerate affected details, search shards, rankings, downloads and
retained public releases. Rebuild or withdraw those outputs and invalidate CDN
caches if selected. If a corrected candidate cannot finish safely, withdraw the
affected slice while preserving unrelated content. Do not promise deletion of
copies already downloaded by third parties.

### VPS and CDN

Add a narrowly scoped static document root and route policy to Caddy after actual
topology inspection. Keep API routing/auth and other hosted apps untouched.
The web process receives read-only access to public artifacts; the bounded
publisher alone can promote releases. Source/temp/backup/secrets are outside the
root; directory listing is disabled. Existing `_headers`/`_redirects` files are
host-specific inputs and must be translated/tested for Caddy, not assumed active.

Use immutable caching for versioned assets and short/revalidated entry HTML.
Content ages and source dates remain visible when jobs stop. Provide a dated
static publication-status page; do not add a per-visit health API. Local liveness
of the site is separate from source freshness. Access logs retain only reviewed
aggregate operational fields under existing privacy policy.

VPS-direct is a valid baseline. CDN is conditional on provider, DNS ownership,
cache/purge/retention, traffic policy and cost review. Route only public content
through shared caches; never broaden caching for signed/private API traffic.
Reference: [Caddy precompressed file support](https://caddyserver.com/docs/caddyfile/directives/file_server).

## Sequential task backlog

Every task starts with current-head dependency/risk review, exact changed areas,
B-BR/BUC mapping and executable checks. All entries below are PLANNED; FQ/RST
implementation is reused only after its actual scoped evidence is verified.

### ESP-00 — Reconcile baseline and freeze selected scope

Dependencies: current maintained dev/main, AGENTS, current PROGRESS and occupied
worktrees. Areas: this plan, existing source/landing evidence. Rules: 01/16.
Record actual commits, outstanding FQ/source/hosting gates and the selected V1
scope. Prove no duplicate owner/task and no unintended priority/D change.
Rollback: keep the planning overlay without enabling any source.

### ESP-01 — Admit source profiles, rights and public fields

Dependency: ESP-00. Areas: source register and existing Official/RST/FQ contracts.
Rules: 02–09/16; BUC-01/05. Inspect bounded real metadata/input and retained
admission evidence; freeze fields, clocks, dedupe, coverage, retention, correction
windows and quotas. Quarantine unsupported profiles. Never infer finality.
Rollback: disable only the unaccepted source profile.

### ESP-02 — Close bounded discovery and correction gaps

Dependency: ESP-01. Areas: existing source adapters/jobs, not a second importer.
Rules: 02/09/10/15; BUC-01/02. Prove unchanged/no-ETag/replaced-file cases,
old-edition corrections, page failure, 429/backoff, duplicate jobs and restarts.
Reuse FQ schedules with their admission switches; defaults remain off until the
actual activation scope is selected. Rollback: pause new jobs, keep accepted data.

### ESP-03 — Freeze module-owned normalized export snapshots

Dependencies: ESP-01 and accepted relevant source publications. Areas: owned
Directory/Official/Quality read ports, export manifests and SQL only if required.
Rules: 01/03–05/10/11; BUC-02. Real PostGIS tests cover consistent bounded
traversal, concurrent corrections, wrong branch, unknown historical windows,
partial batches, replay and identity invalidation. Rollback: retain prior safe
snapshot; no canonical writes by the static builder.

### ESP-04 — Implement deterministic analytics

Dependency: ESP-03. Areas: pure metric functions and versioned projections.
Rules: 05–09/11; BUC-03. Golden tests cover ties, missing data, unit/period
mismatch, sample changes, source overlap and counting units. Reconcile every
ranking with its input census. Rollback: disable affected metric, retain facts.

### ESP-05 — Freeze one enterprise data offering boundary

Dependency: ESP-04. Areas: product/cost/data-license specification only.
Rules: 03/16. Define one buyer, decision supported, geography/coverage, report
example, cost measurements and permitted redistribution. Record demand evidence
or DEMAND_UNVALIDATED; no invented customers/revenue/prices. Private service/D
implementation needs separate selection. Rollback: retain public analytics.

### ESP-06 — Orchestrate bounded data-to-export intents

Dependencies: ESP-02/03/04. Areas: existing jobs and module application ports.
Rules: 10/11/13–15; BUC-02/06. Durable after-publication intent, coalescing,
single active bounded build, pending latest revision and reconciliation after
lost notification. Prove cancellation, stale leases, crash/restart, sustained
updates and independent safety-worker progress. Rollback: pause builds.

### ESP-07 — Accept the data foundation locally

Dependencies: ESP-03/04/06; ESP-05 contract recorded. Rules: 01–11/15/16.
Run scoped real-source fixture/replay/correction/restore/association census,
failure/concurrency/PostGIS tests and record exact accepted source editions.
Exit: ESP-DATA-LOCAL_READY only for selected slices; external source/runtime
acceptance remains separately owed. Rollback: retain last accepted snapshot.

### ESP-08 — Freeze portal routes, fields and editorial templates

Dependency: ESP-00/01. Areas: landing content/navigation contracts and prototypes.
Rules: 03/06–09/12/16. Approve readable mobile/desktop lists/detail/empty/stale
states, dates, attribution, canonical identity routes and methodology. Prototype
may use labelled synthetic data while data work finishes. Rollback: old landing.

### ESP-09 — Generate catalog, prices and bounded history pages

Dependencies: ESP-04/08. Areas: existing landing build and public export reader.
Rules: 03/05/06/09/11/12. Test HTML without JS, complete paginated navigation,
precision, empty periods, missing station data, links and field allowlists.
Rollback: remove generated consultation routes, preserve landing/legal pages.

### ESP-10 — Add bounded static search and filters

Dependency: ESP-09. Areas: TypeScript/search shards and existing browser checks.
Rules: 11/12/15; BUC-04. Test accents/numeric and alphanumeric CNPJ, duplicate
names, input abuse, missing shard, low-memory device and version changes with
zero API calls. Benchmark regional versus compact national locator before
selecting the latter. Rollback: HTML directory navigation remains usable.

### ESP-11 — Publish safely associated official-record pages

Dependencies: ESP-01/03/08 and actual FQ-06/07/08 acceptance for selected sources.
Rules: 04/07–09/14/16; BUC-05. Traverse every eligible public event exactly once,
preserve source detail/status precision, census unmatched records and prove
private-field exclusion/correction/wrong-operator cases. Do not reimplement FQ.
Rollback: withdraw this slice without disabling price/catalog pages.

### ESP-12 — Add deterministic insights and downloadable public data

Dependencies: ESP-04/09; official-record insights also depend on ESP-11.
Rules: 05–09/12/16. Add a small approved question catalog, formula/source links,
print view and bounded downloads with attribution and safe spreadsheet text.
Test formulas, sample-change explanations and full result census. Rollback:
remove affected insight/download while retaining base consultation.

### ESP-13 — Verify reproducible versioned publication artifacts

Dependencies: ESP-09/10 and each selected ESP-11/12 slice. Rules: 03/11/13/14.
Areas: existing build/check scripts, manifest, compression and generated sitemaps.
Test deterministic payloads, public-field allowlists, hostile source text, byte/
file-count budgets and mixed-version browser sessions. Exit: ESP-STATIC-LOCAL_READY.
Rollback: retain previous safe artifact; publication is still separate.

### ESP-14 — Implement scoped static hosting and safe promotion

Dependency: ESP-13; actual topology/permissions/CDN decision recorded. Areas:
Abastevo Caddy/volume/publisher configuration and isolated rehearsal only.
Rules: 13–16; BUC-06/07. Test complete-copy-before-switch, concurrency/fencing,
disk-full/permission/corruption failure, real 404/headers/compression, API outage
independence and withdrawal-aware rollback. No live shared edge mutation is
authorized by this plan. Rollback: restore prior safe static root/config.

### ESP-15 — Wire automatic refresh and correction recovery

Dependencies: ESP-06/14 and accepted source-specific operational scope.
Rules: 09–15; BUC-06/07. Rehearse source change → accepted revision → files →
release; prove unchanged input no-op, restart recovery, cache invalidation,
failed-build preservation and correction withdrawal. Record observed detection
and publication lag separately from ANP publication delay. Rollback: pause
activation while last-safe content remains dated or affected slice withdrawn.

### ESP-16 — Measure capacity and browser performance

Dependencies: ESP-13/14; ESP-15 for concurrent-update scenario. Rules: 12–15.
Freeze workload/hardware/limits first; run bounded isolated campaigns with
remote load generation and semantic checks, not against ANP. Report offered/
achieved rates, bytes, errors, p95, RSS/CPU/I/O/network and remaining headroom.
CDN and VPS-direct results are separate. Rollback/stop on predeclared resource
or API/safety degradation; no invented thousands-of-users acceptance.

### ESP-17 — Rehearse preview, accessibility and operations handoff

Dependencies: ESP-07/13–16 and selected official-record scope. Areas: preview,
evidence, backups/runbooks and browser checks. Verify actual HTTPS/headers/cache,
dated content/coverage, no-API navigation, JS-disabled links, keyboard/large-font
mobile experience, correction and recovery. Preview remains noindex. Record
owned paths, source gates and all unmet external obligations. Rollback: prior
safe preview. Exit: ESP-PREVIEW_ACCEPTED only on actual evidence.

### ESP-18 — Publish the selected public portal

Dependencies: ESP-17, protected delivery and explicit named publication scope.
Areas: approved Abastevo public root/CDN/metadata only. Verify candidate hash,
sources, host ownership, release switch, public smoke and recovery; then update
indexability/sitemaps. Record the exact public release and remaining source
limits. Website publication does not certify the app, API or production G09.
Rollback: last-safe public release with corrected/withdrawn records excluded.

## Capacity hypotheses, costs and acceptance strategy

16 GB RAM is target host context, not a capacity result. Before ESP-16 record
actual vCPU/CPU steal, storage/inodes, uplink/transfer allowance, TLS/CDN and
other workloads. Define independent CPU/RAM/temp/disk/concurrency/time ceilings
for importer, exporter and server; protect existing safety/API budgets. A
single bounded build is the default. Source changes coalesce without starvation.

Proposed workload to freeze or revise from a baseline: 5,000 active clients each
opening one page per minute, plus a separately defined cold-arrival burst. That
is about 83.3 page navigations/s, not automatically 83.3 HTTP requests/s: count
actual HTML/assets/shards per journey. At 200 kB/navigation, this steady example
is about 133 Mb/s before protocol overhead and cache savings. These are sizing
hypotheses, not measured traffic, host capability or promised latency.

Provisional compressed-byte budgets for the first consultation view: HTML at
most 100 KiB; portal-specific JS at most 100 KiB; one initial search shard at
most 250 KiB; history/details fetched only on demand. Measure fonts/CSS and all
transfer too; do not hide them outside the initial-view total. ESP-08/10 records
the final total/interaction budgets before implementation acceptance; changed
budgets require recorded rationale, never a post-failure threshold reduction.

Measure full-build elapsed time, per-step RSS/CPU, staging/export bytes, artifact
count/inodes, release-retention multiplier, recovery time, compressed transfer,
CDN hit/purge behavior and monthly operating-cost inputs. No fabricated price
or service availability commitment. Source-age and build/publish-lag SLOs are
source-specific and frozen at ESP-01/15 before measurement.

## Verification and delivery

Planning validation: local link/anchor/task/dependency consistency, scoped diff
and sensitive-surface inspection plus `git diff --check`. No backend/Android/
landing runtime test is claimed for a documentation-only change.

Executed 2026-10-24 on this isolated documentation patch: a one-off Python 3
checker passed 45 new/changed local links and anchors, the 19-entry task registry,
16 rules/7 use cases, an acyclic dependency map, final-newline/trailing-whitespace
and credential-pattern inspection. `git diff --check` passed. No remote checks,
source-schema revalidation, runtime tests or capacity measurements were executed.

Future task command building blocks (existing commands, selected by changed
scope after reading [backend README](../../backend/README.md),
[test strategy](../backend/TEST_STRATEGY.md) and [landing README](../../landing/README.md)):

```sh
# From backend/, with the documented disposable environment for integration:
go test -race ./internal/modules/official/...
go test -race ./internal/modules/directory/...
go test -race ./internal/modules/quality/...
go test -race -tags=integration ./internal/modules/official/...
go test -race -tags=integration ./internal/modules/quality/...
# From landing/:
npm run typecheck
npm run build
npm run check
npm test
# From repository root:
git diff --check
```

Run changed jobs/privacy/Directory integration and real PostGIS migration/rollback/
EXPLAIN cases immediately when affected, plus Rust preparer tests for actual
changes. Future export/search/browser/hosting commands must be implemented and
recorded at task opening; prose acceptance is not an executable or a passed test.
Use synthetic public fixtures for abuse/fault tests; live-source smoke is bounded
and distinct from deterministic suites. No live production load in this plan.

Follow ADR-020/018 and [DELIVERY_WORKFLOW](DELIVERY_WORKFLOW.md): reconcile onto
maintained dev, one bounded atomic task, immediate risk checks, local checkpoints
without construction CI polling, then the authorized cumulative protected dev →
main delivery with current required Quick verification. Wiki is a separate owned
merged-SHA mirror. This local plan opens no issues/PRs and publishes nothing.

Open decisions to resolve at the named task: actual source profiles/rights and
retention (01); source polling/backfill/age policy (01/02); consistent-export
mechanism (03); one enterprise buyer/value boundary (05); routes/search and byte
budgets (08/10); public legal/contact changes (08/17); correction/cache/retention
window (13/14); actual host/CDN/cost and scoped activation (14–18). None is a
reason to invent a value or mark a blocked source as accepted.

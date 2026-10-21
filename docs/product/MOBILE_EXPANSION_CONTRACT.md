# Mobile vehicles, discovery and official occurrence contract

**Selected EV extension, 2026-10-20:** [EV support rules](EV_SUPPORT_CONTRACT.md)
and [energy/cost method](EV_FINANCIAL_METHOD.md) own BEV/PHEV/hybrid capability,
charging community/catalog/tariff/status and private-session semantics. These
extend the liquid-focused rules below without reinterpreting existing money,
fuel enums, source history or commercial capability boundaries.

Date: 2026-10-20. State: PLANNED / IMPLEMENTATION_NOT_STARTED_BY_THIS_PLAN.
Owner: [execution plan](../planning/MOBILE_EXPANSION_EXECUTION_PLAN.md).
This is a requested product amendment and execution specification, not runtime,
provider, source-completeness or release evidence. Existing PRM/FQ/OPT owners stay
canonical. Proposed behavior must pass its owning task before activation.

## Authority and commercial boundary

[MEX-01 registration v2](VEHICLE_REGISTRATION_POLICY.md) freezes machine-policy,
retained-data and import/lease examples; actual enforcement remains MEX-07/08.

The maintainer explicitly selected one free vehicle and Premium for additional
vehicles. This overrides the earlier three-free-vehicles proposal for new
registrations when implemented; it does not delete existing vehicles or change
current runtime merely through documentation. The subsequent reply resolved
vehicle slots only, not a new free professional-tools offering.

Preserve the existing PRM capability matrix for ledger/report/cloud features
unless explicitly amended. Existing free vehicle configuration, tank estimates,
public reads, accounts and community participation remain free. Reuse the
existing tested three-vehicle implementation ceiling as a technical bound for
the initial multi-vehicle implementation; it is not a newly promised commercial
quota. Do not advertise unlimited vehicles or raise that ceiling without an
explicit quota/capacity amendment. Public pricing, provider setup and sales
activation remain outside this request.

## Business rules

- **B-BR-MEX-01 — Slot authority:** a new free registration admits at most one
  vehicle. Additional registrations require verified Premium capability and the
  current source ceiling. Enforce atomically at the owning persistence boundary,
  including concurrent creation/import/restore; a hidden button is insufficient.
  Do not require network for ordinary allowed free local operation. Reuse bounded
  verified offline capability behavior for paid local creation; never trust a
  client purchase flag for hosted rights.
- **B-BR-MEX-02 — Retained data:** preserve IDs, settings and histories of existing
  over-limit users and expired subscribers. No automatic deletion, hiding or
  destructive archive. Block new over-limit registrations, permit existing-data
  read/export/correction/deletion under PRM policy, and explain the limit. Existing
  free basic vehicle features do not become inaccessible. New professional work
  after expiry follows existing PRM rules. Any different transition needs an
  explicit product amendment, never a convenient migration reset.
- **B-BR-MEX-03 — Financial meaning:** distinguish user-configured estimated
  efficiency from measured full-to-full consumption. Reuse
  [financial-v1 methodology](PREMIUM_FINANCIAL_METHOD.md): exact integer money,
  milli-quantity/metres, shared Go/Kotlin vectors, correct initial-full exclusion,
  partial fills, gaps, reset segments and supported fuel/unit combinations.
  Never average ratios unweighted or combine L and m3. Show UNKNOWN with reason
  when unsupported/incomplete; entry can remain valid without a computable ratio.
- **B-BR-MEX-04 — Comparable windows:** monthly expenses, eligible distance and
  consumption coverage remain separate. Cost/km needs matching cost/distance
  windows; month boundaries use an explicit IANA timezone. Historical correction
  invalidates projections. No fuel-cost double count, inferred income/profit,
  automatic trip tracking, tax result or realized savings claim.
- **B-BR-MEX-05 — Private offline work:** durable owner/origin-scoped Room records,
  drafts and outbox precede UI success. Stable operation IDs, expected revisions,
  deletion epochs and server acknowledgements distinguish local/pending/conflict/
  synced. No financial work on the main thread. Preserve pending edits on retries,
  account changes and subscription expiry without exposing a different owner.
- **B-BR-MEX-06 — Allowed synchronization:** explicit dataset opt-in includes
  structured vehicles, eligible ledger/expenses, favorites, portable preferences
  and existing allowed rules. Exclude keys, credentials, purchase tokens, precise
  GPS, photos, OS permissions and public datasets/caches. Backup covers only
  acknowledged server revisions. Restore previews conflicts and cannot resurrect
  erased data. Reuse PRM free data exit and physical erasure obligations.
- **B-BR-MEX-07 — Local place search:** find states/cities without a network round
  trip or mandatory imported ANP week. Price availability is separate enrichment.
  Preserve IBGE identity, accent/UF/prefix handling and minSdk compatibility.
  Candidate retrieval and typo work are bounded; no per-keystroke whole-catalog
  scan, main-thread CPU scan or per-result database lookup. Obsolete requests
  cannot replace current results. Preserve unknown/no-price municipalities.
- **B-BR-MEX-08 — Typed public exploration:** free-text questions resolve only to
  an allowlisted typed query: dataset/source, fuel/unit/condition, geography,
  period/date basis, metric, sort and bounded page. Never execute user SQL,
  scripts, remote URLs or an unrestricted expression language. Ambiguous intent
  gets visible alternatives; unsupported questions are not fabricated answers.
- **B-BR-MEX-09 — Honest price rankings:** keep ANP weekly observations and
  community eligible current prices separate. Bind fuel, unit, condition, source
  edition, date and coverage to every result. Highest observation differs from
  highest average. Ties use stable order; averages retain denominator/weighting.
  No national completeness or current-offer claim from a sampled historical
  source. Excluded/missing data remains visible as coverage, not zero prices.
- **B-BR-MEX-10 — Official vocabulary:** PMQC assays, enforcement visits/actions,
  explicitly reported infractions and fine observations are separate types.
  An action is not automatically an infraction, and a fine is not automatically
  fuel adulteration. A payment field is not a finality/appeal/clearance verdict.
  Display source wording/status and UNKNOWN when absent; no invented severity,
  guilt badge, legal conclusion or automatic numeric trust score.
- **B-BR-MEX-11 — Attribution:** source-global occurrence facts are distinct from
  a link to a current company or proven historical site. Full valid CNPJ and live
  Directory guards are necessary for current-company attribution. Retain
  CURRENT_COMPANY_ONLY and historical_business_window_known=false when that is
  all the evidence supports. Do not present those facts as proven occurrences at
  today's address, operator or brand. No name/base-CNPJ/proximity matching, silent
  reassignment, business-date invention or source-global fallback for a station.
- **B-BR-MEX-12 — Clocks and case identity:** extraction, event, due, publication,
  source period and association day keep their distinct meanings and precision.
  A date filter declares its basis; a source edition is not an incident year.
  Preserve distinct results sharing a document/procedure. Correlate only keys
  documented by the source. Repeated source snapshots are not independent fines;
  where issued-case identity is absent, count labelled source records instead.
  Do not sum duplicate snapshots into a supposed national total of fines.
- **B-BR-MEX-13 — Occurrence filters:** support verified available fields for
  source/type, UF/city, source period or explicit event date, source status and
  association scope. Fuel is filterable only when explicitly attributed by the
  source; unknown cannot match a guessed product. Show counts for records,
  distinct safely associated stations/companies, unmatched records and coverage
  separately. Unknown geography/status/classification has its own meaning.
  Declare whether UF/city comes from the source occurrence or the currently
  associated Directory address; never project today's address into historical
  source geography. Unavailable source geography remains unknown.
  Do not mix non-retail regulated agents into a station-only list by guessing.
- **B-BR-MEX-14 — Negative evidence:** "No records found in the selected available
  sources/period" is not "no infractions" or "safe fuel". An unavailable source,
  unverified link or incomplete history cannot produce a clean/safe filter result.
  Use explicit available/empty/unknown/unavailable states. Payment never removes
  an official record or improves trust; no paid removal or priority verification.
- **B-BR-MEX-15 — Two public journeys:** provide a national source-record explorer,
  including unmatched evidence without fabricated station links, and a Directory
  station list/detail with safely scoped occurrences. Company-only associations
  are visibly separate from proven site history. Group station cards without
  collapsing their distinct source records. A station can have no current price
  and still be discoverable. Private reports stay out of official public counts.
- **B-BR-MEX-16 — Bounded public reads:** use existing PostgreSQL/modules/jobs and
  typed ports. No provider fetching or ingestion in a public request. Index and
  measure before introducing a materialized read model; add a projection only
  with a measured need and correction/withdrawal/expiry invalidation. No new search
  cluster, broker, generic sync service or generative AI dependency by default.
- **B-BR-MEX-17 — Snapshot/cache scope:** keysets, ETags and caches bind full filters,
  semantic policy and the relevant source-revision vector. Reverify live company
  association before scoped positive rows or a 304. Never silently ignore an
  unsupported station filter. Retired/ambiguous/unavailable bindings fail closed.
  Source-global history remains independently reachable. Respect no-store and
  never renew price/source validity through a cache hit or local save time.
  Offline company-linked results disclose their last check and inability to
  reverify the current association; do not show a current verified badge offline.
- **B-BR-MEX-18 — Coherent Android:** reuse existing navigation, Compose components,
  typography, themes, locales, Room and WorkManager. Keep one optional filter
  sheet and concise summaries instead of permanent technical labels. Retain
  manual refresh; remove fixed "every 15 seconds" copy in all affected locales.
  Reuse OPT-A refresh/persistence work and preserve loaded pagination windows.
  Master exploration is public: rankings and official-record access do not gain
  a paid gate merely because vehicle/cloud work shares this execution batch.
- **B-BR-MEX-19 — Performance evidence:** measure cold/warm startup, keystroke to
  results including debounce, cache/SQL/network components, first useful content,
  scrolling, memory and requests/session on pinned data/configuration. Freeze
  budgets before optimization; no unmeasured "instant", battery or capacity
  promise. A query-plan test for an old catalog does not validate new joins.
- **B-BR-MEX-20 — Scope holds:** no D receipts/business portal/private proof intake,
  iOS, trip/GPS tracking, income/payroll/fleet/tax service, sales activation or
  production rollout. Native/device and runtime acceptance obey their current
  explicit gates. Planning/local source success does not certify them.

## User journeys

- **BUC-MEX-01:** free first vehicle, second-vehicle Premium entry, legacy
  over-limit data, offline eligibility and subscription lapse without data loss.
- **BUC-MEX-02:** refuel full/partial/gap, expense, correction, process restart,
  supported measured consumption, estimated efficiency and unknown explanation.
- **BUC-MEX-03:** month/vehicle comparison, matched windows, timezone boundaries,
  stale report invalidation and free export of held data.
- **BUC-MEX-04:** opt-in/import preview, two-device edit conflict, retry,
  revoke/logout, backup/restore, expiry and erasure without resurrection.
- **BUC-MEX-05:** offline city/state search, typo/accent/UF/code, rapid typing,
  no imported week, unavailable prices and reselect/reopen.
- **BUC-MEX-06:** natural question or suggested query, visible interpretation,
  filtered price ranking, ties, source period and unsupported/ambiguous input.
- **BUC-MEX-07:** national official source-record explorer, filter, bounded full
  traversal, unmatched/unknown records, revision correction and source detail.
- **BUC-MEX-08:** station list with occurrences, current-company scope disclosure,
  no-price station, timeline/detail, retirement/succession and no-global-fallback.
- **BUC-MEX-09:** warm/offline public read, reconnect/manual refresh, retained page
  window, invalidated cursor/304, expired price and unavailable evidence.
- **BUC-MEX-10:** existing free/community regression, accessibility/locales,
  scoped rollback, preserved data and honest acceptance/reporting.

## References checked for planning on 2026-10-20

- [ANP fines source](https://www.gov.br/anp/pt-br/centrais-de-conteudo/dados-abertos/multas-aplicadas-com-vencimento-a-partir-de-2016).
- [ANP enforcement panel](https://www.gov.br/anp/pt-br/centrais-de-conteudo/paineis-dinamicos-da-anp/painel-dinamico-da-fiscalizacao-do-abastecimento).
- [Android offline-first](https://developer.android.com/topic/architecture/data-layer/offline-first).
- [Android Macrobenchmark](https://developer.android.com/topic/performance/benchmarking/macrobenchmark-overview).

Page access does not validate every linked source file or the deployed app.

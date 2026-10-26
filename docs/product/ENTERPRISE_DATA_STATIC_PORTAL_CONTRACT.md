# Enterprise data and static public portal contract

Status: PROPOSED / PLANNING_ONLY. Date: 2026-10-24.
Owner: [execution plan](../planning/ENTERPRISE_DATA_STATIC_PORTAL_PLAN.md).
Tasks: ESP-00–ESP-18 in [ROADMAP](../../ROADMAP.md#enterprise-data-and-static-public-portal).

The maintainer requested two connected planning fronts: organize ANP ingestion
for future business intelligence, and publish professional public queries as
static files on the VPS. This contract authorizes no implementation, source
activation, procurement, VPS mutation or public publication. Future private
business participation remains subject to [Stage D](../planning/FUTURE_EVIDENCE_BUSINESS_PLAN.md).
The named planning request does not lift that implementation hold.

## Product boundary

Visitors browse a free public data portal beside the existing product landing.
Its delivered HTML contains useful content and navigation. Optional JavaScript
searches versioned public static indexes. Visiting a page or applying a supported
filter makes no call to the Abastevo API, ANP API or database. Ordinary HTTP
requests for HTML/CSS/JS/JSON and downloads remain necessary.

The shared data foundation can later support regional intelligence, monitored
station lists, reports and licensed data delivery. Private organizations, team
roles, payment, personalized server alerts and business-management permissions
are excluded from this increment. Public files cannot protect a paid dataset.

## B-BR-ESP-01–16

- **01 — Existing ownership:** Directory owns canonical station identity;
  Official owns official price revisions; Quality/FQ owns official monitoring,
  enforcement and fine records. Reuse their accepted ports and correction rules.
  No parallel station database, quality ledger or trust calculation.
- **02 — Source admission:** Every enabled source has verified origin/format,
  schema, rights/attribution, native identity, coverage, clocks, bounded fetch
  and failure policy. A portal label or publication timestamp does not establish
  the coverage or contents of an uninspected file.
- **03 — Three representations:** Original source artifacts, normalized facts
  and derived analytics have separate ownership and versioned provenance.
  Public exports use a field allowlist; raw documents and internal artifacts
  are never automatically copied into the public document root.
  Allowlisted free text is minimized too (public field policy `esp-02`,
  2026-10-26): a station display name loses every standalone CPF-shaped token
  (11 digits or `NNN.NNN.NNN-NN`, as in MEI legal names) before export, and a
  name left empty falls back to `Posto <CNPJ>`. The portal privacy check keeps
  refusing any CPF-shaped value that still reaches an artifact.
- **04 — Exact identity and time:** Match the full establishment identity
  through Directory/FQ. Preserve historical operator context, current-company
  association and proven site history as distinct concepts. A CNPJ root,
  nearby address or present-day operator cannot inherit an unresolved old case.
- **05 — Exact prices and comparability:** Preserve milli-BRL, fuel, unit,
  collection date and known conditions. Unknown conditions remain unknown.
  Do not blend fuels, units, unrelated weeks or restricted offers. Official
  summary statistics and Abastevo-derived statistics have different labels.
- **06 — Honest price rankings:** National/local rankings mean prices found in
  the identified ANP survey subset and period, with counts, exclusions and ties.
  No claim of the cheapest current station in the entire country. A historical
  minimum is not a currently available offer; unknown data is not zero.
- **07 — Procedural meaning:** An action, notice, proceeding, fine, payment and
  final finding are separate facts. Preserve the source-reported type/status
  and as-of precision; never infer definitive guilt, adulteration or current
  interdiction from a historical fine. Missing later records prove no outcome.
- **08 — Descriptive counts:** Define unique counting units and source overlap
  before ranking. Multiple rows/updates/payments do not automatically mean
  multiple infractions. Raw counts are not a safety score or inspection-adjusted
  risk. Absence in the dataset is not a clean-record certificate.
- **09 — Independent clocks:** Expose collection/event time, source reference
  period/extraction precision, last successful source check and Abastevo
  publication time as applicable. A new build does not refresh the source age.
  A stopped scheduler cannot leave an undated impression of live information.
- **10 — Complete publication:** Fetch, parse and reconcile with bounded work.
  Failed/partial/unknown-layout imports preserve the last accepted state.
  Corrections/supersessions and explicitly missing coverage remain visible.
- **11 — Reproducible snapshot:** Export from pinned source revisions plus
  identity/association/policy versions. A build cannot traverse changing live
  views and claim a coherent snapshot. Identical inputs/toolchain produce the
  same payloads; operational build times are separate from content identity.
- **12 — Public-only static search:** Index only admitted public fields. Bound
  bytes and result counts; escape untrusted values and handle accents/CNPJ
  deterministically. No client-supplied search text executes code, SQL or HTML.
  Deep links and essential lists still work without JavaScript.
  Search v2 (2026-10-26) keeps that bound nationally without a national
  file: `indice.json` (cities with IBGE names, fuels, stopwords), 3-letter
  name-term and CNPJ prefix shards (`p/`, largest 92 KB gzip on the real
  build), one file per city (`m/`), one per fuel (`precos/`, sorted by
  exact price) and `fiscalizacao.json`; `portalcheck` decodes each family
  strictly against its key allowlist. Sort labels say "menor/maior preço",
  never "mais barato".
- **13 — Atomic release and recovery:** Publish complete immutable assets
  before switching the public release pointer. Pages reference their exact data
  version; retain dependencies for the declared cache/rollback window. Concurrent
  builders, interrupted copies and stale jobs cannot replace a newer release.
- **14 — Corrections and withdrawal:** Rebuild affected station pages, indexes,
  aggregates and downloads after source/link corrections. Apply an explicit
  withdrawal/cache-purge procedure to affected historical public artifacts;
  indefinite public retention and rollback resurrection are not acceptable.
  Record external cache limitations and residual exposure without fake erasure.
- **15 — Bounded operations:** Ingestion/build/export has CPU, memory, disk,
  request, duration and concurrency ceilings. Preserve existing safety workers
  and app/API budgets. No visitors trigger imports/builds. Measured host/network
  limits determine capacity; 16 GB RAM alone is no concurrency certification.
- **16 — Independence and scope:** Payment, sponsorship and business status
  change no trust, official facts, enforcement visibility or cheapest ordering.
  Preserve MIT notices, free account/community access and operational media
  retention. No private consumer reports/media/GPS/account identifiers enter
  this official-source public portal.

## BUC-ESP-01–07

- **01 — Admit and discover a source:** operator reviews source contract;
  scheduled worker detects a changed admitted edition. Prove unchanged data,
  missing validators, replaced bytes, redirects, rate limit and source outage.
- **02 — Normalize and publish data:** existing owners validate facts and
  identity, account every input and publish a complete version. Prove duplicate
  annual/monthly overlap, historical CNPJ, correction, partial failure and replay.
- **03 — Produce analytics:** exporter selects a frozen manifest and computes
  period/fuel/geography-scoped metrics. Prove sample changes, empty/one-record
  populations, ties, unknown values and official-versus-derived distinctions.
- **04 — Browse/search:** anonymous visitor opens a city/posto page and optionally
  loads a public search shard. Prove no API/ANP calls, JS-disabled navigation,
  low-memory behavior, malformed input and a release changing mid-session.
- **05 — Inspect official records:** visitor reads a safely linked event and its
  available public fields, original reference, status, date and coverage notes.
  Prove wrong-branch denial, unknown finality and no private field leakage.
- **06 — Publish/recover static files:** bounded builder verifies a candidate,
  publishes it and switches the release. Prove crash/duplicate/stale publisher,
  insufficient disk, pointer failure, cache consistency and rollback.
- **07 — Correct/withdraw/measure:** operator corrects attribution or source
  content, invalidates outputs and verifies caches; separately runs a frozen
  capacity scenario. Prove corrected records do not return after rollback and
  report rates/errors/hardware independently of user-count hypotheses.

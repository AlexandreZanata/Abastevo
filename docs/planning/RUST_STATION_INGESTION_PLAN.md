# Rust station preparation module: national catalog and trustworthy locations

Status: planning overlay; implementation acceptance is recorded per task in
current PROGRESS and linked evidence. Additional RST-10–21 benchmarks are PLANNED.
Research date: 2026-10-09. User requested the plan alongside the separate premium
fuel implementation. This is an execution overlay for existing P25–P29 catalog
work, after the app-first P34–P38 source checkpoints; it does not replace their
IDs, close gates, create issues, deploy, or implement a second Directory.

Extension 2026-10-09: [professional performance campaign](RUST_STATION_BENCHMARK_PLAN.md)
adds twelve bounded phases for ingestion, database construction and geographically
distributed queries. RST-00–05 now have separate audit/contract/implementation
records; this document does not replace their evidence or accept concurrent
RST-06 work. New phases do not imply benchmark execution or measured speedups.

## Outcome and scope

Build a bounded Rust CLI/library which transforms official raw station records
into validated, deterministic, versioned batches ready for insertion/update in
the existing PostgreSQL/PostGIS database. Cover every municipality provided by
the current IBGE reference, including explicit zero-observed-row cities (not proof that no physical station
exists there). National
coverage is a measurable import outcome, not a claim that every physical station
or every station coordinate is present in the sources.

Reuse Go Directory identities, reviewed location revisions, registry staging,
reconciliation and station profiles. Go remains the API/domain/publication owner.
Rust prepares assertions; it cannot award trust, ownership, active status,
precise location or official prices on its own. The first version consumes local
verified downloads from the existing fetcher, reducing network/security scope.
A Rust downloader is optional only after an evidenced need. No national crawl
or Rust runtime/database migration is executed by this plan.

## Verified sources and concrete gaps

### S01 — ANP current retailer registry: national identity/address baseline

[Official registry landing page](https://www.gov.br/anp/pt-br/centrais-de-conteudo/dados-abertos/dados-cadastrais-dos-revendedores-varejistas-de-combustiveis-automotivos)
links a CSV updated 2026-10-08. Its observed header is exactly:

```text
CODIGOISIMP;AUTORIZACAO;DATAPUBLICACAO;RAZAOSOCIAL;CNPJ;ENDERECO;COMPLEMENTO;BAIRRO;CEP;UF;MUNICIPIO;BANDEIRA;DATAVINCULACAO
```

This is a national retailer source, unlike the sampled price survey. The header
has no latitude/longitude, no IBGE municipality code and no explicit SITUACAO
column. Keep combined ENDERECO unchanged alongside parsed components; road/km
addresses require fixtures. DATAPUBLICACAO is an authorization publication date,
not an opening date; DATAVINCULACAO is not an inauguration date either.

**Existing integration gap:** `registry/policy.go` currently requires normalized
`CNPJ, RAZAO_SOCIAL, COD_IBGE, UF, SITUACAO`. Do not feed the real CSV directly into
that contract or fabricate missing columns. RST-01 must add an explicit versioned
source adapter, frozen aliases and provenance for derived municipality/status
facts. Source-page membership in an operating-retailer dataset is evidence with
a snapshot date; mapping it to authorization eligibility needs the Directory
policy owner's tested rule. Missing rows never imply closure or revocation.
[Registry metadata](https://www.gov.br/anp/pt-br/centrais-de-conteudo/dados-abertos/arquivos/arquivos-dados-cadastrais-dos-revendedores-varejistas-de-combustiveis-automotivos/metadados-revendedores-varejistas-combustiveis-automoveis.pdf)
was inspected together with the live CSV header; no national row count was
measured in this planning task.

### S02 — ANP PMQC: official coordinate candidates, incomplete coverage

[PMQC monthly CSV/JSON and metadata](https://www.gov.br/anp/pt-br/centrais-de-conteudo/dados-abertos/pmqc-programa-de-monitoramento-da-qualidade-dos-combustiveis)
include `CnpjPosto`, `DataColeta`, `Latitude` and `Longitude`. The metadata updated
2026-10-07 declares **SIRGAS 2000 / EPSG:4674** and permits empty coordinates.
It does not supply a station accuracy-in-metres field. Samples repeat across
laboratory assays; deduplicate by source sample identity/CNPJ/date/point before
using them as location evidence. Repeated assays are not independent support.

Join by validated full establishment CNPJ, then verify address/UF/municipality
and chronology. A sampled station point is a candidate, not national coverage
or proof of current metre-level precision. Official origin, many decimal places
and several identical rows do not prove a 150m capture decision is reliable.
The files named `pmqc-metadados.pdf` and `orientacoes-analises-pmqc.pdf` currently
arrived with DOCX content in this inspection: validate magic bytes/content type,
not only suffix; never execute macros/formulas or expand archives unboundedly.
[PMQC metadata](https://www.gov.br/anp/pt-br/centrais-de-conteudo/dados-abertos/arquivos/pmqc/pmqc-metadados.pdf).

### S03 — IBGE municipality identifiers and boundaries

Use the [IBGE Localidades API](https://servicodados.ibge.gov.br/api/docs/localidades)
for current codes, names and UF relationships. Never hardcode a municipality
count. Match name + UF through versioned normalization and explicit aliases;
ambiguous/unmatched cases enter quarantine. RFB municipality codes are a
separate namespace: do not equate them with IBGE codes by number.

[IBGE municipal meshes](https://www.ibge.gov.br/geociencias/todos-os-produtos-geociencias/15774-malhas.html)
provide administrative boundaries, referenced to SIRGAS 2000. Validate source
edition and CRS before point-in-polygon checks. City polygons/centroids identify
administrative area; they do not locate a station. Boundary disagreements go to
review, especially roads/borders and changed municipal divisions.

### S04 — Receita Federal, state/municipal portals and DOU: bounded enrichment

[Public CNPJ layout](https://www.gov.br/receitafederal/dados/cnpj-metadados.pdf)
can corroborate establishment identity, address and registration state. It is
not an ANP fuel-retailing authorization or a reliable coordinate source. Do not
import partner/manager names, personal contacts or unrelated company data into
the public station profile. Avoid downloading the entire national business
universe before measuring the need; filter/stream by existing station CNPJs.

State/municipal environmental licensing or geodata portals may supply parcel or
site coordinates, but availability, freshness, licence and CNPJ match must be
verified for each concrete dataset before adding an adapter. DOU acts may
corroborate authorization changes; publication date alone is not operation date.
No universal state source or scraping endpoint is assumed here.

The existing Go adapter names `revendedoresapi.anp.gov.br`; its presence in code
is not proof that a public, permitted nationwide API is currently available.
Validate documentation, access/rate limits and a bounded live sample first.
Nonofficial map/geocoder data can only be a separately attributed, licensed,
reviewed enrichment candidate; never label it ANP. Public Nominatim is not a
national batch geocoder: [usage policy](https://operations.osmfoundation.org/policies/nominatim/).

## B-BR-RST-01–10 and use cases

1. Preserve canonical station IDs and full branch CNPJ as text; validate existing
   numeric/alphanumeric checksum vectors. Nearby points, equal names or one
   address never merge different establishments automatically. Succession is an
   audited revision, not replacement of historical prices/feedback.
2. Separate source assertion, business registration, authorization, operation,
   publication eligibility, location quality and observed product availability.
   Neither registry inclusion nor Rust processing creates a price or trust.
3. Record source URL/reference, content SHA256, source key, snapshot/edition,
   parser and policy versions, fetched/published/effective/observed times; unknown
   values stay null. Use established append-only correction/supersession history.
4. Identical source + bytes + parser/policy version is a no-op. Older evidence
   cannot overwrite fresher accepted facts. Conflicting fields retain both
   assertions and a review reason; no last-row-wins shortcut.
5. A run is complete only after validated EOF/pagination, expected resource
   manifest, row accounting and completeness checks. Truncated/429/error pages,
   unexpected headers and excessive unexplained changes do not publish.
6. Every input row is accepted, duplicate or quarantined with a typed reason and
   source row locator; their counts reconcile. A syntactically valid CNPJ with
   no current official authorization can remain a candidate, never automatic
   active public station.
7. Missing/centroid/unknown-quality coordinates stay ineligible for precise
   capture. Do not turn PMQC coordinates into `reviewed` merely to satisfy the
   existing schema. Raw coordinate candidates belong in staged source evidence.
8. Shared publication goes through Directory/Official owned interfaces and a
   versioned Go loader. Rust has no general write access to canonical, community,
   auth, media or social tables. Narrow staging COPY access is a later option.
9. Server verifies current photo eligibility against reviewed station location
   and existing fix integrity/freshness/accuracy rules. Do not change the 150m
   contract, contributor privacy, photo retention or trust rules in an importer.
10. Every performance claim names dataset shape, hardware, PostgreSQL/PostGIS
    versions, configuration, cold/warm cache, concurrency and measured results.
    Rust and partitioning are hypotheses, not guaranteed speedups.

BUC-RST-01: ingest complete registry → reviewable normalized catalog batch.
BUC-RST-02: replay unchanged batch → no duplicate station/assertion/profile.
BUC-RST-03: changed source/address/authorization → dated correction retaining ID.
BUC-RST-04: join PMQC point → staged location candidate with CRS/provenance.
BUC-RST-05: search city/nearby → fast indexed source-separated results.
BUC-RST-06: crash, bad source or partial run → prior public catalog preserved.

## Architecture and data contract

```text
verified official download + manifest
  -> bounded Rust reader / source adapter
  -> pure normalization / identity + municipality validation
  -> deduplication / coordinate candidates / quarantine
  -> deterministic versioned JSONL + manifest + counts
  -> Go-owned validation + staging loader
  -> existing complete-run reconciliation
  -> Directory + profile + reviewed-location projections
```

Start a single proposed crate `tools/station-prep/` after RST-00 approval of the
actual integration boundary. Suggested modules: `source`, `parse`, `normalize`,
`identity`, `municipality`, `geo_candidate`, `output`, `manifest`, `cli`. Keep
pure transformations independent of network/DB/runtime. No Rust HTTP API,
Android JNI, new microservice, parallel station database or unrelated rewrite.

Version the batch schema before code. Manifest fields: format version, run ID,
source identity/URLs/edition, raw file hashes/bytes, parser/policy versions,
start/end time, municipality-reference hash, counts, completeness and per-output
hashes. Output rows contain stable source key, full normalized CNPJ, source
SIMP/authorization references when supplied, raw/normalized business/address
fields, UF + IBGE code, independently evidenced authorization/status facts and
optional coordinate candidate `{latitude, longitude, original_crs, observed_at,
accuracy_m: null, evidence_ref, review_state}`. Latitude/longitude retain their
original CRS until transformed by a tested geospatial owner.

Keep accepted assertions, coordinate candidates and quarantine as separate
bounded streams. Produce JSONL first for interoperability and inspection; CSV
COPY export may be added after measurement. Do not add Parquet/Arrow/Polars just
to parse a modest CSV. Use structured serialization, not SQL generated by string
concatenation. Hash canonical typed values to avoid whitespace-only churn.
Sorting/deduplication must be bounded: measured in-memory map for a documented
cap or external sorted spill/merge for larger snapshots. Reordered input must
produce the same normalized semantics and deterministic output order.

Proposed library shortlist, **not approved dependencies or versions**: `csv`
for reusable streaming byte records, `serde`/`serde_json` for typed records,
`sha2` for checksums, `encoding_rs` only if a measured encoding needs it, minimal
CLI parser. Pin exact versions/MSRV/lockfile after licence/security review and
justify each addition. Reuse the Go fetcher initially. Tokio/reqwest, parallel
CPU pools and Rust PostgreSQL clients require measured need and their own
failure/security tests. [Rust CSV reader](https://docs.rs/csv/latest/csv/struct.Reader.html),
[bounded Tokio channels](https://docs.rs/tokio/latest/tokio/sync/mpsc/fn.channel.html).

## Database design and performance decision

**Baseline:** keep one canonical PostgreSQL/PostGIS database. Existing migration
000002 already supplies a unique active identifier index, municipality B-tree
and GiST on `directory_stations.current_point`. Migrations 000031/32 provide
registry runs/assertions/coordinate staging, and 000036 profiles/operator
revisions. Inspect actual data, indexes and EXPLAIN before adding duplicates.

City list candidate: composite B-tree on the measured predicate/order, for
example `(state, municipality_code, status, id)`, possibly partial for public
eligible rows if the query predicate matches. Use prepared SQL and keyset
pagination by a stable tie-breaker; no deep OFFSET or JSON address scan per
request. Do not build broad covering indexes containing large JSONB by default.

Nearby search: keep a GiST index whose expression/type matches the query. Use
`ST_DWithin(current_point, query_geography, radius_metres)` for a bounded
geography radius, then stable exact-distance ordering on the reduced set. A
station at a city boundary may be nearby across that boundary; never force the
selected city as a proximity-security boundary. KNN approximations are discovery
optimizations, not the authoritative 150m eligibility decision. [PostGIS radius
query guidance](https://postgis.net/documentation/tips/st-dwithin/).

**Partitioning experiment:** compare (A) unpartitioned canonical catalog with
proper indexes, (B) a separate **rebuildable read projection** partitioned by UF,
and (C) a modest hash partition count on municipality only if volume warrants.
Do not create one database/schema/table per city. Avoid changing the canonical
UUID/FK/active-CNPJ uniqueness model: PostgreSQL partitioned unique keys must
include the partition key, so a naive city partition loses global identifier
uniqueness or forces incompatible composite foreign keys. A query needs usable
partition predicates for pruning; partitioning does not replace indexes.
[PostgreSQL 18 partitioning](https://www.postgresql.org/docs/18/ddl-partitioning.html).

Append-only source/history tables may benefit from monthly run/observation-date
partitions sooner than a comparatively small station catalog. Keep current
projections small. Benchmark real retention/query patterns and partition
maintenance before proposing an append-only migration. BRIN is only a candidate
for large physically time-correlated history, not a replacement for city/GiST
indexes. No production partition migration in the preparation-only MVP.

Load verified output with COPY into private staging, validate there, then apply
bounded transactions through the Go owner. Prefer unchanged-row no-ops and
short locks; keep publication boundaries honest. Existing reconciliation is
complete-run gated but processes assertions individually: it is **not evidence
of an all-stations atomic snapshot switch**. If atomic national publication is
required, add a versioned read projection + active generation pointer as a
separate tested change; do not pretend the existing reconciler guarantees it.
[PostgreSQL COPY](https://www.postgresql.org/docs/18/sql-copy.html),
[INSERT/ON CONFLICT](https://www.postgresql.org/docs/18/sql-insert.html).

No global CNPJ uniqueness shortcut, disabled constraints, relaxed durability,
dropped live indexes, concurrent-index-in-transaction or per-row HTTP lookup.
A recovery ledger tracks staged/validated/applying/applied/failed states with
lease/fencing or existing jobs; restart safely without replaying side effects.

## The 150m location acceptance gate

A national text catalog and a national camera-enabled catalog are separate
outcomes. For each coordinate candidate:

1. Validate finite ranges, lat/lon order, placeholder zeroes, source CRS and
   duplicate/centroid clusters. Preserve original SIRGAS metadata; transform
   EPSG:4674 to the canonical EPSG:4326 with a tested PostGIS/PROJ operation,
   rather than just relabelling coordinates. Record operation/version.
2. Match exact CNPJ and check address/site identity, timestamp and municipality
   boundary. Compare repeated dated candidates and independent licensed official
   site/parcel sources where available. Contradictions stay unreviewed.
3. Establish a reference point appropriate to the actual station site, not the
   legal headquarters or city centre. Large highway sites, opposite-road
   stations, two brands on one block and changed operators need explicit cases.
4. Validate a held-out stratified sample using lawful independently reviewed
   station-site references. Include all UFs where covered, large/small cities,
   rural/highway/border sites and missing/stale/conflicting coordinates. Report
   point error in metres (p50/p95/max), missing/conflict rates and coverage by
   municipality/source. No numerical accuracy claim until this measurement.
5. Only the Directory review/promotion policy may produce `reviewed` locations;
   unknown accuracy remains unknown. Conservative automation thresholds must be
   versioned and calibrated before rollout, with manual review for uncertain
   points. Official source does not automatically pass this gate.
6. Test current server distance cases 149.9m/150m/150.1m, stale/coarse/simulated
   fixes and missing/unreviewed station points on real PostGIS. Device GNSS
   accuracy is reported uncertainty, not proof of an exact physical boundary.
   A future uncertainty margin involving station and device accuracy needs a
   separate product/security contract; this plan does not silently change it.

Publish coverage labels: catalogued, coordinate candidate, reviewed precise,
camera-eligible under the current server policy. Show unknowns honestly and keep
text lookup available. Never publish or retain contributor precise GPS to build
these profiles; capture evidence remains private under existing rules.

## Small sequential tasks for an economical coding agent

Execute **one task per turn/checkpoint**. Do not load the whole repository.
Each task report includes changed files, RED/GREEN evidence, tested revision,
limits and next task. Remote publication/deployment are separate final steps.

- **RST-00 — integration audit:** read AGENTS/FAST_EXECUTION/current PROGRESS,
  this plan, existing registry policy/stage/reconcile and migrations 2/31/32/36.
  Record source adapter/header gaps, publication boundary and benchmark baseline.
  Deliver a bounded ADR proposal/decision with no implementation.
- **RST-01 — source/batch contract:** freeze synthetic real-layout fixtures for
  the 13-column registry, PMQC repeated assays/optional coordinates, IBGE mapping
  and versioned output manifest/schema. Explicitly decide operating-membership
  evidence mapping, encoding, changed-header handling and publication policy.
  No raw production rows in Git; dependency/license/MSRV decision here.
- **RST-02 — pure Rust parser:** implement local-file streaming CSV to typed
  rows, UTF-8/BOM/measured fallback, header aliases, bounds, CNPJ/municipality
  validation and typed quarantine. RED→GREEN; no network or DB yet.
- **RST-03 — determinism/deltas:** hashes, deduplication, stable output, spill
  strategy, row accounting and unchanged replay. Tests cover source reordering,
  corrected bytes, supersession, old evidence and interrupted output write.
- **RST-04 — coordinate candidates:** PMQC join/dedup, original CRS and dates,
  missing/invalid/conflicting/stale/centroid cases. Rust emits candidates only;
  no automatic reviewed flag. Shared Go/Rust fixtures for normalization.
- **RST-05 — owned Go loader:** validate manifests/checksums/schema, load bounded
  staging batches and reuse/extend existing registry ownership. Add immediate
  real-PostGIS tests for empty/upgrade/recovery, restart/concurrent duplicate,
  invalid source, retained IDs and no partial publication claim. Use a restricted
  role; credentials only through the existing operator mechanism.
- **RST-06 — location review gate:** tested CRS transform, independent reference
  sample, evidence-backed promotion/rejection and 150m/fix-integrity cases.
  Deliver measured coverage and error results, not a national accuracy promise.
- **RST-07 — benchmark/index experiment:** synthetic distribution matching the
  measured census size plus 100k/1M stress records, skewed cities and realistic
  history. Compare Go/current baseline vs Rust, index candidates and optional
  read-projection partitions; use identical semantics and output checksums.
  Keep this ID as a bounded pilot. RST-10–21 below expand the full campaign;
  RST-07 alone cannot certify national capacity or select production partitions.
- **RST-08 — incremental operations:** bounded discovery schedule using existing
  jobs, conditional downloads, complete snapshots, 429/backoff/circuit stop,
  checksums/leases/fencing, source outage, crash and recovery. If Rust networking
  is added, enforce HTTPS host/IP/redirect policy including DNS rebinding, size,
  decompression ratio, record length and time limits; test every failure now.
- **RST-09 — city/profile handoff:** source-separated profile projection through
  existing Directory/Profile ports, municipality and nearby pagination, unknown
  locations, source freshness and a staged rollout/rollback runbook. Canonical
  identity and historical facts survive removal/rebuild of the new read view.

### Additional ingestion/database benchmark phases

Full dependencies, acceptance, metric dictionary and reproducible method live in
[the performance campaign](RUST_STATION_BENCHMARK_PLAN.md). All twelve phases
below are PLANNED. Select one task at a time; protocol/corpus/harness work can
start after its own dependencies without changing the RST-06 implementation.

- **RST-10 — protocol and budgets:** freeze hardware, workload/run manifest,
  resource ceilings, stop conditions, measurement layers and provisional SLOs.
- **RST-11 — datasets and geography:** deterministic representative/100k/1M
  fixtures, history, correctness oracle, city strata and uniform/skew/hot-city
  request distributions with explicit weights and zero-row municipalities.
- **RST-12 — qualified harness:** raw samples/histograms, per-stage telemetry,
  SQL/API separation, closed/open load and error/drop/timeout accounting tests.
- **RST-13 — Rust preparation scaling:** parse/normalize/join/dedup/spill/output
  timing, rows/s, CPU, peak memory and disk, full/replay/delta equivalence.
- **RST-14 — database construction:** owned staging/application/query-ready
  timing, index/statistics build, incremental writes, WAL/locks/storage cost.
- **RST-15 — city search distribution:** stable pagination, sparse/dense/empty
  cities, prepared plans, cold/warm/rotating workloads and per-stratum tails.
- **RST-16 — spatial and eligibility cost:** GiST/radius/cross-border reads,
  distance oracle and separate authoritative 150m/fix-integrity performance.
- **RST-17 — physical design comparison:** baseline indexes versus rebuildable
  UF/hash read partitions, history candidates, pruning and maintenance costs.
- **RST-18 — mixed load and capacity:** simultaneous reads/imports, offered vs
  achieved throughput, saturation knee, backlog/freshness and safe headroom.
- **RST-19 — fault/recovery campaign:** worker/DB/disk/source failures, replay,
  duplicates, recovery time and no lost/incorrectly published data.
- **RST-20 — soak and efficiency:** bounded pilot then provisioned 24h/48h
  campaigns, maintenance/growth, deployment-class limits and measured unit costs.
- **RST-21 — decision and regression tiers:** raw evidence/exportable report,
  accepted/rejected/inconclusive options and scoped CI vs manual lab cadence.

This campaign refines existing ROADMAP P29-T01; it does not create a second
catalog acceptance gate or close P29-T02/G29/G09. Database read layout remains a
measured decision; no per-city database/table or canonical identity rewrite.

## Benchmark and acceptance manifest

The detailed acceptance manifest is [RST-10–21](RUST_STATION_BENCHMARK_PLAN.md),
including phase prerequisites, metric units/denominators, geographic traffic
mixes, error/censored-sample accounting, repetitions, uncertainty, isolated
resource budgets and decision criteria. The following original targets remain
provisional; do not report them as measured performance.

Record wall time, CPU time, peak RSS, input bytes/rows, rows/sec, disk spill,
output sizes, changed/no-op/quarantine counts, and DB WAL/lock/transaction impact.
For city/nearby reads record p50/p95/p99, planning time, EXPLAIN (ANALYZE, BUFFERS),
index size, cold/warm cache, concurrent import/read behaviour and total DB size.
Use 1/8/32 read clients and bounded imports on fixed stated hardware; compare
5/20/50-row pages, sparse/large cities, all-city and cross-border nearby queries.

Provisional laboratory targets, **not measured results or SLAs**: parser peak
RSS <=256MiB on a specified representative snapshot, bounded two concurrent
source streams, city first-page p95 <=100ms and nearby p95 <=150ms at eight
clients on the documented workstation. RST-10 must revise/freeze these targets from
real source size and target VPS limits. Partitioning needs a material measured
benefit across the workload, including planning/maintenance/uniqueness costs;
otherwise keep the simpler indexed baseline. Faster parsing cannot compensate
for wrong identities, untrusted coordinates or broken publication semantics.

Required negative fixtures: bad/leading-zero/alphanumeric CNPJ, unknown city,
accent/encoding/header changes, quoted separator/newline, extreme record size,
truncated files/pages, duplicate assays, zero/NaN/swapped/out-of-Brazil points,
unsupported CRS, date reversal, older conflicting status, hash mismatch,
concurrent loader, DB failure, disk full, worker kill and stale lease. Use
synthetic fixtures and real disposable PostGIS; live official-source smoke is
bounded and separately recorded, never a flaky unit-test dependency.

Proposed commands exist only after their targets are implemented: `cargo fmt
--check`, `cargo clippy --all-targets -- -D warnings`, `cargo test --locked`,
shared-contract checks and documented disposable PostGIS tests. Record dependency
licences, audit tooling availability and results without claiming absent checks
passed. Follow existing scoped CI/release cadence; no speculative nightly load
suite or new cron service merely because this plan names a future task.

## Handoff

Use [the bounded implementation prompt](RUST_STATION_INGESTION_PROMPT.md) to select
the next uncompleted task from verified current evidence, not the entire module.
RST-10 is the new documentation/protocol entry for the benchmark extension;
do not restart completed RST-00–05. Existing source plans/contracts remain
canonical: [catalog plan](STATION_CATALOG_PLAN.md), [catalog contract](../product/STATION_CATALOG.md),
[ANP ingestion](../backend/ANP_INGESTION.md), [test strategy](../backend/TEST_STRATEGY.md).
The original plan and this extension are documentation-only changes. Subsequent
Rust/Go implementation has its own task evidence; no benchmark run, national
ingestion, partition migration or location promotion is certified by this plan.

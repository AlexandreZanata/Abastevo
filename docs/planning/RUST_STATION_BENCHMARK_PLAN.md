# Station ingestion and database performance campaign

Status: PLANNED — phases RST-10–21; no benchmark results or SLA certification.
Date: 2026-10-09. Parent: [Rust ingestion plan](RUST_STATION_INGESTION_PLAN.md).
This extension defines how to measure preparation, database construction and
geographically distributed reads. It does not authorize load against a shared
service, deployment, a new database topology or promotion of location trust.

## Ownership, current evidence and boundaries

Keep RST-00–09 and existing P25–29 IDs. RST-07 becomes the bounded pilot; the
phases below provide the full experimental campaign for P29-T01, without
accepting G29, Android acceptance or G09. The opening source head is `e257ee8`:
RST-00/01 have documents/fixtures; RST-02–04 implement offline Rust preparation;
RST-05 implements Go-owned staging. RST-06 changes were concurrently in progress
and are not accepted by this document. Re-read current PROGRESS before execution.
Staging throughput is not canonical publication throughput: freeze the actual
RST-08/09 publication path before measuring end-to-end readiness.

Reuse `infra/scripts/load/{run.sh,stats.py,faults.sh}` and `scripts/tests/test-load.sh`
where suitable. The existing smoke uses closed-loop concurrent shell clients,
500 seeded stations/20 seconds in its test wrapper, pooled nearest-rank
percentiles and a 5xx counter. That is useful regression evidence, not a national
capacity, per-city tail-latency or open-arrival-rate campaign. Extend it through
bounded tasks; do not weaken its existing checks or represent new commands as
already available. New tools such as k6 require need/license/version/security
review before adoption; no dependency is added by this plan.

## Benchmark rules and use cases

- **B-BR-RST-P01 — equivalent work:** compare identical validated semantics,
  datasets, result sets, durability and hardware. Rust vs Go preparation is
  comparable only where Go has an equivalent transform; otherwise mark that
  comparison unavailable and report stages independently.
- **B-BR-RST-P02 — reproducible inputs:** freeze source/fixture hashes, seeds,
  workloads, revisions, versions, settings, resource limits and budgets before
  each run. Count and explain every excluded, failed, censored or dropped sample.
- **B-BR-RST-P03 — correct before fast:** preserve row accounting, canonical
  CNPJ uniqueness, UUID/history, source separation, pending/reviewed locations
  and bounded queues. Any mismatch, silent loss, unauthorized promotion or
  weakened durability fails the variant regardless of speed.
- **B-BR-RST-P04 — isolated experiments:** use explicit disposable lab resources
  and a capped run manifest. Never stress the open phone, staging preview,
  production or a shared database, reset shared statistics, or clear host caches
  as an implied consequence of this planning request.
- **B-BR-RST-P05 — fair geographic coverage:** measure request-weighted totals
  and city strata separately. A fast hot-city cache cannot hide slow small,
  empty, rural, boundary or previously unseen cities. Synthetic request weights
  are hypotheses, never claims about actual user traffic.
- **B-BR-RST-P06 — useful evidence:** retain raw machine-readable samples and
  definitions, confidence/variability and failures. Do not average percentiles,
  discard timeouts, call incomplete instrumentation zero, or choose the best
  run. A dashboard summarizes evidence; it does not replace it.

BUC-RST-P01: reproduce a clean full import and measure each stage separately.
BUC-RST-P02: replay/delta updates while city and nearby reads continue.
BUC-RST-P03: compare indexes and rebuildable partitions against the same oracle.
BUC-RST-P04: find sustainable offered load, headroom and recovery limits.
BUC-RST-P05: detect regressions on the same artifact/hardware/workload family.

## Additional phases, one bounded task at a time

Every phase produces its own English evidence record with input/behavior SHA,
actual commands, machine-readable artifacts, acceptance/limits and next task.
All phases below are PLANNED; no run is implied by their existence. Begin only
the selected phase and verified dependencies, rather than executing the entire
matrix in one turn.

### RST-10 — Experimental protocol and performance budgets

Dependency: verified RST-05 evidence; no dependency on unfinished RST-06 for this
protocol-only task. Inventory the lab and target deployment class, current
queries, migration/tool versions, data ownership and instrumentation gaps.
Freeze a versioned run-manifest schema, metric dictionary, workload IDs, resource
ceilings, stop conditions, allowed variants and wall-time/storage budget.

Deliver a workload/SLO worksheet: scenario, layer, dataset, offered load, p95/p99
budget, allowed errors/drops, ingest/freshness target, memory/disk limits and
recovery target. The original 256MiB parser / 100ms city / 150ms nearby p95 at
eight clients remain provisional laboratory hypotheses; they are not API-wide
or production promises. Establish a baseline before freezing useful relative
regression thresholds. Unknown hardware/cost/traffic stays unknown.

Exit: a reviewer can reproduce and decide pass/fail from the manifest without
inventing thresholds after seeing results. No load runner, database or network
change is required for this phase.

### RST-11 — Representative datasets and geographic request distributions

Dependency: RST-10. Implement deterministic synthetic fixtures/generators and
an independent correctness oracle. Use checksum-valid synthetic establishment
identities, explicit IBGE edition/UF mapping and fictitious station attributes;
prevent collisions and label generated locations as synthetic. Never turn a
generated point into reviewed real evidence or retain contributor GPS.

Create: a tiny correctness set; a representative-size set based on a measured
permitted snapshot; 100k and 1M station stress sets (not claims about real station
counts); and history cardinalities 1/10/100 assertions per station where the
lab storage budget allows. Vary row/field sizes, duplicate/quarantine ratios,
missing coordinates and changed/no-op ratios independently. Preserve actual
skew as aggregate shape when available; do not scale by copying identical CNPJs.
No new national fetch is required just to build synthetic stress cases.

Freeze request traces for: uniform municipalities including zero-row cases;
weights proportional to fixture station count (not population/user traffic);
80% of requests to 20% of selected cities; and 90% to one hot city as an
adversarial case. Pin exact city sets/weights/seeds and rotate which cities are
hot. Stratify by cardinality (empty/1–10/11–100/>100 stations), UF, urban/rural
shape and border cases, reporting empty strata rather than inventing rows.

Exit: deterministic byte hashes and counts, oracle result sets, documented
distribution and statistical weights. Reordered source inputs retain semantics.

### RST-12 — Measurement harness and instrumentation qualification

Dependency: RST-11. Extend existing operator/load tooling, with fixture tests for
latency/count/error math and histogram merging. Record DB statements, API end
to end and client scheduling separately. Qualify an open-arrival-rate driver
alongside closed-loop 1/8/32-client comparisons; document generator CPU/network
headroom and scheduled/started/completed/dropped counters.

Test timeout, non-2xx, semantic wrong-result, cancellation, missed scheduling,
sample-file interruption and instrumentation permission-denied paths. Validate
telemetry overhead with instrumentation on/off in paired trials. SQL execution
plans belong to diagnostic runs, not every timed request. Pin clocks and units;
cross-process lag needs synchronized clocks and recorded uncertainty.

Exit: a deliberately slow/erroring fixture cannot produce a falsely healthy
report; raw samples reconcile to summary counters. Tools/permissions unavailable
are explicit evidence gaps, not fabricated zero values.

### RST-13 — Rust preparation, spill and resource scaling

Dependencies: RST-12 and verified RST-03/04. Measure local read/parse/normalize/
municipality lookup/dedup/join/sort/spill/serialize/hash/manifest separately and
as a complete preparation path. Build optimized binaries with recorded flags;
exclude compile/setup from preparation timing and report them separately.

Run clean snapshot, unchanged replay, source reorder, 1%/10% changed records,
duplicate-heavy, quarantine-heavy, long-field and spill-forced cases. Vary one
bounded setting at a time: buffer/spill budget, batch size and worker count if
parallelism actually exists. No speculative thread pool or runtime addition.

Exit: identical oracle/checksums, accepted+duplicate+quarantined=input accounting,
per-stage time, rows/s and MiB/s with peak process-tree/cgroup memory, temporary
disk and CPU-seconds. Report empirical memory/time scaling from representative
to 100k/1M, no speedup claim against a non-equivalent Go path.

### RST-14 — Database construction and incremental load baseline

Dependencies: RST-13 and RST-05. Use the owned Go loader on an isolated real
PostGIS database. Time database initialization/migrations, validation, staging,
reconciliation, profile/read-view creation, index build and ANALYZE separately;
mark unimplemented publication/profile stages NOT_AVAILABLE. Measure full
build-to-query-ready only when the owned publication path exists and passes
its correctness gate. Direct synthetic seeding tests query mechanics separately
and cannot stand in for ingestion/publication timing.

Measure empty build, unchanged replay, 1%/10% changes, stale/duplicate input and
concurrent duplicate loaders. Compare only implemented bounded row/batch/COPY
strategies under the same constraints/durability; a new staging COPY adapter is
a separate tested code task. Capture WAL, transactions, commit/lock waits,
dead tuples, index/temp bytes and total time to a usable catalog. Assess index
build cost on a fresh lab build separately from concurrent live-index creation.

Exit: IDs/counts/history and failed-run visibility agree with the oracle; cost
per accepted/changed row and query-ready time have explicit denominators. Empty,
upgrade, restricted-role, failure/recovery and concurrent writes get immediate
real-PostGIS/race coverage when implementation changes, not at campaign end.

### RST-15 — City search, pagination and traffic distribution

Dependencies: RST-12/14; the measured read path must be available and identified.
Measure raw SQL and actual Go API separately for exact city listing, supported
text/UF filters, station/profile detail and negative/empty lookup. Use actual
existing endpoints/query predicates; a missing feature is a separate task.
Freeze 5/20/50-row pages and first/middle/last valid keyset cursors, including
same-sort-value ties. Observe deep OFFSET only as a diagnostic comparator if
that SQL exists; do not introduce it into the product.

Exercise all RST-11 geographic distributions at controlled 1/8/32 clients and
the predeclared arrival-rate range. Compare cold/restarted, warm and rotating
unseen-city working sets; pin exact definitions below. Test parameter-sensitive
plans for sparse and large cities, prepared generic/custom plan behavior and
estimate/actual cardinality mismatch. Measure proposed composite/partial indexes
one change at a time with fresh comparable statistics and build/storage costs.

Exit: correct stable pages with no gaps/duplicates, per-query/per-stratum
p50/p95/p99/error counts and offered/achieved rate. Show worst strata, weighted
national workload results and coverage denominators; do not pool unlike mixes.

### RST-16 — Spatial reads and precise-location eligibility cost

Dependencies: RST-15 and verified RST-06 for eligibility cases. Freeze nearby
discovery radii 150m/1km/5km/20km, dense/sparse sites, empty results, tied distances
and cross-city/UF borders; these are lab scenarios, not changed API permissions.
Measure matching GiST/type/predicate use, candidate/result ratio and distance
sort/recheck costs. Benchmark the authoritative capture eligibility path
separately from discovery and preserve its real request/proof preconditions.

Exit: spatial result oracle and 149.9/150/150.1m/fix-integrity negative cases
agree on real PostGIS. Latency, candidate pruning and source/location coverage
are separate metrics. Coordinate completeness or faster KNN never certifies
station-location accuracy or grants capture trust.

### RST-17 — Physical design and partitioning comparison

Dependencies: RST-15/16 baselines. Compare A: existing unpartitioned indexed
catalog; B: rebuildable UF read projection; C: rebuildable municipality-hash
projection with a small predeclared partition count, for example 8/16/32.
Canonical station identities/active-CNPJ uniqueness/FKs stay in their owner.
Measure optional date partitions/BRIN separately on append-only history with
the actual range/retention queries, not as a change to current station semantics.

First compare projection A with B/C using identical columns/indexes to isolate
partition effects; report canonical joins as their own cost. Include single-city,
cross-partition nearby, all-city and CNPJ-only lookups with/without pruning,
generic prepared plans, cache miss, write amplification, build/rebuild, ANALYZE,
vacuum and partition lifecycle. Compare whole-plan buffers once, not the sum
of inclusive parent/child buffer counters.

Exit: a versioned decision using predeclared benefit/regression/resource bounds
and repeated trials, including unsupported/inconclusive cases. Keep A if there
is no material repeatable total-workload benefit. No per-city database/table,
weakened global uniqueness or automatic production migration.

### RST-18 — Mixed reads, ingestion contention and sustainable capacity

Dependencies: RST-17 and verified RST-08/09 operational/read publication paths.
Run reads alone, full import alone, delta alone, then the same reads with full
and incremental ingestion. Preserve identical dataset/workload/configuration.
Sweep offered arrival rate and bounded importer concurrency separately; refine
near the observed saturation knee. An illustrative rate ladder is 5/20/50/100
requests/s, revised in RST-10 from lab limits, not a traffic forecast.

Measure queue delay, connection-pool wait, backpressure, lock contention,
checkpoint/IO interference, freshness/review backlog and per-city tail penalty.
Define sustainable capacity as the highest measured load meeting all frozen
latency/error/drop/resource budgets without growing backlog during the declared
steady interval. Report client count, achieved throughput and headroom against
selected demand, not a single unqualified maximum QPS.

Exit: a capacity envelope and bottleneck evidence. If the load generator limits
the run, classify it as generator-limited, not the server's capacity.

### RST-19 — Failure, replay and recovery under load

Dependencies: RST-18 and RST-08 recovery contracts. Inject bounded worker kill,
DB disconnect, disk-full, malformed/truncated batch, checksum mismatch, duplicate
concurrent loader and stale lease; add upstream timeout/429 only if that fetch
path is actually enabled. Use explicit disposable resources and one fault at a
time before combinations. Preserve required durability settings throughout.

Exit: no lost accepted facts/duplicate canonical identities/false completeness,
and measured detect/retry/recover/backlog-drain times, read degradation and
orphan/temp cleanup. Reconcile before/after manifests and invariants. A lab
48h virtual-clock outage tests logic; it does not prove 48h real uptime or a
backup-restore RPO/RTO. Real restore acceptance remains with its owning plan.

### RST-20 — Soak, maintenance and cost efficiency

Dependency: RST-19. Run a bounded 30-minute pilot, then separately scheduled
24h and 48h campaigns on provisioned isolated resources. Short runs cannot be
labelled soak success. Include repeated imports, retention/partition maintenance,
autovacuum/checkpoints and hot-city rotation; track memory/file/connection growth,
dead tuples, index growth, disk pressure and backlog trends over time.

Compare workstation and a constrained deployment-class lab with pinned CPU/RAM/
storage/IO/network limits. Report CPU-seconds/1k prepared and changed rows,
MiB/WAL per changed row, storage per station/assertion, query resource cost and
minimum measured headroom. If supplied, use dated provider prices to calculate
cost per million successful queries and per full/delta import with stated
utilization/amortization; no invented currency values or free shared CPU.

Exit: maintenance included, growth/leak slopes and variability reported, costs
traceable to measured resources/prices. Unscheduled or interrupted long runs
remain OWED/INCOMPLETE, never silently replaced with extrapolation.

### RST-21 — Decision, reproducible report and regression tiers

Dependency: complete selected RST-10–20 matrix; explicitly name omitted scenarios
and unaccepted obligations. Deliver machine-readable results and standalone
exportable plots: per-stage ingest time/throughput, memory vs rows, DB bytes/WAL,
latency vs offered load, weighted and per-city tails, partition tradeoffs,
freshness/recovery and cost. Each plot names units, workload, counts, hardware,
cache state and uncertainty. Never fill missing runs with zero values.

Record retain/change/reject for each index/partition/batch/parallelism hypothesis,
plus a separate implementation/migration/rollback task for any accepted change.
Freeze a bounded deterministic correctness/resource smoke for scoped ordinary
CI if implemented; noisy performance comparisons run manually on matched lab
hardware, long soak/release campaigns follow existing CI_PLAN. No new recurring
automation or aggregate suite on every documentation edit.

Exit: a clean checkout can reproduce the selected manifest using documented
implemented commands; every performance claim links raw evidence. Accepted lab
budgets do not automatically accept P29-T02/G29/G09 or a production SLA.

## Metric dictionary and denominators

Every record includes `run_id`, scenario/phase, trial/variant, dataset/workload
hashes, code/config versions, layer, unit and observation interval. Raw samples
may stay in an access-controlled local artifact bundle; commit only sanitized
summaries/manifests and small synthetic fixtures. No credentials, DSNs, real
contributor identifiers/GPS or production photos in samples, SQL plans or logs.

1. **Preparation:** when an authorized fetch path is enabled, separately record
   fetch/first-byte/download/decompression time, wire/uncompressed bytes, retry/
   429 counts and compression ratio; offline-file preparation excludes network
   time explicitly. Record raw bytes/rows, accepted/duplicate/quarantined by reason,
   unique CNPJs, unmatched/ambiguous municipalities, coordinate candidate/conflict
   coverage; wall/CPU seconds per stage, rows/s = input rows / stage wall seconds,
   MiB/s = input bytes / 2^20 / read wall seconds, output/spill bytes, temporary
   file count, peak RSS/process-tree or cgroup peak (identify scope), CPU use,
   page faults, IO bytes/ops and read/write wait. Distinguish throughput based
   on input rows from useful accepted rows; replay is not a full parse speedup.
2. **DB load/construction:** validated/staged/applied/changed/no-op/failed rows,
   transactions and batch size, time-to-query-ready, commit/lock/pool waits,
   active/idle connections, lock timeouts/deadlocks, WAL bytes/records/full-page
   images, buffers hit/read/dirtied/written, temp bytes/spills, index/table/total
   size, build/ANALYZE/vacuum durations, live/dead tuples and checkpoint metrics.
   Report WAL/change-row and bytes/station with zero-denominator as undefined.
3. **Reads:** offered/started/completed/successful requests/s; scheduled drops,
   transport errors, timeouts, HTTP outcomes and semantic mismatches separately;
   p50/p90/p95/p99/max and sample count per query/layer/stratum; client queue,
   pool wait, DB plan/execution, serialization, response bytes and API total.
   A timeout is a right-censored latency at its deadline and a failed request,
   not an omitted sample or exact elapsed completion. Goodput counts only correct
   responses meeting the frozen budget. Histograms merge only like scenarios.
4. **Geographic/plan efficiency:** selected/covered/empty municipalities by UF
   and size, request weights, per-stratum latency/errors and worst groups;
   rows examined/returned where observable, actual vs estimated rows, index/
   sequential/bitmap scans, partitions touched/pruned, planning overhead,
   rechecks, heap fetches and shared-hit/(hit+read) with undefined denominators.
   A PostgreSQL shared-buffer hit is not an OS/device cache measurement.
5. **Mixed operation/freshness:** eligible-to-staged/applied/read-visible lag,
   upstream source age separately, unchanged replay time, queue depth/oldest age,
   review-required backlog by reason, service vs enqueue rate, retry volume and
   read p95/p99 ratio with vs without ingestion at identical offered load.
   Waiting for human review is not hidden in an importer throughput figure.
6. **Host, recovery and efficiency:** CPU time/utilization/throttling/steal where
   available, RSS/cgroup/OS cache distinguished, disk capacity/latency/IOPS/
   throughput, network bytes/errors, file descriptors, recovery and cleanup
   durations, verified row loss/duplication, resource/cost per useful unit and
   steady-state backlog/memory/storage slopes. Replication lag is N/A unless
   replication is actually configured; energy is N/A without a real meter.

## Experimental method and acceptance

- Freeze the environment: CPU model/vCPU/quota/governor, RAM/swap/container
  limits, storage/filesystem/free space, host contention, OS/kernel, exact Rust/
  Go/PostgreSQL/PostGIS/PROJ/tool/image versions, release flags, schema/indexes,
  pool size and PostgreSQL `shared_buffers`, `work_mem`, parallelism, JIT,
  durability/checkpoint/autovacuum settings. Data generation/setup is untimed
  and reported separately. Do not change several knobs between variants.
- Cache labels are explicit: process-cold, DB-restarted with OS cache unknown,
  OS-and-DB-cold only on a dedicated proven environment, or warmed to a recorded
  workload. A new connection or DB restart alone does not prove disk cold.
  Do not run privileged global cache drops on a shared host.
- Use a staged matrix: tiny correctness qualification, representative baseline,
  one-factor screening, selected worst-case/combined runs, then long campaigns.
  Do not execute a huge Cartesian product. Predeclare scenario exclusions and
  budgets before screening; optimization decisions need representative and
  worst-stratum evidence, not just a favorable microbenchmark.
- Initial reproducibility policy: one untimed warm-up and at least five paired
  measured trials for short cases, randomized/alternated A/B order, each with
  independently reset comparable data. For steady API cases, predeclare at least
  60s warm-up and 5min observation and extend for sample needs. Report per-trial
  values, median, range and 95% uncertainty using an explicitly versioned method
  on independent runs/time blocks, not falsely independent autocorrelated
  requests. These durations are starting protocol choices, revisable at RST-10.
- Tail reporting policy: target at least 10,000 completed samples per reported
  p99 query/stratum to expose roughly 100 tail observations; this is a declared
  lab minimum, not a guarantee of accuracy. Report N, censored/drop counts and
  uncertainty; smaller strata get descriptive tails marked INCONCLUSIVE for
  gating, or a longer run. Never average per-city or per-run percentiles; retain
  raw histograms for matched weighted totals and independent per-city results.
- Separate closed-loop concurrency comparisons from open scheduled arrival
  rates to expose overload rather than allowing slower clients to reduce demand
  invisibly. Record scheduling delay and unstarted work; maximum VUs/connections
  are capped. Saturation and expected-overload experiments are diagnostic, not
  a way to lower ordinary acceptance budgets.
- Use PostgreSQL stats deltas with recorded reset times, restricted observer
  permissions and isolated workloads. `pg_stat_statements` gives aggregated
  statement data, not request p99; `pg_stat_io`/WAL/lock/activity views explain
  bottlenecks. Pin view/column support to the actual server version. Record
  `track_io_timing`/planning tracking overhead and availability.
- Capture selected `EXPLAIN (ANALYZE, BUFFERS, WAL, SETTINGS, FORMAT JSON)` in
  diagnostic trials on the isolated DB. ANALYZE executes the statement and
  adds profiling cost; it is not the API latency distribution or network
  measurement. Choose supported options per version and state which metrics
  are unavailable; do not infer whole-query runtime memory from planner memory.
- Predeclare absolute budgets plus a materiality rule: for example an optional
  design must improve the selected primary metric by >=15% across repeated
  paired trials without >10% regression in critical strata, unacceptable build/
  maintenance costs or broken correctness. These percentages are proposed
  decision thresholds to freeze/refine at RST-10, not achieved improvements.
  Uncertain comparisons are INCONCLUSIVE; retain the simpler baseline.
- Status per scenario: PASS, FAIL, INCOMPLETE, INCONCLUSIVE or NOT_APPLICABLE
  with reason. Correctness failure stops optimization. Predeclared disk/memory/
  time/queue limits abort safely and leave incomplete evidence; do not drop failed
  trials and rerun until green. Diagnose and version any corrected rerun.

## Artifact contract and execution handoff

Proposed bundle (implemented under RST-12, not existing commands): manifest,
environment/config hashes, dataset summaries, query IDs/parameter distribution,
counts/oracle checks, raw per-request CSV or histograms with failure/deadline
records, per-stage timing JSON, DB/host interval snapshots, EXPLAIN JSON, summary
JSON/CSV, figures and an English decision report. Include UTC intervals plus
elapsed monotonic timings, units, collector version and artifact checksums.
Specify private-artifact retention and sanitize before any publication.

Select `RST-10` to start with documentation/protocol only. Use the companion
[handoff prompt](RUST_STATION_INGESTION_PROMPT.md); verify actual completed
dependencies and do not restart RST-00 or concurrent implementation. Future
commands must be documented and tested when their phase implements them.

Method references checked 2026-10-09:
[PostgreSQL monitoring](https://www.postgresql.org/docs/18/monitoring-stats.html),
[statement aggregates](https://www.postgresql.org/docs/18/pgstatstatements.html),
[EXPLAIN behavior](https://www.postgresql.org/docs/18/sql-explain.html),
[open/closed load models](https://grafana.com/docs/k6/latest/using-k6/scenarios/concepts/open-vs-closed/),
[arrival-rate scheduling](https://grafana.com/docs/k6/latest/using-k6/scenarios/executors/constant-arrival-rate/),
[dropped scheduled work](https://grafana.com/docs/k6/latest/using-k6/scenarios/concepts/dropped-iterations/).
Numeric workloads, durations and decision bounds above are proposed project
protocol choices, not recommendations or measured results from those sources.

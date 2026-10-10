# RST operational hardening and bounded VPS acceptance

Status: IN_PROGRESS. User authorization: 2026-10-10, only Abastevo resources.
Current origin: https://teste.abastevo.com.br; namespace: abastevo-temp.
Other apps, images, databases, shared edge, host services and global cleanup are
outside scope. No load test may disable durability, quotas or safety checks.

## Behavior before implementation

- B-BR-RST-H01: aggregate concurrent import throughput is successfully completed
  batches divided by monotonic elapsed import-window seconds, not summed service
  durations. Per-operation latency remains a separate distribution.
- B-BR-RST-H02: PostgreSQL checkpoint metrics name their semantics and use the
  supported server schema; collection errors fail the run or mark an explicit
  unavailable metric, never silently become zero. PostgreSQL 18 completed
  checkpoints use pg_stat_checkpointer.num_done.
- B-BR-RST-H03: preparation/loader resources stay bounded; checksum validation
  and canonical identity/history are preserved across unchanged deltas/retries.
  Recovery is fenced/idempotent, not deletion of partial or unrelated data.
- B-BR-RST-H04: only complete verified official registry facts may flow through
  existing Directory/Profile owners. Synthetic batches remain labelled test
  evidence; PMQC candidates do not gain reviewed trust from importing them.
- B-BR-RST-H05: capacity requests use actual HTTPS HTTP responses and semantic
  checks, fixed scenario seeds, separate warm-up and observation, offered versus
  achieved rates, errors/drops and raw timed artifacts. Request/s is not users;
  user estimates state their per-user request cadence and safety reserve.
- B-BR-RST-H06: VPS mutations and destructive test fixtures are confined to
  Abastevo and uniquely owned disposable artifacts/databases. Existing persistent
  data is retained. Every campaign has CPU/memory/concurrency/time/disk ceilings,
  health/pressure/OOM stop conditions and cleanup of only its own resources.
- B-BR-RST-H07: short pilot results never certify 24/48h stability. Long-run
  outcomes remain pending until their actual duration and accepted artifacts.

BUC-RST-H01: overlapping importers yield correct aggregate throughput.
BUC-RST-H02: denied/unsupported telemetry fails visibly; valid zero is distinct.
BUC-RST-H03: retry interrupted official ingestion without losing accepted facts.
BUC-RST-H04: serve source-separated station profiles through the owned domain.
BUC-RST-H05: stop load safely when an Abastevo/host resource budget is breached.

Execution: metric RED/GREEN and real PostgreSQL 18 checks first; bounded
preparation/loader/publication changes in separate tested checkpoints; guarded
HTTPS capacity/pilot next; truthful RST-21 report and outstanding long evidence.
Each critical persistence/concurrency change requires immediate negative/race/
real-PostGIS tests. No mutation of another namespace or global image/volume prune.

## Metric checkpoint (2026-10-10)

Status: LOCAL_DONE / INTEGRATION_PENDING for metric corrections only.
Validation: PASS (does not close this overall hardening campaign).

RED reproduced four concurrent 2-second imports incorrectly reporting 0.5/s
and the unsupported checkpoint columns. GREEN reports 2/s over the 2-second
monotonic window ending when the last importer exits (not when later reads end).
Individual maximum latency remains separate. Checkpoint failures propagate;
capacity rejects observed resets. Soak WAL/size/dead/backlog collectors now fail
loudly too, and table size excludes indexes instead of double-counting them.
PostgreSQL schema reference: [PG18 checkpointer statistics](https://www.postgresql.org/docs/18/monitoring-stats.html#MONITORING-PG-STAT-CHECKPOINTER-VIEW).

Commands: targeted `go test -tags=integration`, the same pure regression cases
under `-race`, and a static integration binary in Job
`abastevo-rst-metrics-20261010` (180s deadline, 250m CPU, 256Mi, GOMEMLIMIT 192MiB).
All three tests passed; real PostgreSQL 180006 observed num_done 169 -> 169.
The uniquely created `read_test_*` database was dropped by its own cleanup;
no persistent fixture was truncated or modified. Existing Abastevo API pod
7m/239Mi, DB 22m/182Mi after the run, all service restart counts zero.
Counter is cluster-wide within Abastevo; differences cannot isolate a single
benchmark from concurrent activity. Existing shared host services were untouched.

Historical RST-18 importer-rate and checkpoint values are invalid for comparisons;
RST-20 table/index cost slopes need a rerun with these collectors. Earlier Reader
measurements omit HTTP/TLS/edge overhead. RST-17 UF results are exploratory and
do not authorize a partition migration. Existing 24,552 assertions are synthetic.

## Operational loader contract

B-BR-RST-H08: operational file loads consume at most one bounded JSONL row at a
time plus bounded manifest/input metadata. Exact per-edition membership lives in
PostgreSQL, not a process-wide dedup map. The legacy in-memory adapter remains a
lab compatibility path and is not advertised as the operational loader.
B-BR-RST-H09: one transaction owns a prepared batch under a transaction-scoped
advisory lock. Cancellation/disconnect rolls back its partial attempt; retry
reuses existing assertion IDs, including legacy partial runs. A completed run is
bound to the manifest SHA256; changing a manifest under its run identity is an
error. All inputs complete together, and no retry deletes an assertion/run.
BUC-RST-H06: unchanged deltas complete with stable canonical IDs and separate
per-edition membership; concurrent replay converges; malformed tails, checksum
changes and cancelled transactions do not become visible as completed runs.

B-BR-RST-H10: prepared registry publication pages through complete bound runs,
resolves identity through Directory and ensures an unclaimed profile through the
StationProfile port. PMQC candidates do not create stations, locations, grants or
badges. Valid source municipality/UF fill a missing canonical locality; conflicting
existing locality fails visibly for review. This path does not grant official
eligibility or change the official/community source distinction. Existing profiles
and business projections are preserved. Publication retry is idempotent.

B-BR-RST-H11: ANP metadata defines DATAPUBLICACAO as authorization publication
and DATAVINCULACAO as the distributor relationship date. Neither is an opening
date and no ordering constraint between them is supported. Parser v0.3 / policy
v2 / station-assertion-v2 normalize strict DD/MM/YYYY and ISO calendar dates;
published_at and effective_at retain authorization publication semantics and
brand_linked_at preserves distributor linkage separately. Invalid calendar dates
quarantine; independent date order does not. The v2 checksum has a version prefix
so changed semantics never reuse a v1 assertion identity. Go still accepts
historical v1 batches. The prior RST-01 date-reversal rule is superseded.
Verified source: [ANP registry metadata](https://www.gov.br/anp/pt-br/centrais-de-conteudo/dados-abertos/arquivos/arquivos-dados-cadastrais-dos-revendedores-varejistas-de-combustiveis-automotivos/metadados-revendedores-varejistas-combustiveis-automoveis.pdf).

## Operational ingestion checkpoint (2026-10-10)

Status: LOCAL_DONE / INTEGRATION_PENDING for ingestion source.
Validation: PASS.

- File-backed Rust emission bounds serialized sorting to 4 MiB/4096 rows,
  fan-in 32, 1024 run paths, 512 MiB cumulative framed spill writes and 100 MiB
  per output (one final spill/merge write can cross the disk guard before refusal).
  Manifest publishes last from an owned sibling directory. Failures clean only
  unpublished owned files. The parsed batch still resides in memory under the
  parser's input/row caps; this is bounded emission, not a streaming CSV parser.
- `prepare_registry` is the operational CLI; it generates no synthetic PMQC.
  Input CSV is capped at 100 MiB, aliases at 4 MiB before reading them in full.
  Legacy `emit_run`/`LoadBatch` remain compatibility/benchmark paths.
- Migration 000053 adds per-edition immutable assertion membership and manifest
  binding without deleting/backfilling history. The operational Go loader reads
  JSONL one row at a time, uses server-side exact dedup, one transaction and an
  advisory lock per batch. An unchanged delta has its own accepted membership
  while sharing stable assertion IDs. Replay does not update existing membership.
- Loader and publication retry only transient connection/serialization failures,
  at most 3 attempts by default (hard maximum 5) under one overall deadline
  (hard maximum 30 minutes), with at most 2 DB connections. Integrity/permission
  failures and caller cancellation are terminal. Process restart reuses the
  same bound manifest and resumes idempotently; a supervisor must restart an
  exited process. Publication pages 100 rows through explicit module ports.
- Assertion v2 preserves distributor dates in the prepared artifact, separately
  from authorization publication. Database effective_date follows publication.
  Raw source and prepared files are private operational artifacts, never Git.

Immediate real PostGIS tests ran in Abastevo-only Jobs, each 250m CPU/256Mi,
180s hard deadline, disposable test databases. `abastevo-rst-prepared-accept-20261010`
passed unchanged/concurrent delta, retained partial IDs, malformed-tail/checksum/
cancel rollback, manifest conflict, migration 52->53 failure rollback/recovery,
restricted-role load plus denied canonical writes/DDL, and backend disconnection
followed by convergent retries. `abastevo-rst-publication-final-20261010` passed
loader -> city HTTP -> unclaimed profile HTTP, replay and conflict negatives.
`abastevo-rst-metrics-final-20261010` passed PG18 counters and fail-loud contention
collection; invalid historic mode aggregate was corrected and queries now scope
active current-database waits. No statistics reset or service restart.

Rust full tests/fmt/clippy passed; the added overlarge-row failure test proves
unpublished spill cleanup preserves a neighboring file. Go affected unit/race,
vet and sqlc vet/generate passed. A transient-retry negative test caught that
context.DeadlineExceeded implements net.Error; explicit cancellation refusal
fixed it. No known failure is deferred. Final scoped VPS import/HTTP evidence
follows; these source tests alone do not certify VPS capacity or long stability.

### Cross-host portability correction

The first bounded VPS preparation run correctly refused a quarantine-output hash
mismatch: quarantine row locators included the caller's absolute CSV path.
The operational CLI now uses the source basename, and a subprocess regression
proves all four artifacts are byte-identical across different parent directories.
No DB import occurred during this refused offline run. The original files remain
as audit evidence; only `prepared-portable` is an authorized load input.

## Frozen VPS HTTP protocol

H12: actual HTTPS, external workstation client, no edge/protection changes,
one TLS connection per request; <=20 offered request/s, <=8 in flight, no client
queue. Ladder 1/5/10/20 request/s uses short diagnostic windows, followed by five
independent 60s warm-up + 300s steady trials at the safe selected rate. These are
pilot windows, not a 24/48h result. A 500ms client HTTPS p95 hypothesis is frozen
before measurement and includes network/TLS/edge; the old 100ms Reader hypothesis
is a different layer. Every error/drop/missed arrival/telemetry failure stops the
campaign. Each route needs >=100 successful samples per trial for p95 qualification;
p99 is unavailable below 10,000 per route. Never average percentiles together.

The frozen 9-request cycle weights dense-city 3, Sorriso 1, single-station city 1,
actual zero-station reference city 1, national text 1, detail 1 and profile 1.
This is a chosen stress distribution, not measured human behavior. Source data:
45,617 accepted ANP assertions across 5,503 IBGE municipalities; 5,571 reference
municipalities; 121 unresolved city-name variants retained in quarantine. The
largest source city has 1,521 accepted assertions (SP/3550308); sparse AC/1200054
has one and empty RO/1100098 has zero. Existing non-ANP development station remains.

H13: no maximum-user claim without saturation evidence and a behavior model.
A tested read-rate lower bound R allows only a scenario estimate:
active users = R * reserve_fraction * seconds_per_cycle / GETs_per_cycle.
For example 70% of the tested rate with one GET/30s differs fourfold from four
GETs/30s. This excludes uploads, OCR, writes, auth and image serving; registered
users/DAU/connection concurrency cannot be inferred from read request/s.

Safety is checked every ~5s (bounded SSH latency also included): retain >=12GiB
host available RAM, >=50GiB free disk, load1<=65% of 8 CPUs, every Abastevo
container below 70% of its existing memory limit, healthy/unchanged pods and
restart counts, no OOM, <=4 lock waiters and no growing run backlog. Missing
metrics stop work. Auxiliary Jobs never exceed 250m CPU; preparation 512Mi,
loader/tests 256Mi. Service limits stay API1CPU/1Gi, DB1CPU/1Gi, worker500m/512Mi,
storage500m/1Gi. Guard deletion rechecks this campaign's ownership label and
can delete only one explicitly named disposable Job in abastevo-temp.

### Staging schema compatibility incident

Backup `pre-000053.dump` (mode0600, outside Git) was restored in a uniquely owned
disposable database and retained 24,552 assertions/11 stations before deletion of
that test DB. Backup SHA256: afd5d3778bbd28e80f4d771f2b34f44ec3daa56df1999b8517d143e22aac2a37.
Migration applied only000053. The old API's strict expected-ledger check then
refused schema53, removing readiness and causing HTTPS502. Load admission refused
while unhealthy. This was a deployment sequencing error, not an internet failure;
no data loss or resource-limit increase occurred. Compiled API/worker2cd3fb6 were
mounted read-only from an Abastevo-owned path and only the existing Abastevo
Deployment rolled under its unchanged maxSurge0/maxUnavailable1 strategy.
The private media volume/image/env were preserved; three containers were recreated
as part of that owned rollout, not an OOM/crash. HTTPS readiness recovered200.
Future migrations require preparing/releasing the compatible API/worker together;
never claim zero downtime for this campaign. The temporary command override must
be reflected in the final deployment record, rather than claiming old image tags
identify the running source. No other deployment, image or namespace was changed.

National route qualification initially stopped on HTTP200/detail because the
trace incorrectly expected `id`; the public contract uses `station_id`.
The trace was corrected before capacity measurement. The failed raw diagnostic
is retained and excluded from qualified trials. No protection was weakened.

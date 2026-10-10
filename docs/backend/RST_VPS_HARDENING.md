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

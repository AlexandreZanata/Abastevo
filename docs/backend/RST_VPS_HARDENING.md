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

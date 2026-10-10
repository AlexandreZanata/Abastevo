# RST-21 bundle index (environment prep, not the decision)

Staging index for the RST-21 decision/report task. Every row names
the committed evidence, its scale label and its raw-artifact status:
raw per-run logs lived in ephemeral local bundles (`/tmp/rst*`)
and are NOT committed — only sanitized summaries, manifests and
small synthetic fixtures are versioned, per the metric dictionary.
A clean checkout reproduces every number with the listed commands.

| Phase | Evidence | Scale / verdict |
|---|---|---|
| RST-10 | `RUST_STATION_INGESTION_RST10.md` | protocol frozen, budgets provisional |
| RST-11 | `RUST_STATION_INGESTION_RST11.md` + `contracts/testdata/station-prep/datasets/` | deterministic datasets, frozen tiny oracle |
| RST-12 | `RUST_STATION_INGESTION_RST12.md` | harness qualified (9/9 loopback) |
| RST-13 | `RUST_STATION_INGESTION_RST13.md` | prep 20k/100k/1M matrix, spill accounting |
| RST-14 | `RUST_STATION_INGESTION_RST14.md` + `emit-tiny[/-delta1]/` | construction baseline, chain contracts fixed |
| RST-15 | `RUST_STATION_INGESTION_RST15.md` | 9 workloads raw/API, trigram accept-pending-migration |
| RST-16 | `RUST_STATION_INGESTION_RST16.md` | Vincenty oracle exact, 150m gate pins |
| RST-17 | `RUST_STATION_INGESTION_RST17.md` | DECIDED: UF-LIST direction, hash-16 rejected, BRIN direction |
| RST-18 | `RUST_STATION_INGESTION_RST18.md` | local ≥100rps+4imp, knee (100,200]; VPS 20rps |
| RST-19 | `RUST_STATION_INGESTION_RST19.md` | fault matrix green, fail-loud reload |
| RST-20 | `RUST_STATION_INGESTION_RST20.md` | 30-min pilot green; 24/48h OWED |

Omitted scenarios (never zero-filled): 8/32 partition counts,
1M spatial reads, concurrent live-index creation, upstream
429/timeout fetch, disk-full injection, 48h virtual-clock
outage, VPS soak rerun, currency costs (no prices supplied).
Unaccepted obligations: streaming emission, COPY/delta loaders,
orphan resume/reap, trigram/UF/BRIN migrations, 100ms budget
revision, 24/48h campaigns.

Hypothesis ledger (draft for RST-21 to finalize):
retain UF-LIST direction, trigram GIN, BRIN history;
reject hash-16, generic-plan forcing;
change needed: loader resume/reap, retention strategy,
100ms budget revision, checkpoint instrumentation.

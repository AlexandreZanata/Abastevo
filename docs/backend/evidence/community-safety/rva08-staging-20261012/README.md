# RVA08 staging runtime artifacts — 2026-10-12

Scope: only Abastevo staging, not production certification. API source b058a60,
worker 610bdd9, schema 000074. Original limits retained: API 1 CPU/1 GiB;
worker 0.5 CPU/512 MiB; private storage 0.5 CPU/1 GiB; database 1 CPU/1 GiB.
Training collection OFF, admission zero. Dataset contains 45,618 directory
rows and 70,169 mixed historical assertions; these counts do not establish
that every row is a distinct real ANP station.

## Trials and interpretation

- `https-post074-diagnosis`: before batched search, 120s planned steady window
  stopped after 33.471s; 155 offered, 151 successful, three timeouts, one drop.
- `https-page-batch`: after batched search, full 120s steady window PASS;
  600/600 successful, zero errors/drops, city/national p95 59.55/153.12ms.
  A separate 31-request synthetic consent probe ran during warm-up and finished
  at 22:55:07Z, before the steady window. Do not claim a pure read-only warm-up.
- `https-page-batch-confirmation`: two trials planned; only the first ran.
  It stopped at 115.428s: 563 offered, 554 successful, eight timeouts, one drop.
  Safety telemetry also timed out; it was NOT ignored. City/national p95
  92.98/179.94ms excludes errors, so good percentiles do not make this a pass.
- `https-page-batch-cpu-isolation`: full 120s steady window PASS; 600/600,
  zero errors/drops, city/national p95 74.05/164.55ms. Bounded CPU observation
  ran alongside; that observation is not from the failed confirmation window.

All campaigns use actual VPS-origin HTTPS at 5 offered req/s, at most eight
clients, 10s warm-up and a 500ms p95 budget. Both routes return bounded pages
of 20, checked for the expected items shape. Raw CSV includes failures/drops;
manifest hashes bind CSV and guard data. A guard failure stops scheduling.
Two successes separated by a failure do not establish sustained stability,
external-client performance, a user-capacity number, p99, or a release SLA.
No higher load or limit increase was attempted after the repeat failed.

## Aggregate CPU diagnostic

`observe-cpu.py` reads only aggregate `/proc/stat` and the exact owned PostgreSQL
cgroup for 150s. No other workload, command line, application data, credentials
or network payload is inspected. Run it only on the authorized VPS. The raw
observation is gzip-compressed without alteration; the manifest records both
raw and compressed hashes. Reproduce the summary locally with:

```sh
python3 docs/backend/evidence/community-safety/rva08-staging-20261012/analyze_cpu.py
```

The 1,476 observations span 149.909s: counter-weighted overall steal is 16.75%;
peak sample 83.16% over 0.102s; peak window of at least five seconds is 53.97%
over 5.020s. PostgreSQL used 22.106 CPU seconds and was throttled for 0.690s
across 34 periods in this observation. These are distinct measurements.
Linux defines steal as involuntary wait in the [kernel proc documentation](https://docs.kernel.org/filesystems/proc.html#miscellaneous-kernel-statistics-in-proc-stat).
Virtual CPU contention is an observed factor requiring infrastructure review;
causal attribution for the failed campaign remains unproven. Do not infer that
all pauses are caused by the hypervisor or that changing quotas will cure them.
No VM, shared host or other-container setting was changed.

## Consent, retention and outstanding gates

`consent-16cases.json`: actual signed HTTP acceptance, 31 requests, both
synthetic identities finish OFF. No training derivative was created.
`retention-after074.json`: read-only receipts/provider HEAD after restart;
five originals and four abandoned finals absent, READY elapsed canary still
retained. Full real-clock proof remains due 2026-10-13T22:09:34Z (18:09 Cuiaba),
after the autonomous cleanup boundary at 20:09:34Z. No backdating or manual
canary cleanup qualifies. Later snapshots are in the parent phase record.

Private fixture identifiers, photos, manifests, database dumps and key material
remain outside Git. Two DEAD verification jobs created on October 8 reference
EXPIRED sessions; they were inspected only by aggregate status and not replayed.
Shared-edge 3 MiB proposal is syntax-tested but not authorized/applied. ADB is
empty. Public maximum uploads/admission budget, real phone, elapsed retention,
backup/provider scope and sustained stability remain pending. Remaining RVA08
helpfulness transport and RVA10–14/CS15–25 are not accepted by these artifacts.

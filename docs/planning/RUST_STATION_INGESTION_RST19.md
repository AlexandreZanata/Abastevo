# RST-19 — Failure, replay and recovery under load

Status: TESTED — isolated disposable PostGIS only; one minimal
loader fix, no migration, no product change. Date: 2026-10-09.
Scope: `RUST_STATION_BENCHMARK_PLAN.md` RST-19 over the RST-18
matrix with the verified RST-08 recovery contracts (job leases,
retry budgets, dead-letter; jobs code unchanged since, so RST-08
evidence stands for leases and upstream 429/timeout, whose fetch
path stays disabled). Language: English document; user
communication in Portuguese.

B-BR/BUC: B-BR-RST-P01 (oracle counts agree on every completed
run, including after faults), B-BR-RST-P03 (failures stay failed
and visible — the one silent path found is now a loud error),
B-BR-RST-P04 (fresh disposable database per fault, one fault at
a time). Serves BUC-RST-P01 (ingestion failure costs) and
BUC-RST-P05 (regression pins for recovery behavior).

## 1. Finding and fix (RED → GREEN)

Killing a load mid-stage surfaced the error and kept partial
staged facts — but reloading the same manifest then returned a
nil error with a non-terminal report. Any retry loop treating
nil error as convergence would silently accept an unfinished
run: not false completeness (state never read "complete"),
but a silent non-recovery trap. Fix (5 lines, `LoadBatch`
reload path): a non-complete existing snapshot now returns an
explicit `unfinished <state> (run …): resolve before retry`
error instead of the report. No production callers exist;
replay of complete runs is untouched (idempotent, still
silent-nil as RST-05/14 require).

## 2. Fault matrix (real PostGIS, `-race`, durability asserted:
synchronous_commit on, staging relations logged)

- Worker kill (client cancel mid-stage, 1,500-row batch):
  error surfaces, run never complete, strict partial prefix
  retained, retry fails loudly, live rows unchanged by retry.
- DB disconnect (server-side backend termination mid-stage):
  same invariants through the server path.
- Duplicate cold loaders (2-way first-load race, wide
  window): exactly 1 healthy convergence with 1,500 accepted,
  live rows exactly 1,500 (global dedup, no double staging),
  loser never healthy.
- Malformed/truncated manifest and stream: refused naming the
  position before any run exists; 0 complete runs.
- Checksum mismatch (1 flipped byte): input-named refusal,
  0 complete runs, nothing staged.
- Stale edition (delta then older, committed emits): refused
  in 13.2 ms with a visible failed run, complete-run count
  frozen at 2, second input never staged (fail-fast).
- Read degradation during kill (app path, 2k stations):
  p95 before 2.18 / during 10.01 / after 2.64 ms with zero
  read errors throughout; fault detect 10.0 ms; post-fault
  latency drains immediately (no lingering penalty).

Orphan/temp cleanup: orphaned running runs stay queryable
with their partial facts (nothing lost, nothing completed);
the Go loader writes no temp files. Automatic resume/reap of
orphans is an owed follow-up task — detection is measured,
recovery-by-resume is explicitly not claimed.

## 3. Limits and next task

- Disk-full is UNSUPPORTED here (shared container disk cannot
  be exhausted without endangering neighbors; real acceptance
  stays with the owning storage plan). Upstream timeout/429 is
  NOT_APPLICABLE (fetch path disabled). The 48h virtual-clock
  outage was NOT_EXECUTED (owed; needs a dedicated
  clock-controlled harness — no 48h claim is made anywhere).
- Reproduce: `go test -tags=integration -count=1 -run
  'TestFault|TestLoadBatchIntegration' ./internal/modules/
  directory/adapters/registry/` (~15 s, `-race` clean).
- Next (smallest, separately authorized): RST-20 soak,
  maintenance and cost efficiency (30-minute pilot first;
  24/48h campaigns separately scheduled).

# RST-16 — Spatial reads and precise-location eligibility cost

Status: MEASURED — isolated disposable PostGIS only; no migration, no
product change, no permission change. Date: 2026-10-09. Scope:
`RUST_STATION_BENCHMARK_PLAN.md` RST-16, dependencies RST-15 and
verified RST-06 (authoritative path: `CheckPhotoCaptureLocation`
with production Locate wiring — reviewed station plus PostGIS fix
distance — benchmarked separately from discovery). Language: English
document; user communication in Portuguese.

B-BR/BUC: B-BR-RST-P01 (full Nearby sets agree with the independent
ellipsoidal oracle at every radius/site), B-BR-RST-P03 (boundary and
fix-integrity negatives refuse with the exact policy errors),
B-BR-RST-P04 (fresh disposable database per case, lab scenarios only).
Serves BUC-RST-P01 (spatial read costs) and BUC-RST-P05 (regression
pins for plans and verdicts).

## 1. What was added

- `read_spatial_bench_test.go` (integration): Vincenty direct
  (fix placement) and Vincenty inverse (independent result-set
  oracle, centimetre agreement with PostGIS geography) in-test;
  boundary rig (anchor, unreviewed/point-less stations, RJ border
  group); discovery matrix over radii 150m/1km/5km/15km at dense,
  sparse, empty-ocean and cross-UF border sites with GiST
  plan/candidate/result/buffer diagnostics; a raw-SQL 20km
  diagnostic beyond the 15000m product cap, labelled as such;
  eligibility boundary and fix-integrity agreement on real
  PostGIS; raw fix-distance vs full-gate cost split.
- Discovery keeps product predicates exactly; borders are data
  scenarios, never changed API permissions.

## 2. Measurements (100k census, fresh disposable PostGIS per bench)

Discovery first-page-20, API layer (p50/p95 ms, GiST plan,
examined/returned): dense 150m 1.1/3.0 (Index Scan, 3/3),
1km 5.0/7.3–12.3 (60/21), 5km 12.5/15.7–16.6 (1,140/21),
15km 67.5–69.8/78.3–94.9 (10,373/21, sort-bound); border-mid
150m 0.7–1.8/0.9–2.2 (2/2, SP+RJ mix confirmed), 1km
2.8–6.3/3.6–11.7 (47/21), 5km 6.2–19.5/9.8–33.8 (Bitmap
Heap+Bitmap Index, 1,929/21), 15km 77.7–105.9/87.7–166.5
(parallel Gather+Bitmap, 17,634/21). Sparse and empty-ocean
sites: 0.1–0.5 ms at every radius (0/0, GiST prunes
immediately). Sort/recheck cost concentrates in wide radii:
examined grows ~490x from 150m to 15km while returned stays 21.

Eligibility on real PostGIS: Vincenty pins 149.9/150/150.1m
measure within 0.05m and the authoritative gate allows,
allows and refuses exactly; fix-integrity negatives
(simulated, accuracy 101, future fix, manual, no permission)
refuse `ErrPhotoCaptureIneligible` at 10m without any
database call; unknown, unreviewed and point-less stations
refuse likewise. Gate cost at an eligible 100m fix: raw
PostGIS 0.26/0.37 ms vs full gate 0.44/0.60 ms (+0.17 ms
fix verification overhead).

## 3. Findings and limits

- Wide-radius spatial breaches the provisional RST-10 150 ms
  hypothesis (border 15km p95 up to ~167 ms; dense 15km ~79–95
  ms): same revision owed as the RST-15 1M city finding, owned
  by RST-18 capacity work — recorded, not averaged away.
- 20km runs raw-SQL only (beyond the product 15000m cap);
  same-sort ties on Search stay NOT_APPLICABLE (unique id
  keyset); distance ties are covered by identical-coordinate
  stations exact-once in RST-15, not re-seeded here.
- Faster KNN certifies nothing: verdicts, oracle sets and
  refusal errors are the acceptance facts; latencies are
  separate metrics. No fix coordinates persist anywhere.
- Reproduce: `go test -tags=integration -run NONE -bench
  'BenchmarkSpatial(Discovery|Eligibility)' -benchtime 1x
  ./internal/modules/directory/adapters/read/` (~30 s);
  correctness via `-run 'TestSpatial(OracleAgrees|
  EligibilityBoundary)'`.
- Next (smallest, separately authorized): RST-17 index vs
  partition comparison over the RST-15/16 baselines.

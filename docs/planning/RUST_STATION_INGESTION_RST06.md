# RST-06 — Location review gate

Status: IMPLEMENTED — staging only; no publication-policy change, no new
service, no new credentials. Date: 2026-10-09. Scope:
`RUST_STATION_INGESTION_PLAN.md` RST-06. Language: English document; user
communication in Portuguese.

## 1. What was added

- Migration `000050_registry_review_source.sql` (append-only): run
  source enum gains `review`. Official-lookup reads keep their current
  source scope until a tested publication change says otherwise.
- `registry/review.go` — `ReviewCandidate` promotes one staged
  candidate when a second independent sample corroborates it: same
  exact CNPJ, both from complete runs, both unknown quality with
  supported CRS, transformed agreement within 150 m and observation
  dates within 90 days. Every rule is checked before any write;
  rejection stages nothing. Promotion stages one reviewed assertion
  (transformed EPSG:4326 point, reviewer provenance) on a deterministic
  `review:<candidate>` snapshot, so replay converges and the existing
  complete-run reconciler projects it through the location chain.
- The CRS transform stays inside PostGIS/PROJ behind the
  `Transformer` port (`TransformPoint` query): Go maps CRS labels to
  SRIDs and refuses unknown systems before SQL; non-finite inputs fail
  at the same gate. Fakes pass 4326 through only; they never simulate
  PROJ.
- `Store` gains `GetAssertion` (assertion plus run state); no query
  text changed otherwise, sqlc regenerated with the pinned v1.31.1 and
  `sqlc diff` clean.

## 2. Evidence (revision `e257ee8` + working tree)

- Unit (fake): promotion stages the reviewed row with transformed
  coords; 149 m promotes while 151 m rejects without staging;
  mismatched CNPJ, incomplete runs, reviewed/coordinateless/foreign-CRS
  rows, undated or stale pairings, anonymous reviewers and self-pairing
  all reject with zero runs; replay converges; promotion wires into
  `ReconcileRun` (fake canon records the point).
- Real disposable PostGIS with `-race`: EPSG:4674→EPSG:4326 executes
  with axis order pinned; 4326 is identity; foreign CRS and NaN
  refuse. Measured over 5 sample points: datum shift max 0.0000 m
  (coincident datums in this PROJ build, not a national promise).
  Go corroboration math vs `ST_Distance`: 23950.5 m vs 23914.6 m
  (0.15%, spherical vs spheroid as expected).
- End to end with the real canonicalizer: load → promote → reconcile
  projects `current_quality = reviewed` with a 0.000 m gap to candidate
  evidence. Directory and community (photo-gate) unit suites stay
  green; the 150 m capture rule itself is untouched.
- `go build ./...`, `go vet`, `gofmt` clean. Local `staticcheck`
  predates the pinned toolchain (same as RST-05); CI owns that signal.

## 3. Limits and next task

- True parcel-grade proof and any calibrated 150 m threshold change
  stay future work with licensed references; this gate only admits
  corroborated evidence to the existing reviewed projection.
- Next (smallest): RST-07 benchmark/index experiment — synthetic
  census-shaped distribution plus 100k/1M stress, baseline vs index
  candidates vs optional read-projection partitions with identical
  semantics and output checksums.

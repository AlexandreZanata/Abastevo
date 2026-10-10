# RST-00 — Rust station-prep integration audit and minimal contract

Status: AUDIT_DONE — no implementation, no dependency, no fetch, no DB change.
Date: 2026-10-09. Scope: `RUST_STATION_INGESTION_PLAN.md` RST-00 only.
Language: English document; user communication in Portuguese.

## 1. Ownership boundary (verified)

- Directory owns stable station identity (`directory_stations` UUID),
  identifier history with partial unique index on active
  `(kind, normalized_value)`, append-only location revisions and the
  single reviewed projection (`current_point geography(Point, 4326)`,
  `current_quality`): `backend/db/migrations/000002_directory.sql`.
- Registry staging owns bounded runs and per-row assertions with
  idempotent replay key `(source, source_key, checksum)`; only
  `complete` runs reconcile: `000031_registry_sources.sql`,
  `backend/internal/modules/directory/adapters/registry/stage.go`,
  `reconcile.go`.
- API discovery is allowlisted to `revendedoresapi.anp.gov.br` with
  DNS/IP/redirect guards: `api.go`. Code presence is not proof of a
  public permitted nationwide API; validate docs, limits and a bounded
  sample before use.
- Coordinates are nullable staged evidence only (`000032`); CSV rows
  stay NULL (honest unknown, never centroid-fabricated). Only
  `reviewed` quality projects through the location-revision chain.
- Profiles/operator revisions reference `directory_stations`; no second
  station registry: `000036_station_profiles.sql`.
- Go remains API/domain/publication owner. Rust prepares assertions;
  it gets no general write access to canonical, community, auth, media
  or social tables. Narrow staging COPY is a later option.
- Photo capture gate is unchanged: `PhotoCaptureRadiusM = 150.0`,
  `SitePrecise` required, fix integrity enforced
  (`backend/internal/modules/community/application/photo_capture.go`).
  A station at a city boundary may be nearby across that boundary;
  city is never a proximity-security boundary.

## 2. Real ANP format gaps (no invented data)

Observed 13-column registry header (plan, 2026-10-08):

```text
CODIGOISIMP;AUTORIZACAO;DATAPUBLICACAO;RAZAOSOCIAL;CNPJ;ENDERECO;COMPLEMENTO;BAIRRO;CEP;UF;MUNICIPIO;BANDEIRA;DATAVINCULACAO
```

Required normalized contract (`registry/policy.go`):

```text
CNPJ, RAZAO_SOCIAL, COD_IBGE, UF, SITUACAO
```

Gaps:

1. No `COD_IBGE`: `MUNICIPIO` is a name, not an IBGE code. Map
   name + UF through versioned normalization plus explicit aliases
   against the IBGE Localidades reference; ambiguous/unmatched rows
   enter quarantine. RFB municipality codes are a separate namespace.
2. No `SITUACAO`: `AUTORIZACAO` is an authorization act reference and
   `DATAPUBLICACAO` is its publication date, not operation status or
   opening date. Dataset membership in an operating-retailer snapshot
   is evidence with a snapshot date; mapping it to authorization
   eligibility needs a tested Directory policy-owner rule. Missing
   rows never imply closure or revocation.
3. No coordinates, no IBGE code, no explicit status column. Keep
   combined `ENDERECO` unchanged alongside parsed components;
   road/km addresses need fixtures.
4. Feeding the real CSV directly into the current contract must
   quarantine with `missing_columns` (`ValidateHeader` /
   `QuarantineRun`). That is correct behavior, not a parser bug.

PMQC coordinates are candidates only: `CnpjPosto`, `DataColeta`,
`Latitude`, `Longitude`, declared SIRGAS 2000 / EPSG:4674, empty
coordinates permitted, no accuracy-in-metres field. Deduplicate by
sample identity/CNPJ/date/point; repeated assays are not independent
support. Official origin, decimal places and repeated rows do not
prove 150 m capture reliability. Do not auto-promote to `reviewed`
and do not use them in the 150 m rule without the location review
gate (RST-06): tested EPSG:4674 → EPSG:4326 transform, exact-CNPJ and
address/municipality/chronology checks, independent site/parcel
evidence where available, held-out stratified error measurement
(p50/p95/max in metres) before any calibrated threshold.

## 3. Minimal proposed contract (frozen in RST-01, not implemented here)

Pipeline:

```text
verified official download + manifest
  -> bounded Rust reader / source adapter (local file only)
  -> pure normalization / identity + municipality validation
  -> deduplication / coordinate candidates / quarantine
  -> deterministic versioned JSONL + manifest + counts
  -> Go-owned validation + staging loader
  -> existing complete-run reconciliation
  -> Directory + profile + reviewed-location projections
```

- Proposed crate `tools/station-prep/` starts only after RST-00
  approval. Suggested modules: `source`, `parse`, `normalize`,
  `identity`, `municipality`, `geo_candidate`, `output`, `manifest`,
  `cli`. Pure transforms stay independent of network/DB/runtime.
  No Rust HTTP API, Android JNI, microservice, parallel station
  database, or rewrite. No `Cargo.toml` or dependency added in RST-00
  (verified absent: no `tools/`, `rust/`, or root `Cargo.toml`).
- Streaming: bounded reader, reused buffers, bounded queues/batches,
  typed quarantine with source row locator; accepted + duplicate +
  quarantined counts always reconcile. Memory/speed claims require a
  measured dataset. Note: current Go `StageCSV` buffers the snapshot
  up to `MaxBytes`; Rust must be true streaming, not `ReadAll`.
- Determinism: canonical typed-value hashing (no whitespace-only
  churn), stable output order independent of input order, bounded
  in-memory map with documented cap or external sorted spill/merge,
  identical source + bytes + parser/policy version is a no-op, older
  evidence never overwrites fresher facts, conflicts retain both
  assertions plus a review reason, no last-row-wins.
- Provenance per batch: format version, run ID, source identity/URLs/
  edition, raw file hashes/bytes, parser/policy versions, start/end
  time, municipality-reference hash, counts, completeness, per-output
  hashes. Per row: stable source key, full normalized CNPJ text
  (numeric/alphanumeric checksum vectors, leading zeroes preserved),
  source SIMP/authorization references when supplied, raw/normalized
  business/address fields, UF + IBGE code, independently evidenced
  authorization/status facts, optional coordinate candidate
  `{latitude, longitude, original_crs, observed_at, accuracy_m: null,
  evidence_ref, review_state}`. Unknown timestamps stay null.
  Latitude/longitude keep original CRS until a tested geospatial
  owner transforms them.
- Three bounded output streams: accepted assertions, coordinate
  candidates, quarantine. JSONL first for inspection; CSV COPY export
  only after measurement. No Parquet/Arrow/Polars, no SQL string
  concatenation, no Tokio/reqwest/parallel pools/Rust PG client
  without measured need and failure/security tests.
- Publication atomicity limit (existing): reconciliation is
  complete-run gated but applies assertions individually; it is not
  an all-stations atomic snapshot switch. A versioned read projection
  plus active generation pointer would be a separate tested change.
- City/nearby reads start from existing indexes: municipality B-tree
  and GiST on `current_point` (000002). Candidate: composite B-tree
  on the measured predicate/order, e.g.
  `(state, municipality_code, status, id)` with keyset pagination by
  a stable tie-breaker; nearby via
  `ST_DWithin(current_point, query_geography, radius_metres)` plus
  exact-distance ordering on the reduced set. Partitioning only on
  RST-07 benchmarks across the workload including maintenance and
  uniqueness costs; otherwise keep the indexed baseline. Never one
  database/schema/table per city; never lose global active-CNPJ
  uniqueness or stable UUID foreign keys.

## 4. Unresolved decisions for RST-01

1. Operating-membership → authorization/eligibility mapping rule
   (Directory policy owner).
2. Encoding: UTF-8/BOM plus measured fallback, if needed.
3. Changed-header handling and frozen alias/version policy.
4. Municipality alias source version and update cadence.
5. CRS transform owner and operation/version record.
6. Dependency/license/MSRV pins with security review.
7. Partitioning deferred to RST-07 benchmarks.

## 5. Next task: RST-01 (smallest)

Freeze the synthetic source/batch contract: real-layout 13-column
registry fixtures, PMQC repeated-assay/optional-coordinate fixtures,
IBGE mapping fixtures, and versioned JSONL output manifest/schema.
Decide operating-membership evidence mapping, encoding,
changed-header handling and publication policy. No raw production
rows in Git; dependency/license/MSRV decision belongs to RST-01.

Acceptance: synthetic fixtures cover valid row, bad/leading-zero/
alphanumeric CNPJ, unknown city, accent/encoding/header change,
quoted separator/newline, extreme record size, duplicate assays,
zero/NaN/swapped/out-of-Brazil points, date reversal, conflicting
older status; manifest validates checksums/schema/counts; docs link
clean and `git diff --check` passes. No implementation, live DB
change, or national fetch.

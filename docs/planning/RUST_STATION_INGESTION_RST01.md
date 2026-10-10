# RST-01 — Source/batch contract with synthetic fixtures

Status: CONTRACT_FROZEN — no implementation, no dependency, no fetch,
no DB change. Date: 2026-10-09. Scope: `RUST_STATION_INGESTION_PLAN.md`
RST-01 only. This freezes the versioned source/batch interface that
RST-02 implements. Language: English document; user communication in
Portuguese.

## 1. Frozen decisions

1. Operating membership is evidence, not a grant. A row present in an
   operating-retailer snapshot yields `auth_state: unknown` and
   `eligibility: pending` with `auth_evidence` carrying the snapshot
   date. Mapping membership to `authorized`/`eligible` needs a tested
   Directory policy-owner rule; missing rows never imply closure.
2. Encoding is UTF-8 with BOM stripped when present; the effective
   encoding is recorded per input in the manifest. Invalid bytes
   quarantine the row (or the run when the header is unreadable).
   No silent fallback; a measured fallback needs its own evidence.
3. Headers match by name, never position, against the frozen raw set
   `CODIGOISIMP;AUTORIZACAO;DATAPUBLICACAO;RAZAOSOCIAL;CNPJ;ENDERECO;`
   `COMPLEMENTO;BAIRRO;CEP;UF;MUNICIPIO;BANDEIRA;DATAVINCULACAO`.
   Missing names or unknown columns quarantine the whole run
   (`header_mismatch`), mirroring Go `ValidateHeader`/`QuarantineRun`.
   Alias or version changes are explicit policy bumps with tests.
4. Municipality maps name + UF through Unicode NFKD normalization
   (strip diacritics, collapse whitespace, uppercase) plus an explicit
   versioned alias table against the live IBGE Localidades reference.
   Ambiguous or unmatched rows quarantine (`ambiguous_municipality` /
   `unknown_city`). RFB codes are a separate namespace. Combined
   `ENDERECO` is always preserved alongside parsed components.
5. PMQC points are `EPSG:4674` candidates with `accuracy_m: null` and
   `review_state: pending`. Deduplicate by sample identity/CNPJ/date/
   point; repeated assays are not independent support. Zero, swapped,
   out-of-Brazil and non-finite points quarantine and are never
   auto-corrected or auto-swapped. Stale points stay candidates with
   a staleness note. The EPSG:4674 → EPSG:4326 transform owner and
   operation record belong to RST-06; Rust keeps the original CRS.
6. Publication: Rust emits deterministic JSONL plus a versioned
   manifest only. Go validates manifest/checksums/schema, loads
   bounded staging batches and reuses existing complete-run
   reconciliation. Identical source + bytes + parser/policy version
   is a no-op; older evidence never overwrites fresher facts;
   conflicts retain both assertions with a review reason.
7. Dependencies/MSRV: no crate is created in RST-01 (verified absent:
   no `tools/`, `rust/` or root `Cargo.toml`). Proposed minimal pins
   for RST-02 review: `csv`, `serde`/`serde_json`, `sha2`, minimal CLI
   parser, exact versions plus lockfile after licence/security review.
   No Tokio/reqwest/parallel pools/PostgreSQL client/dataframe
   without measured need and failure tests.

## 2. Versioned schemas

- Batch manifest `station-batch-v1`: format version, run ID, inputs
  (key/ref/edition/sha256/bytes/rows/encoding), parser and policy
  versions, municipality reference (reference + hash pinned at run
  time), start/end time, per-input counts
  (input/accepted/duplicates/quarantined), completeness
  (EOF validated + expected manifest), per-output hashes/bytes/rows.
- Assertion `station-assertion-v1`: source, stable source key
  (normalized full CNPJ text, leading zeroes preserved), row
  checksum, SIMP/authorization references, raw and normalized
  business/address fields, UF + IBGE code, independently evidenced
  auth/eligibility facts, location quality with nullable coordinates,
  optional supersession link. Unknown timestamps stay null.
- Coordinate candidate `station-coordinate-candidate-v1`: source key,
  sample ref, observed date, latitude/longitude in the original CRS,
  `original_crs`, `accuracy_m: null`, evidence ref, review state,
  staleness note.
- Quarantine `station-quarantine-v1`: source, row locator
  (`file:logical-row` or `file:header`), typed reason, detail,
  nullable source key. Reasons: `invalid_cnpj`, `unknown_city`,
  `ambiguous_municipality`, `invalid_point`, `suspect_swap`,
  `out_of_brazil`, `date_reversal`, `header_mismatch`,
  `record_too_large`, `older_conflicting_evidence`.

## 3. Fixture inventory (`contracts/testdata/station-prep/`)

`manifest.json` lists every file. All rows are synthetic with owned
`[RST01-TEST]` markers; no production rows or personal data.

- `registry-13col-sample.csv`: 12 data rows under the real 13-column
  header. Covers valid, leading-zero (`00428184000195`) and
  alphanumeric (`12ABC34501DE35`) CNPJs, unknown city, quoted
  separator plus embedded newline, byte duplicate, short and
  bad-checksum CNPJs, date reversal (`DATAVINCULACAO` before
  `DATAPUBLICACAO`), older conflicting address for the same CNPJ,
  a 2000-char field and a case/accent alias row.
- `registry-13col-header-variant.csv`: renamed `RAZAO_SOCIAL` plus
  `EXTRA_COL`; the run must quarantine with `header_mismatch`.
- `pmqc-sample.csv`: 11 data rows. Covers duplicate assays, a second
  date/point, empty coordinates, zero point, swapped pair,
  out-of-Brazil, `NaN`, a stale 2020 observation, a second station
  and a bad CNPJ.
- `ibge-mapping-sample.json`: illustrative alias table with match,
  ambiguous and unmatched rules; never a census.
- `output-assertions-sample.jsonl` (2 rows) and
  `output-candidates-sample.jsonl` (2 rows): schema-pinning excerpts.
- `output-quarantine-sample.jsonl` (10 rows): one entry per
  quarantined sample row plus the variant header run.
- `batch-manifest-sample.json`: sample manifest carrying the real
  SHA256/bytes/rows of this directory.

Expected evaluation: registry 12 = 7 accepted (one as superseded
history) + 1 duplicate + 4 quarantined; PMQC 11 = 5 accepted
candidates + 1 duplicate + 5 quarantined.

## 4. Acceptance evidence

Validator (stdlib only, no new dependency): both CSVs parse with `;`
delimiter including the quoted separator/embedded newline; every
JSON/JSONL file parses; manifest input/output hashes, bytes and row
counts match the actual files; quarantine reasons use the typed set;
no file contains production-row markers, e-mails, phone patterns or
real coordinates. `git diff --check` passes. Existing Go registry
contract tests are unaffected (separate fixture directory).

## 5. Next task: RST-02 (smallest)

Implement the pure Rust streaming parser over local files only:
`;`-delimited reader with bounded buffers/batches, UTF-8/BOM
handling, frozen header matching, CNPJ/municipality validation and
typed quarantine with row locators. RED → GREEN against these
fixtures; no network or DB. Acceptance: byte-identical duplicate is
a no-op, reordered input yields identical semantics and stable
order, truncated input fails without partial output, and peak RSS
is measured on the sample (no speedup claim).

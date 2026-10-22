# MEX-20a — Actual enforcement workbook native reader

Status: LOCAL_DONE
Validation: PASS
Integration: INTEGRATION_PENDING

Parent: `0ff7010f` (maintained dev, PR #143 head). Date: 2026-10-22.
Depends MEX-17/18, accepted. Rules B-BR-MEX-10/11/12/16; BUC-MEX-07.
Risk: critical ingest/PII (source parsing, personal data exclusion).

Before behavior: the official enforcement workbook published from 2019 on is
read locally and offline, with no new dependency and no network. The pinned
profile is the source itself: one worksheet, the eleven frozen source headers
in their exact order, the source wording of every column and the workbook date
system as the only clock. A row stays a source record: distinct results that
share a document stay distinct rows, and no fine total, guilt, payment,
finality, current-company link or station binding is derived. The reader
minimizes what it emits — personal identifiers, agent names, neighbourhoods and
addresses never leave the reader, and only a valid CNPJ is kept as a subject.
A refusal is a refusal: an unknown profile, a corrupt archive, an expansion
bomb, a document type declaration, an unknown entity or a date that carries
more than a day produce no partial output. Recovery: the previous CSV
enforcement adapter is untouched and stays readable; a new workbook revision
is a new revision that needs a new acceptance, never an automatic replacement
of a verified profile.

## Delivered

- `tools/station-prep/src/deflate.rs`: raw DEFLATE (RFC 1951) decoder —
  stored, fixed and dynamic Huffman blocks, canonical Huffman decoding,
  back-reference bounded by the produced stream, produced output capped by the
  caller.
- `tools/station-prep/src/xml.rs`: bounded streaming XML pull parser — elements,
  attributes, character data, CDATA, comments and processing instructions;
  a document type declaration is refused instead of expanded, an unknown entity
  reference is refused instead of resolved, and depth, attribute, name and text
  budgets are enforced. Only UTF-8/US-ASCII parts are accepted.
- `tools/station-prep/src/zip.rs`: bounded ZIP reader — the central directory is
  the authority, the local header must repeat the same member name, the
  produced bytes are checked against the declared CRC32, and member count,
  expanded total, member size, name length and expansion ratio are refused
  before anything is expanded. ZIP64 and encrypted members are refused.
- `tools/station-prep/src/xlsx.rs`: spreadsheet reader — workbook date system
  (`date1904`) and first worksheet resolved through the workbook relationships,
  shared string table, and streaming rows whose cells are placed by their
  reference; a worksheet wider than the expected profile is refused.
- `tools/station-prep/src/enforcement_xlsx.rs`: the pinned enforcement profile —
  the eleven exact headers, the source wording, the calendar conversion of the
  source date serial (`DATA DO DF`) with the workbook date system, the subject
  shape (company-shaped / personal-shaped / other), CNPJ admission through the
  existing kernel-compatible check digits, and minimized records
  (`enforcement-source-record-v1`) with a content identity hash, a row locator
  and an exact-repeat flag.
- `tools/station-prep/src/bin/prepare_enforcement.rs`: local preparer — reads
  the workbook, writes private artifacts (directory `0700`, files `0600`):
  `enforcement-source-records.jsonl` and `manifest.json` with the input hash,
  the counts, the census and the artifact hash. Standard output carries counts
  only; a refusal removes the artifacts and prints a generic message.
- `tools/station-prep/tests/enforcement_xlsx.rs`: real source acceptance — the
  native census is compared field by field with the independent Python census
  (`scripts/audit-anp-source-census.py`, committed in MEX-17), and every emitted
  record is checked for day precision, kept source wording and the absence of a
  subject identifier that is not a company identifier.

## Actual checks

From this worktree (2026-10-22, parent `0ff7010f`, private workbook outside
Git at `/tmp/bfp-mex17-private/dados-fisc-a-partir-2019.xlsx`, 16,188,087
bytes, sha256 `952743f3fa2b83094ad2011c5a4a1bffc14222eabde1bb4ee3bf4816e2f7db40`):

- Independent census first, then the native reader:
  `python3 scripts/audit-anp-source-census.py --enforcement <workbook> --pmqc
  <pmqc zip> --out /tmp/mex20a-census.json` → PASS in 15.8 s, aggregates only.
- `ANPFUEL_TEST_ENFORCEMENT_XLSX_PATH=<workbook>
  ANPFUEL_TEST_ENFORCEMENT_CENSUS_PATH=/tmp/mex20a-census.json cargo test
  --locked --release --test enforcement_xlsx` → PASS (3.7 s). The two
  independent implementations agree on every aggregate: 244,247 rows;
  141,290 distinct documents; 180,661 distinct subject/date/document/procedure
  tuples; 495 distinct result labels; 116 maximum result characters; 32,822
  results beyond 64 characters; 0 non-numeric dates; `date1904=false`
  (`EXCEL_1900`, day precision, first date 2019-01-02, last date 2026-10-04);
  18 members; 127,781,352 expanded bytes; the 37 source segments with equal
  counts (190,282 `Revenda de Combustíveis`, 30,073 `Revenda de GLP`, 342 blank
  segment); subject shapes 242,948 company-shaped / 1,294 personal-shaped /
  5 other.
- `cargo test --locked` → PASS, 168 tests in 28 binaries, zero failure; 43 of
  them in the library (28 new: 5 DEFLATE, 6 XML, 4 ZIP, 4 spreadsheet, 8
  enforcement profile, 1 real source acceptance).
- Refusal and negative cases, all observed: truncated and invalid DEFLATE
  blocks, output beyond the budget, back-reference before the stream start,
  document type declaration, unknown entity reference, unsupported encoding,
  truncated part, nesting depth, attribute budget, corrupt central directory,
  missing member, duplicate member name, expansion budget, member checksum
  mismatch, header mismatch (quarantined run, no record emitted), date with a
  fractional part, worksheet wider than the profile, non-archive input.
- Personal-data exclusion measured on the produced artifacts: 401 distinct
  personal-shaped identifiers of the source appear **0 times** in
  `enforcement-source-records.jsonl`; the 40,530 distinct agent names and
  45,013 distinct addresses are not emitted — the emitted keys are exactly
  `document_date, document_number, exact_repeat, municipality, procedure,
  record_hash, result, row_locator, schema_version, segment, source,
  subject_cnpj, subject_kind, uf`; no agent name occurs in any emitted result or
  procedure text, and the only address-like coincidences are 1–3 character
  strings.
- `cargo fmt --check` PASS; `cargo clippy --all-targets --locked -- -D
  warnings` PASS (no warning); no new dependency, `Cargo.toml`/`Cargo.lock`
  unchanged.
- Measured run: `prepare_enforcement` writes 244,247 records in 4.8–5.1 s
  wall clock (three consecutive runs) with 67.8–68.1 MB peak resident set —
  the 121.8 MB worksheet part is streamed, not buffered; two consecutive runs
  produce byte-identical artifacts (records sha256
  `712dbe0fb516b7788283b800fd763e7e8cc2b97e3a9d32851cfcd37383cf2c74`, manifest
  hash `9a26ea88c9e9c186` over the reconciled counts and the census).

## Limits

- This is the native reader only. There is no Go runtime acceptance, no typed
  artifact admission, no canonical ID, no manifest/bytes bridge (MEX-20b), no
  append-only storage or migration (MEX-20c), no publication or frozen read
  (MEX-20d), no worker, discovery or static image (MEX-20e) and no whole-source
  acceptance (MEX-20f). The reader admits no source and activates nothing.
- The pinned profile is one worksheet with those eleven headers and a day
  serial date. A different header order, an extra column or a non-day date is
  refused (header mismatch quarantines the run; a wider worksheet and a
  fractional date are refused), not adapted. The reader is stricter than the
  census oracle on the date column: the oracle only counts text that is not
  numeric, while the reader refuses any value that is not an exact day serial;
  the pinned revision has neither, so both counts are 0.
- The date is the source `DATA DO DF` value converted with the workbook date
  system: a document date, not an occurrence, payment, due or finality date, and
  not an incident year.
- The subject is admitted only as a valid CNPJ; 1,299 rows of the pinned
  revision carry no company identifier and keep `subject_kind` so the exclusion
  stays visible. No source-global fact is bound to a current company or a
  proven historical site (B-BR-MEX-11 remains MEX-21).
- The private workbook, its rows, results, names, addresses and identifiers
  stay outside Git, logs and fixtures; only aggregates and the committed
  reader code are versioned.
- No source activation, no network, no database, no deploy, no device, no
  production measurement and no release acceptance is claimed.

Next: MEX-20b (verified typed enforcement artifacts and the Go bridge) still
needs its own authorization; MEX-20c–f, EE-04/05 and the MEX-21–23 chain keep
their own gates.

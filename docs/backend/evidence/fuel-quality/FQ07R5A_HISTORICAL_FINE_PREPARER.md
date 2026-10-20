# FQ-07R5A — Historical fine profiles without semantic invention

Status: LOCAL_DONE
Validation: PASS

Parent74051a36, maintained dev. B-BR-FQ-01/02/06/07/14/16;
BUC-FQ-03/04/06. Bounded native preparer prerequisite, not runtime activation.

Before behavior: actual listed resources differ from2025.2020/2021/2022 use
UTF-8 BOM and ten columns including the source-declared transit date;2020/2021
label the reference Auto de Infração rather than DUF. The2021 .xlsx href actually
serves UTF-8 BOM semicolon CSV, not an XLSX ZIP.2023 is CP850 with five preamble
records and currency-formatted amounts; R$- in the paid field is unknown, never
zero.2024 has UTF-8 BOM, currency/plain money and an unlabelled extra field with
34 SIM values: retain it without assigning meaning. Grouped2016–2019 is one
UTF-8 BOM CSV with a source extraction DAY and a four-record preamble, not four
independent annual snapshots. Preserve source monetary footnotes; never compute
balance, finality, incident/decision dates or historical station ownership.

Implement a separate versioned native history contract and private file-backed
CLI without changing2025/v1 identities. Preserve source period/clock precision,
reference label/value, nullable source transit and paid amount, raw money and
unlabelled field. Minimize company names/CPF. Every original post-header record
must reconcile to accepted, duplicate or redacted quarantine; source footer
metadata is explicitly classified and retained as safe public note annotations,
never counted as a fine. No company name/CPF or unreviewed footer content in process errors/stdout/artifacts.

Immediate acceptance: exact per-profile encoding/header/preamble, calendar/money/
unknown-paid/reference semantics, metadata containment, counts/census and native
CLI permissions/integrity/failure cleanup. Independently reconcile every record
of all six downloaded official historical resources with Python codec/calendar/
check-digit/money logic and pinned hashes. No new dependency. Go bridge, SQL,
worker image/discovery expansion, safe station links and Android remain subsequent
checkpoints; D and production/native-device acceptance remain held.


## Delivered and verified, 2026-10-19

A separate pure Rust sanction_history module and prepare_fine_history CLI support
six exact source profiles without changing2025/v1 serialization/identities. The
only shared2025 code edit makes the existing codec/calendar/money helpers visible
inside the crate; their behavior is unchanged. No dependency addition.

Historical observations retain the actual reference label/value, source-declared
transit date or NULL, due date, raw money, exact integer amount, nullable paid
amount and raw unlabelled field. Source period remains one2016–2019 group or the
actual annual report label. Source extraction preserves DAY2020-02-06 for the
group and MONTH for annual reports. None becomes an incident/decision date or a
historical station-ownership claim. Empty source department is preserved as
unknown, not invented or discarded.2023 R$- remains paid=NULL; raw formatting
survives. No balance/finality/guilt/purchase inference.

Four private hashed JSONL streams contain accepted observations, duplicate
observations, redacted quarantine and allowlisted public source-note annotations.
Every logical post-header record has one census locator in the first three
streams. The six metadata records are explicitly source_metadata quarantine
annotations (two public payment notes plus one blank separator in each2020/2021
source), never fine events. Only exact inspected public note strings with every
other cell empty may be annotated. Other footer content stays redacted. Row/field
budgets include metadata; no source name or CPF appears in emitted rows/errors.

CLI bounds source bytes/rows/fields and aggregate output bytes100MiB, creates
mode700 output/mode600 files, syncs outputs and atomically renames the manifest.
Failures remove owned partial artifacts; an existing output directory is refused
and preserved. Counts-only stdout and static errors contain no source/path/body.
CSV preflight rejects an unfinished quoted field even when LF terminates the file,
while proper quoted multiline discarded names remain one logical record.

## Independent whole-source reconciliation

Python's separate codec/calendar/check-digit/exact-money logic compares every
retained field, nullable value, minimized content identity, original locator,
metadata note, all four stream hashes/bytes and the complete logical manifest.
Pinned official resource hashes (private files never committed):

- 2016–2019:2,301,471bytes,
  15ccbf54367881946104dc6baf65c246b6189638cb306ab06662570451989f25;
  16,440input=16,290accepted+0duplicate+150CPF quarantine; no metadata.
- 2020:304,268bytes,
  aef7fce874b29db234ab7779019c0bc9fcf13d1bdb2de7d59bbf09f51e32654f;
  2,026input=2,001accepted+0duplicate+25quarantine (22CPF+3metadata).
- 2021:720,842bytes, published multas-2021-junho22.xlsx is actually UTF-8 BOM CSV,
  85122d0d1ae999173bac2c772ffb4e14ec414aec3c2f8e30bf7203187936a92d;
  4,976input=4,929accepted+0duplicate+47quarantine (44CPF+3metadata).
- 2022:436,282bytes,
  5ee3df1d9188f273fbf3fbeb7e95f2cea0bc2171fdda23bc4af3cce9478251b5;
  2,996input=2,970accepted+0duplicate+26CPF quarantine.
- 2023:445,861bytes,
  3865025eb04a82ee5d937d703f063d617843eb7a9f5cbf4c18f989caa33839bb;
  3,074input=3,053accepted+0duplicate+21CPF quarantine;
  1,303accepted observations preserve unknown paid=NULL.
- 2024:416,653bytes,
  0723561b182ac0cc3d031a77050049844e3b29ec098472383b0229051cb67dcd;
  3,004input=2,994accepted+0duplicate+10CPF quarantine;
  all34unlabelled SIM values retained without interpretation.

Total32,516post-header records=32,237accepted+0duplicate+279quarantine,
including273CPF and6metadata; actual event denominator is32,510. Whole-source
oracle PASS for all six final source/artifact pairs
(`/tmp/fq07r5a-final-source.log`). No exhaustive runtime/database/public history
claim follows from native preparation.

The independent oracle first failed the initial group/2023 counts:81+78 valid
company observations had an empty department. A focused RED case proved the
unjustified requirement; preserving unknown department fixed all159 without
changing any source value. Another RED case proved csv crate accepted a truncated
quoted field at EOF even with LF; bounded quote framing now refuses it atomically.

## Validation

```sh
cargo +1.99.0 test --offline --locked --manifest-path tools/station-prep/Cargo.toml
cargo +1.99.0 clippy --offline --locked --manifest-path tools/station-prep/Cargo.toml --all-targets -- -D warnings
cargo +1.99.0 fmt --manifest-path tools/station-prep/Cargo.toml -- --check
python3 tools/station-prep/tests/support/fine_history_oracle.py /private/source/file /private/prepared/directory SOURCE_PERIOD
```

Full Rust131 cases PASS, including10 new contract/CLI cases; existing2025,
registry/PMQC/enforcement/sanction regressions PASS
(`/tmp/fq07r5a-full-rust.log`). New cases cover each source profile, actual
reference labels, nullable transit/paid, DAY versus MONTH, empty department,
private-name/CPF/control minimization, original-row census, duplicate identities,
metadata injection/field budgets/footer ordering, exact name/encoding/header/year,
calendar/money/row/byte/frame failure, private CLI artifacts, complete stream hashes,
redacted errors and refusal to overwrite owned existing output. Clippy/fmt PASS;
tracked-secret scan and owned new-file/link/diff review PASS. No SQL/API/Go/runtime,
new dependency, private source fixture or production data was committed.

## Recovery and next

2025 native worker remains the prior tested path. The new history CLI is not
shipped/registered with the worker and cannot enter legacy SanctionInput or the
2025 FineBatch loader by accident: it has a separate schema/streams/clock contract.
Keep imports/discovery off. Next FQ-07R5B verifies this typed Rust→Go boundary and
original locator/metadata census before compatible additive storage/publication/
read contracts (nullable paid, actual reference label, group period and DAY clock),
then bounded history worker/discovery/image integration and safe Directory links.
Live FQ-06 XLSX, linked Android journeys, FQ-16/17 acceptance, protected delivery
and the user's final overview remain owed. D stays held; no deployment/merge or
native-device/production acceptance is claimed.

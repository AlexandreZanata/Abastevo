# FQ-07R1 — Verified official fine CSV preparation

Status: LOCAL_DONE
Validation: PASS

Parent06190644, maintained dev. B-BR-FQ-01/02/06/07/14/16,
BUC-FQ-03/04/06. No runtime import, publication, station association or legal
classification is authorized merely by this preparer checkpoint.

## Frozen behavior before code

The synthetic sanction-v1 shape does not match the official resource. The
2025 CSV has six preamble records, nine columns and CP850 bytes. Its ungrouped
amounts can exceed three digits, DUF is not a unique fine identifier, and
Vencimento GRM is a due date, never a decision date. The source supplies report
extraction month precision only, not an exact extraction day. Preserve that
precision; never manufacture decided_on/as_of days or a native MULTAID.

Add a separate native official-input contract with no dependencies. Strictly
validate the fixed preamble/header, resource year, CP850 byte decoding, row/field/
byte budgets and final framing. Parse real calendar due dates and exact unsigned
money in minor units, keeping source amount/payment text and verbatim procedural
status. Do not infer payment balance, finality, guilt or current legal state.

Retain full validated company CNPJ, proceeding and DUF separately; omit names,
addresses and every CPF/invalid-CNPJ row. Quarantine metadata is redacted and
never contains the rejected subject or raw record. Multiple source observations
for one proceeding/DUF remain distinct. Content-derived observation keys are
explicitly not source-issued fine IDs. Exact minimized observations deduplicate
with reconciled counts; no amount is summed. Emission uses a new versioned
artifact contract so the old ledger cannot silently mislabel due/extraction
fields. Private source files stay outside Git.

Accept tests before implementation, actual2025 source reconciliation, Rust
format/clippy/build and bounded CLI failure/no output on refusal. Go live bridge,
append-only compatible ledger fields, import discovery, station links and public
journey remain subsequent tasks; D remains held.

## Official source inspection, 2026-10-19

Primary listing: [ANP fines](https://www.gov.br/anp/pt-br/centrais-de-conteudo/dados-abertos/multas-aplicadas-com-vencimento-a-partir-de-2016).
Actual resource: [2025 CSV](https://www.gov.br/anp/pt-br/centrais-de-conteudo/dados-abertos/arquivos/mav/multas-aplicadas-2025.csv),
450,504 bytes, SHA256
`277f6cf5c59716581c42c5068fde47827c28eb693ee643c75d7841bda191fe96`.
Headers, in order: Status Processo; Superintendência; Número do Processo;
Número do DUF; CNPJ/CPF; Razão Social; Vencimento GRM; Valor da Multa;
Valor Total Pago. The source preamble names SICOM, report2025, extraction
janeiro/2026. No exact extraction day is available. All3,395 data records have
nine columns;3,375 have valid full company CNPJ,20 carry CPF. Observed3,386
proceeding refs and3,391 DUF refs across all input records demonstrate that
neither alone is a unique observation key. No names/CPF/raw rows are committed.

Separate FQ-06 prerequisite inspection: the actual [enforcement XLSX](https://www.gov.br/anp/pt-br/centrais-de-conteudo/paineis-dinamicos-da-anp/arquivos-dados-brutos-do-painel-dinamico-da-fiscalizacao-do-abastecimento-da-sfi/dados-fisc-a-partir-2019.xlsx)
is16,005,464 bytes, SHA256
`df7dae068fd5ee1e4f250e7e827b49187b6e029cec8c370f29e636d49eb91a69`.
18 ZIP members,126,263,572 expanded bytes; sheet v_dados_abertos_dfs,
11 headers: UF; Município; Bairro; ENDEREÇO; CNPJ/CPF; Agente Econômico;
Segmento Fiscalizado; DATA DO DF; Número do Documento;
Procedimento de Fiscalização; Resultado.241,306 data records;240,015
14-digit subject shapes (check digits not certified here),1,286 CPF-shaped and5
other subject shapes. Dates and document numbers use numeric Excel cells.
139,585 distinct document values and178,497 subject/date/document/procedure
combinations cannot represent241,306 observations without losing results.
The result vocabulary also includes long source text beyond the current64-char
ledger limit. Thus neither synthetic enforcement-v1 nor the old ledger is a
safe live format. ZIP/XML budgets, date-system evidence, result-level identity,
privacy, longer verbatim labels and absence of an extraction clock still need
FQ-06 runtime work. This inspection does not certify all historical resources.

## Implementation and validation

Added `sanction_anp` with strict native CP850/preamble/header/calendar/money
contracts and redacted quarantine; new `prepare_sanctions` CLI emits private
JSONL and a file-hashed `sanctions-prepared-v1` manifest. The immutable observation
key is explicitly content-derived, not a source-issued fine ID. Report year and
extraction month remain in the manifest even for an empty input. Outputs retain
source labels/amounts/paid amounts/DUF/proceeding/due day separately; no subtraction,
summation, guessed decision day or legal promotion. Existing synthetic parsers,
PMQC and canonical Go writers are unchanged. No dependency was added.

RED missing native contract first, then eight native contract/CLI cases PASS:
real calendar/leap day, maximum int64 money/overflow, grouped/ungrouped amounts,
wrong year/header/encoding/framing, row/byte/field budgets, unknown verbatim status,
repeated proceeding/DUF observations, exact minimized duplicates, CPF redaction,
private artifact modes/hash/count integrity, existing-directory preservation
and redacted refusal with no output. The existing complete Rust suite passed
before the final isolated minimization correction; all eight affected cases
and all-target clippy passed afterward. Format/diff/secret/link checks PASS.

The independent Python stdlib oracle found two valid company records rejected
because a discarded company-name field contained control characters. A new RED
case reproduced this; GREEN checks controls only in retained fields, preserving
the safe minimized observation. The discarded name cannot affect trust or output.
The checked private source now reconciles exactly with the independent codec,
calendar/check-digit/money oracle:3,395=3,375 accepted+0 duplicates+20 quarantined
CPF records; every retained typed field matches, all rejected locators match,
all output hashes/bytes match. Extraction precision remains2026-01. Neither
these observations nor paid text constitute distinct-fine or balance totals.

Reproduce (private source downloaded separately; do not add it to Git):

```sh
cargo +1.99.0 test --offline --locked --manifest-path tools/station-prep/Cargo.toml
cargo +1.99.0 clippy --offline --locked --manifest-path tools/station-prep/Cargo.toml --all-targets -- -D warnings
cargo +1.99.0 fmt --manifest-path tools/station-prep/Cargo.toml --check
tools/station-prep/target/debug/prepare_sanctions /private/fines-2025.csv 2025 /private/new-prepared
python3 tools/station-prep/tests/support/fine_source_oracle.py /private/fines-2025.csv /private/new-prepared
```

The oracle pins this exact2025 resource/hash/denominator; drift requires a new
explicit evidence review, not changing assertions to match whatever arrives.
No source file or private prepared output is tracked. No Go DB/schema/import,
worker image, UI, source activation, native device run, VPS or merge in this
checkpoint. Next: typed Go artifact validation and append-only compatible ledger
fields/clock precision, then live discovery/composition and safe station links.
Disable/omit the new preparer to roll back source intake; no published data has
changed. D remains held; full FQ-06/07/14/17 acceptance remains OWED.

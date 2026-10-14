# FQ-01 — Product, module and batch contracts

Status: LOCAL_DONE / INTEGRATION_PENDING. Date: 2026-10-14.
Base: `codex/fuel-quality-plan` atop `origin/dev 08a54dc4` (community batch
merged; planning patch reconciled by ordinary merge, both PROGRESS histories
retained). Depends: FQ-00. B-BR-FQ-01–16 adopted; BUC-FQ-01–09 contracted.
Numeric score policy remains FQ-12; no score, migration, API or Android in
this task.

## Behavior before implementation

No quality package existed: the five-column coordinate parser quarantines any
assay header as `header_mismatch`, and no sample/assay identity, fuel mapping,
manifest shape or correction rule was frozen. This task adds the pure
contract layer only; importers, ledger, discovery and reads stay with
FQ-02/FQ-04/FQ-05/FQ-08.

## Delivered

- `backend/internal/modules/quality/domain/` (new, stdlib-only):
  `doc.go` ownership, `contracts.go` rules, `contracts_test.go` synthetic
  contract cases. No I/O, SQL, network or goroutines.
- Source streams frozen: `pmqc-assay-v1`, `enforcement-v1`, `sanction-v1`.
  The coordinate extract stays a separate RST-owned stream.
- Semantic identifiers: `SampleID{stream, native-ref}` groups one collection
  event; `Assay{sample, assay-ref, result-text, unit, status}` groups each
  determination beneath it. Same-day rows with different native refs stay
  distinct; exact content repeats dedupe with a duplicate count.
- Fuel vocabulary: closed v1 enum reusing the kernel Product wire values by
  exact string parity (pinned in test). Unknown source labels fail in the
  kernel so importers quarantine with `unmapped_fuel`, never coerced.
  `GASOLINA PREMIUM` stays unmapped until FQ-02 verifies actual assay-side
  product text — recorded, not guessed.
- Conformity vocabulary: CONFORME→CONFORMING, NÃO/NAO CONFORME→NONCONFORMING,
  INCONCLUSIVO→INCONCLUSIVE (case/accent-insensitive); anything else is an
  explicit UNKNOWN with a recorded reason, never conforming.
- Manifest: source/stream/edition, resource URL+SHA-256+bytes list,
  predecessor and counts. `Reconcile` refuses empty resources and any
  `input != accepted + duplicates + quarantined` mismatch before publication.
- Corrections append one revision with a reason over a known supersession;
  originals are never rewritten. Colliding active links (same record key)
  both fall to `link_collision` review; undisputed links survive. Inputs are
  never mutated by grouping/review.
- Golden fixtures `contracts/testdata/quality/manifest-valid.json` and
  `manifest-invalid-counts.json` follow the `id/version/schema/valid/`
  provenance pattern; the domain suite loads and enforces them.

## Ownership (no new ADR)

The proposed `quality` bounded context is adopted as a module inside the
existing Go modular monolith: `quality/domain` (this task, pure),
`quality/application` + `quality/adapters` with sqlc-owned SQL (FQ-04),
Directory identity reader, existing scheduler/leases/quotas and guarded
download (FQ-05), existing caller/account proof, Evidence document decision,
Moderation, Privacy and transactional Jobs ports. No new service, table
write outside the module, tenant model or cross-module nested commits. No
durable architecture/policy decision beyond reuse was needed, so no ADR was
recorded; the plan's proposal is promoted to owned contract by this file.

## Source-version mapping (from FQ-00, frozen shapes)

- PMQC: annual editions 2016–2025; monthly 2026 `01,03,04,05,06,07,08,09`
  with `07` duplicated (February label → July file, `02` absent). Importers
  resolve URLs, byte hashes and internal period values per edition and retain
  the discrepancy; a label never proves coverage.
- Enforcement: monthly series with ~2-month delay; visible labels 1998–2018
  and 2019–2025. Actual coverage/identifier fields need FQ-06 inspection.
- Sanctions: annual extraction snapshots through 2025; status is
  extraction-time only. Appeal/finality need FQ-07 source proof.
- Retailer registry (current, updated 2026-10-14) supplies full-establishment
  CNPJ/UUID resolution via Directory; it proves neither historical identity
  nor national sampling.

## Validation

RED→GREEN: first run failed twice — lowercase-accented `Não Conforme`
normalized to `NÃO CONFORME` instead of `NAO CONFORME` (fold ran before
uppercasing; fixed to uppercase-first), and grouping keyed samples by
sample+assay instead of sample (fixed to sample-key buckets). Both were
contract bugs, not test adjustments; rerun GREEN.

- `go test ./internal/modules/quality/...`: 13/13 PASS (stdlib-only,
  kernel parity, conformity, verified-column grouping, repeated assays,
  exact-duplicate dedupe, same-day separation, missing identifiers,
  loss accounting, append-only correction, link collision, 2 manifest
  fixtures, manifest refusals).
- `go vet ./internal/modules/quality/...`, `gofmt -l` clean;
  `go test ./internal/modules/kernel/` PASS (vocabulary untouched).
- `git diff --check` clean; new-file Markdown links resolve;
  scoped secret/PII scan clean (no credentials, CPF, precise GPS, media).
- No ZIP downloaded, no migration, no OpenAPI change (`contracts/openapi`
  untouched; `apicontract` suite unaffected — it globs `testdata/api/` only).

## Recovery

Revise these unpublished contracts without touching the coordinate stream:
delete or supersede `quality/domain` and its fixtures; existing PMQC/location
behavior is independent. Unavailable metadata blocks only its FQ-02 adapter
fields. Next: FQ-02 lossless Rust preparation.

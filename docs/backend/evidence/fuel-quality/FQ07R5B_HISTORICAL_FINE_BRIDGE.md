# FQ-07R5B — Verified typed historical fine boundary

Status: LOCAL_DONE
Validation: PASS

Parent06526993, maintained dev. B-BR-FQ-01/02/06/07/14/16;
BUC-FQ-03/04/06. One typed native boundary prerequisite; no database/runtime.

Before behavior: verify the separate historical native manifest/profile, exact
period/name/header/clock contract, source identity and all four bounded stream
hashes/bytes/counts before returning any typed batch. Preserve nullable paid and
transit, exact reference label/raw money/unlabelled facts, DAY/MONTH and group
identity. Reconstruct every minimized content hash, canonical JSON, sorted unique
accepted stream, duplicate twins and complete original record census. Notes must
be exact safe public annotations of source_metadata quarantine locators, with no
missing/repeated/misclassified annotations. No names/CPF, partial batch or raw
port error on failure. Cancellation/close/budget/tamper failures are mandatory.

Independent application validation must refuse impossible/mismatched source
semantics while preserving unknown department/paid and actual clocks. The type
cannot accidentally enter2025 FineBatch or legacy SanctionInput. Reconcile actual
Rust CLI→Go→application for every row of the six pinned official files after the
independent Python source oracle. No new dependency, SQL/API, image/worker/source
activation, station binding, D or production/native-device acceptance.


## Delivered and verified, 2026-10-19

Application owns explicit frozen historical profile/clock/observation/note types.
They preserve the six original source profiles, actual reference label, nullable
source transit/paid, raw monetary text, unknown department, unlabelled2024 fact
and group DAY versus annual MONTH. Pure validation checks full company CNPJ,
calendar/exact integer money and source-specific semantics; content identity
reconstructs the exact native minimized hash. No float, invented paid zero,
reference relabel, exact extraction day or case/ownership verdict is admitted.

LoadFineHistory verifies the separate manifest before opening any stream: exact
schema/profile/period/source filename/header/clock, expected source byte/hash,
nonnegative bounded complete counts and exact four-stream set with aggregate
byte budget. Each stream is hash/byte/row/UTF-8/canonical-JSON checked. Accepted
observations are sorted/unique and canonical; duplicates require the exact twin
apart from locator. Original post-header locators form one complete unique census.
Known source_metadata quarantines require exactly one allowlisted public-note
annotation each; other quarantines are strictly redacted. Valid observations may
not occur after the first source footer. Notes never add event observations.
Every failure, including late read/close/cancellation, returns a zero batch.

An adversarial port case found a shared scan resource leak when open returned both
an owned stream and an error. A RED test proved it; scan now closes that returned
stream before emitting its static error. Existing native2025 and PMQC prepared
regressions remain green. Another resealed tamper case placed a valid source row
after a metadata footer; Go now enforces the same original-record footer ordering
as Rust. Both failure paths preserve no partial facts.

Native historical retained text now quarantines U+2028/U+2029, which Go's canonical
JSON encoder escapes differently from Rust's. A new RED→GREEN Rust case freezes
this interoperability boundary without changing any of the32,516 real source
records or2025/v1 behavior. Discarded names remain excluded from observation
identity and are not accidentally rejected for irrelevant control characters.

## Validation

- Pure application: all six profiles, unknown department/paid versus actual zero,
  full CNPJ, exact reference label, group/day versus annual/month, calendar,
  nullable transit, overflow/money mismatch, extra unlabelled field, invalid source
  schema/period and control/separator rejection PASS.
- Typed loader: complete accepted/duplicate/quarantine/metadata census PASS;
  30 resealed/raw tamper cases refuse without partial facts, including unknown/
  repeated/missing JSON keys, CPF, dates/money, source identity, wrong profile/
  header/clock, order/twins/locator holes, private/missing/repeated/misclassified
  notes, negative/missing counts, stream/path/hash/canonical framing and digest.
- Six invalid manifest/input/budget cases open no streams. Cancelled-before-open,
  nil/error/returned-stream ports, read/close failures after observations were
  already parsed, and cancellation during the final stream close all return no
  typed facts or private error body. Regression confirms returned stream closed.
- `go test -race -tags=integration ./internal/modules/quality/adapters/prepared ./internal/modules/quality/application -count=1` PASS with the documented trusted PMQC CLI; existing Rust→PMQC bridge remains accepted. Final unit race log
  `/tmp/fq07r5b-final-critical.log` prepared1.088s/application1.014s; integration
  result recorded in `/tmp/fq07r5b-final-integration.log`.
- Actual newly executed history Rust CLI→Go→application across all six pinned
  private official resources PASS15.590s under race, after all final behavior
  edits (`/tmp/fq07r5b-final-source.log`). All32,516records reconcile exactly to
  32,237accepted+0duplicate+273CPF+6metadata. All1,303unknown paid,159unknown
  departments,34unlabelled SIM values, reference labels, clocks and public notes
  survive; every accepted observation independently passes application identity/
  money/calendar semantics. Original Python whole-source oracle evidence is
  [FQ-07R5A](FQ07R5A_HISTORICAL_FINE_PREPARER.md).
- Full Rust132 cases PASS, including the new canonical Unicode case; clippy
  all-targets/-Dwarnings and fmt PASS. Scoped Go vet/staticcheck and API/worker
  compilation PASS. No dependency/SQL/API change. Owned new-file/diff/secret/link
  review PASS; private source or prepared observation files remain outside Git.

```sh
# backend/, explicit manual source acceptance only; no network or database
ANPFUEL_TEST_FINE_HISTORY_PREPARER_PATH=/absolute/trusted/prepare_fine_history ANPFUEL_TEST_FINE_HISTORY_SOURCE_DIR=/private/pinned/source/directory go test -race -tags='integration qualitysource' ./internal/modules/quality/adapters/prepared -run '^TestOfficialHistoricalFineRustGoBoundary$' -count=1 -v
```

## Recovery and next

The historical type/manifest cannot enter the2025 native loader or legacy publisher
accidentally. This boundary has no filesystem/network/database write capability;
callers still must pin trusted native binary and original source provenance. Keep
historical sources disabled until their compatible additive ledger/publication/
reader and worker integration is accepted.2025 default-off worker behavior is
unchanged apart from correctly closing a failed artifact stream.

Next FQ-07R5C: append-only compatible storage with immediate real-PostGIS upgrade,
negative money/calendar/clock/nullable/reference/source-context, concurrency and
staging-lock tests. Then atomic historical publication/leases/read contracts,
source runner/discovery/image integration, safe Directory associations, live FQ-06,
Android/FQ-16/17 and final protected delivery/overview. D remains held; no source
activation, deployment, PR/merge or production/native-device certification.

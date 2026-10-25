# ESP-11 — Publish safely associated official-record pages

Status: LOCAL_DONE (event-level) / INTEGRATION_PENDING
Validation: PASS
Date: 2026-10-24
Branch: `codex/enterprise-static-portal`
Parent: merged FQ-PORTAL-READY `4be5522c`; FQ code `424d71b7`, `9795a36e`.

Rules: B-BR-ESP-04/07–09/14/16; BUC-ESP-05.
[Plan](../../../planning/ENTERPRISE_DATA_STATIC_PORTAL_PLAN.md#esp-11--publish-safely-associated-official-record-pages).
[FQ checkpoint](../fuel-quality/FQ-PORTAL-READY.md).

## Scope delivered

The FQ-PORTAL-READY export port was integrated without editing the FQ producers
beyond the no-published sentinel and an opt-in test snapshot dump. ESP-11 now
renders the accepted FQ-07/FQ-08 fine edition at **event-level**:

- FQ-07 real seven-file acceptance: 35,911 input = 35,612 accepted + 293 CPF
  quarantines + 6 metadata; 35,612 explicit `unmatched` company decisions in the
  empty acceptance Directory; zero identity writes and zero invented links.
  The real run was 81.50 s on disposable PostGIS.
- The 2025 edition used for the portal snapshot was 3,375 records, revision
  `5d384d0d-9508-45f7-9d3a-200cff093673`, lease
  `a18283c151938ddbb56574ddbd897629cda050c887fefbc8ce81e0dd6c150a88`, source
  `multas-aplicadas-2025.csv` SHA-256
  `277f6cf5c59716581c42c5068fde47827c28eb693ee643c75d7841bda191fe96`.
  Its exported JSONL payload SHA-256 was
  `8569b61d827fe58c01ff36f3e131d7be460a832ed3161b7658d2c3500c9a2b06`.
  Revision/lease/payload IDs are run-local; the records and counts are the
  accepted data.
- `QualityOfficialProvider` maps `ExportPin`/`BuildOfficialExport` into the
  durable builder; the builder writes `official-records.jsonl` and
  `official-records-manifest.json` into the candidate and records the FQ pin in
  the state. `Reconcile` detects a changed FQ revision/lease; the dedupe key
  includes the official pin. A no-published FQ stream leaves the gated page.
- `/fiscalizacao/` now renders the frozen snapshot: an index with total/linked/
  unmatched/ambiguous census, 200-record pagination, stable detail pages,
  source/lease/checksum/period/clock and strict “fine is not proof of
  adulteration, guilt or definitive status; absence is not a clean certificate”
  wording. Only `linked` records show a station link; `unmatched` and
  `ambiguous` never do. Linked records appear in the station page’s “Registros
  oficiais vinculados” section with the decision-day limitation.
- The page never relabels a due date as occurrence/ownership; `occurred_on` is
  empty for fines. No CPF, internal notes, private documents, score or risk
  ranking is exported.

## Tests and actual results

FQ real source/pipeline (bounded local fixtures under `/tmp`, no ANP campaign):

```sh
ANPFUEL_TEST_FINE_HISTORY_SOURCE_DIR=/tmp/fq-official-20261024 \
ANPFUEL_TEST_SANCTIONS_PREPARER_PATH=/tmp/fq-native-rust-20261024/debug/prepare_sanctions \
ANPFUEL_TEST_FINE_HISTORY_PREPARER_PATH=/tmp/fq-native-rust-20261024/debug/prepare_fine_history \
ANPFUEL_TEST_DATABASE_URL=... \
go test -race -count=1 -tags=integration,qualitysource ./cmd/worker \
  -run TestOfficialSevenFileWorkerPipeline -v
```

PASS 81.50 s, all seven editions; 35,911=35,612+293+6; 35,612 unmatched and
zero identity writes. The opt-in `ANPFUEL_TEST_EXPORT_DIR` dump produced the
2025 `official-records-2025.jsonl` and manifest under `/tmp/fq-portal-exports`
only; nothing raw was committed.

Quality matrix:

```sh
go test -race -count=1 -tags=integration ./internal/modules/quality/...
```

PASS: adapters 91.7 s, HTTP 62.5 s, jobs 33.6 s, native/prepared/read/application/
domain all green; the export read tests include cursor/sealed-revision refusal,
withdrawn/superseded revisions, linked/unmatched/ambiguous mapping, wrong branch,
succession review, closed storage and privacy.

ESP/portal:

```sh
go test -race -count=1 ./internal/modules/enterpriseexport/...
go test -race -count=1 -tags=integration ./internal/modules/enterpriseexport/adapters/jobs
ANPFUEL_TEST_REAL_OFFICIAL_DIR=/tmp/fq-portal-exports \
  go test -count=1 -v ./internal/modules/enterpriseexport/portalgen \
  -run TestRealOfficialSnapshotPages
```

All PASS. The synthetic portal fixture proves linked/unmatched/ambiguous
rendering, the linked station section, 21 pages/31 files, exact association
wording and no station link for unmatched/ambiguous. The real 2025 snapshot
generated **3,409 pages / 3,419 files / 15,201,059 bytes** with content
`sha256:b3a01f816eaed977d9c0006dd5a7d853563b2f7844846a1c6fc54cd74ccce20a`;
all 3,375 records are event-level `unmatched`, all absolute links resolve and a
real record page shows its CNPJ/source limits. The fake-provider builder test
proves official files/state, no-op on unchanged pin and a new candidate on a
pin change; the provider wrapper test proves the no-published path.

Static: `go build ./...`, affected `go vet`, pinned `staticcheck`, `sqlc vet`,
byte-stable `sqlc generate`, `gofmt` and `git diff --check` PASS.

## Limitations and OWED

- FQ-06 enforcement is still blocked (no real XLSX binding/live Go stream).
- The real accepted 2025 edition is all `unmatched` because its acceptance
  Directory was empty; real `linked`/`ambiguous` portal rendering is proved by
  synthetic PostGIS/portal tests, not by a live production Directory.
- Fine association live revalidation/rebound after a later Directory change is
  OWED (FQ-07R6D). A linked record means “current company at the pinned decision
  day”, not live or historical ownership.
- The FQ export pin is one current stream revision (2025 in this run). Historical
  2016–2024 editions are accepted by FQ but are not combined into one portal
  snapshot; a multi-edition FQ export is OWED. The real test exported all seven
  separately for acceptance.
- The builder’s real FQ+price same-DB run was not executed here; the provider/
  state is tested with a fake and the real FQ export file feeds the real portal
  snapshot. ESP-13/14 will run the builder against the FQ-populated deployment
  DB; no VPS/publication was performed.
- Real price publication remains blocked by ESP-07 over-precision and rights.
  Event-level official records can be published independently once ESP-13/14/18
  authorize the release; no such authorization was exercised.
- Next: ESP-13 — verify the combined candidate artifacts (prices/stations,
  search shards, insights/downloads and official-record pages), then ESP-14.

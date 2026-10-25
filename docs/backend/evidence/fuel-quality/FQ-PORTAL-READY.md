# FQ-PORTAL-READY — Frozen event-level fine export for ESP-11

Status: LOCAL_DONE
Validation: PASS
Integration: INTEGRATION_PENDING
Date: 2026-10-24
Branch: `codex/fq-portal-ready`
Worktree: `.worktrees/fq-portal-ready`
Parent: `origin/dev 721582b8`
Code commits: `424d71b7`, `9795a36e`

## Scope and baseline

This checkpoint unblocks the **event-level** ESP-11 slice without implementing
the static portal. It accepts the seven FQ-07 fine resources already verified by
[FQ-07R5E](FQ07R5E_HISTORICAL_FINE_WORKER.md), reads the published revision
through the existing FQ-08 lease, and adds a read-only frozen export port for the
static builder.

FQ-06 was not included: its raw ANP XLSX columns are still unbound to the
adapter and no live Go enforcement stream exists. The source-admission record
therefore keeps that slice blocked. Nothing here claims FQ-06, FQ-17, score,
guilt, finality, current interdiction, FQ-16 or Android acceptance.

The primary checkout, the map worktree (`/data/dev/worktrees/abastevo-map-ev`)
and the ESP worktree
(`.worktrees/enterprise-static-portal`, `codex/enterprise-static-portal`) were
not modified. The branch was created from `origin/dev`; no `dev` move, stash,
reset, force push or merge to `main` occurred.

## Real FQ-07 edition accepted locally

One bounded GET per exact official resource was used. Files were retained only
under `/tmp/fq-official-20261024` (mode 700, files 600, outside Git). The
2025 official `multas-aplicadas-2025.csv` was stored locally as
`fines-2025.csv` to match the existing acceptance manifest; its URL and checksum
are unchanged. The 2021 CSV is the official file actually served as
`multas-2021-junho22.xlsx`.

Base URL for every row:
`https://www.gov.br/anp/pt-br/centrais-de-conteudo/dados-abertos/arquivos/mav/`

| Edition | File | Bytes | SHA-256 | Input | Accepted | Quarantined | Metadata | Source clock |
| --- | --- | ---: | --- | ---: | ---: | ---: | ---: | --- |
| 2016-2019 | `multas-aplicadas-2016a2019.csv` | 2,301,471 | `15ccbf54367881946104dc6baf65c246b6189638cb306ab06662570451989f25` | 16,440 | 16,290 | 150 | 0 | 2020-02-06 |
| 2020 | `multas-aplicadas-2020.csv` | 304,268 | `aef7fce874b29db234ab7779019c0bc9fcf13d1bdb2de7d59bbf09f51e32654f` | 2,026 | 2,001 | 25 | 3 | 2021-02 |
| 2021 | `multas-2021-junho22.xlsx` | 720,842 | `85122d0d1ae999173bac2c772ffb4e14ec414aec3c2f8e30bf7203187936a92d` | 4,976 | 4,929 | 47 | 3 | 2022-06 |
| 2022 | `multas-aplicadas-2022.csv` | 436,282 | `5ee3df1d9188f273fbf3fbeb7e95f2cea0bc2171fdda23bc4af3cce9478251b5` | 2,996 | 2,970 | 26 | 0 | 2022-04 |
| 2023 | `multas-aplicadas-2023.csv` | 445,861 | `3865025eb04a82ee5d937d703f063d617843eb7a9f5cbf4c18f989caa33839bb` | 3,074 | 3,053 | 21 | 0 | 2024-06 |
| 2024 | `multas-aplicadas-2024.csv` | 416,653 | `0723561b182ac0cc3d031a77050049844e3b29ec098472383b0229051cb67dcd` | 3,004 | 2,994 | 10 | 0 | 2025-03 |
| 2025 | `multas-aplicadas-2025.csv` (local `fines-2025.csv`) | 450,504 | `277f6cf5c59716581c42c5068fde47827c28eb693ee643c75d7841bda191fe96` | 3,395 | 3,375 | 20 | 0 | 2026-01 |
| **Total** | | | | **35,911** | **35,612** | **299** | **6** | |

The accepted denominator is therefore
`35,911 input = 35,612 accepted observations + 293 CPF quarantine records + 6
source metadata records`. No duplicates were retained. The 293 CPF records are
quarantined before publication and are not exportable.

The real run produced one current published revision and one publication-key
lease per edition. These IDs are run-local (the disposable database was dropped)
but prove the pin binding; the rows and payload hashes are the actual accepted
outputs:

| Edition | Revision ID | Lease ID (publication key) | Export rows | JSONL payload SHA-256 |
| --- | --- | --- | ---: | --- |
| 2016-2019 | `a2da2bc0-b5fa-4b65-abfb-4a9a431fb2e7` | `a2e9abe114759c4402f0a125a542e7934c62005819dbb40e9bd01110b94fcfc2` | 16,290 | `91e3ae2e8c609c8ccb28efa6a3383d6593f3570e0df210d9da104da472312d19` |
| 2020 | `b55c2f37-c99b-48bc-b1d6-dc1b9a686387` | `50466011b476d025737b7054532f9907f72beb99bf21213cf9b853baeaf21112` | 2,001 | `238f48f6e1064b0eb8ed225c72f94aa750be9c1076036acc7349421e05ffd3ab` |
| 2021 | `09d7d2dd-c87b-4918-9790-254dcfba6b6e` | `7f2da2d37a4e76eec7d763562c4ecfbf92f947d6f64605bae048e4145a7369e5` | 4,929 | `2c34e47471465968f92dbfc9a92df94ddddabd4dbdd8ccf5b409aadee1d35d68` |
| 2022 | `78c4ece7-0742-4ca2-af31-3f898a0106ff` | `beb51acec36d9c662d78b48eab626d026b069185f0fe7a0be1ff11f04fb4c02a` | 2,970 | `2fdd457dac3d77f5c6a2c884c82b2e9ba4766fa68caf8abe0a27156670824452` |
| 2023 | `f8d2ba95-6ec3-4b07-b9ce-801537018733` | `3bc4ebbcdd42cb2325364b79df42c846c034fd561b97958d5592aa14197272a4` | 3,053 | `e7e50380cf988705c2b2bbd71c58d158facb284afc30cb37db83db7330c34b8c` |
| 2024 | `6cd32701-5765-4a8c-af9b-33812fce92d8` | `648040238fc9bdd5b809c8ad444634938333cbedb31b226df73b07ba198256ff` | 2,994 | `99c696345b54bacbaebad49c75bb93c70fb6e9afd9d1c24f01885027135153d1` |
| 2025 | `251cbbc9-724d-43b2-b744-034bd452eaad` | `a18283c151938ddbb56574ddbd897629cda050c887fefbc8ce81e0dd6c150a88` | 3,375 | `3b9715af0939757eab210d6d00b4c1258e586917c345387c40ac959269c33e5c` |

The `LeaseID` is the frozen `quality_revision_provenance.publication_key`, not a
mutable selector. The export builder pins both revision and lease; a correction
produces a new revision and a new lease.

## FQ-08 published-revision read

The real pipeline test reuses the existing FQ-08 reader. For each edition it
asserts the served lease (`edition`, input/accepted/quarantined counts, source
metadata count and exact source clock) and traverses every accepted public record
through the **same selected revision** (`AtRevision(lease.RevisionID)`), rejecting
repeated IDs and a short count. The frozen export pin then resolves that same
current published revision, verifies `status='published'`, `VERIFIED`
provenance and a complete company census before returning a row.

No HTTP contract or OpenAPI file changed. The new port is an internal read port
for the static builder, not a public endpoint.

## Export port contract

Implemented in `backend/internal/modules/quality/application/official_export.go`
and `backend/internal/modules/quality/adapters/read/export.go`.

```go
type ExportReader interface {
    ExportPin(ctx context.Context) (OfficialExportPin, error)
    ExportSanctions(ctx context.Context, after string, limit int) ([]OfficialRecord, string, error)
}
```

`ExportPin` returns the frozen descriptor (`schema_version`, public-field
policy, source/stream/edition, revision and lease, source URL/checksum/bytes,
source period and clock, loss counts, association policy/scope/decision day,
association census and staleness). `ExportSanctions` pages by native key with a
cursor sealed to the revision:

```text
<revision-uuid>|<native-key>
```

Limits are 1..1000. A cursor from another/superseded revision is refused. The
adapter's `NewExportReader` accepts a pool or a caller-owned read-only
transaction, so the ESP builder can freeze one `REPEATABLE READ` snapshot.

`application.BuildOfficialExport(ctx, reader, out, limit)` traverses the reader
exactly once, writes deterministic JSONL (`SetEscapeHTML(false)`) and returns the
manifest with the SHA-256 of the exact bytes written. It fails closed on a
duplicate/short/changed-revision traversal, a census/row mismatch, a malformed
CNPJ, a linked record without a station, or a review outcome with a station.

### Record allowlist

`OfficialRecord` exposes only: native key, subject CNPJ, event family scope,
source-verbatim status, `occurred_on` (always empty for fines), `as_of`,
evidence reference, `station_id` only when linked, association state/reason,
revision/lease, source URL/checksum, proceeding reference, amount minor/source
text, decision date, due date, source contract/period, extraction month and
report year.

It deliberately excludes CPF, department, paid text/minor units, DUF/reference
labels, internal history revision IDs, unlabelled source fields, notes, raw rows,
owner/account data, documents and precise location. The 293 CPF records never
reach the sanction event table.

### Shape example (synthetic acceptance, not a source row)

```json
{
  "native_key": "observation:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
  "subject_cnpj": "11222333000181",
  "scope": "sanction",
  "status": "source status",
  "evidence": "synthetic.csv:8",
  "station_id": "eeeeeeee-5555-4555-8555-000000000001",
  "association_state": "linked",
  "association_reason": "exact current cnpj; historical business window unknown",
  "revision_id": "dddddddd-4444-4444-8444-000000000002",
  "lease_id": "f91e833410bbe00bffe67a931dd4f4bfd3a7afdd5074ef36a948e1cc0f74a3e4",
  "source_url": "https://www.gov.br/anp/pt-br/centrais-de-conteudo/dados-abertos/arquivos/mav/multas-aplicadas-2025.csv",
  "source_checksum": "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
  "proceeding_ref": "48000.1/2025-11",
  "amount_minor": 150000,
  "amount_text": "1500,00",
  "due_on": "2025-02-28",
  "source_contract": "anp-sicom-cp850-9-v1",
  "source_period": "2025",
  "extraction_month": "2026-01",
  "report_year": 2025
}
```

`occurred_on` and `decided_on` are absent in this native example because the
source supplies no incident/decision day. A due date is never relabelled as an
occurrence or ownership date.

### Manifest shape

```json
{
  "schema_version": "quality-official-export-v1",
  "public_field_policy": "fq-public-v1",
  "source": "sanctions",
  "stream": "sanction-v1",
  "edition": "2025",
  "revision_id": "dddddddd-4444-4444-8444-000000000002",
  "lease_id": "f91e833410bbe00bffe67a931dd4f4bfd3a7afdd5074ef36a948e1cc0f74a3e4",
  "published_at": "2026-10-24T21:55:16Z",
  "status": "published",
  "provenance_state": "VERIFIED",
  "source_url": "https://www.gov.br/anp/pt-br/centrais-de-conteudo/dados-abertos/arquivos/mav/multas-aplicadas-2025.csv",
  "source_checksum": "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
  "source_bytes": 100,
  "source_period": "2025",
  "source_clock_precision": "MONTH",
  "source_clock_value": "2026-01",
  "counts": { "input": 3, "accepted": 3, "duplicates": 0, "quarantined": 0 },
  "association_policy": "current-company-v1",
  "association_scope": "CURRENT_COMPANY_ONLY",
  "association_decision_day": "2026-10-20",
  "association_census": {
    "total": 3,
    "linked": 1,
    "unmatched": 1,
    "ambiguous": 1,
    "wrong_branch": 0,
    "succession_review": 0
  },
  "stale": true,
  "stale_reason": "source extraction month may exceed 180 days",
  "rows": 3,
  "payload_sha256": "<SHA-256 of the exact JSONL payload>"
}
```

The real 2025 manifest uses `revision_id 251cbbc9-724d-43b2-b744-034bd452eaad`,
`lease_id a18283c151938ddbb56574ddbd897629cda050c887fefbc8ce81e0dd6c150a88`,
`rows 3375` and payload SHA-256
`3b9715af0939757eab210d6d00b4c1258e586917c345387c40ac959269c33e5c`.
The real census is all unmatched (see below); the 1/1/1 values above are the
synthetic PostGIS acceptance that proves the linked/ambiguous mapping.

## Association policy and census

The export consumes the existing immutable FQ-07R6 decisions, not a new linker.
The policy remains `current-company-v1` / `CURRENT_COMPANY_ONLY`, with
`historical_business_window_known=false`. A decision is linked only for a
complete, exact, kernel-validated CNPJ with exactly one current active Directory
station. Ambiguity, retirement, wrong branch and succession review carry no
station. No name, address, brand, GPS, base-only or fuzzy match is used.

Public mapping:

| Stored resolution | Export state | Station | Reason preserved |
| --- | --- | --- | --- |
| `bound` | `linked` | exact station UUID | yes |
| `unmatched` | `unmatched` | empty | yes |
| `ambiguous` | `ambiguous` | empty | yes |
| `wrong_branch` | `unmatched` | empty | yes |
| `succession_review` | `unmatched` | empty | yes |

Real accepted census (empty Directory in the disposable acceptance DB):
**35,612 total; 0 linked; 35,612 unmatched; 0 ambiguous; 0 wrong branch; 0
succession review.** The seven real editions therefore support an event-level
portal with no station claim. This proves complete loss accounting, not national
station coverage.

Synthetic real-PostGIS tests prove the other states: one linked, one unmatched
and one ambiguous record in the same revision; wrong-branch and succession
review are exported as unmatched with their exact reasons and no station. The
existing FQ-07R6C/D tests additionally cover a mismatched declared station,
retired identity, wrong branch, multiple candidates and the actual Directory
transaction guard.

A linked value is frozen at the publication decision day. Automatic fine
association revalidation/rebound after a later Directory change remains OWED
(FQ-07R6D); the current revalidation sweep covers PMQC links, not fine company
decisions. The ESP must present `linked` as "current company at the pinned
decision day", not as live or historical ownership, and must rebuild from a new
revision when revalidation is delivered. The real accepted edition is all
unmatched, so this limitation does not block the event-level slice.

## Negative and failure coverage

New tests cover:

- stale cursor from a superseded revision and a malformed cursor;
- withdrawn current revision and no published revision;
- superseded rows never exported; correction moves the pin to the new revision;
- duplicate, short and changed-revision traversal;
- linked/unmatched/ambiguous mapping and wrong-branch/succession review mapping;
- linked record without station, review record with station, invalid/CPF-shaped
  subject, invalid date, bad checksum, empty period/lease;
- closed storage fails as `ErrOfficialExportUnavailable` without `pgx`/pool
  internals;
- deterministic repeat build with identical bytes and payload SHA-256;
- real seven-edition export with complete census and source identity.

## Commands and actual results

Rust preparers (pinned toolchain, locked dependencies, owned `/tmp` output):

```sh
cargo +1.99.0 build --locked --manifest-path tools/station-prep/Cargo.toml \
  --bin prepare_quality --bin prepare_sanctions --bin prepare_fine_history \
  --bin prepare_enforcement --target-dir /tmp/fq-native-rust-20261024
chmod 755 /tmp/fq-native-rust-20261024/debug/prepare_*
```

Result: PASS (initial build 7.41 s; incremental rebuild 0.32 s).

Real seven-file worker/read/export acceptance, disposable loopback PostGIS 5434:

```sh
ANPFUEL_TEST_FINE_HISTORY_SOURCE_DIR=/tmp/fq-official-20261024 \
ANPFUEL_TEST_SANCTIONS_PREPARER_PATH=/tmp/fq-native-rust-20261024/debug/prepare_sanctions \
ANPFUEL_TEST_FINE_HISTORY_PREPARER_PATH=/tmp/fq-native-rust-20261024/debug/prepare_fine_history \
go test -race -count=1 -tags=integration,qualitysource ./cmd/worker \
  -run TestOfficialSevenFileWorkerPipeline -v
```

Result: **PASS 69.57 s** (package 70.592 s); all seven subtests PASS; log line:
`35911=35612accepted+293CPF+6metadata;35612 explicit unmatched company
decisions in empty Directory,zero identity writes`.

Full affected integration matrix:

```sh
ANPFUEL_TEST_QUALITY_PREPARER_PATH=/tmp/fq-native-rust-20261024/debug/prepare_quality \
ANPFUEL_TEST_SANCTIONS_PREPARER_PATH=/tmp/fq-native-rust-20261024/debug/prepare_sanctions \
ANPFUEL_TEST_FINE_HISTORY_PREPARER_PATH=/tmp/fq-native-rust-20261024/debug/prepare_fine_history \
ANPFUEL_TEST_ENFORCEMENT_PREPARER_PATH=/tmp/fq-native-rust-20261024/debug/prepare_enforcement \
go test -race -count=1 -tags=integration ./internal/modules/quality/...
```

Result: PASS — adapters 73.738 s, http 49.917 s, jobs 26.216 s, native 1.867 s,
prepared 1.637 s, read 49.923 s, application 1.431 s, domain 1.099 s.

Unit/static:

```sh
go test -race -count=1 ./internal/modules/quality/...
go build ./...
go vet ./internal/modules/quality/... ./cmd/worker ./cmd/api
GOTOOLCHAIN=go1.27.1 staticcheck ./internal/modules/quality/... ./cmd/worker ./cmd/api
sqlc vet
sqlc generate                 # byte-stable generated query
gofmt -l <changed files>      # clean
git diff --check              # clean
```

All PASS. A scoped credential/private-key pattern scan over the changed files
returned no matches. No source archive, raw row, CPF or binary build output was
committed.

## Limitations and OWED

- **Fine association live revalidation/rebound is OWED.** The snapshot freezes
  the publication decision; it does not re-query Directory at visitor time. The
  real accepted edition is all unmatched, so no stale link is published.
- **No persistent snapshot artifact was retained from the disposable real run.**
  The port and builder are the deliverable; the real payload SHAs above prove
  the traversal. The ESP builder will write the JSONL and manifest.
- FQ-06 live XLSX runtime, FQ-17 scoped acceptance, FQ-16/Android, portal pages,
  search/downloads, VPS/staging deployment, DNS/Caddy/Compose, map work and
  `main` integration are not included.
- Fine imports/discovery remain default off. No production source was activated.
- The raw source files and Rust binaries live outside Git under `/tmp`; their
  exact hashes are recorded above and match the acceptance test constants.

## Handoff to ESP-11

1. Import `quality/application` and `quality/adapters/read`.
2. Open a read-only `REPEATABLE READ` transaction and call
   `read.NewExportReader(tx)`.
3. Call `application.BuildOfficialExport(ctx, reader, jsonlWriter, 500)`. Write
   the returned manifest next to the JSONL; bind `revision_id`, `lease_id`,
   `edition`, `source_url`, `source_checksum`, `counts`, `association_policy`,
   `association_scope`, `association_decision_day`, `association_census` and
   `payload_sha256` into the ESP manifest.
4. Use `association_state` for station links: only `linked` has a non-empty
   `station_id`; `unmatched` and `ambiguous` never link. Display
   `association_reason` and the decision day, and do not claim historical
   ownership.
5. Render `decided_on`/`due_on` as source dates. `occurred_on` is empty for
   fines; a due date is not an incident or ownership date.
6. The real accepted edition is event-level with 35,612 unmatched records. The
   ESP can publish the `/fiscalizacao/` event/detail slice without station
   claims now. Station pages, live current links, FQ-06 enforcement and the
   public/VPS release remain separate work.

No portal page, `portalgen` change, producer change, map change, ESP worktree
change, VPS mutation or `main` merge was performed.

# FQ-07R2 — Verified typed native fine artifact boundary

Status: LOCAL_DONE
Validation: PASS

Parent0a5876c8, maintained dev. B-BR-FQ-01/02/06/07/14/16;
BUC-FQ-03/04/06. The official fine preparer contract is accepted locally;
the old sanction-v1 ledger cannot represent its due/extraction precision.

Before behavior: add a separate Go typed observation boundary, not a conversion
into synthetic SanctionInput. Verify manifest input identity/year/schema/bounds,
all three stream hashes/bytes/counts, logical manifest hash, canonical observation
key, required fields, exact money/calendar/month precision and redacted quarantine
before returning any data. Original row locators form a complete non-overlapping
input census. Duplicate observations must have an accepted semantic twin.
Cancellation/open/read/close/UTF-8/unknown/repeated JSON failures return no batch.
No source file, network, database or Directory write capability is introduced.

Acceptance: negative tamper/framing/budget/precision/CPF/money/loss cases and real
Rust→Go source reconciliation under race; affected Go static/format and Rust
contract checks. Publication, compatible append-only SQL/reads, source resolution,
worker/discovery/image and station journeys remain subsequent tasks. No fake
as_of/decided_on, source-issued fine ID, coverage or release claim. D held.

## Delivered and verified, 2026-10-19

`prepared.LoadFines` returns only `application.FineObservation` in a separate
FineBatch. No conversion into SanctionInput exists. Required year/money pointers
reject absent/null fields, source month remains YYYY-MM, due dates are calendar
validated, raw amount/payment text must equal the exact int64 representation.
The source observation key is independently reconstructed from frozen canonical
Rust field order with no HTML escaping; the emitted canonical row must match.
The existing strict JSON decoder refuses duplicate/case-alias/unknown fields.

All three streams must pass byte/hash/line/count bounds, accepted-key ordering,
semantic duplicate twins and redacted quarantine checks. Every original logical
row8..input+7 appears exactly once across accepted/duplicate/quarantine artifacts;
missing, repeated and foreign locators refuse the entire batch. The logical
manifest hash includes report/extraction precision and all artifact rows/counts.
No partial batch, raw source error, source name/CPF or premature resolution side
effect follows a failure. The caller must still pin the trusted native binary
and original downloaded input identity; hashes alone do not authenticate origin.

Validation PASS:

```sh
# From backend/
GOCACHE=/tmp/fq-completion-go-cache go test -race ./internal/modules/quality/adapters/prepared ./internal/modules/quality/application -count=1
GOCACHE=/tmp/fq-completion-go-cache go vet ./internal/modules/quality/adapters/prepared ./internal/modules/quality/application
GOCACHE=/tmp/fq-completion-go-cache go build ./internal/modules/quality/... ./cmd/api ./cmd/worker
XDG_CACHE_HOME=/tmp/fq-completion-staticcheck GOTOOLCHAIN=go1.27.1 GOCACHE=/tmp/fq-completion-static-cache staticcheck ./internal/modules/quality/adapters/prepared ./internal/modules/quality/application
ANPFUEL_TEST_SANCTIONS_PREPARER_PATH=/absolute/trusted/prepare_sanctions ANPFUEL_TEST_SANCTIONS_SOURCE_PATH=/private/fines-2025.csv GOCACHE=/tmp/fq-completion-go-cache go test -race -tags=integration,qualitysource ./internal/modules/quality/adapters/prepared -run TestOfficialFineRustGoBoundary -count=1 -v
```

RED new boundary absent; GREEN typed loss/census,19 negative tamper cases and
cancellation/nil-port/open/close error containment, plus existing prepared and
application suites. Negative cases include wrong input identity/clock/calendar,
CPF, unknown/repeated/noncanonical JSON, mismatched/missing money, duplicate or
missing records, duplicate without twin, recomputed truncated artifact, path
escape, count/hash mismatch and private quarantine content. Real-source test
runs the actual trusted Rust CLI and independently verifies all3,395 row locators,
3,375 accepted typed observations,0 duplicates and20 CPF quarantines, retaining
extraction2026-01. Its first invocation correctly refused a group-writable local
build binary; mode755 fixed the test prerequisite without weakening the guard.
Final real-source run PASS,3.204s with race; scoped unit race/static/build PASS.

Logs: `/tmp/fq07r2-final.log`, `/tmp/fq07r2-source-final.log`,
`/tmp/fq07r2-static.log`. Format/whitespace, scoped secret and links PASS. Source
files/private artifacts remain outside Git. No dependency, SQL/migration/read
behavior, worker/image, Android, deployment, source activation or remote merge.

## Next and recovery

Next FQ-07R3: append-only storage/read compatibility for due date, source
extraction-month precision, observation identity, DUF/department, original paid/
amount text, preserving legacy snapshots. Immediate real-PostGIS upgrade/replay/
failed-correction and wire-contract tests are required before publishing native
observations. Then verified source discovery, native worker composition and
safe Directory linkage/public journey. FQ-06 native XLSX remains owed as recorded
in FQ-07R1. Omit/disable this boundary to stop intake; nothing has been published.
D and production/device acceptance remain held.

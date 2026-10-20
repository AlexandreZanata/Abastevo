# FQ-07R3A — Additive native fine storage compatibility

Status: LOCAL_DONE
Validation: PASS

Parent820af736, maintained dev. B-BR-FQ-01/02/06/07/14/16;
BUC-FQ-03/04/06. One bounded storage prerequisite before native publication.

Before behavior: append migration106; never modify applied94/105. Preserve
legacy exact-day snapshots and current pointers unchanged. A separate native
source-contract marker permits NULL exact as_of/decided_on only with a valid
source extraction month, report year and all source-specific due/payment/DUF/
department/raw-money fields. Content observation keys are not source fine IDs.
Retain original strings without truncation; do not compute balances or legal
classification. Legacy writes cannot silently opt into the native shape.

Add a sqlc-owned staging query, not runtime source publication. Validate the
actual105→106 upgrade, old row preservation, re-apply no-op, native field/clock
constraints, exact integer money, foreign revision/current-pointer isolation,
failed-write rollback and existing critical quality regressions on real
PostgreSQL/PostGIS immediately. Current public reads remain legacy until the
native publisher and explicit clock/read wire compatibility are implemented in
FQ-07R3B. No API/Android/runtime caller may use this new staging path yet.
D, deployment and production/native-device certification remain held.

## Delivered and verified, 2026-10-19

Append-only106 expands only quality sanction storage: legacy rows receive a
legacy-shape marker with all native-only fields NULL; exact as_of/decided_on,
amount, provenance/publication keys and current pointers remain intact. Native
shape requires a content-observation key, full company identifier shape, report
year, valid YYYY-MM extraction precision, bounded due calendar, DUF/department,
original amount/paid strings and exact nonnegative int64 values. SQL independently
checks raw-money equality using exact numeric arithmetic; it computes no balance
or aggregate. No native exact extraction/decision day can be stored accidentally.
Verbatim status/proceeding bounds expand without truncating prior values.

The sqlc-owned StageNativeFineObservation operation locks the staging revision
FOR SHARE and checks the sanctions/sanction-v1 scope. A native stage cannot append
to a published/withdrawn or foreign source revision; eight concurrent equivalent
stages create exactly one row. No runtime publisher/worker/API caller uses this
query yet. Migration106 has not been applied to any persistent local/VPS source
DB; tests create/drop independent disposable databases from the actual chain.
No applied predecessor migration was edited.

Three new real-PostGIS scenarios PASS, including14 invalid native cases:
105→106 with actual existing published snapshot, no-op migration replay and
stable old publication replay; invalid/missing/excess-precision month, earlier
extraction year, fabricated identity, CPF shape, money disagreements/negative/
malformed text, missing DUF/department/due date, infinite/overlong-year dates;
transaction rollback, concurrent idempotency and retained long labels/paid data;
foreign source refusal and the real PostgreSQL blocked-revision barrier.
The latter observes pg_blocking_pids before withdrawing the locked revision;
the waiting stage rechecks its predicate and inserts no row. No sleep-based race
assumption is used. Existing published pointers remain unchanged throughout.

RED special-date tests demonstrated that PostgreSQL DATE alone accepted infinity
and year10000. The uncommitted development migration now constrains native due
values to0001-01-01..9999-12-31. After this correction the affected complete
quality race/PostGIS/HTTP/job/read matrix was repeated on the final migration,
all eight packages PASS; migration and API-contract race suites also PASS.
The only existing-test changes extend three explicit expected upgrade chains by
106; no assertion, coverage, failure case or acceptance gate was removed.

Commands (backend/, local disposable PostGIS only):

```sh
ANPFUEL_TEST_QUALITY_PREPARER_PATH=/absolute/trusted/prepare_quality ANPFUEL_TEST_PREPARER_PATH=/absolute/trusted/prepare_registry GOCACHE=/tmp/fq-completion-go-cache go test -race -tags=integration ./internal/modules/quality/... -count=1
GOCACHE=/tmp/fq-completion-go-cache go test -race -tags=integration ./internal/platform/migrate ./internal/platform/apicontract -count=1
GOCACHE=/tmp/fq-completion-go-cache go vet ./internal/modules/quality/...
GOCACHE=/tmp/fq-completion-go-cache go build ./cmd/api ./cmd/worker ./cmd/migrate
XDG_CACHE_HOME=/tmp/fq-completion-staticcheck GOTOOLCHAIN=go1.27.1 GOCACHE=/tmp/fq-completion-static-cache staticcheck ./internal/modules/quality/...
sqlc vet
sqlc generate
```

Final logs `/tmp/fq07r3a-quality-final.log` (eight packages, adapters31.957s),
`/tmp/fq07r3a-platform.log` (migrate4.492s, apicontract5.612s), scoped barrier
`/tmp/fq07r3a-storage-final.log`, special-date RED
`/tmp/fq07r3a-calendar-red.log` and clean static log
`/tmp/fq07r3a-static.log`. sqlc re-generation is byte-stable across every owned
package; gofmt/diff/secret/link review PASS. Existing API/worker/migrator compile.
No dependency, Android source or public response contract changed.

## Recovery and next

Native staging is not composed, so omit it to stop new native intake. Applied
schema recovery is forward-only; never roll back106 by deleting source facts.
Existing legacy publication/read behavior remains available and tested.
Next FQ-07R3B must implement the typed native publisher and explicit nullable
exact-day/month-precision read wire contracts before runtime publication; then
safe source-clock-aware Directory association, discovery/native worker/image
and station journeys. GRM due date cannot become a sample/event occurrence or
identity evidence date. Full native history, FQ-06 XLSX, FQ-14/FQ-17/protected
batch delivery remain OWED. No deployment, source activation, device execution,
remote merge, score calibration or D work follows this checkpoint.

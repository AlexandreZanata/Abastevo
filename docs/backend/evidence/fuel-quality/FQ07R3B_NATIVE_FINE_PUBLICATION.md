# FQ-07R3B — Atomic native fine publication and precise public clocks

Status: LOCAL_DONE
Validation: PASS

Parentcb10cc5b, maintained dev. B-BR-FQ-01/02/06/07/14/16;
BUC-FQ-03/04/06. Native preparation, typed verification and106 storage are locally
accepted; no native runtime import is yet composed.

Before behavior: add a typed native-fine publication request with exact accepted
counts, per-observation year/month agreement and strict calendar/money/company
validation. Reuse the owned edition transaction, correction/replay/provenance/
current-pointer machinery. Freeze source report year/extraction month even for
an empty native revision through additive107 metadata. No native station links
or score are derived here; GRM due date is not an event/identity date.

Public sanction page/detail preserve legacy exact-day values and disclose native
month precision, NULL exact as_of/decided_on, source due/raw-money/payment/DUF/
department data and content-observation identity explicitly. Never label a content
hash as a source-issued fine number. Add the source extraction clock to the lease;
its conservative month-age bound uses the existing180-day rule independently of
local publication freshness. A recent import cannot make old source data fresh.
No exact extraction day is stored or represented by this age-bound calculation.

Immediate acceptance: domain clock/money RED→GREEN, real-PostGIS native publication,
replay/concurrent convergence/correction/failed correction/empty revision, frozen
source metadata, historical reads and actual HTTP/OpenAPI/ETag/cursor behavior.
Affected SQL generation/static/build/critical race suites remain required. Native
source fetching/discovery/worker/image, safe station linkage, FQ-06 XLSX and full
FQ-14/FQ-17 remain subsequent tasks. D and production/device gates stay held.

## Delivered and verified, 2026-10-19

NativeFineInput independently checks bounded nonnegative loss counts, exact
accepted-row cardinality, one bounded source resource, full valid company CNPJ,
raw-money/minor-unit equivalence, calendar days, per-row year/month agreement and
content-derived observation identity. Native hash reconstruction uses the frozen
Rust serialization order with empty key/locator slots. Strict text budgets run
before identity hashing. A changed label/paid/amount/DUF/proceeding/clock cannot
reuse the same observation identity; an observation hash is never promoted to a
source-issued fine ID. No name/CPF enters this input type.

PublishNativeFines reuses the existing per-edition transaction/live guard,
immutable revision provenance/resources/periods, correction, monotonic current
pointer and retry convergence. The optional native clock joins the publication
key only for native requests; nil omits it and preserves every old key byte.
Migration107 freezes report year/extraction month by revision, even when no row
was accepted. Native clock, staged observations, provenance and pointer commit
atomically. Forced failure after the first staged observation rolls everything
back; a later correction cannot mutate the older source context or raw facts.

Public sanction pages/details now expose explicit content/legacy identity kind,
DAY/MONTH precision, nullable exact as_of/decided_on, extraction month, due date,
DUF/department and original amount/paid text plus minor units. They expose no
company identifier or personal name. Legacy exact-day values remain unchanged;
new fields are nullable where the legacy contract never supplied them. OpenAPI
matches the actual non-empty native responses. No station association/filter or
legal interpretation is added.

The source month also joins the frozen lease, including empty native revisions.
The existing180-day rule is shared by source and publication clocks. The earliest
possible day in a month is used only as a conservative age bound, never stored or
presented as the extraction day; unavailable/future months are stale. A new local
publication cannot refresh old official evidence. ETags reflect source staleness;
revision/cursor/history fences remain intact.

## Validation

RED new domain/input/publication contracts, then GREEN. Pure exact-money and
month-age tests cover grouped/ungrouped money, int64 maximum/overflow, malformed
values, invalid/future month, exact180-day boundary and uncertain source age.
Typed-input negatives cover company/identity/date/clock/money/text/count/repeated
observation failures and verified empty publications. An additional RED case
showed balanced negative sub-counts could pass the generic legacy reconciliation;
the native boundary now refuses every negative/out-of-budget count before I/O,
without changing unrelated historical contracts.

Real-PostGIS native scenarios PASS: replay/live-guard refusal, correction,
second-stage failure rollback, frozen historical source clock, eight concurrent
publishers converging to one revision and an empty revision retaining its source
clock. Real HTTP/OpenAPI scenario PASS: populated page/detail, complete two-record
keyset traversal, NULL exact-day fields, company identifier omission, source age
independent of publication age, ETag/304 boundary, stale cursor refusal, current
lease ID isolation and explicit superseded history with the original month.

Final complete affected race matrix PASS (all eight quality packages plus migrate
and apicontract), using actual migration chain107 and trusted native preparers:

```sh
# backend/, disposable local PostGIS only
ANPFUEL_TEST_QUALITY_PREPARER_PATH=/absolute/trusted/prepare_quality ANPFUEL_TEST_PREPARER_PATH=/absolute/trusted/prepare_registry GOCACHE=/tmp/fq-completion-go-cache go test -race -tags=integration ./internal/modules/quality/... ./internal/platform/migrate ./internal/platform/apicontract -count=1
GOCACHE=/tmp/fq-completion-go-cache go vet ./internal/modules/quality/...
GOCACHE=/tmp/fq-completion-go-cache go build ./cmd/api ./cmd/worker ./cmd/migrate
XDG_CACHE_HOME=/tmp/fq-completion-staticcheck GOTOOLCHAIN=go1.27.1 GOCACHE=/tmp/fq-completion-static-cache staticcheck ./internal/modules/quality/...
sqlc vet
sqlc generate
```

Logs `/tmp/fq07r3b-final-critical.log`: adapters33.565s, HTTP16.387s,
jobs13.186s, reads9.389s, native/prepared/application/domain PASS,
migrate5.097s and apicontract6.130s. Static/build/vet PASS;
byte-stable generation across every owned sqlc package, format/diff/secret/links
PASS. A missing space before a named SQL parameter was caught by compilation and
fixed before real SQL execution; parameters remain generated and bound.

Actual2025 source, rebuilt through the real Rust CLI and Go typed boundary,
PASS under race in2.518s (`/tmp/fq07r3b-final-source.log`):3,395 input =3,375
accepted +0 duplicates +20 CPF quarantines; every accepted observation also
passes the independent application publication contract and content identity.
The original hash/byte watermark remains the pinned FQ-07R1 resource. No private
source or prepared artifact is tracked; no dependency added.

## Remaining scope and recovery

No native fetch/discovery handler, worker image path, schedule activation,
Directory association, Android sanction journey or deployment is composed by
this checkpoint. These remain FQ-07R4/FQ-14 and scoped FQ-17. No complete national
or historical inventory is claimed from one checked resource. FQ-06 XLSX remains
owed. No numeric score, business/document portal or D implementation follows.

Omit/disable native intake to stop publication; already published safe factual
history remains readable. Apply schema107 recovery forward only. The native
publisher is an internal trusted-source port, never an anonymous write route.
Source URL/download/native binary integrity and live Directory evidence still
belong to the job composition guard. Keep reports/signals/import/UI/cache controls
independent and preserve privacy recovery when intake is disabled.

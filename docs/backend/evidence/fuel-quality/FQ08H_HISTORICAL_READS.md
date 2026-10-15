# FQ-08H — Complete revision access

Status: LOCAL_DONE
Validation: PASS
Integration: INTEGRATION_PENDING.
Date: 2026-10-15.
Behavior parent: 7691a87b.

Before behavior: B-BR-FQ-04/05/07/12/16, BUC-FQ-03/04. Add a bounded anonymous
source-stream revision catalog and explicit revision selection on existing
summary/assay/sample/action/sanction reads. Defaults retain the current stream.
Every selected revision must belong to the route's exact source/stream; unknown
or cross-stream selectors refuse. One resolved lease scopes the entire request,
including detail and cursor validation, without a second current-pointer read.

The catalog is live metadata in stable UUID keyset order, not a frozen copy of a
changing catalog. Clients can restart catalog traversal after concurrent imports;
record traversal is frozen to an explicitly selected revision. Cursors bind the
route, selected revision and every existing filter. No cross-revision mixing.
Published historical editions and superseded verified revisions remain reachable
with explicit status/current labels. Legacy-unverified and withdrawn revisions
are discoverable as unavailable metadata, never fabricated source denominators
or silently served facts. Corrections retain old revision provenance and rows.

Frozen counts, manifest hash, claimed periods and exact public resource hashes
are available on revision detail, independently of mutable edition discovery
metadata. No source CNPJ, private report/owner, GPS, documents or credentials.
Existing kill switch also contains the new endpoints; anonymous reads remain
separate from signed private reports. ETags include revision/status/staleness.

Immediate acceptance: multiple editions/corrections, complete paginated catalog
and record traversal, unknown/cross-stream selectors, cursor replay/filter/scope,
concurrent publication with selected old revision, legacy/withdrawn containment,
actual OpenAPI responses, privacy surfaces, real PostGIS/race and indexed plans.
No migration or staging change is required for this read-only additive phase.

## Validated source behavior

- Anonymous `/v1/quality/revisions?stream=...` has a sealed, filter-bound UUID
  keyset catalog (1–100 entries, `catalog_live: true`). Three exact streams are
  allowed; no caller-supplied source/tenant. `/v1/quality/revisions/{id}?stream=...`
  exposes frozen counts, manifest hash and resource hashes with availability.
- Existing public routes accept optional `revision_id`. A request-local Reader
  preserves shared-reader concurrency safety. Defaults still resolve current;
  explicit verified superseded history stays accessible, with status/current
  labels and `supersedes_revision_id`. Frozen per-revision periods/resources
  do not read mutable edition discovery manifests.
- Unverified/withdrawn metadata retains known revision/publication identity;
  counts/hash are null, resources empty and facts unavailable. Unknown and
  cross-stream references return 404, malformed filters 400 and storage failure
  503. Kill switch covers every route. No private report/account surface added.
- HTTP ETags include selected lease/status/staleness. A historical cursor remains
  valid after correction only when the same revision/filter is selected; current
  or another revision rejects it. Catalog cursors bind stream/limit and tampering
  refuses. Catalog metadata can evolve; it is not a frozen multi-request snapshot.

Immediate validation (local disposable PostGIS only):

```sh
GOCACHE=/tmp/fq-completion-go-cache go test -race -tags=integration ./internal/modules/quality/adapters/read ./internal/modules/quality/adapters/http ./internal/platform/apicontract -count=1
GOCACHE=/tmp/fq-completion-go-cache go test -race -tags=integration ./internal/modules/quality/adapters/read ./internal/modules/quality/adapters/http -run 'TestHistorical|TestQualityHTTPCache' -count=1
```

All PASS. Domain/application/HTTP unit regressions, affected builds/vet/staticcheck,
sqlc vet/byte-stable generation, standard YAML parsing, strict vacuum (zero errors),
gofmt, whitespace and staged/new secret/PII review PASS. Contract tests explicitly
use `-count=1` after external spec/vector changes, including FQ-11 appeal vectors.

Reader acceptance: unknown/cross-stream refusal; three retained revisions across
two editions; complete catalog/assay traversal and sample detail after correction;
original resource hashes/counts; explicit correction ancestry; legacy/withdrawn
containment; original enforcement and sanction snapshot/detail after correction.
HTTP acceptance: original cursor traversal after concurrent correction, selected
versus current reader isolation, private identifier absence, exact old provenance,
catalog cursor scope/tampering, malformed/unknown filters, nullable unavailable
and legacy metadata, known publication time preservation, kill switch and aged
historical detail ETag. Actual populated and unavailable responses validate against
OpenAPI; no test/schema relaxation.

Load: 5,001 synthetic retained revision headers, no invented official source data.
EXPLAIN ANALYZE ran the exact sqlc-generated catalog query captured by pgx tracing,
with its actual bound arguments. The query fetched 101 rows for a 100-entry page,
using `quality_revisions_pkey` and `quality_revision_provenance_pkey`; planning
0.688 ms, execution 0.315 ms in this local run. This is a scoped query proof, not
production certification. No speculative index/denormalization was added. Logs:
`/tmp/fq08h-final-plans.log`, `/tmp/fq08h-selector-cache.log` (outside Git).

Recovery: disable official reads with existing switch; preserve historical rows
and provenance. No migration, deployed runtime/native import, numeric score,
Android acceptance, G09/public pilot or merge is claimed. Next bounded prerequisite:
wire verified native preparation into the Go import composition before FQ-12 onward.
D remains future-only, with documents/business portal excluded.

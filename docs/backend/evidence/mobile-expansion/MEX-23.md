# MEX-23 — National source-record read API and honest coverage

Status: LOCAL_DONE
Validation: PASS
Integration: INTEGRATION_PENDING

Parent: `de13d57a` (maintained dev, PR #145 head). Date: 2026-10-22.
Depends MEX-19/20 (aggregate satisfied by 20a–f)/21/22, accepted. Rules
B-BR-MEX-10/12–17; BUC-MEX-07. Risk: public SQL/privacy/load
(critical: anonymous reads over source facts, facet injection,
unbounded traversal).

Before behavior: the frozen enforcement reads gain bounded global
occurrence pages with exact facets (UF, municipality, segment,
subject kind, document-day range) and a coverage endpoint with valid
counts. Facets are exact matches only — no prefix, fuzzy or proximity;
malformed values return 400, unknown values page empty. Unmatched and
unknown records stay included unless a facet excludes them. Coverage
reports distinct observations versus distinct source-stated cases with
the document-day bounds and basis; counts only, never identifiers. The
document-day basis is declared on every dated response; no occurrence,
finality or extraction clock is invented. Keyset traversal omits and
duplicates nothing; cursors bind revision, filters and limit. Queries
use the facet index with unrelated rows present; limits and
cancellation hold; HTTP bodies validate against OpenAPI. Recovery:
disable facets or coverage without hiding global facts.

## Delivered

- `backend/db/migrations/000113_quality_enforcement_source_facet_idx.sql`:
  append-only composite facet index (revision, UF, segment, kind, day,
  native keyset) for bounded filtered pagination; exact matches only.
- `backend/db/queries/quality/enforcement_source.sql` (+ generated
  Go): `EnforcementSourceFilteredPage` (six optional exact facets plus
  keyset) and `EnforcementSourceCoverage` (observations, distinct
  documents as cases, day bounds).
- `backend/internal/modules/quality/adapters/read/read.go`:
  `EnforcementFilter`, `EnforcementCoverage`,
  `EnforcementFilteredAtLease` and `EnforcementCoverageAtLease` under
  the leased revision; unleased or foreign sources yield nothing.
- `backend/internal/modules/quality/adapters/http/http.go`:
  facet params validated on `/v1/quality/actions` (UF shape, known
  kinds, calendar days, from ≤ to; bound into the cursor hash),
  `date_basis` declared on native items, and
  `/v1/quality/actions/coverage` with ETag. Facet params allowlisted;
  legacy status/scope stay refused for the native source.
- `contracts/openapi/v1.yaml`: six facet params, `date_basis` on
  `QualityAction`, and the `QualityActionCoverage` schema plus the
  coverage path; vacuum zero errors.
- `enforcement_facets_integration_test.go`: faceted pages (each
  facet, combinations, unknown empties, malformed 400s), keyset
  traversal under facets, coverage counts/days/basis, OpenAPI
  validation and identity-leak assertions.
- `enforcement_facet_plan_integration_test.go`: filtered page uses an
  index (no sequential scan) with 20,000 unrelated rows after ANALYZE.

## Actual checks

From this worktree (2026-10-22, parent `de13d57a`):

- `go test -count=1 -tags=integration -run
  TestEnforcementFacetsCoverageAndOpenAPI
  ./internal/modules/quality/adapters/http/` → PASS: 4-record fixture
  with varied facets; UF/segment/kind/period filters exact; combined
  facets; unknown UF pages empty; five malformed shapes return 400;
  coverage 4 observations / 4 cases with day bounds and basis;
  keyset traversal under facets complete; page and coverage bodies
  validate; no CNPJ in any body.
- `go test -count=1 -tags=integration -run
  TestEnforcementFilteredPageUsesIndex
  ./internal/modules/quality/adapters/read/` → PASS: EXPLAIN shows
  index access, no sequential scan, with 20,000 unrelated rows.
- `go test -race -count=1 -tags=integration
  ./internal/modules/quality/adapters/http/
  ./internal/modules/quality/adapters/read/` → PASS (27 s / 31 s),
  including the pre-existing enforcement traversal/publication tests
  with no regression.
- `go test -count=1 ./internal/modules/quality/...` → PASS;
  `go vet ./internal/modules/quality/...` PASS;
  `GOTOOLCHAIN=go1.27.1 staticcheck ./internal/modules/quality/...`
  PASS; `sqlc vet` and `sqlc diff` PASS (migration 000113, queries,
  deterministic generate).
- `vacuum lint -r contracts/openapi/vacuum-rules.yaml
  contracts/openapi/v1.yaml --no-update-check` → zero errors
  (417 warnings / 103 informs, pre-existing documentation notes).
- `git diff --check` PASS; `bash scripts/scan-secrets.sh` PASS.
- No worker, publisher, Directory or Android changes; no source
  activation, no PII in Git/logs/fixtures.

## Limits

- Facets are exact matches on source-stated values. There is no text
  search, prefix, range on identifiers, or blank-segment selector;
  blank segments are included when no segment facet applies.
- Coverage counts the leased revision only; cross-revision comparison
  is left to callers holding two leases. Case counts use
  source-stated documents (MEX-22); no issued-case identity beyond the
  document is claimed.
- The only date basis is the document day. Filters on extraction,
  occurrence or finality dates are refused as unavailable, not
  defaulted.
- No device, no production, no deploy and no release acceptance is
  claimed.

Next: MEX-30 (question interpretation) needs MEX-02/23/29; it consumes
this tested read boundary once authorized.

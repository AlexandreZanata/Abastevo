# FQ-07R6A — Explicit current-company association policy

Status: LOCAL_DONE
Validation: PASS

Parent 3a0d6be4, maintained dev. B-BR-FQ-01/02/06/07/11/14/16;
BUC-FQ-02/03/04. This prerequisite defines the pure policy and bounded application
port before source-specific publication/SQL/public journeys consume it.

Before behavior: a fine observation identifies an establishment CNPJ, not an
incident date or a proven historical operator/site. Current Directory evidence
may associate that company observation only with one exact full validated CNPJ
and one current active canonical station, including retained rows for that same
canonical identity. Different canonical identities (even one retired), inactive/
retired-only evidence, wrong branch/base-only and missing identifiers remain
unassociated with their review reasons. No fuzzy name/address/brand/GPS matching,
station creation, legal outcome, quality eligibility or numeric score follows.

Freeze policy version current-company-v1 on every decision. A successful
decision must carry CURRENT_COMPANY_ONLY scope and explicit unknown
historical business window. Its record type accepts no observation/incident/due/
extraction clock; the injected decision day is operational only. The existing
PMQC collection-date/identity policy remains unchanged. Application-level full CNPJ validation and leading zero/alphanumeric
preservation use the existing pure kernel; the stdlib-only domain retains the
existing opaque shape policy rather than duplicating checksum logic. No source
or Directory input is mutated. Decisions are invariant under evidence order.

Application planning validates a complete bounded unique observation set before
I/O, queries at most512 distinct CNPJs per read-only lookup, indexes only requested
exact evidence, and returns one decision per original observation. No partial
plan survives malformed source/evidence, late cancellation, lookup failure or
budget violation. Errors do not expose source rows or port diagnostics. Distinct
CNPJ queries are shared by repeated observations; source/company record counts
remain distinct. The port exposes no writes, operational Directory dates or
source event dates.

Immediate identity/review/permutation/negative/batch/cancellation/failure tests
and affected compilation/static checks are required. No migration, runtime link,
public station claim, source activation, D, deployment or device acceptance is
claimed. Next: compatible immutable scoped storage/publication with real-PostGIS
fences, live reads/revalidation and actual worker wiring.


## Delivered and verified, 2026-10-20

The stdlib-only CompanyObservationRecord/CompanyObservationDecision types are
separate from dated PMQC SourceRecord/Decision/StationLink. There is no source
incident/due/extraction date input. Policy current-company-v1 permits only
CURRENT_COMPANY_ONLY association with an exact current active canonical identity;
HistoricalBusinessWindowKnown remains false. Two canonical histories route
ambiguity even when one is retired. Retained rows for one canonical identity
converge; Current and StationActive must occur together in the same evidence row.
Wrong branch, absence and inactive/retired-only identities assign no station.
Evidence order does not affect decisions; input values are not mutated. This
policy provides no quality/chemistry/finality/score eligibility.

PlanCompanyAssociations validates the complete unique observation/key budget and
kernel-checked canonical full CNPJs before I/O. It deduplicates/sorts only lookup
identifiers (never source observations) and requests at most512 per call through
a read-only port, with at most two retained canonical candidates per identifier.
Foreign/malformed/over-budget evidence refuses the entire plan. Port arguments
are copied; a port cannot rewrite owned source identifiers or subsequent batches.
Repeated company observations retain separate ordered decisions. Only injected
operational decision day enters the policy. Empty valid sources require no lookup.
Late cancellation/lookup failure discards every result and redacts port diagnostics.

RED: both domain/application tests first failed for missing policy/port symbols.
GREEN: six new test functions cover ten resolution scenarios plus invalid input,
leading-zero/alphanumeric/full/checksum identity, source/key/budget validation
before I/O, deduplicated repeated-company records,513-identifier512+1 batching,
late failure/cancellation, foreign/malformed/oversized evidence/branch data,
initial cancellation and hostile port argument mutation. Existing PMQC resolution
and stdlib-only architecture contracts remain unchanged and pass.

From backend/, GOCACHE=/tmp/fq-completion-go-cache:

- go test -race ./internal/modules/quality/application
  ./internal/modules/quality/domain ./internal/modules/kernel -count=1:PASS.
- go build ./internal/modules/quality/... ./cmd/worker:PASS.
- go vet and staticcheck for affected domain/application packages:PASS.
- gofmt, diff/Markdown-link/secret-surface review:PASS.

No SQL/runtime producer/cross-module writes changed, so no database behavior is
claimed and no aggregate/PostGIS rerun was substituted for these pure-policy
checks. INTEGRATION_PENDING. Next FQ-07R6B: append-only compatible scoped company
association ledger and immutable publication identity, tested immediately against
real PostGIS (constraints, migration preservation, races/withdrawal/rollback).
Then Directory runtime/live fences, current-versus-frozen reads/revalidation,
linked Android journeys, actual FQ-06 XLSX and full FQ-16/17/protected delivery.
D, import activation and device/production acceptance remain held/owed.

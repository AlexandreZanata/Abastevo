# FQ-07R6E1 — Lease-bound current-company read ports

Status: LOCAL_DONE
Validation: PASS

Parent 0e68021b, maintained dev. B-BR-FQ-01/02/06/07/11/14/16;
BUC-FQ-02/03/04/09. Private read prerequisite before actual HTTP/client exposure.

Before behavior: a station-scoped fine read can serve only same-revision bound
CURRENT_COMPANY_ONLY observations whose exact full source CNPJ still resolves to
that requested current active canonical station, without retained ambiguity.
There is no fallback to global events, fuzzy/base matching or dated PMQC links.
Missing old association evidence is explicitly unavailable; a complete census
with no current matched association is empty, never proof of fuel quality or no
violations. Historical business-window knowledge stays false. Generic global
source/history reads remain compatible and reachable without station attribution.

Resolve one frozen source lease and private association scope per request. Read
only candidate full CNPJs through quality-owned queries; fresh Directory evidence
and transaction guards arrive through explicit root-composed read/lock ports.
Fingerprint the allowed company set/policy/source/station for later HTTP cursor/
ETag binding, without disclosing CNPJs. Page/detail use that exact frozen scope.
Before returning positive rows, derive actual bindings from the allowed set and
hold Directory's existing guard locks through the read transaction; retirement/
status/ambiguity between planning and row read fails closed. No cross-module SQL
or writes, synthetic incident dates, business ownership or numeric score follows.

Explicit historical source revision reads still require current exact association
for station-scoped attribution; withdrawn/missing/unassociated/wrong-station facts
never leak into that station's page/detail. Global frozen source facts remain
available under their original source-only contract. Unknown policy/bad census,
missing ports and outages refuse safely. Empty source/scoped result remains typed.

Immediate real PostGIS source/station/revision/status/keyset/detail isolation,
actual Directory retirement/status/race/guard tests, frozen source/history,
unknown/missing evidence, no-source/empty/failure/negative tests and scoped
EXPLAIN checks are mandatory. No public API/Android/activation/deployment/D claim.
Next FQ-07R6E2: actual HTTP/OpenAPI, station/current-set sealed cursors and ETags,
privacy/compatibility and root wiring, then revalidation and Android integration.

## Delivered and verified, 2026-10-20

CompanyScope resolves one frozen sanctions/sanction-v1 lease per request and
seals the allowed full-CNPJ set into a fingerprint without disclosing CNPJs.
Page/detail derive bindings from that exact frozen scope and hold Directory's
existing transaction guard through the read transaction. Retirement, successor
ambiguity (retire plus new binding) and station suspension between scope
resolution and row read refuse the station page and detail with no leaked rows.
The source-global frozen page and detail stay reachable without station
attribution. Request-local AtRevision preserves composed identity ports, and
old lease/scope pairs refuse a newer revision. No cross-module SQL or writes,
no synthetic incident dates, no ownership or score inference.

Tests prove station/source/revision isolation, frozen keyset traversal,
cross-station fingerprint separation, foreign-detail refusal, correction
lease binding, frozen AtRevision reads, and fail-closed page/detail on
retire/successor/suspend with the global history still serving all four
synthetic observations. Reader shares no Directory SQL; View carries no CNPJs.

## Commands and results

From backend/ in .worktrees/main-reconciliation, local loopback 5434
disposable PostGIS, ANPFUEL_DATABASE_URL and ANPFUEL_TEST_DATABASE_URL on the
shared dev database:

- go run ./cmd/migrate: schema already current (through 109).
- go test -race -count=1 -tags=integration ./internal/modules/quality/adapters/read -v: PASS 9.8s, 14 tests including TestCompanyScopedReadIsolatesSourceStationAndFrozenKeysets and TestCompanyScopedReadFailsClosedOnDirectoryChange (retire/successor/status).
- go test -race -count=1 -tags=integration ./internal/modules/quality/adapters -run 'TestCompany|TestScopedFine': PASS 11.3s.
- gofmt -l ./internal/modules/quality/adapters/read; go vet ./internal/modules/quality/adapters/read: clean.
- sqlc vet from backend/: clean; sqlc generate stable with no generated diff.
- git diff --check: clean; secret/PII grep over the seven task files: no secret markers and no real CNPJs outside synthetic test fixtures.

INTEGRATION_PENDING. Import/discovery controls remain off; no public station
filter exposure, HTTP/OpenAPI sealed cursors/ETags, revalidation, Android
journeys, activation, deployment or D certification follows. Next FQ-07R6E2.

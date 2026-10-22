# MEX-19 — Scoped sanctions HTTP and composition

Status: LOCAL_DONE
Validation: PASS
Integration: INTEGRATION_PENDING

Parent: `f0b8b1a9`, maintained dev. Date: 2026-10-22.
B-BR-MEX-11/13/17; BUC-MEX-08/09.

Before behavior: an optional `station_id` on sanctions page/detail selects only
the frozen exact current-company scope and never falls back to global rows.
Unsupported action station filters and unknown/duplicate/empty parameters are
refused explicitly. The source revision, station, exact filters, limit and a
fresh private company-set fingerprint seal continuation and ETag identity.
Fresh scope and actual guarded rows are resolved before any `304`. Public DTOs
contain no source CNPJ or private proof. Company-only attribution and an unknown
historical business window are explicit. Global source/history endpoints remain
compatible. Root composes narrow Directory evidence/transaction-guard ports.
A separate default-off company read switch disables only scoped access, without
toggling source ingest or global reads. Real HTTP/PostGIS
page/detail/revision/withdrawal/cache/cursor/filter/kill/outage/privacy and
actual OpenAPI validation are mandatory; no source activation.

Risk: public factual attribution, private identity leakage, stale `304`, cursor
or cache confusion. Recovery: disable
`ANPFUEL_QUALITY_COMPANY_READS_ENABLED`; global reads and every source/import
switch remain independent. No schema, migration, dependency or infrastructure
change is introduced.

## Delivered

- `quality/adapters/http/company.go`: strict event-query validation (unknown,
  duplicate, empty `station_id`/`revision_id`, `station_id` on actions refused),
  optional scoped page/detail, fresh scope resolution, cursor/ETag binding and
  `public, max-age=0, must-revalidate` scoped caching.
- `quality/adapters/read/company_scope.go`: frozen current-company scope with a
  private fingerprint, revision lock plus Directory identity guard inside the
  read transaction, and no global fallback for empty/unavailable scopes.
- `quality/adapters/http/http.go` and `history.go`: request-local revision
  selection, explicit missing-port refusal and scoped routing before row reads.
- `cmd/api/quality_company_reads.go`: root composition of the typed Directory
  evidence/guard ports; `main.go` uses the factory.
- `platform/config`: `ANPFUEL_QUALITY_COMPANY_READS_ENABLED`, independent and
  default `false`; global reads remain enabled.
- `contracts/openapi/v1.yaml`: optional `station_id`, scoped `304/404`,
  `QualityCompanyScope`, and route descriptions; global responses unchanged.
- `backend/README.md` and `backend/.env.example`: operator documentation for the
  independent default-off switch and scoped cache/evidence policy.
- Tests: `mex19_integration_test.go`, `company_integration_test.go`,
  `cmd/api/quality_company_reads_integration_test.go`, extended `config_test.go`.

## Finalization regressions added

- A detail revalidation with the pre-change ETag after Directory retirement,
  suspension or successor resolution answers `404`, never a cached `304`.
- A real `enforcement-v1` revision cannot be selected as a sanctions revision
  on scoped page, detail or the global sanctions read (`404`).
- A canonical station with Directory evidence but no current-company observation
  yields `200` with zero items, `available=true`,
  `reason=NO_CURRENT_COMPANY_MATCH`, `historical_business_window_known=false`;
  its detail is `404` and no global row is substituted.

## Actual checks

From this worktree (2026-10-22, parent `f0b8b1a9`):

- RED before the implementation: five actual HTTP filter assertions failed
  (`/tmp/bfp-mex19-red.log`).
- Targeted MEX-19 HTTP/PostGIS suite PASS (`6.329s`): scoped isolation,
  mutation-sealed cursors, cache, privacy, OpenAPI, retirement/status/successor,
  withdrawal, missing/failing ports, independent kill switch, global
  compatibility, foreign-stream revision and empty scope.
- Final affected matrix, one run, loopback disposable PostGIS
  (`ANPFUEL_TEST_DATABASE_URL=...127.0.0.1:5434...`) with the four pinned
  preparer executables at mode `755`:
  `go test -race -count=1 -p 2 -tags=integration ./internal/modules/quality/...
  ./internal/modules/directory/adapters ./cmd/api ./internal/platform/config
  ./internal/platform/apicontract` PASS, 12 packages:
  quality adapters `58.280s`, http `25.969s`, jobs `16.600s`, native `1.186s`,
  prepared `1.120s`, read `33.028s`, application `1.094s`, domain `1.015s`,
  Directory adapters `23.831s`, `cmd/api` `8.361s`, config `1.011s`,
  apicontract `5.995s`.
- Production factory exercised over actual loopback HTTP/PostGIS
  (`/tmp/bfp-mex19-root-http.log`), including a retired binding becoming empty.
- `go vet` and `GOTOOLCHAIN=go1.27.1 staticcheck` on quality/`cmd/api`/config
  PASS; runtime/tests/scanner remain Go `1.27.3`.
- `sqlc vet` and `sqlc diff` PASS (no schema/query drift).
- `vacuum lint -r contracts/openapi/vacuum-rules.yaml contracts/openapi/v1.yaml
  --no-update-check` PASS: zero errors, 392 retained warnings and 103 informs
  (existing; this is not a zero-warning result).
- `gofmt -l` on the affected Go trees clean; `git diff --check` PASS;
  `bash scripts/scan-secrets.sh` PASS. Synthetic CNPJs/fixtures were reviewed;
  no real PII, private coordinate, credential or source file is committed.
- Android inputs are unchanged by MEX-19, so the previously accepted JVM/APK/
  lint evidence remains applicable; no Android rerun was required.

## Limits

- The company read switch stays default `false`; no staging/production
  activation, deploy, tag or source import was performed. Signed
  contribution-to-feed, broad manual/device acceptance and production
  measurements remain OWED.
- Current-company-only scope does not prove historical operation at a site and
  unknown remains distinct from clean. National coverage is not certified.
- OpenAPI warnings/informs are pre-existing and retained.
- MEX-20 and EE-04–10 remain blocked on their owning source/review gates.

Next: publish tested `dev`, open the protected dev→main batch PR, require the
current-head/base checks and merge only after all required checks pass. Wiki is
published once only after that eventual merge, from the actually merged SHA.

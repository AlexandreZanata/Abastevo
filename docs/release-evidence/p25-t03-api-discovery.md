# P25-T03 — Paginated and targeted ANP API discovery

Status: LOCAL_DONE on `codex/phase-25-national-registry`. Task: P25-T03.
Binds B-BR-D02/D03/D05/D06/D10 and BUC-D01. Discovery only: typed page
traversal + targeted lookup into the same staging ledger; no fetch in
public handlers; no live ANP call from this sandbox.

## Behavior (TDD, critical-first)

- `registry/api.go` `Discover` with frozen `ProductionAPIConfig`
  (host `revendedoresapi.anp.gov.br`, page 100, 50 pages / 1,000
  requests, 1 MB, 30 s): targeted CNPJ lookup or scoped
  (UF/municipality) traversal following `nextCursor`; global quotas;
  `429`/5xx bounded retries (injectable sleep); 404 on targeted lookup
  completes honestly empty (0 accepted, no corruption); other
  non-200, malformed pages and transport failures fail the run
  explicitly; quota exhaustion quarantines (`page_quota`/
  `request_quota`) so partial traversal never masquerades as a full
  snapshot. Unsupported CNPJ forms fail before any network call.
- Module-owned transport (no cross-module adapter import, per
  ownership rules): exact-host allowlist, HTTPS-only (HTTP solely for
  loopback tests), dial-time IP inspection (public unicast; loopback
  only under test override — no rebinding gap), redirects re-validated
  (max 3), per-request byte caps.
- `parseAPIRecord` reuses the frozen T01 policy (kernel CNPJ text,
  status mapping, eligibility) plus coordinate honesty: out-of-range/
  NaN rejected, non-WGS84/SIRGAS2000 CRS rejected, reviewed-without-
  coordinates downgraded to unknown, coordinates never upgrade an
  unknown wire. Row sha256 over raw item JSON; same
  `(source, source_key, checksum)` idempotence with source
  `registry-api`.
- Fixed from real failures: test-helper/type name collisions,
  `EffectiveDate` now carried on assertions (API `publicadoEm`
  parsed, CSV stays NULL).

## Validation

- Unit 8/8 PASS (`-race`): page + cursor-duplicate accounting,
  bad-record honesty (short CNPJ, bad CRS, out-of-range lat),
  429-retry-then-success, off-allowlist refusal, targeted query shape
  + unsupported-CNPJ pre-network refusal, fixture shape conformance.
- Integration (real PostGIS, `-race`): 2-page traversal stages 2,
  replay converges, 1 complete run; targeted 404 completes empty.
  Fresh disposable DBs (migration incl. 000031).
- Regression: full `directory/...` unit + `sqlc vet` PASS;
  `vacuum lint` + `apicontract` PASS (contracts untouched).
- `git diff --check` PASS; `scan-secrets.sh` PASS (synthetic CNPJs
  only; no URLs with credentials).

## Limits and next

- No live ANP fetch performed (sandbox egress + frozen production
  host awaits T02-style reconfirmation at implementation opening);
  national throughput calibration stays P29-owned.
- Next: P25-T04 canonical reconciliation and zero-price public reads.

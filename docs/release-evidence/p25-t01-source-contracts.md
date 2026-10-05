# P25-T01 — Source contracts, eligibility and registry fixtures

Status: LOCAL_DONE on `codex/phase-25-national-registry`. Task: P25-T01.
Binds STATION_CATALOG B-BR-D01–D12 and BUC-D01/D05/D08. Semantics
freeze (docs + executable policy + versioned fixtures); no staging,
reconciliation, migration or API change.

## Frozen contracts

- `docs/product/REGISTRY_SOURCES.md`: approved HTTPS origins,
  redirect/DNS/IP validation, conditional fetch + checksums, quotas
  (CSV daily full; API targeted ≤1,000 lookups/day), provisional
  header-name field contract (reconfirm live at T02), frozen
  authorization/operation/eligibility/location states + transitions,
  exact-match auto-resolve vs quarantine precedence, atomic
  publication boundaries, `[CALIBRATE]` caps/thresholds/budgets and
  the fixture manifest. No OSM/partner copying in P25.
- Executable policy `backend/.../directory/adapters/registry/`:
  `ValidateHeader` (required names by case-insensitive name match;
  unknown columns flagged), `QuarantineRun` (any missing/unknown →
  quarantine, previous catalog preserved), `ValidCNPJText` (kernel
  alphanumeric program: letters/leading zeros preserved, check digits
  verified), `MapStatus` (ATIVA→authorized, SUSPENSA→suspended,
  CANCELADA→revoked, else honest unknown), `DecideEligibility`
  (eligible only with exact identity + structured address +
  authorization, never once withdrawn).
- Fixtures `contracts/testdata/registry/` v1: manifest + 10-row CSV
  (numeric/alphanumeric/leading-zero/check-valid CNPJs, duplicate row,
  unknown column, invalid + unknown-status rows) + 3-record API page
  (reviewed/city-centroid/unknown). All synthetic `[P25-TEST]`
  markers; no real personal data; no emails/phones asserted by test.

## Validation

- RED→GREEN: `registry` package tests failed to build before
  `policy.go`/`fixtures.go` existed; GREEN 6/6 after (header
  require/unknown/missing, CNPJ preserve/reject, status freeze,
  eligibility allow/deny matrix, manifest markers + no-PII surface).
- A made-up CNPJ (`00428184000104`) failed check digits mid-task and
  was replaced by generated check-valid synthetic values (recorded
  honestly; the generator was a temp throwaway, not committed).
- `gofmt` clean; `go vet` on the package PASS (via `go test` build);
  `git diff --check` PASS; `scan-secrets.sh` PASS.
- Docs: local links/anchors + whitespace/secret review PASS. No
  runtime gate inferred from fixtures or docs.

## Limits and next

- Provisional column names and `[CALIBRATE]` budgets must be
  reconfirmed/measured at T02/P29 — they are explicit hypotheses.
- Next: P25-T02 bounded CSV snapshot staging on this branch.

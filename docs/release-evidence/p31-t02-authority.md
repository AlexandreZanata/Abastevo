# P31-T02 — Operating-company and corporate authority verification

Status: LOCAL_DONE on `codex/phase-31-verified-representation`. Task: P31-T02.
New `stationprofile/authority` package (pure, stdlib-only): applicant
linkage + sufficient powers for the exact branch. Review/grants
consume it in P31-T03; Receita/QSA/corporate-act retrieval stays an
explicit OPEN access item.

## Behavior (TDD)

- `Verify` over independently obtained `CompanyFacts`: document-bound
  identity (empty document denies — homonyms never resolve),
  branch establishment, recorded position with `manage` powers,
  branch coverage, mandate currency, joint-signature routing,
  role/claim consistency (managers cannot claim administration).
- Denials are terminal facts (custodian e-CNPJ holder, powerless
  shareholder, name-only match, branch mismatch, expiry, wrong role);
  gaps (unknown branch/company, missing co-signer) route to review as
  insufficient — never auto-approval, never weak fallback.

## Validation

- Unit (`-race`) 4/4: administrator approval, custodian/shareholder/
  homonym denials, mismatch/expiry/joint cases, unknown-company
  deferral — PASS.
- `gofmt`/`go vet` clean.
- `git diff --check` PASS; `scan-secrets.sh` PASS (synthetic
  documents only).

## Limits and next

- OPEN (unchanged): Receita/QSA access path, corporate-act retrieval
  procedure, joint/matrix-filial source rules (P30-T01 OPEN items).
- Next: P31-T03 restricted review and atomic scoped grants.

# P31-T01 — Independent signature and declaration verification

Status: LOCAL_DONE on `codex/phase-31-verified-representation`. Task: P31-T01.
Binds B-BR-P04–P08/P16 and BUC-P03. Exact content binding, chain
building, validity and explicit revocation; unknown revocation stays
indeterminate; synthetic PKI proves logic only — real ICP-Brasil/gov.br
trust proof needs the provisioned environment (recorded, not claimed).

## Behavior (TDD, critical-first)

- `stationprofile/verify` package (stdlib `crypto/x509` only, no new
  dependency): digest match → declaration containment → PEM chain
  build against injected roots → validity window → revocation source.
  Outcomes valid/invalid/indeterminate with safe reasons; no network,
  no clock beyond policy time, no lock held. PDF ByteRange/CMS
  extraction stays a documented manual operator process (official
  tools on exact bytes) — this package verifies extracted content,
  never screenshots or validation reports.
- Declaration lifecycle (sqlc, no migration — columns existed):
  `GetDeclaration`, `BumpAttempts`, `ConsumeDeclaration`,
  `ExpireDeclaration`, `GetProof`, `SetProofStatus`.
- Application `VerifyProof`: received-only proofs, open-claim and
  active-binding checks, TTL enforcement, bytes + operator-supplied
  chain resolution before any write; terminal results consume the
  binding and stamp verified/rejected; indeterminate bumps without
  consuming and expires the declaration only at the frozen cap of 5
  (attempts never reset, replays cannot manufacture tries).

## Validation

- Unit (`-race`): 8 vector tests (valid binding, tamper, expiry,
  revocation, unknown-revocation indeterminacy, forgery, test-roots
  rejected in production mode, missing material) + service tests
  (valid consume, tamper path, indeterminate bumps → exhaustion
  invalidation, closed-proof refusal) — PASS.
- Integration (real PostGIS, `-race`): open → submit → verify valid
  with synthetic PKI; proof row `verified`, declaration consumed.
  Fresh disposable DBs.
- Fixed from real failures (not weakened): typed-nil interface
  panic, JSON test interpolation, non-UUID ids, helper gaps,
  expiry-clock alignment.
- `go vet` clean; `gofmt` clean.
- `git diff --check` PASS; `scan-secrets.sh` PASS (synthetic PKI
  generated at runtime; no committed keys/certs).

## Limits and next

- OPEN: approved trust roots + revocation source + CMS extraction
  operator process for production (P30-T01 OPEN items, unchanged);
  fake roots never prove real trust.
- Next: P31-T02 operating-company and corporate authority verification.

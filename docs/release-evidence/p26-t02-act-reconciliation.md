# P26-T02 — ANP acts and chronological reconciliation

Status: LOCAL_DONE on `codex/phase-26-regulatory-discovery`. Task: P26-T02.
Binds B-BR-D02–D05/D10 and BUC-D02/D06. Deterministic classification +
chronology + Directory reconciliation; correction chains stay review
work (T03); no live source; no model interpretation anywhere.

## Behavior

- `dou/classify.go`: accent-folded keyword rules — revocation wins,
  then explicit corrections, then grants with fuel-retail context;
  fuel context without a verb is ambiguous (quarantine), anything else
  unrelated. `ExtractCNPJs` validates every candidate with the kernel
  program (formatted accepted, invalid dropped silently as
  non-evidence).
- `StageActs(edition, acts)`: only single-identifier grants and
  revocations stage (auth/pending-eligibility, edition date as
  effective date, row sha256); corrections (chain resolution),
  multi-identifier acts, unrelated and ambiguous acts skip honestly
  with counts. Grant dates can never become inauguration dates: no
  opening-date output exists on assertions, stations or reads.
- Staged DOU assertions reconcile through the existing
  `ReconcileRun` + canonicalizer: same CNPJ converges, revocation
  projects `revoked` on the same identity, unknown stays pending for
  the P27 exact-match verification.
- Migration `000033_dou_run_sources.sql` (append-only constraint
  evolution, edited pre-publication on this branch): run ledger admits
  `dou-editions`/`dou-acts`.

## Validation

- Unit 6/6 PASS (`-race`): grant/correction/revocation,
  unrelated/ambiguous quarantine, CNPJ extract/validate/drop +
  formatted, staged assertion shape (key/auth/effective-date), skip
  matrix (ambiguous + multi).
- Integration (real PostGIS, `-race`): grant edition stages +
  reconciles to one station; later revocation edition revokes the
  same identity (`status = revoked`, identity unchanged, detail-
  readable). Fresh disposable DBs (migrations incl. 000032/000033).
- Fixed from real failures (not weakened): test-fixture wording that
  matched the grant rule, `dou-acts` CHECK membership, fake-UUID FK
  violation (real canonicalizer + Reader now prove identity).
- Regression: `go vet` clean; full `directory/...` unit PASS.
- `git diff --check` PASS; `scan-secrets.sh` PASS (synthetic CNPJs).

## Limits and next

- BLOCKED_ACCESS: live INLABS fetch unverified; fixture schema
  provisional. Correction chains + operator review protocol are T03.
- Next: P26-T03 regulatory review, catch-up and phase acceptance.

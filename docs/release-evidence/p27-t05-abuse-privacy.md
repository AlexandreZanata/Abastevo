# P27-T05 — Intake abuse, privacy and lifecycle acceptance

Status: LOCAL_DONE on `codex/phase-27-station-intake`. Task: P27-T05.
Binds B-BR-D01–D12 and BUC-D03–D06. Abuse/privacy/lifecycle proven
across intake, verify, account, moderation, evidence and privacy
suites; public projections verified free of private data.

## Behavior

- Author liveness gate: `VerifyPorts.AccountLive` is consulted by
  both automation and human review — erased/suspended authors defer
  (automation) or fail closed with `ErrAccountGone` (review);
  reactivation unblocks without data rewrite. Production wiring reads
  the account status via the existing PG store (composition root
  only, no cross-module imports).
- Abuse proven: 20/day quota boundary enforced at SQL (20 accepted,
  21st refused); concurrent same-key submits converge on one row;
  concurrent reviewers converge 1-0 with the loser closed; revoked
  sessions denied at the intake gate (401/403); offline retries reuse
  the stable key (no duplicate visible station).
- Privacy: suggestions are owner-or-reviewer private (IDOR-safe reads,
  `no-store` everywhere); decided rows follow account-audit retention
  parity; pending rows of erased authors stop processing (gate
  above). A dedicated retention sweeper is deferred and explicit —
  no silent expiry invented.
- Lifecycle: suggest → exact-match verify → approved station
  searchable/detail-readable with zero prices; revocation sticks;
  pins stay unknown-quality and unprojected; public reads carry no
  private data and no price/trust coupling (proven by the T03/T04
  suites rerun below).
- Operator review capacity: pending queue via `ListPendingSuggestions`
  (bounded batches); arrival handling time measured at review time —
  no invented headcount (staffing checklist stays P23-T03-owned).

## Validation

- NEW: author-liveness unit (defer + fail-closed + pending preserved)
  and integration (suspended author stays pending against a real
  accounts row); quota-boundary integration (20/21); all PASS
  (`-race`, real PostGIS).
- Cross-module: account + moderation + evidence + privacy unit suites
  PASS; full `directory/...` unit + integration PASS; Android
  suggest/stations suites PASS; `go build ./...` PASS; `vacuum lint`
  + `apicontract` + `check-compat.sh` PASS.
- `git diff --check` PASS; `scan-secrets.sh` PASS.
- G27 specialized exit recorded once below; no device runs per
  directive; no production certification inferred.

## Limits and next

- Retention sweeper + review UI/CLI consume the frozen ports when
  authorized; device-level intake proof joins the end manual batch.
- Next: P27 phase exit (LOCAL_DONE checkpoint + evidence), then P29
  (`codex/phase-29-catalog-acceptance` via `--from-checkpoint`).
  P28 stays optional (selected source only).

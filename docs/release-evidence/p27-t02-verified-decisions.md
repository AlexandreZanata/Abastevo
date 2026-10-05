# P27-T02 — Verified decisions, corrections and reviewed locations

Status: LOCAL_DONE on `codex/phase-27-station-intake`. Task: P27-T02.
Binds B-BR-D02/D04–D10 and BUC-D03/D04/D06. Exact-match automation +
audited review + unknown-quality pins (never projected); corrections
and appeals stay explicit; no arbitrary pin becomes canonical.

## Behavior (TDD, critical-first)

- Migration `000035_suggestion_decisions.sql` (append-only): audited
  decision rows (approved/rejected + reason + reviewer + optional
  station link); `DecideSuggestion` transitions pending-only in SQL.
- Application `verify.go`: `AutoVerify` approves only exact official
  matches (proposal CNPJ already in staged official assertions from a
  complete run, same municipality/state); unknown stations, conflicts
  and doubt stay pending (never auto-rejection); decided replays are
  harmless no-ops; unknown-resolver errors defer while transport
  errors surface. `Decide` enforces reviewer + reason, pending-only
  transitions (concurrent reviewers converge on the SQL guard),
  pins user coordinates as unknown-quality revisions. Appeal = a new
  suggestion; reviewers never edit proposals.
- Adapters: `IntakeStore` decision methods; `jobs.VerifySweep`
  (`suggestion-verify` v1, batch 25) running automation over pending
  batches; worker wiring with official-first resolution (unknown CNPJs
  defer instead of minting stations from user input — caught and fixed
  during implementation) + unknown-quality pin recording + hourly
  enabled schedule (exact-match only, cheap no-op when empty).
- sqlc `FindOfficialAssertion` (complete runs only),
  `CreateDecision`/`DecideSuggestion`/`ListPendingSuggestions`;
  `sqlc vet` + `generate` clean.

## Validation

- Unit (`-race`): auto-verify approve/defer/conflict/replay,
  reviewer/reason/pending/closed/missing rules, sweep
  approve-and-defer + envelope refusals — all PASS.
- Integration (real PostGIS, `-race`): exact match approves + links
  with the pin stored as the single `unknown` revision and no
  canonical projection; conflicting municipality stays pending;
  concurrent reviewers converge 1-0 with the loser closed. Fresh
  disposable DBs (migrations incl. 000034/000035).
- Fixed from real failures (not weakened): unknown-station error
  semantics, non-UUID test station FK violation (real canonicalizer +
  Reader now prove identity), race flipped decision ids.
- Regression: full `directory/...` unit + integration PASS; `go vet`
  clean; `go build ./...` PASS (worker + api).
- `git diff --check` PASS; `scan-secrets.sh` PASS.

## Limits and next

- Human review UI/CLI consumes the pending queue via service ports;
  evidence revalidation at review time is a runbook step (operator
  checks expiry before approving photo-backed cases).
- Next: P27-T03 server catalog, offline cache and canonical action
  targets (Android).

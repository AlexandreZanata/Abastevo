# P26-T03 — Regulatory review, catch-up and phase acceptance

Status: LOCAL_DONE on `codex/phase-26-regulatory-discovery`. Task: P26-T03.
Binds B-BR-D03–D05/D10/D12 and BUC-D02/D04/D06. Review edges +
catch-up + runbook; correction persistence query frozen for review
tooling; G26 stays BLOCKED on live access per plan (recorded, not
pretended).

## Behavior

- `dou/review.go`: `MissingDates` (weekday backfill from checkpoints,
  weekends never listed, bad ranges empty), `LinkCorrection`
  (supersession edge refusing unrelated/self links; ambiguous chains
  stay quarantined), republication convergence by checksum on the same
  identity (traceable distinct rows, one station).
- sqlc `SetAssertionSuperseded` (NULL-guarded) for review tooling;
  `sqlc vet` + `generate` clean.
- `docs/operator/dou-editions.md`: review queue paths, catch-up,
  freshness/lag + escalation, conflicting-fact routing, G26 BLOCKED
  state.
- Review concurrency/role denial reuse moderation ports (no new
  authority); late revocation projects on the converged identity
  (proven T02 integration).

## Validation

- Unit 4/4 PASS (`-race`): weekday catch-up + seen-skip, bad-range
  emptiness, correction link + refusals, republication checksum
  convergence.
- Regression: full `directory/...` unit PASS; `dou` + `registry`
  suites PASS; `go vet` clean.
- `git diff --check` PASS; `scan-secrets.sh` PASS.
- G26 specialized exit: recorded as BLOCKED (live INLABS access
  unavailable); correct recovered state proven in isolation only.

## Limits and next

- Next: P26 phase exit (LOCAL_DONE checkpoint + evidence), then P27
  (`codex/phase-27-station-intake` via `--from-checkpoint`).

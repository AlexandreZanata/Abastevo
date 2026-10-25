# PAY-22 — batch integration to protected main

Status: INTEGRATION_PENDING (PR open, CI pending)
Validation: PASS (local candidate gates)
Date: 2026-10-25
Branch: `codex/payments-stripe-asaas` integrated into `dev`
PR: `AlexandreZanata/Abastevo#150` (dev → main, ready)
Heads: dev `6e5e8518` onto main `c82823cb` (PR #147).

Objective: coordinate latest dev/main, merge owned payment changes
normally, and deliver through one cumulative protected dev → main PR
with a guarded finish. No force, no rebase of published history, no
admin bypass, no direct-main development.

## B-BR/BUC addressed before implementation

- B-BR-PAY-24: local proof and remote activation stay distinct; this
  record claims integration progress, not a merge.
- BUC-PAY-10: the candidate preserves operate/recover behavior across
  the dev union (maps, Android test-only, payments).

## Implementation

- Verified `origin/dev` (`721582b8`) is fully contained in the payments
  branch, so integration is a fast-forward merge with no conflict and
  no foreign code imported: 56 commits (maps delivery, Android
  test-only, payments PAY-00–21 plus two CI-hardening fixups).
- Merged `codex/payments-stripe-asaas` into `dev` in an isolated
  worktree (main checkout and all other work branches untouched),
  pushed `721582b8..6e5e8518` fast-forward only.
- Local CI findings fixed in new commits on the front (secret scanner
  flagged a live-key literal in the sandbox guard and a live-looking
  value in PAY-21 evidence; both redacted without weakening refusal).
- Opened PR #150 (ready, not draft) with real identifiers, included
  phases, validation results and explicitly open obligations. No
  `Closes`: no eligible issue meets completion criteria; PAY-21 stays
  pending by contract.

## Validation performed

On the exact PR head (`6e5e8518`), worktree `/tmp/opencode/pay22-dev`:

```sh
make quick-verify
ANPFUEL_TEST_DATABASE_URL=<local disposable DSN> \
  go test -race -tags=integration -count=1 ./internal/modules/payments/... ./internal/modules/subscription/...
ANPFUEL_TEST_DATABASE_URL=<local disposable DSN> \
  go test -race -tags=integration -count=1 ./cmd/api/
```

Results:

- `make quick-verify`: ok (45s, selection=full, head pinned).
- Race payments/subscription suites PASS; race `cmd/api` suite PASS.
- Remote checks at PR creation: Quick verification, integration, test
  and fast jobs queued (2026-10-25T12:37:58Z). No polling: merge awaits
  the required check through the guarded finish helper.

## Remaining obligations

- Await required Quick verification (plus any other enabled signal) on
  PR #150; fix failures in new commits; changed head/base invalidates
  this proof.
- Guarded finish (`git-flow.sh finish --base origin/main --pr 150
  --required "Quick verification"`), ancestry verification, dev
  retention/sync, eligible issue closure through the merged PR.
- PAY-21 sandbox proof (credentials), PAY-23 VPS deployment, wiki
  mirror per merged batch — all separately gated, none claimed here.

Next action: complete the guarded merge on green CI, or report and fix
red CI in new commits.

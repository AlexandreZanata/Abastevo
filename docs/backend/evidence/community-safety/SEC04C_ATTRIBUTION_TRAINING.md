# SEC04-C — Attribution, aggregates and training split

Date: 2026-10-11. Status: LOCAL_DONE. Scope: credential
minimization + social attribution/aggregates on real SQL +
training/operational separation — no worker-scope changes, no
local-queue sweep (D). Head: maintained `dev` + this task.
Plan: [remediation plan](../../../planning/SECURITY_REMEDIATION_PLAN_2026-10-11.md)
(SEC04, AUD-05). Contract: B-BR-A05/F02…F06, B-BR-016; training
consent (CS09–CS11). Constraint: other authors' replies/history
survive the author; consent withdrawal rides revocation, never
operational expiry; secrets never persist past the owner.

Validation: PASS

## Changes

- `db/queries/account/accounts.sql` (+ regenerated sqlc, vet
  clean): `DeleteAccountKeys`. MemStore + PGStore `DeleteAccount`
  minimize key verifiers (lookup + salted verifier rows) in the
  same atomic transaction; the audit row and alias stay, the
  secrets that authenticate as the owner go.
- Real-SQL proofs: key login after delete is `key-invalid`,
  both lookups miss, and the handle is free for a new account
  (unit + PG); social composition (ratings/comments/reply/vote
  across two authors, erase one): erased rows leave public reads,
  surviving reply stays verbatim, rating stats rebuild to the
  remaining author, foreign vote denominators survive, re-erase
  converges zero; training consent/derivative counts identical
  before/after social erasure (separate paths, same database).

## Validation (actual)

- `go test ./internal/modules/account/... ./internal/modules/feedback/... ./internal/modules/privacy/...`
  green (incl. new minimization unit). `-race -tags=integration`
  green: account adapters (key-minimization proof) + feedback
  adapters (social composition proof).
- `go build ./...`, `go vet`, `gofmt -l` clean, `sqlc vet`
  clean, `git diff --check` clean, `scan-secrets.sh` passed
  (synthetic handles only).
- RED pre-GREEN: pre-fix key login survived deletion
  (verifier rows retained — the minimization gap); social
  composition failed before the surviving-reply assertion was
  pinned against tombstoned parents.

## Limits / owed

- Training-derivative purge executes in the worker privacy scope;
  this task proves separation (consent/inventory untouched by
  social erasure) and consent-keyed revocation (existing CS10D
  proofs), not a live worker run from an account deletion.
- Full account→reads→restore evidence with worker execution,
  failure injection and ledger replay: SEC04-D. No VPS/phone
  change; no production claim.

Next microtask: SEC04-D local cleanup + replay proofs.

# SEC07-A — Proof-based refresh reuse detection

Date: 2026-10-11. Status: LOCAL_DONE. Scope: refresh invalid-vs-reuse
split with bounded hashed rotation history (no JWKS/Android change).
Head: maintained `dev` + this task.
Plan: [remediation plan](../../../planning/SECURITY_REMEDIATION_PLAN_2026-10-11.md)
(SEC07, AUD-08). Contract: fail-closed rotation; proven replay revokes
the family; random guesses never revoke; history bounded at
`domain.MaxRefreshHistory = 8` salted verifiers, never plaintext.
Constraint: history beyond the bound ages out to invalid (documented,
not silent); no token/family logging anywhere.

Validation: PASS

## Changes

- `account/domain`: `ErrSessionInvalid` + `VerdictCode`
  `session-invalid`; `RotatedVerifier{Salt,Hash}` + `MaxRefreshHistory=8`.
- `account/application`: `Store.RefreshHistory` port; `Refresh()`
  mismatch path checks the hashed history with the token hasher —
  history hit revokes + `ErrSessionReuse`, otherwise `ErrSessionInvalid`
  with the live family untouched. `MemStore` archives the superseded
  verifier newest-first, pruned to the bound.
- `account/adapters/pgstore`: migration `000060`
  `account_session_refresh_history` (PK family+hash, family index,
  cascade delete); sqlc `Insert/List/PruneRefreshHistory` (vet+generate
  clean, stanza extended); `Rotate` archives+prunes in the same tx;
  `RefreshHistory` lists newest-first capped at the bound.
- `account/adapters/http`: `session-invalid` → 401
  `account.session-invalid` (reuse/revoked/expired mappings kept).
- Tests: unit `sec07a_refresh_reuse_test.go` (invalid-vs-reuse,
  bound, 8-way parallel) + real-PostGIS
  `sec07a_refresh_integration_test.go` (invalid-vs-reuse, 8-way
  parallel, 12-rotation bound). Pre-existing reuse/revoke/expired
  suites re-green.

## Validation (actual)

- `go test ./internal/modules/account/...` green (incl. 3 new unit).
- Real PostGIS `-race -tags=integration` full account tree green
  (incl. 2 new integration, pre-existing reuse/revoke suites).
- `go build ./...`, `go vet ./...`, `gofmt -l` (clean), `sqlc vet`,
  `git diff --check` clean; secret scan passed.
- RED pre-GREEN: pre-fix `Refresh()` revoked on ANY hash mismatch
  (single `RevokeFamily` line); the new invalid-guess test inverts
  that characterization — random token now refuses without revoking
  while the superseded token still revokes.

## Limits / owed

- Verifiers older than the 8-deep window refuse as invalid without
  revoking (bounded-memory trade-off, recorded here).
- Concurrent same-token racers: exactly one winner; losers stay
  fail-closed (reuse/revoked/invalid) and may revoke via the
  pre-existing lost-race path — no parallel chain ever forks.
- No JWKS/Android change (SEC07-B/C next). No production claim.

Next microtask: SEC07-B per-issuer JWKS expiry.

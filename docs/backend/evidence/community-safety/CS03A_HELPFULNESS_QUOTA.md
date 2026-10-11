# CS03A — Helpfulness admission quota

Date: 2026-10-11. Status: LOCAL_DONE. Scope: quota on helpfulness
writes, token-stability/kiosk binding proof, exhaustion test — no
migration, no new tables, no VPS/APK change. Head: maintained `dev`.
Plan: [phased plan](../../../planning/COMMUNITY_SAFETY_EXECUTION_PLAN.md).
Rules: B-BR-CS03, BUC-CS02/CS03.

## Wiring

- `HelpfulnessPorts.CheckQuota` (new, required): `Cast` consumes the
  `write` budget keyed by caller fingerprint before any store work;
  nil wiring fails closed with `ErrQuotaDenied`. Removals stay
  unquoted cleanup (feedback precedent); reads stay outside
  transactional quotas (identity design). Denials surface as
  `QuotaDeniedError` for the existing 429 + Retry-After mapping.
- `cmd/api/main.go` and the HTTP e2e fixture map identity `QuotaError`
  onto `QuotaDeniedError`, mirroring `votePorts` exactly.

## Binding frozen (proof, not aspiration)

`AttributionToken` is per-account and stable across device keys, so
the token-bound vote already satisfies "one account vote to one
stable contribution/revision, across device keys": two keys of one
account converge onto one vote; one shared fingerprint (kiosk/NAT)
keeps accounts separate while sharing the write budget. No new
identity primitive; devices never mint votes for an account.

## Store hardening found by the quota tests

Wiring quota exposed two latent races in the CS02A tally path: retry
inside an aborted transaction (25P02), and concurrent first-cast
rebuilds overwriting with stale snapshots. `bumpHelpfulnessTally`
now does single-pass UPDATE, then serializes the rare missing-row
path on the advisory target lock (row-then-advisory order everywhere)
with UPDATE-again/INSERT-or-exact-rebuild and outer-tx retry as
backstop. In-tx retry is gone.

## Validation (actual)

- Unit application: denial surfaces `QuotaDeniedError` with the retry
  delay and never mutates (subject/op asserted `fp`/`write`);
  same-account two keys converge; shared-fingerprint two accounts
  stay separate.
- Unit HTTP: denial maps to 429 + `Retry-After: 90` +
  `community.quota-exceeded`.
- Integration (real `Limiter`, policy write 3/hour, real pg store):
  3 budgeted casts succeed, 4th denied with honest RetryAfter within
  the window, tally exactly 3/0, denied vote never stored.
- Regression: CS02A 11/11 re-green, concurrent + quota tests twice
  under `-race`; full unit module and full HTTP integration package
  green.
- `go build`, `go vet`, `gofmt -l`, `git diff --check` clean;
  `scan-secrets.sh` passed.

## Limits / owed (CS03B)

- Limiter cardinality cap, IPv4-mapped normalization proof, key
  rotation/old-digest retirement tests, per-route/account/verified-
  installation/trusted-IP layering beyond fingerprint+write.
- Erasure account → refs fan-out: CS09 path. Moderation target: CS05.

Next microtask: CS03B — limiter cardinality, IP normalization, rotation.

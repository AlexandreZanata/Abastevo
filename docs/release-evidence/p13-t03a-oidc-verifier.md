# P13-T03 — Google/Apple verification (sliced: T03A / T03B / T03C)

Status: T03A LOCAL_DONE on `codex/phase-13-free-accounts` (phase PR #27,
draft). Issue: #24. The task spans protocol, persistence and transport, so
it ships in three letter-suffixed slices with explicit acceptance below.
One slice per atomic commit; issue #24 stays open until the phase PR merges.

## Slice acceptance (frozen before coding)

- T03A — OIDC verifier core + JWKS sources + stub issuers + memory nonces
  (this commit): RS256/ES256 verification from stdlib crypto, frozen
  issuer allowlist, exact audience, expiry inside 120s skew, single-use
  nonces, unknown-kid rotation refusal, tamper/malformed refusal, outage
  fails closed, Apple relay binds the opaque subject, JWKS cache honors
  the 1h TTL, HTTPS-only fetch. Pure unit tests with stub-minted tokens.
- T03B — migration `000021` (`provider_links`, `oidc_nonces`) + link/
  unlink application + pg stores + real-DB integration: happy link both
  providers, cross-account refusal, last-method unlink refusal, nonce
  replay across processes, JWKS rotation against DB-backed nonces.
- T03C — link/unlink transport with session auth + OpenAPI paths +
  sandbox evidence (stub-issuer protocol runs; live-provider contact
  stays deferred to P13-T05/G13 device evidence, recorded, never mocked
  green).

## T03A what changed

- `account/adapters/oidc/oidc.go` — `Verifier` implementing the T01
  `domain.ProviderVerifier` port: allowlist → token split → JWKS fetch →
  kid select → RS256/ES256 verify → iss/aud/expiry/nonce/sub checks →
  nonce consume. Stable `ErrOIDC*` codes incl. new `oidc-nonce-mismatch`
  (token bound to another login, not a replay).
- `account/adapters/oidc/stub.go` — `StubIssuer` minting RS256/ES256 keys
  and tokens for tests/local runs, with JWKS JSON rendering.
- `account/adapters/oidc/jwks.go` — `CachedKeys` (frozen 1h TTL),
  `HTTPKeys` (HTTPS-only, 5s timeout, 64 KiB cap), `MemKeys`,
  `FailingKeys`, `ProductionJWKS` endpoints, `MemNonces`.
- `account/domain/ports.go` — `ErrOIDCNonceMismatch` + verdict mapping.
- Tests: 8 suites (valid both algs, wrong iss/aud/exp + skew edge,
  nonce single-use + mismatch, unknown-kid rotation + tamper + malformed,
  outage closed, Apple relay subject, cache TTL, HTTPS fetch + refusals).

## Validation (exact commands, this host)

```sh
cd backend && gofmt -l ./... && go vet ./internal/modules/account/... \
  && go test -count=1 ./internal/modules/account/...
make quick-verify
git diff --check
```

Outcome: 8/8 OIDC suites plus all account packages green; `gofmt`/`vet`
clean; `quick-verify` ok; `diff --check` clean; secrets scan clean.

## Limits (not claimed)

- No link/unlink persistence or endpoints (T03B/T03C); no live Google/
  Apple contact anywhere — sandbox evidence lands in T03C against stubs,
  live-provider runs stay outstanding to device acceptance.
- Email remains display/relay only; merge-by-email stays refused by
  construction (no code path accepts it — enforced in T03B).

## Rollback

Pure addition: deleting `adapters/oidc/` restores prior behavior. No
migration, no flag, no data.

## Next

P13-T03B provider links + nonces persistence + real-DB integration (#24,
same branch).

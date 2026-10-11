# SEC07-B — Per-issuer JWKS expiry and bounded refresh

Date: 2026-10-11. Status: LOCAL_DONE. Scope: JWKS cache/refresh/
redirect policy only (no session/Android change). Head: maintained
`dev` + this task.
Plan: [remediation plan](../../../planning/SECURITY_REMEDIATION_PLAN_2026-10-11.md)
(SEC07, AUD-09). Contract: frozen 1h TTL kept; expiry independent per
issuer; unknown kids trigger exactly one TTL-bypassing refetch
(rotation verifies, removal still refuses); outage fails closed as
unavailable; JWKS redirects never followed (stable endpoints).
Constraint: no external network in tests (local httptest TLS only);
plaintext keys never logged.

Validation: PASS

## Changes

- `account/adapters/oidc/jwks.go`: `CachedKeys.at` is now per issuer
  (`map[string]time.Time`); `Fetch` stamps only the fetched issuer;
  failed refetch keeps the old stamp and fails closed (retries next
  call, never serves expired keys). New `RefreshingKeySource`
  + `CachedKeys.Refresh` (one unconditional fetch, restamps only that
  issuer). `HTTPKeys.Fetch` pins a copied client with a refusing
  `CheckRedirect` (caller's client never mutated, default-client
  redirect-following closed); https-only + 64KiB cap kept.
- `account/adapters/oidc/oidc.go`: `Verify` retries `selectKey` once
  through `RefreshingKeySource` on unknown kid (refresh failure →
  unavailable; still unknown → unknown-issuer). Non-refreshing
  sources keep the old immediate refusal.
- Tests (`sec07b_jwks_test.go`, 4 new): alternating providers past TTL
  (apple fetch at +50m does not prolong google; google refetches at
  +70m while apple stays cached — 3 source calls); rotation via one
  refresh then verification; removal refusing after refresh past TTL;
  outage failing closed as unavailable; malicious local redirect
  refused with zero attacker requests.

## Validation (actual)

- `go test ./internal/modules/account/...` green (incl. 4 new +
  pre-existing TTL/outage/rotation/tamper suites).
- `go build ./...`, `go vet`, `gofmt -l` (clean), `git diff --check`
  clean; secret scan passed (prior turn, no secret-adjacent change).
- RED pre-GREEN: shared-`at` code served google stale at +70m (2
  fetches, want 3 — one provider prolonged the other); unknown kids
  never retried (rotation needed a TTL wait); default client followed
  the 302 (attacker saw the request).

## Limits / owed

- Removal propagates at most after the issuer TTL (cache stays
  authoritative while fresh — recorded, not silent).
- Stale-on-outage is not served (fail-closed by policy).
- Production JWKS URLs unchanged. No session/Android change.

Next microtask: SEC07-C Android refresh serialization.

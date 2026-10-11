# SEC02-A — Proxy-fairness regressions (pending SEC02-B resolver)

Date: 2026-10-11. Status: LOCAL_DONE. Scope: three skipped
regression tests only — no behavior change (resolver lands in
SEC02-B). Head: maintained `dev` + this task.
Plan: [remediation plan](../../../planning/SECURITY_REMEDIATION_PLAN_2026-10-11.md)
(SEC02, AUD-03). Constraint: raw IPs stay out of logs/storage
(hashed subjects only); intentional anonymous price writes are not
an auth bypass.

Validation: PASS (suites green; flaw body proven FAILING unskipped)

## Regressions (all `t.Skip("SEC02-B: ...")`, repo Skip precedent)

Identity (`identity/adapters/http/proxy_regression_test.go`):

- `TestProxyPeersShareOneBucket`: two independent clients (distinct
  X-Forwarded-For + fingerprints) behind one proxy peer must yield
  two IP subjects. Unskipped today: FAIL — both collapse onto one
  peer subject (the AUD-03 flaw, bounded proof, not a flood run).
- `TestSpoofedForwardingHeaderIgnored`: direct untrusted peer with
  forged XFF must bill the TCP peer, never the header. Guard:
  passes before and after (headers already ignored); kept to pin
  spoof rejection through the resolver change.
- `TestMalformedForwardingFallsBackToPeer`: malformed/multi/empty
  XFF behind a proxy peer must still bill exactly one subject per
  request (peer fallback, no crash, no spoof). Guard: passes
  before and after.

Each flaw body was verified to FAIL with skips stripped (first
test FAILs, guards PASS), then restored skipped so CI stays
green. Resolver, allowlist wiring and budget split ride SEC02-B/C.

## Validation (actual)

- `go test ./internal/modules/identity/adapters/http/`: green
  with 3 skips. `go vet`, `gofmt -l`, `git diff --check` clean;
  `scan-secrets.sh` passed (synthetic TEST-NET IPs only).

## Limits / owed

- Resolver, trusted-proxy allowlist, global/per-client budget
  split, Caddy→API integration proof: SEC02-B/C.
- Shared VPS edge changes are out of scope; Abastevo-only
  authorization. No live readiness inference.

Next microtask: SEC02-B trusted resolver + wiring.

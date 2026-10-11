# CS12A — Consent endpoints against the live stack

Date: 2026-10-11. Status: LOCAL_DONE. Scope: full-stack signed
integration tests for the three consent endpoints only — no
production wiring change, no VPS/APK. Head: maintained `dev` +
this task.
Plan: [phased plan](../../../planning/COMMUNITY_SAFETY_EXECUTION_PLAN.md)
(CS12, B-BR-CS06), [server contract](../community-safety/CS09C_CONSENT_HTTP.md).
Rules: B-BR-CS06, BUC-CS05.

Validation: PASS

## Fixture (same composition cmd/api mounts)

Fresh disposable PostGIS per test (full migration chain), real
identity verifier (`api.example.invalid`), real registrar
attribution, real guarded consent store, real application
commands, frozen terms pair — with real P-256 enrollment and
per-request SigV4-style proofs, byte for byte like the
community full-stack harness.

## Coverage (each endpoint live, 7 tests)

- Owner grant → status granted → revoke (purge signal) →
  status revoked, receipts exact, no-store asserted.
- Identical grant replay under a live clock converges
  (`replayed: true`) through the whole stack.
- Second enrolled owner reads its own default OFF (real-auth
  isolation, no oracle).
- Guest (unsigned) refuses 401 `privacy.auth-required` on all
  three endpoints before any state work.
- Revoked keys (contributor deleted after enrollment) refuse
  401 with zero rows persisted.
- Tampered body after signing refuses with zero rows persisted.
- Superseded terms refuse 400 `privacy.terms-outdated`.

All 7 passed on the first run: the composition was already
correct — the RED was the missing coverage itself, recorded
honestly. No code change outside the test file.

## Validation (actual)

- `go test -race -count=1 -tags=integration
  ./internal/modules/privacy/...`: all packages green on
  disposable PostGIS. Unit suites green. `go vet` (both tag
  sets), `gofmt -l`, `git diff --check` clean;
  `scan-secrets.sh` passed (synthetic keys only, in-test).

## Limits / owed

- Write budget: consent endpoints carry no quota wiring — the
  per-owner revision-ordered singleton is self-limiting
  (authenticated, idempotent, no amplification); recorded, not
  silently assumed.
- Per-route business metrics: no framework exists (only the
  runtime metrics listener); success/failure shapes stay
  asserted in tests. Client offline/reopen/account/device
  scenarios: covered in CS11A/B unit tests, not duplicated
  here.
- Soak SLO proof after 2026-10-12T16:11:11Z: unchanged.

Next microtask: soak SLO proof after the wall clock (then CS12 close-out).

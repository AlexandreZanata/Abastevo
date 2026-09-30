# P09-T01 local runtime validation follow-up

Scope: the user requested all tests on this machine's IP, with no real server.
Baseline `c51fa03f57b0c9f0a429e41c51bbd21ff2dcbb9c`; branch
`codex/phase-09-production-validation`; publication scope LOCAL_ONLY.
B-BR-001/002/004/005/008/011/014/016 and BUC-001…008 guide existing behavior;
no new trust, payment or tenant rules are introduced.

## Demonstrated regressions and corrections

- Public identity routes were specified but not mounted. The actual process now
  exposes bounded challenge, registration and two-key rotation endpoints.
- Valid `GASOLINE_REGULAR` contributions were refused by applying the Portuguese
  ANP-label parser to the wire enum; exact wire products/units are now checked
  without rewriting money or silently accepting a mismatched unit.
- Signed registration/rotation could consume challenges with substituted nonce,
  purpose or fingerprint metadata. Regression tests failed before the binding
  checks. Server metadata/time now govern proofs, both rotation proofs bind the
  same new-key intent, and concurrent old-key revocation has exactly one winner.
- Trailing/duplicate JSON, oversized/truncated bodies, expired-equality and
  excessive-future signature windows are rejected. Production configuration
  requires an explicit canonical host and persistent cursor signing key.
- Private reads and mutations no longer turn into 304 through If-None-Match;
  public weak ETags use HTTP syntax. Request IDs are bounded/sanitized and agree
  between response header and error envelope.
- Storage redirects were followed; tests demonstrated three redirected requests.
  Transfers now refuse them and omit presigned URLs/remote bodies from errors.
- API readiness previously checked only connectivity. It now verifies all
  embedded migration checksums; worker startup rejects incompatible schemas.
- The pinned Caddy image rejected the original global trusted-proxy directive.
  Corrected syntax validates, and access/runtime filters remove request and
  response-header objects (precise queries, socket IPs and custom proofs).

## Local matrix

Completed 2026-09-30, local-only. Tested backend/contracts/infra/scripts/Makefile
fingerprint: `48dc47514dba28cb4862c63611d75dd1bf6367e154036ec9c2a182070cfd2e8c`.
No immutable release candidate or remote CI is claimed. Logs remain in
`/tmp/anpfuel-production-validation/`; full report is `final4/summary.json`.

- `make test-local-backend`: PASS. All unit/race suites: 666 passing test events,
  47 packages; all integration/race suites: 818 passing test events, 59 packages.
  Integration includes the unit cases, so counts must not be added as distinct
  tests. One intentional skip is `TestGenerateVectors` (fixture authoring helper),
  not a backend acceptance case. No failures or data races.
- Actual API + worker + private S3 process flow: PASS. Actual API with storage
  deliberately absent: PASS (503, no false evidence acceptance).
- `go vet`, `staticcheck`, `sqlc vet`: PASS. `govulncheck`: zero findings.
- Infrastructure mutation harness: 7/7; deploy/rollback harness: 9/9;
  encrypted backup/corruption/password/retention checks: 10/10; restore with
  deletion-ledger replay and guarded disposal: 9/9. These use only the isolated
  Compose DB. Environment-cleaned subprocesses preserve its project identity.
- Load: 30,860 origin requests in the bounded 20-second profile, p95 22.2 ms,
  zero 5xx; DB timeout/fail-closed/recovery, denied anonymous upload and capped
  disk-pressure harness: 7/7. This is a workstation smoke, not capacity acceptance.
- Pinned Caddy with a temporary internal CA: PASS for TLS 1.2/1.3, ready/live,
  forbidden routes and query/cookie/custom-proof privacy on successful requests
  and a real upstream connection failure (502). No host trust-store changes.
- Required local `quick-verify`: PASS, full selection, 9 seconds; gofmt/build/vet,
  selected behavior suites, staticcheck, sqlc generation/diff, OpenAPI lint
  (zero errors/warnings, nine informational notices), secret review and vuln scan.
- G09 record harness: PASS (BLOCKED record; COMPLETE/missing-blocker refusal).
  `git diff --check`: PASS. No CI checks were bypassed or claimed as run remotely.
The repeatable entry point is `make test-local-backend`, with an isolated dynamic-
port Compose project, tmpfs PostGIS 18/PostGIS 3.6 and pinned private S3 emulator.
`make test-local-edge` uses the pinned Caddy 2.10.2 image and ephemeral internal CA.

The real HTTP scenario checks registration/duplicate identity, reads, exact price,
idempotent/conflicting/replayed writes, owner isolation, private upload, valid JPEG,
malformed JPEG rejection, worker validation, independent confirmation, self-vote
refusal, dispute and key substitution/revocation. Missing storage explicitly
returns 503 in a separate actual-process run. Moderation, export/erasure, consensus,
leases, migration recovery and concurrency retain real-PostGIS suite coverage;
this is not a claim that unmounted privacy HTTP routes were exercised.

## Limits and local runtime

The API is bound to `172.19.2.11:18093`, with isolated validation DB/storage and
ANP discovery disabled. Temporary configuration contains synthetic credentials
only and is not committed. Images are test fixtures, not a production storage
recommendation. The main local suite removes only its unique Compose project;
the review API uses separate dedicated validation containers.

Local HTTP/internal-CA TLS/S3, bounded workstation load, local restore/deletion
replay and adapter-based operator tests do not prove real R2/CDN/public TLS,
off-host disaster recovery, production capacity or legal acceptance. G09 stays
BLOCKED; no Android implementation, release tag, production deployment or wiki
sync occurred in this local-only follow-up.

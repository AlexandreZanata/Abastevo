# P08-T08 security and dependency release review (local evidence)

Date: 2026-09-30. Scope: launch-blocking auth/privacy/storage review on
head `8ff3797` (phase `codex/phase-08-hardening`). Method follows
[SECURITY_PRIVACY](../../docs/security/SECURITY_PRIVACY.md) STRIDE plus
the pinned scans in `scripts/check-security.sh` (static review,
`govulncheck`, focused adversarial suites) proven by
`scripts/tests/test-security.sh`. No product behavior changed; this is
a review gate with evidence, not a feature.

## Verdict

No unresolved critical/high findings on the reviewed tree. Release
still needs P09 staging acceptance (30-minute matrix, live R2/edge,
restore on provisioned infra) before any pilot claim.

## What was reviewed

- Spoofing/tampering: P-256/RFC 9421 vectors (P03-T01), single-use
  nonce with concurrent-replay refusal, covered-component/body-digest
  rejection, append-only facts, checksummed official revisions,
  server-verified frozen evidence (different final key, orphan sweep).
- Disclosure: private bucket by construction (no ACL grants in
  `evidence/adapters/storage`), 60 s case-bound evidence URLs never
  logged, redacted config/telemetry logs, owner-scoped reads sharing
  one 404 shape, edge strips `Cookie`/`Authorization` on the shared
  set (P08-T06 invariance tests), metrics loopback-only.
- DoS: 1 MiB API body cap, 30 MiB ANP fetch cap, 3 MiB JPEG cap with
  12 MP/8192 px pre-decode bounds, quotas (10/day uploads,
  registration/write limits with 429/Retry-After), lease/fencing
  queues with dead-letter replay, bounded alerts.
- Elevation: no public admin routes; moderation/privacy operator work
  via restricted `cmd/ops` with mandatory actor+reason audit;
  migrator-only DDL; staging/prod DB has no published ports; Caddy
  `admin off`, `trusted_proxies private_ranges`, TLS 1.2/1.3 only.
- SSRF: ANP fetcher allowlist + redirect-target recheck + public-IP
  dial policy with explicit denied prefixes, TLS 1.2 minimum,
  size/timeout/conditional-fetch bounds, temp-file cleanup.

## Dependency and image inventory (reviewed pins)

Go `go1.27` / toolchain `go1.27.1`. Direct: `chi v5.3.2`,
`pgx v5.11.0`, `kin-openapi v0.149.0` (all MIT, permissive transitives
only; no `replace` directives; `go.sum` present). Tool pins:
`sqlc v1.31.1`, `staticcheck v0.8.1`, `govulncheck v1.8.0`,
`vacuum v0.30.6` (see `backend/README.md`).

Images: `golang:1.27.1-bookworm@sha256:69a7b978…f99195` (builder),
`gcr.io/distroless/static-debian12:nonroot@sha256:afa5c872…57f7ab`
(runtime, `USER 65532:65532` all roles),
`postgis/postgis:18-3.6@sha256:60f6ad1d…80b8677`,
`caddy:2.10.2-alpine@sha256:4c6e91c6…e530d`; roles ship immutable
`ANPFUEL_RELEASE` tags, never `latest`. Full module list:
`cd backend && go list -m all`.

## Commands and outcomes (tested tree)

- `bash scripts/check-security.sh` — full gate green: static review
  ok, `govulncheck ./...` reports no vulnerabilities, 18 focused
  adversarial packages green (`config`, `telemetry`,
  `httpserver`, `identity/profile+domain+adapters/auth+application`,
  `official/adapters/source`, `evidence/storage+media+domain`,
  `community/adapters/http`, `moderation/domain+application`,
  `privacy/domain+application`).
- `bash scripts/tests/test-security.sh` — 10/10: reviewed static
  tree accepted; 7 mutants refused (public DB port, floating image,
  open `trusted_proxies`, dev secret, unpinned builder, root
  runtime, `go.mod` replace); live `govulncheck` clean; adversarial
  selection executes. RED per fault class proven by the refusals,
  GREEN on the reviewed tree.
- `bash scripts/scan-secrets.sh` — green via the gate (tracked files
  plus secret-shaped patterns in compose/Caddy/Dockerfile/examples).

## Limits (explicit non-claims)

- Emulator plus isolated R2 staging pending (no registry access for
  the minio image, no staging credentials here); the storage adapter
  targets real SigV4 so both run when available (P05-T02).
- Live edge TLS/proxy-authority and the 30-minute acceptance load
  (100k stations, 1M rows, signed-write admission, shared cache)
  run on provisioned staging in P09, not here.
- Dependency scan is `govulncheck` on Go modules plus digest/secret
  static review; OS-layer/image CVE scanning runs at build/deploy
  time on provisioned infra.
- A failed candidate blocks release: fix on this phase branch,
  rerun the affected gate on the new head, never weaken thresholds
  or hide test absence.

# ESP-14 — Scoped static hosting and safe promotion

Status: LOCAL_DONE
Validation: PASS
Integration: INTEGRATION_PENDING
Date: 2026-10-24
Branch: `codex/enterprise-static-portal`
Parent: ESP-13 `06e871e9`

Rules: B-BR-ESP-13/14/15/16; BUC-ESP-06/07.
[Plan](../../../planning/ENTERPRISE_DATA_STATIC_PORTAL_PLAN.md#esp-14--implement-scoped-static-hosting-and-safe-promotion).
[ESP-13 verified artifacts](ESP-13.md).
[Hosting/promotion contract](../../../operator/portal-static-hosting.md).

## Topology decision

Only an **isolated local rehearsal** was authorized and implemented:

- one pinned Caddy container
  (`caddy:2.10.2-alpine@sha256:4c6e91c6…`), loopback port `18080`, HTTP only;
- a private release root (`0700` directories, `0600` files) outside any public
  document root;
- the selected `current` symlink is the only path exposed to Caddy, mounted
  read-only;
- no DNS, TLS certificate, CDN, shared edge, API proxy or database access;
- the rehearsal edge always sends `X-Robots-Tag: noindex, nofollow`.

The production host/CDN/TLS/permissions, cache purge, multi-host locking,
disk quotas and public origin remain **OWED**. No shared `infra/caddy/Caddyfile`
or Compose topology was changed.

## Delivered

- `portalgen/release.go`: atomic publisher with
  - non-blocking `flock` per release root;
  - optional fence on the current content hash;
  - complete copy into `releases/.tmp-*`, second full `portalgen.Verify` of the
    copied tree, including the expected content hash;
  - rename to the content-addressed release directory and atomic symlink switch;
  - atomic `state.json`, idempotent re-promotion, stale-state reconciliation,
    rollback and withdrawal.
- `backend/cmd/portalrelease`: `promote`, `rollback`, `withdraw`, `verify`
  operator commands. No DB/API/source access.
- `infra/caddy/portal-rehearsal.Caddyfile`: static root, gzip, 404 handler,
  security/cache headers and no API proxy.
- `infra/scripts/rehearse-portal.sh`: validates Caddy, promotes A, starts the
  isolated container, checks headers/compression/404/API independence, promotes
  B, rolls back, corrupts A and rolls back to B, withdraws A to B and finally
  withdraws without fallback. It uses only a random loopback container, a
  private root and a trap cleanup.
- `docs/operator/portal-static-hosting.md`, `infra/README.md` and Makefile
  targets `test-portal-release` / `rehearse-portal`.

## Real rehearsal

The ESP-13 artifacts under `/tmp/fq-portal-ready` were used:

| Candidate | Content | Shape |
| --- | --- | --- |
| A | `sha256:bd6649fd5f9c1a3be5972a07cc5cfd1dc07329c681cedb39e3ae32ab510002f2` | 3,409 pages / 3,419 files / 15,231,739 bytes, noindex |
| B | `sha256:75bc8a1a21702982ebf651aa2c14b8e8b1cd9005c7e68dbea7f80f80fd935682` | 3,409 pages / 3,421 files / 15,932,636 bytes, indexable + search JS |

The rehearsal ran with
`PORTAL_RELEASE_ROOT=/tmp/fq-portal-ready/real-2025-release-root-final` and
`PORTAL_KEEP=1`; the retained root is `0700` with `state.json` `0600`. It ended
with no `current` symlink and both releases explicitly withdrawn.

Observed through the isolated Caddy:

```text
/consultar/                       200  text/html; charset=utf-8
                                        Cache-Control: public, max-age=0, must-revalidate
                                        X-Content-Type-Options: nosniff
                                        X-Robots-Tag: noindex, nofollow
                                        Content-Encoding: gzip
/assets/portal.<hash>.css         200  text/css; charset=utf-8
                                        Cache-Control: public, max-age=31536000, immutable
                                        Content-Encoding: gzip
/assets/portal.<hash>.js          200  text/javascript; charset=utf-8
                                        Content-Encoding: gzip
/nao-existe/                      404  "Página não encontrada"
/v1/health/live                   404  (no API proxy)
/health/live                      404  (no API proxy)
sitemap.xml after A               404  (A is noindex)
sitemap.xml after B               200
sitemap.xml after rollback A      404
sitemap.xml after corrupt rollback B  200
sitemap.xml after withdraw A->B   200
/consultar/ after withdraw B      404
```

The corrupt-A step verifies that `portalrelease verify` rejects the current
corrupted release and that `rollback --target B` restores a valid previous
release without trusting the corrupt current. Re-promoting A then replaces its
corrupt release directory from the verified candidate; withdrawing A with
fallback B and then withdrawing B without fallback prove withdrawal-aware
rollback and a clean 404 state.

## Tests and actual results

Go, from `backend/`:

```sh
go test -race -count=1 ./internal/modules/enterpriseexport/...
go test -race -count=1 -tags=integration ./internal/modules/enterpriseexport/...
go build ./...
go vet ./internal/modules/enterpriseexport/... ./cmd/portalgen ./cmd/portalcheck ./cmd/portalrelease
GOTOOLCHAIN=go1.27.1 staticcheck ./internal/modules/enterpriseexport/... ./cmd/portalgen ./cmd/portalcheck ./cmd/portalrelease
sqlc vet
sqlc generate
```

PASS: unit portalgen 24.897 s; integration portalgen 13.677 s; other
enterpriseexport packages green. `go build`, vet, staticcheck, sqlc and
byte-stable generation PASS. `gofmt`, `git diff --check` and shell syntax PASS.

New release tests cover:

- complete copy before the atomic switch (current stays on A while B copies);
- promotion, idempotent re-promotion, stale-state reconciliation, rollback and
  withdrawal;
- fence mismatch and a busy lock;
- injected `ENOSPC` and corrupted-copy failures preserving the current;
- unreadable/unwritable root permission failure;
- corrupted current release refused by `verify` and recovered by rolling back
  to a safe release;
- missing state with an existing current pointer failing closed;
- private root/state permissions and withdrawn-release resurrection refusal.

Caddy rehearsal:

```sh
PORTAL_ARTIFACT_A=/tmp/fq-portal-ready/real-2025-portal \
PORTAL_ARTIFACT_B=/tmp/fq-portal-ready/real-2025-portal-full \
PORTAL_SITE_URL=https://portal.example \
PORTAL_RELEASE_ROOT=/tmp/fq-portal-ready/real-2025-release-root-final \
PORTAL_KEEP=1 \
bash infra/scripts/rehearse-portal.sh
```

PASS: `rehearse-portal ok: isolated static root, atomic switch/rollback,
headers/compression/404 and API independence`. `caddy validate` PASS with the
pinned image. No container or shared edge remained after the run.

## Limitations and OWED

- **Production hosting is not implemented.** The actual host, CDN, TLS/DNS,
  filesystem ownership, cache purge, image/release pinning on the host and
  multi-host fencing remain OWED to ESP-18. The flock is single-host only.
- The rehearsal Caddy is HTTP loopback and forces noindex; it is not the public
  preview or release edge. ESP-17/18 own preview/browser/public acceptance.
- Real prices remain blocked by ESP-07 over-precision and ESP-01 rights; the
  rehearsal uses the verified ESP-13 artifact with the real event-level official
  slice and the synthetic station/price fixture.
- Automatic source refresh/correction publication is ESP-15, not this phase.
- No API, database, source, VPS, DNS, shared Caddy, Compose or production
  mutation occurred. Nothing was published.

## Next

ESP-15 — wire automatic source refresh and correction recovery into this
verified candidate/release path. Not implemented here.

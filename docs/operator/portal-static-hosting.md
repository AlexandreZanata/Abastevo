# Static portal hosting and promotion (ESP-14)

Status: isolated local rehearsal implemented; production hosting is **not**
selected or provisioned. This document records the scoped topology decision and
the operator commands; it does not authorize a shared-edge, DNS, CDN or
production change.

## Topology decision

- **Rehearsal only:** one pinned Caddy container
  (`caddy:2.10.2-alpine@sha256:4c6e91c6…`), loopback port `18080`, private
  release root under `/tmp`, no DNS/TLS/CDN and no API proxy.
- **Private release root:** directories `0700`, files `0600`. Only the selected
  `current` symlink is exposed to Caddy, read-only. `state.json` and
  `releases/` never sit under a public document root.
- **No shared edge mutation:** the existing staging/production
  `infra/caddy/Caddyfile` and Compose topologies are unchanged. Production
  host/CDN/TLS/permissions and the real public origin remain OWED to ESP-18.
- **Preview remains noindex:** the rehearsal Caddyfile sends
  `X-Robots-Tag: noindex, nofollow` even when the candidate contains an
  indexable sitemap.

## Release layout

```text
<release-root>/
  .promote.lock               flock, 0600
  state.json                  atomic release/rollback metadata, 0600
  releases/<64hex>/           complete verified candidate, 0700/0600
  current -> releases/<64hex> atomic symlink (the only served path)
```

`portalgen.Verify` must pass on the candidate before any copy. The publisher
copies into `releases/.tmp-*`, verifies the copied tree again, renames it to the
final content-addressed directory, and only then atomically replaces `current`.
A failed copy, hash mismatch, corruption, permission error, missing state or
concurrent promotion leaves the previous `current` unchanged. The OS releases
the flock on process exit; a crashed publisher cannot leave the lock held.

## Publisher commands

From the repository root:

```sh
cd backend && go build -o /tmp/portalrelease ./cmd/portalrelease

/tmp/portalrelease promote --artifact <candidate> --root <release-root> \
  --site-url https://portal.example
/tmp/portalrelease promote --artifact <candidate> --root <release-root> \
  --site-url https://portal.example --fence --expect-current <sha256:...>
/tmp/portalrelease verify --root <release-root> --site-url https://portal.example
/tmp/portalrelease rollback --root <release-root> --target <sha256:...> \
  --site-url https://portal.example
/tmp/portalrelease withdraw --root <release-root> [--fallback <sha256:...>] \
  --site-url https://portal.example
```

- `--fence` compares the current content hash before doing any work; use it for
  automation. `--expect-current ""` is valid for the first promotion.
- `rollback` only selects a recorded, non-withdrawn release that still passes
  `portalgen.Verify`; it can recover from a corrupted current release.
- `withdraw` marks the current release withdrawn. With `--fallback` it atomically
  switches to that safe release; without it, the `current` symlink is removed so
  the static site returns 404. A withdrawn release cannot be rolled back.
- `verify` checks the current symlink, state consistency and every file digest.

## Caddy rehearsal

`infra/caddy/portal-rehearsal.Caddyfile` serves `current` with:

- `X-Content-Type-Options: nosniff`, `Referrer-Policy`, `X-Frame-Options`,
  `Permissions-Policy` and `X-Robots-Tag: noindex`;
- `encode gzip`;
- immutable `Cache-Control` for `/assets/*` and revalidation for document
  routes;
- a real 404 response and `file_server` fallback;
- no `/v1/*` or `/health/*` proxy, so the static site does not depend on the
  API being up.

Run the complete isolated rehearsal:

```sh
PORTAL_ARTIFACT_A=/path/to/verified-candidate \
PORTAL_ARTIFACT_B=/path/to/second-verified-candidate \
PORTAL_SITE_URL=https://portal.example \
bash infra/scripts/rehearse-portal.sh
```

The script builds `portalrelease`, validates Caddy, promotes A, starts the
loopback Caddy container, checks 200/404/headers/compression, promotes B,
rolls back A, corrupts A and rolls back to B, withdraws B to A, and finally
withdraws without fallback. It removes its container and temporary root unless
`PORTAL_KEEP=1`/`PORTAL_RELEASE_ROOT` is set.

## Limitations

- Production host/CDN/TLS/DNS, shared-edge routing, cache purge, multi-host
  locking, disk quotas and capacity remain OWED to ESP-14's deployment scope and
  ESP-18. The current flock is single-host.
- Real price publication remains blocked by ESP-07/rights; the rehearsal proves
  the selected event-level official artifact plus the synthetic price fixture.
- Automatic source refresh/correction publication is ESP-15, not this document.
- No API, database, source or shared edge is contacted by the publisher or the
  rehearsal.

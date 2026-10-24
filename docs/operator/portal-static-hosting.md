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

## Automatic refresh and correction (ESP-15)

Automatic refresh is implemented as a private worker job and remains **off by
default**. It requires the enterprise export builder and these settings:

```sh
ANPFUEL_ENTERPRISE_PORTAL_REFRESH_ENABLED=true
ANPFUEL_ENTERPRISE_PORTAL_ROOT=/var/lib/abastevo/portal
ANPFUEL_ENTERPRISE_PORTAL_SITE_URL=https://portal.example
ANPFUEL_ENTERPRISE_PORTAL_INDEXABLE=false       # explicit preview default
ANPFUEL_ENTERPRISE_PORTAL_SEARCH_JS=/opt/abastevo/search.<hash>.js  # optional
```

`ANPFUEL_ENTERPRISE_EXPORT_ENABLED` must also be true; `PORTAL_ROOT` must be an
absolute private path distinct from the export root. The worker schedules
`enterprise-portal-refresh` every 15 minutes. A fine or official publication
enqueues an export build; a successful new export state enqueues a refresh
immediately. The periodic job recovers lost hooks.

The refresh reads the durable export candidate, generates a portal candidate,
verifies it, and promotes it with an expected-current fence. If the export state
and source pin are unchanged, or the generated content hash already equals the
current release, it records a no-op and leaves the release untouched. A failed
generation, copy, verification or promotion preserves the current release and
records `failed` in `portal-refresh-state.json`. Orphan `candidates/portal-*`
directories older than 24 hours are removed; a crash after promotion is
recovered as a no-op on the next run.

If the current FQ official revision is no longer published/withdrawn, the
refresh:

- withdraws the affected release when the current release contains official
  records;
- uses a newer safe release without official records as fallback if one exists;
- otherwise removes `current`, so the static site returns 404 rather than
  serving a withdrawn official slice.

The receipt records `last_outcome`, `last_error`, `trigger`, `observed_at`,
`export_built_at`, `portal_built_at`, `released_at`, `withdrawn_at`,
`export_content_sha256`, `portal_content_sha256`, source revision/lease/
edition/checksum/clock, generation seconds and observed/export-to-release
seconds. `observed_at` is the local source/build detection time and is **not**
the ANP publication delay; the source clock is recorded separately and no ANP
delay is inferred. The receipt is private and never served by the static root.

## Limitations

- Production host/CDN/TLS/DNS, shared-edge routing, cache purge, multi-host
  locking, disk quotas and capacity remain OWED to ESP-14's deployment scope and
  ESP-18. The current flock is single-host.
- Automatic refresh is composed and tested locally but disabled by default; no
  production activation or source schedule was changed.
- Real price publication remains blocked by ESP-07/rights; the rehearsal proves
  the selected event-level official artifact plus the synthetic price fixture.
- No API, database, source or shared edge is contacted by the publisher or the
  rehearsal.

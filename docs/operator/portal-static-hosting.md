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

### Official-only refresh scope

`ANPFUEL_ENTERPRISE_PORTAL_OFFICIAL_ONLY=true` restricts the whole refresh
pipeline to the admitted official-records slice: generation, verification,
promotion, rollback and withdrawal all run in official-only mode, and an
official-only refresh can never emit the full portal or widen sources. It
requires a published official revision (refuses without one) and withdraws to
404 instead of falling back to a full release. Build official-only candidates
with the maintained `cmd/portalexport` command (read-only repeatable-read
transaction over `ExportPin`/`BuildOfficialExport`, verified with
`portalcheck --official-only`); one-off uncommitted helpers are not a
substitute.The refresh reads the durable export candidate, generates a portal candidate,
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

## Capacity and browser drill (ESP-16)

The capacity harness is an isolated local drill, not a CDN/VPS/production
capacity claim. It requires an already verified release root with a current
portal (ESP-13/14/15), a pinned Caddy image and the local Python/Node/Chrome
toolchain. It never contacts the API, database, ANP or the shared edge.

```sh
# self-tests for the driver, stats, report and browser probe
make check-portal-capacity

# baseline: 100 req/s open-loop for 30 s, gzip, 32 clients
PORTAL_RELEASE_ROOT=/path/to/release-root PORTAL_SITE_URL=https://portal.example make portal-capacity

# concurrent update: atomic promote then fence-protected rollback
PORTAL_RELEASE_ROOT=/path/to/release-root PORTAL_SITE_URL=https://portal.example PORTAL_UPDATE_ARTIFACT=/path/to/second-verified-candidate PORTAL_UPDATE_AT_SECONDS=8 PORTAL_ROLLBACK_AT_SECONDS=16 make portal-capacity
```

The script freezes `workload.json` before the run with host CPU/memory/disk,
artifact content hash, images, browser version, URL mix, rates and budgets. It
starts the pinned Caddy container and a separate pinned Python generator
container on a private Docker network, samples Caddy CPU/memory/network/block
I/O, runs the existing qualified open-loop driver with gzip, then runs a
headless Chrome probe for `/consultar/`, a station page and `/fiscalizacao/`
with cold cache on desktop and mobile CPU-throttled viewports.

Predeclared defaults:

| Metric | Budget |
| --- | ---: |
| p95 / p99 | 50 ms / 100 ms |
| errors | 1% |
| achieved rate | 90 req/s at 100 offered |
| Caddy CPU / memory | 80% / 256 MiB |
| browser FCP / LCP / CLS | 1500 ms / 2500 ms / 0.1 |
`portal_report.py` fails closed on a breached budget, drops, timeouts,
semantic mismatches, generator limitation, external/API browser resources or
errors. The trap stops/removes the Caddy/generator containers and the private
network; the release root is not modified by the baseline. The update scenario
intentionally promotes the second artifact, then rolls back to the original
content hash; its load mix excludes JS/sitemap across the switch so the drill
tests the atomic HTML/CSS/shard/CSV path without pretending old assets are
retained. Results stay private under `PORTAL_RESULTS_DIR` (default
`/tmp/abastevo-portal-capacity-*`).

The measured numbers in ESP-16 evidence are **local loopback only**: no CDN,
TLS, real network latency, real mobile device or VPS-direct campaign. Those
campaigns and any production capacity statement remain OWED.

### Target capacity campaign (OWED — predeclared limits, not executed)

No campaign runs against ANP, and none may degrade the API. When a target
environment is authorized, run the ESP-16 harness there with these frozen
inputs: the budgets table above, a 100 req/s open-loop profile plus the
atomic promote/rollback update scenario, and these stop conditions —
any breached budget, any error/drop, Caddy above 80% CPU or 256 MiB, or
any API/safety degradation observed during the window: stop immediately,
roll back to the last safe release, and record the failed campaign
without rerunning for a better number.

## Preview, accessibility and operations handoff (ESP-17)

`infra/scripts/preview_portal.sh` runs an isolated HTTPS preview with Caddy's
local internal CA, a noindex edge and a dedicated private release root. It does
not touch the shared edge, DNS, API, database or ANP. It promotes the noindex
candidate, starts `https://localhost:18443` (HTTP `:18082` redirects), verifies
TLS/headers/cache, probes desktop/mobile accessibility, then promotes a full
corrected candidate, rolls back and rehearses a release-root backup/restore.

```sh
PORTAL_PREVIEW_ROOT=/path/to/new-preview-root PORTAL_PREVIEW_ARTIFACT_A=/path/to/noindex-candidate PORTAL_PREVIEW_ARTIFACT_B=/path/to/full-corrected-candidate PORTAL_SITE_URL=https://portal.example make preview-portal
```

Checks are fail-closed:

- HTTPS/HTTP2, TLS 1.2 verification against the copied internal CA, HSTS,
  HTTP->HTTPS 308, gzip, immutable assets, revalidation and noindex headers;
- dated period/source/checksum/official disclaimers, no-API navigation and no
  external resources;
- metadata/landmarks, one `h1`, labelled controls, non-empty links, skip link,
  first-focus order and visible 3 px focus outline;
- JS-disabled state directory/station navigation;
- 200% font mobile reflow with no document horizontal overflow and no small
  primary targets;
- screenshots for desktop, mobile-large-font and JS-disabled states;
- A→B promotion and rollback under the preview edge;
- backup of `state.json`, `releases/` and `current`, restore verification and
  rollback between releases.

Results live under `PORTAL_RESULTS_DIR`
(`/tmp/abastevo-portal-preview-*` by default). The internal CA is copied there
for the TLS check and is not a public certificate. The noindex A release is the
final preview current; the full B candidate is used only for the correction /
recovery rehearsal under the same noindex edge. B's own meta may be indexable
because it is a publication candidate; the preview edge header blocks indexing.

### Release backup and recovery

The release root is private and reproducible; back up `state.json`, `releases/`
and the `current` symlink together. The private `current` pointer governs
promotion only: whatever serves HTTP (edge config, static root, a serve copy)
is a separate pointer owned by the same maintenance step — see the
serving-pointer contract in the [publication gate](PORTAL_PUBLICATION.md#rollback-withdrawal-reasons-and-the-serving-pointer). Restore into a new private root, run
`portalrelease verify --root <restored> --site-url <origin>`, then rehearse
rollback across the restored releases before switching any preview. The ESP-17
rehearsal verified the archive hash, restored current A, switched to B and back
to A. Do not restore a release root over a live `current`; publish through the
publisher and its fence.

If the current release is corrupt, use `portalrelease rollback` to a verified
previous release; if the state is corrupt or missing, restore the backed-up
root; if the official source is withdrawn, use `withdraw` (fallback or 404).
Public preview/VPS handoff remain OWED.

### Cross-release asset retention

Promotion, rollback, refresh and withdrawal prepare and verify the retained
asset store before switching the serving pointer; `retained` never exposes a
page whose assets are still missing. `portalrelease retain --root <root>
--keep 2 --max-bytes 104857600` remains a repair/rebuild command.
The store contains the mandatory current release plus the newest
non-withdrawn releases within `keep`; a rollback target older than the
history window is still mandatory, so promotion ordering cannot break the
served page. Assets are hash-verified, safe `assets/...` paths, hardlinked
once when shared, and counted once. Budgets fail closed before the pointer
changes; the previous store stays intact.
`retained` is an atomically replaced symlink to an immutable generation
under `retained-generations/`; readers never observe the asset root missing
between two renames. A failed switch keeps the previous generation, an
interrupted build is cleaned on the next operation, and a missing link is
recovered from the newest complete generation. The edge serves `/assets/*`
from `retained` and everything else from `current`, so a cached old page
loads its exact dependencies after a switch or rollback with no version
mixing. `withdraw` rebuilds the store without the withdrawn release's
exclusive assets and removes older generations, so retired content cannot
return through retention or an old generation; shared assets stay while
referenced.

## Limitations

- Production host/CDN/TLS/DNS, shared-edge routing, cache purge, multi-host
  locking, disk quotas and capacity remain OWED to ESP-14's deployment scope and
  ESP-18. The current flock is single-host. See the
  [public portal publication gate](PORTAL_PUBLICATION.md); the portal is not
  publicly released.
- Automatic refresh is composed and tested locally but disabled by default; no
  production activation or source schedule was changed.
- ESP-16 measured local loopback only. CDN and VPS-direct campaigns, real
  network/TLS, real mobile devices and production capacity remain OWED.
- ESP-17's preview uses a local internal CA and headless Chrome, not public TLS,
  a real device/screen reader or a shared preview edge. Public preview
  handoff remains OWED; see above.
- Real price publication remains blocked by ESP-07/rights; the rehearsal proves
  the selected event-level official artifact plus the synthetic price fixture.
- No API, database, source or shared edge is contacted by the publisher or the
  rehearsal.

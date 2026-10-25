# ESP-16 — Measure capacity and browser performance

Status: LOCAL_DONE
Validation: PASS
Integration: INTEGRATION_PENDING
Date: 2026-10-25
Branch: `codex/enterprise-static-portal`
Parent: ESP-15 `71e4f92b`

Rules: B-BR-ESP-12–15; BUC-ESP-06/07.
[Plan](../../../planning/ENTERPRISE_DATA_STATIC_PORTAL_PLAN.md#esp-16--measure-capacity-and-browser-performance).
[Hosting/promotion contract](../../../operator/portal-static-hosting.md#capacity-and-browser-drill-esp-16).

This is a **local loopback drill**, not CDN, VPS-direct or production capacity
acceptance. No thousands-of-users claim is made.

## Frozen workload and hardware

The campaign freezes `workload.json` before starting the server: host CPU,
memory, disk, artifact content hash, images, browser version, URL mix, rates and
budgets. The measured host was:

- 13th Gen Intel Core i7-13620H, 16 CPUs, 31,793 MiB total RAM (11,310 MiB
  available at the baseline run), Linux 7.2.6, 33,757 MiB free disk.
- Caddy `caddy:2.10.2-alpine@sha256:4c6e91c6…` in an isolated container on a
  private Docker network; separate generator container
  `python:3.11-slim@sha256:cce12316…`; host headless Chrome
  `156.0.8078.47`.

Artifact: the verified ESP-13/14 full real-official portal
`sha256:75bc8a1a21702982ebf651aa2c14b8e8b1cd9005c7e68dbea7f80f80fd935682`
(3,409 pages / 3,421 files / 15,932,636 bytes) at `https://portal.example`,
served by the ESP-14 rehearsal Caddyfile. The official 2025 FQ slice is real;
station/price pages remain the synthetic fixture because real prices are blocked
by ESP-07/rights.

Baseline workload: open-loop 100 req/s for 30 s, 32 clients, 1000 ms deadline,
gzip, mix `8 consultar / 4 station / 3 CSS / 2 shard / 1 CSV / 2 JS / 1
sitemap`. Concurrent-update workload: `8 consultar / 4 station / 2 CSS / 2
shard / 1 CSV` (JS and sitemap excluded across the switch), promoting the
noindex ESP-13 artifact `sha256:bd6649fd…` at 8 s and rolling back the original
at 16 s.

Predeclared budgets: p95 ≤50 ms, p99 ≤100 ms, errors ≤1%, achieved ≥90 req/s,
Caddy CPU ≤80%, Caddy memory ≤256 MiB, browser FCP ≤1500 ms, LCP ≤2500 ms,
CLS ≤0.1.

## Delivered harness

- `infra/scripts/load/portal_capacity.sh`: verified release root/current check,
  private Docker network, separate generator container, gzip load, Caddy
  resource sampler, concurrent promote/rollback scenario, browser probe and
  fail-closed report. A trap removes containers/network; the baseline does not
  modify the release root.
- `infra/scripts/load/portal_report.py`: frozen-budget evaluation over the
  stats, driver, Docker resource samples and browser JSON; writes
  `summary.json` and exits nonzero on breach.
- `infra/scripts/load/browser_probe.mjs`: headless Chrome via the DevTools
  Protocol (Node's built-in WebSocket, no dependency), cold cache, desktop and
  mobile (CPU 4×) viewports, FCP/LCP/CLS/TTFB/load/transfer/JS heap, same-origin
  and `/v1` resource checks.
- `stats.py` now reports successful bytes, observation window, achieved rate and
  bytes/s; `drive.py` gained gzip transfer measurements with semantic
  decompression. Existing harness behavior remains default-off/identity.
- `make check-portal-capacity` runs all four self-tests; `make portal-capacity`
  runs the campaign.

Semantic checks passed before load: `/consultar/`, station page, CSS, JS,
search shard, station CSV and sitemap content; real custom 404; gzip and
immutable/revalidation/noindex headers; `/v1/health/live` and `/health/live`
404 (no API proxy). The browser recorded zero external and zero API resources.

## Baseline results

| Request metric | Value |
| --- | ---: |
| Offered / successful | 3,000 / 3,000 |
| Achieved rate | 100.03 req/s |
| p95 / p99 / max | 1.47 ms / 3.45 ms / 10.40 ms |
| Errors / drops / timeouts / wrong / cancelled | 0 / 0 / 0 / 0 / 0 |
| Response bytes / throughput | 12,105,822 / 403,649 B/s |
| Caddy CPU peak | 6.46% |
| Caddy memory peak | 26.64 MiB (container working set) |
| Caddy network RX / TX | 1,540,000 / 15,400,000 B |
| Caddy block read / write | 2,670,000 / 49,200 B |

Browser medians (3 cold runs per page/viewport; desktop 1366×768 DPR1, mobile
390×844 DPR3 CPU 4×; FCP = LCP on these pages):

| Page | Desktop FCP/LCP | Mobile FCP/LCP | Transfer | CLS | Errors |
| --- | --- | --- | ---: | ---: | ---: |
| `/consultar/` | 16 / 16 ms | 32 / 32 ms | 7,646 B | 0 | 0 |
| station | 16 / 16 ms | 36 / 36 ms | 7,598 B | 0 | 0 |
| `/fiscalizacao/` | 24 / 24 ms | 184 / 184 ms | 15,945 B | 0 | 0 |

## Concurrent-update results

The load continued while the release switched at 8 s and rolled back at 16 s:

```text
promoted current=sha256:bd6649fd… previous=sha256:75bc8a1a…
rolled back current=sha256:75bc8a1a… previous=sha256:bd6649fd…
```

| Request metric | Value |
| --- | ---: |
| Offered / successful | 3,000 / 3,000 |
| Achieved rate | 100.03 req/s |
| p95 / p99 / max | 1.25 ms / 1.44 ms / 14.67 ms |
| Errors / drops / timeouts / wrong / cancelled | 0 / 0 / 0 / 0 / 0 |
| Response bytes / throughput | 4,686,971 / 156,279 B/s |
| Caddy CPU peak | 5.91% |
| Caddy memory peak | 25.72 MiB |
| Caddy network RX / TX | 1,420,000 / 7,800,000 B |
| Caddy block read / write | 184,000 / 49,200 B |

Browser medians after rollback (same viewports):

| Page | Desktop FCP/LCP | Mobile FCP/LCP | Transfer | CLS | Errors |
| --- | --- | --- | ---: | ---: | ---: |
| `/consultar/` | 24 / 24 ms | 56 / 56 ms | 7,646 B | 0 | 0 |
| station | 20 / 20 ms | 32 / 32 ms | 7,598 B | 0 | 0 |
| `/fiscalizacao/` | 24 / 24 ms | 108 / 108 ms | 15,945 B | 0 | 0 |

The update run's request mix excludes JS/sitemap across the switch; the
baseline run covers the full asset mix. No station/price claim is inferred from
these synthetic pages.

## Remaining headroom (local, against frozen budgets)

| Metric | Budget | Baseline | Update |
| --- | ---: | ---: | ---: |
| p95 | 50 ms | 1.47 ms (97.1% headroom) | 1.25 ms (97.5%) |
| p99 | 100 ms | 3.45 ms (96.6%) | 1.44 ms (98.6%) |
| errors | 1% | 0% | 0% |
| achieved rate | 90 req/s | 100.03 (+11.1%) | 100.03 (+11.1%) |
| Caddy CPU | 80% | 6.46% (91.9% headroom) | 5.91% (92.6%) |
| Caddy memory | 256 MiB | 26.64 MiB (89.6%) | 25.72 MiB (90.0%) |
| browser FCP | 1500 ms | max 184 ms (87.7%) | max 108 ms (92.8%) |
| browser LCP | 2500 ms | max 184 ms (92.6%) | max 108 ms (95.7%) |
| CLS | 0.1 | 0 | 0 |

## Commands and actual results

```sh
make check-portal-capacity
```

PASS in 6.34 s: stats/driver/report/browser self-tests, including gzip semantic
decompression, error/drop/cancel accounting, budget breach, and CDP resource
checks.

Baseline:

```sh
PORTAL_RELEASE_ROOT=/tmp/fq-portal-ready/esp16-release-root \
PORTAL_SITE_URL=https://portal.example \
PORTAL_RESULTS_DIR=/tmp/fq-portal-ready/esp16-baseline-final3 \
bash infra/scripts/load/portal_capacity.sh
```

PASS. Results: `esp16-baseline-final3` (private, outside Git).

Concurrent update:

```sh
PORTAL_RELEASE_ROOT=/tmp/fq-portal-ready/esp16-release-root \
PORTAL_SITE_URL=https://portal.example \
PORTAL_RESULTS_DIR=/tmp/fq-portal-ready/esp16-update-final3 \
PORTAL_UPDATE_ARTIFACT=/tmp/fq-portal-ready/real-2025-portal \
PORTAL_UPDATE_AT_SECONDS=8 PORTAL_ROLLBACK_AT_SECONDS=16 \
bash infra/scripts/load/portal_capacity.sh
```

PASS. Results: `esp16-update-final3`; final current restored to
`sha256:75bc8a1a…`.

Additional validation: `bash -n portal_capacity.sh`, `python3 -m py_compile`
stats/drive/report, `node --check browser_probe.mjs`, `git diff --check`.
`workload.json` and `summary.json` are retained in each result directory; no
raw workload/source/PII was committed.

## Limits and OWED

- **No CDN or VPS-direct campaign was run.** This is a same-host loopback
  drill with a separate generator container, not a remote load-generator fleet,
  CDN edge, TLS path or production VPS. CDN/VPS results are separate OWED work.
- **No production capacity claim.** The host is a workstation; the artifact and
  Caddy run locally. The numbers cannot be quoted as production throughput or
  user capacity.
- Caddy `docker stats` memory is the container working set, not a process RSS
  measurement; CPU is the peak of ~1 s samples; network/block I/O are cumulative
  container counters. The generator container's own CPU/I/O is not reported,
  but it recorded no drops and `generator_limited=no`.
- The browser is headless Chrome against loopback, not a real mobile device,
  network, TLS/CDN path or accessibility session. ESP-17 owns preview,
  accessibility, keyboard/large-font and operations handoff.
- Real prices remain blocked by ESP-07/rights; the portal's station/price data
  is the synthetic fixture. The real FQ official 2025 slice is present.
- The concurrent-update drill excludes old JS/sitemap across the switch because
  cross-release retention of old content-addressed assets is not implemented.
  A browser holding an old page could request an old JS/sitemap and get 404
  after a release that removes it. That retention/versioned-asset policy is
  OWED before a public cache claim.
- The baseline does not modify the release root; the update drill intentionally
  promotes/rolls back the local root and restores the original current. No VPS,
  DNS, shared Caddy, Compose, API, database or ANP access occurred.
- No `main` merge, PR, production activation or publication occurred.

## Next

ESP-17 — preview, accessibility and operations handoff. Not implemented here.

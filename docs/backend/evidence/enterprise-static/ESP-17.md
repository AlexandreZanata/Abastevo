# ESP-17 — Rehearse preview, accessibility and operations handoff

Status: LOCAL_DONE
Validation: PASS
Integration: INTEGRATION_PENDING
Date: 2026-10-25
Branch: `codex/enterprise-static-portal`
Parent: ESP-16 `a6c3758e`

Rules: B-BR-ESP-13–16; BUC-ESP-06/07.
[Plan](../../../planning/ENTERPRISE_DATA_STATIC_PORTAL_PLAN.md#esp-17--rehearse-preview-accessibility-and-operations-handoff).
[Hosting/promotion contract](../../../operator/portal-static-hosting.md#preview-accessibility-and-operations-handoff-esp-17).

## Scope

An isolated HTTPS preview with Caddy's local internal CA, a noindex edge and a
dedicated private release root. No shared edge, DNS, API, database, ANP, CDN or
production mutation occurred. The preview is a local drill; public TLS/preview
handoff remains OWED.

## Bug found and fixed

The preview probe found that `/metodologia/` and `/404.html` had no shared
stylesheet or search script: `generateStatic` built those pages from a raw
`pageBase` literal instead of `g.base`, leaving `CSSURL`/`ScriptURL` empty. The
pages rendered unstyled and the methodology page lacked the site script.

Fix: both pages now use `g.base`. Regression tests assert the stylesheet on
`metodologia`/`404` and the search-script reference on `consultar`/
`metodologia`/`404`. The ESP-16 measurements were taken before this small
two-page fix; ESP-17 uses freshly generated artifacts:

| Artifact | Content | Shape |
| --- | --- | --- |
| A, noindex preview | `sha256:7c37f485f1e3bda2eaabf7339132ff6734ae5c41233c8e94e136bba1aa43fc48` | 3,409 pages / 3,419 files / 15,231,855 bytes |
| B, full correction candidate | `sha256:914d8b9fab77c807fbf15ef2829b7edc42e6620e4caeb3b022eb2b9694179a64` | 3,409 pages / 3,421 files / 15,932,892 bytes |

Both carry the real 2025 FQ official slice over the synthetic station/price
fixture; real prices remain blocked by ESP-07/rights.

## Delivered

- `infra/caddy/portal-preview.Caddyfile`: `https://localhost:18443` with
  `tls internal`, HTTP `:18082` 308 redirect, HTTP/2, gzip, HSTS, security
  headers, immutable assets, revalidation and `X-Robots-Tag: noindex`.
- `infra/scripts/preview_portal.sh`: promotes A, starts the isolated preview,
  copies the internal CA, verifies HTTPS/headers/cache/redirect/noindex/no-API,
  runs the preview probe on A, promotes B with a fence, probes B, rolls back A,
  probes recovery, then archives and restores the release root.
- `infra/scripts/load/preview_probe.mjs`: headless Chrome CDP probe for
  metadata/landmarks/labels/links, one `h1`, skip link, JS-disabled state and
  station navigation, keyboard focus order/visibility, 200% font mobile reflow,
  same-origin/no-API resources and screenshots.
- `make check-preview-probe` and `make preview-portal`.
- Operator runbook for preview, release-root backup/restore and recovery in
  `docs/operator/portal-static-hosting.md`.

## Actual preview results

Local HTTPS headers observed through Caddy (A current):

```text
HTTP/2 200
strict-transport-security: max-age=3600
x-content-type-options: nosniff
referrer-policy: strict-origin-when-cross-origin
x-frame-options: DENY
permissions-policy: geolocation=(), microphone=(), camera=()
x-robots-tag: noindex, nofollow
cache-control: public, max-age=0, must-revalidate
```

CSS asset: `cache-control: public, max-age=31536000, immutable`,
`content-type: text/css; charset=utf-8`, `content-encoding: gzip`. The HTTP
listener returned 308 to the HTTPS URL. TLS 1.2 verification against the copied
internal CA passed; `/v1/health/live` and `/health/live` returned 404. A had no
sitemap/script; B returned `sitemap.xml` 200 and carried the search script; the
rollback restored A's 404/no-script state. The final preview current is A.

Preview probe A, B and recovered-A all passed with **0 failures and 0
warnings**:

| Check | A / B / recovered |
| --- | --- |
| Pages checked | 7 (consultar, preços, state, station, fiscalização, metodologia, dados) |
| Dated/source/official markers | all present (station source/checksum; official negative-certificate/history disclaimers) |
| `lang`, one `h1`, landmarks, skip link | pass |
| Labelled controls / non-empty links / image alt | pass |
| No-JS `<noscript>` + state directory | 2 state links; state page and station page render |
| Keyboard | 10 stops; first is skip link; focus-visible solid 3 px outline |
| Large font mobile (200%) | 6 pages, `scrollWidth == innerWidth`, 0 small primary targets |
| External / API resources | 0 / 0 |
| Screenshots | 14 per artifact (desktop, no-JS, mobile-large-font) |

The no-JS state page was `Preços em Minas Gerais (MG)` with its sample and
`postos` links; the station page still showed `Posto Alfa`, source, checksum and
history.

## Correction/recovery and backup

A → B promotion and rollback under the preview edge passed with no request or
page failure. The preview current ended at A; B remains a previous release. The
edge noindex header is the preview gate; B's own meta is indexable because it is
a publication candidate, not a preview build.

Release-root backup/restore:

```text
backup: state.json + releases/ + current
sha256: a99a3ee9c1d1d3aeaece22d978ce33f4c6efca09b63e4369abcaa1a9f712fb94
restored verify: current=sha256:7c37f485…, 3,409 pages / 3,419 files
restored rollback: B → A
```

The restored root verified A, selected B and returned to A. This proves the
runbook; it does not replace source/export artifacts, which are the ultimate
reproducibility input.

## Commands and actual results

```sh
make check-preview-probe                 # PASS
bash infra/scripts/preview_portal.sh     # PASS 61.98 s
```

The full preview command used the new A/B artifacts, preview root
`/tmp/fq-portal-ready/esp17-preview-root` and results
`/tmp/fq-portal-ready/esp17-preview-results` (private, outside Git). It verified
the internal CA TLS, HTTP/2/HTTPS headers/redirect/noindex, A/B/recovery probes,
screenshots and backup/restore.

Portalgen regression:

```sh
go test -count=1 ./internal/modules/enterpriseexport/portalgen
go test -count=1 -tags=integration ./internal/modules/enterpriseexport/portalgen \
  -run TestRealOfficialSnapshotPages
```

PASS. `bash -n`, `node --check`, Caddy `validate`, `gofmt`, `git diff --check`
also PASS. No raw source/PII, credential or release artifact was committed.

## Outstanding gaps

- The preview uses Caddy's local internal CA and `localhost`, not public
  DNS/TLS/CDN or a shared preview edge. Public preview provisioning and TLS
  certificates remain ESP-18 handoff work.
- Headless Chrome is not a real mobile device or screen reader; screenshots and
  DOM/CDP checks are not a substitute for device/assistive-technology acceptance.
  Focus order and visible outline passed; color contrast is not measured here.
- B's meta is indexable because it is the publication candidate; the preview
  edge blocks indexing with `X-Robots-Tag`. A dedicated noindex full preview
  build is OWED if the product needs page-level noindex on a full candidate.
- Cross-release retention of old content-addressed JS/sitemap is not
  implemented; an old page requesting a removed asset can 404 after a release.
- Real prices remain blocked by ESP-07/rights; the preview uses the real
  official 2025 slice and the synthetic price fixture.
- Production host/CDN/TLS/DNS, public preview/VPS handoff, cache purge,
  multi-host retention and release certification remain OWED.
- No `main` merge, PR, publication or shared-edge mutation occurred.

## Next

ESP-18 — publication handoff, community/release operations and final scoped
certification planning. Not implemented here.

# P25 Release Evidence — Static abastevo Landing Page

- **Milestone:** 19 — `P25 — Static abastevo landing page`
- **Gate:** G25-STATIC-READY
- **Date:** 2026-10-05
- **Branch:** `codex/phase-25-static-landing`
- **Status:** G25-STATIC-READY achieved for local static artifact; publication and store activation procedures documented.

## 1. Executive Summary

Phase P25 delivers the complete, truthful, high-performance static website and legal presence for **abastevo**. Built with pure static HTML5, accessible CSS design tokens, and TypeScript 7.0.2 compiled directly via `tsc` to a standard browser JavaScript module, the site runs with **zero runtime frameworks** or client libraries. All essential content, navigation, and legal statements work with JavaScript disabled.

Asset references are content-hashed with SHA-256 for long-term immutable caching (`Cache-Control: public, max-age=31536000, immutable`). The site adheres strictly to B-BR-L01–L07 product rules, displaying a truthful prelaunch store state ("Em breve na Google Play") with zero phantom ratings or fake download counts, explicit distinction between community and ANP survey sources, a strict 24-hour retention statement for audit photos, and an absolute no-cookie/no-tracker policy.

## 2. Pinned Toolchain & Reproducibility

| Component | Pinned Version / Value | Verification |
|---|---|---|
| Node.js | v26.3.1 | Built-in test runner (`node --test`), `assert`, ES modules |
| TypeScript | `7.0.2` exact (devDependency only) | `tsc --project tsconfig.json`, `strict: true`, `noEmitOnError: true` |
| Runtime dependencies | None (0) | Handcrafted HTML5/CSS3/DOM APIs only |
| Asset hashing | SHA-256 8-character hex | `styles.[hash].css`, `main.[hash].js` |
| Reproducibility | Byte-identical output | Verified by `landing/tests/build-reproducibility.test.mjs` |

## 3. Page Inventory & Content Contracts

| Path | Purpose | Key Content & Constraints |
|---|---|---|
| `/` | Landing page | Clear H1, 3-step guide (Consulte, Confira, Contribua), illustrative comparison labeled fictional, project summary with privacy highlights, prelaunch store notice ("Em breve na Google Play"), FAQ via native `<details>`, quiet footer citing ANP source and MIT license. |
| `/privacidade/` | Privacy statement (B-BR-L07) | Zero cookies/trackers, no account or GPS required for browsing, audit photos expire in <= 24 hours across all copies (project policy), LGPD rights and contact channel. |
| `/excluir-conta/` | Account & data deletion (Play Console requirement) | Clear web instructions for account erasure request, data scope removed vs. aggregated price facts, backup horizon clarification. |
| `/termos/` | Terms of use & community guidelines | MIT open source software terms, brand name/logo ownership, community reporting and reviewed moderation policy (no automatic deletion by votes). |
| `/404.html` | Not found fallback | Friendly Portuguese message, links back to home and GitHub repository, `<meta name="robots" content="noindex, nofollow">`, no canonical link. |

## 4. Brand Identity & Derived Assets

All assets are strictly derived from approved sources under `docs/assets/brand/`:

| Emitted Asset | Source | Dimensions / Format | Size / Budget |
|---|---|---|---|
| `assets/brand/favicon.svg` | `docs/assets/brand/abastevo-app-icon.svg` | Vector SVG | 23.8 KB |
| `assets/brand/favicon.ico` | `docs/assets/brand/abastevo-app-icon-1024.png` | 16x16, 32x32, 48x48 ICO | 15.1 KB |
| `assets/brand/apple-touch-icon.png` | `docs/assets/brand/abastevo-app-icon-1024.png` | 180x180 PNG | 14.3 KB |
| `assets/brand/icon-192.png` | `docs/assets/brand/abastevo-app-icon-1024.png` | 192x192 PNG | 15.5 KB |
| `assets/brand/icon-512.png` | `docs/assets/brand/abastevo-app-icon-1024.png` | 512x512 PNG | 56.5 KB |
| `assets/brand/og-image-v1.png` | `docs/assets/brand/abastevo-logo.svg` + tokens | 1200x630 PNG | 96.1 KB (budget <= 300 KB) |
| `site.webmanifest` | PWA manifest | Standard JSON | 481 bytes |
| `icon-provenance.json` | Provenance log | JSON | 1.1 KB |

## 5. Technical SEO & Response Policy

- **Canonical origin:** `https://abastevo.com.br/` (validated HTTPS apex, trailing slash normalized).
- **Titles & Descriptions:** Checked by automated tests:
  - Titles: 30 to 65 characters (`abastevo | Preços de combustíveis com a comunidade`, `49` chars).
  - Descriptions: 110 to 165 characters (`Consulte preços de combustíveis com fonte...`, `147` chars).
- **Open Graph / Twitter:** `og:site_name`, `og:locale` (`pt_BR`), `og:image` (versioned `og-image-v1.png`), `twitter:card` (`summary_large_image`).
- **Structured Data (JSON-LD):**
  - Prelaunch state: `@graph` with `WebSite`, `Organization` (with GitHub `sameAs`), and `WebPage`. Negative assertion: zero `MobileApplication`, ratings, or fake price offers.
  - Published state: adds `MobileApplication` schema pointing to confirmed Google Play package `com.anpfuel`.
  - Legal pages: valid `BreadcrumbList`.
- **Discovery files:**
  - `robots.txt`: allows `/`, points to `https://abastevo.com.br/sitemap.xml`.
  - `sitemap.xml`: lists exactly the 4 canonical indexable 200 URLs; omits 404; omits fabricated `lastmod`.
  - `llms.txt`: factual project summary for LLMs and AI crawlers (D-L07).
  - `.well-known/security.txt`: RFC 9116 compliant security policy with advisories contact.
- **Security headers baseline (`_headers`):**
  - `X-Content-Type-Options: nosniff`
  - `Referrer-Policy: strict-origin-when-cross-origin`
  - `Permissions-Policy: camera=(), microphone=(), geolocation=()`
  - `Content-Security-Policy: default-src 'self'; img-src 'self' data:; style-src 'self'; script-src 'self'; object-src 'none'; base-uri 'self'; form-action 'none'`
  - `Strict-Transport-Security: max-age=31536000; includeSubDomains`

## 6. Performance Budget Measurements

Measured against built production artifact under `landing/dist/`:

| Metric | Budget | Measured | Margin | Result |
|---|---|---|---|---|
| CSS (gzipped) | <= 25 KiB | 2.5 KiB (2,605 B) | -89.8% | **PASS** |
| JS (gzipped) | <= 10 KiB | 1.3 KiB (1,308 B) | -87.2% | **PASS** |
| Total initial transfer (HTML + CSS + JS) | <= 500 KiB | ~8.0 KiB | -98.4% | **PASS** |
| Social preview image | <= 300 KiB | 93.9 KiB (96,137 B) | -68.7% | **PASS** |
| Contrast ratios (WCAG AA) | >= 4.5:1 text, >= 3:1 UI | 6.27 to 16.92 | Meets/exceeds | **PASS** |

## 7. Verification Evidence

### Automated Suite (`landing/tests/`)
All 21 automated tests pass cleanly in 1.46s:
- `build-reproducibility.test.mjs`: byte-identical output across consecutive builds (PASS).
- `no-leaks`: zero `.ts`, `.map`, or private config in `dist/` (PASS).
- `content-without-js.test.mjs`: core headings, links, and sections render without JS (PASS).
- `headers-and-redirects.test.mjs`: security headers and 301/404 redirect rules (PASS).
- `origin-validation.test.mjs`: strictly enforces valid HTTPS origin without paths (PASS).
- `progressive-enhancement.test.mjs`: optional interactions graceful degradation (PASS).
- `visual-tokens.test.mjs`: color contrast meets WCAG AA standards (PASS).
- `store-state.test.mjs`: prelaunch truthful copy vs. published verified Play Store link (PASS).
- `seo-and-metadata.test.mjs`: title/description lengths, Open Graph, Twitter cards, icon links (PASS).
- `json-ld.test.mjs`: schema graphs, ID uniqueness, prelaunch negative assertions, published mobile app schema, breadcrumbs (PASS).
- `indexing-and-sitemap.test.mjs`: sitemap XML validity, robots.txt, llms.txt, security.txt, preview `noindex` guard (PASS).

### Integration & Gate Checks
- `npm --prefix landing run typecheck`: PASS (0 errors).
- `npm --prefix landing run build`: PASS.
- `npm --prefix landing run check`: PASS.
- `npm --prefix landing test`: 21/21 PASS.
- `bash scripts/tests/test-gate-selection.sh`: 15/15 PASS.
- `git diff --check`: PASS (0 whitespace/formatting defects).
- `bash scripts/scan-secrets.sh`: PASS (0 leaks or high-confidence secrets).

## 8. Honest Observations & Pending Evidence

1. **Owed manual batch:** Under user directive 2026-10-02 (PLANNING), emulator and device executions are paused until all P phases complete. Manual cross-browser inspection and WhatsApp/Telegram social link-preview rendering are recorded as owed and will be executed during the final manual validation batch alongside mobile acceptance.
2. **Field metrics:** Core Web Vitals (CWV) field measurements and Google Search Console performance data are recorded as PENDING actual public website traffic.
3. **Gate scope:** G25-STATIC-READY certifies that the static landing artifact is complete, truthful, secure, and ready for publication. It does **not** certify Android/backend production release (G09/G24), does not deploy the site, and does not claim search ranking.

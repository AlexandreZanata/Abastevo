# abastevo — Landing Page

Static landing page and public web presence for the **abastevo** project.

## Stack and Architecture

- **Authored markup:** Plain static HTML5, accessible semantic structure, Portuguese (`pt-BR`).
- **Styles:** Plain CSS with design tokens, responsive layout (mobile-first, 48rem and 64rem breakpoints), high-contrast accessible focus rings, print stylesheet, zero remote fonts.
- **Typography:** Self-hosted variable Manrope for display/navigation/CTAs/prices and Source Sans 3 for prose/FAQ/legal copy; Latin and Latin Extended WOFF2, `font-display: swap`, hash-checked assets and SIL OFL notices. Source/license/security provenance: [font manifest](static/assets/fonts/provenance.json). No font CDN or mobile typography change.
- **Language & Compilation:** Pure TypeScript **7.0.2** (`strict: true`, `noEmitOnError: true`) compiled to standard browser JavaScript module.
- **Progressive enhancement:** All essential content, navigation, and legal links exist in plain HTML and function completely with JavaScript disabled or unavailable. TypeScript is used only for optional enhancements (active section highlight via IntersectionObserver, copy button helper, and the portal's regional static search shard loader).
- **Asset fingerprinting:** `scripts/build.mjs` generates content-hashed asset filenames (e.g. `styles.[hash].css`, `main.[hash].js`) and rewrites HTML references for safe immutable caching (`Cache-Control: public, max-age=31536000, immutable`).
- **Zero runtime dependencies:** Built without frameworks (no React, Astro, Next, Svelte, Tailwind). Only TypeScript compiler is used as devDependency. Test suite uses Node's built-in test runner (`node --test`) and `assert`.

## Directory Layout

```text
landing/
  static/
    index.html                    # Main landing page
    privacidade/index.html        # Privacy statement (B-BR-L07)
    excluir-conta/index.html      # Account/data deletion request page (Play requirement)
    termos/index.html             # Community guidelines and moderation terms
    404.html                      # Real 404 page with noindex
    robots.txt                    # Crawler policy & sitemap pointer
    site.webmanifest              # Web application manifest with icons
    llms.txt                      # Factual project summary for LLMs (D-L07); content links generated
    .well-known/security.txt      # RFC 9116 security policy & contact
    favicon.ico                   # Root legacy multi-resolution favicon
    _headers                      # Security headers & cache rules for static hosts
    _redirects                    # Redirect definitions
    assets/
      styles.css                  # Token-based styles
      fonts/                      # Local variable WOFF2, OFL notices & provenance
      brand/                      # Approved SVGs, derived icons & versioned social preview
        favicon.svg               # Modern vector favicon
        apple-touch-icon.png      # 180x180 iOS touch icon
        icon-192.png              # 192x192 Android manifest icon
        icon-512.png              # 512x512 Android splash icon & schema logo
        og-image-v1.png           # 1200x630 versioned social preview (<300 KiB)
        icon-provenance.json      # Provenance and derivation log
  content/
    manifest.json                 # Validated page manifest: static pages, hubs, categories, popular guides
    guias/*.md                    # Guides (front matter + Markdown subset) → /guias/{slug}/
    ferramentas/*.md              # Calculator explanations → /ferramentas/{slug}/
    paginas/*.md                  # E-E-A-T pages → /sobre/, /politica-editorial/
  src/
    main.ts                       # TypeScript progressive enhancement
    search.ts                     # Portal static search (SP-10); emits search.<hash>.js
    tools.ts                      # Calculators: pure formulas + optional DOM enhancement → tools.<hash>.js
  scripts/
    build.mjs                     # Asset fingerprinting & HTML build
    check.mjs                     # Artifact, SEO, JSON-LD, link & budget verification
    content/                      # Manifest loader, Markdown renderer, layouts, shared data
  tests/                          # Automated contract & behavior tests
  dist/                           # Deployable built output (gitignored)
  package.json
  package-lock.json
  tsconfig.json
  README.md
```

## Commands

```bash
# Clean install exact dependencies
npm ci

# Typecheck TypeScript sources with noEmit
npm run typecheck

# Build deployable static artifact to dist/
npm run build

# Verify build integrity, security headers, and size budgets
npm run check

# Run automated test suite
npm test
```

### Content pipeline (SEO-08)

Every page comes from `content/manifest.json`: authored HTML pages are listed
there with a fixed `lastmod`, and guides, category hubs (`/guias/{categoria}/`),
the `/guias/` and `/ferramentas/` hubs, calculators and `/sobre/` /
`/politica-editorial/` are generated from Markdown. The build fails on invalid
front matter (required fields, title/description bounds, real dates, known
category, unique slug matching the file name) and on any internal link or
`related` route that is neither a landing page nor a known portal route
(`/precos/`, `/precos/{uf}/`, `/consultar/`, `/dados/`, `/metodologia/`,
`/curiosidades/`, `/fiscalizacao/`).

Guide front matter:

```yaml
---
title: Como é formado o preço da gasolina no Brasil   # <title>, 25–65 chars with " | abastevo"
headline: Optional longer H1 / Article headline
description: 110–165 characters
summary: Optional card text
slug: como-e-formado-o-preco-da-gasolina             # must equal the file name
category: precos                                     # a manifest category
tags: [gasolina, impostos]
datePublished: 2026-11-14
dateModified: 2026-11-14                            # drives sitemap lastmod
related:
  - /guias/icms-sobre-combustiveis/
  - /precos/
---
```

The in-house Markdown subset supports `##`–`####` headings (no h1, no skipped
levels), paragraphs, `**bold**`, `*italic*`, `` `code` ``, `[links](url)`
(https, internal paths, `#anchors` only), one-level `-`/`1.` lists, pipe tables
followed by a `: caption` line, `>` quotes, standalone `![alt](/assets/...)`
images (alt required), fenced code and `::: resumo|nota|atencao` callouts. All
text is HTML-escaped; raw HTML is never passed through.

`sitemap.xml` is generated from the same registry: indexable pages only, one
`lastmod` per URL (front matter `dateModified` or the manifest date, never the
build time) and no `priority`/`changefreq`.

### Calculators (SEO-10)

`/ferramentas/{etanol-ou-gasolina,custo-de-viagem,consumo-medio,custo-por-km,encher-o-tanque}/`
explain the formula and a worked example in plain HTML. `src/tools.ts` holds the
pure parsing (pt-BR decimal comma), validation and formulas used by the tests,
plus a DOM enhancement that reveals the form, reads optional `?campo=valor`
prefill (canonical stays the clean URL) and writes results with `textContent`
into a polite live region. No values leave the browser.

### Portal search asset

`npm run build` emits `dist/assets/search.<hash>.js` from `src/search.ts`. The
enterprise portal generator consumes that exact file with `--search-js`; the
browser loads one immutable regional shard (`/dados/busca/<release>/<uf>.json`)
and never calls the Abastevo API, the ANP API or a database. The search is an
optional enhancement: the HTML directory navigation remains usable without it.

### Build Options

`scripts/build.mjs` accepts command-line arguments:

- `--origin <url>`: Canonical HTTPS origin (defaults to `https://abastevo.com.br`). Insecure or malformed origins are strictly rejected.
- `--store-state <prelaunch|published>`: State of Google Play Store listing. Default is `prelaunch`.
- `--store-url <url>`: Verified Play Store URL. Required if `--store-state published`. Must match `https://play.google.com/store/apps/details?id=com.anpfuel`.
- `--out-dir <path>`: Output directory (defaults to `dist`).
- `--api-origin <url>` (or `ABASTEVO_API_ORIGIN` env): API origin baked into
  `<meta name="abastevo-api">` for the `/contribuidores/` live list. Must be
  an `https:` origin without path when set; empty (default) keeps the
  enhancement disabled and the page fully static.

### Check Options

`scripts/check.mjs` verifies every emitted landing page (unique titles and
descriptions, self-canonical on the configured origin, valid JSON-LD, no inline
scripts, resolving internal links with portal routes allowlisted) and the
sitemap/robots/llms files. `--origin <url>` defaults to
`https://abastevo.com.br`; `--dist-dir <path>` defaults to `dist`.

## Hosting & Response Policy

The generated `dist/` directory contains plain static files ready to deploy to any modern static hosting provider.

- **Preferred Hosts:** Cloudflare Pages or Netlify natively support the provided `_headers` and `_redirects` files, enabling:
  - Custom security headers (`nosniff`, `strict-origin-when-cross-origin`, strict CSP).
  - Permissions policy disabling camera, microphone, and geolocation.
  - HSTS with subdomains.
  - Long immutable caching for `/assets/*` (1 year) and zero-cache revalidation for HTML files.
  - Real 404 response for unmatched paths.
- **GitHub Pages:** Can serve the files, but custom response headers are not configurable. In-document metadata provides defense-in-depth if GitHub Pages is chosen.

The 404 follows B-BR-L04 (essential content is static) and BUC-L02 (the
repository link works without JavaScript). The build uses the home page as
the source of its header and footer, with section links pointing back to the
home page. An unknown URL returns HTTP 404 with `noindex` and offers direct
links to the home page and the tutorial hub.

## Rollback Procedure

All static deployments are immutable artifacts:
1. **Immediate Rollback (Dashboard):** In the static host management console (e.g. Cloudflare Pages or Netlify), click **Rollback** to instantly reactivate the prior successful deployment.
2. **Git Rollback:** Revert to the previous verified commit SHA, run `npm run build && npm run check`, and deploy the resulting `dist/` directory.

## Publication-Time Discoverability Checklist

When website publication is explicitly authorized:
1. Configure custom domain DNS (apex `abastevo.com.br` and `www.abastevo.com.br` redirect) pointing to static host.
2. Verify TLS certificate and HTTPS enforcement.
3. Add the website link to the GitHub repository homepage and `README.md`.
4. Register `https://abastevo.com.br` in **Google Search Console** and submit `https://abastevo.com.br/sitemap.xml`.
5. Import property in **Bing Webmaster Tools**.
6. Monitor Search Console for crawl errors or indexing coverage.

## Google Play Store Activation Procedure

When the Android app is officially published on Google Play:
1. Confirm the live Play Store URL: `https://play.google.com/store/apps/details?id=com.anpfuel`.
2. Build the landing page in published mode:
   ```bash
   node scripts/build.mjs --store-state published --store-url "https://play.google.com/store/apps/details?id=com.anpfuel"
   ```
3. Run checks to verify the build:
   ```bash
   node scripts/check.mjs
   npm test
   ```
4. Confirm that:
   - The CTA renders the link "Ver na Google Play" instead of the prelaunch notice.
   - The JSON-LD in `dist/index.html` includes the `MobileApplication` schema pointing to the live URL.
   - `Organization` structured data `sameAs` includes the Play Store link.
5. Deploy `dist/` to the static hosting provider.

## Landing redesign follow-up (2026-10-11)

Use branch **landing** and the isolated `.worktrees/landing-refresh` checkout for
this user-authorized website scope. [LR00–LR39 plan](../docs/planning/LANDING_REDESIGN_EXECUTION_PLAN.md),
[implementation prompt](../docs/planning/LANDING_REDESIGN_EXECUTION_PROMPT.md) and
[authentic reference/media](../docs/assets/landing/redesign-20261011/README.md) keep
the hero, real app gallery and tutorials separate. Fonts are integrated locally;
production redesign, complete tutorials and SEO follow-up remain planned.
The reference lives in documentation and is not emitted by the production build.

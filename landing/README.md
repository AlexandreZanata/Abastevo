# abastevo — Landing Page

Static landing page and public web presence for the **abastevo** project.

## Stack and Architecture

- **Authored markup:** Plain static HTML5, accessible semantic structure, Portuguese (`pt-BR`).
- **Styles:** Plain CSS with design tokens, responsive layout (mobile-first, 48rem and 64rem breakpoints), high-contrast accessible focus rings, print stylesheet, zero remote fonts.
- **Language & Compilation:** Pure TypeScript **7.0.2** (`strict: true`, `noEmitOnError: true`) compiled to standard browser JavaScript module.
- **Progressive enhancement:** All essential content, navigation, and legal links exist in plain HTML and function completely with JavaScript disabled or unavailable. TypeScript is used only for optional enhancements (active section highlight via IntersectionObserver, copy button helper).
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
    sitemap.xml                   # Static sitemap (canonical 200 URLs only)
    site.webmanifest              # Web application manifest with icons
    llms.txt                      # Factual project summary for LLMs (D-L07)
    .well-known/security.txt      # RFC 9116 security policy & contact
    favicon.ico                   # Root legacy multi-resolution favicon
    _headers                      # Security headers & cache rules for static hosts
    _redirects                    # Redirect definitions
    assets/
      styles.css                  # Token-based styles
      brand/                      # Approved SVGs, derived icons & versioned social preview
        favicon.svg               # Modern vector favicon
        apple-touch-icon.png      # 180x180 iOS touch icon
        icon-192.png              # 192x192 Android manifest icon
        icon-512.png              # 512x512 Android splash icon & schema logo
        og-image-v1.png           # 1200x630 versioned social preview (<300 KiB)
        icon-provenance.json      # Provenance and derivation log
  src/
    main.ts                       # TypeScript progressive enhancement
  scripts/
    build.mjs                     # Asset fingerprinting & HTML build
    check.mjs                     # Artifact, SEO, JSON-LD & budget verification
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

### Build Options

`scripts/build.mjs` accepts command-line arguments:

- `--origin <url>`: Canonical HTTPS origin (defaults to `https://abastevo.com.br`). Insecure or malformed origins are strictly rejected.
- `--store-state <prelaunch|published>`: State of Google Play Store listing. Default is `prelaunch`.
- `--store-url <url>`: Verified Play Store URL. Required if `--store-state published`. Must match `https://play.google.com/store/apps/details?id=com.anpfuel`.
- `--out-dir <path>`: Output directory (defaults to `dist`).

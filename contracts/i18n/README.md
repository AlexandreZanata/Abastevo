# i18n contracts

Shared inputs for the two public generators:

- `landing/` (Node) reads `locales.json` through
  `landing/scripts/content/i18n.mjs`; it owns the landing locales now.
- `backend/internal/modules/enterpriseexport/portalgen` (Go) reads the same
  registry and glossary in SEO-28.

`locales.json` is the single locale registry: codes, URL prefixes, HTML and
hreflang values, `og:locale`, decimal separators, `enabled` flags and the
translated route segments. `glossary/{locale}.json` maps pt-BR terms to the
terms used by the app and the site. `../testdata/i18n/routes.json` holds the
URL vectors both generators must reproduce.

Do not add a locale or change a prefix in one generator only. `enabled` stays
`false` until reviewed translations and the named reviewer exist
([ADR-026](../../docs/decisions/026-multilingual-public-site.md)).

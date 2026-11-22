# i18n contracts

Shared inputs for the two public generators:

- `landing/` (Node) reads `locales.json` through
  `landing/scripts/content/i18n.mjs`.
- `backend/internal/modules/enterpriseexport/portalgen` embeds a generated
  copy of the registry alongside its message catalogs. After a registry
  change, run `go generate ./internal/modules/enterpriseexport/portalgen`
  from `backend/`. A Go test requires the copy to match the shared source.

`locales.json` is the single locale registry: codes, URL prefixes, HTML and
hreflang values, `og:locale`, decimal separators, date formats, `enabled`
flags, reviewers, section segments and fuel slugs.
`glossary/{locale}.json` maps pt-BR terms to the
terms used by the app and the site. `../testdata/i18n/routes.json` holds the
URL vectors both generators must reproduce.

A locale or prefix change belongs in the shared registry. A localized page
requires an enabled locale and the review recorded by its generator. The
landing uses a reviewed dictionary and page translation. The portal uses a
reviewed catalog and the same Portuguese data gates for indexability. See
[ADR-026](../../docs/decisions/026-multilingual-public-site.md),
[ADR-027](../../docs/decisions/027-ai-reviewed-landing-translations.md) and
[ADR-028](../../docs/decisions/028-ai-reviewed-portal-translations.md).

The `es` glossary targets Latin American readers: the neutral word comes
first and the regional one beside it on first mention, as in "gasolina
(nafta)" and "diésel (gasoil)".

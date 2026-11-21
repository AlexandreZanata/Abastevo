# Search Console report

`search_report.py` is a read-only tool for the owner. It never runs in CI or on
the VPS. It reads Google Search Console, compares it with the public sitemaps
and writes a report by locale and page family, so the effect of a change to
one family (state pages, fuel-by-city pages, guides) can be read in one table.

It uses the Python standard library only and makes three kinds of request:

| Request | Host | Token |
| --- | --- | --- |
| `GET /sitemap.xml`, `GET /sitemap-portal.xml` and the files that index lists | `--origin` | not sent |
| Search Analytics, `POST …/searchAnalytics/query` (pages, then pages with queries) | `www.googleapis.com` | sent |
| URL Inspection, `POST …/urlInspection/index:inspect` (one per inspected URL) | `searchconsole.googleapis.com` | sent |

Every request names the client as `abastevo-search-report/1`; the public edge
answers 403 to the default Python signature. Redirects are not followed.

## Get a token

The script needs an OAuth 2.0 access token with the read-only scope
`https://www.googleapis.com/auth/webmasters.readonly`, issued for a Google
account that can open the property in Search Console. An API key does not
work: both APIs return private data and require a signed-in user.

One way that needs no Google Cloud project is Google's OAuth 2.0 Playground:

1. Open <https://developers.google.com/oauthplayground/> signed in with that
   account.
2. In the first step, type the scope
   `https://www.googleapis.com/auth/webmasters.readonly` and authorize it.
3. In the second step, exchange the authorization code for tokens and copy
   the access token.

An access token expires after about one hour. Get a new one when the script
answers `HTTP 401`. The script takes only the access token, never the refresh
token, and the read-write `webmasters` scope is not needed: it only reads.

Hand the token to the script through the environment, without leaving it in
the shell history or in any file:

```bash
read -rs GSC_ACCESS_TOKEN && export GSC_ACCESS_TOKEN
```

The script never prints, logs or writes the token, and sends it only to the
two Google hosts above.

## Run

From the repository root:

```bash
python3 scripts/seo/search_report.py
```

| Input | Default | Meaning |
| --- | --- | --- |
| `GSC_ACCESS_TOKEN` | none, required | The access token. |
| `GSC_SITE` | `sc-domain:abastevo.com.br` | The property as Search Console names it. A URL-prefix property ends with `/`. |
| `--end` | 3 days before today | Last day, `YYYY-MM-DD`. The newest days are still incomplete in Search Console. |
| `--start` | 27 days before `--end` | First day, `YYYY-MM-DD`. The default period is 28 days. |
| `--origin` | `https://abastevo.com.br` | Origin whose sitemaps list the submitted URLs. |
| `--out` | `.local/seo/` | Output directory. |
| `--inspect` | `20` | Sitemap URLs to inspect per locale and family; `0` skips URL Inspection. |

URL Inspection is slow (one request per URL) and Google limits it to 2,000
requests per property per day, so one run inspects at most 500 URLs. When the
cap binds, the number per family shrinks evenly. A family's URLs are taken at
even steps of its sorted list, so one state does not fill the sample.

A failure prints one line and exits 1 without writing any file:

```text
search-report: HTTP 403 from https://www.googleapis.com/…/searchAnalytics/query: User does not have sufficient permission for site …
```

`HTTP 429` and `HTTP 5xx` are retried once after 5 seconds; nothing else is
retried.

## Output

| File | Content |
| --- | --- |
| `report.md` | The four sections below. |
| `pages.csv` | One row per page with impressions: `url`, `locale`, `family`, `in_sitemap`, `clicks`, `impressions`, `ctr`, `position`. |
| `queries.csv` | One row per page and query: `url`, `query`, `locale`, `family`, `clicks`, `impressions`, `ctr`, `position`. |
| `inspection.csv` | One row per inspected URL: `url`, `locale`, `family`, `verdict`, `coverage_state`. |

`report.md` has:

1. One table per locale with a row per family: URLs in the sitemaps, URLs with
   impressions, clicks, impressions, CTR and the average position weighted by
   impressions.
2. The inspected URLs per family, counted by coverage state.
3. The 20 queries with the most clicks in each family.
4. The pages with at least 50 impressions whose CTR is below the median of
   their family. The median is taken over the pages of the same locale and
   family that also have at least 50 impressions; smaller pages are mostly at
   zero and would pull every median down to zero.

Search Console hides rare queries for privacy, so the query rows add up to
less than the page rows. In `queries.csv`, a query that starts with `=`, `+`,
`-` or `@` gets a leading apostrophe so a spreadsheet does not run it as a
formula; the portal CSV downloads follow the same rule.

**Never commit the output.** It holds the site's search queries and index
state. `.local/` is git-ignored; if you pass `--out`, keep it under `.local/`
or outside the repository, and do not paste the files into issues or pull
requests.

## Families

`classify(path)` returns `(locale, family)`. The locale is the URL prefix of
[`contracts/i18n/locales.json`](../../contracts/i18n/locales.json) (`/en`,
`/es`); every other path is `pt-BR`. Under a locale prefix, guides and tools
are recognized by that locale's segment names from the same file.

| Family | Routes |
| --- | --- |
| `home` | `/` |
| `guias` | `/guias/…` |
| `ferramentas` | `/ferramentas/…` |
| `como-usar` | `/como-usar/…` |
| `glossario` | `/glossario/…` |
| `institucional` | `/sobre/`, `/politica-editorial/`, `/privacidade/`, `/termos/`, `/excluir-conta/`, `/contribuidores/` |
| `precos-hub` | `/precos/` |
| `combustivel` | `/precos/{fuel-slug}/` |
| `estado` | `/precos/{uf}/` |
| `cidade` | `/precos/{uf}/{city-slug}/` |
| `combustivel-estado` | `/precos/{fuel-slug}/{uf}/` |
| `combustivel-cidade` | `/precos/{fuel-slug}/{uf}/{city-slug}/` |
| `paridade` | `/precos/etanol-ou-gasolina/` |
| `posto` | `/postos/…` |
| `forum` | `/forum/…` |
| `fiscalizacao` | `/fiscalizacao/…` |
| `dados` | `/dados/…` |
| `consultar` | `/consultar/` |
| `metodologia` | `/metodologia/` |
| `curiosidades` | `/curiosidades/` |
| `outros` | Anything else, and every URL of another host or scheme. |

- A `{uf}` is two lowercase letters and a `{city-slug}` is a lowercase slug.
  Other shapes under `/precos/`, such as the retired `/precos/uf/MT/…` routes,
  are `outros`.
- The fuel slugs are a copy of the `Slug` values of `fuelVocabulary` in
  `backend/internal/modules/enterpriseexport/portalgen/seo_head.go`. The test
  reads that file and fails when the two lists differ.
- The locale registry holds no translated page slugs. Under a locale prefix, a
  single slug that no other rule matches (such as `/en/about/`) is the
  registry's `page` kind and counts as `institucional`.

When a new route family ships, add it to `classify()` and to the tests. The
portal routes are described in
[`docs/enterprise/portal-routes.md`](../../docs/enterprise/portal-routes.md).

## Tests

```bash
make test-seo
```

The tests replace the HTTP layer with a function that serves fixture sitemaps
and Search Console answers. They need no token and reach no external host.

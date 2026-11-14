# Enterprise portal routes and editorial contract

Status: FROZEN FOR CONSTRUCTION / PROTOTYPE_SYNTHETIC. Date: 2026-10-24.
Task: ESP-08. Owner: [enterprise data/static portal plan](../planning/ENTERPRISE_DATA_STATIC_PORTAL_PLAN.md#esp-08--freeze-portal-routes-fields-and-editorial-templates).
Rules: [B-BR-ESP-03/06–09/12/16](ENTERPRISE_DATA_STATIC_PORTAL_CONTRACT.md).

This contract freezes the public consultation routes, fields, templates, states
and editorial rules. It does not generate real pages, enable a source, publish a
file or change the existing landing. The prototype under `landing/prototype/` is
labelled synthetic data and is not part of the landing build or `dist/`.

## Route families

| Route | Purpose | Indexability | Data |
| --- | --- | --- | --- |
| `/consultar/` | Station/name/CNPJ search and directory entry | indexable; query strings canonical to `/consultar/` | Directory + latest official observation |
| `/postos/{uuid}/` | Canonical station detail and dated history | indexable only with an official ANP price in the 8 most recent survey weeks and a complete address (D-SEO-02); otherwise `noindex,follow` | Directory + Official |
| `/precos/` | National overview and links to fuel, state and municipality summaries | indexable | Official current revisions |
| `/precos/{fuel-slug}/` | Fuel hub: national summary, table by state, cities with that fuel (SEO-07) | indexable when the current survey has the fuel | Official current revisions |
| `/precos/{uf}/` | State summary and municipalities (lowercase UF, D-SEO-01) | indexable when the state has observations in the current survey; otherwise honest no-data page `noindex,follow` | Official current revisions + Directory geography |
| `/precos/{uf}/{cidade-slug}/` | Municipality summary and cheapest-in-sample list (D-SEO-01) | indexable when the municipality has observations in the current survey (small samples keep the no-ranking state); otherwise honest no-data page `noindex,follow` | Official current revisions + Directory geography |
| `/fiscalizacao/` | Source-reported official events, event-level | first page indexable when the official slice is present; pagination self-canonical `noindex,follow`; record pages always `noindex,follow` (D-SEO-03) | FQ-07/FQ-08 frozen export; FQ-06 still gated |
| `/curiosidades/` | Approved deterministic data questions | indexable | ESP-04 metrics; official-record questions remain blocked by ESP-11 |
| `/dados/` | Bounded CSV downloads and download manifest | indexable | Latest ESP-03/04 snapshot |
| `/metodologia/` | Sources, dates, clocks, methods, rights and corrections | indexable | Source admission register + metrics definitions |
| `/forum/` | Editorial forum hub: latest posts and categories (SEO-12) | indexable while at least one editorial post is published | Forum editorial web projection (B-BR-SEO-08) |
| `/forum/{categoria-slug}/` | Category hub with its editorial posts | indexable while the category has a published editorial post; no page for empty categories | Forum editorial web projection |
| `/forum/{categoria-slug}/{titulo-slug}-{id8}/` | One editorial post | indexable while published and visible; otherwise absent (404) | Forum editorial web projection |
| `/404.html` | Unknown route or unknown station UUID | noindex | none |

`/postos/{uuid}/` is the only canonical station identity route. Slugs, CNPJ,
name changes and address changes never redirect to another station. An unknown
or withdrawn UUID returns a clear no-data/404 page; it never inherits another
operator's prices or official records.

Search URLs (`/consultar/?q=...`, `?uf=...`) are not indexable variants and
carry a canonical link to `/consultar/`. Empty search queries and filter
combinations are not generated as indexable pages.

### Official records (ESP-11)

`/fiscalizacao/` consumes a frozen FQ `quality-official-export-v1` JSONL and
manifest from the same read-only snapshot as the price export. It validates the
payload SHA-256, rows, accepted/input counts and association census before
rendering. The index is paginated at 200 records; record pages are stable,
source-reporting and explicit that a fine is not proof of adulteration, guilt or
definitive legal status, and that absence is not a clean-record certificate.
The page never presents a due date as occurrence or ownership and never invents
a station link. The accepted 2025 real edition contains 3,375 event-level fines;
all are `unmatched` in the acceptance Directory. Linked/ambiguous rendering is
covered by synthetic PostGIS/portal tests; live revalidation of fine links is
OWED. The gated placeholder remains when no official snapshot is present.

### Insights and downloads (ESP-12)

`/curiosidades/` renders a fixed, approved catalog of deterministic questions over
the same frozen revision: coverage count, national gasoline minimum/maximum
comparison (or an explicit sample-change explanation), fuel coverage, municipality
sample sufficiency, source dates and an exclusion census. Each card shows its
formula and methodology link. There is no generative article, forecast,
“worst station” label or official-record claim while ESP-11 is blocked. The page
is print-friendly through the existing `@media print` rules.

`/dados/` links bounded exports from the latest revision: station identity CSV,
latest station prices CSV and national/state/municipality summary CSV, plus a
JSON manifest with hashes, byte/row counts and notes. CSVs are UTF-8, carry source
and period columns, omit private fields and prefix dangerous leading spreadsheet
characters (`=`, `+`, `-`, `@`, tab) with an apostrophe. Downloads do not include
history, user media, CPF, contributor GPS or internal documents, and the page
states the pending rights and precision gates.

### Search implementation (ESP-10)

`/consultar/` keeps a GET form and an HTML state directory. With JavaScript it
loads exactly one immutable regional shard at
`/dados/busca/{release}/{uf}.json` (or `sem-uf`); no national all-stations locator
is generated. Query normalization is accent-insensitive; numeric and
alphanumeric CNPJ punctuation is stripped; results are capped at 50 and input is
truncated at 80 characters. Shards contain only UUID, display/normalized name,
CNPJ, municipality code/name, UF, source status and the station's latest fuel
counts/prices. A missing or release-mismatched shard falls back to the HTML
directory; obsolete requests are aborted. No API, ANP call or database is used.
A synthetic 6,000-station scale test measured a 2,000-station SP shard at
346,084 bytes / 23,639 gzip versus a synthetic national locator at 1,020,084 /
70,029; the regional design is selected. Real-scale measurement remains OWED
while the real edition is blocked by ESP-07.

## Search-engine contract (SEO-01–SEO-08, SEO-12)

Owner decisions D-SEO-01..09 were accepted on 2026-11-14 in the
[SEO growth plan](../planning/SEO_GROWTH_PLAN.md). The generator applies them
as follows; tests in `portalgen` prove each rule on fixtures.

- **B-BR-SEO-01 One indexation decision.** One pure function decides whether a
  page is indexable from its family and data facts. The robots meta and the
  sitemap read that same decision: indexable pages are `index,follow` and in a
  sitemap; gated pages are `noindex,follow` and in no sitemap. A build with the
  global indexable switch off is `noindex,nofollow` everywhere, has no sitemap
  and a disallowing `robots.txt`.
- **B-BR-SEO-02 Station gate.** A station page is indexable only when the
  station has a valid official ANP price (BRL, canonical unit, collected inside
  its survey week) in one of the 8 most recent distinct survey weeks of the
  snapshot **and** a complete address: a street with letters, a municipality
  name and a known UF.
- **B-BR-SEO-03 Readable geo URLs.** States are `/precos/{uf}/` (lowercase UF);
  cities are `/precos/{uf}/{slug}/`. The slug is: Unicode NFD, drop combining
  marks, lowercase, remove apostrophes (`'` and `’`), replace every run of
  characters outside `[a-z0-9]` with `-`, trim `-`. The IBGE name is the slug
  source; slugs are unique per UF over the full IBGE table (tested). A code
  outside the IBGE table, or a slug that collides inside a build, gets the
  `-{ibge}` suffix. No page links to the retired `/precos/uf/` routes.
- **B-BR-SEO-04 Sitemap index.** `sitemap.xml` is a `<sitemapindex>` of
  `sitemaps/portal-{family}-{n}.xml` files (families `hubs`, `precos`,
  `postos`, `forum`), each at most 45,000 URLs and 50 MB. Per-URL `lastmod` is the date
  the page data last changed: the current revision publication date for price
  pages and hubs derived from it, the latest valid observation date for a
  station, the official edition publication for `/fiscalizacao/`, and the build
  date only for pages without a data date. No `priority` or `changefreq`.
- **B-BR-SEO-05 Head contract.** Every page with a configured site URL carries
  an absolute self `<link rel="canonical">` (trailing slash), Open Graph and
  Twitter tags (`og:site_name` `abastevo`, `og:locale` `pt_BR`, landing image
  `/assets/brand/og-image-v1.png`). Titles use the survey end date
  (`dd/mm/aaaa`), never “hoje”; descriptions are 120–160 characters, carry only
  figures shown on the page and are unique per indexable page. ALL-CAPS
  registry names are shown in Portuguese title case in the title/H1 while the
  registry and legal names stay visible.
- **B-BR-SEO-06 Structured data.** Inline JSON-LD describes only visible facts:
  `BreadcrumbList` mirroring the visible breadcrumb (Início › Preços › UF ›
  Cidade › Posto), `CollectionPage` + `ItemList` on hubs, fuel, state and city
  pages, `GasStation` with `PostalAddress` (no ratings or offers) on stations
  and `Dataset` with `DataDownload` files, the license text shown on `/dados/`
  and temporal/spatial coverage on `/dados/`.
- **B-BR-SEO-07 Linking.** City pages link nearby cities (k=8, centroid of the
  city's own station coordinates; omitted when no coordinates exist), other
  cities of the UF, the city's stations, fuel hubs and the related landing
  guides/tools. Station pages link their city with a descriptive anchor, up to
  10 other stations of the city and the fuel hubs of their observations. No
  brand field exists in the export, so “mesma bandeira” is not rendered. Every
  indexable page receives at least 3 inbound body links from distinct pages;
  for stations the bound is `min(3, n)` where `n` is the number of indexable
  stations in its city. The link-graph verifier enforces these invariants plus
  self-canonical sitemap URLs and no link to a missing route (landing routes
  are an explicit allowlist).
- **B-BR-SEO-08 Web forum, editorial posts only (SEO-12, D-SEO-06).**
  - *Input.* The export snapshot carries only `PUBLISHED`, `VISIBLE`
    `EDITORIAL` posts whose source is an Abastevo editorial source (kind
    `ABASTEVO`) with approved rights and publication enabled, read through the
    forum module's own export port inside the export's read-only transaction,
    and only while the forum read switch (`ANPFUEL_FORUM_ENABLED`) is on. News,
    bulletins, user topics and comments never enter it (UGC waits for SEO-13).
    Public fields only: id, category (code, name, description), title,
    introduction, body, first publication and update instants, coarse region
    (UF, IBGE), fuel, station id and the source display name. A post whose
    public text holds a CPF-shaped token stays out of the snapshot and is
    counted, because the portal privacy check would otherwise refuse the whole
    release. The posts are a snapshot file, so any published-post change also
    changes the export content hash.
  - *Routes.* `/forum/`, `/forum/{categoria}/` (slug of the category name;
    no page for an empty category) and `/forum/{categoria}/{titulo}-{id8}/`:
    the title slug (B-BR-SEO-03 rules, at most 80 characters cut at a `-`)
    plus the first 8 hex characters of the post id; a route still colliding in
    a build uses the full id. A title edit moves the route; nothing is indexed
    yet, so no redirect is kept.
  - *Body.* Rendered with the Android rules: blocks split on blank lines; a
    block whose non-blank lines all start with `- ` is a bullet list; a
    single-line block of at most 80 UTF-16 units not ending in `.:;,!?…` is a
    subheading; anything else is a paragraph keeping its line breaks. Text is
    always escaped: no HTML, Markdown or clickable links from the body.
  - *Post page.* H1 title, first publication and update dates (day in
    America/Sao_Paulo), “Equipe abastevo” attribution linking `/sobre/` when
    the landing shares the build, links to the state and city price pages,
    the fuel hub and the station page only when those pages exist in the same
    build, “Outras publicações” (same-category ring, same region, then a ring
    over all posts, so every post receives links from other posts) and the
    category list. Head: `og:type` `article` with published/modified times;
    JSON-LD `DiscussionForumPosting` (headline, an excerpt of the visible body
    as text, datePublished, dateModified, author Organization “Equipe
    abastevo” with `{site}/sobre/`, url, isPartOf the `/forum/` hub) plus the
    breadcrumb Início › Fórum › Categoria › Post. Hubs carry `CollectionPage`
    + `ItemList`.
  - *Indexing and links.* Every forum page is indexable while it exists and is
    listed in the `forum` sitemap family; lastmod is the newest update day of
    the page's posts. State and city price pages show “No fórum” with up to 3
    posts of the same region (city: same municipality first, then statewide
    posts of its UF); `/precos/` lists the 3 latest posts and their
    categories; the portal hubs, header and footer link `/forum/`. Official-
    only builds have no forum.
  - *Withdrawal and latency.* A hidden, removed or rejected post, or one whose
    source loses rights, leaves the next export, so the next release has no
    page (404) and no sitemap entry. The export reconcile job (every 6 h)
    compares a fingerprint of the exported fields with the export state and
    enqueues a build; the build enqueues a portal refresh. Worst-case latency
    is about 6 h plus build and generation time.

## Page fields (public allowlist)

- **Station list:** canonical UUID, display name, CNPJ only while the active
  binding exists, municipality/UF when known, source-reported operation status,
  survey period, fuel, unit, exact milli-BRL amount formatted as BRL, collection
  date, source revision and an explicit sample/coverage note.
- **Station detail:** all list fields plus allowlisted address components,
  `location_quality`, dated observations by fuel/unit/condition, history points,
  missing-period notes, source URL/checksum, revision ID, source publication
  time and Abastevo publication time. Reviewed coordinates are not rendered in
  V1; no map/tile/geocoding is implied.
- **Summaries:** period, geography (national/state/municipality), fuel/unit,
  station sample count, source observation count, min/max/spread in exact
  milli-BRL, derived label, coverage/exclusion note and methodology link.
- **Official records (later):** source-reported type/status/date/as-of/evidence
  reference, current-company vs historical scope and unknown-finality note.
- **Never public:** CPF, private account/owner data, contributor GPS, user
  photos, internal notes, raw source paths, unverified coordinates, private
  documents or a station risk score.

## Editorial rules

- Say “Preços encontrados na pesquisa da ANP de [start] a [end]”. Do not say
  “preço atual”, “mais barato do Brasil” or “o posto mais barato agora”.
- A ranking is “menores preços encontrados nesta amostra e período”, with sample
  count, ties and exclusions. A historical minimum is not a current offer.
- Official facts and Abastevo-derived calculations are separate: “Fonte: ANP”
  plus “Cálculo Abastevo” for derived summaries/rankings. Never present a derived
  mean as an official ANP statistic.
- Missing data is “Sem observação nesta pesquisa”, never zero. An absent week is
  a gap, not a price of R$ 0,00.
- Status is “informado pela fonte” (active/suspended/revoked); it is not a
  quality, trust or safety score. A fine or action is not proof of adulteration
  or definitive guilt.
- Every price page shows the survey period, collection date, source and last
  Abastevo publication. A stopped scheduler cannot leave an undated “live”
  impression.
- Attribution: “Fonte: ANP — Levantamento de Preços de Combustíveis” with a link
  to the official page; “Não afiliado à ANP”; MIT code notice in the footer.
- No guaranteed savings, laboratory claims, risk ranking or generated article.

## Templates and states

1. **List template:** search form (`GET /consultar/`), UF/city selectors, static
   result cards and links. JavaScript is optional; without it the static cards
   and region links remain usable.
2. **Detail template:** identity header, status, latest observations table,
   dated history table, coverage/missing notes, source/methodology links.
3. **Summary template:** sample denominator, min/max/spread, derived label,
   coverage note and a link to the station list.
4. **Empty state:** “Sem preço ANP nesta pesquisa” plus why (not surveyed,
   missing identity, no observation) and a link to the methodology.
5. **Stale state:** explicit source date/period, last successful check and
   “não é preço ao vivo”; affected data remains visible with its age.
6. **Official-record gate:** `/fiscalizacao/` explains that the slice is not yet
   available; it shows no fabricated records and does not block prices.
7. **Error/404:** clear message, link to `/consultar/` and `/metodologia/`; no
   API call.

## Navigation and accessibility

- Portal header: `Consultar`, `Preços`, `Metodologia`, `Fiscalização` (gated),
  `Fórum` (only when the build has editorial posts), plus a link back to the
  Abastevo app/landing.
- Semantic landmarks, one `<h1>`, skip link, table captions/headers, visible
  focus, keyboard operation, 200% zoom/large font, AA contrast, print styles.
- Mobile-first responsive layout at the existing 48rem/64rem breakpoints.
- Essential content, navigation and source dates work with JavaScript disabled.
- No visitor request calls the Abastevo API, ANP API or database. Only static
  same-origin HTML/CSS/JS/JSON/download requests are allowed. CNPJ/search input
  is not sent to analytics or detailed access logs.

## Performance budgets (provisional; ESP-10 final)

- HTML per consultation page: ≤ 100 KiB compressed.
- Portal-specific JS: ≤ 100 KiB compressed.
- One initial regional search shard: ≤ 512 KiB compressed (raised 2026-10-26
  from the synthetic 250 KiB after the first real national build: SP shard
  8,381 stations = 461,000 gzip bytes, dominated by UUID/CNPJ entropy).
- Portal pages: ≤ 100,000 per release (real national build: 52,578).
- History/detail payloads load only on demand; no national full database to
  every visitor.

## Source and publication gates

- Prices/registry: rights `RIGHTS_PENDING` (ESP-01) and the real weekly edition
  is `BLOCKED_OVER_PRECISION` (ESP-07). Real price pages cannot be published
  until a recorded monetary-precision policy and rights decision exist.
- `/fiscalizacao/` stays gated until FQ-06/07 live runtime, safe station links
  and the selected official-record acceptance exist.
- PMQC/curiosidades/downloads are optional later slices (ESP-11/12).
- The prototype uses labelled synthetic data; no real source row is published.

## Rollback

The prototype is isolated under `landing/prototype/` and is not part of
`landing/scripts/build.mjs` or `dist/`. Removing it leaves the existing landing,
guides, legal pages and store state untouched.

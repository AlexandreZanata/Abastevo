# Organic traffic expansion plan

Historical baseline: 2026-11-21, at `main` `e4a67580` plus PR #240 head
`a1498ffd`. Section 8 records owner decisions. The
[ROADMAP](../../../ROADMAP.md) tracks remaining implementation and publication.

This plan extends the
[organic growth and multilingual contract](organic-growth-i18n-plan.md) ("the
contract") and [ADR-026](../../decisions/026-multilingual-public-site.md). The
contract still owns the hreflang, translation and editorial rules. This file
owns three things:
- the measured state on 2026-11-21;
- a new order for the contract tasks that are still open;
- the new tasks SEO-39 to SEO-66. Section 6 lists the remaining work.

When the owner accepts it, move each accepted rule change into the contract,
record the ADR-026 amendments as a new decision, and list the tasks in
[ROADMAP.md](../../../ROADMAP.md). Remove each task from this file when it is
done.

## 1. Goal

Bring the largest possible organic audience to abastevo.com.br, in Portuguese
and in other languages, and turn those visitors into app users and
contributors. "Organic" here covers every unpaid channel within reach: Google
and Bing search, Google Discover, AI answer engines, the Play Store, shared
links and links from other sites.

The contract rules do not change: no cookies or third-party trackers, official
and community data stay separate, no invented numbers, no "posto mais barato
perto de você" claims, and no promise of traffic, rankings or dates.

## 2. State on 2026-11-21

Everything in this section was read from the code at the base revision or
from the live site on 2026-11-21.

### 2.1 Contract tasks already built

| Task | State | Evidence |
|------|-------|----------|
| SEO-20 keyword map | Built. The two forum posts are not retargeted yet. | `primaryQuery` check in `landing/scripts/content/site.mjs`. The live forum sitemap still lists both old posts. |
| SEO-21 head and H1 polish | Done and live. | Live pages carry `index,follow,max-image-preview:large,max-snippet:-1` and the descriptive home, city and station H1s. Article `citation` is in `layouts.mjs`. |
| SEO-22 feeds | Guides feed done and live (`/guias/feed.xml`). No feed for forum editorial posts. | `landing/scripts/build.mjs`, `tests/guides-feed.test.mjs`. |
| SEO-24 navigation hubs | The home states grid is live. No landing page links to `/forum/`. | `renderHomeStates` in `layouts.mjs`. |
| SEO-25 locale registry | Done. `en` and `es` are enabled; the recorded reviewer is the AI review of [ADR-027](../../decisions/027-ai-reviewed-landing-translations.md). | `contracts/i18n/locales.json`. |
| SEO-26 multilingual landing build | The build emits 16 pages per locale with hreflang groups, switcher, draft and stale gates, locale homes and hubs. | `site.mjs`, `tests/i18n-build.test.mjs`. |
| SEO-56 `en` and `es` on the landing | Build scope: the home, about, editorial policy, 5 calculators, 13 editorial guides and 5 guide categories per locale, plus the guide and calculator hubs. | `landing/content/translations/`. |
| SEO-57 Spanish regional vocabulary | Done: neutral Latin American wording, with the regional word beside it on first mention. | `contracts/i18n/glossary/es.json`, the contract's section 6.4. |
| SEO-27 localized calculators | Done. | `parseDecimal` in `landing/src/tools.ts`. |
| SEO-40 one robots.txt source | Built, not served yet. `landing/static/robots.txt` is the source and the web-tier block repeats it byte for byte. | `landing/tests/indexing-and-sitemap.test.mjs`, `business-web/tests/caddy.py`. |
| SEO-23 IndexNow | Built, switched off. The key file ships at the landing root; the submitter has its own timer. | `infra/deploy/agent/indexnow.py`, `tests/test-indexnow.py`. |
| SEO-39 Search Console report | Built. The owner runs it with a token; its tests run in the quick gate. | `scripts/seo/search_report.py`, `scripts/seo/README.md`. |
| SEO-24 forum link | The landing footer links `/forum/`. A card in the home "Explore" grid is not built: it needs approved art. | `renderFooterColumns` in `layouts.mjs`. |
| SEO-22 forum feed | Built: `/forum/feed.xml`, advertised on the forum pages. | `portalgen/forum_feed.go`. |
| SEO-66 llms.txt | Built: the data surface, both feeds and both sitemaps are listed. | `landing/static/llms.txt`, `check.mjs`. |
| SEO-41 labels and titles | Built: units in words, titles of at most 65 characters, fuel synonyms. | `portalgen/seo_head.go`, `titles_units_test.go`. |
| SEO-43 mean and comparison | Built: the sample mean from 5 stations, and a comparison block that shows a movement only for the same station sample. | `portalgen/price_stats.go`. |
| SEO-29 and SEO-42 fuel by area | Built: `/precos/{fuel}/{uf}/` and `/precos/{fuel}/{uf}/{cidade}/`. | `portalgen/fuel_area.go`. |
| SEO-44 dated answers | Built: a reading of the survey and a question block. The lowest-price question never names a cheapest station. | `portalgen/price_answers.go`. |
| SEO-45 ethanol to gasoline ratio | Built: `/precos/etanol-ou-gasolina/` and a ratio line on state and city pages. | `portalgen/parity.go`. |

Translated content: per locale, the home, the about page, the editorial
policy, the 5 calculators and 6 guides. Both dictionaries and every page are
`reviewed`, by the AI review that ADR-027 allows. The build still refuses to
enable a locale whose dictionary is a draft.

Everything in the table from SEO-40 down is built on the wave 1 branch and reaches production only with its release. The rules are B-BR-SEO-09 to B-BR-SEO-12 in [portal routes](../../enterprise/portal-routes.md).

The baseline predates the SEO-28 portal locale implementation. The
[multilingual contract](organic-growth-i18n-plan.md#6-multilingual-architecture)
defines its current behavior. The ROADMAP tracks public deploy and verification.

### 2.2 Live numbers

| Area | Fact |
|------|------|
| Indexable URLs | 6,471. Landing sitemap: 49. Portal: 6 hubs, 432 price pages (27 states, 7 fuel hubs, 398 cities), 5,979 stations, 5 forum URLs. |
| Languages | pt-BR only on that date: `/en/` and `/es/` returned 404. The landing ships both locales with the release of ADR-027. |
| Content | 13 guides of about 1,000 to 1,400 words, 5 calculators, 16 app tutorials, 2 forum editorial posts. None of the 24 wave 2 pieces is written. |
| Price pages | Latest survey only: stations, observations, minimum, maximum and spread per fuel. No mean, no change from the previous week and no history on national, state or city pages. Station pages show up to 5 weeks. |
| Discovery | No IndexNow. Whether the sitemaps are submitted in Search Console and Bing is still unconfirmed (SEO-19). |
| Social preview | One static image (`og-image-v1.png`) for every page. |
| App | Not on Google Play. The landing builds as `prelaunch` and shows "Em breve". [Play readiness](../../store/play-readiness.md) names two blockers: a signed AAB and the Console declarations. Android builds ship as GitHub prereleases. |
| App links | No App Links, no `assetlinks.json`. The app shares only the home URL, from Settings. Forum share sends text with no URL. Station and city screens have no share action. |
| robots.txt | Production serves an inline block from `infra/caddy/staging-web.Caddyfile`. The landing's own `robots.txt`, which names the allowed search agents, never reaches visitors. |
| Speed | One probe: HTML responses in 0.15 to 0.21 s, zstd-compressed, 6 to 10 KB on the wire. This is not a field measurement. |
| Index coverage | Unknown. It cannot be read from outside; Search Console is the only source. |

### 2.3 What limited traffic at the baseline

In order of weight:

1. **Visitors cannot install the app.** Until the Play listing is live, search traffic can only use the web pages. Publishing the app belongs to the store track (D-L08), not to this plan, but every acquisition number depends on it.
2. **Too few pages match what people search in Portuguese.** A city has one page for all fuels. People search one fuel at a time ("preço da gasolina em Cuiabá"), ask for the average ("preço médio"), and ask what changed this week. A competitor that ranked for a probe query on 2026-11-21 (numerando.com.br) has one page per fuel and city, with history and a weekly summary.
3. **The measured public pages were Portuguese.** The `en` and `es` landing build defined 16 pages each, while the portal build still had only pt-BR price pages.
4. **The domain is new and has no links.** It has served the site since 2026-11-14. Nothing announces new pages to search engines, and nothing brings links.
5. **Editorial output is small.** 13 guides, and no publishing cadence yet.
6. **Source rights rest on an owner decision, not on a license.** On 2026-11-21 the owner accepted the risk of attributed reuse of prices, the station registry, enforcement and fines ([source admission](../../enterprise/source-admission.md#f8-commercial-rights-decision-emp-gov-016), EDR-394). No dataset license or ANP statement exists and PMQC stays `RIGHTS_PENDING`. Every new price family must show the ANP attribution, source link and survey period, and is withdrawn if ANP objects.

## 3. Reference: sorrifit.com on 2026-11-21

The contract's section 3 holds the full comparison. This is what changed in
three days and what it adds.

| Fact | 2026-11-18 | 2026-11-21 |
|------|-----------|-----------|
| URLs in the sitemap | 3,088 | 3,396 (283 pages × 12 locales) |
| Exercise pages | 234 | 251, plus 16 muscle hubs and the index |
| Blog | 3 posts, pt-BR only | 8 posts, in all 12 locales with translated slugs |

Page anatomy, measured on sample pages:
- **Entity page** (an exercise): about 875 words. Steps, common mistakes, a visible FAQ, related entities, "similar" links and an app call to action. 59 internal links.
- **Hub**: the count is in the title ("48 opções com passo a passo").
- **Blog post**: about 4,600 words. Sections for the entities it mentions, the calculators to use with it, a FAQ, references and "continue reading". 66 internal links.
- **Head**: 13 reciprocal hreflang links on every page, repeated in the sitemap.
- **Root**: a 302 by `Accept-Language`, at the root only.

Their index coverage could not be checked from outside either.

What this adds to the contract's conclusions:

1. **Their reach comes from one dataset rendered through one template into every locale.** Our equivalent uses reviewed catalogs to render data pages for each locale's permitted families and cities (SEO-28, SEO-59).
2. **They now translate every article into every locale within days.** At that speed the translations are probably machine output (an inference, not a verified fact). Ours go through a second, reviewing AI pass and say so to the reader (ADR-027); a human review replaces it when a reviewer exists.
3. **Articles and entities link both ways.** Our guides link to data pages and the portal links back to guides. Wave 2 keeps the minimum of 10 contextual links.
4. **Every page ends with the app call to action.** Ours can only do that honestly once there is an install path (SEO-64).

The demand difference in the contract still holds. Exercises have the same
demand in every language. Brazilian fuel prices have Portuguese demand, plus
real pockets abroad: Argentine, Uruguayan and Paraguayan drivers (who search
for "nafta"), visitors, and people who compare fuel prices between countries.

## 4. What we already have, and the lever each one feeds

| Asset | State | Lever |
|-------|-------|-------|
| ANP weekly station prices | Every imported week is kept in the database. The portal export carries 5 weeks (12 at most). | Fuel × city and fuel × state pages, weekly change, parity, history (SEO-29, SEO-42 to SEO-46). |
| Sample sums per area and fuel | Computed in `enterpriseexport/analytics`, never rendered. | A mean labelled as Abastevo-derived (SEO-43). |
| ANP weekly summary files | Downloaded by discovery. No parser; `official_summary_prices` is empty. | The official ANP mean (SEO-47). |
| Weekly per-state bulletin | Built for the forum (`forum-official-mean-v1`). | Bulletin pages (SEO-14). |
| ANP historical series | Published by ANP as semester files. No importer here. | Multi-year history pages (SEO-46). |
| Station registry | About 49k stations. Bairro and coordinates are in the snapshot. Brand is parsed by the preparers but has no database column. | Brand hubs (SEO-50). |
| ANP fines | 35,612 records from 2016 to 2025, admitted under CC BY. | Enforcement by state and city (SEO-51). |
| EV charging catalog | 1,298 Brazilian places from Open Charge Map (CC BY 4.0 and CC0). Served by the API, not by the portal. | EV charging pages with clean rights (SEO-48). |
| Inmetro PBE table reader | `tools/station-prep/src/ev_pbe.rs` reads the 2026 PDF. Combustion rows are quarantined. | Vehicle consumption pages (SEO-49). |
| Community prices | In the app and the API only. | Same-day prices on the web: the one thing ANP-only sites cannot copy (SEO-53). |
| Multilingual build | `en` and `es` landing and portal builds use reviewed catalogs. | SEO-56, SEO-28 publication, SEO-59. |
| App in 8 locales | `en`, `pt-BR`, `es`, `de`, `fr`, `ja`, `ru`, `zh-CN`. | The store listing and the light locale set (SEO-37, SEO-59). |
| Capture and card tooling | `docs/store/cards/build_cards.py` (headless Chrome and Pillow), tutorial captures. | Social images and short videos (SEO-35, SEO-63). |
| Open CSV downloads | `/dados/` with Dataset JSON-LD. | Press and research links (SEO-36, SEO-61). |
| Deploy agent and portal refresh job | A smoke test after each deploy. The portal refreshes within about 15 minutes of a new ANP week. | IndexNow (SEO-23). |
| Abastevo Empresas and the Data API (PR #240) | Sales are off. `/empresas/` is the app shell, `noindex`. | Fleet articles now. Public product and API pages when sales open (section 11). |

## 5. Strategy

The levers, ranked by how much reach each should add for the work it costs.
The ranking is a judgment from the facts above, not a forecast.

| # | Lever | Why it ranks here | Tasks |
|---|-------|-------------------|-------|
| 1 | pt-BR price pages that match real queries | Brazil is where the demand is, and the data is already in the snapshot. | Built in wave 1; SEO-46 and SEO-47 remain |
| 2 | Measurement and fast discovery | Without Search Console data every later decision is a guess. IndexNow costs little. | SEO-19, SEO-23 (both owner) |
| 3 | English and Spanish | Both builds support these locales. Public deployment and search coverage still need verification. | SEO-18, SEO-56, SEO-28 publication, SEO-32 |
| 4 | Editorial cadence | Two pieces a week build topical authority and give the data pages incoming links. | SEO-54, SEO-55, SEO-31, SEO-35 |
| 5 | An install path and share loops | They turn traffic into users, and users into links and return visits. | D-L08, SEO-64, SEO-34, SEO-62, SEO-65 |
| 6 | New datasets | EV charging and vehicle consumption are new query families. EV has clean rights. | SEO-48, SEO-49, SEO-52 |
| 7 | Links and press | A new domain needs links. The data is the reason to link. | SEO-36, SEO-60, SEO-61 |
| 8 | More locales | Small demand per locale, but cheap once the dictionary drives the pages. | SEO-59 |
| 9 | Same-day community prices on the web | The strongest differentiator. It needs a privacy review and enough contributions first. | SEO-53 |

## 6. Tasks

Sizes: S under 1 day, M 1 to 3 days, L about 1 week. Each task is one
change with its own tests. Landing checks are in
[landing/README.md](../../../landing/README.md) and backend checks in
[backend testing](../../backend/testing.md). Tasks marked "contract" keep the
specification in the contract's section 10; only the changes are written here.

### 6.1 Foundations

- **SEO-19 Search engine baseline (S, owner, contract).** Open the Search Console domain property. Submit `sitemap.xml` and `sitemap-portal.xml`. Import the property into Bing Webmaster Tools. This is the first task: nothing else can be judged without it.
- **ESP-18 Source rights (owner).** Send the drafted clarification request to ANP and record the answer or the owner's decision (G-D05). The price families below can be built in the meantime; they inherit the `ANPFUEL_ENTERPRISE_PORTAL_INDEXABLE` switch.
- **SEO-23 Wave 1 activation (S, owner).** Wave 1 pages are live. Its VPS steps are not applied yet:
  - apply the web-tier ConfigMap, so the new `robots.txt` is served;
  - raise `ANPFUEL_ENTERPRISE_EXPORT_WINDOW_WEEKS` from 5 to 12 in the worker environment, so the history tables of the fuel by area pages show 12 survey weeks;
  - after the release that ships the key file, install `indexnow.py` and the two `abastevo-indexnow` units, set `ABASTEVO_INDEXNOW_KEY` and `ABASTEVO_INDEXNOW_ENABLED=1`, and enable the timer;
  - read the first run with `journalctl -u abastevo-indexnow.service`. The steps are in the [deploy agent runbook](../../operations/deploy-agent.md#indexnow-submitter);
  - once Search Console has data, run the [search report](../../../scripts/seo/README.md) for the first baseline.
- **SEO-24 Forum card on the home (S, owner design).** The footer links the forum. A card in the home "Explore" grid needs art in the approved style.
- **SEO-20 Forum retarget (S, owner, contract).** Apply the two payloads in the contract's section 7.1 through the admin console.

### 6.2 pt-BR data pages

Every family here goes through the single indexation function in
`portalgen/seo_policy.go` (B-BR-SEO-01) and gets its own sitemap file, scale
test and link-graph check.

- **SEO-46 Price series and history (L, three steps).**
  1. Export a compact weekly series per area and fuel (count, minimum, maximum, sum) for every week in the database. Render 52 weeks on the fuel × area pages.
  2. Add an importer for the ANP semester historical files, with the same append-only rules as the weekly ingestion.
  3. Add `/precos/{fuel}/historico/` and `/precos/{fuel}/{uf}/historico/`, with yearly and monthly tables, and add a history CSV to `/dados/`.
- **SEO-47 ANP official weekly summary (M).** Parse the `resumo_semanal` files that discovery already downloads into `official_summary_prices`, export them, and show "preço médio (ANP)" as the official figure beside the sample numbers. Until this lands, SEO-43 is the only mean on the page.
- **SEO-14 Weekly bulletin (M, contract).** Reuse the per-state means of `forum-official-mean-v1`. Still `noindex` until an editor adds the summary.
- **SEO-48 EV charging pages (L).**
  - Routes: `/eletropostos/`, `/eletropostos/{uf}/` and `/eletropostos/{uf}/{cidade}/`.
  - Source: the admitted Open Charge Map catalog. Each place shows connector types, power, access and the source with its license.
  - Add the catalog to the portal export as its own section, and state the coverage honestly on every page.
  - Gate: a city page needs at least one place with a known connector.
  - Link each place to its fuel station page when the catalog has the link.
- **SEO-49 Vehicle consumption pages (L, needs G-D07).**
  - Extend the PBE reader to combustion and flex rows, and admit the table in the source register.
  - Routes: `/veiculos/{marca}/` and `/veiculos/{marca}/{modelo}/`.
  - Content: the labelled city and highway consumption for each fuel; the model's own ethanol parity ratio, from its two consumption figures; the cost per km at this week's state sample mean. The page says that label figures are laboratory values.
  - One page per model, with versions as rows. No page per version.
- **SEO-50 Brand hubs (M).** Add the brand column, which the preparers already parse, to the registry load and the export. `/postos/bandeira/{marca}/` lists states and cities with priced stations of that brand. Indexable only above a station count that the owner sets from the data.
- **SEO-51 Enforcement by state and city (M).** `/fiscalizacao/{uf}/` and `/fiscalizacao/{uf}/{cidade}/`: counts by year and the records list, from the admitted fines. Record pages stay `noindex` (D-SEO-03).
- **SEO-52 Glossary (M, landing).** `/glossario/{termo}/`, about 60 terms taken from the guides and the [product glossary](../glossary.md). Each has a definition of 100 to 250 words, a source, and links to the guide and data page that use the term. Terms join the keyword map. The short format makes this the cheapest family to localize.
- **SEO-53 Community prices on the web (L, needs G-D08).** Show consensus community observations on station and city pages, in their own block with their own dates, never mixed with ANP numbers. No contributor identity and no location beyond the station. Needs the privacy review, and starts only when contribution volume makes the block non-empty in more than a few cities.

Not built now, with the reason:
- **Route cost pages** ("quanto gasta de São Paulo ao Rio"). Nothing computes road distance on the server or at build time.
- **Neighbourhood pages.** The bairro field exists, but most would be thin. Reconsider with Search Console data for the largest cities.

### 6.3 Editorial program

- **SEO-54 Article template, version 2 (M).**
  - A visible question block built from real questions: Search Console queries, "People also ask", the forum.
  - A "Dados citados neste guia" block that links the data pages the text relies on.
  - A "Calculadoras para usar junto" block.
  - Each article has its own 1200 × 630 image (SEO-35).
  - Validators in `check.mjs`: at least 10 contextual internal links, with at least 3 to data pages, 1 to a tool and 2 to sibling guides; at least 2 primary sources under "Fontes"; a `primaryQuery`.
- **SEO-55 Refresh the 13 guides and the landing SERP copy (M).** Add the question block, the missing synonyms and the missing subtopics to each guide. Rewrite titles query-first with the brand last. The home title is owner-reviewed copy, so the owner approves the new text. Change `dateModified` only when the substance changes.
- **SEO-35 Social preview images (M, contract, moved earlier).** Discover and WhatsApp previews both depend on a large image per page.
- **SEO-31 Editorial wave 2 (L, ongoing, contract).** Two pieces a week. Before writing, record each title's query and volume, with source and date, as the contract requires. Proposed order:

  | Order | Piece | Why now |
  |-------|-------|---------|
  | 1 | Preço da gasolina por estado (pillar; contract piece 1) | Links the 27 state hubs and the new fuel × state pages. |
  | 2 | Preço do gás de cozinha: como é formado e por que varia (new) | GLP is in the survey and has no guide. The audience is wider than drivers. |
  | 3 | Quanto gasta de combustível um motorista de aplicativo (piece 10) | Heavy users, and the cost-per-km tool exists. |
  | 4 | Planejar viagem de carro: combustível e pedágio (piece 16) | The holiday travel season. |
  | 5 | ICMS dos combustíveis no ano corrente: valores por litro (new, updated yearly) | The value changes on a known date. A dated page earns the yearly searches. |
  | 6 | Pode misturar etanol e gasolina no flex? (piece 7) | A short, clear question. |
  | 7 | Como denunciar um posto à ANP (piece 20) | Links `/fiscalizacao/`. |
  | 8 | Preço do etanol: safra e entressafra (piece 3) | Pairs with SEO-45. |

  Then the remaining contract pieces, and three new clusters:
  - **Electric and hybrid:** cost per km against gasoline, and how public charging is billed. Pairs with SEO-48.
  - **Fleet:** how to control fleet fueling, with a free spreadsheet template to download. Leads to Abastevo Empresas when it opens.
  - **Truck drivers:** Arla 32, S10, and diesel cost per km.
- **Review cadence.** Each piece is reviewed at 30 and 90 days (contract section 9). A piece that ranks on page 2 gets links and an update before a new piece is written in the same cluster.

### 6.4 Multilingual rollout

Three tiers. Each locale opens with a reviewed dictionary (ADR-026).
ADR-027 and ADR-028 allow a disclosed AI review on the `en` and `es` landing
and portal.

| Tier | Locales | What ships |
|------|---------|-----------|
| 0 | pt-BR | Everything. |
| 1 | `en`, `es` | The home, about, editorial policy, 5 calculators, the guide set and portal pages for national prices, fuel, state, fuel × state, methodology, data, insights and the cities on the closed allowlist (G-D03). The public glossary and additional guides remain separate content tasks. |
| 2 | `fr`, `de`, `ja`, `ru`, `zh-CN` (the app's other locales) | A light set of about 50 pages: the home, the app page, 5 calculators, the national price page, 7 fuel hubs, 27 state hubs, and 3 pillar guides. No city pages. |

- **SEO-56 Grow en and es on the landing (M).**
  - Translate the app page when it exists (SEO-64). The current editorial guide set and guide categories ship in both locales.
  - Add one Search Console URL-prefix property per locale folder, so each locale has its own report.
  - Replace the AI review of a page with a human review when a fluent reviewer exists.
- **SEO-28 Portal locale publication (S, owner).** Reapply the web-tier ConfigMap after the locale release and verify the public routes and sitemaps. The scope and eligibility rules are in ADR-028 and the contract.
- **SEO-30 Units and SEO-58 Currency context (deferred).** Revisit alternate units or approximate foreign-currency displays in a separately authorized change. An exchange-rate display needs an admitted rate source (G-D04).
- **SEO-32 International set (L, contract; order changed).** Spanish first, because of the summer trips:
  1. Cargar combustible en Brasil: guía para conductores extranjeros.
  2. Cuánto cuesta la nafta en Brasil: precios por estado (a guide that links the localized state hubs, starting with Rio Grande do Sul, Santa Catarina and Paraná).
  3. Cargar combustible en la frontera con Brasil.
  4. Etanol o gasolina, with the calculator.
  5. Pagar el combustible en Brasil (Pix, tarjeta, precio por forma de pago).

  Then the same five in English, then the rest of the contract's section 7.4. Pages published now will mostly serve the end of this season and all of the next one; indexing a new locale takes time, and this plan promises no date.
- **SEO-59 Light locale set (L, needs G-D02).** Open Tier 2 one locale at a time, after Tier 1 is live and its sitemap shows no systematic "crawled, not indexed" problem. Each locale needs a native review of the dictionary, the portal catalog and its 3 guides.
- **SEO-33 Tutorials in en and es (L, contract).** Unchanged, after SEO-56.

### 6.5 Distribution and links

- **SEO-36 Press and data page (M, contract).**
- **SEO-61 Monthly data release (M).** One page a month, built from the series (SEO-46 step 1): where each fuel rose and fell most, per state, with a chart image and the CSV. It is the item journalists can cite, and the source of the press page's archive.
- **SEO-60 Listings and first links (S, owner, with drafts by an agent).** One-time submissions, each a genuine listing:
  - the reuse registry of the federal open data portal, if it accepts applications (confirm the procedure first);
  - the GitHub repository's website link and topics;
  - open-source and open-data directories;
  - one build-story post each on TabNews and dev.to.

  Mirrors of the price data on other platforms wait for G-D05.
- **SEO-62 Share actions on web pages (S).** A share control on data pages and guides: the Web Share API where the browser has it, and a plain WhatsApp link otherwise. No tracker and no third-party script.
- **SEO-63 Short videos (M, owner, optional).** One video of 60 to 90 seconds per calculator and per top guide, recorded with the tutorial capture tooling and published on YouTube. Guides link the video with a local thumbnail, so no third-party request happens before a click.
- **Later, unchanged:** the embeddable widget (after rights) and forum thread indexing (SEO-13).

### 6.6 App and store loop

- **D-L08 Play publication (store track).** Not a task of this plan, and its largest dependency.
- **SEO-64 App page and install path (M, see G-D09).** `/app/`: what the app does, captures, the 8 languages, and the install options that exist today. It switches to the Play badge and `MobileApplication` JSON-LD when the store state is `published`. Every data page and guide links to it.
- **SEO-34 Share links and App Links (M, contract).** Add the share action to station, city and forum screens, and include the URL in the forum share.
- **SEO-37 Store listing localization (S, contract; widened).** Listings in all 8 app locales, written from the reviewed dictionaries, with a keyword pass on the title and short description.
- **SEO-65 Install attribution (S).** Store links carry Play referrer parameters that name the page family. The Play Console then reports installs by source, with no tracker on the site.

### 6.7 Measurement

- **SEO-17 Measurement loop (contract).** Run it monthly with the search report (`scripts/seo/search_report.py`).
- **SEO-38 First-party aggregated stats (M, contract).** Unchanged.

## 7. Order and batches

Batches are sized for one pull request each. Batches in the same wave touch
different directories and can run in separate worktrees.

| Wave | Batch | Tasks | Main paths |
|------|-------|-------|-----------|
| 0 | Owner | SEO-19, ESP-18, SEO-18, SEO-20, SEO-23, SEO-24, decisions in section 8, Play publication | none |
| 1 | C: editorial engine | SEO-54, SEO-55, SEO-35, first pieces of SEO-31 | `landing/scripts/`, `landing/content/guias/` |
| 2 | D: en and es landing | SEO-56, SEO-64, the Spanish half of SEO-32 | `landing/content/translations/` |
| 2 | E: series | SEO-46, SEO-47, SEO-14, SEO-61 | `backend/…/official/`, `enterpriseexport/`, portalgen |
| 3 | F: portal locale publication | SEO-28 public verification | web-tier ConfigMap, portal release |
| 3 | G: new datasets | SEO-48, SEO-49, SEO-52, SEO-50, SEO-51 | `charging/`, `tools/station-prep/`, portalgen, landing |
| 3 | H: loops | SEO-34, SEO-62, SEO-65, SEO-37, SEO-36, SEO-60 | `app/`, landing |
| 4 | I: later | SEO-59, SEO-33, SEO-53, SEO-63, SEO-13, SEO-38 | several |

Dependencies:
- Batches A (discovery) and B (pt-BR price depth) of wave 1 are built.
- Batch D starts after batch C merges, because both edit the landing layouts.
- Batch F follows publication of the locale build.
- SEO-61 needs SEO-46 step 1. SEO-49 needs G-D07. SEO-53 needs G-D08.
- SEO-31 runs every week from wave 1 onward.

## 8. Owner decisions

The accepted decisions supplement ADR-026 and the multilingual contract.

| ID | Decision | Answer |
|----|----------|--------------------|
| G-D01 | Who reviews translations | Accepted. A separate AI review pass is recorded with the model name and disclosed to readers of the `en` and `es` landing and portal (ADR-027 and ADR-028). Other locales require human review. |
| G-D02 | Locale tiers | Accept the three tiers of section 6.4. Tier 2 opens one locale at a time, after Tier 1 is live, without waiting for proof of demand, and is pruned by the contract's 90-day rule. |
| G-D03 | Localized city pages | Accepted. The closed IBGE-code list contains the 27 capitals and 33 selected border twin cities. Localized pages require a Portuguese page in the same snapshot. Sant'Ana do Livramento is eligible when that counterpart exists. |
| G-D04 | Currency and units on en and es pages | Deferred. Use BRL per litre for liquid fuels and the canonical survey units for CNG and LPG. Foreign-currency and per-gallon displays require separate scope approval. |
| G-D05 | Source rights while ANP has not answered | Send the request now. Keep the current public display with attribution, and build the new price families behind the same switch. Hold everything that redistributes the data elsewhere: mirrors, the widget, and Dataset markup beyond `/dados/`. |
| G-D06 | Means on public pages | Show the sample mean labelled as Abastevo-derived (SEO-43) now, and the official ANP mean when SEO-47 lands. |
| G-D07 | Vehicle consumption source | Admit the Inmetro PBE table for combustion and flex vehicles. |
| G-D08 | Community prices on the web | Yes, after the privacy review, and only as a separate, dated block. |
| G-D09 | Install path before Play | Publish `/app/` now with the honest state. Offer the signed prerelease APK there as a test build, with its checksum, until the Play listing is live. |
| G-D10 | Fuel × city threshold | 5 priced stations for the fuel in the current survey: the municipal minimum the portal already applies. No new threshold. |

The owner accepted G-D05, G-D06 and G-D10 for wave 1. G-D02 and G-D07 through
G-D09 remain open.

Open from the contract: the named editor and bio (I18N-D07).

## 9. Measurement

Read monthly from the SEO-39 report, per URL family and locale:
- pages indexed against pages submitted;
- impressions, clicks, CTR and average position;
- the number of distinct non-brand queries that bring clicks;
- referring domains, from the Search Console and Bing link reports;
- installs by source, from the Play Console, once the listing is live.

Rules, added to the contract's section 9:
- **No targets before a baseline.** The owner sets targets from the first two monthly reports.
- **A new family is judged at 90 days.** If most of its URLs are still not indexed, improve its content and links once. If that changes nothing, set its weakest members to `noindex`.
- **Thresholds are data.** The municipal minimum, the station window and the brand-hub threshold change only with a report that shows why.
- **Titles and descriptions are rewritten only for pages with impressions and a CTR below their family's median.**

## 10. Risks

| Risk | Mitigation |
|------|------------|
| Rights for prices and the registry stay undefined while the page count grows | G-D05. All price families share one switch. Families with clean rights (EV, fines, glossary, guides, tools) do not depend on it. |
| Fuel × city pages read as thin or duplicate | The station gate, the series, the week summary and the nearby-city links make each page distinct. Pages under the gate are not emitted. The 90-day rule prunes the rest. |
| A derived mean is read as an official number | A fixed label with the station count, a methodology link, and the official figure beside it once SEO-47 lands. |
| Translation errors in prices or legal facts | Numbers render from data, never from translated text. Review packets check number equality. Legal pages stay `noindex` with Portuguese prevailing. |
| Crawl budget on a new domain | Stations and fuel × city stay pt-BR. Tier 2 is about 50 pages per locale. IndexNow submits changed URLs only. |
| Traffic with nowhere to convert | SEO-64 now; the Play publication is tracked as the dependency it is. |
| Editorial capacity | A fixed cadence of 2 pieces a week, with the order in section 6.3. No news chasing. |
| A change to the label or title rules hurts existing rankings | SEO-41 changes copy, never URLs. Compare Search Console CTR and position per family before and after. |

## 11. Not doing

These add to the contract's section 12:
- Translating station pages, fuel × city pages or the forum.
- Machine-translated pages published without review, in any tier.
- The word "hoje" on pages that show weekly data.
- Route, neighbourhood or "near me" pages without data behind them.
- Public Abastevo Empresas pricing or Data API pages while sales are off. Until then, fleet content is editorial only.
- Mirroring the price data on other platforms before the rights decision.
- Paid links, link exchanges, or automated posting to communities.

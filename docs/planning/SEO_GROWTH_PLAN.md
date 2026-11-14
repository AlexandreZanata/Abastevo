# SEO growth and Google indexing plan

Status: DECISIONS_ACCEPTED 2026-11-14 / IMPLEMENTATION_IN_PROGRESS.
Date: 2026-11-13. Task namespace: SEO-00–SEO-17.
Registry: ROADMAP registration happens at SEO-00 (not done by this plan).

## Authority and actual baseline

The maintainer asked for a professional plan to improve SEO and Google indexing
of the public website: articles, a web-visible forum, dense internal linking and
many static pages, using `https://sorrifit.com/` as the SEO reference. Scope is
local documentation. Saving this plan implements nothing, changes no DNS,
Search Console, VPS, Caddy or staging state, publishes no content and accepts no
gate. Every SEO task below is PLANNED.

Planning is isolated in the worktree `.worktrees/seo-growth-plan` (branch
`codex/seo-growth-plan`) based on `origin/dev` `fac002e7`. The primary checkout
(detached `e22824db`, unrelated Android/Evidence edits) was excluded. SEO-00
reconciles the then-current dev/main before implementation.

Measured on 2026-11-13 (read-only):

- **Production domain is not live.** `abastevo.com.br` does not resolve. The
  production `infra/caddy/Caddyfile` proxies only `/v1/*` and `/health/*`.
  Google currently indexes nothing of ours; every SEO result depends on the
  production web release, which this plan does not authorize.
- **Staging is correctly hidden.** `teste.abastevo.com.br` sends
  `X-Robots-Tag: noindex, nofollow`, robots `Disallow: /` and 404 for
  `/sitemap.xml` ([staging web Caddyfile](../../infra/caddy/staging-web.Caddyfile)).
- **Two generators share one origin.**
  - `landing/` (Node build, hand-authored HTML): 26 pages plus 404 — home, legal,
    `/contribuidores/`, `/guias/` (3 guides), `/como-usar/` (17 tutorials).
    Good head metadata (canonical, OG/Twitter, JSON-LD BreadcrumbList/
    TechArticle/Organization/WebSite). Page list is hard-coded in
    `landing/scripts/build.mjs` and `landing/static/sitemap.xml`, which does not
    scale to many pages.
  - `backend/internal/modules/enterpriseexport/portalgen` (Go): the staging
    release `published_at 2026-11-14T00:02:54Z` holds **52,977 pages**:
    49,154 `/postos/{uuid}/`, 3,375 `/fiscalizacao/registro/{id}/`,
    398 `/precos/uf/{UF}/municipio/{ibge}/`, 27 `/precos/uf/{UF}/`, plus
    `/consultar/`, `/precos/`, `/curiosidades/`, `/dados/`, `/metodologia/` and
    fiscalização pagination.
- **Portal SEO gaps** (sampled Cuiabá city page and one station page):
  - no `rel=canonical`, no Open Graph/Twitter, no JSON-LD;
  - generic titles/descriptions (`Preços em Cuiabá | abastevo`; station titles
    are the raw ALL-CAPS registry name without city);
  - station breadcrumb is `Consultar › Posto`, not `Brasil › UF › Cidade › Posto`;
  - a station page links to its city once and to no sibling stations; a city
    page links to its stations and state but to no neighbouring cities,
    guides, tools or forum content;
  - one flat sitemap with build-time `lastmod` for every URL and a hard
    50,000-route failure that the current ~49.6k sitemap routes nearly reach;
  - the landing and the portal each own a `robots.txt`/`sitemap.xml`; production
    needs one root robots file and a sitemap index.
- **Forum** (FOR plan): public anonymous read API exists (`/v1/forum/categories`,
  `/v1/forum/posts`, `/v1/forum/posts/{id}`); posts carry category, kind and
  nullable coarse `region{uf,municipality_ibge}`, `fuel_product`, `station_id`.
  Body is plain text ≤4,000 chars. Editorial posts are published on staging;
  comments are off. There is **no web rendering** of the forum today.

Reuse, do not recreate: [ESP plan](ENTERPRISE_DATA_STATIC_PORTAL_PLAN.md)
(portal data/routes/budgets), [P25 landing](STATIC_LANDING_PLAN.md) and
[landing redesign](LANDING_REDESIGN_EXECUTION_PLAN.md), [Forum plan](FORUM_ABASTEVO_PLAN.md).
This overlay does not reorder ADR-019 app-first work, certify G09 or G24, or
authorize production web hosting.

## Reference study: sorrifit.com

Inspected 2026-11-13 (public HTML/sitemap, read-only). SvelteKit prerendered
pages; one urlset sitemap with 3,088 URLs = ~257 Portuguese URLs × 12 locales.

| sorrifit pattern | Evidence | Abastevo adaptation |
| --- | --- | --- |
| Entity catalog → one page per entity | 250 `/exercicios/{slug}` pages | Station, city, state and fuel pages already exist (53k); add quality gates and richer templates |
| Fixed page template per entity | H1, Como fazer, Erros comuns, FAQ, Músculos, Equipamento, **Exercícios parecidos** | Fixed sections: price summary, history, stations, **Cidades próximas / Outros postos**, perguntas, related guides/tools |
| Hub pages per group | `/exercicios/peito` CollectionPage + ItemList, title "47 opções…" | State hubs, fuel hubs, (later) brand hubs with counts in titles |
| Pillar blog posts link down | `/blog/treino-de-peito` ~5.7k words, 19 entity links, calculator links, Article + FAQ JSON-LD, dated | Pillar guides linking to 10+ data pages, tools and cluster guides |
| Free tools as link magnets | 4 calculators, WebApplication + FAQ | Fuel calculators (etanol/gasolina, viagem, consumo, custo/km) |
| Sitewide hub links | Nav + footer link exercises, blog, every tool | Footer with 27 states, capitals, tools, guides, forum |
| Conversion page kept out | robots `Disallow: /baixar` | Keep `/github`, `/codigo` redirects out; disallow search shards |
| Multilingual + hreflang | 12 locales, `x-default` | **Not copied** (see D-SEO-07): demand for Brazilian fuel prices is pt-BR |

Weaknesses not to copy: identical `lastmod` for every URL (build date), only
three blog posts linked only from nav/footer, tools never linked from body copy,
FAQPage/HowTo markup that no longer yields Google rich results (FAQ rich results
are limited to authoritative government/health sites since 2023 and HowTo rich
results were retired), `priority`/`changefreq` that Google ignores.

## Principles

1. **Honesty over clicks.** Existing rules stay: source and date visible,
   official ANP and community data separate, no "cheapest near you" promises,
   no invented prices, coverage, traffic or ranking. Titles use the survey date,
   never "hoje" for weekly ANP data.
2. **Index only pages a person would want to land on.** Every indexable page has
   unique, data-backed or editorial substance; the rest is `noindex,follow` and
   out of the sitemap. This is the defence against Google's scaled-content and
   thin-content policies with 50k+ generated pages.
3. **One canonical URL per piece of content**, consistent trailing slash, one
   host (`https://abastevo.com.br`), 301 for everything else.
4. **Links are part of the product.** Every page answers "where next?" with
   descriptive anchors; link structure is verified by tests, not by hope.
5. **Static first.** Extend the existing Node landing build and Go portalgen;
   no framework, server rendering, CMS, tracker or new dependency without the
   recorded need/license/security review (AGENTS rule 11).
6. **Privacy is unchanged.** No cookies, no third-party analytics, no PII or
   precise contributor location in pages; UGC indexing has its own gate.

## Target information architecture

```text
/                                   landing (Organization, WebSite, app)
├── /precos/                        Brasil hub: fuels, states, methodology
│   ├── /precos/{combustivel}/      fuel hub (gasolina, etanol, diesel-s10, …)   [SEO-07]
│   ├── /precos/{uf}/               27 state hubs                                 [D-SEO-01]
│   │   └── /precos/{uf}/{cidade}/  surveyed cities (398 today)                  [D-SEO-01]
├── /postos/{id}/                   station pages (indexable only past the gate) [D-SEO-02]
├── /consultar/                     search UI (JS shards disallowed in robots)
├── /fiscalizacao/                  aggregate pages; records gated              [D-SEO-03]
├── /dados/                         open data downloads (Dataset JSON-LD)
├── /metodologia/, /curiosidades/   methodology, approved data questions
├── /guias/                         articles hub + /guias/{categoria}/ + /guias/{slug}/ [D-SEO-04]
├── /boletim/{aaaa-mm-dd}/          weekly ANP bulletin (human summary required)  [SEO-14]
├── /ferramentas/                   calculators hub + one page per tool          [SEO-10]
├── /forum/                         web forum: categories and posts             [SEO-12]
├── /como-usar/                     app tutorials (existing)
├── /sobre/, /politica-editorial/   E-E-A-T pages                                [SEO-09]
└── legal pages (existing)
```

Click depth target from `/`: hubs ≤2, cities ≤3, stations ≤4, articles ≤3.

## Page templates

Title ≤ ~60 visible characters where possible, description 120–160, both
unique per indexable URL (verified). Proposed pt-BR formulas:

| Family | Title | H1 / required sections |
| --- | --- | --- |
| City | `Preço da gasolina, etanol e diesel em Cuiabá (MT) – ANP {dd/mm}` | Resumo por combustível (min/médio/máx da amostra, data), menores preços da amostra, postos, histórico curto, cidades próximas, outras cidades de MT, guias/ferramentas relacionadas, perguntas (respostas datadas), fonte |
| State | `Preço dos combustíveis em Mato Grosso por cidade – ANP {dd/mm}` | Resumo estadual, ranking de cidades por combustível, lista de cidades, estados vizinhos, guia "preço por estado" |
| Fuel hub | `Preço da gasolina comum no Brasil por estado – ANP {dd/mm}` | Nacional, tabela por UF, cidades extremas da amostra, guia de escolha do combustível, calculadora |
| Station | `{Nome em caixa normal} – preços em Cuiabá (MT)` | Endereço/bandeira, preços datados, histórico, outros postos em Cuiabá, mesma bandeira na cidade, link à cidade, registros oficiais quando aplicável |
| Guide | Question-shaped headline matching intent | Answer first, sources, dates, author/editorial line, links down to data and tools, "leia também" |
| Tool | `Calculadora etanol ou gasolina` | Working form (progressive enhancement), formula explained without JS, worked example, related tools, data links |
| Forum post | Post title | Body, category, region/fuel/station links, related posts, editorial attribution |

Station display names are normalized to Portuguese title case for titles/H1
(prepositions lowercase) while the registry/legal name stays visible; reuse
`portalgen/station_names.go` normalization if it already applies, otherwise
extend it with tests.

Structured data (JSON-LD, describing only what is visible on the page):
BreadcrumbList on every page; CollectionPage + ItemList on hubs; `GasStation`
(name, PostalAddress, url; no ratings/offers until a separate decision) on
stations; `Dataset` + `DataDownload` + license + temporal/spatial coverage on
`/dados/` (Google Dataset Search fit); `Article` with author/publisher and
`datePublished`/`dateModified` on guides and bulletins; `WebApplication` on
tools; `DiscussionForumPosting` on forum posts. FAQ blocks may stay for users,
with no rich-result expectation.

## Internal linking model

Testable invariants (implemented by the link-graph verifier in SEO-06, run on
the combined landing + portal + content output before every release):

1. Every sitemap URL returns 200, is `index`, self-canonical and is reachable
   from `/` within the depth targets above.
2. Every indexable page has at least 3 inbound **contextual** links (body
   links, excluding header/footer/nav), and no indexable page is an orphan.
3. No internal link points to a 404, a redirect or a `noindex` page, except
   documented navigation exceptions (for example legal pages).
4. Breadcrumbs mirror the URL hierarchy and match BreadcrumbList JSON-LD.
5. Anchors are descriptive (`preço da gasolina em Cuiabá`), never bare
   "clique aqui"; the verifier flags generic anchors.

Modules per family:

- **City:** cidades próximas (k nearest, centroid derived from the city's own
  station coordinates — no new data source), outras cidades da UF, postos na
  cidade, combustíveis da cidade, guias (etanol ou gasolina when both fuels are
  sampled), calculadora com preço médio pré-preenchido, posts do fórum with the
  same `municipality_ibge`/UF.
- **Station:** breadcrumb via UF and city, outros postos na cidade (bounded),
  mesma bandeira na cidade, cidade e combustível hubs.
- **Guides:** each pillar links to ≥10 data pages and the relevant tools; each
  cluster links to its pillar and 2–3 siblings; "leia também" by shared tags.
- **Forum post:** region/station/fuel chips link to the matching data pages;
  data pages link back to recent posts for the same region.
- **Global:** footer with 27 states, capitals, tools, guides, forum; landing
  home gains a states grid, popular guides and tools.

## Indexation policy

| Family | Indexable when | Otherwise |
| --- | --- | --- |
| Landing, guides, tools, `/sobre/`, hubs | Always (published) | — |
| State, fuel hub | Current survey has observations | Page kept, `noindex,follow` |
| City | ANP sample in the configured recent window | Honest no-data page, `noindex,follow` |
| Station | Passes D-SEO-02 gate (recent official price + complete address) | `noindex,follow`, not in sitemap |
| Fiscalização record | Per D-SEO-03 (recommended: never at first) | `noindex,follow` |
| Fiscalização pagination | Self-canonical, `noindex,follow` | — |
| Forum post | Published, visible, editorial (UGC only after SEO-13) | Hidden/removed: 404/410 and dropped from sitemap |
| Weekly bulletin | Human summary present | `noindex,follow` until reviewed |
| Search results, query-string variants, JSON shards | Never | robots `Disallow` for shard paths; canonical to clean URL |

Sitemaps: `/sitemap.xml` becomes a sitemap index pointing to per-family files
(`landing`, `guias`, `ferramentas`, `forum`, `precos`, `postos-N`, …), each
≤50,000 URLs/50 MB. `lastmod` per URL is the date its content last changed
(survey date for price pages, last observation for stations, `dateModified`
for content), never the build time. No `priority`/`changefreq`. Staging keeps
`noindex` headers; production must not ship them (guard test on the production
Caddyfile and on built HTML in production mode).

## Content program

Authoring: guides stay human-written and source-cited, reviewed in PRs, with
`datePublished`/`dateModified` and the editorial attribution chosen in
D-SEO-09. AI assistance is allowed only with human fact-checking against cited
sources; no mass-generated articles.

Wave 1 backlog (proposals, pt-BR, grouped by pillar; existing guides marked):

- **Como o preço se forma:** como é formado o preço da gasolina; ICMS por litro
  e o seu bolso; por que cidades vizinhas têm preços diferentes; diesel S10 x
  S500; preço da gasolina por estado (data-backed, refreshed weekly, links to 27
  UFs); histórico do preço da gasolina (série ANP).
- **Escolher o combustível:** etanol ou gasolina (existing, add calculator);
  gasolina comum ou aditivada; GNV compensa?; quanto custa carregar um carro
  elétrico; octanagem e gasolina premium.
- **Economizar:** formas comprovadas de economizar combustível; como calcular o
  consumo médio (km/l); quanto custa uma viagem de carro; calibragem e consumo;
  ar-condicionado ou janela aberta.
- **Qualidade e direitos:** como identificar combustível adulterado; bomba baixa
  e a medida-padrão de 20 litros; como denunciar um posto à ANP; seus direitos no
  posto (preço exibido, nota fiscal, Procon); o que significam as multas da ANP
  (links to `/fiscalizacao/`).
- **Dados e transparência:** pesquisa semanal da ANP (existing); como ler preços
  (existing); dados abertos de combustíveis e como usá-los (links `/dados/`);
  metodologia do abastevo.
- **Comunidade:** como contribuir com preços no app; regras da comunidade
  (summary linking `/termos/`).

Tools (SEO-10): etanol ou gasolina; custo de viagem; consumo médio; custo por
km (combustão x elétrico); quanto custa encher o tanque. Each works without
API calls (CSP `connect-src 'self'` holds), explains the formula without JS
and links to state/city price pages; query-prefilled variants canonicalize to
the clean tool URL.

Forum on the web (SEO-12): render published editorial posts as static pages
under `/forum/`, generated from the same public read projection the API serves,
reusing the Android text-formatting rules (subheading/bullet heuristics) in Go
for parity. A guide and a forum post never duplicate each other: an editorial
post that summarizes a guide links to the guide and carries its own original
text. UGC indexing waits for SEO-13.

## Owner decisions

The maintainer accepted every recommendation on 2026-11-14 and asked for
implementation and deployment. D-SEO-09 resolves to the "Equipe abastevo"
Organization (no personal names published).

| ID | Decision | Recommendation (accepted) |
| --- | --- | --- |
| D-SEO-01 | Geo URL scheme | Switch to `/precos/{uf}/{cidade-slug}/` before first production indexing (no ranking to lose yet); 301 from current `/precos/uf/{UF}/municipio/{ibge}/` |
| D-SEO-02 | Station indexing gate | Indexable only with an official ANP price in the recent survey window (propose 8 weeks, tune with Search Console) and a complete address |
| D-SEO-03 | Fiscalização record pages | `noindex` at launch; aggregate pages indexable; revisit with legal review |
| D-SEO-04 | Articles section | Reuse `/guias/` as the single articles hub with categories; add `/boletim/` for dated weekly data |
| D-SEO-05 | Authoring format | In-house Markdown subset (headings, lists, links, tables, images) with front matter in the landing build; no new dependency |
| D-SEO-06 | Forum web scope | `/forum/` with editorial posts first; UGC behind SEO-13 |
| D-SEO-07 | Languages | pt-BR only; no hreflang until a real non-Portuguese audience exists |
| D-SEO-08 | Measurement | Google Search Console + Bing Webmaster Tools only; no cookie or third-party analytics |
| D-SEO-09 | Author attribution | Named human editor(s) with bio page if the owner accepts publicity; otherwise "Equipe abastevo" Organization with editorial policy |

## Tasks

Each task follows DELIVERY_WORKFLOW: B-BR/BUC before behavior, RED → GREEN,
scoped tests, `git diff --check`, secret-surface review, one atomic commit per
coherent change. Generated-output tests run on fixtures; nothing here deploys.

### SEO-00 — Reconcile baseline and record decisions

Areas: this plan, ROADMAP, PROGRESS. Re-measure portal page counts and landing
pages on current dev/main; record D-SEO-01..09 answers; register SEO tasks in
ROADMAP. Exit: decisions recorded. Rollback: none (docs).

### SEO-01 — Portal head contract

Dependency: SEO-00. Areas: `portalgen/templates.go`, `generator.go`,
`station_names.go`. Add canonical, OG/Twitter, robots, title/description
formulas and title-case station names. Tests: unique non-empty title/description/
canonical per indexable fixture page, length bounds, escaping, preview builds
stay `noindex`. Rollback: previous templates.

### SEO-02 — Readable geo URLs and redirects

Dependency: D-SEO-01. Areas: portalgen routes, staging/production Caddy
redirect map. Slugify with accents/apostrophes/hyphens (`ALTA FLORESTA
D'OESTE` → `alta-floresta-doeste`), prove uniqueness per UF over the IBGE asset,
301 old routes. Tests: full-IBGE collision test, redirect table, no internal
link to old routes. Rollback: keep code routes.

### SEO-03 — Indexation policy engine

Dependency: SEO-01, D-SEO-02/03. Areas: portalgen page base. One function
decides `Indexable` per page from data; robots meta and sitemap inclusion read
the same decision. Tests: gate boundaries, stale/no-data cases, no `noindex`
URL in any sitemap. Rollback: previous all-or-nothing flag.

### SEO-04 — Sitemap index and unified robots

Dependency: SEO-03. Areas: portalgen sitemap writer, landing build, Caddy
routes for `/robots.txt` and `/sitemap*.xml` in production config (staging
unchanged). Per-family files, per-URL `lastmod`, size/count limits, landing
sitemap generated from the content manifest. Tests: XML schema, limits, every
`loc` resolves in the combined output, production config has no
`X-Robots-Tag: noindex`. Rollback: single sitemap.

### SEO-05 — Structured data per family

Dependency: SEO-01. Areas: portalgen templates, landing layouts. Types listed
above. Tests: JSON parses, required properties, values match visible text,
no ratings/offers emitted. Rollback: remove JSON-LD blocks.

### SEO-06 — Linking modules and link-graph verifier

Dependency: SEO-02/03. Areas: portalgen (nearby/sibling modules), new verifier
script run on combined output. Implements the invariants above. Tests: depth,
contextual inbound ≥3, orphans, broken/redirect/noindex targets, generic
anchors, deterministic ordering. Rollback: modules off; verifier stays.

### SEO-07 — Fuel hub pages

Dependency: SEO-06. Areas: portalgen. Only fuels/units present in the ANP
survey; no cross-unit comparison (GNV m³, GLP 13 kg). Tests: unit labels, empty
fuel weeks, links to state/city pages. Rollback: remove family.

### SEO-08 — Content pipeline in the landing build

Dependency: D-SEO-04/05. Areas: `landing/scripts/build.mjs`, new
`landing/content/`, layouts. Replace the hard-coded page list with a validated
content manifest; front matter (title, description, dates, category, tags,
related data routes); Markdown subset renderer with escaping tests; hub,
category and "leia também" generation; existing pages keep their URLs.
Tests: renderer XSS/escaping, required front matter, unique slugs, link targets
resolve against the portal route manifest, existing landing tests pass.
Rollback: previous hard-coded build.

### SEO-09 — E-E-A-T pages

Dependency: D-SEO-09, SEO-08. `/sobre/`, `/politica-editorial/` (sources,
corrections, AI-assistance and independence statement, no ANP affiliation),
author/team page; Organization JSON-LD links them. Rollback: remove pages.

### SEO-10 — Tools

Dependency: SEO-08. `/ferramentas/` hub and the five calculators in plain
TypeScript with no-JS explanation. Tests: formula unit tests (decimal comma,
zero/negative/absurd input), progressive enhancement, CSP compliance,
accessibility. Rollback: remove tools.

### SEO-11 — Editorial wave 1

Dependency: SEO-08/09. Content-only PRs from the backlog, each with cited
sources, data links and review. Not a code task; no publication before the
production release.

### SEO-12 — Forum editorial pages on the web

Dependency: D-SEO-06, SEO-04/06. Areas: portalgen (or a sibling generator in
the same export job) reading the public forum projection. Category hubs, post
pages with stable `{slug}-{short-id}` routes, region/fuel/station links,
`DiscussionForumPosting`. Hidden/withdrawn posts disappear on the next release
(404) and leave the sitemap. Tests: visibility states, text formatting parity
with Android rules, escaping, no author PII, slug stability on title edits
(redirect). Rollback: remove `/forum/` output; API unaffected.

### SEO-13 — UGC indexing gate (later)

Dependency: Forum comments/community posts enabled, moderation and privacy
review. Index only moderated threads above a substance threshold; user links
`rel="ugc nofollow"`; pseudonyms only. Rollback: `noindex` all UGC.

### SEO-14 — Weekly bulletin

Dependency: SEO-04/08. `/boletim/{date}/` generated from the accepted survey
with tables and charts; `noindex` until an editor adds the human summary.
Rollback: remove family.

### SEO-15 — Landing and navigation integration

Dependency: SEO-06/08/10. Home states grid, popular guides, tools, forum entry;
shared footer hubs across landing and portal; `llms.txt` lists hubs. Tests:
existing landing a11y/no-JS/budget tests. Rollback: previous home.

### SEO-16 — Production launch onboarding (operational)

Dependency: authorized production web release, DNS and TLS for
`abastevo.com.br` (+ `www` 301). Verify the domain in Google Search Console
(DNS TXT) and Bing Webmaster Tools, submit the sitemap index, inspect sample
URLs per family, optionally enable IndexNow for Bing. Needs explicit owner
authorization at execution; nothing here is pre-approved.

### SEO-17 — Measurement loop

Dependency: SEO-16. Monthly Search Console review by URL family (indexed vs
submitted, impressions, CTR, top queries); record baselines at launch instead of
invented targets; tune D-SEO-02 windows, titles and pruning/merging of pages
that stay unindexed or get no impressions.

## Risks

- **Thin/scaled content** with ~50k station pages on a new domain → gates
  (SEO-03), contextual modules, sitemap prioritization of hubs; expect gradual
  indexing.
- **Stale or misread prices** → survey date in title/body, no "hoje", source
  line on every page, honest no-data pages.
- **Legal/reputation** on fiscalização records and brand names → D-SEO-03,
  nominative brand use only, existing caveats retained.
- **Duplicate content** between guides, bulletins and forum → one canonical,
  original text per page, verifier checks canonical consistency.
- **Cross-generator breakage** (landing links to portal routes that disappear)
  → stable city/station URLs with no-data states and release-time link check.
- **Staging leakage** → staging keeps `noindex`; production guard tests ensure
  the header never ships there.
- **No production domain yet** → all work can be built and tested locally, but
  search impact starts only after SEO-16.

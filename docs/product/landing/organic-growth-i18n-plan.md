# Organic growth, articles and multilingual indexing plan

Owner decisions were accepted on 2026-11-18 as
[ADR-026](../../decisions/026-multilingual-public-site.md). This file owns the
multilingual rules, the content program and the task specifications. Open
work is tracked in [ROADMAP.md](../../../ROADMAP.md) under Web/landing/SEO. Task
IDs continue the earlier SEO work, from SEO-18 to SEO-38. When a task is done,
update the rule it touches here and remove its ROADMAP item.

## 1. Goal and scope

The owner wants more organic visitors and more users for abastevo.com.br, and
better indexing in other languages through articles and i18n. The model is
`https://sorrifit.com/`.

This plan covers:
- the public landing (`landing/`, Node build);
- the static portal (`backend/internal/modules/enterpriseexport/portalgen`, Go);
- the editorial program;
- the growth loops between the web, the Android app and the community;
- how results are measured.

It does not change the rules on privacy, sources or honesty: no cookies or
third-party trackers, official and community data stay separate, no "posto
mais barato perto de você" claims, and no invented numbers. It also does not
authorize production certification, which stays with the release chain in the
ROADMAP.

**What changed.** The earlier SEO plan decided "pt-BR only; no hreflang until a
real non-Portuguese audience exists" (D-SEO-07). ADR-026 supersedes that rule.
Until the locale tasks ship, the site is still pt-BR only.

## 2. Baseline (measured 2026-11-18, read-only)

| Area | Fact |
|------|------|
| Host | `https://abastevo.com.br` is behind Cloudflare. www → apex 301, http → https 308, no slash → slash 308, real 404, zstd compression, HSTS and strict CSP are all in place. |
| Verification | A Google site-verification TXT record exists on the apex. Whether the sitemaps are submitted, and the Bing import, are unconfirmed (SEO-16). |
| robots.txt | Allows `*`. Disallows `/admin/`, `/empresas/` and `/dados/busca/`, and blocks GPTBot and ClaudeBot. Lists `sitemap.xml` and `sitemap-portal.xml`. |
| Landing sitemap | 49 URLs: home, 17 `/como-usar/`, 19 `/guias/` (13 guides, 5 category hubs, index), 6 `/ferramentas/`, `/sobre/`, `/politica-editorial/` and legal pages. |
| Portal sitemap index | 6 hubs, 432 price pages (national, fuel, 27 states, cities), 5,979 indexable stations out of about 49k (gate D-SEO-02), and 5 forum URLs. That is about 6,470 indexable URLs in total. |
| Head quality | Every page has a canonical, Open Graph tags and JSON-LD: Article, BreadcrumbList, CollectionPage+ItemList, GasStation and DiscussionForumPosting. Titles are data-specific, for example "Preço da gasolina, etanol e diesel em Cruzeiro do Sul (AC) – pesquisa ANP 14/11/2026". |
| Languages | Everything is pt-BR (`<html lang="pt-BR">`, `og:locale pt_BR`, JSON-LD `inLanguage` pt-BR). `/en/` returns 404 and no page has hreflang. |
| App | The Android app already ships 8 locales: en (default), pt-BR, es, de, fr, ja, ru and zh-CN. The Play listing is pt-BR plus en-US. The app has no web share links and no Android App Links (`.well-known/` holds only `security.txt`). |
| Visibility | A public web search for "abastevo" returned nothing from the domain. The domain has served the site only since 2026-11-14. The real baseline must come from Search Console. |

Problems found:
- **Keyword cannibalization.** The forum editorial posts "Etanol ou gasolina? Como fazer a conta…" and "Como funciona a pesquisa semanal da ANP…" target the same query as `/guias/etanol-ou-gasolina/` and `/guias/pesquisa-anp/`.
- **Weak H1s.** The home H1 is a slogan ("Mais informação para quem vive na estrada"). City H1s are short ("Preços em Cruzeiro do Sul") and station H1s carry only the name.
- **Missing robots directive.** No `max-image-preview:large`, so Google Discover may show only small thumbnails.
- **No feeds and no IndexNow.** Bing, and ChatGPT search, which is fed by Bing, discover new pages only by crawling.
- **Strings hard-coded in pt-BR.** The layouts (`landing/scripts/content/layouts.mjs`), the calculators (`landing/src/tools.ts`, labels plus decimal-comma parsing) and the portal templates (`portalgen/templates.go`, `jsonld.go`, `forum.go`) all have them.

## 3. Reference study: sorrifit.com (re-checked 2026-11-18)

The site is SvelteKit, server-rendered, behind Cloudflare. It has 3,088 URLs:
257 pages × 12 locales.
- **Pages per locale.** 251 come from an exercise dataset: 234 exercise pages and 16 muscle hubs plus the index. Five are tools (index plus 4 calculators) and one is the home.
- **Blog.** Only 3 posts, in pt-BR only.
- **Age.** Lastmod values are uniform build stamps and the posts date from 2026-11-11, so the site is new. Its strength is crawl volume plus clean structure, not proven rankings.

| SorriFit practice | Copy? | Abastevo adaptation |
|---|---|---|
| Locales in subdirectories. pt-BR at the root, others at `/en/`, `/es/` and so on, with translated section names and slugs. | Yes | pt-BR stays at the root, so existing URLs keep their signals. Other locales go under `/{lang}/` with translated slugs. |
| Reciprocal hreflang in the head and in the sitemap, plus `x-default`. | Yes, and fix their inconsistency | `x-default` always points to the pt-BR page, on every family. |
| Pages are only partly translated: German and the Asian locales keep English slugs. | No | Slugs are fully translated, or the locale does not ship. |
| An Accept-Language 302 at the root only, with a 1-year language cookie. | No | We promise no cookies, and the redirect breaks edge caching of the home. Plain links only, with an optional JavaScript suggestion banner that is never a redirect. |
| Entity dataset → template page → hub → "similar" links. | Already done (portal) | Extend it per locale, and add a **fuel × state** family. |
| Hub titles carry counts ("37 opções…"). | Yes | Use real counts, for example "312 postos com preço ANP em Curitiba (PR)". |
| Pillar posts: short-answer box, TOC, tables, FAQ, cited references, about 45 internal links. | Yes | We already have a "Resposta rápida" box and a TOC. Add a `citation` field to JSON-LD and require ≥ 10 links to data and tools. |
| Free calculators in every locale, with WebApplication schema. | Yes | Localize all 5 calculators. |
| `max-image-preview:large`, `max-snippet:-1`. | Yes | Add to both generators. |
| `llms.txt` with links to every language. | Yes | Add localized sections. |
| FAQPage and HowTo markup. | No | FAQ rich results are limited to government and health sites, and HowTo rich results are retired. Keep FAQ as visible content only. |
| Uniform build-date lastmod, `priority`, `changefreq`. | No | Use the real data date, as the portal does today. |
| Article links go one way: entities do not link back to guides. | No | Data pages link back to their pillar guide (SEO-26). |

The main difference between the two sites: exercise pages have the same global
demand in every language. Brazilian fuel-price demand is mostly Portuguese.
Copying 49k station pages into 12 languages would create near-duplicate pages
with little search demand. That is the pattern Google's scaled-content and
thin-content policies target, and it would waste crawl budget on a new domain.
Abastevo therefore translates the page families that have foreign demand, not
everything.

## 4. Strategy

Grow along three paths, in this order:

1. **pt-BR depth first.** Brazil is the market. More data-backed pages, a weekly editorial cadence and a clean link graph produce most of the realistic traffic.
2. **Multilingual where demand exists.** English and Spanish versions of the home, hubs, state and city pages, tools, methodology, data, and a dedicated international guide set. The audiences are visitors driving in Brazil, Mercosur neighbours crossing the border, expats, researchers and journalists. More locales open only when data shows demand.
3. **Growth loops outside Google.** App ↔ web sharing (WhatsApp), Android App Links, a weekly bulletin, feeds and IndexNow, a press/data page, and honest community distribution.

Principles:
- Every indexable page answers a real query with unique substance. Everything else is `noindex,follow` and stays out of the sitemaps.
- Each search query has one URL. A keyword map in the content registry is enforced by the build.
- A translation is a reviewed localization, never raw machine output. An unreviewed locale page is not emitted.
- Static first: extend the two existing generators, with no CMS, framework or new runtime dependency. `golang.org/x/text` is already in `backend/go.mod`; anything else needs the need/license/security review.
- No traffic, ranking or date promises. Decisions follow Search Console data (D-SEO-08).

## 5. Owner decisions (accepted 2026-11-18)

The owner accepted every recommended answer. Two inputs are still open:
- the named en and es reviewers (I18N-D05), needed before the first locale page ships;
- the editor's name and bio (I18N-D07), with "Equipe abastevo" as the fallback until then.

| ID | Decision | Accepted answer |
|----|----------|-----------------|
| I18N-D01 | Supersede D-SEO-07 and make the public site multilingual | Accepted as ADR-026, and the AGENTS.md conventions line is updated. |
| I18N-D02 | Wave A locales | `en` and `es`. The Wave B candidates are the app's other locales (`fr`, `de`, then `ja`, `ru`, `zh-CN`), each opened only by the rule in section 9. |
| I18N-D03 | URL scheme | pt-BR at the root, unchanged. `/en/…` and `/es/…` with translated section names and slugs. City and station names are never translated. |
| I18N-D04 | Which families are translated | Home, `/sobre/`, `/politica-editorial/`, tools, the international guide set, `/precos/` hubs (national, fuel, state, city), `/metodologia/`, `/dados/` and `/curiosidades/`. **Not translated:** stations, `/fiscalizacao/`, the forum and legal pages. Legal pages get an informative noindex translation, and Portuguese prevails. |
| I18N-D05 | Translation workflow | An AI draft, then a fluent human review against the glossary, recorded in front matter. The build refuses to emit an unreviewed page. Name the reviewer for en and es. |
| I18N-D06 | Currency and units on en/es pages | Show BRL per litre always. Showing an approximate US$/gallon value needs a BCB PTAX exchange-rate source admitted under the source register. Recommended: defer, and add only per-gallon in BRL. |
| I18N-D07 | Author attribution (revisits D-SEO-09) | A named human editor with a bio page, which is stronger E-E-A-T. The fallback is the current "Equipe abastevo" with the editorial policy. |
| I18N-D08 | Editorial capacity | Owner plus AI-assisted drafting with human fact-check: 2 pt-BR articles a week, the weekly bulletin summary, and 2 translated or international pages a week. |
| I18N-D09 | Privacy-safe first-party stats | Aggregated Caddy access-log counts by path, referrer and country, with IP truncated, no cookies and 30-day retention. They are enabled only together with the privacy policy update (SEO-38). Until then, Search Console and Bing only. |

## 6. Multilingual architecture

### 6.1 Locale registry and routes

There is one locale registry, `contracts/i18n/locales.json`. Each entry holds
the code, `hreflang`, `og:locale`, an `enabled` flag, the reviewer, number and
date formats, and the route-segment translations:

| Family | pt-BR (root) | en | es |
|--------|--------------|----|----|
| Home | `/` | `/en/` | `/es/` |
| Guides | `/guias/{slug}/` | `/en/guides/{slug}/` | `/es/guias/{slug}/` |
| Tools | `/ferramentas/{slug}/` | `/en/tools/{slug}/` | `/es/herramientas/{slug}/` |
| Prices | `/precos/{fuel}/`, `/precos/{uf}/{city}/` | `/en/fuel-prices/{fuel}/`, `/en/fuel-prices/{uf}/{city}/` | `/es/precios/{fuel}/`, `/es/precios/{uf}/{city}/` |
| Methodology, data, insights | `/metodologia/`, `/dados/`, `/curiosidades/` | `/en/methodology/`, `/en/data/`, `/en/insights/` | `/es/metodologia/`, `/es/datos/`, `/es/curiosidades/` |
| About, editorial policy | `/sobre/`, `/politica-editorial/` | `/en/about/`, `/en/editorial-policy/` | `/es/sobre/`, `/es/politica-editorial/` |

Fuel slugs are translated per locale, for example `gasolina-comum` →
`regular-gasoline` → `gasolina-comun`. UF codes and IBGE city slugs are the same
in every locale. Both generators read the registry. Shared test vectors in
`contracts/testdata/i18n/` prove that Node and Go produce the same URL for the
same translation key.

### 6.2 hreflang and canonical contract (B-BR-I18N-01)

- Every page in a translation group lists all of its **indexable** members, itself, and `x-default` pointing to the pt-BR member, all as absolute URLs.
- The canonical is always self. A page is never canonicalized to another locale.
- hreflang never points to a `noindex`, redirected, missing or non-canonical page. A locale member that fails its indexation gate drops out of every member's list in the same build.
- The same decision function that sets robots and sitemap membership (B-BR-SEO-01) also decides hreflang membership.
- Values: `pt-BR`, `en`, `es` (plus `x-default`). Pages also get `og:locale` and `og:locale:alternate`, and JSON-LD `inLanguage` per page.
- Tests in `landing/scripts/check.mjs` and in portalgen's link-graph verifier cover reciprocity, self-reference, `x-default`, the 200 status, self-canonical targets and no cross-family groups.

### 6.3 Sitemaps

**Landing.** The landing keeps one `sitemap.xml` urlset with every emitted
locale, because the edge routes `/sitemaps/*` to the portal. Search Console
coverage per locale is read with a URL-prefix filter.

**Portal.** The portal splits its index per locale:
`sitemaps/portal-{family}-{locale}-{n}.xml`.

**Both.** hreflang lives in the HTML head only. That keeps one source and
avoids the 40k-line sitemap bloat seen at SorriFit. Lastmod rules are
unchanged.

### 6.4 Localization rules

- **Formatting.** Numbers, currency and dates come from the locale registry: pt-BR `R$ 6,29` and `14/11/2026`, en `R$ 6.29` and `Nov 14, 2026`, es `R$ 6,29` and `14/11/2026`. Prices always stay in BRL per litre. A per-gallon value in BRL is shown on en pages only (I18N-D06).
- **Glossary.** A glossary per locale lives in `contracts/i18n/glossary/{locale}.json`, seeded from the app's `values-en` and `values-es` strings so the app and the site use the same terms. Examples:
  - etanol hidratado → "hydrous ethanol (E100)"
  - gasolina comum → "regular gasoline (ethanol blend)"
  - posto bandeira branca → "unbranded station"
  - ANP → "ANP, Brazil's oil and fuels regulator"
- **Localized copy.** Copy is localized, not translated word for word. Pages explain Brazilian context foreigners lack: attendant service, ethanol in gasoline, payment by Pix or card, and price differences by payment method.
- **Language switcher.** Plain `<a hreflang lang>` links to the same page in each enabled locale. When a page has no translation, the link goes to that locale's home. There is no automatic redirect and no cookie. An optional dismissible JavaScript banner can suggest the visitor's language, and it stores nothing.
- **Search.** `/consultar/` stays pt-BR in Wave A. Localized pages link to it with a note.
- **Rendering.** Untrusted data is still rendered with `textContent` and escaping. Translated Markdown goes through the same in-house renderer.

### 6.5 Translation workflow

- **Storage.** Translations live in `landing/content/translations/{locale}/{section}/{slug}.md`. The pt-BR sources do not move.
- **Front matter.** Each translation adds:
  - `translationOf`: the pt-BR route;
  - `translationKey`;
  - `sourceRevision`: the pt-BR `dateModified` it was translated from;
  - `translationStatus`: `draft` or `reviewed`;
  - `reviewedBy`;
  - `reviewedAt`.
- **Build gates.** The build emits only `reviewed` pages of enabled locales. A pt-BR `dateModified` newer than `sourceRevision` marks the translation stale. A stale page still builds, with a visible "updated in Portuguese on …" note, but it is `noindex,follow` and leaves hreflang and the sitemap until it is reviewed again. The check compares dates only, so builds stay reproducible.
- **Interface strings.** Strings for the layout and the calculators live in `landing/content/i18n/{locale}.json`. Portal template strings live in a Go message catalog using `golang.org/x/text`. A parity test asserts the same keys and placeholders in every locale, the same check the app uses for its 8 locales.

## 7. Content program

### 7.1 Method

- **Keyword sources (free, no tracker on the site).** Google Search Console queries, once data exists; Google Trends (BR, AR, US); Keyword Planner; autocomplete and "People also ask"; questions from the forum and the app. Volumes are recorded with their source and date, never estimated.
- **Keyword map.** `landing/content/manifest.json` gains `primaryQuery` per guide, tool and hub. The build fails when two landing pages share a primary query. Forum editorial posts must link to the owning guide (`landing_route`) and target a different query, such as a discussion or update angle. This fixes the two current overlaps (SEO-20).

#### Retargeting the two overlapping forum editorial posts (SEO-20 runbook)

Migration `000258_forum_post_landing_route.sql` adds an optional nullable `landing_route` column to `forum_posts` (validated against `^/(guias|ferramentas)/[a-z0-9]+(-[a-z0-9]+)*/$`), exported via the `forum` snapshot section and rendered by `portalgen` as a top `"Guia completo"` / `"Calculadora"` link chip when the landing origin is enabled and the route belongs to the allowlist.

To resolve the two cannibalizing pt-BR editorial posts already in the database via `admin-web` (`/admin/forum/`):

1. Open `/admin/forum/` in `admin-web` with an authorized editorial admin session.
2. Locate each overlapping post and note its current `revision` (sent as `expected_revision`).
3. Update `category`, `title`, `excerpt`, `body_text`, and `landing_route` (field **"Guia ou ferramenta da landing (opcional)"**) using the non-cannibalizing payloads below, provide a justification (`reason`, 10–500 characters), and submit (`POST /v1/admin/forum/posts/{id}/edit`).
4. Verify after the next portal export/build that both `/forum/...` pages render the `"Guia completo"` chip pointing to `/guias/etanol-ou-gasolina/` and `/guias/pesquisa-anp/`, respectively, without altering existing `/forum/` URLs (`post_id` stays stable).

Ready-to-apply payloads (`POST /v1/admin/forum/posts/{id}/edit`, fill `expected_revision` with each post's current database revision):

1. **Etanol vs. gasolina overlap -> retargeted to field-efficiency / parity edge cases**
   ```json
   {
     "category": "FUELS",
     "title": "Rendimento do etanol no uso real: quando a paridade foge dos 70%",
     "excerpt": "Fatores práticos no uso diário — trânsito urbano versus rodovia, partida a frio e variação regional de margens — que deslocam o ponto de empate do etanol em relação à regra fixa dos 70%.",
     "body_text": "A razão de 70% entre etanol hidratado e gasolina comum funciona como atalho inicial, mas o ponto real de equilíbrio varia conforme o carro e o trajeto.\n\nO que muda na prática:\n\n- Uso urbano vs. rodoviário: engarrafamentos, paradas frequentes e ar-condicionado alteram o consumo específico de cada combustível de forma diferente.\n- Calibração e geração do motor: motores flex modernos apresentam relações de consumo próprias no ciclo urbano e na estrada, informadas na ficha do veículo.\n- Dispersão de preços na cidade: diferenças de centavos entre bairros costumam pesar mais na bomba do que arredondamentos da regra geral.\n\nUse a relação calculada para o seu veículo como referência e consulte o guia metodológico completo quando quiser comparar a fórmula passo a passo.",
     "landing_route": "/guias/etanol-ou-gasolina/",
     "expected_revision": "<current_db_revision>",
     "reason": "Reposicionar post editorial para ângulo de uso real e vincular ao guia /guias/etanol-ou-gasolina/ (SEO-20)"
   }
   ```

2. **Pesquisa semanal ANP overlap -> retargeted to interpreting missing municipal averages**
   ```json
   {
     "category": "ABASTEVO_DATA",
     "title": "Leitura da amostra semanal da ANP: por que um município fica sem média",
     "excerpt": "Entenda por que cidades podem ficar sem média publicada em determinadas semanas da pesquisa da ANP — rotação amostral, tamanho da coleta e calendário — e como ler as séries locais.",
     "body_text": "Ao consultar o histórico de preços em cidades menores ou em feriados prolongados, é comum notar semanas sem média publicada para algum combustível.\n\nPrincipais motivos previstos na metodologia pública:\n\n- Cobertura amostral limitada: nem todos os municípios brasileiros integram o levantamento semanal contínuo, e cidades maiores recebem amostras proporcionais à frota.\n- Número mínimo de coletas: quando menos postos ativos são pesquisados na janela para determinado produto, a média municipal pode não aparecer naquele recorte.\n- Prazos de fechamento semanal: revisões de calendário e feriados nacionais podem ajustar o volume consolidado da semana.\n\nSempre observe o número de postos na amostra e a data da coleta exibidos na página da cidade antes de comparar semanas seguidas.",
     "landing_route": "/guias/pesquisa-anp/",
     "expected_revision": "<current_db_revision>",
     "reason": "Reposicionar post editorial para leitura de lacunas amostrais e vincular ao guia /guias/pesquisa-anp/ (SEO-20)"
   }
   ```
- **Article template.** Already partly in place. Each article has:
  - a "Resposta rápida" box;
  - a TOC;
  - a visible byline with published and updated dates;
  - tables where data helps;
  - "Fontes" with links to primary sources (laws, ANP, CONFAZ, Inmetro), also emitted as JSON-LD `citation`;
  - ≥ 10 contextual internal links, to at least 3 portal data pages, 1 tool and 2 sibling guides;
  - one app or forum call to action;
  - no fabricated statistics.
- **Reverse links.** Each portal data family links to its pillar guide. For example, city pages link to "Por que cidades vizinhas têm preços diferentes" and fuel hubs to the matching "Escolher o combustível" guide (SEO-26).

### 7.2 pt-BR editorial wave 2 (24 pieces, about 12 weeks at 2 per week)

Titles are working pt-BR copy, and each item avoids the 13 existing guides.
Facts such as the current ethanol and biodiesel blend percentages are checked
against the current CNPE/ANP rule when the piece is written.

| Cluster (category) | Pieces |
|---|---|
| Preços e mercado (`precos`) | 1. Preço da gasolina por estado: por que varia e onde está mais barata (pillar, links the 27 state hubs). 2. Como é formado o preço do diesel. 3. Preço do etanol: safra, entressafra e por que oscila. 4. Por que a gasolina sobe e desce: petróleo, dólar, Petrobras e impostos (evergreen, not news). |
| Escolher combustível (`escolher-combustivel`) | 5. Mistura de etanol na gasolina e de biodiesel no diesel: o que muda no carro. 6. Gasolina premium (alta octanagem) vale a pena? 7. Pode misturar etanol e gasolina no tanque do flex? 8. Etanol aditivado ou comum. 9. Arla 32: o que é e quando usar. |
| Economia e perfis (`economia`) | 10. Quanto gasta de combustível um motorista de aplicativo (custo por km, links the tool). 11. Moto: consumo e custo por km. 12. Diesel e frete: custo por km do caminhoneiro. 13. Calibragem dos pneus e consumo. 14. Ar-condicionado ou janela aberta. 15. Velocidade econômica na estrada. 16. Planejar viagem de carro: combustível e pedágio (pillar for the trip tool). |
| Direitos e qualidade (`direitos-e-qualidade`) | 17. Posto pode cobrar mais no cartão? Preço por forma de pagamento. 18. Bomba baixa: como conferir o volume. 19. Teste da proveta: o consumidor pode pedir? 20. Como denunciar um posto à ANP, passo a passo (links `/fiscalizacao/`). |
| Dados (`dados`) | 21. Como usar os dados abertos de preços de combustíveis (CSV from `/dados/`, for journalists and developers). 22. Gasolina nas capitais: como comparar com a pesquisa ANP. 23. O que a pesquisa da ANP não mostra (coverage limits, honest framing). 24. Histórico do preço da gasolina, only if history data and source rights allow. |

Later waves: fleet cost control (ties to Abastevo Empresas when it opens), EV
charging cost and charging points (when the EV catalog is public), and themes
from forum questions.

### 7.3 Weekly bulletin (SEO-14, already planned)

`/boletim/{aaaa-mm-dd}/` is generated from the accepted ANP survey and is
`noindex` until an editor adds a 150–300-word summary. Each issue links every
state hub that changed most, by real computed deltas. It feeds the RSS feed,
social cards and the press page. It is the main freshness signal and the main
citation target for journalists.

### 7.4 International set, Wave A (en and es)

| Page | en | es |
|---|---|---|
| Pillar: fuel guide for drivers visiting Brazil (ethanol vs gasoline in rental flex cars, attendants, Pix and cards, diesel S10, reading the sign) | Driving in Brazil: a fuel guide for visitors | Cargar combustible en Brasil: guía para conductores extranjeros |
| Border guide that links the portal city pages of surveyed border cities (for example Foz do Iguaçu, Uruguaiana, Sant'Ana do Livramento, Ponta Porã, Corumbá), only cities that have pages | Fuel near Brazil's borders | Cargar combustible en la frontera con Brasil (Argentina, Uruguay, Paraguay, Bolivia) |
| Localized versions of existing pillars | Ethanol or gasoline (the 70% rule); How Brazil's fuel prices are formed; ICMS explained; Diesel S10 or S500; How to read a Brazilian fuel price sign | the same five |
| Why Brazilian gasoline contains ethanol | yes | yes |
| Paying for fuel in Brazil (price by payment method, Pix) | yes | yes |
| Data audience: the ANP weekly survey explained, and open fuel-price data for Brazil (links `/en/data/`) | yes | yes |
| 5 calculators, `/sobre/`, editorial policy, home, methodology, data, insights | yes | yes |

That is about 20 editorial or tool pages per locale. The portal adds the
national, fuel, state and city hubs per locale, about 430 pages each, once the
portal catalog is reviewed. App tutorials (`/como-usar/`) follow in Wave A2,
captured again in en and es under the capture contract. Mixing pt-BR
screenshots into English pages is not allowed.

## 8. Growth loops beyond search

1. **App → web sharing.** Station, city and forum screens get a "Compartilhar" action that produces the canonical web URL, which renders a rich preview on WhatsApp. The station UUID used by the app and the portal must be verified to be the same.
2. **Android App Links.** Publish `/.well-known/assetlinks.json` with the release certificate fingerprint, and add `autoVerify` intent filters for `/postos/`, `/precos/` and `/forum/`. Shared links open the app when it is installed and the site otherwise.
3. **Store listing localization (ASO).** Add es (and Wave B locales) to the Play listing using the app's existing translations, and link to the matching locale home. When the store is published, every locale shows the badge and the MobileApplication JSON-LD (D-L08).
4. **Feeds and fast discovery.**
   - Atom feeds for guides, bulletins and editorial forum posts.
   - IndexNow submission of changed URLs from each release, by diffing the sitemap lastmod values. This feeds Bing, Yandex, Seznam and Naver, and Bing feeds ChatGPT search.
   - Localized sections in `llms.txt`.
   - Keep blocking training crawlers and allowing search crawlers.
5. **Social preview images.** Build-time OG images per state hub, fuel hub and guide, at 1200×630, generated with the existing store-asset tooling at build time, not at runtime.
6. **Press and data page.** `/imprensa/` and `/en/press/` hold:
   - how to cite;
   - the bulletin archive;
   - logos;
   - contact;
   - a monthly data release ("onde o combustível mais subiu").

   An embeddable price widget for news sites, with an attribution link, comes only after the portal source-rights item is resolved.
7. **Community distribution (owner, manual, no spam).** One genuine post per channel for useful pieces: TabNews and dev.to for the open-source build story, Reddit r/brasil and r/carros, ride-hailing driver and trucker groups for the cost guides, and a WhatsApp Channel for the bulletin. Never automated posting and never purchased links.
8. **Forum UGC indexing (SEO-13, later).** Moderated threads above a quality threshold become long-tail pages after the privacy review.

## 9. Measurement and decision rules

- **Tools.** Google Search Console (domain property, all sitemaps submitted), Bing Webmaster Tools and the Play Console acquisition report. First-party log stats only if I18N-D09 is accepted.
- **Monthly review (SEO-17, extended per locale).** Indexed / submitted per sitemap file, impressions, clicks, CTR and position per family and locale, top queries per page, and pages with impressions but CTR below the family median (title and description rewrite candidates).
- **Pruning.** A page family or locale with most URLs still "Discovered/Crawled – currently not indexed" after 90 days gets better content or links first. If nothing changes after a second cycle, its weakest members become `noindex`.
- **Opening a Wave B locale.** Open a locale when Search Console shows sustained impressions in that language on the en pages, or the app's install or locale share in that language justifies it, and a reviewer exists. The owner sets the exact thresholds from the first 2 months of baseline data, never in advance.
- **Editorial.** Each piece is reviewed at 30 and 90 days. Pieces that rank on page 2 get links and updates before new pieces are written on the same cluster.

## 10. Tasks

Size: S under 1 day, M 1–3 days, L about 1 week. Each task is one PR-sized
change with its own tests, run with the commands in [landing/README.md](../../../landing/README.md)
and the backend testing guide.

### Phase 0: baseline and owner inputs

- **SEO-18 Owner inputs (S, owner).** Name the en and es reviewers, and optionally the editor with a bio, per I18N-D05 and I18N-D07.
- **SEO-19 Search engine baseline (S, owner).** Confirm both sitemaps are submitted in Search Console and imported into Bing. Export the first coverage and performance numbers per family. This closes the operational part of SEO-16.

### Phase 1: pt-BR quick wins

- **SEO-20 Keyword map and cannibalization fix (S).** Add `primaryQuery` to the manifest with a uniqueness test. Rewrite or retarget the two duplicate forum editorial posts so they link to their guide via `landing_route` (see the section 7.1 runbook).
- **SEO-21 Head and template polish (M).**
  - `max-image-preview:large, max-snippet:-1` in both generators.
  - A descriptive home H1, keeping the slogan as a subtitle. The current slogan H1 is owner-reviewed copy in [content.md](content.md), so the owner approves the new text first.
  - City H1 "Preço da gasolina, etanol e diesel em {Cidade} ({UF})" and station H1 "{Nome} – {Cidade} ({UF})".
  - Article JSON-LD `citation` from the "Fontes" links.
  - Tests in `seo-and-metadata`, `json-ld`, `seo_head_test` and `jsonld_test`.
- **SEO-22 Feeds (S).** Atom feeds for guides and editorial forum posts, linked with `<link rel="alternate">`, and validated in `check.mjs`.
- **SEO-23 IndexNow on release (M).** A key file in the landing, and a changed-URL diff and submission step in the release or deploy agent after the smoke test passes. It is idempotent, rate-limited and non-blocking, with a failure test.
- **SEO-24 Navigation hubs (M).** The existing SEO-11 and SEO-15: a home entry for `/forum/`, a states grid, and shared footer hubs across the landing and the portal.

### Phase 2: i18n foundation (landing)

- **SEO-25 Locale registry and route contract (M).** `contracts/i18n/locales.json`, the glossary seed and cross-language URL vectors. Node and Go tests read the same vectors.
- **SEO-26 Multilingual landing build (L).**
  - Locale-aware manifest and the translations directory.
  - Layout string dictionaries; `lang`, `og:locale` and its alternates, `inLanguage`.
  - hreflang groups and the language switcher.
  - Per-locale 404s and sitemaps, and the root sitemap index merge.
  - The reverse links from data families to pillar guides (section 7.1).
  - `check.mjs` hreflang validators.
  - Tests: reciprocity, `x-default`, no hreflang to noindex, stale-translation gate, unreviewed page not emitted, no JavaScript needed.
- **SEO-27 Localized calculators (M).** Move `tools.ts` labels, messages and examples to the locale dictionary, with locale-aware decimal parsing and formatting. Tests: "6,29" vs "6.29" per locale, rejection messages per locale, inputs never leave the browser.

### Phase 3: i18n portal

- **SEO-28 Portal locale layer (L).**
  - A message catalog per locale (x/text), with a parity test.
  - Locale routes for hubs, fuel, state, city, methodology, data and insights.
  - Per-locale indexation gate: locale enabled, catalog reviewed, plus the existing data gates.
  - hreflang groups, sitemaps `portal-{family}-{locale}-{n}.xml`, and a link-graph verifier per locale.
  - Stations stay pt-BR and are linked from localized city pages.
  - Tests on fixtures, the scale test, and `portalcheck` strict.
- **SEO-29 Fuel × state family (M, pt-BR first).** `/precos/{fuel}/{uf}/` with real per-state data (city table, min/median/max in the sample, survey date), indexable only when the state has that fuel in the current survey. Linked from fuel hubs and state hubs, and localized in Phase 3.
- **SEO-30 Units (S, depends on I18N-D06).** BRL per gallon on en pages. US$ only after a PTAX source admission.

### Phase 4: content

- **SEO-31 Editorial wave 2 (L, ongoing).** The 24 pt-BR pieces in section 7.2, 2 a week, each with sources, links and a keyword map entry.
- **SEO-14 Weekly bulletin (M, existing).** Section 7.3.
- **SEO-32 International set en/es (L).** Section 7.4: about 20 reviewed pages per locale, plus the portal catalog review.
- **SEO-33 Tutorials en/es (L, Wave A2).** Locale captures under the capture contract, plus translated chapters.

### Phase 5: growth loops

- **SEO-34 App share links and App Links (M, Android and landing).**
  - A share action producing canonical web URLs.
  - `assetlinks.json` with the release certificate SHA-256.
  - Intent filters with `autoVerify`.
  - Tests: URL builder unit tests, and the assetlinks file validated in `check.mjs`.
- **SEO-35 Social preview images (M).** Build-time OG images per hub and guide. Any rasterizer dependency needs the dependency review.
- **SEO-36 Press and data page (M).** `/imprensa/` and `/en/press/` plus the monthly data release template. The widget waits for source rights.
- **SEO-37 Store listing localization (S, owner and agent).** es and Wave B listings from the existing app strings, with links to the locale homes.

### Phase 6: measure and iterate

- **SEO-17 Measurement loop (existing, extended).** Section 9 rules per family and locale. Wave B locale decisions follow these rules.
- **SEO-38 First-party aggregated stats (M, I18N-D09).**
  - A scheduled job on the VPS turns Caddy access logs into daily counts by path, referrer host and country.
  - IPs are truncated before aggregation and raw logs are kept for at most 30 days. The site sets no cookies and runs no client script.
  - The privacy policy (`/privacidade/`) and the security docs are updated in the same change.
  - Tests: the aggregation drops the IP and query strings, and retention deletes old raw files.

**Order.** SEO-19 and SEO-20–24 (in parallel) → SEO-25 → SEO-26 and SEO-27 →
SEO-28 → SEO-29 and SEO-30. SEO-18 (reviewers) must be done before the first
locale page ships in SEO-32. SEO-31 and SEO-14 start right after
SEO-20 and keep running. SEO-32 needs SEO-26 (and SEO-28 for portal pages).
SEO-34–37 can run any time after SEO-21. SEO-38 is independent.

## 11. Risks

| Risk | Mitigation |
|---|---|
| Thin or duplicate localized pages trigger scaled-content signals | Translate only families with foreign demand. Apply the reviewed-only build gate, the same data gates per locale and 90-day pruning. |
| Mis-translated price or legal facts | Glossary, a human reviewer, prices rendered from data (never translated text), and legal pages noindex with Portuguese prevailing. |
| Crawl budget spread over a new domain | Stations are not localized. Per-locale sitemaps monitored. IndexNow for changed URLs only. |
| Source-rights gate on portal data (ROADMAP "Portal source rights") | Localized portal pages inherit the same `INDEXABLE` switch. The widget and data redistribution wait for the rights decision. |
| Stale translations | `sourceRevision` stale check, a visible note, then noindex after the grace period. |
| Editorial capacity | A fixed cadence of 2 + 2 a week, bulletins half-automated, and no news chasing. |
| Promise of results | None is made. Every decision uses Search Console data. |

## 12. Not doing

- Machine-translating every page into 12 languages.
- Localized station pages.
- City or keyword doorway pages without data.
- Accept-Language or IP redirects, and language cookies.
- FAQPage or HowTo markup for rich results.
- `priority` / `changefreq`.
- Purchased links, link schemes or automated community posting.
- Third-party analytics or tag managers.
- AI-generated articles published without fact-check.
- Ranking or "posto mais barato" claims.

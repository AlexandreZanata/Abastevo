# Landing content, visual and decision contract (P25-T01)

State: **LOCAL_DONE — APPROVED**, 2026-10-05. Scope: documentation and two static wireframe SVGs only; no website code, dependency, remote record or publication. Owner: [P25-T01](../../ROADMAP.md#p25-t01) under the [static landing plan](../planning/STATIC_LANDING_PLAN.md) (rules B-BR-L01–L07, use cases BUC-L01–L05). T01 wireframes and tokens approved by maintainer (D-L05).

This contract freezes what P25-T02–T05 build. Changing copy, section order, tokens or a claim class goes back through this document first.

## Page inventory

| Path | Purpose | Indexed / sitemap | Notes |
|---|---|---|---|
| `/` | Product explanation, source distinction, links | Yes / Yes | One H1; six content sections plus footer |
| `/privacidade/` | Privacy statement (B-BR-L07) | Yes / Yes | Derived from maintained policy; see [Legal pages](#legal-pages-b-br-l07) |
| `/excluir-conta/` | Web path to request account/data deletion | Yes / Yes | Required by Google Play for apps with account creation |
| `/termos/` | Community terms | Yes / Yes | Reporting/moderation rules from the moderation policy |
| `/404.html` | Real 404 | No / No | Short message with links to home and repository |

Not in scope: blog, city pages, translations, dashboards, forms, analytics, newsletter.

## Verified-feature inventory and claim classes

Public wording follows the class of each capability. Classes are checked against repository state on 2026-10-05; none of them is a production claim.

| Class | Meaning | Allowed wording |
|---|---|---|
| **E1** | Exists in the preserved Android code and was released earlier under the previous name (APK on the upstream repository) | Present tense, framed as the project's foundation; no download link to the older release (B-BR-L03) |
| **E2** | Integrated in this repository (backend and Android phases merged), not released, device validation owed | Future or “em desenvolvimento” wording |
| **P** | Planned only | “Planejado” wording, never present tense |

| Capability | Source | Class |
|---|---|---|
| Browse ANP weekly price surveys by fuel, state and city; works offline | README imported features | E1 |
| Up to 3 vehicle profiles and estimated full-tank cost | README imported features | E1 |
| Optional one-shot device location, navigation to Maps/Waze, local price-drop alerts | README imported features | E1 |
| Community price observations, source/date/condition display, “unknown/disputed” outcomes | [Product contract](PRODUCT_CONTRACT.md), PROGRESS P19–P24 | E2 |
| Photo contribution with private evidence limited to 24 hours | [Local media policy](../security/LOCAL_MEDIA_LOCATION_POLICY.md), PROGRESS P21 | E2 |
| Free accounts (email code, Google, Apple), ratings, 280-character comments, votes, reports | [Free account access](../security/FREE_ACCOUNT_ACCESS.md), [moderation policy](COMMUNITY_MODERATION.md), PROGRESS P22 | E2 |
| Paid optional hosted benefits | Product contract (LATER) | P — not mentioned except the “payment never buys trust” commitment |
| Ethanol-versus-gasoline comparison | No verified feature | Excluded from copy and FAQ until a feature exists |
| “Cheapest near me”, savings figures, coverage claims, user counts, ratings | No evidence | Forbidden (B-BR-L01) |

## Copy deck (pt-BR)

Frozen strings for the home page. Section order matches the [wireframes](#visual-contract). Brackets show the store state.

**Title** (50 characters): `abastevo | Preços de combustíveis com a comunidade`
**Meta description** (146 characters): `Projeto de código aberto para consultar preços de combustíveis com fonte e data visíveis, separando observações da comunidade e referências da ANP.`
**H1:** `abastevo: preços de combustíveis com a comunidade`
**Lead:** `Um projeto de código aberto para consultar preços de combustíveis em postos brasileiros, com a fonte e a data de cada informação sempre visíveis. Comunidade e ANP aparecem separadas: uma nunca substitui a outra.`
**Status line:** `Em desenvolvimento. O aplicativo para Android ainda não foi publicado.`
**Primary link:** `Ver no GitHub` → repository (ordinary link; same tab).
**Store notice [prelaunch]:** `Em breve na Google Play` (plain text). **[published]:** `Ver na Google Play`, one link or the official badge, in one place only.

**Illustrative comparison** (heading `Comunidade e ANP, lado a lado`; label `Exemplo ilustrativo · dados fictícios`): two cards with the fictional place `Posto Exemplo` and the fictional value `R$ X,XX`; the community card reads `Observação enviada por pessoa · há algumas horas · apoio visível`; the ANP card reads `Levantamento publicado pela ANP · semana da pesquisa · não é preço da bomba agora`. Example values are always visibly fictional (no realistic price); text alternative: “Exemplo ilustrativo com dados fictícios: a mesma pergunta respondida pela comunidade e pela referência da ANP.”

**Como funciona:** `1 Consulte` — `Escolha cidade e combustível e veja os preços disponíveis, cada um com fonte, data e condição.` · `2 Confira` — `Veja se o preço vem da comunidade ou da referência da ANP e quando foi registrado.` · `3 Contribua` — `Quando o app estiver disponível, será possível enviar a foto de um preço (planejado).`

**Para quem é** (optional; include only if each line stays verifiable): `Motorista particular` — `Quem abastece com frequência e quer comparar informações com fonte e data.` · `Motorista de aplicativo` — `Quem roda muito e precisa estimar o custo de encher o tanque.` · `Pequeno frotista` — `Quem cuida de poucos veículos e quer organizar o custo de abastecer.` (vehicle profiles and tank estimate are E1).

**O projeto:** `Código aberto, com fonte e data à vista. Participar da comunidade será gratuito; pagamento nunca aumenta a confiança de um preço (planejado).` plus link `Ver código no GitHub`.
**Privacidade em resumo** (each line must stay true at publication, see legal pages): `Consultar preços não exige conta nem localização.` · `Este site não usa cookies nem rastreadores.` · `Fotos de comprovação serão privadas, apagadas em até 24 h (política do projeto).` · `Código aberto sob licença MIT.`

**Aplicativo / Acompanhe o projeto:** `Android · Em breve na Google Play` and `Acompanhe o projeto pelo GitHub; este site não pede cadastro nem e-mail. Ainda não há versões publicadas.` Link: repository. The repository currently has **no releases and an empty releases feed** (checked 2026-10-05), so the Releases page and `releases.atom` links appear only after a first release exists; the older release on the upstream repository is not linked.

**Perguntas frequentes** (native `<details>`; answers to be written from the sources named, no rich-result goal): `Como saber o preço de combustível de um posto?` (describe source/date/condition display and the “desconhecido” outcome; E2, future wording) · `O que é a pesquisa de preços da ANP?` (verify against the [official ANP page](https://www.gov.br/anp/pt-br/assuntos/precos-e-defesa-da-concorrencia/precos/levantamento-de-precos-de-combustiveis-ultimas-semanas-pesquisadas) when writing the answer; a dated reference, not the pump price now) · `Qual a diferença entre preço da comunidade e da ANP?` · `O aplicativo será gratuito?` (community participation free; optional future paid benefits never raise trust; planned).

**Footer:** links `GitHub · Privacidade · Excluir conta · Termos · Contato`; text `Código aberto sob licença MIT. Não afiliado à ANP. Dados de preços da ANP: gov.br/anp. Marcas e nomes citados pertencem a seus titulares.` and a pointer to [TRADEMARKS](../../TRADEMARKS.md). The Google brand attribution is added only at store activation, copied from the guideline current then.

## Legal pages (B-BR-L07)

**Google Play check** (source and date recorded, as required): Play Console Help, [account deletion requirements](https://support.google.com/googleplay/android-developer/answer/13327111), read 2026-10-05: an app that lets users create an account must provide an in-app deletion path **and a web link where users can request account and data deletion**; this applies even if parts of the app work without an account; Data safety deletion questions must be completed. [User Data policy](https://support.google.com/googleplay/android-developer/answer/10144311), read 2026-10-05: a valid privacy policy is expected in the store listing and inside the app, and data practices must be disclosed in the Data safety section. This is a dated reading of Google's help pages, not legal advice or a compliance claim; it is repeated at listing time.

| Page | Content sources | Must state | Must not state |
|---|---|---|---|
| `/privacidade/` | [Security and privacy plan](../security/SECURITY_PRIVACY.md), [location/media policy](../security/LOCAL_MEDIA_LOCATION_POLICY.md), [free accounts](../security/FREE_ACCOUNT_ACCESS.md), [backend notice draft](../backend/PRIVACY_NOTICE.md) | Data categories (device contributor key, account email/provider identity, photos, location, price facts), purpose, retention, who can access, rights, backup aging, contact (D-L04); site uses no cookies/trackers | Legal approval, response deadlines (legal review sets them), shipped status of unreleased features |
| `/excluir-conta/` | Rights workflow in the privacy plan, account erasure rules | How to request deletion without the app (contact channel), what is deleted, what remains anonymized, backup delay up to the backup horizon | Automated deletion that does not exist; identity disclosure without proof |
| `/termos/` | Community moderation policy | Reporting, reviewed removals, no automatic deletion by votes, free participation, MIT software license vs. name/logo rights | Moderation powers that do not exist |

**Retention reconciliation (resolved).** The README and the media policy state that app-owned photos expire within **24 hours across all copies**. The retention table in the privacy plan lists sanitized evidence at 14 days; that line is the earlier v1 baseline, and the same document states that the P15 media target replaces it. The [media policy](../security/LOCAL_MEDIA_LOCATION_POLICY.md) supersedes it and records the 14-day behavior as an implementation gap until forward changes pass tests. Decision: public wording uses 24 hours, labelled “política do projeto”, never as a guarantee for released software until the production release evidence exists.

**Documentation gap found (blocks T03 wording, not T02).** The backend privacy notice draft and the privacy plan inventory still say accounts and email are “not collected in MVP”, while the adopted [free account design](../security/FREE_ACCOUNT_ACCESS.md) collects email-code, Google and Apple identity. The privacy page cannot be written from the older sentence. A bounded refresh of those two documents (or an explicit statement in T03 evidence of which text governs) must precede T03's legal pages.

## Owner decisions and observations

| ID | Status | Evidence (2026-10-05) |
|---|---|---|
| D-L01 | **Resolved by observation** | `github.com/AlexandreZanata/brazil-fuel-prices` returns HTTP 301 to `https://github.com/AlexandreZanata/abastevo`, which is public, MIT-licensed, default branch `main`; the repository description and website fields are empty. The canonical repository URL for the site is `https://github.com/AlexandreZanata/abastevo`. The local `origin` still uses the old name (works through the redirect; changing it is a separate local choice). The plan and ROADMAP are updated accordingly. |
| D-L02 | **PENDING — maintainer confirmation** | Public registry data shows `abastevo.com.br` registered on 2026-09-28 with Cloudflare name servers and no resolving address or web response at check time; the registrant is not disclosed. Ownership is **not verified**; the maintainer confirms whether it is theirs, or chooses another origin. This is not a trademark or availability clearance. |
| D-L03 | **Criteria frozen, choice PENDING** | See [Hosting criteria](#hosting-criteria). If D-L02 is the maintainer's domain on Cloudflare, Cloudflare Pages is the natural fit. |
| D-L04 | **PENDING** | Role mailbox or repository issue tracker. Needed before the legal pages and `security.txt`. |
| D-L05 | **APPROVED 2026-10-05** | Wireframes and tokens reviewed and approved by maintainer. |
| D-L06 | **Decided: default** | `landing/` in this repository under the existing gate; separate repository only on explicit request. |
| D-L07 | **PENDING (T04)** | Default direction: normal crawlers allowed; AI-crawler policy recorded at T04. |
| D-L08 | **PENDING (store publication)** | Public listing URL and package identity after the app exists. |

## Hosting criteria

Evaluate the chosen host against these before publication (document the verification at selection): custom response headers (security headers, preview `X-Robots-Tag`, per-path cache), redirects (HTTP→HTTPS, `www`→apex, trailing slash), real 404 status, automatic HTTPS, preview deployments, access-log privacy posture, cost/limits and rollback to a previous immutable artifact. Observation on 2026-10-05 from search summaries (re-verify at selection): GitHub Pages does not allow custom response headers (only in-document `meta robots` is possible); Cloudflare Pages and Netlify support a `_headers` file; Netlify marks its default deploy preview URLs `noindex`. The plan's artifact is host-neutral; only the headers/redirects source files depend on the choice.

## Visual contract

Maintainer-reviewable static artifacts: [375 px wireframe](../assets/landing/wireframe-375.svg) and [1280 px wireframe](../assets/landing/wireframe-1280.svg). They are low-fidelity (structure, hierarchy, copy), generated as plain SVG with no script, font file, image or external reference, and are **not** the final visual style. Layout decisions captured there:

- Mobile: one column; header logo plus three anchor links in a row (no JS); hero, comparison cards stacked, three step cards stacked, optional profiles as a list, project block with privacy summary card, app card, FAQ as full-width disclosures, quiet footer.
- Desktop: content width about 60 rem (960 px); hero with the illustrative comparison beside the introduction; three columns for “Como funciona” and “Para quem é”; project block as two columns; app and FAQ side by side.
- One primary outline link (“Ver no GitHub”) and one plain-text store notice; no sticky bar, popup, carousel, autoplay, countdown or repeated download banner.

**Frozen tokens** (names are contract, final CSS names follow):

| Token | Value | Use |
|---|---|---|
| `--ink` | `#0b1849` | Headings, primary text (taken from the approved wordmark fill) |
| `--text-muted` | `#44506b` | Body secondary text |
| `--link` / `--focus` | `#0042c5` | Links, outline control, focus ring (3 px, 2 px offset) |
| `--brand-blue` | `#0052f7` | Non-text brand accents only |
| `--green-text` | `#03664b` | ANP label text |
| `--surface` | `#f5f8fc` | Alternate section background |
| `--footer-bg` | `#eef2f9` | Footer |
| `--tint-blue` / `--tint-green` / `--tint-amber` | `#e9f0ff` / `#e8f6ef` / `#fff3dc` | Label pills (`--ink` or `#7a4b00` text on amber) |
| `--line` | `#c5cee0` decorative / `#7b87a3` interactive boundary | Card and control outlines |
| Type | System font stack; 16 px base, line height 1.6; H1 about 2–2.75 rem fluid; H2 1.5–1.75 rem; small 0.875 rem | No remote fonts |
| Spacing | 4 px base: 4, 8, 12, 16, 24, 32, 48, 64, 96 | Section padding 48–96 |
| Radius / shadow | 8 and 12 px, pill; no shadows except focus | Calm, flat |
| Breakpoints | 48 rem (three-column rows), 64 rem (two-column hero/project) | Plus 320 px minimum |
| Touch target | At least 44 × 44 CSS px | Links in header/footer get padding |

Contrast ratios computed from the WCAG relative-luminance formula on 2026-10-05:

| Foreground on background | Ratio | Target |
|---|---|---|
| `--ink` on white / surface | 16.92 / 15.89 | AA text 4.5 |
| `--text-muted` on white / surface / footer / blue tint / green tint | 8.06 / 7.57 / 7.18 / 7.05 / 7.24 | AA text 4.5 |
| `--link` on white / surface / footer | 8.12 / 7.63 / 7.23 | AA text 4.5 |
| `--green-text` on white / green tint | 6.99 / 6.27 | AA text 4.5 |
| `--link` on blue tint | 7.11 | AA text 4.5 |
| `#7a4b00` on amber tint | 6.74 | AA text 4.5 |
| `--brand-blue` on white (graphics) | 5.92 | Non-text 3 |
| `#7b87a3` interactive boundary on white | 3.60 | Non-text 3 |
| `#c5cee0` decorative outline on white | 1.58 | Decorative only; never the sole indicator of a control |

**Dark appearance: not in P25.** The approved wordmark is navy and the project has no dark-background logo variant; creating one would invent brand artwork. Automatic dark mode is deferred until an approved variant exists. This resolves the plan's conditional dark-scheme allowance.

## Identity assets

Use only approved sources under `docs/assets/brand/`: [logo SVG](../assets/brand/abastevo-logo.svg), [wordmark SVG](../assets/brand/abastevo-wordmark.svg), [app icon SVG](../assets/brand/abastevo-app-icon.svg) and [1024 px icon PNG](../assets/brand/abastevo-app-icon-1024.png). T04 derives the favicon set (SVG, ICO, 180, 192, 512), `site.webmanifest` icons and the 1200×630 social image from them with recorded provenance; no new logo, lettering or illustration is created.

## Approvals and open inputs

Required before T02 opens: **D-L05** (maintainer reviews the wireframes, copy deck and tokens above) and confirmation of **D-L02**. Required before T03's legal pages: D-L04, the privacy-document refresh described above. Not required for T02: hosting choice (D-L03), crawler policy (D-L07), store listing (D-L08). No dates, traffic, costs or remote identifiers are invented.

## Validation record

Checks run for this docs-only task: local link and anchor check of this contract and the plan; SVG well-formedness and absence of script/image/external references in both wireframes; rendered-image inspection of both wireframes at their native widths; contrast computation as listed; character counts of title and description; repository, redirect, releases and registry observations dated above; `git diff --check` and scoped secret/private-data review. No browser acceptance, runtime or publication check is claimed.

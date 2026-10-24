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
| `/postos/{uuid}/` | Canonical station detail and dated history | indexable when the station has an admitted observation; `noindex,follow` when empty/stale | Directory + Official |
| `/precos/` | National overview and links to state/municipality summaries | indexable | Official current revisions |
| `/precos/uf/{uf}/` | State summary and municipalities | indexable when the state has a sample | Official current revisions + Directory geography |
| `/precos/uf/{uf}/municipio/{ibge}/` | Municipality summary and cheapest-in-sample list | indexable when the municipality has at least five priced stations; otherwise `noindex,follow` | Official current revisions + Directory geography |
| `/combustiveis/{fuel}/` | Fuel/unit definitions and period context | indexable | Official metadata; reserved for ESP-09/12 |
| `/fiscalizacao/` | Source-reported official events | `noindex,follow` until the selected FQ source/runtime is accepted | FQ-06/07 public reads; currently gated |
| `/curiosidades/` | Deterministic data questions | indexable only after ESP-12 formula approval | ESP-04/11/12 metrics |
| `/metodologia/` | Sources, dates, clocks, methods, rights and corrections | indexable | Source admission register + metrics definitions |
| `/404.html` | Unknown route or unknown station UUID | noindex | none |

`/postos/{uuid}/` is the only canonical station identity route. Slugs, CNPJ,
name changes and address changes never redirect to another station. An unknown
or withdrawn UUID returns a clear no-data/404 page; it never inherits another
operator's prices or official records.

Search URLs (`/consultar/?q=...`, `?uf=...`) are not indexable variants and
carry a canonical link to `/consultar/`. Empty search queries and filter
combinations are not generated as indexable pages.

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
  plus a link back to the Abastevo app/landing.
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
- One initial regional search shard: ≤ 250 KiB compressed.
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

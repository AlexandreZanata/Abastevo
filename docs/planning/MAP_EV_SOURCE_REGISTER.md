# Map and EV source/commercial-use admission register

Date: 2026-10-27. Status: MAP-01 DECISIONS_RECORDED /
MC-A1 OFFICIAL UNIVERSE RECORDED; SRC-ANP station facts MAINTAINER_ADMITTED
2026-10-27 (see "Maintainer decision on ANP station facts").
This register qualifies candidates; it does not grant rights or activate a source.
Implementation must fill exact version/SHA, raw digest, license evidence digest,
redistribution/derivative obligations, attribution text, retained notices, limits,
responsible task and admission decision for every shipped artifact. Unknown fields
mean NOT_ADMITTED. Code, data, artwork, hosting and protocol rights are separate.

## MAP-01 admission decisions

Decision vocabulary: `ADMITTED_FOR_SPIKE` permits local, non-shipped technical
evaluation only; `ADMITTED_FOR_SOURCE` permits implementation and tests but not
deployment; `PENDING` blocks the dependent artifact; `REJECTED` must not be used.
No decision below authorizes production distribution, an external account, a
public OSM write or a provider contact.

| ID | Candidate | Exact evidence | Decision | Remaining obligation |
| --- | --- | --- | --- | --- |
| SRC-COMAPS | CoMaps native source | `4587b8ad4d12c9f59982709fdaf03cc925948827`; LICENSE SHA-256 `09e8a9bc…f1130b`; NOTICE SHA-256 `307a6cfe…e6acc`; copyright inventory SHA-256 `5d0156f5…8121e`; [T07 audit](../backend/evidence/maps/MAP-14_T07_AUDIT.md) 215 components, 0 unresolved, 219-file notices/SBOM | `ADMITTED_FOR_SPIKE` | Human legal sign-off and public source offer; do not ship an AAR/APK until accepted. Engine SHA/ABI/patch change requires re-audit |
| SRC-GEN | CoMaps generator/styles/fonts/resources | Same source commit; `tools/python/maps_generator`, `data/countries.txt`, `data/styles`, `data/fonts` | `PENDING` | T07 notices/SBOM cover code and retained `data/copyright.html`; per-asset font/style/symbol review, exact input PBF/extract digest and reproducible regional build remain |
| SRC-OSM | OSM/Geofabrik Brazil extract | No extract selected yet; T07 ODbL derived-database/source-offer determination recorded | `PENDING` | Exact extract URL/date/bytes/SHA-256; publish the durable source/derived-data offer at release |
| SRC-ANP | ANP station facts | `revendedoresapi.anp.gov.br` `/v1/combustivel` and `/v1/glp`; open-data CSVs downloaded 2026-10-27 (45,807 automotive + 57,127 GLP rows) | `MAINTAINER_ADMITTED` (2026-10-27) for per-station factual fields and API coordinates, verbatim and attributed | Basis and limits in "Maintainer decision on ANP station facts"; no republication of ANP files or their arrangement; optional Fala.BR confirmation does not block |
| SRC-OCM | Open Charge Map subset | No provider subset selected | `PENDING` | Per-record/provider license and attribution; `opendata` flag alone is insufficient |
| SRC-COMM | Abastevo community data | Existing account/consent/moderation contracts | `ADMITTED_FOR_SOURCE` under existing rules | Public extract/relicense review; no private GPS/photos |
| SRC-ICON | Generated map pictograms | `docs/assets/map-icons/` provenance and hashes; Android exports integrated in the accessible list | `ADMITTED_FOR_SOURCE` | Preserve MIT notice/provenance; device list semantics/48 dp/icon composition PASS; MAP-18 native map-background/theme review remains |
| SRC-PLATFORM | Android location/TTS | Platform APIs | `ADMITTED_FOR_SOURCE` | No Play Services or cloud TTS requirement; permission/fallback tests |
| SRC-CAR | AndroidX Car App Library | Not selected/versioned | `PENDING` | Exact revision/license/dependency tree; external host acceptance separate |

Behavior and fixture decisions are frozen in
[the map discovery contract](../product/MAP_DISCOVERY_CONTRACT.md) and
`contracts/testdata/map-discovery/cases-v1.json`. Source admission is not runtime
activation.

## Local proof source use (2026-10-25; not a production admission)

The MAP-20 local acceptance downloaded the official ANP registry CSV, the
2026-10-11..17 station survey XLSX and the Curitiba registry API response, and
used the admitted OSM Curitiba PBF. Exact URLs, bytes, SHA-256, review method
and the 62 reviewed/33 priced counts are in the
[MAP-20 local receipt](../backend/evidence/maps/MAP-20-minimum-local-20261025/receipt.json).
This is **local proof only**: it does not admit ANP redistribution, does not
authorize a public package/data offer and does not change `SRC-ANP` from
`PENDING` for a production server artifact. The OSM package keeps ODbL-1.0
attribution. No municipal average, centroid or proximity-only station link was
used.

## Primary stack

- **CoMaps native source:** Apache-2.0 core is the committed commercial-use engine source, extracted under MAP-14 rather than consumed as a published SDK. Pin the exact upstream commit and every included dependency; vendor the minimal core/JNI host, maintain a documented patch set, build our own AAR reproducibly, and preserve LICENSE/NOTICE while marking modifications. Inspect tools, third-party code, fonts/styles/artwork and endpoints. Do not reuse the brand/logo or ship the CoMaps application/CDN. Extraction and ENGINE_GATE feasibility remain unproven until MAP-14 executes. [License](https://github.com/comaps/comaps/blob/main/LICENSE), [NOTICE](https://github.com/comaps/comaps/blob/main/NOTICE), [copyright inventory](https://github.com/comaps/comaps/blob/main/data/copyright.html), [Apache terms](https://www.apache.org/licenses/LICENSE-2.0).
- **Native generator and map resources:** derive from the audited same source/toolchain. A core license alone does not admit every input/resource. Generate compatible regional files from admitted data and serve them through Abastevo-controlled public endpoints. CoMaps documents a versioned regional map-server layout; do not infer unlimited commercial CDN capacity or automatic rights to all prepared files. [Build guide](https://github.com/comaps/comaps/blob/main/docs/INSTALL.md), [map hosting](https://www.comaps.app/support/how-can-i-host-a-custom-map-server-for-downloads/).
- **OSM / Geofabrik public extracts:** ODbL reference roads/POIs, including fuel/charging candidates. Commercial reuse is conditional on the applicable attribution and share-alike/database obligations. Source revision and exact extract bytes must be pinned. Public Geofabrik files omit contributor personal metadata; do not acquire private history/user metadata. [OSM copyright](https://www.openstreetmap.org/copyright), [Brazil extracts](https://download.geofabrik.de/south-america/brazil.html), [fuel tags](https://wiki.openstreetmap.org/wiki/Tag:amenity=fuel), [charging tags](https://wiki.openstreetmap.org/wiki/Tag:amenity=charging_station).
- **ANP official station survey facts and registry:** reuse the existing admitted pipeline where applicable; add station-level prices from official open files with exact metadata, publication dates, units and reuse evidence. Resolve exact current dataset license/terms before a new redistribution format. Historical product-level/municipal summaries are separate from a station fact. [Official recent surveys](https://www.gov.br/anp/pt-br/assuntos/precos-e-defesa-da-concorrencia/precos/levantamento-de-precos-de-combustiveis-ultimas-semanas-pesquisadas), [station-level historical series](https://www.gov.br/anp/pt-br/assuntos/precos-e-defesa-da-concorrencia/precos/precos-revenda-e-de-distribuicao-combustiveis/serie-historica-do-levantamento-de-precos), [metadata](https://www.gov.br/anp/pt-br/centrais-de-conteudo/dados-abertos/arquivos/shpc/metadados-serie-historica-precos-combustiveis.pdf).
- **Open Charge Map:** open-licensed subset only. User contributions are CC BY 4.0; imported providers may have different rights. Store provider attribution/license alongside every retained assertion. Neither an API key nor an `opendata` flag replaces the per-provider decision. Prefer the experimental bulk export with a pinned commit/selected country when qualified; it is not guaranteed live status. [Rights/API](https://openchargemap.io/develop), [export](https://github.com/openchargemap/ocm-export).
- **Abastevo public community and private records:** existing contracts govern collection/publication/moderation, purpose, retention and owner rights. Price/status reports are not automatically OSM imports/exports. Map display must respect existing contributor agreements; do not retroactively relicense private data to ODbL. Review any new public extract rights explicitly.
- **Existing Rust/Go/PostGIS/Kotlin/Room/Compose tools:** preserve the repo lockfiles/notices. Review additions by exact transitive license/security before adopting. PostgreSQL/PostGIS application use is not the same act as redistributing a modified database executable; record actual distribution obligations.
- **Android platform location/TTS:** use system APIs with permission/fallback handling, not a new required cloud API or proprietary location SDK. Installed speech engines/voices are user choices; no closed voice dataset is bundled. Core browsing/navigation must remain functional without Play Services or a network voice engine.
- **AndroidX car client, optional:** admit exact client-library revision/license/dependency tree if used. Android Auto host/platform distribution and certification are separate external constraints; this plan does not claim the host is open-source or required for phone use. [Official navigation integration](https://developer.android.com/training/cars/apps/navigation).
- **Generated Abastevo pictograms:** original generic artwork created for this request; raw output, prompt, normalization, SVG and hashes are retained. No CoMaps/operator logo or third-party icon was copied into these new assets. Keep project MIT notice and generated provenance; no claim of reproducible stochastic generation.

## Optional inputs; not prerequisites for an open-data launch

- **PBE/Inmetro and manufacturer specifications:** useful references, but exact version/redistribution rights/variant matching must be admitted. Public PDFs do not automatically qualify an entire derived catalog. Manual battery/connector/consumption input keeps the core journey independent. [PBE](https://www.gov.br/inmetro/pt-br/assuntos/avaliacao-da-conformidade/programa-brasileiro-de-etiquetagem/tabelas-de-eficiencia-energetica/veiculos-automotivos-pbe-veicular).
- **SRTM/USGS elevation:** candidate for later terrain refinement only after exact product/public-use terms and data compatibility are recorded; not a substitute for measured consumption. [Source](https://www.usgs.gov/centers/eros/science/usgs-eros-archive-digital-elevation-shuttle-radar-topography-mission-srtm-1).
- **OCPI:** an open specification for authorized operator exchange, not an open feed. New non-open operator datasets are outside this open-only release. A later provider must explicitly offer rights compatible with this policy; no account/contact/payment/credential creation now. [Specification](https://github.com/ocpi/ocpi).
- **Photon/other geocoders:** not needed if the native offline search covers requirements. Evaluate only if a proven gap remains; no unnecessary second search stack. [Photon](https://github.com/komoot/photon).

## Explicitly not required/admitted by this plan

No required Google Maps/Places, proprietary Mapbox service, scraped PlugShare/operator maps, commercial-only dynamic feed, upstream review/photo database or unspecified SDK. OpenStreetMap's standard tile server forbids offline bulk/prefetch and is not a free production SLA. Public Nominatim is limited to an aggregate maximum of one request per second and forbids autocomplete/systematic queries. Do not build the product on those public endpoints. [Tile policy](https://operations.osmfoundation.org/policies/tiles/), [Nominatim policy](https://operations.osmfoundation.org/policies/nominatim/).

Open-Meteo's free hosted API permits noncommercial use only; apps with subscriptions/advertising are commercial examples. This release does not use that endpoint. A future self-hosted open stack needs a separate code/data/deployment license review, not a URL switch. [Terms](https://open-meteo.com/en/terms).

MapLibre, OpenFreeMap and PMTiles can be legitimate open alternatives, but are not the chosen native format/renderer in this CoMaps-derived plan. Do not add both engines or feed MWM packages into MapLibre. Reconsider only with an evidence-backed revised architecture decision.

## MC-A1 official reseller universe (2026-10-27)

Scope: enumerate the official reseller sources and define denominators per kind.
No raw data is in Git; the two CSVs live only under the private VPS root
`/opt/abastevo-temp/map-completion-20261027/inputs/anp/` (mode 700).

### Endpoints and open datasets

| Kind | Source | Evidence | Fields / coordinates |
| --- | --- | --- | --- |
| Automotive fuel | `GET https://revendedoresapi.anp.gov.br/v1/combustivel` (OpenAPI 3.0.1, API 1.0) | `swagger/v1/swagger.json`; params `cnpj`, `uf`, `municipio`, `numeropagina` (the service also accepts `tamanhoPagina`); raw pages previously harvested by the national pipeline | `latitude`, `longitude`, `latitude_ANP4C`, `longitude_ANP4C`, `validacao`, `estimativaAcuracia`, `srid` (observed 4674/SIRGAS2000), `src`, `dataObtencao`, `origemInformacao`, plus registry/authorization/address/products fields |
| Automotive fuel registry | `https://www.gov.br/anp/pt-br/centrais-de-conteudo/dados-abertos/arquivos/arquivos-dados-cadastrais-dos-revendedores-varejistas-de-combustiveis-automotivos/dados-cadastrais-revendedores-varejistas-combustiveis-automoveis.csv` | 7,902,828 bytes; SHA-256 `eec94784af356616f893304c282e265bcd36c3845699b76432df3a504e89855b`; page updated 2026-10-27 09:21; daily cadence | 13 columns; **no coordinates** |
| GLP resellers | `GET https://revendedoresapi.anp.gov.br/v1/glp` | Same OpenAPI; `AC` returned 526 rows and no coordinate fields | Registry/authorization/address/distributor/class fields; **no coordinates** |
| GLP registry | `https://www.gov.br/anp/pt-br/centrais-de-conteudo/dados-abertos/arquivos/arquivos-dados-cadastrais-das-revendas-de-gas-liquefeito-de-petroleo-glp/cadastro-revendas-glp.csv` | 11,785,474 bytes; SHA-256 `be9b6b188da7b55094f9f6b658a10a7d53f891ee56b8234810046b41e6710172`; page updated 2026-10-27; daily cadence | 14 columns including `CLASSE`; **no coordinates** |
| TRR / TRRNI | No API endpoint or open-data CSV found | ANP distribution pages link to CDP; the CDP export is CAPTCHA/manual ("não permite consulta automatizada") | Not admitted; no denominator claimed |
| Aviation / GNV / marine / floating / school / supply points | CDP "Consulta de Postos" type filter lists these types | No API endpoint or open-data CSV found; CDP export is CAPTCHA/manual | Not admitted; no denominator claimed |

### Denominators

| Kind | Rows | Unique CNPJ | Notes |
| --- | --- | --- | --- |
| Automotive fuel | 45,807 | 45,807 | Matches the Phase 1 API harvest and the existing registry inputs |
| GLP resellers | 57,127 | 57,124 | 3 duplicate CNPJ rows retained as source rows; no coordinates |
| **Total declared universe** | **102,934** | **102,931** | TRR/aviation/etc. excluded until an open source exists |

The 3,003 directory stations without UF (2,924 LPG from the weekly price survey
and 79 priced fuel stations absent from the current registry/API) are accounted
for as: LPG price-survey points → GLP kind; 79 fuel stations → automotive fuel
kind. They are not added to the denominators above because they are already
directory rows; the dossier step must join them by CNPJ or mark them absent
from the official registry.

### Licence finding and decision

The ANP open-data pages state that all site content is published under
**Creative Commons Atribuição-SemDerivações 3.0 Não Adaptada (CC BY-ND 3.0)**.
The API manual references the same open-data page and states no separate API
licence. NoDerivatives does not authorize publishing a transformed/derived
dossier that combines ANP facts with Receita/IBGE and computed location tiers.

Initial decision (superseded the same day): the new `station-dossier`/derived
artifact was held as BLOCKED_PENDING_LEGAL. See the maintainer decision below.

### Maintainer decision on ANP station facts (2026-10-27)

The maintainer resolved the SRC-ANP block for the map completion batch. This is
a project decision with its legal basis recorded, not a legal opinion.

Basis:

- Lei 9.610/1998 art. 8º, V: "as informações de uso comum tais como
  calendários, agendas, cadastros ou legendas" are not protected by copyright.
- Lei 9.610/1998 art. 7º, XIII and § 2º: a database is protected only by an
  original selection/arrangement, and that protection "não abrange os dados ou
  materiais em si mesmos". The ANP reseller registry is an exhaustive official
  list, and the project uses its facts, not its arrangement.
- Decreto 8.777/2016 art. 2º, III: federal open data must carry a licence that
  allows "livre utilização, consumo ou cruzamento, limitando-se a creditar a
  autoria ou a fonte". Crossing with Receita/IBGE is the use this policy names.
- The CC BY-ND 3.0 footer governs the site's copyrightable content. The
  project's existing ESP register already reads it as a site-content notice,
  not a dataset licence.

Admitted use: per-station factual fields (identity, authorization, address,
products, coordinates and their accuracy fields) and weekly price facts, kept
verbatim with source, URL, retrieval time and "Fonte: ANP" attribution in
the app, API and static files. Our own derived values (tiers, validation
codes, matches) are labeled as Abastevo's and never presented as ANP's.

Not admitted: republishing ANP's CSV/API responses or their file layout as a
download, altering an official value, implying ANP endorsement, or using site
texts/images. Personal data stays excluded (B-BR-MAP-17).

Takedown/confirmation: if ANP objects, the affected fields are withdrawn
from public outputs in the next release. An optional Fala.BR confirmation can
be sent by the maintainer (text below); it does not block construction or
staging. The ESP enterprise register keeps its own `RIGHTS_PENDING` states for
bulk enterprise downloads, which this decision does not cover.

Optional Fala.BR text (maintainer sends; agents never contact third parties):

> Solicito confirmação de que os dados cadastrais dos revendedores varejistas
> de combustíveis automotivos e de GLP, publicados em dados abertos e na API
> revendedoresapi.anp.gov.br (incluindo latitude/longitude), podem ser
> reutilizados e cruzados com outras bases públicas em aplicativo gratuito e de
> código aberto, com atribuição "Fonte: ANP", nos termos do art. 2º, III, do
> Decreto 8.777/2016. A licença CC BY-ND 3.0 do rodapé do site se aplica a
> esses conjuntos de dados?

### Universe scope decisions (2026-10-27)

- Automotive fuel resellers (45,807) are the driver-map universe.
- GLP resellers (57,127) enter the catalog and dossier; on the map they are a
  separate "Gás de cozinha (GLP)" layer, off by default. The API has no GLP
  coordinates, so they pin only through the address tiers.
- TRR, TRRNI, aviation, marine, floating and supply-point classes are
  NOT_APPLICABLE: they are not consumer retail for drivers and have no open
  source; the CAPTCHA export is never automated.
- ANP API politeness: at most one request per 20 s, honour `Retry-After`,
  stop at the first challenge and resume from a per-UF checkpoint on the next
  run. The CSVs are the denominators; the API is used only for coordinates.

## EV and hybrid driver sources (assessment 2026-10-27)

No Brazilian authority publishes an open registry of public EV chargers:
charging is a free activity under ANEEL rules, distributor connection records
are not open data, and the ANEEL open-data portal returns no charging dataset.
The best map for EV and hybrid drivers is therefore a cross-checked
combination, each layer labeled with its source:

| Need | Source | Status | Use |
| --- | --- | --- | --- |
| Fuel stations and prices (hybrids) | SRC-ANP | `MAINTAINER_ADMITTED` | Official stations, weekly prices |
| Charger locations | SRC-OSM `amenity=charging_station` | ODbL, pending extract pin | Primary open geometry, sockets/operator tags |
| Charger locations | SRC-OCM, only POIs whose data provider licence is CC BY 4.0 (OCM contributors) | `PENDING` per provider | Second independent source |
| Charger confidence | OSM ∩ OCM match within 50 m and same socket family | Derived (Abastevo) | "Confirmado por 2 fontes" label; single-source stays unconfirmed |
| Charger live status | SRC-COMM working/not-working reports (MAP-21) | `ADMITTED_FOR_SOURCE` | Dated community status, never official |
| Vehicle efficiency | Inmetro PBEV tables (MJ/km, EV range km, hybrids) | `PENDING`: exact edition, file and reuse terms | Defaults for the MAP-26 energy model; manual input stays available |
| Charging cost at home | ANEEL distributor tariffs (open data) | `PENDING`: dataset and licence | Cost per km electric vs fuel per municipality |
| Terrain | SRTM (USGS) | Optional (already listed) | Energy estimate on hilly routes |
| Operator live data | OCPI feeds | Not admitted (no open rights) | Only with explicit written rights later |

Hybrid-driver value: the same route can show the cheapest official fuel price
and the nearest confirmed charger, with estimated cost per km for each energy.
Admission work for OCM, PBEV and ANEEL tariffs is task MC-A8.

## Source admission record template

For every source/artifact record:

1. Stable source/provider ID, purpose, required/optional status and exact URL/revision.
2. Code license and all shipped transitive licenses; separate data/artwork license.
3. Evidence URL/date and retained terms/license bytes/hash.
4. Commercial use, copying, modification, offline caching, redistribution and derivative-database obligations.
5. Source/raw bytes SHA-256 and parser/profile/rights version.
6. Data fields retained/excluded, public/private treatment and attribution text.
7. Update/expiry/revocation/deletion behavior and coverage limits.
8. Security/resource constraints, test fixtures and permitted acquisition method.
9. Decision ADMITTED/REJECTED/PENDING with concrete reason and task evidence.

A successful parse, a public URL, a green CI check or physical table separation
never substitutes for this decision. Admit only the minimal necessary regional
subset; no completeness claim about all Brazilian stations or live availability.

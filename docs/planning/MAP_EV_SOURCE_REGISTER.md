# Map and EV source/commercial-use admission register

Date: 2026-10-24. Status: RESEARCHED / BYTE_AND_TRANSITIVE_ADMISSION_PENDING.
This register qualifies candidates; it does not grant rights or activate a source.
Implementation must fill exact version/SHA, raw digest, license evidence digest,
redistribution/derivative obligations, attribution text, retained notices, limits,
responsible task and admission decision for every shipped artifact. Unknown fields
mean NOT_ADMITTED. Code, data, artwork, hosting and protocol rights are separate.

## Primary stack

- **CoMaps native source:** Apache-2.0 core is a commercial-use candidate. Pin upstream commit and every included dependency; preserve LICENSE/NOTICE, mark modifications and inspect tools, third-party code, fonts/styles/artwork and endpoints. Do not reuse the brand/logo. No published drop-in Android SDK or extraction feasibility is asserted here. [License](https://github.com/comaps/comaps/blob/main/LICENSE), [NOTICE](https://github.com/comaps/comaps/blob/main/NOTICE), [copyright inventory](https://github.com/comaps/comaps/blob/main/data/copyright.html), [Apache terms](https://www.apache.org/licenses/LICENSE-2.0).
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

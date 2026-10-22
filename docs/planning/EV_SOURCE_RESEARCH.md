# Electric mobility source research and admission register

Reviewed: 2026-10-20, America/Cuiaba. Scope: Brazilian vehicle references,
charging places, tariffs/status, community information and local processing.
Owner: [EV extension](EV_FULL_SUPPORT_EXECUTION_PLAN.md), integrated into
[MEX](MOBILE_EXPANSION_EXECUTION_PLAN.md). Product: [EV contract](../product/EV_SUPPORT_CONTRACT.md).

**2026-10-21 sample qualification:** [EE-01](../backend/evidence/ev-support/EE-01.md)
pins three actual Brazilian OCM POIs/reference and the actual PBE PDF bytes.
Provider1 reports CC-BY4.0; PBE reuse rights remain pending. The minimized sample
register is not a national census or source activation/rights blanket approval.

## Research result and limits

No single comprehensive, open Brazilian federal dataset of every charging place,
connector, price and live occupancy was established. This is a bounded research
finding, not proof that no such dataset could exist. Use separate source roles
and explicit provenance; do not turn an incomplete registry into national truth.

The register contains 23 external source/protocol families and two first-party
data classes. Primary publisher pages/docs were inspected through web research;
the individual entries state important access/coverage limits. It is not an
exhaustive list of every operator or a national data census. Search snippets,
page dates, public maps and global marketing are not proof of source rights,
current Brazilian coverage, successful API access or operating capacity.

Documentation access did not create accounts, request commercial access, buy
products, contact operators, obtain keys, call authenticated EV APIs, download
national OSM files or import customer data. No raw dataset was committed. A
public PDF was inspected through the web PDF text/screenshot path, not through
a production parser/full-file checksum/census. No adapter is runtime-approved.

Admission labels below mean: PRIORITY = qualify first; CONDITIONAL = rights,
coverage, credentials or measured need required; REFERENCE = context only;
FIRST_PARTY = new app data subject to the owning contract. All production source
activation remains gated by EE tasks, source-specific rights and actual tests.

## Vehicle specifications and official reference sources

### EVS-01 — Inmetro PBE Veicular
- Role: Brazilian model/version energy-efficiency and range references. PRIORITY.
- Evidence: [current index](https://www.gov.br/inmetro/pt-br/assuntos/regulamentacao/avaliacao-da-conformidade/programa-brasileiro-de-etiquetagem/tabelas-de-eficiencia-energetica/veiculos-automotivos-pbe-veicular),
  [linked PDF](https://www.gov.br/inmetro/pt-br/assuntos/regulamentacao/avaliacao-da-conformidade/programa-brasileiro-de-etiquetagem/tabelas-de-eficiencia-energetica/veiculos-automotivos-pbe-veicular/mascara-pbev-2026_19_jan-rev01.pdf/@@download/file).
- Observed: the page's displayed filename/update and PDF's internal update do
  not align simply with the URL name. Text inspection exposes energy in MJ/km,
  electric range and equivalent km/L fields; equivalent km/L is not liquid fuel
  use by a BEV. Pin actual bytes/profile/source edition, units and footnotes.
- Gate: verify redistribution terms/attribution, actual format and independent
  complete row oracle. No assumed CSV/API, battery capacity, connector or charging
  curve where the source does not provide it. Lab reference is not personal use.

### EVS-02 — Manufacturer technical sheets and manuals
- Role: battery gross/usable capacity, AC/DC limits, connectors and vehicle
  restrictions missing from PBE. CONDITIONAL, exact Brazilian market/version.
- Evidence example: [BYD model page](https://www.byd.com/br/car/dolphin-mini) and
  [manufacturer technical PDF](https://www.byd.com/content/dam/byd-site/br/fichas-t%C3%A9cnicas---update-2025/julho/Ficha_T%C3%A9cnica_Dolphin_Mini_16_Jul_25.pdf).
- Gate: pin market/year/trim/document, validate reuse rights and minimize copied
  material; a readable specification is not automatic permission for wholesale
  catalog reproduction. Do not infer one trim's connector/capacity for another.

### EVS-03 — ANEEL charging regulation
- Role: regulatory context, not charger inventory or operator occupancy. REFERENCE.
- Evidence: [ANEEL topic](https://www.gov.br/aneel/pt-br/assuntos/veiculos-eletricos),
  [official norm publication](https://www2.aneel.gov.br/cedoc/pubren20211000.pdf),
  [Inmetro regulatory analysis](https://www.gov.br/inmetro/pt-br/assuntos/regulamentacao/analise-de-impacto-regulatorio/realizadas/2024/conectores-para-carregamento-de-veiculos-eletricos-plug-in/relatorio/@@download/file).
- Access limit: some direct ANEEL norm fetches returned 403; indexed norm text
  and official Inmetro analysis establish context, not a fresh consolidated legal
  certification. Recheck effective rules before an operational/legal claim.

### EVS-04 — Inmetro metrology and conformity material
- Role: distinguish regulatory work from an equipment certification. REFERENCE.
- Evidence: [2026–2027 Dimel agenda](https://www.gov.br/inmetro/pt-br/assuntos/regulamentacao/agenda-regulatoria/RTAC003139.pdf),
  [charger FAQ](https://www.gov.br/inmetro/pt-br/acesso-a-informacao/perguntas-frequentes/avaliacao-da-conformidade/aparelhos-eletrodomesticos-e-similares/carregadores-de-veiculos-eletricos-estao-no-escopo-da-portaria-inmetro-ndeg-148-de-2022-carregadores-de-veiculos-eletricos-devem-ser-certificados).
- Gate: an agenda/AIR is not an issued approval or inspection record. Do not
  display an Inmetro/ANEEL approval badge from a map listing or protocol support.

### EVS-05 — ANEEL distributor application tariffs
- Role: contextual electricity tariff reference, never a public charger quote
  or automatically the user's final home bill. CONDITIONAL.
- Evidence: [dataset and license metadata](https://dadosabertos.aneel.gov.br/dataset/tarifas-distribuidoras-energia-eletrica).
- Observed metadata declares ODbL; government-hosted does not imply CC0. Preserve
  distributor, subgroup, modality, period, unit and effective dates. TE/TUSD
  reference components must not silently become final all-in BRL/kWh. Use actual
  known amounts or clearly labelled assumptions for personal estimates.

## Charging location candidates and bulk data

### EVS-06 — Open Charge Map API and reference data
- Role: candidate charging places, operators, connectors and reported metadata.
  PRIORITY, subject to per-provider license qualification.
- Evidence: [publisher API documentation](https://www.openchargemap.org/develop/api),
  [publisher developer guide](https://github.com/openchargemap/ocm-docs/blob/master/System/DeveloperGuide.md).
- Documented API key/fair usage; mixed provider licenses. Request open-data
  records and retain provider-specific rights/attribution. Do not assume all
  replies may be redistributed/offline, or that listed status is live telemetry.

### EVS-07 — Open Charge Map experimental bulk export
- Role: alternative reproducible ingestion path, not a second independent
  corroborating source for the same OCM assertion. PRIORITY qualification.
- Evidence: [README](https://github.com/openchargemap/ocm-export/blob/main/README.md),
  [country directories](https://github.com/openchargemap/ocm-export/tree/main/data).
- Observed Brazil directory and publisher-described experimental per-POI JSON
  export with reference data. Pin commit and manifest; inspect current completeness,
  source clocks, raw user/contact fields, per-provider licenses and deletions.
  Do not clone/download the whole world merely to import Brazil.

### EVS-08 — OpenStreetMap charging objects
- Role: candidate location/access/connector data. CONDITIONAL on ODbL boundary.
- Evidence: [charging tags](https://wiki.openstreetmap.org/wiki/Tag:amenity=charging_station),
  [charge-point model](https://wiki.openstreetmap.org/wiki/Tag:man_made=charge_point),
  [data license](https://www.openstreetmap.org/copyright).
- Optional/missing tags and mapping conventions are not verified opening hours,
  available sockets or measured output. Preserve node/way/relation identity and
  version; avoid double-counting a site and its child equipment. Resolve license
  obligations before mixing into a derived public database; separation alone is
  not a legal exemption. Keep MIT software notices and data licenses distinct.

### EVS-09 — Geofabrik Brazil extracts
- Role: bounded OSM acquisition option, not an independent location source.
  CONDITIONAL; prefer measured regional/minimized extraction if appropriate.
- Evidence: [Brazil](https://download.geofabrik.de/south-america/brazil.html),
  [download/metadata/license explanation](https://www.geofabrik.de/data/download.html).
- Bulk PBF is large; freeze compressed/expanded/memory/time limits before fetch.
  Public extracts omit personal contributor metadata described by the provider.
  Preserve ODbL/attribution and regional coverage. Do not load national PBF on
  Android or use public OSM endpoints as a per-keystroke national backend.

### EVS-10 — Overture Places
- Role: supplemental place candidates; not guaranteed charging equipment or
  live tariff/status coverage. CONDITIONAL, not a core dependency.
- Evidence: [Places guide](https://docs.overturemaps.org/guides/places/),
  [theme/source licensing](https://docs.overturemaps.org/attribution/).
- Verify current taxonomy, Brazilian charger samples, release/GERS identity and
  per-theme/provider terms. Places licensing differs from Overture transportation
  or divisions; do not apply one theme's permission to all data. No dynamic
  charger feed was established by this research.

## Protocols, partner candidates and commercial alternatives

### EVS-11 — OCPI / EVRoaming Foundation
- Role: partner protocol for locations, equipment, tariffs and authorized
  sessions/CDRs. PRIORITY interoperability contract; feeds are CONDITIONAL.
- Evidence: [foundation](https://evroaming.org/),
  [official repository](https://github.com/ocpi/ocpi),
  [tariff specification](https://github.com/ocpi/ocpi/blob/2.3.0/release/core/mod_tariffs.asciidoc).
- Pin each partner's supported version/modules, identifiers, scopes, rights,
  token rotation and freshness contract. An open protocol does not grant access
  to operator data, roaming, reservations, payment or charging commands.

### EVS-12 — OCPP / Open Charge Alliance
- Role: charger-to-management-system protocol reference. REFERENCE.
- Evidence: [official protocol page](https://openchargealliance.org/protocols/open-charge-point-protocol/).
- Do not create a charger-control backend merely to build a discovery/cost app.
  No arbitrary charger connection, direct control, billing or hardware integration
  is authorized by the current EV data-support scope.

### EVS-13 — Tupi
- Role: Brazilian operator/platform partnership candidate. CONDITIONAL.
- Evidence: [platform](https://tupimob.com/), [partnership page](https://tupimob.com/parceria/).
- Public product/partnership descriptions were inspected, not an authorized
  third-party API contract. Need explicit fields, Brazil coverage, refresh,
  redistribution/offline rights, pricing and technical access before ingestion.

### EVS-14 — EZVolt
- Role: Brazilian platform/operator candidate. CONDITIONAL.
- Evidence: [publisher](https://ezvolt.com.br/).
- Public service/app descriptions do not grant a feed or live-status access.
  No endpoint was guessed or called. Qualify agreement and source scope first.

### EVS-15 — Zletric
- Role: operator/network and customer-specific tariff candidate. CONDITIONAL.
- Evidence: [publisher](https://www.zletric.com/).
- Public information includes different products/membership conditions. Do not
  publish a member rate as the standard rate for everyone. No open feed/access
  or complete current coverage was established.

### EVS-16 — Raizen/Shell Recharge
- Role: prospective operator source, not assumed current access. CONDITIONAL.
- Evidence: [publisher's dated partnership announcement](https://www.raizen.com.br/sala-de-imprensa/raizen-power-e-99-ampliam-parceria-para-impulsionar-a-mobilidade-eletrica-nacional).
- The inspected announcement is historical. Reverify current operating entity,
  network ownership and availability of a licensed data agreement; brand/CNPJ
  history must not become an immutable location identity or automatic feed.

### EVS-17 — PlugShare
- Role: possible commercial catalog/API option. CONDITIONAL, not open seed data.
- Evidence: [publisher API access policy](https://help.plugshare.com/hc/en-us/articles/4418950880659-PlugShare-Charging-Stations-API-Documentation-Access),
  [terms](https://company.plugshare.com/terms.html).
- Publisher directs users to commercial licensing. Do not scrape its map,
  reviews/photos or reverse-engineer the app to populate Abastevo. No license,
  quotation or developer key has been acquired.

### EVS-18 — TomTom EV APIs
- Role: optional commercial POI/availability provider. CONDITIONAL.
- Evidence: [API](https://docs.tomtom.com/ev-charging-stations-availability-api/documentation/ev-charging-stations-availability-api/ev-charging-stations-availability),
  [market coverage](https://docs.tomtom.com/ev-charging-stations-availability-api/documentation/product-information/market-coverage).
- Important observed limit: Brazil is marked for static EV data, but not dynamic
  EV data in the inspected coverage matrix. Do not promise Brazilian live
  occupancy simply because a global availability API exists. Confirm current
  contract/region, quotas, allowed cache and sample replies before admission.

### EVS-19 — HERE EV Charge Points
- Role: commercial alternative to qualify, not a validated Brazilian live feed.
  CONDITIONAL.
- Evidence: [publisher product sheet](https://assets.here.com/x7rx8ayph7ee/32b79353-8f28-4728-8a39-8f3dd91cd0bd/ed42458d774754bc651f13c5d3f229cb/ev_charge_points_one-pager_here.pdf).
- Global product features do not establish Brazilian coverage/freshness. One
  legacy documentation URL was inaccessible. Obtain current regional docs,
  license, sample and cost/retention terms rather than guessing an endpoint.

### EVS-20 — Google Places EV fields
- Role: optional governed online enrichment; not an assumed offline seed source.
  CONDITIONAL with strict storage/display boundary.
- Evidence: [EV fields](https://developers.google.com/maps/documentation/places/android-sdk/reference/com/google/android/libraries/places/api/model/EVChargeOptions),
  [Places policies](https://developers.google.com/maps/documentation/places/web-service/policies).
- Policies restrict prefetch/storage/caching subject to exceptions and impose
  attribution/display conditions. Do not bulk copy Places into Room/PostgreSQL,
  ship it as an offline national database or redistribute its reviews/photos.
  Key/billing/product rights and actual Brazilian completeness remain untested.

## Coverage context and first-party information

### EVS-21 — ABVE / Tupi published statistics
- Role: contextual adoption/infrastructure comparisons. REFERENCE.
- Evidence: [ABVE publisher](https://abve.org.br/home-2/).
- Published totals are not a redistributable row-level location inventory or
  availability feed. Separate definitions of place/equipment/connector and dates
  before comparing totals. Underlying licensed access was not established.

### EVS-22 — EPE / MME electromobility studies
- Role: planning/market context, not current station-level offers. REFERENCE.
- Evidence: [PDE 2035 electromobility study](https://www.gov.br/mme/pt-br/assuntos/secretarias/sntep/publicacoes/plano-decenal-de-expansao-de-energia/pde-2035/cadernos/pde-2035_caderno_eletromobilidade.pdf).
- Preserve study/observation dates and underlying statistical sources. Do not
  turn a forecast, aggregate chart or cited operator count into a live catalog.

### EVS-23 — SENATRAN aggregated fleet data
- Role: geographical adoption context and possible demand analysis. REFERENCE.
- Evidence: [2026 fleet portal](https://www.gov.br/transportes/pt-br/assuntos/transito/conteudo-Senatran/frota-de-veiculos-2026).
- Aggregates do not identify charger compatibility, current charging demand or
  a private driver's vehicle. No VIN/plate/owner lookups or personal datasets are
  necessary. Inspect fuel/propulsion categories and dated files before comparison.

### EVS-24 — Abastevo community observations
- Role: public dated tariff components, access and operating-experience reports.
  FIRST_PARTY, NEW_IMPLEMENTATION_REQUIRED.
- Evidence owner: [EV contract](../product/EV_SUPPORT_CONTRACT.md), existing
  [community experience](../product/COMMUNITY_EXPERIENCE.md) and
  [moderation](../product/COMMUNITY_MODERATION.md).
- Signed free-account writes, scoped target/tariff conditions, temporal evidence,
  corrections/replay/quotas/moderation and confidence explanations are required.
  A contributor is not an operator or official inspector; no GPS mandate,
  fabricated live availability or automatic price from a private session.

### EVS-25 — Private user charging sessions/home tariff
- Role: owner-only cost/usage history and optional Premium sync. FIRST_PARTY,
  NEW_IMPLEMENTATION_REQUIRED; not public source corroboration.
- Evidence owner: [financial method](../product/EV_FINANCIAL_METHOD.md).
- Record actual paid total, energy measurement basis and odometer/time windows.
  Private home location, route/SOC history, bills and credentials cannot enter
  public maps, rankings or snapshots of public data. No document/OCR intake.

## Admission work and evidence required before activation

For every admitted source record publisher/URL/origin allowlist, retrieval rights,
per-provider data license, offline/display/redistribution permissions, evidence
snapshot/hash, schema/version, Brazilian sample, clock semantics, source IDs,
coverage/limitations, rate/byte/row/depth/CPU/memory budgets and allowed fields.
Classify metadata, tariff, status, community claim and private ledger separately.
None inherits another source's freshness or authority.

Pin reproducible approved snapshots, independently reconcile each stream's input,
accepted/duplicate/quarantined/metadata counts, and retain source-specific lineage.
Nested equipment counts are not the input-place row denominator. OCM API/export
and OSM/Geofabrik are two transports of the same underlying evidence, not two
independent witnesses. A checksum proves bytes, not regulatory approval, consent
or an authentic tariff for the user.

The recommended initial qualification path is PBE plus manufacturer supplements
for exact vehicle variants, OCM open-licensed Brazilian candidates, first-party
community observations and private sessions. OSM is a separately gated source
after license-boundary approval. Commercial/live feeds remain optional or
EXTERNAL_ACCESS_PENDING until explicitly licensed and tested. No contact or
purchase is authorized by this document.

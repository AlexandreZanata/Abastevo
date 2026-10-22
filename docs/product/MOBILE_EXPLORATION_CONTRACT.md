# Mobile exploration v1 — typed questions and official-record journeys

Date: 2026-10-21. State: CONTRACT_FROZEN / RUNTIME_PENDING (MEX-02).
Rules: B-BR-MEX-08–18; journeys BUC-MEX-06–10.
[Planned shapes](../../contracts/openapi/mobile-exploration-planned.json),
[compatibility examples](../../contracts/testdata/mobile-expansion/exploration-v1.json)
and [counting examples](../../contracts/testdata/mobile-expansion/counting-v1.json)
freeze future contracts. The schema fragment has no paths and changes no live
OpenAPI/API. Existing FQ/PRM/OPT owners and actual runtime gates remain required.

## Typed intent and comparable prices

Free public questions produce RESOLVED with a visible typed query, NEEDS_CHOICE
with two or more labelled alternatives, or UNSUPPORTED with an explanation.
"Qual a gasolina mais cara do Brasil?" does not choose source/week/condition or
silently equate highest observed station price with highest state/city average.
Ask for missing meaning, retaining the rest of the user's selection. Resolution
is an allowlisted grammar, never executable SQL, script, remote URL or unlimited
provider request. No AI/search-service dependency is introduced by this task.

`PriceQuery` freezes version1 and `policy_version=mex-exploration-v1`, with:

- One source: ANP_WEEKLY plus survey-week start, or COMMUNITY current eligible
  projection. A historical weekly reference is not a current pump offer.
- Exact product/unit: liquid fuels use L; CNG uses M3; LPG_P13 uses KG_13. Keep
  additive GASOLINE_ADDITIVED distinct from GASOLINE_PREMIUM_GRADE; the Android
  legacy GASOLINE_PREMIUM spelling must not be reinterpreted on the wire.
- Exact condition: STANDARD has no qualifier; conditional APP/LOYALTY/OTHER
  questions identify the program/qualifier; CASH/DEBIT/CREDIT keep their exact
  condition and any supplied qualifier. Actual source/endpoint capability may
  support fewer conditions. Never silently drop unsupported conditions.
- COUNTRY/STATE/MUNICIPALITY with BR, explicit UF and stable IBGE code where
  relevant. Verify code/UF membership against the existing catalog, not spelling
  or a national price-presence list. Location existence and price coverage differ.
- OBSERVATION_PRICE grouped by STATION, or AREA_MEAN grouped by municipality/state,
  sorted ASC/DESC. Area means use eligible source-observation totals/denominators,
  not averages of unweighted city averages. Show eligible observations, distinct
  stations/areas and missing coverage separately. Do not average raw community
  reports as though repeated reports were independent supported station prices.
- Bounded pages (1–100) and an optional opaque cursor. No client-side filtering
  of a downloaded page presented as the complete national ranking.

Reuse existing exact Money/condition/kernel types. Average sorting/comparison
uses exact integer/rational values, formatting only at display. Stable ties use
the semantic value then stable geography/station/source-record identities; no
random order, paid placement or additional precision fabricated by formatting.
Every price result must carry source edition/revision, dates, unit/condition,
eligible denominator and coverage. Unknown/missing/excluded prices are not zero.
The shape/oracle tests here do not implement aggregation, parser, ranking SQL or
prove national coverage; those are MEX-28–31 acceptance.

Electricity is not an added liquid FuelProduct. EE-02/11/14/25 must extend these
typed shapes for connector-compatible energy/tariff scenarios and known fees
before a combined mobile master surface is accepted. No literal lowest kWh rate
may stand for cheapest charging visit when mandatory components are unknown.

## Official query, fields and availability

`OfficialQuery` selects actual source IDs (pmqc/enforcement/sanctions), explicit
record kinds, geography/basis, date basis/precision, reported-status selection,
association scope and bounded page. Its syntactic shape is not permission to
claim a source supports a field. Before execution, resolve the admitted source
manifest/capabilities and reject unsupported combinations explicitly.

Filter capabilities are SUPPORTED, UNSUPPORTED or UNKNOWN, with a reason:

- PMQC assay fuel is filterable only when explicitly present and mapped by its
  accepted profile. That does not make fine/action records fuel-specific.
- EVENT_DAY needs an actual event/inspection day. DUE_DAY is a payment due day.
  Extraction/source-period dates and report year are never incident dates.
- SOURCE_STATUS is an exact allowlisted source-reported value; absence stays
  unknown. Neither raw paid=true nor payment dates prove finality, guilt,
  exhaustion of appeal, cleared fuel or clearance of the business.
- SOURCE_GEOGRAPHY uses prepared source geography. Current Directory address is
  an independent CURRENT_DIRECTORY filter available only with a current-company
  scope and live verification. Never substitute it for historical occurrence
  geography. Missing municipality/segment/fuel stays unknown, not guessed.
- CURRENT_COMPANY requires the accepted FQ company chain and guards.
  HISTORICAL_SITE is unsupported without independently accepted dated ownership/
  site evidence. Current-company scopes retain
  historical_business_window_known=false. A matching full CNPJ cannot supply a
  missing historical business interval or relabel today's operator/address.

In the filter sheet, unsupported fields are disabled with a short reason; an
unsupported deep link/query gets UNSUPPORTED_FILTER, never an ignored filter.
No-source/unknown/failed lookup cannot select a supposedly safe/clean station.
Known non-retail agents stay separate from a station-only list; unknown regulated
segment is not presumed to be a fuel retailer. Validate source/type combinations
even when each token separately passes the shape schema.

## Source record identity, clocks and counting

ASSAY, ENFORCEMENT_ACTION, REPORTED_INFRACTION and FINE_RECORD remain separate.
An inspection/action is not an automatically established infraction; a fine is
not an adulteration finding. Preserve source-reported result/status without
turning it into severity, guilt, an EVSE violation or a new numeric trust score.
Private community reports are absent from public official counts and schemas.

Identity is source + stream + frozen revision + stable source-record key, with
the accepted source's identity_kind. CONTENT_OBSERVATION is not SOURCE_ISSUED_CASE.
Missing issued case ID remains null. A document/procedure can contain several
distinct results: keep each accepted source record. Never collapse on the shared
document reference, company or address, or count repeated snapshots as newly
issued fines. The reviewed example has four rows across two snapshots, two
distinct results sharing a document, and a selected-revision record count of2;
the issued-fine count is UNKNOWN, not4 or2.

A national filtered aggregate selects one revision per source/stream. Duplicate
revision contexts are invalid, including two revisions of the same stream.
Historical revision comparison is a separate explicitly labelled journey, not
an additive total. Source record count, associated station count and unmatched
record count have distinct meanings and denominators. The source corpus is not
an inventory of all offences/stations in Brazil.

Clocks preserve basis and precision: EVENT_DAY, DUE_DAY, EXTRACTION_DAY,
EXTRACTION_MONTH, COLLECTION_LOCAL_DATETIME, SOURCE_REPORT_YEAR,
PUBLICATION_INSTANT and ASSOCIATION_DAY. An unzoned source collection clock
cannot acquire Z/UTC by assumption. Year/month precision must not acquire an
invented day. Retain the existing source's original clock/provenance evidence;
these generic examples do not replace FQ's actual typed source fields.

Date filters use explicit from-inclusive/to-exclusive boundaries and their
matching DAY/MONTH precision. Validate calendar values and from < to in the
owning application; shape validation alone is insufficient. Unknown event dates
do not match an event-date filter based on due/extraction/publication values.
Dates refer to source facts; generation/save/refresh/association cannot make an
old source or price fresh. Source-specific FQ staleness rules stay authoritative.

## Result, cache and pagination semantics

AVAILABLE means selected source records can be shown; EMPTY means zero in the
selected available source/revision/filter, never proof of safety or no offences.
UNKNOWN and UNAVAILABLE expose a reason with null counts and no invented rows.
For this v1 aggregate, any selected required source that cannot be read prevents
a complete count: return UNAVAILABLE, or invite an explicit narrower selection.
Do not silently return a partial set as the complete requested national result.

Source-global records stay accessible independently of a Directory association.
Company evidence unavailable must not fall back to source-global rows on that
station. Unknown linkage is separate from absence of source records. A station
without a current price can still have Directory details and official history.

The source revision vector, exact normalized filters (including condition/program,
metric, date/precision, source/current geography), schema/semantic policy, page
order and association fingerprint bind keysets/ETags/cache scope. Set-valued
filters canonicalize deterministically; values, null and unsupported are distinct.
Changing any relevant input invalidates the old cursor/representation; ask for
restart with loaded state retained. Filter/group before pagination in the backend.
No ingestion/provider fan-out happens in a public request.

Fresh Directory guards are required before scoped positive rows and any 304;
MEX-18/19 own actual read-lock/HTTP proof. Offline cached company facts use
LAST_CHECK_ONLY and last_checked_at, a visible inability to reverify, and no
current verified badge. A local cache hit/304 never renews source validity.
Owner/proof/financial responses remain no-store; public persistence requires
OPT's accepted caching contract. Failure preserves dated retained data without
claiming a successful check or replacing a valid-empty snapshot with an error.

## Android journeys and reviewed Portuguese copy

Reuse Explore, current station detail, filter sheet, navigation and shared
Compose/Room/WorkManager components; this task does not add a bottom tab or
shipping resources. Technical schema names stay out of the normal product flow.

1. Explore → type a supported question → inspect chosen meaning → clarify
   source/product/period/metric if missing → ranked page → station/source detail.
   Candidate labels: "Maior preço observado" and "Maior média por estado".
2. Explore → "Registros da ANP" → choose type/source/period/geography → paged
   source results, including unmatched records → source/provenance detail.
   Labels distinguish "Análises de qualidade", "Ações de fiscalização",
   "Infrações informadas pela fonte" and "Registros de multas".
3. Station list/detail → eligible occurrence filters → grouped station cards →
   distinct records/timeline. Current-company-only copy: "Associado à empresa
   atual; não comprova ocorrência neste endereço". No-price copy: "Sem preço
   atual disponível"; it does not remove the station from official exploration.
4. Empty: "Nenhum registro encontrado nas fontes e no período selecionados".
   Unavailable: "Fonte indisponível para esta consulta". Unsupported filter:
   "Filtro não disponível nesta fonte". Offline company cache: "Dados salvos;
   vínculo atual não verificado. Última verificação: …". Never "posto limpo",
   "sem infrações", "combustível seguro" or a paid-removal action.
5. Keep manual refresh and meaningful source age. Remove fixed "a cada15s"
   wording through the owning OPT implementation/reconciliation; do not claim
   source freshness from a periodic spinner. Preserve loaded page/scroll state,
   accessible controls, large fonts, TalkBack and all affected locales.

All official/price exploration remains free and independent of registration,
GPS and Premium. Private vehicle/ledger/report/cloud journeys stay under PRM
and registration v2. D proof/receipts/business portal and iOS remain deferred.

## Implementation acceptance still owed

Shape tests (54 examples), three independent counting examples and documented
counterexamples freeze compatibility only. Semantic applicability, actual
calendar/catalog checks, SQL/locking, query bounds, cursor privacy, loss/census,
source correction/withdrawal, exact aggregates and Android wire/state/accessibility
must pass MEX-17–32 and their FQ/OPT/EE prerequisites. No unsupported source or
current-company badge becomes available merely because its shape is documented.

# External references checked

Status: research of 2026-11-10. Facts that this plan takes from outside the repository, each with its source and the date it was read. They are inputs to decisions, not decisions, and must be rechecked when a task that depends on them starts. Items marked `UNVERIFIED` could not be confirmed on a primary source in this research. Nothing here is legal advice.

## 1. Android: location during a work trip

| Fact | Source (read 2026-11-10) |
|---|---|
| From Android 14 every foreground service declares a type; the `location` type needs the `FOREGROUND_SERVICE_LOCATION` permission plus coarse or fine location granted at runtime. | developer.android.com/develop/background-work/services/fgs/service-types |
| A location foreground service must be started while the app has a visible activity. Started that way it keeps receiving location with the app in the background or the screen off, and `ACCESS_BACKGROUND_LOCATION` is not needed. | developer.android.com/develop/sensors-and-location/location/permissions; …/services/fgs/restrictions-bg-start |
| Starting a location foreground service from the background (for example from a boot receiver) needs background location. A trip therefore cannot resume recording silently after a reboot, a task-manager stop or a force-stop; the user must act. | developer.android.com/develop/background-work/services/fgs/restrictions-bg-start; …/fgs/handle-user-stopping |
| No time limit is documented for the `location` service type (limits exist for `dataSync`, `mediaProcessing` and `shortService`). | developer.android.com/develop/background-work/services/fgs/timeout |
| On Android 12 and later the user can grant approximate location only; the app detects it by requesting fine and coarse together and reading which was granted. Approximate is about 3 km². | developer.android.com/develop/sensors-and-location/location/permissions/runtime |
| On Android 13 and later the notification permission is not required to run a foreground service, but when it is denied the service notice appears only in the task manager. | developer.android.com/develop/ui/views/notifications/notification-permission |
| Play requires a declaration (with a video) for each foreground-service type used by apps targeting Android 14 or later. Background location needs a separate declaration form and prominent in-app disclosure. | support.google.com/googleplay/android-developer/answer/13392821; …/answer/9799150 |
| Since 31 August 2026 new apps and updates on Play must target API level 36; Android 17 (level 37) adds a location button, a persistent location indicator, a local-network permission and stricter background audio. | developer.android.com/google/play/requirements/target-sdk; developer.android.com/about/versions/17/behavior-changes-17 |
| A `Location` exposes accuracy, speed, bearing, elapsed-realtime and a mock flag (`isMock()`). | developer.android.com/reference/android/location/Location |
| The platform geofencing API allows 100 geofences per app and needs background location when targeting API 29 or later. | developer.android.com/develop/sensors-and-location/location/geofencing |
| Doze suspends network and defers jobs when the device is stationary and unplugged; Play restricts asking for a battery-optimisation exemption. | developer.android.com/training/monitoring-device-state/doze-standby |
| Design guidance for drivers: minimal information per screen, short flows. | developers.google.com/cars/design/create-apps/apps-for-drivers/design-tasks |

`UNVERIFIED`: whether stock Android keeps a foreground service alive after the app is swiped away on every device; any official statement on manufacturer-specific background killing; whether a foreground service is exempt from Doze network restrictions; official battery-drain figures for continuous GPS (none found).

Consequence used in the plan: tracking starts from the visible trip screen in a location foreground service; no background-location permission; interruptions are recorded as gaps and the user is asked to resume; geofences are evaluated on the server from received fixes.

## 2. Maps, routing and navigation options

| Option | What it offers | Cost and terms found | Source (read 2026-11-10) |
|---|---|---|---|
| Engine already in the app (CoMaps-derived, ADR-022) | Offline vector map, search, car routing, voice guidance on Android | Already built; packages hosted by the project; Apache-2.0 core | Repository: `maps/engine/`, `docs/adr/022-comaps-derived-native-map-engine.md` |
| MapLibre GL JS (web) and MapLibre Native | Open map rendering; needs a tile source | BSD licences; no usage fee | github.com/maplibre/maplibre-gl-js; github.com/maplibre/maplibre-native |
| Project-hosted vector tiles (single-file archive served by range requests) | Tile source for the web map with no tile server | Planet archive about 120 GB; a Brazil extract is estimated at 3 to 5 GB (`UNVERIFIED` estimate); Brazil OSM extract is 2.0 GB | docs.protomaps.com/pmtiles; docs.protomaps.com/basemaps/downloads; download.geofabrik.de/south-america/brazil.html |
| OpenFreeMap public instance | Free vector tiles, no key | No service-level commitment; attribution required | openfreemap.org |
| OpenStreetMap public tile servers | Raster tiles | Best effort; bulk and offline download prohibited; commercial access may be withdrawn | operations.osmfoundation.org/policies/tiles |
| Public Nominatim | Geocoding | At most one request per second; systematic queries discouraged; vehicle-tracking services are told to run their own instance | operations.osmfoundation.org/policies/nominatim |
| Self-hosted Valhalla | Routing with truck costing, map matching, isochrones, pt-BR instructions | MIT; tiled design with modest memory | github.com/valhalla/valhalla; valhalla.github.io/valhalla/api/route/api-reference |
| Self-hosted OSRM | Routing, matching, table | BSD-2; memory grows with the extract (planet about 123 GiB; Brazil estimated 4 to 5 GiB, `UNVERIFIED`) | github.com/Project-OSRM/osrm-backend |
| Self-hosted GraphHopper | Routing with custom profiles, map matching | Apache-2.0 engine; hosted plans paid | github.com/graphhopper/graphhopper; graphhopper.com/pricing |
| Google Maps Platform | Maps SDK for Android free; Routes from USD 5 per 1 000; Navigation SDK USD 25 per 1 000 destinations after 1 000 free | Terms forbid storing content beyond stated limits (30 days for coordinates from several services), combining with non-Google maps, and building turn-by-turn from Directions plus Maps SDK; Navigation SDK and Maps SDK cannot share an app | developers.google.com/maps/billing-and-pricing/pricing; cloud.google.com/maps-platform/terms; cloud.google.com/maps-platform/terms/maps-service-terms |
| Mapbox | Maps, Directions, Navigation SDK (per user and per trip) | Applications primarily for use in vehicles need a separately purchased licence; results may not be cached; data processed in the United States | mapbox.com/pricing; mapbox.com/legal/tos; mapbox.com/legal/product-terms |
| HERE, TomTom | Tiles and routing with monthly free quotas | Fleet and asset licences are sales-led; paid prices not all public | here.com/get-started/pricing; docs.tomtom.com/pricing |
| Deep links to Google Maps or Waze | Hand-off of a destination | No callback: trajectory, arrival and reroutes are not returned to the app | developers.google.com/maps/documentation/urls/get-started; developers.google.com/waze/deeplinks |

OpenStreetMap licence notes: attribution to OpenStreetMap must be visible on maps; share-alike applies to a publicly used derivative database, not to internal use, and a company's own GPS data kept beside OSM data is a collective database (osmfoundation.org/wiki/Licence/Attribution_Guidelines; osmfoundation.org/wiki/Licence/Licence_and_Legal_FAQ). No guideline addresses map-matched traces specifically (`UNVERIFIED`); the plan keeps raw fixes as the source of truth.

Sampling practice: Google's driver SDK reports every 10 seconds by default with a 5-second minimum (developers.google.com/maps/documentation/mobility/driver-sdk/on-demand/android/vehicle-ready). Byte sizes per fix and per row used in [09-architecture](09-architecture.md) §9 are estimates.

## 3. Fiscal documents

| Fact | Source (read 2026-11-10) |
|---|---|
| NF-e (model 55) and NFC-e (model 65) exist only digitally; validity comes from the issuer's signature and the authorisation of use; the printed DANFE is auxiliary. | CONFAZ, Ajuste SINIEF 07/05 and Ajuste SINIEF 19/16 (confaz.fazenda.gov.br/legislacao/ajustes) |
| Access key: 44 positions = state code (2) + year and month (4) + issuer CNPJ (14) + model (2) + series (3) + number (9) + emission type (1) + numeric code (8) + check digit (1). With alphanumeric CNPJ the key pattern is six digits, twelve alphanumerics, twenty-six digits; the check digit is modulo 11 with each character valued as its ASCII code minus 48. | Nota Técnica Conjunta 2025.001 (listed at nfe.fazenda.gov.br; read from a mirror) |
| Check-digit weights 2 to 9 from right to left, digit = 11 − remainder, 0 when the remainder is 0 or 1. | `UNVERIFIED` on the primary manual; secondary source only |
| NFC-e QR code has three versions. In versions 2 and 3 the parameter `p` starts with the access key, then the version, then the environment; version 3 online carries only those three. Base URLs differ by state. | NT 2025.001 v1.02 (read from a mirror) |
| The public "Consultar NF-e" page asks for the key and an image captcha meant to stop automated queries. | nfe.fazenda.gov.br (consulta resumo page) |
| The distribution web service (NFeDistribuicaoDFe) serves only parties to the document (recipient, carrier, authorised third party) using their own ICP-Brasil certificate whose CNPJ root matches; model 55 only; 90-day window; the recipient gets a summary until it registers a manifestation; hourly limits apply and violations block the CNPJ for an hour. | NT 2014.002 v1.40 (read from a mirror) |
| NFC-e consultation is offered by the tax authority of the issuer's state by key or QR code for at least 180 days. No national or certificate-free programmatic interface for third parties was found. | Ajuste SINIEF 19/16, cláusula 17ª |
| Cancellation: NF-e within 24 hours of authorisation when no circulation occurred; NFC-e within 30 minutes; a correction letter cannot change values or parties and does not exist for NFC-e. A stored XML is a snapshot and is never updated. | Ajuste SINIEF 07/05 cl. 12ª and 14ª-A; Ajuste SINIEF 19/16 cl. 15ª |
| The schema rule comparing product value with quantity × unit value tolerates R$ 0.01 either way and is optional per state. | NT 2011/004 |
| Field precision: quantity up to 4 decimals, unit value up to 10, product value 2. | `UNVERIFIED` in this research (from prior knowledge of the manual) |
| Recipients keep fiscal documents for the period set by tax law; anchors are CTN art. 173 (five years) and art. 195. | planalto.gov.br/ccivil_03/leis/l5172compilado.htm |
| SERPRO sells an official "Consulta NF-e" interface to companies with a certificate; coverage of NFC-e and price were not confirmed. | gov.br service page; `UNVERIFIED` details |
| CT-e (model 57) and MDF-e (model 58) are fiscal documents of transport services and cargo manifests; a fleet logbook is neither. | CONFAZ Ajuste SINIEF 09/07 and 21/10 |

## 4. CNPJ and station register

| Fact | Source (read 2026-11-10) |
|---|---|
| CNPJ becomes alphanumeric for new registrations from July 2026: 14 positions, the first 12 alphanumeric, two numeric check digits; existing numbers do not change; "0001" no longer identifies a head office definitively. | Receita Federal questions and answers on the alphanumeric CNPJ (gov.br/receitafederal), IN RFB 2.229/2024 |
| Check digits: modulo 11 with character value ASCII − 48; weights 5,4,3,2,9,8,7,6,5,4,3,2 for the first digit and 6,5,4,3,2,9,8,7,6,5,4,3,2 for the second; official test value `12.ABC.345/01DE-35`. | Same source |
| ANP publishes a daily register of authorised fuel retailers with authorisation number, legal name, CNPJ, address and brand, and an interface with the same data. It names no partners or representatives. | gov.br/anp open-data page for "revendedores varejistas de combustíveis automotivos" |
| The only licence statement found on the ANP site is a site-wide Creative Commons Attribution-NoDerivatives 3.0 notice; no dataset-specific licence was found. | Same page |
| ANP requires pump and panel prices with two decimals (three were used until May 2022). | ANP price FAQ citing Resolução ANP 948/2023 art. 22 |

## 5. Labour and privacy

| Fact | Source (read 2026-11-10) |
|---|---|
| Lei 13.103/2015 lists "diário de bordo" among the instruments by which a professional driver's journey is recorded, at the employer's choice. | planalto.gov.br/ccivil_03/_ato2015-2018/2015/lei/l13103.htm |
| Portaria MTP 671/2021 defines three kinds of electronic time registration (REP-C, REP-A, REP-P) with technical and registration requirements; only such a system produces the legal time file. | gov.br/trabalho-e-emprego (REP pages and compiled Portaria) |
| A labour court accepted phone geolocation as digital evidence of working hours, limited to the alleged hours. | tst.jus.br news of 17 May 2024 |
| LGPD legal bases include legal obligation, contract performance and legitimate interest; consent is revocable at any time; legitimate interest does not apply to sensitive data. | planalto.gov.br/ccivil_03/_ato2015-2018/2018/lei/l13709.htm |
| ANPD published a guide on legitimate interest with a three-step test (purpose, necessity, balancing). | gov.br/anpd, guide released 2 February 2024 |
| Controllers notify the authority and data subjects of relevant incidents within three business days. | Resolução CD/ANPD 15/2024 (read through ANPD pages) |
| International transfers use standard contractual clauses. | Resolução CD/ANPD 19/2024 |

`UNVERIFIED`: any ANPD guidance specific to worker geolocation (none found); full texts of the ANPD resolutions (read through summaries).

## 6. Billing, store policy and storage

| Fact | Source (read 2026-11-10) |
|---|---|
| Google Play requires its billing system for in-app purchase of digital features and services and lists business and productivity software among the examples; no exemption for enterprise sales appears. A service consumed only outside the app, and "consumption-only" apps that do not sell in the app, are allowed; apps may not lead users to another payment method. | support.google.com/googleplay/android-developer/answer/9858738 |
| Distribution outside Play is not subject to Play billing. | support.google.com/googleplay/android-developer/answer/10281818 |
| Provider fees published (Brazil): Efí — Pix 1.19 %, boleto R$ 3.45, card 3.49 % at sight, automatic Pix R$ 3.50 per settlement; Stripe — card 3.99 % + R$ 0.39, boleto R$ 3.45, Pix 1.19 % by invitation, billing 0.7 % of volume; Asaas — fixed fees per received Pix or boleto (R$ 2.15 and R$ 1.08 shown, labels not captured), native service-invoice issuance; Pagar.me — Pix 0.99 %. | sejaefi.com.br/tarifas; stripe.com/br/pricing; asaas.com/precos-e-taxas; pagar.me/precos |
| Automatic Pix: the receiving institution may charge the receiver; the payer is not charged. | Banco Central implementation material |
| Cloudflare R2: USD 0.015 per GB-month standard storage; Class A operations USD 4.50 per million; Class B USD 0.36 per million; no egress fee. | developers.cloudflare.com/r2/pricing |
| Backblaze B2: from USD 6.95 per TB-month; free egress up to three times stored volume. | backblaze.com/cloud-storage/pricing |
| Fleet software reference: Fleetio lists USD 4, 7 and 10 per vehicle per month on annual billing. Brazilian platforms checked (Ticket Log, Cobli, Infleet, Gestran) publish no prices. | fleetio.com/pricing and the vendors' sites |

`UNVERIFIED`: webhook de-duplication features of any provider (the plan de-duplicates on the provider's event identifier itself); Mercado Pago and iugu fees (secondary sources); Brazil's place in Google's newer billing-choice programme.

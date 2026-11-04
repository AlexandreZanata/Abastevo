# Charging in Mato Grosso's ten largest municipalities

B-BR-EVC-MT10-01. Rank Mato Grosso municipalities by the latest IBGE 2026
population release. Admit publicly documented charging facilities inside the
ten municipality polygons. Keep source and location evidence, group duplicate
reports by facility, and preserve the two Sorriso public place IDs. Unknown
access, connector counts and operating status stay unknown.

BUC-EVC-MT10-01. Produce an auditable edition with per-city counts and source
decisions. Load and replay on disposable PostGIS, refuse another target or a
conflicting edition, and verify the public staging census and Android map.
Preserve the previous Sorriso snapshot and the independent national profile.

The scope is fixed by the [IBGE October 2026 revision](https://ftp.ibge.gov.br/Estimativas_de_Populacao/Estimativas_2026/POP2026_20261029.pdf)
and the pinned municipality mesh. The dated extract and operator evidence
describe partial coverage of real installations, with no real-time claim.

## Reviewed inventory

The 2026-10-29 IBGE publication provided 142 unique Mato Grosso
municipalities. The reviewed census has 26 facilities. The regional
edition adds 24 and the original Sorriso edition supplies the other two.
The later [Sorriso supplement](SORRISO_CHARGING_SUPPLEMENT.md) adds four
more, bringing the combined staging census to 30. The later [Cuiabá supplement](CUIABA_CHARGING_SUPPLEMENT.md)
adds 30 more, bringing the combined staging census to 60.

- Cuiabá — 9
- Várzea Grande — 3
- Rondonópolis — 4
- Sinop — 2
- Sorriso — 2, retained with their published IDs
- Tangará da Serra — 2
- Lucas do Rio Verde — 1
- Primavera do Leste — 1
- Cáceres — 1
- Barra do Garças — 1

The [reviewed regional data](../../../../data/src/main/resources/charging/mt-top10-pilot-v1.json)
contains the rank and population, WGS84 facility point, address, source links,
and the precise population and boundary hashes. Its database offer is
[ODbL 1.0](https://opendatacommons.org/licenses/odbl/1-0/). It credits
[OpenStreetMap contributors](https://www.openstreetmap.org/copyright) and the
Open Charge Map providers Easy Volt and YellotMob (CC0 for the imported
locations). The earlier [source rights receipt](EVC-06.md#real-catalog-replay-and-source-identity-correction)
documents the publisher releases. No external directory was copied as a
catalog. A named charging installation, a public source or independent
firsthand evidence, and a corroborated facility location were reviewed
together.

The approved open-source snapshot supplied 19 candidate records across
five cities. The review retained named and corroborated facilities and
supplemented them from operators, venue records and firsthand evidence.
Nearby records at the same Pantanal Shopping facility and the two records
for the Estocolmo dealership were reduced to one facility apiece.
For Posto Alvorada in Primavera do Leste, the Volvo address, ANP
establishment and a recent check-in agree with the published location.
The old OCM point was about 180 m from the reviewed point. A second
directory placed the same facility several kilometres away and was
excluded. The SAU 14 candidate geocoded to Nova Mutum and was withheld
from Lucas do Rio Verde. Hotel Mandino has guest access only, recorded
in its display label. Employee-only Sicredi and residential leads were
withheld. The closest retained pairs, Ibis and Tangará Shopping (151 m),
and Estocolmo and GWM Canopus (212 m), identify distinct premises.

Public directories disagree on coverage. For Cuiabá, the reviewed sources
listed [45](https://www.ondecarregar.com.br/eletropostos/mt/cuiaba/),
[60](https://recarregados.com/eletropostos/mt/cuiaba/) and
[79](https://vatts.com.br/eletropostos/mt/cuiaba/) candidates. Those totals
include differing definitions of access and source records. They do not
establish a denominator of operating facilities. Uncorroborated pins, unnamed
records, uncertain access and conflicting sites still need source review.

## Reproduction and staging

Render the SQL into a private work directory with the same IBGE mesh.

```sh
python3 scripts/render-mt-ev-pilot.py --boundaries "$PILOT_DIR/municipalities.geojson" > "$PILOT_DIR/seed.sql"
```

Run it with `psql -v ON_ERROR_STOP=1` on disposable PostGIS or the authorized
staging connection, with `abastevo.sorriso_pilot_target` set to `test` or
`staging`. The loader checks the pinned polygons, source links and stable IDs.
It locks the profile and verifies the exact rows inside one transaction.
Use an explicitly reviewed new edition if the dataset changes. Retain the
original Sorriso profile and IDs when comparing snapshots.

The staging API returned 24 new details matching every reviewed place field,
and both previous Sorriso details. Its national profile remained
empty. The deployed API still answers HTTP 400 for the two list profiles.
The staging Android build uses the admitted coverage and per-ID detail
routes while that API is running. The regional fallback bounds concurrency
to six requests, checks the exact edition and IDs, and preserves cached
data during failures. The future paginated response must contain the same
regional ID set before Android commits a replacement.

The renderer produced identical SQL twice. Disposable PostGIS accepted
two concurrent executions with one batch and 24 places. It rejected a
missing target and a changed edition, preserving the admitted rows.
The charging HTTP integration suite passed with race detection, and Go
vet passed. All 44 focused Android tests, debug lint and APK assembly
passed. The debug APK SHA-256 is
`f3129aa4d79d5863c7e6fb3225e4fc7532bfcae99d662ea76380115af7a5423d`.
On a physical Android 16 device, the updated app loaded the regional
snapshot, switched to Cuiabá, found FORBS Santa Cruz II in the map
search and opened its charging card.

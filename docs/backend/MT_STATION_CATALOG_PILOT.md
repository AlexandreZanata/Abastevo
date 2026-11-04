# Mato Grosso station catalog pilot

B-BR-D01/D02/D03/D05/D06/D10/D12 and BUC-D01/D05/D06/D08 apply to the
official fuel registry. This pilot uses the complete dated ANP fuel CSV as its
denominator, reconciled by full establishment CNPJ with the ANP API and the
Directory. Preserve canonical IDs and recorded facts. Keep authorized stations
searchable even with no price or verified point. A missing registry row never
closes a station. A municipality name variant may resolve only through a
reviewed state and IBGE code, without fuzzy merging.

The municipal map reads the whole bounded, paginated Directory catalog. Pins
require a supported location tier. A station with no usable coordinate stays
in the municipality list. If a page fails, the map may use the last complete
cache for that city.

Community reports belong to the existing private, authenticated correction
flow under B-BR-COR-01–08. A report alone cannot change official ANP data or
remove a canonical identity. The operator reviews the proposal and retains an
audit trail. Charging sites belong to the separate EVC catalog and need
licensed source evidence.

## Verified Mato Grosso coverage, 2026-11-04

The ANP registry edition dated 2026-11-03 and the ANP API response retrieved on
2026-11-04 agreed on all 1,179 establishment CNPJs. The
[municipality census and input hashes](evidence/mt-station-catalog-20261104.json)
record counts across all 142 IBGE municipalities. The ANP source writes
“Santo Antônio do Leverger”; the [IBGE municipality 5107800](https://www.ibge.gov.br/cidades-e-estados/mt/santo-antonio-de-leverger.html)
is “Santo Antônio de Leverger.” Matching these names by the official code
restored all eight missing establishments. Three other Directory CNPJs absent
from this edition remained untouched.

Staging had 1,171 official establishments and 1,072 pinned before the pilot.
After publication it had all 1,179, with 1,077 pinned and 102 available in the
city list only. The source had 1,068 coordinate pairs: 1,052 were inside the
registered municipality and 16 were outside. Of ten previously unpinned
establishments with ANP positions, five met the boundary gate. The pilot
published those five and retained the existing location tiers for all previously
pinned stations. Further coordinates need documented source and review.

The full staging job reached its time limit after one missing station had
published. The exact remaining seven were loaded and verified through
`abastevo-mt-catalog-delta-retry-20261104`. Its first attempt stopped before
database access because the job user could not enter the input directory. The
successful retry preserved the original receipts, and the location replay
returned five unchanged rows. The live public API returned all 1,179 official
records across 12 pages, including 8 in Leverger (3 pins), 25 in Sorriso (25 pins)
and 149 in Cuiabá (147 pins, two pages). Readiness returned HTTP 200. These are
staging data results. The scheduled sync needs the corrected alias generator
in its next deployed artifact.

Validation covered the exact 1,179-record import and five-pin replay in
disposable PostGIS, the Directory integration suites with race detection,
Go vet, deterministic audit replay and truncated input rejection. The Android
debug build and lint passed along with 45 selected unit tests, including
municipal pagination and the authenticated correction wire contract. The APK
was installed on a physical device, where the Sorriso map, station profile
and all three correction options opened. The form was closed without sending
a real report. Both staging charging profiles returned an empty first page.

## Repeatable workflow

1. Obtain the dated [ANP registry CSV](https://www.gov.br/anp/pt-br/centrais-de-conteudo/dados-abertos/dados-cadastrais-dos-revendedores-varejistas-de-combustiveis-automotivos),
   all ordered pages from the [ANP Revendedores API](https://revendedoresapi.anp.gov.br/swagger/index.html)
   for the same UF, the [IBGE municipality list](https://servicodados.ibge.gov.br/api/docs/localidades)
   and an approved IBGE municipality mesh. Use a private working directory
   outside the checkout. Record the SHA-256 of each input and the retrieval
   timestamp. A public web directory can inform a manual review when an
   official coordinate is missing.
2. Save the IBGE list as `ibge.json` and generate the same alias reference used
   by the daily registry refresh. Run the audit with one `--api` per response
   file in page order. The audit refuses a truncated page sequence, mismatched
   ANP membership, unknown or ambiguous municipality, and Directory locality
   conflicts. `--directory` takes a private five-column TSV in this order:
   CNPJ, IBGE code, status, quality, and point flag (0 or 1).

   ```sh
   go -C backend run ./cmd/station-sync --reference-dir "$PILOT_DIR"
   python3 scripts/audit-station-uf.py --uf MT \
     --registry "$PILOT_DIR/anp.csv" --api "$PILOT_DIR/anp-api-first.json" \
     --aliases "$PILOT_DIR/aliases.json" --boundaries "$PILOT_DIR/municipalities.geojson" \
     --directory "$PILOT_DIR/directory.tsv" --output "$PILOT_DIR/audit"
   ```

3. Reconcile the complete `coverage.json` against the backend. Feed the scoped
   CSV and alias reference to the existing `prepare_registry` and
   `station-load --publish` commands. Run `station-load --verify` and compare
   CNPJ membership after publication. Verify authorization and address
   corrections with the existing Directory procedure. If the full edition
   reaches its execution budget, compare the live set again and prepare a
   bounded delta for remaining identities. Preserve the original receipts.
4. With the D1 location tier authorized for the operation, generate the
   optional `locations.ndjson` and its manifest with `--prepare-anp-pins`.
   These contain ANP-reported points inside their matching municipality for
   missing or unlocated stations. Run `station-locate --dry-run` before
   applying, then repeat it to verify replay. Keep existing pins, reviewed
   points and municipality disagreements out of the proposed changes. A
   usable address without a point stays in the city list.
5. Page through the public Directory, compare total membership and pins, and
   exercise a large municipality, a smaller municipality and a station
   without coordinates in Android. The app caps the city load at 25 pages
   of 100 and retains the complete city cache if a cursor repeats or the load
   fails. Use [registry recovery](REGISTRY_DAILY_SYNC.md) for operational
   retries. If a published record or point is disputed, retain the registry
   identity and supersede the affected field through the audited correction
   or recovery workflow. Repeat the census by UF before expanding publication.

## Community corrections and EV

An Android station detail can send name, address or permanent closure proposals
through the existing private correction endpoint. The app uses the canonical
station UUID and a stable submission key on retry. A station review may apply
an eligible location correction through the existing tier ladder. Publication
of accepted name, address and closure proposals remains a separate reviewed
projection. The official registry and historical station identifiers stay
available for audit.

The local OSM and OCM charging generation documents 25 sites in 11 MT
municipalities. Sorriso had zero in that generation. Public commercial
directories reported different totals and helped identify review leads.
The [charging rights and publication process](evidence/evc/EVC-06.md) owns
admission and deployment of those records. A statewide fuel snapshot leaves
the separate charging generation and its national activation gate independent.

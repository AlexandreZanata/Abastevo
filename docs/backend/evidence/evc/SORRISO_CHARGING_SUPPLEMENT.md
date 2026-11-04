# Sorriso charging follow-up

B-BR-EVC-SORRISO-02. Use Google for finding and cross-checking candidates.
Publish independently corroborated charging facilities and coordinates
inside IBGE municipality 5107925. Do not import Google Maps content.
Merge reports for the same facility. Leave a general-purpose power outlet
out of the charging map when dedicated equipment cannot be verified.
Distinguish charging location provenance from the selected price source.

BUC-EVC-SORRISO-02. Replay the supplemental batch on disposable PostGIS,
refuse the wrong target and changed edition, and preserve both previously
published Sorriso IDs. Check public staging details and open the new sites
on the physical Android map. Verify the open-source coverage notice on a
charging-only card while the ANP price tab is selected.

## Reviewed on 2026-11-04

The [versioned manifest](../../../../data/src/main/resources/charging/sorriso-supplement-v1.json)
contains four additional facilities. Google Maps helped locate candidate
businesses. The [Google Travel listing for Hotel Uipi](https://www.google.com/travel/hotels/entity/ChkIgKCU5K-4xeNbGg0vZy8xMW55dHR3X3pxEAE)
corroborated that property's charging amenity. Names and addresses were
checked against the operator, municipality or published business records.
Coordinates were derived independently of Google and tested against the
pinned IBGE geometry for Sorriso.

- **Machado Supermercados, Parque Universitário**. The [opening announcement](https://www.abras.com.br/clipping/redes-de-supermercados/118041/machado-supermercados-inaugura-primeira-loja-em-sorriso-mt)
  and [Procon record](https://www.procon.mt.gov.br/documents/d/procon/tabela-produtos-analisados-pascoa-final)
  place the store on Av. Noêmia Tonello Dalmolin. The approximate pin is the
  [mapped supermarket](https://www.openstreetmap.org/node/13190847895) near the
  [reported successful charge](https://carregados.com.br/estacoes/estacao-mercado-machado-663702).
  The Tancredo Neves listing did not establish a second charger.
- **DelMoro Supermercados, Centro Sul**. The [store directory](https://delmoro.com.br/)
  lists Av. Belo Horizonte, 94. The [mapped building](https://www.openstreetmap.org/way/766396829)
  is near the [successful charging report](https://carregados.com.br/estacoes/estacao-delmoro-supermercados-3173)
  with a different street address. The pin marks the approximate footprint
  center. Public access is unknown. A report mentions a store card.
- **Hotel Uipi**. The [municipality](https://turismo.sorriso.mt.gov.br/atrativos-turisticos/hospedagem/hotel-uipi)
  lists Rua Idemar Riedi, 13348. The [hotel footprint](https://www.openstreetmap.org/way/786899226)
  is near the [successful charging report](https://carregados.com.br/estacoes/estacao-hotel-uipe-941094).
  The pin marks the approximate footprint center. Public access is unknown.
- **Turia, Av. Santa Maria**. The [directory entry](https://recarregados.com/eletropostos/mt/sorriso/turia-epzbymrhr99h)
  and [successful charging reports under Solturi](https://carregados.com.br/estacoes/estacao-solturi-solar-1139797)
  agree on number 950 and the charging location. The edition uses one
  rounded pin and leaves operator and hardware specifications unset.

The [Imperial/Goldmen report](https://carregados.com.br/estacoes/estacao-hotel-imperial-sorriso-246456)
describes a general-purpose industrial outlet requiring portable equipment.
It remains outside this edition. Entries that only establish a business
registration, repeat a facility, or lack a usable address remain outside it.
Charging reports provide evidence of past use. Access conditions, equipment
and current availability can change. OpenStreetMap-derived locations in the
manifest carry [OpenStreetMap attribution](https://www.openstreetmap.org/copyright)
and are available under ODbL 1.0.

## Replay and public verification

Use the boundary pin and operator procedure in the
[regional pilot evidence](MT_TOP10_CHARGING_PILOT.md). Add the explicit
Sorriso option to render this edition.

```bash
python3 scripts/render-mt-ev-pilot.py --sorriso \
  --boundaries "$PILOT_DIR/municipalities.geojson" > "$PILOT_DIR/sorriso-supplement.sql"
```

The seed accepts an explicit test or staging target. It preserves the
regional batch and the original Sorriso batch. Updates and removals should
be reviewed against the cited facility and a new report before preparing a
replacement edition.

Two concurrent loads into disposable PostGIS produced one supplemental
batch and four places. A wrong target and a changed manifest were rejected.
The two original Sorriso records and the 24 regional records remained.
The charging HTTP integration suite with race detection and Go vet passed.

On 2026-11-04 the staging API returned the four supplemental details,
matching the reviewed IDs, names, coordinates and access flags. Both
original Sorriso detail routes still responded. The deployed API returns
HTTP 400 for the supplemental paginated route. The staging Android build
uses the exact edition and count from coverage and retrieves only the four
reviewed IDs through detail routes. The paginated endpoint is in PR #188.

All 45 focused Android tests, debug lint and APK assembly passed. The
instrumentation regression was compiled. On a physical Android 16 device
the Sorriso map search opened the charging cards for Machado, DelMoro,
Hotel Uipi and Turia. After reinstalling the final APK, Machado's card
showed the open-source coverage notice while retaining the ANP price
tab. The final debug APK SHA-256 is
`386c4cb0146b3092a647522ae553f205660ba3661c0e027b7e2e6bb6b50019b8`.

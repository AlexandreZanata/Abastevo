# Cuiabá charging follow-up

B-BR-EVC-CUIABA-01. Use Google for finding and cross-checking candidates.
Admit charging facilities with independent evidence and a location inside
IBGE municipality 5103403. Preserve the nine published Cuiabá IDs, merge
reports about the same facility, and keep access and current operating
status unknown where the evidence does not establish them.

BUC-EVC-CUIABA-01. Replay the reviewed edition on disposable PostGIS with
wrong-target and changed-manifest refusal. Verify the new details and the
nine existing places on the staging API. Open new charging cards on the
physical Android map after loading the updated staging snapshot.

## Reviewed on 2026-11-04

The [Cuiabá manifest](../../../../data/src/main/resources/charging/cuiaba-supplement-v1.json)
records 30 additional facilities alongside the nine original Cuiabá
records. It includes per-place source links, access flags, approximate
WGS84 coordinates and dates of successful public check-ins when available.
The combined staging census is 39 in Cuiabá, six in Sorriso and 60 across
the ten reviewed municipalities.

1. Use [Google Maps search](https://www.google.com/maps/search/eletropostos+Cuiab%C3%A1+MT/)
   for discovery and identity corroboration. Piloto at Posto Gontijo,
   Joy Energy, HRC and Rede Loja appeared in that search. The manifest
   draws its coordinates from independently reviewed public evidence.
2. Review [Carregados city candidates](https://carregados.com.br/estacoes?cidade=cuiab%C3%A1&estado=mato+grosso+%28mt%29)
   and compare the observed installations with
   [Recarregados](https://recarregados.com/eletropostos/mt/cuiaba/), venue
   directories and OpenStreetMap. An additional directory carrying the
   same coordinates does not create a second observation. Treat a success
   as evidence of use on that date. Leave live status, tariffs and
   connector details unset.
3. Compare candidate addresses and coordinates against the existing
   regional manifest, then verify municipality 5103403 with the pinned
   IBGE polygons. Round the source point to five decimals. Use named
   OpenStreetMap facility geometry where the independent charging report
   supports the match. The manifest carries
   [OpenStreetMap attribution](https://www.openstreetmap.org/copyright)
   and its data offer is ODbL 1.0.

### Decisions that affected admission

- The older AC listing and the newer Zon listing at
  [Shopping 3 Américas](https://carregados.com.br/estacoes/estacao-shopping-3-americas-cuiaba-2073427)
  describe one shopping facility. The [shopping footprint](https://www.openstreetmap.org/relation/10253982),
  [Ultra Academia](https://www.openstreetmap.org/way/1503614065) and
  [Galeria Itália](https://www.openstreetmap.org/way/162805784) establish
  distinct sites. Their charging reports point to three different venues.
- Keep Atacadão Tijucal separate from the previously published Atacadão on
  Miguel Sutil. The [Tijucal retailer page](https://www.atacadao.com.br/loja/cuiaba-tijucal)
  resolves the street address. The [Comper branch directory](https://www.comper.com.br/3049/nossas-lojas)
  distinguishes the four branches in the supplement.
- Use the public installation names for HRC and Despraiado. HRC's
  street numbers differ by source. Despraiado appears under Igreen,
  Helicharge and Tupi. Their positions match named reports and mapped
  charging equipment. The manifest avoids asserting current operation or
  a single network for Despraiado.
- Rede Loja has a mapped dedicated charging node, a corroborating listing
  and a venue match. Access and check-in date are unknown. Fiemt, Hotel
  Taiamã and Nativas Grill have older charging observations. Their access
  is also unknown. Cia Charge has successful and failed reports. Access
  and current operation remain unknown. The [Enerzee launch release](https://www.meiofiltrante.com.br/Noticia/132781/enerzee-inaugura-o-primeiro-eletroposto-da-enerzee-recharge-em-cuiaba)
  places the new installation at the same Jardim Paulista property.
- Preserve the nine earlier IDs. Skip candidate duplicates at Pantanal
  Shopping, Estação Cuiabá and Atacadão Miguel Sutil. Keep unidentified
  chargers, residence listings, employee-only points, the unresolved
  BMW/ECCODubai address pairing and leads without enough independent
  installation evidence out of this edition. Counts from directories
  mix facilities, individual charge points and restricted sites.

## Replay

Use the [regional pilot procedure](MT_TOP10_CHARGING_PILOT.md) and the
same pinned IBGE file. Select Cuiabá explicitly.

```bash
python3 scripts/render-mt-ev-pilot.py --cuiaba \
  --boundaries "$PILOT_DIR/municipalities.geojson" > "$PILOT_DIR/cuiaba-supplement.sql"
```

Review later changes against the stable facility ID and current evidence.
Prepare a separate reviewed edition for updated facts, disputes or removal.

The renderer produced identical SQL on replay. Two concurrent loads on
disposable PostGIS produced one batch and 30 places. A wrong target and
changed manifest were refused. The earlier 24 regional, four supplemental
Sorriso and two original Sorriso records remained. The charging HTTP
integration suite with race detection and Go vet passed.

Staging coverage returned 30 supplemental places. Public detail reads
matched all 30 new and nine existing Cuiabá IDs, coordinates, names and
access flags. The deployed API still returns HTTP 400 on the supplemental
paginated route. The Android staging build uses coverage and the reviewed
IDs on the existing detail endpoint. The paginated route is part of
protected PR #188.

All 46 focused Android tests, debug lint and APK assembly passed. The
physical Android 16 test opened the Piloto Posto Gontijo, Joy Energy
Parque Ohara and ChargeOn Atacadão Tijucal charging cards. The Gontijo
search also showed the separate ANP fuel record. The test APK SHA-256 is
`dc39257fa6b87cd95b911f8425171888ead6e43955074b2275ee91eeafb4719c`.

# Charging in Mato Grosso's next ten municipalities

B-BR-EVC-MT20-01. Use positions 11 through 20 from the same pinned
2026 IBGE population estimate as the first regional batch. Use Google
for discovery and cross-checking. Accept only corroborated charging
facilities located inside each municipality. Preserve previous IDs.
Leave access, connectors and current operation unknown where evidence
does not establish them. Explicitly account for municipalities without
an admitted place.

BUC-EVC-MT20-01. Verify all municipalities and reviewed locations against
the pinned boundary mesh. Replay the new batch on disposable PostGIS,
reject another target and changed content, and preserve the previous
batches. Verify the public staging details and representative cards in
the physical Android map.

## Scope and results

The [IBGE estimate revised on 2026-10-29](https://ftp.ibge.gov.br/Estimativas_de_Populacao/Estimativas_2026/POP2026_20261029.pdf)
has 142 Mato Grosso municipalities. Sort the MT rows by estimated
population and select positions 11 through 20 after the first regional
batch. The [versioned manifest](../../../../data/src/main/resources/charging/mt-next10-pilot-v1.json)
pins the population PDF and the municipality mesh and carries exact
coordinates, reviewed names, addresses, access flags and source links.

- 11. **Nova Mutum** (65,663) — 3. Águia Branca, Energize-C Parque do Sol and Posto Bem Brasil.
- 12. **Alta Floresta** (63,017) — 2. Bionova Energia Solar and Eletroposto Mirante.
- 13. **Pontes e Lacerda** (56,720) — 3. Supermercado Favorito, Posto Tuiuiú and Magalhães Engenharia.
- 14. **Campo Novo do Parecis** (53,393) — 1. Real Auto Posto.
- 15. **Campo Verde** (51,969) — 3. Retrofit Energias, Campo Real Atacado and Supermercado Itália.
- 16. **Juína** (48,987) — 0 admitted.
- 17. **Confresa** (39,368) — 1. Estacionamento Casa dos Para-Choques.
- 18. **Juara** (36,276) — 0 admitted.
- 19. **Peixoto de Azevedo** (33,907) — 0 admitted.
- 20. **Sapezal** (33,520) — 0 admitted.

The supplement adds 13 facilities in six municipalities. The combined
staging catalogue has 73 facilities in 16 of the 20 reviewed
municipalities. Zero admitted means the evidence review did not confirm a
dedicated installation under these criteria.

## Source decisions

- Compare named candidates across Carregados, Recarregados, Vatts,
  operator publications and OpenStreetMap. Record the date of a successful
  public charging report when one was available. Repeated directory
  entries can share one origin. Exact coordinates come from the
  corroborated, independently sourced location and are rounded to five
  decimals before testing the IBGE polygon. Google Maps supplied discovery
  and identity checks, including
  [Águia Branca and Bem Brasil](https://www.google.com/maps/search/eletropostos+Nova+Mutum+MT/).
  The lookup in Sapezal suggested a facility in Tangará da Serra. The
  Peixoto and Juara lookups also suggested facilities outside the target
  municipality. Google listing coordinates and reviews are absent from
  the manifest.
- [Volvo's release](https://mb.cision.com/Main/23605/4171536/3532665.pdf)
  confirms Águia Branca at km 582. Prefer the successful charging report
  near the [mapped premises](https://www.openstreetmap.org/node/6110487391)
  over a second pin about 340 metres west. Use one facility. A
  [published equipment photograph](https://apiportal.femic.com.br/project/docs/a4ba-a37c-1537-1696474379184.pdf)
  documents Bem Brasil. Current access there is unknown. The SAU 14
  candidate remains pending because its directory pin falls in Nova Mutum
  while the [concessionaire](https://novarotadooeste.com.br/wp-content/uploads/2025/11/CARTA_SERVICO_Atualizado10.11.pdf)
  names Lucas do Rio Verde at km 640.7 and
  [one directory reports an outage](https://rotas.ninja/carregadores/nova-mutum-mt).
- Combine the Favorito addresses around the mapped building at
  Marechal Rondon and Rio Grande do Sul. Keep a separate Bom Jesus lead
  pending physical corroboration. Merge the Shell and Real Auto records
  that report the same charging location. In Campo Verde the Retrofit
  report and the Plugin listing are within about ten metres and refer to
  the same business. Only the existing central Itália branch enters.
  The [announced branch](https://plantaodanoticia.com.br/2026/10/30/italia-supermercado-inaugura-nova-unidade-na-avenida-mato-grosso-em-campo-verde-no-dia-18-de-novembro/)
  is a separate business location with no corroborated charger.
- The Confresa site has a [successful charging report](https://carregados.com.br/estacoes/estacao-estacionamento-casa-dos-para-choques-8015)
  and matching venue identity on Rua Lázaro Nogueira. Street numbers and
  access conditions vary, so the manifest uses the street and leaves
  access unknown. Magalhães Engenharia also retains unknown access given
  older reports. Keep the generic Energize-C pin near a highway police
  base in Pontes e Lacerda pending exact venue evidence.
- Exclude the general-purpose outlets listed for
  [Hotel Verona](https://carregados.com.br/estacoes/estacao-hotel-verona-6978),
  [Auto Posto Milla](https://carregados.com.br/estacoes/estacao-auto-posto-milla-1811399),
  and the [100 Fronteiras](https://recarregados.com/eletropostos/mt/juara/eletroposto-100-fronteiras-epadtsk2x2tm)
  and Casa das Ferragens entries in Juara. Juara also has a restricted
  entry with conflicting address evidence. Peixoto's
  [União do Norte listing](https://vatts.com.br/eletropostos/mt/peixoto-de-azevedo/uniao-do-norte-coming-soon/)
  is marked as coming soon. Sapezal searches surfaced installers and
  out-of-town facilities.

The manifest credits [OpenStreetMap contributors](https://www.openstreetmap.org/copyright)
and Open Charge Map's Easy Volt CC0 record for Águia Branca. The edition
is an ODbL 1.0 data offer. It carries no live status, prices or hardware
specifications.

## Replay

Use the same pinned IBGE municipality mesh and the
[regional pilot procedure](MT_TOP10_CHARGING_PILOT.md). Select the edition
explicitly.

```bash
python3 scripts/render-mt-ev-pilot.py --next-ten \
  --boundaries "$PILOT_DIR/municipalities.geojson" > "$PILOT_DIR/next-ten-seed.sql"
```

Two concurrent executions on disposable PostGIS produced one batch and
13 places. Wrong-target and changed-manifest attempts were rejected. The
four earlier pilot batches replayed successfully with 60 places.

The charging HTTP integration suite with race detection and Go vet passed.
Public staging coverage and detail reads matched all 13 new places and
the published counts of the four earlier batches. The staged API still
returns HTTP 400 on the new paginated profile. The Android staging
build reads the exact edition and reviewed IDs through existing coverage
and detail endpoints until the paginated profile in PR #188 is deployed.

All 47 focused Android tests, debug lint and APK assembly passed.
On a physical Android 16 device the map searches opened the charging
cards for Posto Águia Branca in Nova Mutum, Eletroposto Mirante in Alta
Floresta and Retrofit Energias in Campo Verde. The test APK SHA-256 is
`2e3ddad8b2a27688590a9f814797cac7c8fc9c6c0f05dbdf8ce26a295216f2ae`.

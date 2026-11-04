# Sorriso charging pilot

B-BR-EVC-SORRISO-01. Publish a reviewed Sorriso charging batch in staging
through a dedicated profile. Keep national and OCM source identities
separate. A pin requires public evidence of charging at the facility and
a corroborated location inside municipality 5107925. Unknown equipment,
tariffs and live availability stay unknown.

BUC-EVC-SORRISO-01. Load the frozen seed twice on disposable PostGIS.
Verify one unchanged batch, stable place IDs, no changes to national or OCM
data and explicit refusal outside staging. Query the public staging API,
open the map on the staging Android build with no pilot cache, and select
the pin. The previous snapshot remains readable during a failed refresh.

## Source decisions

- SAU 17. The [road operator's charging notice](https://novarotadooeste.com.br/br-163-em-mato-grosso-e-primeira-rodovia-do-brasil-a-ter-autonomia-para-veiculos-eletricos/)
  and its [operating inventory](https://novarotadooeste.com.br/nova-rota-do-oeste-neutraliza-100-das-emissoes-de-carbono-na-operacao-da-br-163/)
  identify BR-163 km 784.7 and public charging. The address identifies the
  service base. The [ANTT report](https://www.gov.br/antt/pt-br/assuntos/rodovias/relatorios/acompanhamento-ambiental-das-concessoes-rodoviarias-1/por-concessao/acompanhamento-ambiental-rota-do-oeste-1/arquivos/pgr-cro_final.pdf)
  confirms the base's municipality. The PT SAU 17 survey point in the
  [state gazette](https://iomat.mt.gov.br/portal/edicoes/download/14632)
  was converted from SAD69 to WGS84 in PostGIS and rounded to four decimals.
  The text around the survey point lists a different municipality. The
  matching base identifier, IBGE boundary and [independent station location](https://carregados.com.br/estacoes/estacao-efx---sau17-rota-do-oeste-3125)
  corroborate the site within 75 m. The pin is approximate at the facility.
- BYD Disbrava Sorriso. The [dealer's listing](https://byddisbrava.com.br/byd-sorriso/)
  and its [public store data](https://byddisbrava.com.br/api/content)
  confirm Avenida Idemar Riedi 8370, sala 02. A
  [firsthand charging report on 2026-11-02](https://carregados.com.br/estacoes/estacao-byd-sorriso-782785)
  identifies the charger at the same facility. The approximate location
  matches the report's map and the dealer's address. Access rules and
  equipment specifications still need direct confirmation.
- Delmoro, Turia/Solturi, Machado, Uipe and Imperial remain leads.
  Reviewed listings sometimes disagree on address, name or access, and
  no sufficient primary source evidence was found for their admission.
  Two Machado listings are close enough to require a duplicate review.

Both candidate coordinates fall inside the official IBGE polygon
5107925 (mesh SHA-256
`5491bd548fe381cd388ba81c9edfbf4cf2749891e2fd32c1079ac71f7aba1c10`).
The pilot contains independently reviewed public facility facts. It
contains no bulk directory extract, live status, tariffs or connector
claims. The source links are retained here for future verification.

## Load and verify

Run [the reviewed seed](../../../../backend/db/seeds/sorriso_charging_pilot.sql)
with `psql -v ON_ERROR_STOP=1` on disposable PostGIS or the staging database,
after setting `abastevo.sorriso_pilot_target` to `test` or `staging` for the
session. Use the authorized namespace connection when publishing to staging.
The seed serializes replays, keeps stable IDs and rejects a changed edition.
The public API exposes this profile through `GET /v1/charging/places` on the
canonical staging origin after the API rollout. The Android map downloads the
pilot on a cache miss and retains its last snapshot on a transient failure.
On the preceding staging API, HTTP 400 from the list triggers a bootstrap via
the existing coverage and detail endpoints. It validates the exact edition
and retrieves only the two stable IDs from the seed. An available national
or OCM snapshot takes precedence. Before retiring this pilot, reconcile the
Sorriso records with the replacement source.

On 2026-11-04 the staging seed and its replay returned one edition, two
places and no connector groups. Both public detail reads returned their
stable IDs and the readiness endpoint returned HTTP 200. The older list
endpoint still returned HTTP 400. The national catalogue had no active
generation. Disposable PostGIS integration covered concurrent seed replay,
missing approval, stable IDs, staging profile access and refusal of the
list, detail and coverage routes outside staging. Go vet and the charging
and API unit suites passed.

Android debug build and lint passed. All 48 focused tests passed,
including the shared-origin wiring regression, fresh pilot cache,
offline replay, and refusal of a mismatched bootstrap edition. The
debug APK (`9cfb20960e4f42a832c3a89f4ddbda205857b83e7480a3c1a7ce9392e10907b3`)
was installed on a physical Android 16 device. From the Sorriso map,
the SAU 17 search showed one charging result and opened its card.
Searching BYD rendered the charging pin in the city viewport.

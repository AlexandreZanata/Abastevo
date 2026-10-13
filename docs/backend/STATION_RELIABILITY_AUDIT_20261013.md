# Station reliability audit — Sorriso

Task: ING-AUDIT-01. Opened 2026-10-13 by explicit maintainer request.
Status: LOCAL_DONE / INTEGRATION_PENDING; national activation PENDING EXPLICIT CONFIRMATION. Scope: Abastevo ingestion, station identity presentation and bounded Abastevo-only VPS validation. No other VPS application, shared edge or account/media behavior is authorized.

## Behavior before implementation

- B-BR-ING-A01: One full validated establishment CNPJ resolves to one canonical UUID. Name/address/proximity never merge distinct CNPJs. Multiple products, surveys, source assertions and legal/trade names are evidence for that identity, not additional stations.
- B-BR-ING-A02: Recognize the actual ANP `status/succeeded/data/searchPageFilter` envelope and numeric `numeropagina` traversal. Missing/changed envelope, inconsistent totals, wrong page, truncation, quota and oversized bodies never produce a complete run. Every physical retry consumes the request budget.
- B-BR-ING-A03: API coordinates remain unreviewed candidates. Preserve source CRS, acquisition date, validation, accuracy and origin. Regulatory registry membership and claimed metre accuracy never establish current geographic validity. Unknown locality is explicit; API municipality names are not IBGE codes. No source-import side effect enables the capture gate.
- B-BR-ING-A04: Rust rejects ambiguous repeated headers and quarantines rows with a different field count; never silently select one of two CNPJ columns or discard surplus fields.
- B-BR-ING-A05: In the mixed Android price/directory view, a full CNPJ already visible in the selected price list appears once. Directory-only and unidentified stations remain available. Recompute after source/filter changes without deleting offline data or collapsing distinct establishments.
- BUC-ING-A01: Fetch bounded official Sorriso evidence → stage candidates → replay → compare canonical CNPJ/UUID and untouched reviewed projections.
- BUC-ING-A02: Load official prices plus canonical directory → suppress exact overlapping presentation identities → retain access through existing canonical resolution.

Risks/checks: source-contract changes and jobs require adversarial/retry tests and immediate real PostGIS/replay/concurrency validation. Rust format/clippy/locked tests; Android pure selection tests, affected compilation/lint; diff/secret review. VPS uses namespace `abastevo-temp` only, bounded requests and explicit staging, never destructive integration fixtures against the persistent database.

## Sources

- [ANP official reseller panel](https://www.gov.br/anp/pt-br/centrais-de-conteudo/paineis-dinamicos-da-anp/paineis-dinamicos-do-abastecimento/painel-dinamico-de-relatorio-de-revenda).
- [Power BI reseller report supplied by the maintainer](https://app.powerbi.com/view?r=eyJrIjoiODBmZmMwOGItYmI3NC00NzI3LTg1OTQtNDgwN2FhOTU2OWZmIiwidCI6IjQ0OTlmNGZmLTI0YTYtNGI0Mi1iN2VmLTEyNGFmY2FkYzkxMyJ9).
- [ANP API user manual](https://www.gov.br/anp/pt-br/centrais-de-conteudo/paineis-dinamicos-da-anp/paineis-dinamicos-do-abastecimento/api-revendedores-manual-usuario.pdf), version 1.0, 2024-12-13, pages 7–13. Documents geographic validation, accuracy, acquisition date, origin and SRID; maximum 5,000 rows per page.
- [Live official Swagger](https://revendedoresapi.anp.gov.br/swagger/index.html) and [OpenAPI JSON](https://revendedoresapi.anp.gov.br/swagger/v1/swagger.json), checked 2026-10-13: `/v1/combustivel`, `cnpj`, `uf`, `municipio`, `numeropagina`.
- [Bounded Sorriso query](https://revendedoresapi.anp.gov.br/v1/combustivel?municipio=SORRISO&uf=MT), checked 2026-10-13: 25 rows / 25 full CNPJs, 24 coordinate pairs, 24 explicitly unvalidated geographic positions and one empty validation/point.

## Initial findings

The live Abastevo database has 25 Sorriso stations, 25 distinct UUIDs, no current points and no reviewed points. The national active-identifier duplicate query returns zero. Dallas is CNPJ `06047857000143`; Smile/Brescansin is `02412522000126`; Jaber is `16845329000193`. Each exists once canonically. The screenshot repetition is explained by the Android screen unconditionally appending the directory to weekly price rows; price and registry use trade/legal names differently. Cached directory pages may retain old presentation until refresh.

The existing API adapter expected a synthetic `items/nextCursor` contract, omitted the official endpoint path, and accepted any JSON object as an empty complete page. The deployed daily CSV→Rust pipeline is separate: its latest successful job verified 45,617 assertions and preserved ten curated records; an earlier readiness failure retried successfully. This proves the API defect exists, not that it caused the screenshot repetition.

Source coordinates include observations from 2018/2019. No claim is made that ANP's returned point is the current surveyed forecourt position. The existing 150m gate remains blocked until reviewed evidence exists.

B-BR-ING-A06: Reimporting the same geographic observation under a different run/UUID is not independent corroboration. The review gate refuses equal checksums or the same source/date/CRS/point, before creating any review run. Absolute observation-age acceptance still requires an explicit operator policy; this task does not auto-review any point.

## Independent corroboration sources

1. [Receita Federal CNPJ open-data layout](https://www.gov.br/receitafederal/dados/cnpj-metadados.pdf): establishment CNPJ (base + branch + check digits), trade name, status, CNAE and structured address. Useful for identity, branch/succession and address discrepancy review; its published establishment layout supplies no station-coordinate pair. RFB municipal codes require an explicit namespace mapping to IBGE. Do not ingest partner/person/contact fields. RFB status is not ANP authorization.
2. [IBGE CNEFE 2022](https://www.ibge.gov.br/estatisticas/sociais/populacao/38734-cadastro-nacional-de-enderecos-para-finsestatisticos.html) and [official coordinates/methodology](https://www.ibge.gov.br/biblioteca/visualizacao/livros/liv102063.pdf): public address coordinates and geocoding-level information. Useful as dated independent address/site evidence, not a current CNPJ-matched fuel-station certification. Preserve the original reference/date and geocoding level; address match must be reviewed, never nearest-neighbor identity fusion. Scope potential ingestion to public business addresses, excluding households.
3. [OpenStreetMap](https://www.openstreetmap.org/copyright): community fuel POIs can corroborate address/site geometry when identity is independently matched. Coverage, freshness and precision vary; community data does not grant regulatory or geographic trust. Attribution/ODbL terms require a separately reviewed data-use design before new persistent integration. No OSM dataset is added by this task.
4. [IBGE territorial change](https://educa.ibge.gov.br/criancas/voce-sabia/22741-novo-municipio.html): Boa Esperança do Norte was installed on 2025-01-01 from parts of Sorriso/Nova Ubiratã. Some returned ANP records still name Boa Esperança districts under Sorriso. This is a discrepancy-review lead, not proof of each site's new municipality; use the current official polygon and a reviewed point before changing locality.

Recommended sequence: ANP identity/authorization evidence → RFB full-establishment corroboration → current IBGE locality/boundary → dated ANP/CNEFE/OSM point candidates → independent recent review → projected point → existing 150m PostGIS gate. Same ANP CSV/API/Power BI views are related evidence, not three independent geographic observations. No provider additions, contracts, paid services or bulk secondary imports are activated.

## Corrections and recovery

The API uses the actual official route/envelope, validates complete numbered pages and snapshot totals, bounds every physical retry/body, refuses redirects, checks CNPJ/UF/municipality scope, and retains coordinates as unknown-quality staging assertions. Missing pagination is accepted only for ANP's observed explicit successful `Nenhum dado encontrado` response. HTTP 404 and arbitrary/synthetic JSON no longer prove absence. Parser-version-prefixed snapshot keys prevent replay of old false-empty completions. Original runs remain intact; API runs record `registry-api-v2`.

`station-discover` is a bounded manual stage-only operator CLI (exact CNPJ or municipality+UF, <=5 pages, <=10 requests, <=5-minute deadline, pool max2). It creates no canonical stations, profiles, reviewed points or migrations. A new snapshot identity is required for a new source edition; exact replay does not refetch. Candidate metadata preserves source acquisition date, CRS/SRID, validation, claimed accuracy and origin; free-text inspection notes/products/people are not retained. Locality remains null when no IBGE mapping is supplied by the source.

Rust 0.3.1 refuses repeated headers in both readers and quarantines mismatched field counts. Corrected one pre-existing synthetic PMQC empty-point fixture containing an accidental sixth delimiter; its source hash/byte count is updated, with unchanged expected accepted candidates. Added independent malformed-column tests rather than relaxing parsing. Pipeline fingerprint changes with parser 0.3.1 so a prior unchanged snapshot cannot bypass revalidation.

Android derives directory-only rows from the full cached page and currently visible price CNPJs. No cache row or source provenance is deleted; changing filters/source results recomputes the supplement. The existing price-to-canonical detail resolver remains the action path. Unknown-CNPJ and different-branch stations are retained. Native installation/device verification is separate from passing JVM/build/lint evidence.

Recovery: retain the previous immutable ingestion tool directory and last successful state; a failed source/page stays failed/quarantined and cannot publish. Revert only the ingestion CronJob tool-directory reference if necessary, without changing API/worker/private-storage, shared edge or other applications. Never remove identity/history or promote unreviewed points to make an acceptance check green.


## Validation and actual VPS results

[Sanitized machine-readable evidence](evidence/station-reliability-20261013/result.json) and [successful VPS job output](evidence/station-reliability-20261013/vps-job.txt) pin counts and binary digests. Tested source starts from main `b7800b2` in an isolated checkout; occupied dev and its unrelated edits remain untouched.

- RED: duplicate registry header and surplus fields accepted; same-observation review promoted; official API returned false empty completion/incorrect URL; mixed-directory selection retained duplicate presentation. Regression tests reproduced each behavior before correction. Initial sandbox/cache prerequisites and a compile-only RED attempt are not behavioral evidence.
- Rust: `cargo +1.99.0 test --locked`, `clippy --locked --all-targets -- -D warnings`, `fmt --check`, and release build `prepare_registry` passed (isolated target directory). New malformed PMQC and duplicate-header tests preserve explicit quarantine.
- Go: focused registry/jobs/registrysync/station-sync suites, `go vet` for affected packages, actual CLI builds, and real disposable PostGIS `go test -race -count=1 -tags=integration` on registry, directory jobs, canonical adapters and directory reads passed. Final affected API/review integration plus jobs/pipeline unit/race suites reran after parser/snapshot-version changes. Covers complete pages, stale legacy replay, empty/no-results, malformed/null/truncated/count drift, retry/page/body quotas, foreign scope, CRS/date/NaN/missing pairs, copied evidence, concurrent imports and stable identity. Integration fixtures were never pointed at the persistent VPS database.
- Android: full domain 493 and app 300 JVM tests passed with no skips/failures; lint zero errors and debug APK passed. Final state-visibility regression passed after the hidden-price correction, with affected lint/APK rebuilt. Native device installation/execution was not requested/performed here.
- Real VPS Rust: public national snapshot produced 45,738 = 45,617 accepted + 0 duplicates + 121 quarantined, 43,328,274 assertion bytes. Source-mounted read-only, output ephemeral, 250m CPU / 256 MiB memory limits. No national canonical publication occurred.
- Real VPS API: 25 accepted, zero rejected, 24 coordinate candidates, all 25 unknown quality. Exact replay returned run `a066a838-5f01-42fe-8705-b067817f3092` twice without additional rows. All 25 full CNPJs match 25 distinct existing canonical UUIDs; nationwide active-identifier duplicates remain zero. Runs record parser `registry-api-v2`.
- Real PostGIS geometry: after EPSG:4674→4326 transformation, all 24 source points satisfy the inside 149.9m and outside 150.1m probes at radius150m. These are synthetic offsets around actual public candidate coordinates, not physical location proof and not permission to bypass quality. Sorriso's canonical projections remain 25 stations / zero points / zero reviewed locations.
- First VPS negative harness failed because `sed` appended a repeated header after the source's CR in CRLF, leaving the original header intact. Retained failed Job; corrected test strips CR before constructing the repeated header and fails closed. The initial job stopped before API staging. No parser relaxation or concealed failed result.
- Resource checks passed for the bounded jobs: no OOM, no API/worker/storage restarts and all ready. Initial high-load readings deferred the job until load1 fell below the existing 5.2 reserve threshold. Sparse guard snapshots do not certify sustained capacity.

The approval reviewer rejected launching the full national persistent-data pipeline because it could republish >45k rows and requested explicit confirmation beyond auditing/testing. That command was not executed. The active ingestion CronJob tool reference remains the previous version, with no API/worker/storage/shared-edge or other-application rollout. The corrected binaries remain in a separate ingestion tool directory. Next operational step, only after explicit confirmation: guarded existing national pipeline rehearsal, verify stable IDs/curated facts, then switch only the ingestion CronJob tool reference; preserve prior tools/state for recovery. Do not claim active daily hardening from isolated-job success.

Final source integration still requires maintained dev reconciliation and the protected dev→main process; no remote PR/merge/wiki/release claim is made. The Android APK contains the presentation correction but existing installations change only after a separate authorized install/update.

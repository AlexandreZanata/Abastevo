# ANP registry source contracts (P25-T01 freeze)

Status: FROZEN policy for P25 implementation, 2026-10-05. Owner: [catalog plan](../planning/STATION_CATALOG_PLAN.md); target rules [STATION_CATALOG](../product/STATION_CATALOG.md) B-BR-D01–D12. Semantics freeze here; wire schemas/migrations/API deltas land in P25-T02…T05 only after this policy. Provisional numeric budgets are marked `[CALIBRATE]` and get measured in P25-T02/P29 — they are explicit hypotheses, not provider SLAs.

## Sources and access (B-BR-D10/D11)

- **registry-csv**: ANP open-data cadastral CSV for operating automotive-fuel resellers, declared daily frequency. Initial full import, then one complete daily reconciliation. Approved origin only (dataset page in the catalog plan); redirect/DNS/IP validation, conditional fetch, checksum recorded per run. No caller-provided URL anywhere near this path. Credentials: none required (open data); if access changes, secrets live in operator scope, never task payloads/logs.
- **registry-api**: ANP reseller API (general or CNPJ/UF/municipality queries, paginated). Incremental/targeted lookups for newly reported/changed CNPJs only — never a national request-per-station crawl. Global per-source quota + bounded jitter/backoff; pagination/page-byte limits explicit.
- Rights: open-data redistribution recorded as permitted for catalog use at the owning task's opening (reconfirm at T02: license text, attribution, persistence). No OSM/partner/commercial-POI copying in P25.

## Field contract (provisional column names — reconfirm live at T02)

CSV parsing keys on **header names**, never positions; unknown headers quarantine the run (never silently dropped columns). Required logical fields:

| logical field | CSV header candidates | API JSON path | rules |
|---|---|---|---|
| cnpj | `CNPJ`, `cnpj` | `cnpj` | text, numeric 14 or documented alphanumeric; leading zeros preserved; checksum-invalid → quarantine |
| corporate name | `RAZAO_SOCIAL` | `razaoSocial` | trimmed, non-blank for eligibility |
| trade name | `NOME_FANTASIA` | `nomeFantasia` | optional |
| address | `LOGRADOURO/NUMERO/COMPLEMENTO/BAIRRO/CEP` | `endereco.*` | structured components verbatim; free-text single-line → quarantine |
| municipality IBGE | `COD_IBGE` | `codigoIbge` | 7 digits; unknown → location `unknown`, eligibility `pending` |
| UF | `UF` | `uf` | 2 letters |
| authorization act | `ATO_AUTORIZACAO`, `DATA_PUBLICACAO` | `autorizacao.*` | reference + dates; missing → authorization `unknown` |
| status | `SITUACAO` | `situacao` | mapped below; unmapped value → quarantine |
| coordinates | — (not in registry) | `coordenadas?` | only when supplied with CRS/accuracy metadata; otherwise `unknown` (never centroid-fabricated) |
| products | `PRODUTOS` | `produtos[]` | informational; never creates price rows (D01) |

## Frozen states and transitions (B-BR-D04)

- `authorization`: `unknown → authorized | suspended | revoked`. Only forward by sourced act evidence; `revoked` needs explicit revocation evidence (a missing row never revokes — D05).
- `operation`: `unknown → operating | closed-temporarily | closed`. `closed` needs sourced evidence or audited review.
- `eligibility`: `ineligible | pending → eligible → withdrawn`. Eligible iff: exact-valid identity (D02) + structured address + (`authorized` OR operator-verified) + not withdrawn. Review-approved unverified entries follow the separate P27 policy, never this default.
- `location_quality`: `unknown | city-centroid | reviewed` (existing wire values reused). Verified coordinates only from reviewed projections; user pins/capture GPS are private suggestions (D06).
- All transitions append-only with supersession; corrections retain history (D03).

## Precedence and conflicts (B-BR-D02/D03/D05)

CSV full snapshot vs API targeted vs DOU acts (P26) vs suggestions (P27): exact identity+address+status matches auto-resolve **only** under the frozen publication policy; status conflicts, ambiguous address/geometry, unsupported CRS or unexpected national deltas quarantine the affected run/record for review. Runs carry checksum/parser-version/completeness markers; partial snapshots never publish (atomic boundaries); a single missing row never retires a record.

## Safety limits and budgets `[CALIBRATE]`

- Caps: CSV ≤ 100 MB / ≤ 500k rows / row ≤ 8 KiB; API page ≤ 100 records / page ≤ 1 MB; source fetch timeout 30 s, ≤ 3 bounded retries with jitter; targeted API ≤ 1,000 lookups/day.
- Review thresholds (versioned, reused from survey imports): >1% quarantined rows or >20% unexplained row-count drop vs previous accepted run → operator review before publication.
- Budgets to measure in P25-T02: import rows/s, peak heap/temp bytes, transaction time/locks, queue age; API p95/error rate in P29.
- Privacy: no emails/phones/documents in fixtures or logs; restricted owner/contributor data never enters provenance (D09); all-copy 24h media rule applies to any suggestion evidence (P37).

## Fixtures and provenance

`contracts/testdata/registry/`: `manifest.json` (v1, files + sha + provenance) + `registry-csv-sample.csv` (10 synthetic rows, owned `[P25-TEST]` markers: numeric/alphanumeric/leading-zero CNPJs, duplicate row, unknown column, invalid row, unknown status) + `registry-api-sample.json` (3-record page: reviewed + city-centroid + unknown). All CNPJs synthetic test values; no real personal data. A Go fixture-contract test pins manifest/fields/markers and the no-PII surface.

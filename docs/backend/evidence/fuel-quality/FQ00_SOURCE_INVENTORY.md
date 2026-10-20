# FQ-00 — Current code and official source inventory

Status: LOCAL_DONE / INTEGRATION_PENDING. Date: 2026-10-14.
Base: `codex/fuel-quality-plan` at `03c98dc1` (parent dev `757c09ae`, ancestor of main `68483db9`).
Depends: FQ-PLAN selection after app-first/safety checkpoints. No bulk fetch, no live readiness claim.

## 1. Code inventory (inspected, not changed)

- Rust `tools/station-prep/src/pmqc.rs`: strict 5-column contract
  `AMOSTRAID, CNPJPOSTO, DATACOLETA, LATITUDE, LONGITUDE` (`PMQC_HEADER`).
  Any missing/unknown/repeated header quarantines the whole run as
  `header_mismatch` — no row is accepted. There is no assay/result/unit/
  conformity parsing anywhere in the file (grep for `assay|ensaio|conform`
  returns zero content matches).
- Output contract is `station-coordinate-candidate-v1`: source `pmqc`,
  original CRS `EPSG:4674`, `accuracy_m: null`, `review_state: pending`.
  Empty coordinates stay accepted-without-point; zero/non-finite/out-of-bounds
  points are quarantined, never moved or auto-swapped.
- Go `backend/internal/modules/directory/adapters/registry/prepared.go`:
  `LoadPreparedBatch` owns catalog staging/publication through explicitly owned
  modules. Rust prepares bounded artifacts; only Go publishes canonical identity.
- Directory owns exact full-establishment CNPJ → canonical UUID resolution.
  Name/address/brand/proximity alone never establish identity (ING-AUDIT boundary).

## 2. Coordinate/assay difference (demonstrated statically)

Feeding an assay-level extract (e.g. columns `ENSAIO`, `RESULTADO`, `UNIDADE`,
`CONFORMIDADE`) to `parse_pmqc` cannot yield candidates: the header is not a
superset match, so the run returns `RunState::Quarantined` with
`header_mismatch` and zero candidates/duplicates. The current PMQC stream
preserves sample identity + coordinates only; it discards nothing assay-level
because it never accepts assay-level input. FQ-02 must add a separate lossless
sample/assay stream; its sample-level dedup must not discard distinct assays.

## 3. Official availability (bounded verified-TLS checks, 2026-10-14)

Single GET per page with `--max-time 25`, bodies discarded except one 270 KB
PMQC HTML page parsed locally for resource hrefs. No ZIP/CSV downloaded.

| Source | URL | Observed |
|---|---|---|
| S-FQ-01 PMQC page | `.../dados-abertos/pmqc-programa-de-monitoramento-da-qualidade-dos-combustiveis` | 200 `text/html`, 0.90 s |
| S-FQ-01 PMQC metadata (linked) | `.../arquivos/pmqc/pmqc-metadados.pdf` | 200 `application/vnd.openxmlformats-officedocument.wordprocessingml.document` (DOCX bytes under `.pdf` name), 0.30 s |
| S-FQ-01 analysis guidance (old `.pdf` URL) | `.../arquivos/pmqc/orientacoes-analises-pmqc.pdf` | 404 `application/json`, 0.15 s — page now links `orientacoes-analises-pmqc.docx` instead |
| S-FQ-01 PMQC tutorial (new) | `.../arquivos/pmqc/pmqc-tutorial.pdf` | linked on page; content not fetched (out of FQ-00 scope) |
| S-FQ-02 enforcement | `.../dados-abertos/acoes-de-fiscalizacao` | 200 `text/html` |
| S-FQ-03 fines | `.../dados-abertos/multas-aplicadas-com-vencimento-a-partir-de-2016` | 200 `text/html` |
| S-FQ-04 retailer registry | `.../dados-abertos/dados-cadastrais-dos-revendedores-varejistas-de-combustiveis-automotivos` | 200 `text/html` |
| S-FQ-04 complaint channel | `.../canais_atendimento/ouvidoria` | 200 `text/html` |

## 4. Period/resource inventory (from PMQC page HTML, 2026-10-14)

- Annual: `pmqc_2016.zip` … `pmqc_2025.zip` — all 10 present.
- Monthly 2026 linked: `01, 03, 04, 05, 06, 07, 08, 09` present (8 hrefs).
- `pmqc_2026_07.zip` appears TWICE (February and July labels resolve to the
  same file); `pmqc_2026_02.zip` is absent. A link label alone does not prove
  February coverage — FQ-01/FQ-05 must resolve URLs, byte hashes and internal
  period values and retain this discrepancy.
- No byte hashes, sizes, internal headers, assay vocabulary or sample grouping
  were verified: no archive was downloaded, so native identifiers, licenses per
  edition and raw schema remain UNVERIFIED (owned by FQ-01 adapter contracts).

## 5. Scope exclusions and gaps

- Out of scope: laboratory procurement, paid trust, automatic complaint
  submission, tenant model, iOS, ML classifier, always-on service.
- Unverified: actual PMQC sample/assay column names/types, result/unit/
  conformity vocabulary, enforcement action/notice/interdiction keys, fine
  proceeding/amount/status semantics, historical CNPJ succession, per-edition
  licenses/attribution, score thresholds (FQ-12).
- Test-agent branch `codex/community-first` and primary-checkout dirty
  Evidence Store edit were not touched; this change adds one evidence file
  plus a PROGRESS entry only.

## Validation

- `curl` availability matrix above (5 pages + 2 metadata URLs), all `--max-time 25`.
- `python3` href inventory of the single saved PMQC HTML (993 hrefs, 57
  pmqc/dataset candidates; 10 annual + 8 monthly hrefs with duplicated 07).
- `grep -rn assay|ensaio|conform tools/station-prep/src/pmqc.rs` → no content
  matches; `PMQC_HEADER` 5-column + `header_mismatch` rule read at source.
- `git diff --check` clean; new-file Markdown links resolve locally;
  scoped secret/PII scan clean (no credentials, CPF, precise GPS, or media added).

## Recovery

Retain this inventory/hash evidence. Unavailable metadata (notably the
analysis-guidance `.pdf` → `.docx` move and the missing February resource)
blocks only its FQ-01 adapter contract fields, not unrelated plan work.
No partial national publication, staging import, or score decision follows.

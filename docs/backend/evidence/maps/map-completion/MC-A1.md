# MC-A1 official universe and source register

Status: LOCAL_DONE for the inventory; Track A blocked at SRC-ANP.
Date: 2026-10-27. Time: ~25 minutes (OpenAPI discovery, two CSV downloads,
counts, licence review).

## Endpoints and datasets

- OpenAPI: `https://revendedoresapi.anp.gov.br/swagger/v1/swagger.json`
  (OpenAPI 3.0.1, `ANP.Revendedores.API` 1.0). Paths: `/v1/combustivel`,
  `/v1/glp`; parameters `cnpj`, `uf`, `municipio`, `numeropagina`; the live
  service also accepts `tamanhoPagina`.
- Automotive fuel registry CSV (private VPS copy):
  7,902,828 bytes, SHA-256
  `eec94784af356616f893304c282e265bcd36c3845699b76432df3a504e89855b`,
  45,807 rows, 45,807 unique CNPJ, 13 columns, no coordinates.
- GLP registry CSV (private VPS copy): 11,785,474 bytes, SHA-256
  `be9b6b188da7b55094f9f6b658a10a7d53f891ee56b8234810046b41e6710172`,
  57,127 rows, 57,124 unique CNPJ (3 duplicate rows), 14 columns, no
  coordinates.
- Raw files stay only at
  `/opt/abastevo-temp/map-completion-20261027/inputs/anp/` (mode 700); nothing
  raw entered Git.
- No open API/CSV was found for TRR, TRRNI, aviation, GNV, marine, floating,
  school or supply-point resellers. The CDP export is CAPTCHA/manual and is not
  admitted.

## Denominators (rows by UF)

| UF | Automotive | GLP |
| --- | ---: | ---: |
| AC | 193 | 526 |
| AL | 568 | 750 |
| AM | 858 | 1,331 |
| AP | 145 | 176 |
| BA | 3,722 | 3,273 |
| CE | 2,027 | 1,572 |
| DF | 390 | 576 |
| ES | 799 | 1,448 |
| GO | 2,071 | 3,423 |
| MA | 1,795 | 1,675 |
| MG | 5,042 | 6,723 |
| MS | 632 | 1,332 |
| MT | 1,177 | 1,882 |
| PA | 1,756 | 2,244 |
| PB | 885 | 1,024 |
| PE | 1,634 | 1,494 |
| PI | 1,240 | 1,221 |
| PR | 2,957 | 4,070 |
| RJ | 1,987 | 2,025 |
| RN | 756 | 938 |
| RO | 542 | 775 |
| RR | 151 | 301 |
| RS | 3,182 | 4,637 |
| SC | 2,097 | 2,849 |
| SE | 339 | 586 |
| SP | 8,384 | 9,486 |
| TO | 478 | 790 |
| **Total** | **45,807** | **57,127** |

The API counts already observed for AC..MG match the CSV rows exactly (for
example AC 193 fuel/526 GLP, BA 3,722/3,273, MG 5,042/6,723). A 1-second API
sweep hit a Cloudflare managed challenge (HTTP 429) after 17 fuel UFs; no
challenge was bypassed. The CSV totals are the authoritative denominators for
A2 and match the Phase 1 fuel harvest (45,807).

## Licence finding

The ANP open-data pages carry the site-wide **CC BY-ND 3.0** licence
(Atribuição-SemDerivações 3.0 Não Adaptada). The API manual points to the same
open-data page and states no separate API licence. The metadata PDFs are
scanned and carry no additional licence; their fields confirm daily cadence and
the absence of coordinates in the CSVs.

Consequence: a new derived dossier that merges ANP fields with Receita/IBGE and
computed tiers is not admitted by NoDerivatives. SRC-ANP remains the existing
importer; the new artifact is **BLOCKED_PENDING_LEGAL**. MC-A2..A7 were not
executed and no substitute source or invented rule was used.

## Verification

- Both CSVs downloaded with `curl --fail`, sizes and SHA-256 recorded above.
- Header/count/unique-CNPJ/class checks ran on the VPS with Python's `csv`
  module; no row contents were copied to Git or logs.
- Free VPS disk before the download: 193 GB (above the 100 GB stop line).
- No code, migration or deployment changed in this task.

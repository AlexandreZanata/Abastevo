# Data Sources — ANP Fuel Price Survey

## Official page

https://www.gov.br/anp/pt-br/assuntos/precos-e-defesa-da-concorrencia/precos/levantamento-de-precos-de-combustiveis-ultimas-semanas-pesquisadas

Updated weekly. Each week publishes **two** XLSX files.

## Download URL pattern

Base path: `https://www.gov.br/anp/pt-br/assuntos/precos-e-defesa-da-concorrencia/precos/arquivos-lpc/{YEAR}/`

| PriceTableType | File name pattern | Example |
|----------------|-------------------|---------|
| `WEEKLY_SUMMARY` | `resumo_semanal_lpc_{start}_{end}.xlsx` | `resumo_semanal_lpc_2026-06-07_2026-06-13.xlsx` |
| `STATION_DETAIL` | `revendas_lpc_{start}_{end}.xlsx` | `revendas_lpc_2026-06-07_2026-06-13.xlsx` |

Dates use ISO format `YYYY-MM-DD`. The ANP listing page must be scraped to discover URLs (no fixed permanent URL).

## Discovery strategy (for SyncJob)

1. Fetch the ANP listing page HTML.
2. Extract all `href` links matching `arquivos-lpc/{year}/(resumo_semanal|revendas)_lpc_*.xlsx`.
3. Compare with locally stored `SurveyWeek` records.
4. Download only missing or updated files.
5. Parse and import in background (WorkManager, unmetered + charging optional).

## File 1: Weekly Summary (`resumo_semanal_lpc`)

**Size:** ~250–350 KB per week  
**Sheets:** `BRASIL`, `REGIOES`, `ESTADOS`, `MUNICIPIOS`, `CAPITAIS`

### Header rows

Rows 0–5 are metadata. **Row 6** (0-indexed) is the column header. Data starts at row 7.

### MUNICIPIOS sheet columns

| Column | Type | Example |
|--------|------|---------|
| DATA INICIAL | Excel serial date | `46180` → 2026-06-07 |
| DATA FINAL | Excel serial date | `46186` → 2026-06-13 |
| ESTADO | string (uppercase, no accents) | `SAO PAULO` |
| MUNICÍPIO | string (uppercase) | `ADAMANTINA` |
| PRODUTO | string (Portuguese) | `ETANOL HIDRATADO` |
| NÚMERO DE POSTOS PESQUISADOS | int | `8` |
| UNIDADE DE MEDIDA | string | `R$/l` |
| PREÇO MÉDIO REVENDA | decimal | `3.42` |
| DESVIO PADRÃO REVENDA | decimal | `0.19` |
| PREÇO MÍNIMO REVENDA | decimal | `3.28` |
| PREÇO MÁXIMO REVENDA | decimal | `3.80` |
| COEF DE VARIAÇÃO REVENDA | decimal | `0.056` |

### Sample stats (week 2026-06-07 → 2026-06-13)

- ~2,344 municipality rows
- ~380 unique municipalities
- 27 states
- 7 fuel products

## File 2: Station Detail (`revendas_lpc`)

**Size:** ~1.5–2.5 MB per week  
**Sheet:** `POSTOS REVENDEDORES`

### Header rows

Rows 0–6 are metadata. **Row 7** is the column header. Data starts at row 8.

### Columns

| Column | Type | Example |
|--------|------|---------|
| CNPJ | string (13 digits, leading zeros lost in Excel) | `61602199002409` |
| RAZÃO | string | `COMPANHIA ULTRAGAZ S A` |
| FANTASIA | string | `ULTRAGAZ` |
| ENDEREÇO | string | `RUA AMARO CASTRO LIMA` |
| NÚMERO | string | `1852` |
| COMPLEMENTO | string | |
| BAIRRO | string | `VILA NOVA CAMPO GRANDE` |
| CEP | string | `79106361` |
| MUNICÍPIO | string | `CAMPO GRANDE` |
| ESTADO | string (full name) | `MATO GROSSO DO SUL` |
| BANDEIRA | string (brand) | `ULTRAGAZ` |
| PRODUTO | string | `GLP` |
| UNIDADE DE MEDIDA | string | `R$ / 13 kg` |
| PREÇO DE REVENDA | decimal | `125.00` |
| DATA DA COLETA | Excel serial date | `46181` |

### Sample stats (same week)

- ~19,676 price rows
- ~7,386 unique stations (CNPJ)
- 7 product labels (slightly different naming vs summary file)

## Product name mapping

Summary and station files use different labels. Normalize at import:

| FuelProduct | Summary label | Station label |
|-------------|---------------|---------------|
| `ETHANOL` | ETANOL HIDRATADO | ETANOL |
| `GASOLINE_REGULAR` | GASOLINA COMUM | GASOLINA COMUM |
| `GASOLINE_PREMIUM` | GASOLINA ADITIVADA | GASOLINA ADITIVADA |
| `DIESEL_S500` | OLEO DIESEL | DIESEL S500 |
| `DIESEL_S10` | OLEO DIESEL S10 | DIESEL S10 |
| `CNG` | GNV | GNV |
| `LPG_P13` | GLP | GLP |

## Historical data scale

| Scope | Per week | 1 year (~52 weeks) | 5 years |
|-------|----------|---------------------|---------|
| Summary (municipal) | ~2.3K rows | ~120K rows | ~600K rows |
| Station detail | ~20K rows | ~1M rows | ~5M rows |

For millions of rows on-device:
- Store summary data by default (lightweight, covers city search).
- Make station detail opt-in or rolling window (e.g. last 12 weeks).
- Use Room with proper indexes: `(survey_week_id, state, municipality, fuel_product)`.
- FTS5 virtual table for municipality autocomplete.
- Batch insert in transactions (1000 rows/batch).
- Consider WAL mode and `PRAGMA synchronous=NORMAL` for import speed.

## Alternative: CSV historical series

For full historical backfill, ANP also publishes CSV at:
https://www.gov.br/anp/pt-br/centrais-de-conteudo/dados-abertos/serie-historica-de-precos-de-combustiveis

Better for bulk import (streaming), but different schema — evaluate separately.

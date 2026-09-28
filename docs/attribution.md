# Third-party attribution

This document lists open-source assets bundled with the ANP Fuel Prices app and their licenses.

## Fuel product icons (Phase 14)

### Source

Custom fuel glyphs created for this project. Raster sources and traced SVG copies live under `docs/assets/fuel-icons/`. Android drawables are transparent PNGs in `app/src/main/res/drawable-nodpi/`.

### Trademark policy (BR-014 / Phase 14.1.2)

Icons are **generic** fuel-industry symbols only. The app does **not** include:

- Petrobras, Ipiranga, Shell, or any retail brand logos
- Brand-specific pump artwork or trademarked color schemes tied to a distributor

### Icon mapping (`FuelProduct` → drawable)

| `FuelProduct` | Source file | Android drawable | Notes |
|---------------|-------------|------------------|-------|
| `ETHANOL` | `ETANOL.png` | `ic_fuel_ethanol.png` | Green droplet + leaf |
| `GASOLINE_REGULAR` | `GASOLINA COMUN.png` | `ic_fuel_gasoline_regular.png` | Orange pump |
| `GASOLINE_PREMIUM` | `GASOLINA ADTIVADA.png` | `ic_fuel_gasoline_premium.png` | Distinct premium pump |
| `DIESEL_S500` | `DIESEL S500.png` | `ic_fuel_diesel_s500.png` | Purple pump, S500 label |
| `DIESEL_S10` | `DIESEL S10.png` | `ic_fuel_diesel_s10.png` | Purple pump, S10 label |
| `CNG` | `GNV.png` | `ic_fuel_cng.png` | CNG cylinder |
| `LPG_P13` | `GLP.png` | `ic_fuel_lpg_p13.png` | LPG tank |

Drawables are full-color PNGs; Compose renders them with `Color.Unspecified` tint.

### Legacy MDI icons

Previous generic icons from [Material Design Icons (Pictogrammers)](https://pictogrammers.com/library/mdi/) (Apache 2.0) remain archived under `docs/assets/icons/mdi/` for reference only.

## ANP data source

Fuel price data originates from the Brazilian National Agency of Petroleum, Natural Gas and Biofuels (ANP). ANP datasets are public government information; see [docs/data-sources.md](data-sources.md) for URLs and update cadence.

## Related documents

- [architecture.md](architecture.md) — UI layer
- [glossary.md](glossary.md) — `FuelProduct` domain terms

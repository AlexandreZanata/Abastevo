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

Fuel price data originates from the Brazilian National Agency of Petroleum, Natural Gas and Biofuels (ANP). ANP datasets are public government information; see [docs/architecture/data-sources.md](../architecture/data-sources.md) for URLs and update cadence.

## Related documents

- [architecture.md](../architecture/android.md) — UI layer
- [glossary.md](../product/glossary.md) — `FuelProduct` domain terms

## Sign in with Google

The [Google Identity branding guidelines](https://developers.google.com/identity/branding-guidelines) supply the
[official full-color G](https://developers.google.com/static/identity/images/g-logo.png)
in [the Android drawable](../../app/src/main/res/drawable-nodpi/google_sign_in_g.png).
Google owns the mark. The button uses [Google Sans](https://github.com/googlefonts/googlesans)
at medium weight, subset to the localized button labels, and distributed under the bundled [SIL Open Font License 1.1](../../app/src/main/assets/licenses/google-sans-ofl.txt).

# v2 screenshots

Capture these screens for the README and Play Store listing after `./gradlew :app:installDebug`.

| File | Screen | What to show |
|------|--------|--------------|
| `week-picker.png` | Onboarding or week picker sheet | Week list, **Use latest week** CTA, year grouping |
| `home-fuel-icons.png` | Home | Municipality prices with **fuel icons + labels**, active week chip |
| `search-national.png` | Search | Query with state disambiguation (e.g. "Bom Jesus") |
| `safe-area.png` | Home or prices (rotated) | No content under status or gesture nav bars |

Recommended emulator: **Pixel 8, API 34, gesture navigation enabled**.

Automated layout captures (CI / local): `FuelProductIconScreenshotTest`, `SafeAreaScreenshotMatrixTest`.

Save PNG files in this directory and reference them from the root [README.md](../../README.md#screenshots).

# v3 screenshots

Capture these screens for the README and Play Store listing after `./gradlew :app:installDebug`.

| File | Screen | What to show |
|------|--------|--------------|
| `home-tank-cost.png` | Home | Tank fill cost card(s) below survey week label |
| `home-tank-placeholder.png` | Home | Placeholder CTA when no vehicles registered |
| `vehicles-form.png` | Vehicles | Add/edit vehicle with fuel type and price drop alert toggle |
| `onboarding-location.png` | Onboarding | Optional location prompt after first sync |
| `stations-navigate.png` | Stations | Station row with **Navigate** button |
| `settings-notifications.png` | Settings | Notification permission hint when alerts enabled |

Recommended emulator: **Pixel 8, API 34**, with gesture navigation enabled.

**Seed data for Home + vehicle screenshots:**

```kotlin
InstrumentedAppDataSeeder.seedReturningUserHomeStateWithVehicle(context)
```

Automated layout captures: `HomeScreenTest`, `FuelProductIconScreenshotTest`, `SafeAreaScreenshotMatrixTest`.

Save PNG files in this directory and reference them from the root [README.md](../../README.md#screenshots).

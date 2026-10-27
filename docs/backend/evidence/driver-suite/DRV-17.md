# DRV-17 — Quick refuel and energy entry

Status: LOCAL_DONE
Validation: PASS (JVM + owned API35 emulator; no field/device-matrix claim)
Integration: INTEGRATION_PENDING
Parent: a6c2837d. Behavior: 24c85e5a. Contract: [DRV-17](../../../product/DRV_17_CONTRACT.md).

T01: the hub now exposes real `Abastecimento rápido` and `Recarga rápida`
actions for the first vehicle. They navigate to the existing ledger form with
`refuel`/`charge` preselected; the route carries only the UUID and action and
rejects other actions. Existing tests prove pt-BR quantity/centavos parsing,
double-tap convergence, edit/restart draft restoration and typed invalid input.

T02: the full→partial→full chain, gap/segment and mixed-fuel/GNV unknown reasons
were re-run in the frozen domain tests. The existing monthly screen renders
measured vs unknown and keeps purchases visible; this phase did not add a
second ratio engine.

T03: charge entry now accepts a separate fees field. Energy Wh, total, fees,
SOC, tariff revision and session context remain separate in the draft; invalid
fees are refused. PHEV shared distance remains explicitly unknown
(`phev-shared-distance`); no per-motor efficiency or kWh↔litre conversion is
invented.

## Actual validation

- JVM: `LedgerEntriesViewModelTest` (including fees and stacked invalid state),
  `RoutesTest`, `PremiumConsumptionTest`, `PremiumEnergyTest` PASS.
- `:app:testDebugUnitTest` and `:app:compileDebugKotlin` PASS.
- Owned API35/16KB emulator: **10/10 instrumentation PASS** across
  `DriverHubScreenTest` (quick actions/states) and `DriverFoundationComposeTest`
  (legacy exact-unit/consumption UI). An emulator package-service crash on the
  first install required one reboot; the rerun passed.
- `scripts/check-mobile.sh --static-only` PASS.

## Deliberate limits

No new HTTP sync, driver-specific report screen, GPS, shift form or field/
device-matrix acceptance. The ratio display is the existing monthly indicators;
DRV-18/23 own later turn/result screens. Strings are added to the default locale;
full eight-locale/accessibility proof remains DRV-29. No push/PR/merge in this
checkpoint.

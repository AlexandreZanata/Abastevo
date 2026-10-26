# DRV-17 — Quick refuel and energy entry contract

Date: 2026-10-26. B-BR-DRV-02/11. Freeze before behavior.

## Quick entry

- The Driver Hub exposes `Abastecimento rápido` and `Recarga rápida` for the
  first owned vehicle. They route to the real existing ledger form with the
  dataset preselected (`refuel` or `charge`); no new parallel ledger exists.
- Refuel keeps pt-BR quantity/total/odometer/full-partial, exact milli-litres,
  centavos and metres. Charge keeps kWh→Wh, total, fees, SOC, tariff and session
  context separate. Fees are stored in their own field, never folded into energy
  or total.
- Save is double-tap guarded and idempotent; edit reuses the stable record id and
  revision; a process/restart restores the draft and units. Invalid input is
  refused with a typed field error.
- Legacy ambiguous odometer values still require the explicit review choice;
  saving is blocked until resolved.

## Consumption and energy truth

- The existing full→partial→full chain, mixed-fuel/GNV exclusions, gap/segment
  and missing-anchor UNKNOWN reasons remain the authority. Purchases remain
  visible even when the ratio is unavailable; interval cost still uses eligible
  purchases.
- BEV/PHEV energy uses purchased Wh, cost and distance; fees are not energy.
  PHEV shared distance cannot produce per-motor efficiency (`phev-shared-distance`
  remains UNKNOWN). Monthly indicators keep measured, reference and comparison
  values distinct and show the unknown reason.
- No kWh↔litre equivalence, invented efficiency, or zero-filled missing value is
  admitted.

## Evidence boundary

JVM ledger/domain/route tests and owned-emulator Compose checks must prove the
quick actions, exact pt-BR round-trip, fees separation, double-tap/restart and
the existing full-partial-full/unknown rendering. Real vehicle/network sync,
field and driver-day gates remain later.

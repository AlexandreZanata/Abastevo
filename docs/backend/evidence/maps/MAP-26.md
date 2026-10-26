# MAP-26 — Pure electric energy model

Status: LOCAL_DONE
Validation: PASS
Integration: INTEGRATION_PENDING

Parent: `b2e3b49b` (MAP-25). Contract: [MAP_26_CONTRACT](../../../product/MAP_26_CONTRACT.md), written first. Scope is a pure Kotlin domain component: no backend, API, storage, UI, charging-stop planning (MAP-27), provider or elevation/climate source, and no device is involved because nothing here touches Android or I/O. No trip-feasibility claim.

## What exists

`domain/ev/EvEnergyModel.kt`: useful capacity in Wh, state of charge and reserve in basis points, consumption in milli-Wh/km, distance in metres, power in W. Results are `Known(value)` or `Unknown(reason)`; a missing input is never a zero. Integer/BigInteger arithmetic with conservative rounding: usable energy, reach and arrival state of charge round down; consumed energy and calibrated consumption round up. Operations: `usableEnergyWh` (at or below reserve is a known 0), PHEV usable portion with scope `ELECTRIC_PORTION_ONLY`, `segmentEnergyWh` with an explicit non-negative margin (0..1000 permille) as the only sensitivity input, `reachableDistanceMetres`, `arrivalSocBp`, `minimumChargeSeconds` (a LOWER BOUND only, declared power being an upper bound on delivered power) and sum-first `calibrateConsumptionMilli` from completed private sessions (≥ 20 km total). Elevation and climate are not modelled.

## Evidence

- **Independent vectors.** `contracts/testdata/ev/trip-energy-v1.csv` (53 rows: usable energy, segments, reach, arrival, charging lower bound, calibration, boundary reserve/SOC/capacity, PHEV portion, rounding, ceilings, negative distance/energy/consumption, missing inputs) is generated and re-verified by `trip-energy-oracle.py`, a separate Python/Fractions implementation that shares no code with Kotlin; `python3 contracts/testdata/ev/trip-energy-oracle.py --check` fails on any drift, so the CSV cannot be rewritten to hide a defect. The hand calculations written before coding (42,000 Wh usable; 280 km and 254.545 km with a 10 % margin; 15,000 / 16,500 Wh for 100 km; 1,851.75 → 1,852 Wh; arrival 5,500 / 7,691 / 5,250 bp; 2,160 s lower bound; 200 Wh/km sum-first versus 162.5 unweighted) match the oracle output.
- **Kotlin execution.** `EvEnergyModelTest` executes all 53 rows plus property checks: reach is monotonic in SOC, capacity, reserve, consumption and margin (margin swept 0..1000); driving the claimed reach never needs more than the usable energy and one more kilometre exceeds it; segment energy is sub-additive within 1 Wh; calibration is sum-first and feeds reach; arrival never exceeds the exact value; PHEV scope; unknown results carry no number. The first test run was a real RED (the API did not exist), then the implementation made it green.
- **Mutation checks** (each reverted): segment ceil→floor fails 4 tests, reach floor→ceil 2, calibration ceil→floor 2, removing the reserve floor 2.
- **Suites.** `:domain:test :application:test` BUILD SUCCESSFUL (domain 768 tests, application 471, no failures); no Android, Go or database surface changed, so no other suite was re-run.

## Not delivered

No Go/Rust parity implementation (the contract keeps this as a client-side model); no UI or persistence; charging stops and road-distance feasibility belong to MAP-27; elevation/climate/battery-health adjustments are out of scope. Results are estimates for owner-declared inputs, never a guaranteed trip.

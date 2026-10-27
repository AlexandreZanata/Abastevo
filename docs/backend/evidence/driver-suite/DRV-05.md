# DRV-05 — Distance sources, partitions and GPS quality

Status: LOCAL_DONE
Validation: PASS
Integration: INTEGRATION_PENDING

Tested behavior: `fadfca59`. Contract: [DRV-03–07](../../../product/DRIVER_03_07_CONTRACT.md).

T01: production Go/Kotlin select one compatible scoped distance basis, reject
predicted route as measured travel, reversed readings, cross-segment/gapped or
foreign-window data. 200 km odometer + 190 km GPS selects 200, never 390.
T02: complete explicit partition 120 paid / 200 work km yields 60% and 80 empty;
missing classification stays unknown and totals/paid bounds are checked.
T03: pure Kotlin immutable GPS reducer uses a versioned caller-declared policy.
Synthetic drift, slow movement, jump, stale/future, accuracy, invalid coordinate,
reorder, tunnel, reboot and antimeridian tests pass. Last good fix and drift
anchor are distinct; gaps reanchor without filling travel. Aggregates retain
quality reasons and known partial estimate; raw fix log representation is
redacted. Geometry uses a declared spherical estimate, not odometer truth.

RED `/tmp/drv05-red.log`: actual 390000 rather than selected 200000. GREEN
`/tmp/drv05-green.log`: 3 distance + 4 GPS tests, PASS. Go driver-domain race
suite and vet PASS (`/tmp/drv05-go.log`). Diff/secret review PASS.
No location API, permission, service or server raw-trace field added. The
synthetic thresholds are explicitly test policy, not production calibration.


No deployment, provider activation or suite/UI release claim. Next DRV-06.

# DRV-04 — Shifts, clock recovery and multiapp time

Status: LOCAL_DONE
Validation: PASS
Integration: INTEGRATION_PENDING

Tested behavior: `6a66f414`. Contract: [DRV-03–07](../../../product/DRIVER_03_07_CONTRACT.md).

T01: READY/RUNNING/PAUSED/CLOSED/RECOVERY_REQUIRED transitions, bounded exact
replay receipts and version conflicts. Owner-wide atomic application port
refuses a second active vehicle/origin; 20 starts on real JVM threads have one
winner. Foreign authority rows/account changes fail closed. T02: monotonic
milliseconds retain subsecond work, pause is excluded; reboot, rollback,
missing checkpoint and process recovery retain explicit gaps rather than
inventing work. Gap review can remain unknown or record declared manual work;
it ends PAUSED and never starts GPS. T03: production Go/Kotlin union runs all
three shared time examples, including 6h+5h-4h=7h, duplicates and adjacent spans.

RED `/tmp/drv04-red.log`: actual 0 rather than 25200. GREEN
`/tmp/drv04-green.log` (DriverShiftTest 5, DriverMoneyTest 14), final authority
`/tmp/drv04-authority.log` (3), Go `go test -race ./internal/modules/driver/domain`
plus vet (`/tmp/drv04-go.log`), all PASS. Diff/secret checks PASS.

No durable shift adapter, global offline-device uniqueness or live recording is
claimed: DRV-08/14/22 own those boundaries. Only the atomic port/model are here.


No deployment, provider activation or suite/UI release claim. Next DRV-05.

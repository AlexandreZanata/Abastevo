# DRV-03 — Exact earnings and result rules

Status: LOCAL_DONE
Validation: PASS
Integration: INTEGRATION_PENDING

Tested behavior: `97b42c60`. Contract: [DRV-03–07](../../../product/DRIVER_03_07_CONTRACT.md).

T01: Go/Kotlin detailed and inclusive-net bases validate known components and
bounds; signed adjustments have type, target and revision. T02: cash movements
do not add earnings; recorded and modeled matched-window numerators stay
separate and incomplete coverage stays unknown. T03: exact weighted sums and
rationals preserve signed losses, refuse overflow/duplicate costs and mismatch.

RED: `/tmp/drv03-red.log` actually produced 0 rather than 45000 and accepted
ambiguous inputs. GREEN: `go test ./internal/modules/driver/domain
./internal/modules/metrics/...`, Go vet; `:domain:test --tests '*DriverMoneyTest'`
(14 tests), all PASS. Twelve frozen manual rows execute production rules in
both languages. Original financial-v1 unchanged. Logs `/tmp/drv03-go.log`,
`/tmp/drv03-kotlin.log`. Diff/secret checks PASS.

Pure rules only; storage/HTTP/mobile driver hub not claimed. Next DRV-04.

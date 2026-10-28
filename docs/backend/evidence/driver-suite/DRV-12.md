# DRV-12 — Accept the real driver backend chain

Status: LOCAL_DONE
Validation: PASS (local disposable PostGIS and HTTP; no provider/staging proof)
Integration: INTEGRATION_PENDING
Parent: 470e1d26. Behavior: 69bdb374. Contracts:
[DRV-10](../../../product/DRV_10_CONTRACT.md), [DRV-11](../../../product/DRV_11_CONTRACT.md).

T01: the real HTTP handler now accepts signed owner commands for earnings,
shift start, cost-profile save and earning/cost erasure. The integration journey
uses the actual pgx/sqlc stores, disposable PostGIS and real HTTP JSON: save an
earning, replay it (`replayed=true`, one row), erase it, start a shift, save a
cost profile, build a report (45000 earned, 10000 modeled cost, 35000 modeled
result), read it, correct the earning, then receive 409 on the retained read.
A foreign owner session is denied 403. No owner data is served unsigned.

T02: the report snapshot survives a pool/process restart; source edits make the
stored snapshot stale; account soft deletion removes snapshots; age-based
cleanup removes an expired snapshot and the retained read returns 404. The HTTP
error path preserves `no-store` and omits ETag. Capability lapse still allows the
free retained report read while paid build is denied (DRV-11 test).

T03: the checkpoint ran the complete backend unit suite, driver PostGIS suite
under `-race`, `go build ./...`, vet (including integration tags), staticcheck,
sqlc vet/diff, gofmt, `git diff --check`, OpenAPI lint and apicontract golden
vectors. A synthetic capability grant is used only in tests; it is not evidence
of a provider purchase, staging deployment or production certification.

## Actual validation

`/tmp/drv12-final-integration.log` (51 top-level PASS under `-race`),
`/tmp/drv12-final-unit.log` (full backend unit PASS). PostGIS container
`abastevo-pr150-postgis-20261025`, loopback port 55439. The journey is local
HTTP + real database, not a staged/mobile/provider journey.

## Deliberate limits

No real identity-key verifier composition test, real payment grant, staging
deploy, mobile/device flow, provider activation or production certification.
Command HTTP covers earning/shift/cost and erasure only; distance, maintenance
and reconciliation command routes remain application-only. Time ratios remain
unavailable until shift wall-clock projection lands. The inherited Android
coverage gate is unchanged.

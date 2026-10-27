# DRV-15 — Private driver cloud sync, conflict and restore

Status: LOCAL_DONE
Validation: PASS (backend disposable PostGIS; no Android client/UI claim)
Integration: INTEGRATION_PENDING
Parent: 27a0fc95. Behavior: 24aa536a. Contract: [DRV-15](../../../product/DRV_15_CONTRACT.md).

T01: migration 000138 extends cloud consent to 0..127 and registers
`driver_earnings`, `driver_distance`, `driver_costs` and `driver_maintenance` in
the cloud dataset allowlist. Consent is explicit per dataset. Driver payloads
are normalized by the driver domain; strict decoding rejects raw GPS/unknown
fields and credentials. The backup manifest hashes/lists consented datasets;
the existing exclusion list remains intact.

T02: a real two-device (same owner) test applies a driver earning, pulls it on
the second device, replays the same operation, receives a typed conflict for a
stale revision, snapshots the dataset, and previews/commits a restore on the
second device. A foreign owner sees no driver snapshot and cannot preview the
restore. Raw `gps_trace` payload is refused.

T03: erasing `driver_earnings` advances the epoch, purges the old snapshot and
invalidates the old cursor; the pre-erasure snapshot cannot be previewed/
restored. After explicit re-consent under the new epoch, new data syncs and a
fresh snapshot contains only the new record. This is backend two-device proof;
Android import/UI mapping is not implemented in this phase.

## Actual validation

- Full `cloudsync` integration suite under `-race` PASS, including the new
  driver dataset test (PostGIS, 127.0.0.1:55439).
- Full backend unit tests for `cloudsync` and `driver` PASS.
- `go vet`, integration-tag vet, staticcheck, SQLC vet/diff, gofmt and
  `git diff --check` PASS.
- Migration 000138 is append-only; no existing consent/records rows are reset.

## Deliberate limits

No Android client sync/import/UI, real provider/cloud object storage, guest
adoption, location collection or production certification. `driver_shifts` is
not registered in this batch (its owner/gap payload needs the same client
mapping and is deferred); the four registered datasets cover the financial/
distance continuation. The inherited coverage gate is unchanged.

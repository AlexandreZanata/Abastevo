# DRV-15 — Private driver cloud sync, conflict and restore contract

Date: 2026-10-26. B-BR-DRV-17/18; BUC-DRV-06/09/10. Freeze before behavior.

## Registered datasets and consent

Four explicit cloud datasets are registered: `driver_earnings`,
`driver_distance`, `driver_costs` and `driver_maintenance`. They use the existing
owner/family/device/signed-key cloud authority and are never enabled globally.
Consent is per dataset (mask bits 8/16/32/64), with a maximum 127 mask. Driver
cloud sync remains independent of local retention and Premium lapse for reads.

Payloads are normalized by the driver domain:
- earnings: owner/vehicle, platform/coverage, UTC window, IANA timezone, exact
  centavos and SHA256 source;
- distance: derived metres/quality/coverage/segment/odometer endpoints only;
- costs: latest per-vehicle cost profile with effective dates and ledger links;
- maintenance: dated schedule with segment/reset context.

Raw GPS coordinates/traces, credentials, tokens, passenger data and unknown
fields are rejected by strict decoding. The backup exclusion list remains
credentials/photos/location/auth/signing/purchase/permissions/professional/
advanced_rules.

## Conflict, snapshot and restore

- Existing cloud revision/operation receipts apply: stale expected revisions
  return a conflict with current/proposed payload; duplicate operations replay;
  old cursors force a resnapshot; foreign owners/devices cannot list, snapshot,
  preview or restore another owner.
- Snapshots include only acknowledged consented datasets and their checksum.
- Restore preview is owner-scoped and a second device of the same owner can
  commit it. A snapshot from before an erasure epoch is purged/rejected and the
  tombstoned id cannot be recreated under the old epoch; new post-erasure data
  requires the new epoch/consent.
- No guest-adoption path is activated. The existing generic preview remains
  owner-scoped; a guest/foreign principal sees no held records.

## Evidence boundary

Real disposable PostGIS must prove two-device same-owner pull/snapshot/restore,
conflict/replay, foreign refusal, explicit consent, raw-GPS rejection, erasure
epoch and old-snapshot refusal. Android client import/UI mapping and real
provider/cloud storage are later phases; backend registration is not a mobile
acceptance.

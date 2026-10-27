# DRV-09 — Cost profiles, maintenance and ledger links contract

Date: 2026-10-26. B-BR-DRV-13/14/18; BUC-DRV-01/04/06/08. Freeze before behavior.

## Scope

This phase adds private PostgreSQL storage for dated vehicle cost profiles,
maintenance schedules, exact ledger links, and record/owner erasure. It does
not add an HTTP route, Premium admission, cloud backup consent, mobile UI or a
second ledger/refuel/expense writer. Migration 000135 is additive after 000134.

All mutations require the transaction-bound cloudsync ownership port. Actual
cost links additionally require a transaction-bound ledger port owned by the
ledger module; the driver adapter never issues SQL against ledger tables.

## Cost profiles

- A profile is owner/vehicle scoped, has an ownership kind OWNED/LEASED/
  FINANCED, an IANA timezone, an inclusive `from` day and an optional exclusive
  `until` day.
- Profiles are append-only versions per vehicle. A save carries the expected
  version and must produce exactly `expected + 1`; old versions remain readable
  and are never overwritten. Exact operation/body retries converge; stale
  versions and changed operation bodies conflict.
- Cost items retain kind, provenance, effective period, nullable centavo amount
  and, for ACTUAL items, an existing ledger source ID and source revision.
  MODELED items (reserve/depreciation) have no ledger link. Zero centavos is a
  known amount; NULL is unknown.
- Lease and depreciation remain incompatible; principal is capital, interest
  and fees are operating/modeled per the frozen DRV-03–07 rules. No default
  market, tax, depreciation or commission rate is invented.
- ACTUAL items are linked in the same transaction that inserts the profile.
  The ledger port locks the authoritative record, requires the same owner,
  vehicle and exact revision, rejects deleted/voided records, and returns the
  authoritative amount. The item amount must equal that amount (including both
  being unknown). A stale revision, duplicate/ambiguous source ID or a missing
  ledger port fails without writing the profile. The driver never writes a
  ledger record and never creates a duplicate expense.

## Maintenance schedules

- Schedules are owner/vehicle scoped, append-only versioned and effective for
  an inclusive/exclusive day period.
- Kind is an explicit owner-entered category; basis is ODOMETER, DATE or
  ODOMETER_OR_DATE. Interval metres/days, baseline metres, next-due metres and
  date fields are nullable. NULL remains unknown; zero remains a known zero.
- `odometer_segment` records the ledger vehicle segment at entry. A reset/
  segment change is an explicit new schedule version; no cross-reset
  subtraction or invented odometer continuity is performed here.
- Revisions and operation receipts follow the cost-profile rules.

## Erasure, export and backup ownership

- Each new dataset has an owner-scoped deletion epoch and tombstones:
  `driver_cost_owner_state`/`driver_cost_tombstones` and
  `driver_maintenance_owner_state`/`driver_maintenance_tombstones`.
  Record erasure keeps the identity/version tombstone and blocks stale
  recreation; owner erasure advances the epoch and clears private payloads.
- Get/list ports provide owner-scoped reads for later export/report phases.
  No HTTP export, backup manifest or cloud allowlist is enabled here.
- Physical account deletion cascades; a soft-delete trigger erases cost,
  maintenance, command and tombstone rows. Movement rows from DRV-08 retain
  their account-level trigger; per-record movement tombstones and full history
  export remain DRV-11/15/27 obligations and are not claimed complete.
- The current cloud backup allowlist remains vehicles/favorites/preferences;
  all driver datasets stay excluded from hosted backup until a signed protocol
  is implemented and admitted.

## Required evidence

Real disposable PostGIS proves 134→135 migration, profile/schedule version and
restart, null-versus-zero, exact ledger links (including stale/deleted/foreign
and concurrent update fencing), operation replay, tombstone/epoch erasure,
owner isolation, soft/physical account cleanup and bounded list/read. Pure Go
tests prove effective-period and maintenance validation. No HTTP, mobile,
device, cloud or production acceptance is inferred.

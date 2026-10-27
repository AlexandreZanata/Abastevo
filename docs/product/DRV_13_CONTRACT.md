# DRV-13 — Owner-scoped Android state and migrations contract

Date: 2026-10-26. B-BR-DRV-02/06/07/09/10/17. Freeze before behavior.

## Scope

This phase adds the Room foundation for driver records before the UI/sync
phases. It does not add a driver screen, GPS collector, WorkManager recorder,
network sync or cloud consent. Existing ledger, map, vehicle and session tables
are unchanged.

## Schema and ownership

- Room schema 29 adds `driver_shift`, `driver_shift_gap`, `driver_earning`,
  `driver_earning_revision`, `driver_distance`, `driver_local_command` and
  `driver_draft`. Every primary key starts with `owner_account` + `origin`;
  every DAO read filters both. There is no unscoped driver query.
- Integer units stay integer: distance metres, money centavos, duration seconds.
  NULL remains unknown; zero remains a known zero. No localized string is the
  storage source of truth.
- No driver table or draft contains latitude, longitude, a raw trace, a GPS
  active flag or an automatic-start marker. Rehydration cannot start location.
- Drafts store a JSON body plus exact metre/centavo/second fields and a unit
  version. Reopening after process death returns the same integer values.

## Atomic local writes

- Earning save writes the current fact, an immutable revision and the first
  local command in one Room transaction. A duplicate revision aborts and rolls
  back the entire save.
- Shift save replaces the shift and its gaps atomically; distance save writes
  the observation and command atomically.
- Owner/origin switches, guest (`owner=""`) and process restart cannot read
  another partition. No silent migration of drafts between owners.

## Migration and repair

- Migration 28→29 is append-only: it creates the seven driver tables/indexes
  and does not drop, alter or delete ledger, map, vehicle or session rows.
- A fresh install opens schema 29; a supported 28→29 upgrade preserves seeded
  ledger draft, vehicle and saved-place rows. Close/reopen preserves driver
  drafts.
- Legacy ambiguous odometer repair remains the existing DRV-02/05 ledger-draft
  flow (explicit preview + CAS/receipt, idempotent retry). This phase does not
  duplicate or auto-apply that repair.

## Evidence boundary

Room schema JSON 29, JVM migration checks and an owned Android emulator migration
+ persistence test are required. UI/Compose, GPS, cloud sync, API26 and
production acceptance remain later gates. A compile-only result is not enough.

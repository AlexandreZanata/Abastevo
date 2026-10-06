# P27-T03 — Server catalog, offline cache and canonical action targets

Status: LOCAL_DONE on `codex/phase-27-station-intake`. Task: P27-T03.
Binds B-BR-D01/D02/D04/D06/D07 and BUC-D05. Room-persisted catalog
with offline replay; canonical UUID targets verified across
social/contribution consumers; no price fabrication, no CNPJ-as-UUID.

## Behavior (TDD)

- Room v8: `server_station_cache` (UUID PK, nullable coords, position
  order, no invented geometry) + `server_catalog_meta` (single-row
  cursor); `MIGRATION_7_8` additive-only; schema `8.json` committed.
- `DirectoryStationRoomCache` (bound to `ServerStationCache`,
  memory impl retired): fresh traversals replace the page set,
  details upsert singly, empty loads null, IDs lowercase-normalized.
  Nearby stays transient (never persisted). Offline replay = a fresh
  instance over the same DAO returns the last good page + cursor with
  zero network.
- Interface change (honest ripple): `ServerStationCache` methods are
  now `suspend` (Room-backed); memory impl removed, fakes updated,
  Hilt graph resolves (dropped the uninjectable clock lambda caught by
  Dagger, fixed the non-suspend test `clear()`).
- Canonical targets verified: feedback (`FeedbackTarget`),
  contribution (`ContributionTarget`), outbox draft (`station_id`
  UUID-checked) all refuse legacy CNPJ rows; server list order is the
  stable sort (no missing-price fabrication); legacy expert tools
  untouched.

## Validation

- NEW: `DirectoryStationRoomCacheTest` 4/4 (replace+order, restart
  replay, empty + unknown-coords honesty, case-insensitive detail +
  clear) + `Migration7To8UnitTest` (additive-only, no index, no drop).
- Regression: full `:domain` + `:application` + `:data` +
  `:app` suites + `:app:assembleDebug` + Hilt KSP PASS; backend
  `vacuum lint` + `apicontract` PASS (contracts untouched).
- `git diff --check` PASS; `scan-secrets.sh` PASS.

## Limits and next

- Device-side v7→v8 migration proof joins the end manual/device
  batch (unit SQL assertions here; no emulator per directive).
- Next: P27-T04 lightweight suggest-correct-status Android journey.

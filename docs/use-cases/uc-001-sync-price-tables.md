# UC-001 — Sync Price Tables

| Field | Value |
|-------|-------|
| **ID** | UC-001 |
| **Name** | Sync Price Tables |
| **Actors** | End User (manual), System (scheduled) |
| **Layer** | Application → Infrastructure |

## Goal

Download and import the latest ANP `PriceTable` files so the user can browse fuel prices offline.

## Preconditions

- Device has sufficient storage (estimate: 50 MB free for v1).
- For download: network available (unless only importing a queued file).
- No other `SyncJob` in active state (BR-015).

## Main flow

1. Actor triggers sync (`SyncRequested` event: `MANUAL`, `SCHEDULED`, or `FIRST_LAUNCH`).
2. System transitions `SyncJob` to `DISCOVERING`.
3. System fetches ANP listing page and extracts `PriceTable` URLs.
4. System compares URLs with local `SurveyWeek` catalog.
5. For each missing or updated file:
   - Download `WEEKLY_SUMMARY` (`PriceTableType.WEEKLY_SUMMARY`) — **always**.
   - If preference `syncStationDetail` is true (default), download `STATION_DETAIL`.
6. System parses files (BR-001, BR-002), imports rows (BR-003).
7. System emits `PriceTableImported` per file.
8. System transitions to `COMPLETED`, emits `SyncJobCompleted`.
9. UI updates `DataReadiness` to `READY` or `PARTIAL`.

## Alternative flows

### A1 — No new data

- **WHEN** all discovered URLs already imported with same checksum  
- **THEN** skip download, emit `SyncJobCompleted` with `SYNC_NO_NEW_DATA`  
- **AND** show informational message (not error)

### A2 — Network failure during download

- **WHEN** HTTP fails after retries  
- **THEN** transition to `FAILED`, emit `SyncJobCompleted` with error  
- **AND** preserve all existing local data (BR-011)  
- **AND** user can retry

### A3 — Partial import (summary ok, station failed)

- **WHEN** summary imports but station file fails  
- **THEN** mark `DataReadiness` as `PARTIAL`  
- **AND** municipality averages remain available

### A4 — Wi‑Fi only preference

- **WHEN** `autoSyncOnWifi` is true and connection is metered  
- **THEN** defer scheduled sync (WorkManager constraint)  
- **AND** manual sync still allowed with user confirmation dialog

## Business rules

- BR-001, BR-002, BR-003, BR-011, BR-014, BR-015

## Domain events

- `SyncRequested`
- `PriceTableDiscovered`
- `PriceTableDownloaded`
- `PriceTableImported`
- `PriceTableImportFailed`
- `SyncJobCompleted`

## Postconditions

- New `SurveyWeek` records exist for each successfully imported file.
- `DataReadiness` reflects latest successful import.
- Audit log entry: sync timestamp, files processed, row counts (immutable history).

## UI requirements

- Progress: stage label (discover / download / import) + optional percentage.
- Success: subtle confirmation or update home timestamp.
- Failure: retry button + offline cache still usable.

## i18n keys (minimum)

- `sync_progress_discovering`
- `sync_progress_downloading`
- `sync_progress_importing`
- `sync_completed`
- `sync_failed`
- `info_sync_up_to_date`

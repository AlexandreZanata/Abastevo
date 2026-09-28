# UC-008 — Manage Settings and Storage

| Field | Value |
|-------|-------|
| **ID** | UC-008 |
| **Name** | Manage Settings and Storage |
| **Actors** | End User |
| **Layer** | UI → Application |

## Goal

Configure app preferences, manage local storage, and view data source information.

## Preconditions

- None (settings always reachable).

## Main flow

1. User opens Settings screen.
2. User can change:
   - **Language** (`en`, `pt-BR`) — applies immediately via i18n.
   - **Preferred location** — clears or sets via UC-003.
   - **Sync station detail** toggle (BR-008, default on).
   - **Auto-download latest survey week** toggle (BR-020, default on).
   - **Auto sync on Wi‑Fi only** (BR-014).
   - **Station retention weeks** (BR-013).
   - **Show price history** (UC-006).
3. On each change, system emits `PreferencesUpdated`.
4. User can tap **Sync now** → UC-001.
5. User can view **Storage usage**: summary size, station detail size, week count.
6. User can **Clear cache**:
   - `STATION_DETAIL_ONLY` — remove station rows per retention policy.
   - `ALL` — remove all imported data, reset to `EMPTY` (confirmation dialog).

## Alternative flows

### A1 — Clear all with confirmation

- **WHEN** user confirms clear all  
- **THEN** emit `CacheCleared(ALL)`, wipe DB, return to UC-002 flow

### A2 — Storage full during sync

- **WHEN** import fails for insufficient space  
- **THEN** show `STORAGE_FULL`, suggest clear station detail

## Business rules

- BR-008, BR-011, BR-013, BR-014, BR-020

## Domain events

- `PreferencesUpdated`
- `CacheCleared`

## Postconditions

- Preferences persisted locally.
- Storage reflects user choices.

## i18n keys

- `settings_title`
- `settings_language`
- `settings_sync_station_detail`
- `settings_auto_download_latest_week`
- `settings_sync_wifi_only`
- `settings_retention_weeks`
- `settings_storage_usage`
- `settings_clear_cache`
- `settings_clear_cache_confirm`
- `settings_anp_attribution_link`

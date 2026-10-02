# P23-T01 — Favorites and justified opt-in return flows

Status: LOCAL_DONE on `codex/phase-23-community-operations`. Issue: #91. Entry G22 satisfied (P22 PR #90 merged `cdd395f`, issues #86–#89 CLOSED, wiki `12f75e8`). Binds B-BR-C01–C06 and BUC-C01–C05. No migration, no backend change, no new permission, no iOS work.

## What this task adds

- Duplicate-alert suppression for the existing local opt-in return flow (UC-014 vehicle price-drop alerts): the same drop notifies once per vehicle per evaluated week; repeat evaluations show nothing new; a new week re-arms. New domain port `PriceDropAlertHistoryRepository` (device-local only, no account/backend/PII), wired through `EvaluatePriceDropAlertsUseCase`, persisted in a dedicated Preferences DataStore (`price_drop_alert_history`) via a pure tested codec, bound in `RepositoryModule`/`UseCaseModule`. Clearing app data resets history (one repeat at most) — safe and explicit. Untouched: opt-in toggle, permission gating, cancel-on-disable, skip reasons, notification content.

## Deliberately not built (no measured need)

- Follows, city/station activity subscriptions and backend push: not implemented — the local UC-014 alerts already cover the return journey and no measured need exists. Frozen contract for IF they ever are (must land before any client): per-account ownership with server-side derivation, one-tap unsubscribe, erasure covering subscriptions plus their alert history, keyset pagination; no endless feed or ranking incentives without evidence. Until that contract exists as code-backed tests, subscription code must not exist.
- Station favorites store: none exists to preserve or migrate; vehicle alert preferences are preserved byte-for-byte (no prefs schema change).

## Validation

- `EvaluatePriceDropAlertsUseCaseTest` +2 (same-week silence, new-week notify + record), `PriceDropAlertHistoryCodecTest` 4 (round-trip, blank, isolation, stability).
- `:domain:test` 408 + `:application:test` 248 + `:data:testDebugUnitTest` 207 (1 pre-existing live-network skip), 0-fail (`--rerun-tasks`); `:app:assembleDebug` PASS (Hilt graph compiles).
- `git diff --check` clean; scoped secret review (local-only keys, random UUIDs, no PII/GPS).

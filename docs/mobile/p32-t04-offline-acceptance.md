# P32-T04 — Offline, accessibility and real-device acceptance (code slice)

Branch: `codex/phase-32-station-profile-app`. Device/emulator runs: OWED at conjunto closure per user directive (no runs during construction); this slice delivers the code-regression half.

## Implemented

- `domain/profile/ProfileOfflineRule.kt` — restart recovery: idle / requeue-valid / drop-expired-with-notice / blocked-revoked; cached authority never accepted across restarts.
- `ClaimRestartRecoveryTest` (application) — restarted use-case instance honors already-seen nonces (replay denied after accept).

## Validation

- RED→GREEN: offline-rule 4/4 + restart-recovery 1/1.
- Full affected suites, 0 failures: `:domain:test` + `:application:test` (36s), `:data:testDebugUnitTest` + `:app:testDebugUnitTest` (76s) — BUILD SUCCESSFUL.
- `:app:assembleDebug` PASS. `git diff --check` PASS. `scan-secrets.sh` PASS.
- Accessibility: badge/status/action labels are plain-text pt-BR strings bound to the same content-description path (no icon-only meaning); large-text/font-scale and TalkBack rows OWED on device.
- No backend/Room change → no migration/PostGIS run in this task (P31 backend already proven on its branch).

## Phase P32 exit (LOCAL_DONE)

T01 public profile/badge, T02 claim export-sign-import, T03 management/invitations/contest, T04 offline/code acceptance — all LOCAL_DONE with pushed atomic commits (`bd732bf`, `85f010c`, `46da74b`, T04 commit). G32 code checkpoint complete; device/manual rows, final cumulative CI/PR/merge/wiki OWED at conjunto closure. iOS archived; G09 UNCERTIFIED.

## Owed device/manual rows (P32)

File-URI permission expiry, no-photo cache, low-end parse memory, screen reader/font scale, old-API cache fallback, offline pending vs expired/revoked on device, process-restart file flows — planned in P38-T02 union matrix, execution OWED.

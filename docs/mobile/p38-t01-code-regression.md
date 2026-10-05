# P38-T01 — Integrated code journey regression

Status: LOCAL_DONE on `codex/phase-38-app-acceptance`. Task: P38-T01.
Scope: Explore → station/community → free login → capture/review →
queued status → comment/vote/report → deletion/export-where-implemented,
with empty city, no price, bad auth, offline/restart, revoked identity,
media expiry, migration/search and server outage. No device run; end
manual/device batch stays OWED.

## Regression sweep (one pass, failures retained — none occurred)

- Backend unit (affected P34–P37 contracts): `go test
  ./internal/platform/apicontract/... ./internal/platform/health/...
  ./internal/modules/directory/... ./internal/modules/account/...
  ./internal/modules/identity/... ./internal/modules/feedback/...
  ./internal/modules/moderation/... ./internal/modules/evidence/...
  ./internal/modules/privacy/...` → all `ok` (0 failures).
- Backend real-PostGIS integration (`-tags=integration`, disposable
  `127.0.0.1:5434`): health + directory + account + feedback +
  moderation + evidence + privacy → every package `ok`, zero
  non-ok lines (400 invalid-lat / 404 unknown-UUID honesty,
  deletion-revocation atomicity, vote/report replay, expiry jobs all
  green).
- Android: `./gradlew :domain:test :application:test
  :data:testDebugUnitTest :app:testDebugUnitTest :app:assembleDebug
  --no-daemon` → BUILD SUCCESSFUL (89 tasks). Covers: UUID
  discovery/cache/fallback (P35), staging origin guard + auth +
  discussion wiring (P36), contextual capture + outbox submit (P37),
  plus untouched legacy suites (search, ANP browsing, OCR, outbox,
  moderation, onboarding — no regressions).
- Contracts: `vacuum lint` PASS (0 errors, 27 informs, 97/100);
  `apicontract` PASS. `git diff --check` PASS; `scan-secrets.sh` PASS.
- Live (no bypass): staging `curl` 60 on health/ready/nearby (Fortinet
  middlebox CA, per P34-T01); server JSON UNVERIFIED, no `-k`/
  trust-all/HTTP fallback anywhere in `data/src/main`.

## Journey coverage (code-level, live owed)

- Explore/list/nearby/detail with UUIDs + last-known recovery: unit +
  ViewModel suites green; live reads BLOCKED_LIVE.
- Free login/session/key flows + denial/replay/revocation/deletion:
  backend + app suites green; provider delivery/device callbacks OWED.
- Capture/review with station context + outbox submit + worker retry:
  suites green; camera/low-end encode/device recovery OWED.
- Ratings/comments/replies/votes/reports on UUID targets + abuse
  matrices: suites green; device-level discussion proof OWED.
- Export: contract frozen, implementation OWED (P36-T03 decision).
- Media: 24h all-copy matrix green in isolation; live path
  BLOCKED_MEDIA (no attached VPS storage).

## Limits and next

- No failures to retain; no thresholds reduced, no skips added, no
  errors suppressed. Owed live/device/provider/media rows are listed,
  not waived.
- Next: P38-T02 consolidated end manual matrix (plan only, no runs
  until all selected source phases complete).

# UGC safety evidence — report and block users

Date: 2026-11-15. Task: Google Play readiness item 2.5
([PLAY_STORE_READINESS](PLAY_STORE_READINESS.md)). Rules:
[community safety contract](../product/COMMUNITY_SAFETY_CONTRACT.md)
(B-BR-SAFE-01…09, BUC-SAFE-01…05). Base: `origin/dev` `db43e949`
(tree of main `7ab0b477` after PR #224). Status: LOCAL_DONE /
INTEGRATION_PENDING. This record deploys nothing.

## Decision: where the block list lives

The block list is stored server-side per account (`community_user_blocks`),
not on the device:

- It follows the blocker to every signed-in device and survives reinstall.
- Public reads stay anonymous and cacheable and never carry an author id.
  No new public link between forum and station content is created
  (B-BR-FOR-04).
- The app asks a private signed endpoint which items on screen to hide. It
  skips the question while the account has no blocks. If the answer fails
  or takes longer than 5 s, the content shows (fail open), and the next load
  or refresh asks again.

Blocking needs an account, like every other community write. A block is not
a moderation signal; reporting is the moderation path.

## Backend

| Item | Where |
|---|---|
| Module | `backend/internal/modules/safety` |
| Migration | `000239_community_user_safety.sql`. The number was reserved with the F4 batch, which allocates from 000240. |
| Routes | under `/v1/community/`, wired in `backend/cmd/api/safety.go` |
| Erasure step | `backend/cmd/worker/main.go` |

| Check | Result |
|---|---|
| Domain rules (`safety/domain`): content kinds, reasons, detail bounds, self target, caps, quota error, hidden subset, labels, fixed case detail | PASS |
| HTTP (`safety/adapters/http`): the 5 routes require a signed session; strict JSON; a client-named account is refused; validation runs before the store; error codes and `Retry-After`; no account id or internal text in responses; the signature verifier reads the exact body | PASS |
| `cmd/api`: routes mounted; a guard refusal answers 401 | PASS |
| Real PostGIS (`-race -tags=integration`, disposable `postgis/postgis:18-3.6`): 9 tests, see the detail below | PASS |
| Mutation probe: removing the self-target check fails `TestSafetyTargetRefusals` | caught |
| Affected suites with `-race -tags=integration`: safety, forum, feedback, moderation, platform/migrate, cmd/api | PASS |
| `staticcheck` (`GOTOOLCHAIN=go1.27.1`, as in CI), toolchain `gofmt`, `go vet`, `vacuum` on OpenAPI, `git diff --check`, `gitleaks` | PASS |

The 9 real-PostGIS tests cover:

- **Blocks:** block lifecycle and replay; hidden subset across all content kinds; unblocking a foreign or unknown block answers 404.
- **Refused targets:** pending topics, editorial posts, hidden comments, replies under a hidden parent, deleted station comments, wrong content kind, and the user's own content (self target).
- **Reports:** a repeated report replays the first one; one open case per reported account, with PII-free detail; the report quota is rolling.
- **Limits:** the active-block cap (409) is checked before the 24-hour window quota.
- **Rollback and concurrency:** a window-quota refusal stores nothing; a guard refusal stores nothing; 8 concurrent blocks produce 1 row.
- **Erasure and schema:** erasure runs in bounded batches; schema constraints hold.

## Android

Implemented in the `app`, `application`, `data` and `domain` modules, with
strings in all 8 locales.

- **Forum topic:** a ⋮ menu with *Report topic*, *Report user* and *Block user*.
- **Comments and replies:** a ⋮ menu with *Report comment*, *Report user* and *Block user*. The viewer's own content shows no such menu.
- **Station comments:** *Report user* and *Block user* were added to the existing menu.
- **Profile → Manage account → Blocked users:** lists blocks and unblocks them.
- **Hidden-content filter:** applied to the feed, conversations, best-answer previews, the live stream and station comment lists. An opened topic from a blocked author shows a neutral notice instead.
- **Terms version:** `PublishedLegalDocuments` moves to `2026-11` together with the updated Terms page (B-BR-TA03), so the in-app terms screen asks every user to accept the new version.

| Check | Result |
|---|---|
| New unit tests: domain 8, application 17, data 15, app view models 41, string parity 2 (same keys and placeholders in all 8 locales) | PASS |
| Full `./gradlew :domain:test :application:test :data:testDebugUnitTest :app:testDebugUnitTest` (subagent run, before the Terms version bump) | all green except `PublishedLegalDocumentsTest`, fixed by the version bump |
| `scripts/check-mobile.sh --static-only`, `git diff --check` | PASS |
| `:domain:jacocoTestCoverageVerification` | Fails at 0.86 against a 0.90 minimum, as before this change. The new safety package is at 95.8%. CI runs `./gradlew test`, not this rule. |

## End to end: emulator against a local API

Set-up:

- Disposable PostGIS with the full migration chain, ending at 000239.
- The local API on `127.0.0.1:18080` in development mode, forum enabled.
- `app-mapProbe.apk` (`-PmapApiLocalProof=true`) installed as the separate package `com.anpfuel.app.mapprobe` on emulator `emulator-5588` (API 36), using `adb reverse`.
- Fixtures created through SQL:
  - two other accounts (author B, reader C);
  - a published topic by each;
  - B's comment and B's reply under C's topic;
  - a directory station with one comment by B and one by C.
- The viewer created a username-only account in the app and accepted the Terms, version `2026-11`.

| Step | Observed | Server state |
|---|---|---|
| Topic menu on B's topic | *Report topic*, *Report user*, *Block user* | — |
| Report topic (Defamation) | "Report received…" card; the topic stays visible | `forum_reports` row and an OPEN `FORUM_POST` case |
| Report user from the topic (Harassment + detail) | "Report received. The moderation team will review it." | one `community_user_reports` row with the private detail; an OPEN `COMMUNITY_ACCOUNT` case on B with fixed detail `community user report via FORUM_POST <id>`, without the reporter's text |
| Report user again from B's reply, then from B's station comment | "You already reported this user. The report is under review." | still 1 report and 1 case |
| Block user from B's comment (confirmation dialog) | "User blocked…"; B's comment, B's reply and its preview disappear; C's comment stays; expanding C's replies shows nothing from B | block row with label `''`, source `FORUM_COMMENT` |
| Forum feed | B's topic is gone at once and after refresh | `/blocks/hidden` 200 |
| Station page | B's station comment is hidden; C's shows | — |
| Profile → Manage account → Blocked users | "Forum participant · Blocked on Nov 15, 2026" → Unblock (confirmation) → "User unblocked." and the empty state | 0 blocks |
| After unblock | B's topic is back in the feed; both station comments show | — |
| Block user from B's station comment | B's comment is hidden; C's stays | block with label `e2e-autor-b`, source `STATION_COMMENT` |
| Own comment (posted, then approved in the DB) | only *Edit* and *Delete*; no report or block menu | — |

Findings fixed during the run:

1. **Every signed safety call answered 401.** The handler decoded the body
   before signature verification and did not restore it, so the verifier
   hashed an empty body. Regression test
   `TestSafetySignedOwnerSeesExactBody` failed (RED), then passed (GREEN) once
   the body was restored.
2. **Top-level station comments never showed on devices.** This bug predates
   the change. Android's `org.json` reads `"parent_id": null` through
   `optString` as the text `"null"`, so the list filter dropped every
   top-level comment. The JVM `org.json` used in unit tests does not, so the
   client now checks `isNull`; a contract test documents the shape. The fix
   was confirmed on the emulator.

Observations, not fixed here:

- **Station comments are off in production.** Station comments and ratings
  sit behind the local `feedback_enabled` flag. Its default is off and
  nothing turns it on, so production users currently see no station UGC.
  The E2E turned the flag on in the test package only.
- **Under heavy host load the filter can let content through.** With load
  average about 46 (other sessions' builds), a hidden-content question
  overlapped a slow session refresh and passed the 5 s budget. The feed
  showed B's topic once, and the next refresh hid it. Under the same load,
  one ANR came from a dropdown popup window waiting for a focus event. The
  app's main thread was idle (stack captured), and the ANR did not happen
  again when retried.
- **Forum detail showed the text `null`.** This happened only for the SQL
  fixtures, whose `excerpt` was NULL. Topics created through the API store
  an empty excerpt.

Clean-up: the test package was uninstalled, the `adb reverse` was removed
and the local API was stopped. The user's `com.anpfuel.app.debug` app on the
shared emulator was not touched.

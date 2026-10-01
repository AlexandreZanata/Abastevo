# P17-T02 — Android functional social flows

Status: LOCAL_DONE on `codex/phase-17-social-iphone-parity` (second
task; phase draft PR #59 open). Issue: #56. No backend change, no
migration, no existing screen redesigned, no new string resources.

## Slice acceptance (frozen before coding)

- T02 (this commit) — Expose rating/comment/reply/vote/report
  functionality through the P17-T01 shared use cases with data
  transport, DI and ViewModel state. Tests first: 280 limit,
  no-login action, revision edit, denied moderation, outage/retry.
  Each wired action works end to end (ViewModel → use case →
  gateway, proven by MockWebServer + ViewModel tests); no redesign,
  no fake success states.

## What changed

- `data/.../remote/FeedbackHttpClient.kt` — `FeedbackGateway` over
  the ten published P14 routes (comment/reply/edit/remove, comment/
  reply pages, vote/remove/tally, report). Writes carry the live
  session (`family_id` + `access_token` via `AuthSessionStore`;
  author derives server-side); missing/expired sessions never touch
  the network (GATE_REQUIRED). Backend codes map to stable kinds
  (401 → GATE_REQUIRED, 403 self-vote → SELF_VOTE, 403 other →
  NOT_AUTHOR, 404 → COMMENT_NOT_FOUND, 409 stale-revision →
  STALE_REVISION, 429 → QUOTA_EXCEEDED); IO/empty/unmapped are
  TRANSPORT with fixed messages. Replies pages reuse the
  `comments` envelope (`writePage`); `created_at` parses via
  `Instant` (minSdk 26) with 0 fallback. Vote receipts use a
  `local:<id>#<rev>` correlation key — the wire confirms votes with
  the tally, not an id (documented at the call site).
- Ratings (`rate`/`deleteRating`/`stats`) refuse explicitly with
  GATE_REQUIRED ("ratings transport not published"): the backend
  has store + application but no wired path (recorded P14 gap —
  T02 evidence "no transport", openapi "preview schema only").
  The ViewModel exposes no rating action, so nothing fakes stars.
- `data/.../local/preferences/FeedbackFlagStore.kt` — flag,
  default OFF (rollback is flag OFF).
- `data/.../repository/FeedbackRepositories.kt` — in-memory
  first-page cache (never presented as fresh) + bounded outbox
  (100 ops, drop-oldest with visible counter).
- `data/.../di/FeedbackModule.kt` — HTTP client provider (same
  preview `.invalid` base as CommunityModule: no resolution until
  deployment config lands); `RepositoryModule` binds
  gateway/cache/outbox/flag; `UseCaseModule` provides
  `SubmitFeedbackUseCase` + `GetFeedbackPageUseCase`.
- `app/.../community/FeedbackDisplay.kt` — pure: 280 counter,
  agreement line (floor truncation, one decimal, "No votes"),
  fixed reject labels (no private reasons in copy).
- `app/.../community/FeedbackViewModel.kt` — Hilt VM: prepare,
  loadFirstPage (no login needed), comment/reply/edit/vote/
  removeVote/report, retry with the same stable op id,
  single-flight. Blank session → SignInRequired (no IO);
  over-280 → INVALID (no IO); DomainException → INVALID;
  Rejected kinds for rollback; Queued for outage.
- Tests: `FeedbackHttpClientTest` (6, MockWebServer),
  `FeedbackDisplayTest` (4), `FeedbackViewModelTest` (8, mockk +
  StandardTestDispatcher).

## RED → GREEN

- RED: new tests failed compilation (`Unresolved reference
  FeedbackHttpClient/FeedbackDisplay/FeedbackViewModel`) plus one
  self-made naming slip (`GetFeedbackPageOutcome` for the real
  `FeedbackPageOutcome`), fixed before GREEN.
- GREEN: `./gradlew :domain:test :application:test
  :data:testDebugUnitTest :app:testDebugUnitTest
  :app:assembleDebug` all BUILD SUCCESSFUL; result XMLs confirm
  HttpClient 6/6, Display 4/4, ViewModel 8/8, 0 failures, 0 skipped.

## Validation (exact commands, this host)

```sh
./gradlew :domain:test :application:test :data:testDebugUnitTest :app:testDebugUnitTest :app:assembleDebug --no-daemon
bash scripts/check-mobile.sh --static-only
git diff --check
bash scripts/scan-secrets.sh
```

Outcome: BUILD SUCCESSFUL (40s); `check-mobile --static-only`
ok; `diff --check`/secrets clean.

## Limits (not claimed)

- No Compose panel and no new string resources in this slice:
  panels need frozen copy + 8-locale backfill (P10-T08 precedent);
  VM states + Display copy are designed for screens. Panel
  composition follows under #56 before phase merge.
- Ratings UI waits on the backend ratings path (P14 gap); the
  gateway refusal is explicit and tested.
- Session refresh on 401 stays with the P13 auth flow; the client
  surfaces GATE_REQUIRED so the UI routes to sign-in.
- Full device-matrix proof stays with P17-T04/G17, never claimed
  here. Rollback: feedback flag OFF preserves existing free/offline
  behavior and compatible API.

# CS00 — Audit and frozen implementation slice

Date: 2026-10-11. Status: LOCAL_DONE. Scope: audit only, no runtime,
migration, VPS, APK, GitHub or wiki change. Opening head: maintained
`dev` at `61f7c1b` (plan opened at `0f3601c`; `61f7c1b` is the pushed
Android CI coroutine-isolation correction, remote checks pending).
Plan: [phased plan](../../../planning/COMMUNITY_SAFETY_EXECUTION_PLAN.md).
`dev` contains `main` (15 ahead, 0 behind); fast-forward-able, no conflicts.

Meaning freeze (from the plan, unchanged): votes mean contribution
**helpful / not helpful**, UI `Contribuição útil` / `Contribuição não
útil`, wire `HELPFUL` / `NOT_HELPFUL`. They never validate a price and
never feed confidence, consensus, ranking or star ratings.

## Reused inventory (all paths verified on this head)

- Community confirmations/disputes (NOT helpfulness):
  `backend/internal/modules/community/application/votes.go`
  (`EligibilityWindow = 48h`, `VoteStore`/`VotePorts`, `ConfirmDTO`,
  retry converge, `CheckAccount` refusal), `domain/confirmation.go`
  (immutable vote), migration `000013_community_votes.sql` (unique
  observation+contributor pair, unique contributor+submission, disputes
  open-unique reporter+target+reason; reasons PRICE_CHANGED,
  WRONG_STATION, WRONG_PRODUCT, WRONG_CONDITION, EVIDENCE_MISMATCH,
  OTHER), HTTP `POST /v1/observations/{id}/confirmations|disputes`
  (`community/adapters/http/http.go`), OpenAPI `v1.yaml:340,388`,
  Android `data/.../CommunityVoteHttpClient.kt`,
  `app/.../community/CommunityVoteViewModel.kt|Panel.kt`.
- Feedback VALID/INVALID votes (pattern reuse, not semantics):
  `feedback/application/votes.go` (`CastVote`/`RemoveVote`/`Tally`/
  `RebuildTally` under one transaction), `domain/feedback.go`
  (`VALID`/`INVALID` frozen), migration `000025_feedback_votes.sql`
  (one live vote per account/comment/revision, partial unique where
  `deleted_at IS NULL`; percentage `floor(10000*V/total)`, null on zero),
  HTTP `.../votes`, `.../votes/remove`, `.../tally`, `.../report`,
  OpenAPI `v1.yaml:2662,2723`, error codes `feedback.<kebab>`.
  Note: Android `strings.xml:453-455` already labels VALID/INVALID as
  "Helpful/Not helpful"; helpfulness cards need distinct context so the
  two meanings never mix.
- Moderation: `moderation/domain/case.go` (`PolicyV1`, targets
  OBSERVATION/DISPUTE/CONTRIBUTOR/EVIDENCE/COMMENT/STATION_CLAIM,
  OPEN/IN_REVIEW/RESOLVED/REJECTED, P1-P3, `MaxReasonChars = 200`,
  `MaxDetailChars = 500`), migration `000016_moderation.sql`
  (one open case per target), policy
  `docs/product/COMMUNITY_MODERATION.md`, operator CLI
  `backend/cmd/ops/moderation.go`.
- Identity quotas: `identity/domain/quota.go`, policy v1 —
  register 10/hour, write 200/hour, challenge 60/hour.
  `HashIPSubject` stores only `ip:<keyid>:<hex>` under a rotating key;
  raw IPs never reach storage. IPv4-mapped IPv6 normalization and key
  rotation tests are CS03 work, not assumed done.
- Feed: `GET /v1/community/feed` (`community/adapters/http/feed.go`;
  default limit 20, `ValidateFeed` caps the route at **50**, orders
  `recent|cheapest|best|worst`, `FilterHash("community-feed-v1", …)`,
  sealed cursor, `refresh_after_seconds = 15`, errors
  `community.bad-feed-filter|bad-feed-cursor|feed-unavailable`),
  `adapters/read/feed.go` (limit+1, explicit `FeedItem` allowlist with
  no voter/owner fields), `db/queries/community/feed.sql` (canonical
  city/product/unit/STANDARD/AVAILABLE/expiry predicates, keyset with
  `station_id` tie-breaker, `CASE` ORDER BY — measured plan review owed
  in CS06), shared bounds `httpapi/respond.go`
  (`DefaultLimit = 20`, `MaxLimit = 100`; route-50 vs shared-100 needs
  the explicit compatibility decision already flagged in B-BR-CS05),
  cursor `httpapi/cursor.go` (`CursorTTL = 15min`, HMAC-SHA256,
  filter-bound), money exact milli-BRL, Android
  `app/.../ui/community/CommunityFeedViewModel.kt|CommunityScreen.kt`.
- Privacy/evidence/media: photo `ExpiresAt = receipt + 24h`
  (`community/application/photo_capture.go:125`); evidence
  `MaxUploadBytes = 3 MiB` (`evidence/domain/session.go:20`,
  OpenAPI `max_bytes: 3145728`), day-bounded reservations with
  idempotent release (`evidence/application/reserve.go`); retention
  `ModerationClosedTTL = 365d`, `LedgerHorizon = 35d`
  (`privacy/application/retention.go`); media policy
  `docs/security/LOCAL_MEDIA_LOCATION_POLICY.md` M01–M06 (single 24h
  deadline, JPEG target ≤150 KiB / hard cap ≤256 KiB / edge ≤1600px /
  ≤2MP, signed reads ≤ min(60s, remaining), app cache ≤24h); jobs
  `SKIP LOCKED` fencing leases (`platform/jobs/doc.go`), dedupe unique
  (`000007_jobs.sql`).
- Location: `MaxClaimAccuracyM = 100`, `MaxAcceptableAccuracyM = 100`,
  `MaxPlausibleSpeedMS = 100`; 150m capture rule stands; ANP presence
  is not reviewed GPS.

## Missing inventory (CS01+ builds this, nothing invented here)

- No `HELPFUL`/`NOT_HELPFUL` identifier exists anywhere in backend,
  Android, or OpenAPI (verified by search; only the VALID/INVALID UI
  strings above). Tables, routes, DTOs and tallies are CS01/CS02 work.
- Community observations carry `PolicyVersion` but **no per-edit
  revision counter** (unlike feedback comments). Binding one vote to
  account+contribution+immutable-revision (B-BR-CS01) needs a CS01
  target decision first (observation + decision sequence is the
  candidate; `000008` already uniques decision sequences).
- No `CONTRIBUTION` moderation target; report taxonomy for it unchosen.
- Training derivative inventory, per-image consent tables and TEST_ONLY
  receipts do not exist (PC05 NOT_IMPLEMENTED, collection OFF).

## Target/reference mapping (input to CS01, not a contract yet)

Helpfulness target must resolve to a stable public contribution
reference plus an immutable revision with server-resolved author, per
B-BR-CS01. Aggregated price cards (multi-observation projections) must
never be voted on directly. Owner overlay (`my choice`) is served only
on the private read path, never in public caches.

## Supported orders and frozen stop limits

Orders: `recent`, `cheapest`, `best`, `worst` (feed). Benchmark freeze
(from the plan, unchanged): pilot first, then ≥5 independent ≥120s
windows at safe rates with separate warmup; p50/p95/p99/max, offered/
completed/rejected, 429 vs unexpected, drops, bytes, CPU/RSS/cgroup,
pool waits, rows/blocks, WAL/temp, queue age/depth, storage and
cleanup lag; wall-clock throughput; staging stays below the guarded
5 req/s ceiling without consuming host reserve; no max-user claim.

## Proposed API/error shapes (PROPOSED, frozen in CS01)

Family `POST /v1/contributions/{ref}/helpfulness`-style commands with
stable client submission IDs (converge/replace/remove/idempotent, same
durable rules as confirmations and feedback votes); public tallies
`{helpful, not_helpful}` plus private `my_choice` overlay; error codes
in the existing `community.<kebab>` / `feedback.<kebab>` style.
Shapes stay proposals until CS01 contract tests pin them; OpenAPI and
fixtures follow the freeze, not this file.

## Threat anchors (register detail lives in CS00/CS12 per plan)

Key farm vs register 10/h + keyed budgets; shared NAT vs rotating IP
digests (no permanent ban); forged cursor vs MAC+filter-hash+15min TTL;
oversize/malformed vs ValidateFeed + 3 MiB reserve + parser guards;
multipart/abandoned vs atomic reserve + reconciliation; tally inflation
vs unique pair + in-transaction recompute; brigading vs one-open-case +
audited ops (no auto-removal); stale offline ON vs server revocation
invalidating old grants; mock/stale/coarse fixes vs 100m/100m/s gates
with TEST_ONLY isolation. Residual: volumetric edge DDoS, coordinated
genuine accounts, compromised devices.

## Independent blocker (unchanged by this audit)

PR127 Android `test` failed at `0f3601c` with
`UncaughtExceptionsBeforeTest` in `CommunityPricesViewModelTest`
(run 38140418786). Fix `61f7c1b` (shared test dispatcher) is LOCAL_DONE
and pushed; remote checks pending. No merge on lucky green; wiki
`IMAGE_OUTSIDE_ASSETS` blocker is separate. CS phases do not depend on
PR127 except at final delivery.

## Validation

- `git diff --check`: clean.
- Secret/PII surface: this file contains no secrets, keys, tokens,
  account IDs, GPS, photos or raw IPs — constants only.
- No runtime, migration, VPS, device, CI, or dependency change in CS00.
- Exit met: testable scope/ownership above; no invented endpoint,
  limit, or live gate (proposals explicitly marked PROPOSED).

Next microtask: CS01 — helpfulness domain and contract (pure tests +
frozen reference/overlay/erase effects), starting with the observation
vs decision-sequence target decision recorded above.

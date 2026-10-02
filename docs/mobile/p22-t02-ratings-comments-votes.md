# P22-T02 — Ratings transport plus agreement-vs-confidence display

Status: LOCAL_DONE on `codex/phase-22-social-moderation`. Issue: #87. Entry P22-T01 satisfied (account journey LOCAL_DONE); P14/P17 comment/reply/vote contracts integrated and reused unchanged. Binds B-BR-F02, B-BR-C04 and BUC-C03. No migration, no new permission, no iOS work.

## What this task adds

- Backend ratings transport (the recorded P14 gap): `POST /v1/feedback/ratings` (1–5 stars, one current rating per account/target, idempotent converge, revision bump), `POST /v1/feedback/ratings/remove` (unknown keys → `rating-not-found`, no aggregate touch), `GET /v1/feedback/ratings/stats` (public anonymous read, exact count/sum, null mean when empty — never an invented score). Author derives server-side from the live session; every response is `no-store` with no account identifiers.
- Application `GetFeedbackPageUseCase.stats` (public read, no login; blank targets fail fast, transport fails closed as `Unavailable`).
- Android `FeedbackHttpClient.rate/deleteRating/stats` against the three routes with stable-kind mapping (`rating-out-of-range`, `rating-not-found`); the explicit `GATE_REQUIRED` refusal is gone — no fake path remains.
- `FeedbackViewModel.rate/deleteRating/loadStats` with local 1–5 guard (no IO), blank-session `SignInRequired`, stable-op-id retry, and new `Rated/RatingDeleted/StatsLoaded` states (rating outcomes no longer reuse comment states).
- `FeedbackDisplay.ratingLine` (floor one-decimal mean with exact count, `No ratings` when empty) kept apart from price confidence: stars describe personal experience (B-BR-F02), agreement describes comment validity (F06), neither certifies a pump price (B-BR-C04).

## Reused and verified (not rebuilt)

Comment/reply/edit/vote/report transport, use cases, outbox retry, 280-scalar portable rules and agreement math — all P14/P17-owned and untouched.

## Auth/critical gate (real PostGIS)

- Backend `feedback` unit + `-tags=integration` vs local PostGIS (project dev DB): all packages ok — rate/remove/stats handlers, golden vectors, existing rating/vote/comment stores.
- OpenAPI `vacuum lint` passes with the pre-existing 27 informs only.

## Explicitly deferred (not waived)

- Screen-level discussion section (`StationDetailSheet` community card / `Comunidade` hub wiring with agreement-vs-confidence copy): needs the station-identity mapping (list-row CNPJ → directory UUID target) before any consumer — next slice, not silently dropped.
- Real Google/Apple provider proof: P24-owned device evidence (per P22-T01).
- Report/moderation journeys: P22-T03.

## Validation

- `go test ./internal/modules/feedback/...` + `-tags=integration` 0-fail; `apicontract` golden vectors (4 new rating vectors) PASS; `vacuum lint` PASS.
- `:domain:test` 408 + `:application:test` 246 + `:data:testDebugUnitTest` 203 (1 pre-existing live-network skip) + `:app:testDebugUnitTest` 142, 0-fail (`--rerun-tasks`); `:app:assembleDebug` PASS.
- `git diff --check` clean; `scripts/scan-secrets.sh` PASS (no secrets/PII/GPS/photo content).

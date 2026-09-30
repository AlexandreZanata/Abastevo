# P14-T03 — Comments, replies and transport

Status: LOCAL_DONE on `codex/phase-14-station-feedback` (phase PR
#33, draft). Issue: #30 (slice 3 of 5; issue stays open until the
phase PR merges). No live-provider contact.

## Slice acceptance (frozen before coding)

- T03 (this commit) — bounded comments/replies plus the first
  feedback transport: `000024` (depth CHECK, tombstones, live
  indexes), author-scoped CAS edits, parent-validated one-level
  replies, keyset paging, session-authed writes with server-derived
  authors, anonymous reads with alias-only views, 6 OpenAPI paths +
  `FeedbackCommentView`, main wiring with narrow closures. Only
  active accounts write; no silent truncation; replies share 280.

## What changed

- `db/migrations/000024_feedback_comments.sql` (append-only) +
  8 owner queries + committed regeneration.
- `feedback/domain`: `StoredComment`/`CommentView`, `ErrComment-
  NotFound`/`ErrNotAuthor`/`ErrStaleRevision`/`ErrParentInvalid`/
  `ErrAuthorForbidden`/`ErrSessionInvalid` + verdicts. Gate
  failures map to author-forbidden (no account-status oracle);
  missing and foreign share one 404 (no ownership oracle,
  community precedent).
- `feedback/application`: `CommentStore`, `Service` (submit/
  reply/edit CAS/delete/view/list with limit clamp + opaque
  cursors), MemStore parity.
- `feedback/adapters`: transactional `PGStore` (lock → author/
  revision checks → mutate); `http` package (201/200 envelopes,
  400/401/403/404/409 mapping, no-store, verbatim text, proof
  never echoes).
- `contracts/openapi/v1.yaml`: `feedback` tag, view schema,
  6 paths; `vacuum` 0/0; 2 view vectors keep `apicontract` green.
- `cmd/api/main.go`: feedback service + session/account/station
  closures (no cross-module imports in the handler).

## RED → GREEN

- RED proven by dropping the MemStore author check in edits: a
  foreign edit proceeds instead of refusing not-author; GREEN on
  restore.
- `sqlc` inferred the keyset cursor as timestamptz: explicit
  `::uuid` casts fixed before any test ran green.
- OpenAPI first landed paths under `components.schemas` (19
  resolving errors) and collected 24 lint warnings across three
  iterations (`no-request-body` → POST noun path is repo
  convention; requestBody descriptions; array/media examples;
  global tag; HTTP verb in path → `remove`); final `vacuum`
  0 errors/0 warnings, each root-caused above, not hidden.

## Validation (exact commands, this host)

```sh
cd backend && GOTOOLCHAIN=go1.27.1 go build ./... && sqlc vet && sqlc generate
gofmt -l internal/modules/feedback cmd/api; GOTOOLCHAIN=go1.27.1 go vet ./internal/modules/feedback/... ./cmd/api/...
GOTOOLCHAIN=go1.27.1 go test -count=1 ./internal/modules/feedback/...
export ANPFUEL_TEST_DATABASE_URL=postgres://anpfuel:anpfuel@127.0.0.1:5434/anpfuel?sslmode=disable
GOTOOLCHAIN=go1.27.1 go test -count=1 -race -tags=integration ./internal/modules/feedback/...
vacuum lint -r contracts/openapi/vacuum-rules.yaml contracts/openapi/v1.yaml --no-update-check
(cd backend && GOTOOLCHAIN=go1.27.1 go test -count=1 ./internal/platform/apicontract/...)
make quick-verify
git diff --check
```

Outcome: unit green (text/ownership/revision/paging matrix);
integration with `-race` green (ownership flow, real-DB paging
walk, 16-way same-parent replies, verbatim markup round-trip);
`vacuum` 0/0/22 info; `apicontract` ok; `quick-verify`
selection=full ok; `diff --check` clean; secrets scan clean.

## Limits (not claimed)

- No votes/projections/jobs (T04); no moderation/privacy (T05).
- Reads are no-store; shared-cache policy deferred (edge work).
- Cursors are opaque but unsealed (tamper fails closed with
  400); sealed cursors stay a future hardening.

## Rollback

Append-only migrations stay; code rollback = delete comments
slices (domain records, application, adapters incl. http),
OpenAPI paths/view and main wiring. Ratings (T02) keep working.

## Next

P14-T04 validity votes + percentages (#31, same branch). Issue
#30 stays open until the phase PR merges.

# P14-T01 — Feedback domain and contracts

Status: LOCAL_DONE on `codex/phase-14-station-feedback` (phase PR
pending, first push with this commit). Issue: #28 (slice 1 of 5;
issue stays open until the phase PR merges). No live-provider
contact; no handlers, migrations or paths in this slice.

## Slice acceptance (frozen before coding)

- T01 (this commit) — frozen semantics: integer 1–5 ratings, one
  current rating per account/target; 280-scalar plain text with
  trim + CRLF normalization (replies share the limit, depth one);
  floor basis-point agreement with null for zero votes; station +
  fuel target shape; revision/audit rules documentary. Executable
  vectors in `contracts/testdata/feedback/` replayed by Go;
  Kotlin/Swift parity for rating + agreement (+ shared 280 text
  boundary); additive OpenAPI preview schemas (no paths).

## What changed

- `contracts/testdata/feedback/` — 18 golden vectors (text
  valid/280/281/empty/accents/emoji/combining/CRLF/invalid-UTF8,
  rating 5/1/0/6, agreement 2-1/0-3/3-0/0-0, revision
  supersede), every scalar count verified against normalized
  counting before commit.
- `backend/internal/modules/feedback/domain/` — stdlib-only
  values (`Rating`, `CommentText`, `Agreement`, `Target`),
  frozen bounds, stable verdict codes; fixture-shape +
  replay suites (vectors and code cannot drift).
- `domain/.../portable/PortableFeedback.kt` + test — range +
  floor math + null-zero + negative refusal + shared 280 hook.
- `iosApp/.../PortableFeedback.swift` + XCTest — 1:1
  transcription (precondition mirrors `require`);
  implemented-unverified per owner decision.
- `contracts/openapi/v1.yaml` — additive `FeedbackRating`,
  `FeedbackComment`, `FeedbackAgreement` (nullable bps for
  no-votes); 6 api vectors keep `apicontract` green.

## RED → GREEN

- RED proven by loosening the scalar bound by one: the 281
  fixture replays `ok` instead of `text-too-long`; GREEN on
  restore.
- Vector-count mismatches (3 hand-counted scalars) found and
  fixed pre-commit by programmatic verification.

## Validation (exact commands, this host)

```sh
cd backend && GOTOOLCHAIN=go1.27.1 go test -count=1 ./internal/modules/feedback/... \
  && GOTOOLCHAIN=go1.27.1 go test -count=1 ./internal/platform/apicontract/...
./gradlew :domain:test --no-daemon
bash scripts/check-mobile.sh
make quick-verify
git diff --check
```

Outcome: Go 7 suites incl. 18/18 fixture replays green;
Kotlin `PortableFeedbackTest` 5/5; `check-mobile` full ok
(Android green, Swift skip recorded outstanding); `vacuum`
0/0/14 info; `apicontract` ok; `quick-verify` selection=full
ok; `diff --check` clean; secrets scan clean.

## Limits (not claimed)

- No ratings/comments persistence, transport, jobs or
  projections (P14-T02…T04); no moderation/privacy (P14-T05).
- Swift compiles/runs on macOS only (release horizon).
- Platform string-model note: Go rejects invalid UTF-8; Kotlin
  counts lone surrogates singly; Swift counts scalars — vectors
  document the cases, each platform behaves per its model.

## Rollback

Pure addition (fixtures, one Go package, two Kotlin files, two
Swift files, preview schemas + vectors): deleting them restores
the tree. No migration, no flag.

## Next

P14-T02 rating transactions + aggregates (#29, same branch).
Issue #28 stays open until the phase PR merges.

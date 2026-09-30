# P15-T01 — Media budgets and forward contract

Status: LOCAL_DONE on `codex/phase-15-lightweight-photos` (phase PR
pending, first push with this commit). Issue: #34 (slice 1 of 5;
issue stays open until the phase PR merges). No live-provider
contact; no handlers, migrations or paths in this slice.

## Slice acceptance (frozen before coding)

- T01 (this commit) — frozen forward budgets: JPEG wire ≤150 KiB
  target/≤256 KiB hard cap, longest edge ≤1600, ≤2 MP integer
  bound, ≤3 encoding attempts, 32 MiB per-image working-memory
  hypothesis (measured on P12 devices in P15-T02), single 24 h
  deadline from server first receipt with no extension parameter
  by construction. Executable vectors in
  `contracts/testdata/media/budgets-v1.json` replayed by Go;
  deployed v1 (`image/jpeg`, 3 MiB, 24 h session TTL) untouched;
  OpenAPI 3 MiB limit untouched (changes in P15-T03 only).
  Protocol + notice target 24 h for all copies; migration/rollout
  compat: existing objects need append-only timestamp migration
  (P15-T04), no deadline reset on restore (M05), old 14/30-day
  retention replaced only when T04 passes.

## What changed

- `contracts/testdata/media/budgets-v1.json` — 9 golden vectors
  (target/cap exact, cap+1, edge+1, 2.01 MP, small, empty,
  deadline exact, retry-no-extension) with provenance.
- `backend/internal/modules/evidence/domain/budgets.go` —
  stdlib-only frozen constants (`Forward*`), `ValidateForwardWire`
  (no coercion; dimensions arrive server-measured in T03),
  `ForwardVerdictCode`, `ForwardDeadlineAfter` (no extension
  parameter by construction).
- `budgets_test.go` — frozen-constants guard, boundary matrix,
  9/9 fixture replay, deadline-no-extension; RED proven below.
- `docs/security/LOCAL_MEDIA_LOCATION_POLICY.md` — status notes
  budgets frozen; pipelines stay NOT IMPLEMENTED.

## RED → GREEN

- RED proven by loosening the cap by one (`256<<10 + 1`): the
  frozen-constants guard and the `cap-plus-one` fixture replay
  both fail (`bytes-over-cap` replays `ok`); GREEN on restore.
- Fixture/code drift is a failure by design: header or bound
  mismatch fails the replay before any pipeline reads it.

## Validation (exact commands, this host)

```sh
cd backend && GOTOOLCHAIN=go1.27.1 go build ./... && sqlc vet
gofmt -l internal/modules/evidence
GOTOOLCHAIN=go1.27.1 go vet ./internal/modules/evidence/...
GOTOOLCHAIN=go1.27.1 go test -count=1 ./internal/modules/evidence/...
git diff --check
```

Outcome: build + `sqlc vet` clean (no schema change);
`evidence/domain` 4/4 new suites green, whole `evidence/...`
green; `gofmt`/`vet`/`diff --check` clean; secrets scan clean.

## Limits (not claimed)

- No native pipeline (P15-T02), backend revalidation (T03),
  expiry enforcement (T04) or device acceptance (T05).
- 32 MiB working-memory figure is a hypothesis until P15-T02
  measures it on P12 low-resource devices.
- Kotlin/Swift budget parity belongs to P15-T02 native ports.

## Rollback

Pure addition (fixture, one Go file + test, doc status line):
deleting them restores the tree. No migration, no flag, no
behavior change to deployed v1 flows.

## Next

P15-T02 native lightweight photo pipeline (#35, same branch).
Issue #34 stays open until the phase PR merges.

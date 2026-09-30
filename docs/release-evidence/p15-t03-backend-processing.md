# P15-T03 — Bounded backend media processing

Status: LOCAL_DONE on `codex/phase-15-lightweight-photos` (phase PR
#39, draft). Issue: #36 (slice 3 of 5; issue stays open until the
phase PR merges). No migration, camera, storage or live-provider
contact in this slice.

## Slice acceptance (frozen before coding)

- T03 (this commit) — forward revalidation lane beside deployed
  v1: `media.InspectForward` (JPEG magic + strict structure +
  1600 edge / 2 MP header caps, zero pixel allocation on bombs),
  `media.SanitizeForward` (full decode + decoder-output re-encode
  across qualities 85/70/55, ≤3 attempts, >256 KiB refuses instead
  of crushing), `application.VerifyForward` (one download capped
  one byte past the 256 KiB server cap, claim bind, forward caps,
  sanitized JPEG upload, immutable copy expiry from first receipt
  = VERIFYING instant + 24 h). No client value widens anything:
  download bound, pixel caps, qualities and deadline are server
  constants; session MaxBytes never governs this lane. Worker
  concurrency stays serial per process (existing dispatcher:
  one claimed job at a time), so one image is ever in flight and
  the 2 MP header cap bounds its RSS; measured below. Cutover
  from v1 lands in P15-T04 with the timestamp migration; v1
  untouched here.

## What changed

- `adapters/media/forward.go` — `InspectForward`,
  `SanitizeForward`, `ForwardQualities` (len == 3 attempts).
- `application/verify.go` — additive `Outcome.ExpiresAt`
  (forward deadline; v1 leaves it zero).
- `application/forward_verify.go` — `VerifyForward` orchestration
  with stable v1 reason codes.
- Tests: `forward_test.go` (happy, 20000² bomb header, 1601
  edge, forged PNG/empty/truncated, EXIF strip + re-inspect,
  noisy over-cap refuse, 2 MP measured memory) +
  `forward_verify_test.go` (happy + expiry math, client-budget
  independence, corrupt/forged refusals, claim mismatch,
  terminal replay, interrupted upload).

## RED → GREEN

- RED proven by accepting the first encode unconditionally (cap
  check neutered): the noisy over-cap frame verifies instead of
  refusing `too-large`; GREEN on restore.

## Validation (exact commands, this host)

```sh
cd backend && GOTOOLCHAIN=go1.27.1 go build ./... && sqlc vet
gofmt -l internal/modules/evidence
GOTOOLCHAIN=go1.27.1 go vet ./internal/modules/evidence/...
GOTOOLCHAIN=go1.27.1 go test -count=1 ./internal/modules/evidence/...
GOTOOLCHAIN=go1.27.1 go test -count=1 -race -tags=integration ./internal/modules/evidence/...
git diff --check
```

Outcome: build + `sqlc vet` clean (no schema change); 6 new
media + 6 new application suites green; whole `evidence/...`
unit + `-race` integration green; measured 2 MP sanitize heap
growth 3.3 MB vs 32 MiB hypothesis (10× headroom);
`gofmt`/`vet`/`diff --check` clean; secrets scan clean.

## Limits (not claimed)

- Lane proven but not wired: v1 Verify serves traffic until the
  T04 cutover + `received_at` migration (UpdatedAt-at-VERIFYING
  is the documented first-receipt proxy here).
- 2 MP smooth-frame measurement is host-RSS evidence, not the
  P12-device + worker-RSS matrix (release horizon per owner
  decision pattern).

## Rollback

Pure addition (one media file + test, one application file +
test, one additive struct field): deleting them restores the
tree. No migration, no flag, v1 behavior unchanged.

## Next

P15-T04 expiry deletion and restore enforcement (#37, same
branch). Issue #36 stays open until the phase PR merges.

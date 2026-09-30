# P15-T05 — Media privacy and device acceptance

Status: LOCAL_DONE on `codex/phase-15-lightweight-photos` (phase PR
#39, draft). Issue: #38 (slice 5 of 5; issue stays open until the
phase PR merges). No camera, storage-config or live-provider
contact in this slice; device runs stay deferred per owner
decision, recorded — never claimed.

## Slice acceptance (frozen before coding)

- T05 (this commit) — G15 exit: real-PostGIS E2E of the full
  low-memory audit path (reserve → complete → forward verify →
  logical access expiry at the deadline with bytes present →
  sweep deletes bytes + marks rows → repeat converges), with
  restart-resume (fresh store handle), outage-then-retry and
  refusal log-redaction proofs; privacy notice reconciles
  deleted-photo vs retained-fact (bytes gone, fact + 90-day
  hashes stay, expired never advertised as available) and
  discloses the powered-off limitation; runbook freezes the
  same limitation. G15 evidence separates logical access expiry
  from actual deletion; nothing is claimed from an emulator.

## What changed

- `adapters/e2e_forward_test.go` (integration) —
  `TestForwardLifecycleEndToEnd` (full path incl. restart,
  outage/retry, convergence, audit-zero, row-survives) +
  `TestForwardRejectionsStayRedacted` (fixed-vocabulary
  reasons, no payload/key material in errors).
- `docs/backend/PRIVACY_NOTICE.md` — sanitized row now reads
  every-copy 24 h; new after-expiry and powered-off paragraphs;
  target note updated (media rows forward, account/social rows
  pending).
- `docs/operator/retention.md` — frozen powered-off limitation
  section.

## RED → GREEN

- RED proven by zeroing the forward `ExpiresAt`: the E2E
  first-receipt + 24 h assertion fails; GREEN on restore.

## Validation (exact commands, this host)

```sh
cd backend && GOTOOLCHAIN=go1.27.1 go build ./... && sqlc vet
GOTOOLCHAIN=go1.27.1 go test -count=1 ./internal/modules/evidence/... ./cmd/ops/
export ANPFUEL_TEST_DATABASE_URL=postgres://anpfuel:anpfuel@127.0.0.1:5434/anpfuel?sslmode=disable
GOTOOLCHAIN=go1.27.1 go test -count=1 -race -tags=integration ./internal/modules/evidence/...
./gradlew :domain:test :application:test :data:testDebugUnitTest :app:testDebugUnitTest :app:assembleDebug --no-daemon
bash scripts/check-mobile.sh
git diff --check
```

Outcome: backend unit + `-race` integration green (incl. the
2 new E2E suites); Android baseline BUILD SUCCESSFUL;
`check-mobile` full ok (Swift SKIP recorded outstanding);
`diff --check` clean; secrets scan clean.

## Limits (not claimed)

- No device run (Android/iOS hardware stays release-horizon);
  2 MP/32 MiB figures are host-measured until device matrices.
- Worker-RSS matrix and live-R2 overdue drill stay outstanding
  to release per owner decision pattern.

## Rollback

E2E test + doc paragraphs only (no behavior change): deleting
them restores the tree. No migration, no flag.

## Next

G15 exit review + phase closure (#38, same branch; issues
#34–#38 stay open until PR #39 merges).

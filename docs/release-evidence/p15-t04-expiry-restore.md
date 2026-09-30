# P15-T04 — Expiry deletion and restore enforcement

Status: LOCAL_DONE on `codex/phase-15-lightweight-photos` (phase PR
#39, draft). Issue: #37 (slice 4 of 5; issue stays open until the
phase PR merges). No camera, storage-config or live-provider
contact in this slice.

## Slice acceptance (frozen before coding)

- T04 (this commit) — all-copy 24 h enforcement replacing the
  14-day default / 30-day case cap for photo bytes (M01/M04):
  append-only `000029` (`received_at` first-receipt stamp,
  backfilled from `created_at`, sweep index; old media goes
  overdue on the next pass, no deadline reset); `RecordVerified`
  persists the VERIFYING instant; sweeper deletes finals past
  first-receipt + 24 h (extensions no longer move bytes; their
  columns stay as audit history); quarantine 24 h and 90-day
  hash purge unchanged (hashes under the separate privacy
  review, M06); at-deadline read denial in `ops evidence-url`
  even when bytes still exist; `AuditOverdue` failing-policy
  signal + reserve intake breaker (503 while overdue copies
  exist); restore replays the same convergent sweep before
  traffic (recovery runbook already requires retention first);
  DB backups carry keys/hashes only — photo bytes live in object
  storage outside backup scope by construction.

## What changed

- `000029_evidence_received_at.sql` + `sqlc.yaml` evidence
  stanza + `evidence.sql` (`InsertObject.received_at`,
  `StaleFinalCandidates`/`OverdueFinals` on
  `COALESCE(received_at, created_at)`, `GetObject.received_at`).
- `domain.ObjectRef.ReceivedAt` + `ReceivedOrCreated`.
- `adapters/store.go` — `ObjectData.ReceivedAt`,
  `RecordVerified` persists it, `StaleFinalCandidates` maps it,
  new `OverdueFinals`.
- `application/sweep.go` — `CopyRetention` 24 h single
  deadline, `FinalDue` on first receipt ignoring extensions,
  new `AuditOverdue`/`OverdueReport`.
- `application/reserve.go` — optional `Ports.Enforcement`
  breaker before quota burn + `ErrEnforcementUnhealthy`.
- `adapters/http/http.go` — 503
  `evidence.enforcement-unhealthy` mapping.
- `adapters/jobs/verify.go` — `ReceivedAt: sess.UpdatedAt`
  (completion intent = first receipt).
- `cmd/api/main.go` — breaker wired to `OverdueFinals` limit 1.
- `cmd/ops/evidence.go` — `evidenceCopyExpired` denial +
  `evidence_test.go` (deadline edges, backfill fallback,
  zero-stamp open).
- `docs/operator/retention.md` — hourly sweep row now reads
  every-copy 24 h.
- Tests: rewritten `TestFinalDuePolicy` (24 h, deadline edge,
  extension ignored, backfill fallback), `TestAuditOverdue`,
  breaker test, PG `TestPGReceivedAtAnchorsDeadline` (stale +
  overdue select, delete convergence) and
  `TestPGReceivedAtFallsBackToCreated` (NULL stamp path).

## RED → GREEN

- RED proven by restoring the old 14-day `FinalDue` rule: the
  25-hour-due, extension-ignored and backfill cases fail; GREEN
  on restore.

## Validation (exact commands, this host)

```sh
cd backend && GOTOOLCHAIN=go1.27.1 go build ./... && sqlc vet && sqlc generate
gofmt -l internal/modules/evidence cmd/api cmd/ops
GOTOOLCHAIN=go1.27.1 go vet ./internal/modules/evidence/... ./cmd/ops/ ./cmd/api/
GOTOOLCHAIN=go1.27.1 go test -count=1 ./internal/modules/evidence/... ./cmd/ops/
export ANPFUEL_TEST_DATABASE_URL=postgres://anpfuel:anpfuel@127.0.0.1:5434/anpfuel?sslmode=disable
GOTOOLCHAIN=go1.27.1 go test -count=1 -race -tags=integration ./internal/modules/evidence/...
git diff --check
```

Outcome: build + `sqlc vet` clean (`sqlc generate` no diff
beyond the new queries); unit green (rewritten policy,
audit, breaker, ops expiry); `-race` integration green
(incl. received-at select/overdue/convergence + NULL
fallback); `gofmt`/`vet`/`diff --check` clean; secrets scan
clean.

## Limits (not claimed)

- Worker-RSS matrix on P12 devices stays release-horizon
  (host-RSS evidence in T03).
- Hash 90-day bound unchanged (separate privacy review, M06).
- No live-R2 overdue drill (synthetic storage fakes + PG).

## Rollback

Migration `000029` is additive (column + index + backfill);
code rollback = revert policy/store/read/breaker additions and
docs. Old columns stay; v1 14/30-day semantics do NOT return
without a new reviewed change.

## Next

P15-T05 media privacy and device acceptance (#38, same
branch). Issue #37 stays open until the phase PR merges.

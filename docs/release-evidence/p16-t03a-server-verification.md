# P16-T03A — Server-side location verification

Status: LOCAL_DONE on `codex/phase-16-location-integrity` (phase PR
#44, draft). Issue: #42 (slice 1 of P16-T03; issue stays open
until the phase PR merges). No intake wiring, migration, PostGIS
query or live-provider contact in this slice.

## Slice acceptance (frozen before coding)

- T03A (this commit) — server distrust policy:
  `VerifyDeviceFix` recomputes the frozen verdict from claimed
  metadata on the **server receipt clock** and refuses any
  claimed verdict that differs (`forged-location-claim`, fixed
  vocabulary). Freshness/skew derive from receipt-minus-captured;
  there is deliberately no client-claimed age field, so a claim
  without a capture instant can never verify. Honest
  non-verified states (SIMULATED/MANUAL/DENIED) pass through
  without granting claims. `TeleportRisk` flags implausible
  station-to-station jumps (> 100 m/s, conservative) for review,
  never auto-reject. Refusals and verified outputs serialize to
  fixed codes/bands only: coordinates never persist, never log.
  12 claim + 6 teleport vectors in
  `contracts/testdata/location/intake-v1.json` replayed by Go.
- T03B (next) — persistence (location bands migration),
  submit intake + PostGIS proximity/teleport, API vectors,
  DeriveSignals wiring, HIGH-gate proof.

## What changed

- `contracts/testdata/location/intake-v1.json` — 12 claim
  vectors (fresh match; forged mismatch/flag/source; stale,
  replayed, future, skew-over, timeless; honest simulated/
  manual/denied; garbage verdict) + 6 teleport vectors.
- `backend/internal/modules/community/application/location_verify.go` —
  `DeviceFix` (no client age by design), `VerifyDeviceFix`
  (recompute + server clock + fixed refusal), `FixRejection`,
  `TeleportRisk` (100 m/s frozen).
- `location_verify_test.go` — fixture replay, receipt-time
  requirement, coordinate-leak scan over refusals and outputs.

## RED → GREEN

- RED proven by disabling the verdict-match gate: forged,
  stale, replayed, skewed, timeless and even garbage-`TRUSTED`
  claims verify with `AllowsClaim:true`; GREEN on restore.

## Validation (exact commands, this host)

```sh
cd backend && GOTOOLCHAIN=go1.27.1 go build ./... && sqlc vet
gofmt -l internal/modules/community
GOTOOLCHAIN=go1.27.1 go vet ./internal/modules/community/...
GOTOOLCHAIN=go1.27.1 go test -count=1 ./internal/modules/community/...
git diff --check
bash scripts/scan-secrets.sh
```

Outcome: build + `sqlc vet` clean (no schema change); 3 new
suites green (12/12 claim + 6/6 teleport replays); whole
community module green; `gofmt`/`vet`/`diff --check`/secrets
clean.

## Limits (not claimed)

- Policy proven, not wired: no submit intake, no persisted
  bands, no PostGIS proximity yet (T03B); HIGH-gate proof lands
  with the wiring.
- Within-window fix reuse converges on observation idempotency
  by design (no fix fingerprint store, per privacy review).

## Rollback

Pure addition (fixture, one Go file + test): deleting them
restores the tree. No migration, no flag, no behavior change to
existing intake (DeriveBands untouched).

## Next

P16-T03B persistence + intake + PostGIS (#42, same branch).
Issue #42 stays open until the phase PR merges.

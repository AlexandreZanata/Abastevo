# P33-T01 — Adversarial, resource and review-operations campaign (code slice)

Branch: `codex/phase-33-profile-acceptance` (base P32 `acb28db`). Frozen workload: 50-claim rival burst (10 accounts × 5 keys), 20-way same-key divergent burst, 15 proof-parser vectors + 200-mutation fuzz; synthetic fixtures only, no live/provider/load campaign, no production data.

## Findings fixed (not deferred)

1. **Publish-window misleading 404 (real, burst-triggered):** under same-key burst, a same-request replay could observe the claim row before its first declaration published and receive `ErrClaimNotFound`. Fixed with `ErrClaimBusy` (retryable transient, same request safe to replay) in `application/claim.go`, mapped to HTTP 409 `claim.busy-retry` in `adapters/claims_http.go`. Before: 3/10 suite runs failed with leaked not-found; after: 10/10 `-race` runs GREEN.
2. **Harness data races (test-only):** `NewID` counters (`n++`) in `claim_test.go`/`review_test.go` raced under concurrent tests (production uses thread-safe UUIDs). Fixed with `atomic.Int64`; this also explains the earlier intermittent `TestReviewConcurrentApprovalsConverge` race report.

## Tests added

- `application/adversarial_test.go`: rival burst stays private (50 created, per-account sets disjoint, 50 unique, zero cross-account visibility); divergent burst yields exactly 1 created, divergent replays conflict (≥1, silent merge forbidden), same-request window replays get Busy-or-replay (never not-found).
- `domain/proof_adversarial_test.go`: 15 vectors (empty/oversize/magic-mismatch/PFX/P12/PEM/KEY/encrypt/JS/embedded/launch/XFA/key-markers/password) all refused typed; 200-mutation fuzz, no panic, ~1s.
- `adapters/claims_busy_test.go`: forged publish window → HTTP 409 + `claim.busy-retry` body.

## Validation

- `go test -race -count=1 ./internal/modules/stationprofile/...` — 5/5 pkgs ok; application pkg 10/10 consecutive `-race` runs.
- `go vet`, `gofmt -l` clean; `bash scripts/check-compat.sh` ok (additive error only, no wire break); `git diff --check` PASS; `scan-secrets.sh` PASS (synthetic tripwires only).
- Review-operations throughput/staffing drill and provider/device rows stay OWED (P33-T02/T03 + conjunto closure); G09 owns production certification.

## Limits / next

No migration/SQL/OpenAPI change. P33-T02 end-to-end authority/privacy/device reacceptance next. No PR/CI/merge/wiki per batch closure. iOS archived; G09 UNCERTIFIED.

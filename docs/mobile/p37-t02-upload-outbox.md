# P37-T02 — Live upload and outbox lifecycle

Status: LOCAL_DONE on `codex/phase-37-live-contributions`. Task: P37-T02.
Binds idempotent retry (stable `client_submission_id`, fresh nonce per
send), owner status/cancel and process recovery (existing outbox +
worker contracts). Additive; no backend change; no device run.

## Behavior (DDD)

- `CaptureOcrViewModel.submitConfirmed()` closes the missing
  capture→outbox link: requires the `Confirmed` state **and** the T01
  canonical target, rejects cross-fuel misattribution (target wire vs
  confirmed product), then enqueues metadata-only
  (`EnqueueContributionUseCase`, photo plumbing stays future work and is
  labelled honestly by the use case). Outcomes: `NoTarget`,
  `FuelMismatch`, `Disabled`, `Queued(historical)`, `Failed(reason)` —
  a transient network failure stays `Failed` and never becomes
  ACCEPTED. Identity proof travels at worker submit time through the
  existing contribution gateway (reserve → presigned PUT → complete →
  observation with `Idempotency-Key`/`X-Nonce`).
- `CaptureScreen` Confirmed branch gains an explicit Send button plus
  honest submit states (queued / failed-with-reason / invalid target);
  Done stays a plain back action. en/pt-BR strings +3.
- Downstream (verified, not rebuilt): Room outbox enqueue/status/
  cancel, `ContributionWorker` retry with fresh nonce per send (object
  success + finalize failure stays FAILED and retries the same id —
  never a duplicate observation), and app-restart recovery (existing
  P21-T03/P22-T04 suites, rerun below). No demonstrated backend/
  contract defect was found, so no backend fix precedes the consumer.

## Validation

- GREEN: `CaptureOcrViewModelTest` +2 (no-target stays `NoTarget`
  before/after confirm; mismatch rejected, matching target enqueues
  non-historical) → suite PASS. Fixed two `operator`-modifier call
  sites (`enqueue.invoke`) — no weakened assertions.
- Regression: full `:app:testDebugUnitTest` capture subset PASS;
  `:application:test` + `:data:testDebugUnitTest` contribution/outbox
  subsets PASS; `:app:assembleDebug` PASS (Hilt resolves the new
  `EnqueueContributionUseCase` injection).
- Contracts unchanged: `vacuum lint` + `apicontract` PASS; live `curl`
  60 unchanged (no bypass).
- `git diff --check` PASS; `scan-secrets.sh` PASS.

## Limits and next

- BLOCKED_LIVE: upload negotiation/bytes/completion against staging
  unverified from this runner (TLS trust gap); identity-proof live
  exercise owed. BLOCKED_MEDIA (see T03): no attached VPS private
  storage, so photo bytes stay out of the live path.
- OWED: end manual/device batch, provider/media proof. No PR/CI/merge/
  wiki per ADR-018. iOS archived. G09 UNCERTIFIED.
- Next: P37-T03 private storage and all-copy expiry on this branch.

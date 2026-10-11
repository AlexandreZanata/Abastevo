# SEC06-D — Retry, refusal and attempt budgets

Date: 2026-10-11. Status: LOCAL_DONE. Scope: attempt cap/park,
deterministic jitter, Retry-After honor, permanent-vs-transient
persistence — no sync-flow change, no measurements (E). Head:
maintained `dev` + this task.
Plan: [remediation plan](../../../planning/SECURITY_REMEDIATION_PLAN_2026-10-11.md)
(SEC06, AUD-11). Contract: frozen revision/lease/owner/origin
semantics; stable operation id with fresh proof per attempt.
Constraint: closed permanent-refusal set by review, never extended
silently; parked rows keep history for status/retention.

Validation: PASS

## Changes

- `application/.../PortableOutbox.kt`: `MAX_DISPATCH_ATTEMPTS = 25`
  (hourly-capped backoff ⇒ more than a day of retries, then park:
  attempts pinned, tick `MAX_VALUE`, never eligible again);
  deterministic ±25% jitter by command/attempt hash (fleet
  de-synchronization, test-stable, no RNG plumbing).
- `domain/.../ContributionOutboxPorts.kt`: `ContributionRateLimited`
  (429 hint, transient family); `failDispatch` accepts an optional
  server hint (bounded by the backoff ceiling); `refusePermanently`
  parks with a stable reason; `OutboxBudgets` unchanged.
- `data/.../PhotoProofTransport.kt`: 429 maps to the hint
  (negative/malformed/absent fall back to plain refusal, existing
  backoff applies); `mapErrorStatus` is unit-pinned.
- `data/.../RoomContributionOutboxRepository.kt` + DAO
  `markRefused`: hint override tick, permanent park with reason
  (live claims only, second park is a no-op).
- `DispatchContributionUseCase`: honors the hint; permanent
  allowlist (`owner-or-origin-changed`, `invalid-or-unscoped`,
  `invalid-dispatch-receipt`) parks distinctly, everything else
  keeps transient backoff; terminal photo states unchanged.
- Per-run budgets (pinned, pre-existing load-bearing): one
  command per worker run, unique work per command id, single
  nonce per attempt; gateway `checkScope` refuses foreign-owner
  sends; logout/deletion purge rides SEC04-D.
- Tests: cap/park/jitter ranges + determinism; hint honored and
  capped at store level; permanent-vs-transient persistence;
  fresh nonces under stable operation id; transport mapping
  matrix; FIFO fairness across two owners; pre-existing logout,
  lost-ACK (durable receipt refresh), cancel-race and
  scope-change suites re-green.

## Validation (actual)

- `:domain:test`, `:application:test`, `:data:testDebugUnitTest`
  (full), `:app:testDebugUnitTest` + `:app:assembleDebug` green —
  incl. 8 new tests.
  `git diff --check` clean; no secrets/PII (synthetic scopes).
- RED pre-GREEN: attempts grew unboundedly (30th retry still
  scheduled); 429 hints died in transport mapping; permanent and
  transient refusals shared one undistinguished FAILED row.

## Limits / owed

- Parked rows keep FAILED display (no new state/enum/schema);
  resubmission mints a new command (documented path).
- Byte budgets: payload intents stay small by construction;
  photo bytes ride the server 3 MiB verification (SEC03-B).
- Measured histories: SEC06-E. No production claim.

Next microtask: SEC06-E measured histories.

# SEC07-C — Android refresh serialization

Date: 2026-10-11. Status: LOCAL_DONE. Scope: client refresh
serialization + denial mapping only (no transport/backend change).
Head: maintained `dev` + this task.
Plan: [remediation plan](../../../planning/SECURITY_REMEDIATION_PLAN_2026-10-11.md)
(SEC07, AUD-08/09). Contract: one flow serializes load/network/save
against logout behind the Android mutex; one ambiguous reply costs
exactly one API call (retries are explicit caller decisions, never
transparent replays); unavailable authority keeps the stored session
while permanent denials clear it; account/session/owner checks fail
closed. Constraint: no token logging; portable session stays
`java.*`-free.

Validation: PASS

## Changes

- `domain/.../PortableAuth.kt`: `Verdict.SESSION_INVALID`
  (`session-invalid`, mirrors the SEC07-A backend verdict).
- `application/.../PortableAuthFlow.kt`: `isAuthDenial` gains
  `SESSION_INVALID` — a wrong stored credential clears local
  authority without the expired-family key fallback (the family may
  be live; the stored token is doomed).
- Tests: `Sec07cRefreshSerializationTest` (5 new) pins serial refresh
  (8 racing callers, 50ms API dwell, max overlap 1, one call each),
  refresh/logout serialization, ambiguous `unavailable` costing one
  call with the session kept, the six-verdict denial matrix clearing
  store+keys, and foreign-account refresh clearing as CLIENT_INVALID.
  Existing denial matrix extended with `SESSION_INVALID`.

## Validation (actual)

- `:application:test` + `:domain:test` (full) green — incl. 5 new.
- `git diff --check` clean; no secrets/PII (synthetic sessions).
- RED pre-GREEN: the same 8-racer proof behind
  `DirectAuthOperationLock` overlaps fully (maxOverlap=8, probe since
  removed); with the mutex shape it pins at 1. `SESSION_INVALID`
  previously fell through to keep-the-doomed-token.

## Limits / owed

- JVM proof mirrors the Android mutex shape
  (`SerializedAuthOperations`); on-device scheduling stays with the
  caller coroutines (unchanged).
- Explicit caller retries after ambiguity replay the same token by
  caller decision and may hit server reuse-revocation — recorded,
  not transparent.
- No transport/backend change. No production claim.

Next microtask: SEC08-A location assurance policy.

# CS11C — Per-photo training opt-in at review

Date: 2026-10-11. Status: LOCAL_DONE. Scope: opt-in state,
checkbox row, strings and JVM tests only — no screen placement,
no submission wire, no Portuguese copy. Head: maintained `dev` +
this task.
Plan: [phased plan](../../../planning/COMMUNITY_SAFETY_EXECUTION_PLAN.md)
(CS11, B-BR-CS06), [CS11B view](CS11B_CONSENT_VIEW.md). Rules:
B-BR-CS06, BUC-CS05.

Validation: PASS

## Added

- `TrainingPhotoOptInState`: every photo starts disallowed;
  checks apply only while server consent is granted (a stale ON
  never authorizes selections); unchecks always apply; clearing
  consent wipes checks; blank ids never store. Lock-guarded for
  review gestures from any thread; the future queue producer
  reads `allowedIds` together with the current server consent,
  which stays authoritative at commit time.
- `TrainingPhotoOptInRow`: explicit per-photo checkbox at
  review — unchecked by default, disabled with the gate
  explanation without granted consent (`onChange` never fires a
  check then). State hoists to the caller; TalkBack via a pure
  description helper; MaterialTheme type scales, no fixed
  heights.
- 2 English strings (values/ stays English-only, CS08B
  precedent; wired only to delivered behavior).

## Validation (actual)

- `TrainingPhotoOptInStateTest` 6/6 (default-off start,
  gated check, refusal without consent, uncheck paths,
  consent-cleared wipe, blank ids). RED before GREEN
  (compile failure on the missing state).
- `TrainingPhotoOptInA11yTest` 4/4 (allowed/gated/plain/
  blank composition).
- Full `:app:testDebugUnitTest` and `:app:assembleDebug`
  green (no regressions). `git diff --check` clean. No
  PII/secrets in code, strings or this file. Device matrix
  stays CS14-owned.

## Limits / owed

- Screen placement in the capture review flow and host
  wiring: screen work, not started.
- Submission wire for per-photo flags + queue producer: the
  flags ride the future producer payload; the server enforces
  current-consent + scope at commit regardless (CS10A
  contract), so an unwired flag authorizes nothing.
- Portuguese product copy: translation pass later. Verified
  production controller/contact/processors: still none —
  provisional digest stands.
- Soak SLO proof after 2026-10-12T16:11:11Z: unchanged.

Next microtask: soak SLO proof after the wall clock (then CS12 integration).

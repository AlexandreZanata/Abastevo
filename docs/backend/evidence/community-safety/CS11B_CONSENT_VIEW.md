# CS11B — Training-consent ViewModel, panel, DI, strings

Date: 2026-10-11. Status: LOCAL_DONE. Scope: ViewModel, panel,
strings, DI bindings — no screen placement, no per-photo
plumbing, no Portuguese copy (CS11C). Head: maintained `dev` +
this task.
Plan: [phased plan](../../../planning/COMMUNITY_SAFETY_EXECUTION_PLAN.md)
(CS11, B-BR-CS06), [CS11A client](CS11A_CONSENT_CLIENT.md). Rules:
B-BR-CS06, BUC-CS05.

Validation: PASS

## Added (mirrors the helpfulness stack, CS08B)

- `TrainingConsentViewModel`: Guest (blank account, zero IO) /
  Loading / Content (server-exact granted/state/terms/revision
  plus pending flag) / Updating / Offline / Pending (queued
  offline revocation, explicit and distinct) / Rejected (fixed
  kind + sanitized message). `prepare` syncs pending first and
  renders the server state; the toggle grants/revokes then
  reloads so the switch never shows a claim the server does not
  hold; single flight (second tap ignored); retry reloads.
- `TrainingConsentPanel`: switch with server state, terms line,
  regrant notice, pending notice, deletion note and rights
  entry; guest renders nothing (account state owns login);
  Offline/Pending/Rejected cards with retry. TalkBack via a pure
  description helper (switch + server state + terms + pending);
  MaterialTheme type scales for large text, no fixed heights.
- 11 English strings (values/ stays English-only, CS08B
  precedent; wired only to delivered behavior).
- DI: client provider, local-store/gateway binds, use-case
  provider (all modules compile; Hilt KSP green).

## Validation (actual)

- ViewModel 8/8 (guest silence with zero use-case calls,
  foreground content load, grant-then-reload, revoke-then-
  reload, single flight, offline + retry, pending explicit,
  fixed-kind rejection) with `runTest(dispatcher)`.
- A11y 4/4 (granted/pending/off/blank composition).
- Full `:app:testDebugUnitTest`, `:data:testDebugUnitTest` and
  `:app:assembleDebug` green (no regressions; bindings resolve).
- `git diff --check` clean. No PII/secrets in code, strings or
  this file. Device matrix stays CS14-owned.

## Incident: KSP false-start (resolved, no code impact)

One-shot scripted insertion of the use-case provider tripped
`kspDebugKotlin` with `NonExistentClass` cascading over the
module. Bisected piece by piece (baseline green; return type,
each param, name and annotations each green alone) — the
byte-identical method re-applied through a verified edit
builds clean, and the full provider is present and compiling.
Likely a transient incremental file state from the scripted
edit; kept the lesson (verified edits only) and the green
tree. Hilt KSP + full compile prove the bindings.

## Limits / owed

- Screen placement in Settings and host wiring: screen work,
  not started (no per-screen network calls added).
- Per-photo opt-in plumbing at review: CS11C. Portuguese
  product copy: translation pass later with CS11C.
- Verified production controller/contact/processors: still no
  verified values — provisional digest stands. `.invalid`
  contact stays staging-only.
- Soak SLO proof after 2026-10-12T16:11:11Z: unchanged.

Next microtask: CS11C — per-photo opt-in at review (then soak proof).

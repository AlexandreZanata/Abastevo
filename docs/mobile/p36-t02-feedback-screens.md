# P36-T02 — Station and fuel feedback screens

Status: LOCAL_DONE on `codex/phase-36-live-community`. Task: P36-T02.
Binds B-BR-C04 (vote agreement separate from price confidence),
280-scalar one-level replies, author ownership and rate limits
(existing P14/P22 transports reused unchanged). Additive; legacy ANP
detail untouched; no device run.

## Behavior (DDD)

- Domain `feedback/FeedbackTarget`: canonical social target (UUID
  station + wire fuel + account, blank = honest guest). Legacy CNPJ
  rows and unknown wire products are refused explicitly — no manual
  station-ID textbox, no trust bonus, no moderator power.
- `StationFeedbackSection` (new, embedded in
  `ServerStationDetailSheet` when a wire fuel is present, else the
  honest pending card stays): prepares `FeedbackViewModel` with the
  resolved UUID target, loads first page + stats; renders 1–5 stars,
  280-char input with live remaining count, comment list with
  Helpful/Not-helpful votes + report (signed only), stats line via
  `ratingLine`, queued-with-retry and rejected-with-retry states, and
  the guest note for anonymous readers. Invalid targets render an
  honest note, never an invented thread.
- `StationsScreen` passes `WireFuelMapper.toWire(selectedFuelProduct)`
  + `uiState.serverAccountId` (guest default; session binding is
  P36-T03). en/pt-BR strings added (12 each side incl. T02 extras).
- No new trust semantics: author-vote denial, unique vote/change
  replay, revocation and hidden-content rules stay in the reused
  transports/use cases (existing suites prove them).

## Validation

- RED→GREEN: `FeedbackTargetTest` failed compilation before
  `FeedbackTarget.kt` existed; GREEN 3/3 after (signed/guest/CNPJ +
  wire rejections).
- Regression: `:app:testDebugUnitTest --tests
  ...ui.stations.* --tests ...community.*` PASS (incl. untouched
  `StationsViewModelTest` + `FeedbackViewModelTest`); `:domain:test`
  PASS; `:app:assembleDebug` PASS (Compose + Hilt + resources).
- Contracts (unchanged backend): `vacuum lint` + `apicontract` PASS;
  live `curl` 60 unchanged (no bypass). Screen-level discussion
  device proof stays owed to the end manual batch.
- `git diff --check` PASS; `scan-secrets.sh` PASS.

## Limits and next

- BLOCKED_LIVE: discussion reads/writes unverified from this runner
  (TLS trust gap). OWED: device-level discussion proof + provider
  session proof at the end manual batch.
- OWED: end manual/device batch, media proof. No PR/CI/merge/wiki per
  ADR-018. iOS archived. G09 UNCERTIFIED.
- Next: P36-T03 private activity and account rights on this branch.

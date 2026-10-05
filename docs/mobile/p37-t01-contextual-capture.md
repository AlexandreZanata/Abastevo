# P37-T01 — Contextual capture and review

Status: LOCAL_DONE on `codex/phase-37-live-contributions`. Task: P37-T01.
Binds one-explicit-submit/no-silent-publication (P21-T02 intact),
existing KiB/one-image budgets (P15) and location-integrity contracts
(P16). Additive; legacy context-free FAB flow preserved; no device run.

## Behavior (DDD)

- Domain `contribution/ContributionTarget`: canonical capture target
  (UUID station + wire fuel). Legacy CNPJ rows and unknown wires are
  refused; the target fixes *where* an observation lands, never the
  price/condition (still explicitly confirmed). Account proof travels
  separately at submit (P37-T02).
- `CaptureOcrViewModel.bindTarget(stationId?, fuelWire?)`: null clears
  to the context-free flow; invalid sets honest `targetInvalid`
  (capture blocked, no misattribution). New `target`/`targetInvalid`
  flows; confirm/manual paths untouched.
- `CaptureScreen` accepts optional `stationId`/`fuelProductWire`,
  binds on launch, shows the target banner (`Posting to station
  <short> · <wire>`) and short-circuits invalid targets with a back
  action. Review/confirm UI unchanged (candidates, manual entry,
  explicit fuel + condition picks, one explicit confirm).
- Navigation: `Routes.capture(stationId, fuelWire)` builds
  `capture?stationId=&fuel=`; `CAPTURE_WITH_TARGET` composable parses
  args (empty → null → context-free). Server detail
  `onUpdatePrice` now routes with UUID + selected fuel wire;
  legacy/FAB entries keep the context-free route. en/pt-BR strings +2.

## Validation

- RED→GREEN: `ContributionTargetTest` failed compilation before the
  value existed; GREEN 2/2 after (valid + CNPJ/wire/blank rejections).
- GREEN: `CaptureOcrViewModelTest` +1 (valid binds, CNPJ invalid,
  null clears) → suite PASS; `Routes`/navigation suites PASS;
  `:app:assembleDebug` PASS.
- Regression (media budgets intact): `:domain:test` + `:data:
  testDebugUnitTest` photo/OCR/capture subsets PASS; backend
  `evidence/...` unit PASS (the one alarming `FAIL` line during the
  session was my own mistyped module path
  `internal/modules/observations/...`, not a code failure — rerun
  without it is green). Huge/invalid-image and bounded-allocation
  behavior stays covered by the existing P15/P21 suites (no new media
  semantics introduced here).
- Contracts unchanged: `vacuum lint` + `apicontract` PASS; live `curl`
  60 unchanged (no bypass).
- `git diff --check` PASS; `scan-secrets.sh` PASS.

## Limits and next

- BLOCKED_LIVE: capture→submit→upload exercise unverified from this
  runner (TLS trust gap). OWED: low-end encode measurement + device
  camera proof at the end manual batch.
- OWED: end manual/device batch, provider/media proof. No PR/CI/merge/
  wiki per ADR-018. iOS archived. G09 UNCERTIFIED.
- Next: P37-T02 live upload and outbox lifecycle on this branch.

# MEX-32 — Shared loading, refresh policy and concise copy reconciliation

Status: LOCAL_DONE
Validation: PASS
Integration: INTEGRATION_PENDING

Parent: `188c2cdb` (maintained dev, EE-24). Date: 2026-10-23.
Depends MEX-06/13/15/27/31; owns OPT-A00–A05 shared changes.
Rules B-BR-MEX-05/17/18; BUC-MEX-09/10. Risk: lifecycle/cache.

Before behavior: one accepted community policy already drives the city
feed and Home summary from lifecycle/event/manual triggers with
single-flight, bounded retry and offscreen/offline stop. The remaining
surfaces must not restart an equivalent active request on duplicate
triggers, must keep their loaded windows until a replacement succeeds,
and must not turn the server's advisory `refresh_after_seconds` into a
fixed promise. The UI must not claim a 15-second refresh anywhere.

## Delivered

- Community summary/feed decode now accepts any bounded advisory
  `refresh_after_seconds` (0..3600) instead of requiring exactly 15;
  the value still never drives refresh policy. Cache snapshots record
  the observed server hint and decode through the same bounded path.
- `ChargingExploreViewModel.refresh` is single-flight: repeated taps
  converge on the active pack read and the previous rows stay visible
  until the new generation commits.
- `OccurrenceExplorerViewModel.refresh` no longer restarts an active
  equivalent first-page read; filter/scope changes still cancel because
  they are a different query.
- Verified all eight locale files contain no fixed 15-second refresh
  claim. Existing OPT-A community policy and pagination behavior were
  not duplicated or rewritten.

## Actual checks

- Charging test: two refresh triggers while a gated repository read is
  active call the repository once; completing the gate clears the
  refreshing state.
- Occurrence test: duplicate refresh during an active first page keeps
  one gateway call and still renders the page.
- Community summary test: hint 30 accepted; -1 and 3601 refused;
  existing identity/unit/expiry negatives remain green.
- `:app:testDebugUnitTest` charging + occurrence PASS.
- `:data:testDebugUnitTest` community summary PASS.
- `:app:compileDebugKotlin :data:compileDebugKotlin` PASS.
- Existing community policy tests from OPT-A remain unchanged/green in
  the affected selector.
- `git diff --check` and secret scan PASS.

## Limits

- No new background scheduler or polling was added. Device lifecycle
  and physical slow-request evidence remain OWED under the standing
  deferral.
- Byte/SQL measurement belongs to EE-28/MEX-35; this unit makes no
  latency or byte claim.

Next: EE-26 EV-source aging and cache/status acceptance.

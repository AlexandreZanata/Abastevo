# MEX-34 — Official source-to-mobile acceptance

Status: LOCAL_DONE
Validation: PASS
Integration: INTEGRATION_PENDING

Parent: `c9c7d13a`. Date: 2026-10-23.
Depends MEX-17–27/31/32. Risk: attribution/loss/coverage.

## Actual chain

- Pinned private enforcement workbook
  (`dados-fisc-a-partir-2019.xlsx`, 16,188,087 bytes, SHA-256 match),
  independent census `/tmp/abastevo-mex20f-census.json` and the trusted
  nonroot preparer binary.
- `go test -tags=integration ./cmd/worker -run
  '^TestEnforcementPinnedWholeWorker'` PASS in 60.4 s with a fresh
  disposable PostGIS: native preparer → Go bridge → source lease →
  bounded worker → PostGIS publication, replay/correction and full
  traversal against the frozen census. Source flags remain OFF.
- Preparer identity/census assertions refused the wrong workbook first
  (2019 file), then passed only with the pinned 16,188,087-byte
  workbook; no partial publication was accepted.
- Android occurrence/public selectors
  (`domain.occurrence`, `data.remote.occurrence`,
  `app.ui.occurrence`, `app.ui.stations`) PASS on the frozen typed
  contracts; server-revision/stale/empty/missing and filter/detail
  behavior remain covered by MEX-25/26/27 tests.

## Limits

- Android decoding of the exact whole-source pages, physical
  Room/reopen and TalkBack/font runs were not executed in this session;
  they remain OWED. No live official-source activation, no numeric
  trust calibration, no production data, no deployment.
- Census is source acceptance, not a claim of complete Brazilian
  coverage or live availability.
- MEX-20f already proved the whole-source artifact and privacy counters;
  this unit re-ran the pinned worker chain and the mobile-side typed
  selectors without altering production flags.

Next: EE-28/MEX-35 measurements.

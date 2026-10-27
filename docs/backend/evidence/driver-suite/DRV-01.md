# DRV-01 — Exact kilometre input and financial presentation

Status: LOCAL_DONE
Validation: PASS
Integration: INTEGRATION_PENDING

Parent: e9f1ce25. B-BR-DRV-02/03/11. Existing ledger regression repair only; no shift, GPS or earnings feature introduced.

## Actual RED and GREEN

Two meaningful existing-form tests ran before implementation (not just compile failures): km readings 10000/10500 persisted [10000,10500], failing expected [10000000,10500000]; comma input 12345,6 returned ODOMETER_INVALID. `/tmp/drv01-red.log` and the original test XML captured these failures.

Added exact domain LedgerDistance with existing bounds, comma/dot ungrouped decimal, no floating point, metre precision and symmetric localized edit values. All three refuel/expense/charge editors use the conversion; charge preserves its stricter 1e9-metre ceiling. A real form→draft→actual GetMonthlyIndicatorsUseCase test proves 500000 metres, 25/2 km/L and 52 centavos/km after two full fills, not a duplicated test formula. Reopening 12345,6 km and saving again preserves exactly 12345600 metres and one record.

LedgerValueFormatter converts internal rational centavos to BRL before one final HALF_UP rounding, and shows localized decimal efficiency instead of raw fractions. Both fuel and combined cost cards use it. Existing eight locale labels identify purchases/km, not an invented exact inventory-consumption cost. Old financial-v1/canonical raw values stay unchanged.

A third RED exposed an adjacent actual decimal overflow: 184467440737095516.16 in the centavo parser returned 0 through Long truncation. Exact longValueExact now refuses wraparound in the existing quantity/money/rate helpers. The regression also covers a huge quantity/rate previously able to wrap into a plausible small value. No ceiling was raised.

## Verification

Focused domain/app tests: LedgerDistance3, DriverFoundationRegression3, LedgerValueFormatter3 and existing LedgerEntriesViewModel9, total18, zero failures/errors. Invalid/nonfinite/signed/ambiguous/sub-metre/out-of-bounds input, zero, one-metre/max bounds, charge bound, edit roundtrip, malformed ratios, exact fractional-centavo half-up and localized efficiency are covered.

Actual command: JAVA_HOME=/data/dev/java/candidates/java/current ANDROID_HOME=/data/dev/android/sdk/Sdk ./gradlew --offline --no-daemon --max-workers=1 :domain:test --tests '*LedgerDistanceTest' :app:testDebugUnitTest --tests '*DriverFoundationRegressionTest' --tests '*LedgerValueFormatterTest' --tests '*LedgerEntriesViewModelTest' :app:assembleDebug :app:lintDebug

Result: BUILD SUCCESSFUL, 3m24s, `/tmp/drv01-acceptance.log`. A later label-only amendment re-runs :app:processDebugResources and :app:lintDebug; BUILD SUCCESSFUL in 1m58s, /tmp/drv01-labels.log; zero lint errors, existing warnings retained. Compilation/build is not device execution. No schema/SQL/OpenAPI/dependency change in this phase; no persistent/user DB touched. diff-check and changed/new-file secret review required at closure. Legacy classification and owner review follow in DRV-02; this phase does not bulk-convert history.

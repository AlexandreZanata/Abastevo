# Driver foundation fixtures

`planned-financial-v1.json` is the immutable planning baseline, not a runtime
acceptance claim. `driver-unit-v1.psv` executes current consumption rules in Go
and Kotlin, exact localized input/review in Kotlin, and display in the app.
The original `../premium/financial-v1.json` corpus is preserved.

`driver-manual-reference-v1.psv` freezes DRV-02-T03 worked examples for future
earnings/time behavior. Go and Kotlin **test-only reference consumers** compute
these examples independently with integers and exact rational arithmetic.
There is no production earnings, shift, time-union or modeled-result API/UI
added by this fixture. Later phases must run these same expectations against
their actual domain/application/storage/UI paths before claiming implementation.
No synthetic example certifies a real driver's financial result.

## Independent manual derivation

- V06: 50000 - 10000 + 2000 + 3000 - 0 = 45000 centavos earned.
- V07: inclusive net 40000 already includes tips; do not add 3000 again.
  Gross and fees remain unknown.
- V08: earned 45000 and bank received 30000 are different measures. No 75000
  earned total follows from that payout.
- V09: costs 13000 + 2000 + 5000 = 20000; recorded result 45000 - 20000 =
  25000 centavos. 200000 m = 200 km; 28800 s = 8 h. Ratios: 125 c/km,
  3125 c/h.
- V10: additional estimates 4000 + 6000 = 10000; modeled result 15000,
  75 c/km and 1875 c/h. Recorded 25000 is not overwritten.
- V11: [0,21600) and [7200,25200) overlap for 14400 s; union is 25200 s,
  not the summed 39600 s. Intervals are half-open seconds from one origin.
- V17/M04: missing and nonpositive denominators yield null plus a reason;
  never a made-up zero ratio.
- V20/M02: incomplete costs do not imply a complete operating result, even
  when some amounts are known.
- M01: 10000 - 12000 = -2000; divided by 100 km and 1 h gives -20 c/km
  and -2000 c/h. Preserve a loss rather than clamping it to zero.
- M03: 100 c / 3 km = 100/3 c/km; 100 c / (7000/3600 h) = 360/7 c/h.
- M05: contained/duplicated/adjacent intervals cover [0,30) once, total 30 s;
  raw lengths sum to 46 s. M06 leaves the [10,20) gap, union 20 s.
- M07: the explicit 5000-centavo refund subtracts once, yielding 40000.

These examples select meanings, not provider rates, financial advice or
production input ceilings. No float, date/value unit heuristic, bank call,
platform access or raw GPS trace is involved. `null` is literal unknown.

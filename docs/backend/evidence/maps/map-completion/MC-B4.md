# MC-B4 correction abuse tests

Status: LOCAL_DONE. Date: 2026-10-27. Base: `d9ed4579` plus this task.
Time: ~30 minutes.

## Coverage

Unit (`directory/application/correction_test.go`):

- location bounds, null island, wrong country, unknown JSON member and wrong
  types are refused;
- text fields reject HTML/control text; address validates postal code,
  municipality and UF;
- notes reject e-mail, URL, CPF and oversize text; unknown status/field fail.

Real PostGIS integration (`directory/adapters/correction_integration_test.go`,
disposable `postgis/postgis:16-3.4`, `-tags=integration`):

- same key + same body replays the row (no quota/open false positive);
- same key + changed body conflicts;
- a second open correction for the same account+station+field is refused;
- foreign owner read/cancel returns not-found (IDOR-safe);
- cancellation is audited only by state, not a fake decision;
- accepted location appends a `reviewed` revision, projects it, and writes the
  append-only decision with reason/decider;
- a second decision on the same row is closed;
- two concurrent decisions converge on one success and one closed;
- two concurrent submissions for one field converge on one open row;
- the 20/day quota refuses the 21st submission.

## Commands and results

```sh
ANPFUEL_TEST_DATABASE_URL=<local disposable postgis> go test -count=1 \
  ./internal/modules/directory/application/ -run TestNormalizeCorrection
# ok ... 0.006s
ANPFUEL_TEST_DATABASE_URL=<local disposable postgis> go test -count=1 \
  -tags=integration ./internal/modules/directory/adapters/ -run TestCorrectionIntegration
# ok ... 15.997s
go test -count=1 ./...   # exit 0, 0 FAIL
```

## Limits

The municipality-polygon check and distance-to-current-point evidence rule
arrive with the IBGE malha in MC-A3; until then the service enforces Brazil
bounds, null-island refusal and coarse proximity only. No contributor GPS is
stored. The note validator is deliberately conservative (URL/e-mail/CPF/long
digit runs rejected).

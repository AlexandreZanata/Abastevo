# ESP-07 — Accept the data foundation locally

Status: LOCAL_DONE / SYNTHETIC_SLICE_ACCEPTED
Validation: PASS
Integration: INTEGRATION_PENDING
Real source acceptance: BLOCKED_OVER_PRECISION
Date: 2026-10-24
Branch: `codex/enterprise-static-portal`
Parent: ESP-06 `bcc78d95`.

Rules: B-BR-ESP-01–11/15/16; BUC-ESP-01/02/03.
[Plan](../../../planning/ENTERPRISE_DATA_STATIC_PORTAL_PLAN.md#esp-07--accept-the-data-foundation-locally).

## Selected slice and exit

The accepted local slice is the synthetic/contract data foundation: Directory
identity/status/CNPJ export, Official current price revisions and corrections,
the bounded export manifest, deterministic analytics and the durable build/
reconcile orchestration. The real ANP weekly edition was exercised end to end
but is **not accepted** because the review gate found 880 over-precision prices
(4.29% of 20,489 rows). No rounding rule was invented and no source row was
silently dropped.

`ESP-DATA-LOCAL_READY` is therefore limited to the synthetic slice. Real-source
acceptance, real VPS publication and current national coverage remain OWED.

## Exact real edition inspected

- Source page: ANP weekly survey, updated 2026-10-23 16:29 (ESP-01).
- File: `revendas_lpc_2026_10_11-2026_10_17.xlsx`.
- Size: 1,875,300 bytes.
- SHA-256: `e148758e496dfa3166210358185d05c27ac4cd8c20e5071979d5559e3faaa6d6`.
- Sheet: `POSTOS REVENDEDORES`, header row 10.
- Parse: 20,489 data rows, 7,709 distinct CNPJs, 7 products, 6 collection dates,
  no parser error. Products: GLP 3,281; GNV 345; GASOLINA COMUM 4,526;
  GASOLINA ADITIVADA 3,438; ETANOL 3,850; DIESEL S500 1,908; DIESEL S10 3,141.
- File location: private `/tmp`, never committed.

## Real-source findings and corrections

1. **Lost CNPJ leading zeros.** The numeric ANP column produced 12–14 digit
   values. `official` import now left-pads numeric values to 14 digits before
   the strict kernel checksum; non-numeric/alphanumeric values pass through.
   Unit test: `TestPadSourceCNPJRestoresLostLeadingZeros`.
2. **Dot decimal.** The workbook prices use a dot (e.g. `5.59`) while the kernel
   accepts comma; the import now normalizes a single dot for parsing while
   retaining the original source text as `raw_price_text`. Unit test:
   `TestSourcePriceTextAcceptsANPDecimal`.
3. **Station product labels.** The real station file uses `GLP`, `DIESEL S500`
   and `DIESEL S10`; the kernel only knew `GLP P13`, `OLEO DIESEL S500` and
   `OLEO DIESEL S10`. The three station labels are now additive mappings with
   kernel tests. This removed all 8,330 `unknown-fuel-label` quarantines.
4. **Over-precision remains blocked.** After the corrections: 7,708 stations
   created, 19,609 staged prices, 880 `over-precision` rows, run `failed`,
   revision `needs-review`. The review gate correctly refused a 4.29%
   quarantine share (limit 1%). The real edition has no published revision and
   the build/analytics never ran against it.

The 880 rows require a recorded monetary-precision policy: either extend the
representation beyond milli-BRL or approve a documented rounding rule with the
raw text retained. Neither is invented here. Until then, real price publication
is blocked; the portal can be built and tested with synthetic fixtures.

## Synthetic/contract acceptance

All of the following ran on the disposable local PostGIS database:

```sh
go test -race -count=1 -tags=integration \
  ./internal/modules/enterpriseexport/... \
  ./internal/modules/official/adapters/jobs \
  ./internal/modules/official/adapters/read \
  ./internal/modules/directory/adapters/read \
  ./internal/modules/kernel ./internal/platform/config
```

All PASS. This covers the complete synthetic chain: import/replay/no-op,
same-week and older-week correction, current-pointer reads, invalidated CNPJ,
suspended/revoked status, export pagination/manifest/content hash, analytics
census/ties/missing/sample changes, durable build coalescing, busy lock,
cancellation, reconciliation and after-publication hook failure retention.

`go build ./...`, affected `go vet`, pinned `staticcheck`, `sqlc vet`, byte-stable
`sqlc generate`, `gofmt` and `git diff --check` all PASS.

The integration run that includes `./cmd/worker` still fails its FQ-native
worker cases because `ANPFUEL_TEST_QUALITY_PREPARER_PATH`,
`ANPFUEL_TEST_SANCTIONS_PREPARER_PATH` and `ANPFUEL_TEST_ENFORCEMENT_PREPARER_PATH`
are not built in this environment. Those are FQ/worker prerequisites, not ESP
regressions; the `cmd/worker` unit suite (including the new export dispatcher
isolation) passes.

## Manual real-source commands

```sh
ANPFUEL_MANUAL_XLSX=/tmp/esp07-real.xlsx \
  go test -tags=manual -count=1 -run TestManualRealWeekly -v \
  ./internal/modules/official/adapters/anp

ANPFUEL_TEST_DATABASE_URL=... ANPFUEL_MANUAL_XLSX=/tmp/esp07-real.xlsx \
  go test -tags='integration realsource' -count=1 \
  -run TestManualRealWeeklyEndToEnd -v \
  ./internal/modules/enterpriseexport/adapters/jobs
```

The parser smoke PASSes. The end-to-end smoke correctly SKIPs at the
`needs-review` gate and records the counts above.

## Limitations and recovery

- Real-source acceptance is blocked by the 880 over-precision rows; no real
  edition is published or exported.
- Station geography is not populated by the weekly price import; real
  municipality/state summaries also require the admitted registry/Directory
  import. The synthetic analytics proves the metric functions, not real
  municipal coverage.
- Source rights remain `RIGHTS_PENDING` for prices/registry (ESP-01).
- The last safe synthetic candidate/state is retained; a failed or cancelled
  build leaves it unchanged. No public release pointer exists yet.
- Next: ESP-08 — freeze portal routes, fields and editorial templates using
  labelled synthetic data while the real-source precision decision is pending.

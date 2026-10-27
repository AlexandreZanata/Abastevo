# DRV-00 — Frozen foundation decisions

Date: 2026-10-26. Scope: DRV-00/01/02 only, selected by the owner. Future DRV-03–35 stay planned. The owner confirmed professional Premium. This freeze changes neither subscription enforcement nor production/provider state.

## Source and ownership

Isolated maintained-dev clone: `/tmp/abastevo-driver-suite-planning-20261026`, planning parent `42250934`, runtime parent `f1e1d0db`. Remote main fetched as `eee1b33f`; remote dev fetched as `f1e1d0db`; main is an ancestor of this source. Other primary/MAP checkout edits were inspected read-only and are excluded. No other branch is moved or modified.

MEX-09/PRM06/09 own exact consumption and financial-v1; MEX-10 owns backend ledger/report reacceptance; MEX-11/12/13 own durable Android entries and monthly views; EE owns charge/energy methods; MAP owns navigation/location consumers. Reuse those boundaries. New driver work is not a duplicate ledger or payments module. Actual runtime has Room schema 27 and production SQL migrations through the current head; applied versions are never rewritten. Inspect any later main/schema changes again before integration.

## Selected decisions before behavior

1. **D-DRV-01, capability:** professional tools remain Premium. Existing `professional.write`/`professional.report` and live server authority are reused. No checkout/provider activation or invented grant in this batch. Held-data exit stays free.
2. **D-DRV-02, financial meanings:** earned value, received money, wallet transfer and payout are distinct. Net-only summaries preserve unknown gross/fee components. Cash and economic estimates remain separate. Future typed adjustment links, never negative refuels, own reversals.
3. **D-DRV-03, distance:** metre integers remain canonical. Decimal km input accepts one decimal separator (comma or dot), with no grouping/exponent/sign ambiguity; maximum precision is one metre. Entry/edit/reopen convert symmetrically. BRL cost displays two decimals, HALF_UP at final display only; ratios retain exact intermediates. Measured efficiency displays localized decimals, not an unexplained rational string.
4. **D-DRV-04, old history:** no value/size/date heuristic proves a legacy unit. Missing explicit provenance is UNVERIFIED, not automatically 1000 times too small. Preserve original values/revisions; review requires owner intent, preview, body/revision fencing and deterministic retry. Ambiguous raw records remain readable/exportable and cannot supply a certified metric.
5. **D-DRV-05, work time:** UTC plus explicit IANA report timezone, defaulted from device only at initial selection; monotonic active-time accounting and reboot gaps. Future app online intervals use union rather than addition. No shift/time runtime is built in this batch.
6. **D-DRV-06, privacy:** manual operation needs no GPS. Optional work recording later requires its own explicit session start/stop and review under Android/Play policy. Navigation is not recording consent. Raw traces are off by default and excluded from cloud/logs/training; no collection is added here.
7. **D-DRV-07, ownership:** existing account/origin/guest partitions remain; no guest adoption, restore or erasure bypass. All synthetic scenarios contain no passenger, account-secret, real receipt or precise contributor location.
8. **D-DRV-08, allocation proposal for later task:** largest fractional remainder, stable ID tie-break, exact source-centavo sum; reserves and actual service costs are separate views. Owner values/effective dates define recurring cost; no invented tax/commission/depreciation rate. Freeze detailed time-proration coverage at DRV-06 before implementing it.
9. **D-DRV-09, offline:** reuse existing bounded capability mechanism; the known vehicle admission lease is 24 hours, not automatic authority to create a different professional lease. Professional local entry/report acceptance must use its owning capability policy before future new paid behavior. No new quota or entitlement is invented.
10. **D-DRV-10, recovery:** corrections produce a new acknowledged revision/operation under existing contracts; no destructive down migration. Unknown or invalid values do not become zero. Keep financial-v1 unchanged and test Go/Kotlin parity for methods implemented in this batch.

## Existing ceilings and scoped workload

Use actual PremiumBounds: quantity 1..1,000,000,000 milli-units; actual paid total 0..1,000,000,000,000 centavos; general odometer 0..1,000,000,000,000 metres; note 280 runes; command batch 50. Charge has its separately frozen 1,000,000,000-metre ceiling: do not accidentally enlarge it when fixing UI conversion. Currency is BRL. These are input bounds, not performance or commercial promises.

Synthetic workloads: two full anchors for the unit defect; full/partial/full discounted chain; 50 queued commands for the existing batch boundary; 1,000 normalized rows for the existing bounded comparison projection (200 entries/year over five years is only a workload assumption); 1,001 rows must not return a truncated full report. No national ingest, external platform call or shared-VPS load is needed here. Performance acceptance budgets remain owned by DRV-31 and must be declared before its measurements.

## Frozen scenarios and acceptance

- Owned ICE: 10000→10500 km, 40 L, BRL 260 → 500 km, 12.5 km/L, BRL 0.52/km.
- Partial fills: initial 50 L/BRL 300 excluded; partial 20 L/BRL 120 plus full 20 L/BRL 124 across 400 km → 10 km/L and BRL 0.61/km.
- Rental/multi-app: owner-entered lease and expenses, no added depreciation; six and five online hours overlapping four produce seven unique hours when future shift rules are implemented.
- EV/PHEV: preserve exact energy/fees and shared-distance method; no guessed per-motor kilometres. Charge odometer respects its existing stricter bound.
- GNV: raw m3 purchases valid; unsupported measured-consumption method stays UNKNOWN, never fake km/L.
- Legacy/missing data: no invented origin; review/correction must retain original data, fence stale edits and survive retry. Incomplete coverage shows a reason, never zero-cost profitability.

DRV-01 tests execute the current actual form/domain/formatting chain. DRV-02 freezes/classifies legacy repair and extends only the foundation needed for its safe acceptance; new earnings, shifts, GPS and driver hub are outside this batch.

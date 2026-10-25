# ESP-04 — Implement deterministic analytics

Status: LOCAL_DONE
Validation: PASS
Integration: INTEGRATION_PENDING
Date: 2026-10-24
Branch: `codex/enterprise-static-portal`
Parent: ESP-03 `19ced4c4`.

Rules: B-BR-ESP-05–09/11; BUC-ESP-03.
[Plan](../../../planning/ENTERPRISE_DATA_STATIC_PORTAL_PLAN.md#esp-04--implement-deterministic-analytics).

## Before behavior

The snapshot had normalized stations and prices but no versioned, deterministic
metrics. Existing rankings/summaries were server-side reads without a frozen
input manifest, explicit denominator, tie handling, sample-change detection or
missing-period semantics. A page could not prove which rows produced a number.

## Delivered

New pure package `backend/internal/modules/enterpriseexport/analytics` with no
database, network, clock or filesystem dependency:

- **Coverage** per revision: every source row is included or counted in exactly
  one exclusion bucket (`unknown_station`, `unknown_fuel`, `unit_mismatch`,
  `unsupported_currency`, `invalid_date`, `period_mismatch`,
  `older_observation`); directory stations, unpriced stations, included
  observations, distinct stations, unlocated observations and fuel/unit counts.
- **Summaries** by revision/fuel/unit for national, state and municipality:
  station count denominator, source observation count, exact milli-BRL sum,
  min, max and spread, plus a station-sample SHA-256. The method label states
  that these are Abastevo-derived from ANP station facts, not an ANP official
  summary. The national metric is computed from station observations, never by
  averaging municipal means.
- **Rankings** for national and state geography: amount ascending, station UUID
  ascending tie-break, explicit `tie_count`/`tied_with_previous`, population
  count, operation status and cutoff behavior that keeps all stations tied at
  the truncation amount.
- **Station history**: one point per input revision, latest valid observation
  per revision, explicit `missing` gaps (never zero), and unit/period validation.
- **Summary change**: comparable only when fuel/unit/geography and the station
  sample hash and count match; otherwise `sample_changed` with no fabricated
  percentage.
- **Versioning/provenance**: `abastevo-metrics-v1`, `input_content_sha256` from
  the export manifest, fixed metric definitions for summary, cheapest,
  coverage, history and comparison.

No migration, dependency, route, source activation or public artifact was added.

## Golden tests and actual results

From `backend/`:

```sh
go test -race -count=1 ./internal/modules/enterpriseexport/...
go vet ./internal/modules/enterpriseexport/...
GOTOOLCHAIN=go1.27.1 staticcheck ./internal/modules/enterpriseexport/...
gofmt -l internal/modules/enterpriseexport
git diff --check
```

All PASS. The golden matrix covers:

- exact national/municipal/state sums, min/max/spread and denominators;
- a national mean distinct from an equal-weight average of municipal means;
- two stations tied at 5899, stable UUID order and tie flags;
- truncation at two rows preserving the cutoff tie (three entries, cutoff ties
  reported);
- a newer same-station observation replacing the older one, with the older row
  counted as `older_observation`;
- unit mismatch, period mismatch, unknown fuel/station, unsupported currency and
  invalid date exclusions;
- census reconciliation (`included + excluded = source rows`) and ranking
  population equal to coverage stations;
- a three-week station history where the third week is a gap, not a zero;
- comparable summary change (+1100 sum, +300 min, +200 max, −100 spread) and a
  same-count but different-station-set sample change;
- input-order independence (shuffled stations/revisions/prices produce identical
  JSON);
- rejection of missing content hash, duplicate station/revision and orphan
  revision rows.

No live source, database or published artifact was used; the inputs are explicit
golden fixtures.

## Limitations

- The projection is a pure value bound to `input_content_sha256`; writing a
  versioned analytics file and page shards belongs to ESP-09/12.
- No ANP official summary ingestion was added, so every summary is labeled
  `ABASTEVO-derived`. Official-summary metrics require a separate admitted
  representation.
- Quality/enforcement/fine metrics are deferred to ESP-11; no risk score or
  station ranking on enforcement exists.
- Ranking output is national/state; the same pure functions can rank a selected
  municipality at page build, but no municipality ranking artifact was frozen.
- No capacity or compressed-byte measurement was made in this pure phase.
- Next: ESP-05 — freeze one enterprise data offering boundary (specification only,
  no private management/billing implementation).

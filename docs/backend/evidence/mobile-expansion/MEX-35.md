# MEX-35 — Discovery performance and resource acceptance

Status: PARTIAL (bounded evidence; combined campaign OWED)
Validation: PASS (measured scope)
Integration: INTEGRATION_PENDING

Parent: `dd9d9c7e` (maintained dev, EE-28). Date: 2026-10-23.
Depends MEX-03/06/28–32/34. Risk: capacity/false optimization.

## Evidence reused (not a new run)

- MEX-06 fast picker: indexed keystroke p95 0.35 ms vs 5.13 ms before,
  host JVM; generation fencing and filtered composition unchanged.
- OPT-A/EE-26: community foreground 5-minute calls 21→6 fresh, 21→0
  offline, 21→5 outage-then-silence; first paint from snapshot needs
  0 network calls. These are prior measured artifacts, cited with their
  records, not re-measured here.
- EE-28: real prep 0.78 s cold / 0.66 s warm, 15.1 MiB RSS, byte sizes;
  real Rust→Go→PostGIS→13-page public traversal 1.28 s.
- MEX-24/28/29: indexed filter/ranking plans on realistic unrelated
  rows, cold/warm cache SQL/bytes and revision-bound cursors are in
  their records.

## OWED

- A single combined search/ranking/filter workload with pinned p50/p95,
  query counts, payload, memory and first-content on a physical device.
- Cold/warm full-catalog and dense-record/rapid-typing mobile campaign,
  interruption/outage matrix and safety/private-job contention.
- Android Macrobenchmark/device runs under the standing deferral.

## Decision

No new optimization or cache service is justified by the current
evidence. Existing indexed/bounded reads and event/manual refresh remain
the selected design. No latency, percentile or capacity claim is made
beyond the cited records; the combined campaign stays OWED.

Next: EE-29/MEX-36/37.

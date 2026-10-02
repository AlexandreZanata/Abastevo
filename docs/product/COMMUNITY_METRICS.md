# Community aggregate metrics and pilot operating model

State: **FROZEN definitions**, 2026-10-02. Owner phase P23-T03 (issue #93).
Definitions freeze here before any instrumentation: no metric below
may be collected, logged or displayed until its row is marked
COLLECTED with the stated source. Existing backend/frontier modules
are locally integrated; the new commercial UX is not. No future
phase is implemented by this document.

## Collection posture (verified, not promised)

- Android ships zero analytics SDKs (dependency scan 2026-10-02:
  no Firebase/Crashlytics/Sentry/telemetry client) and no
  advertising ID; location, notifications and photos are runtime
  opt-ins. Anything the app does not already persist locally is
  NOT_COLLECTED.
- Backend service metrics stay under the existing label policy
  (`docs/operator/monitoring.md`): bounded series, route templates
  only, contributor/station/observation IDs can never be label
  values (pinned by unit test), private scrape endpoint.
- Consent basis: account-gated writes already require a live
  session; notifications/location/photos require OS runtime grant;
  anonymous reads need nothing. No new consent surface is created
  by these definitions.

## Frozen aggregate definitions

| Metric | Purpose | Denominator / window | Min. aggregation | Retention / deletion | Status |
|---|---|---|---|---|---|
| Fresh station coverage | Pilot readiness per city | Directory stations in the pilot city; 7-day survey cadence | City-level only; no per-contributor breakdown | Recomputed; no retained series | DEFINED |
| Observation age (median) | Staleness honesty | Current projections in the pilot city; rolling 7 days | City-level, n≥5 stations or withheld | Recomputed; no retained series | DEFINED |
| Confirmation delay | Pipeline health | Import → consensus completion; per survey week | Job-system counters only (`jobs_oldest_queued_seconds` exists) | Existing job retention | COLLECTED (infra counters) |
| Valid / rejected / disputed share | Consensus quality | Community decisions per survey week | Week-level shares; no contributor rows | Follows observation retention | DEFINED |
| Contribution completion | Funnel health | Started vs submitted captures | Local-only; never leaves the device | App-data lifetime (user-cleared) | DEFINED, device-local |
| Report volume | Abuse load signal | Reports filed per city per week | City/week counts; reporter identity never stored anywhere | Case retention (12 months, audit rows first) | COLLECTED (moderation cases) |
| Moderation backlog / response | Staffing adequacy | Open cases; report → decision latency | Operator-only; never public | Case retention (12 months) | COLLECTED (ops queue listing) |
| Opt-in return participation | Return-flow value | Alert-enabled vehicles with ≥1 shown alert per week | Cohort counts, n≥5 or withheld | Device-local history (user-cleared) | DEFINED, device-local |

Rules: percentages render only with the exact denominator beside
them (no 0% certainty, no invented coverage); sparse cells show the
dated ANP reference instead; nothing here creates per-user tracking.

## Low-coverage city handling

Below usable community density the product stays honest: UNKNOWN /
no-coverage states, dated ANP reference, no simulated members,
prices or activity. Opening a city requires the staffing checklist
below recorded in its pilot plan — never implied by this document.

## Staffing / capacity / moderation checklist (per pilot city)

- Named moderation coverage with a response target and a triage
  cadence; backlog/response reviewed against the table above.
- Support route staffed (re-report path exists in-app since P22;
  in-app help surface since P23-T02).
- Privacy operations reachable (export/erase runbook,
  24-hour evidence expiry, 12-month audit purge).
- Stop/rollback criteria owned by the rehearsal (P23-T04) and the
  pilot plan — not by this document.

## Costs from measured utilization only

Measured 2026-10-02 (local, warm cache): backend full unit suite
passes with zero failures; `-race` PostGIS integration per package
in seconds-to-tens-of-seconds; app unit suites in ~1–2 min;
`quick-verify` 12–13 s; feedback hot paths ~0.5–0.7 µs/op
(application layer). Pilot hosting, storage and staffing costs are
unknown until the pilot plan attaches real numbers — no budget is
invented here.

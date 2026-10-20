# Stage C — Economy and retention

State: PLANNED / NOT_IMPLEMENTED_BY_THIS_PLAN, 2026-10-14.
Owner: [master](APP_CONSOLIDATION_PLAN.md). Tasks OPT-C00–C08.
Rules: B-BR-OPT-01/02/11–16/18. Use cases: BUC-OPT-05/06/07/10.

## Outcome and prerequisites

Useful source-aware alerts, honest observed history and estimated net savings
after a fueling detour. Entry: A/B source acceptance and consistent current
prices/conditions/Directory identity. Reuse basic weekly alerts, notification/
history repositories, vehicle types, TankFillCostRule and existing price history.

Reconcile [PRM07 and later alert/provider amendments](PREMIUM_BACKEND_APP_PLAN.md);
do not duplicate implemented queues/APIs or assume actual provider acceptance.
Existing basic free functionality and explicit subscription boundaries remain;
new work cannot buy price truth or paywall a free baseline.

History/local alerts need no routing provider, Premium purchase or D receipts/
portal. No billing expansion, prediction, Trust Graph, missions, attestation or
new GPS admission is hidden in C.

## History and alert contract

Preserve source/station/product/unit/full condition and observed/published/valid
dates plus coverage. ANP survey and community changes remain separate series.
"Lowest in 30 days" means among available comparable records within that window,
not unseen continuous monitoring. Sparse/selective history cannot prove weekday
patterns or forecasts. Reuse bounded facts/projections rather than storing every
poll or personal attribution indefinitely for analytics.

Start selected alerts with chosen favorite/threshold/observed drop. Use accepted
current projections, not likes/raw submissions. Recheck source/condition/price
validity, rule revision, target, account/device/revocation and applicable authority
before dispatch. Promotions need supported explicit conditions; stale/disputed/
unknown data cannot create a favorable offer.

Opt-in/OS permission, quiet policy, cooldown, bounded dedupe and retry apply.
Correction/replay cannot create phantom repeated drops. Local delivery can
precede remote provider proof. Evaluated/queued/provider-accepted/device-received
are distinct. Rule delete/logout/revoke must stop future private sends during
worker/reconnect races. Privacy jobs remain independent of payment.

## Net economy contract

Compare compatible product/condition and equal requested volume against an
explicit available reference station/price. Use exact money/decimal types and
freeze units/rounding/reference travel-fuel price in OPT-C01.

```text
purchase saving = volume × (reference unit price − candidate unit price)
extra travel fuel = extra route distance ÷ applicable distance-per-fuel-unit consumption
net estimated saving = purchase saving − extra travel fuel cost − extra tolls
```

Preserve existing TankFillCostRule decimal HALF_UP display semantics unless a
named new calculation has separately documented rules; no silent intermediate
rounding. Litres/m³/kg and differing-fuel consumption are not interchangeable.

Extra distance is the detour against a baseline route, with on-route/return
effects where applicable, not straight-line distance. Show extra time separately;
monetize it only at an explicit user-supplied value. No guaranteed exact savings.

Unknown consumption/point/route, stale price or incompatible condition/unit
yields an explicit insufficiency reason. Never fabricate reviewed coordinates,
current prices or route distances. Routing/nearby remains optional for contribution.

Pilot a bounded supported geography. Select permitted routing access/adapter
and reviewed dependency/license/security/cost before integration; shortlist via
existing indexed reads, route bounded candidates and measure requests/cache/
fanout. Manual origin or consented transient location are options; no precise
contributor GPS in logs, persistent travel traces or cloud trip history.

## Sequential tasks

### OPT-C00 — Reconcile existing user-economy capabilities

- **Depends:** OPT-B08.
- **Inputs/areas:** basic alerts/history/notifications, PRM07/amendments, vehicle/
  tank-cost rules and read ports.
- **Deliver:** reuse/gap and free/paid/provider map; actual data/point/consumption
  coverage for selected increment.
- **Tests/acceptance:** map basic alert permissions/dedupe, history gaps and exact
  tank-cost cases; source does not prove live provider readiness.
- **Recovery:** preserve existing free behavior; audit makes no product mutation.

### OPT-C01 — Freeze history/alert/net-cost contracts

- **Depends:** OPT-C00.
- **Inputs/areas:** product rules, shared Go/Kotlin vectors, actual read/alert APIs.
- **Deliver:** statistic coverage language, alert validity/dedupe, money/
  consumption/distance units, baseline path, rounding and unknown outcomes.
- **Tests/acceptance:** independent hand oracles: negative/no saving, return/
  on-route detour, low volume, toll/time opt-in, wrong units and partial history.
  No D or new payment dependency.
- **Recovery:** revise unpublished rules, preserve legacy weekly/tank semantics.

### OPT-C02 — Bounded source-aware history

- **Depends:** OPT-C01. Critical SQL where changed.
- **Inputs/areas:** official/community facts, read ports and A cache.
- **Deliver:** stable target/period, coverage/gaps, bounded pagination and public
  offline-history presentation.
- **Tests/acceptance:** correction/identifying-content erasure, same-day events,
  missing periods, page ties/reorder, origin switch and real PostGIS query plans.
- **Recovery:** disable added statistics while preserving existing dated facts.

### OPT-C03 — Useful alert evaluation and durable dedupe

- **Depends:** OPT-C02.
- **Inputs/areas:** existing rule evaluators, projections, Jobs/inbox/history.
- **Deliver:** selected predicates, freshness, revision-aware dedupe and atomic
  candidate/job intent when server-owned.
- **Tests/acceptance:** fresh/stale/disputed/conditional/unknown, correction/replay,
  parallel evaluation, disable/foreign owner, bounded fanout, crash/rollback;
  real SQL for affected transactions.
- **Recovery:** stop added evaluator; preserve basic local alerts.

### OPT-C04 — Delivery and account-safe controls

- **Depends:** OPT-C03.
- **Inputs/areas:** notification channel/repositories, settings/inbox and accepted
  provider only if selected.
- **Deliver:** local-first selected alerts, opt-out/quiet/cooldown, safe deep link
  and truthful delivery state.
- **Tests/acceptance:** permission denial, queued expiry, account/origin/logout/
  revoke/delete races, duplicate reconnect, provider timeout/429/lost response,
  no PII/precise location. Actual remote delivery needs provider/device proof;
  local-only acceptance is a valid explicitly bounded increment.
- **Recovery:** disable remote dispatch independently; keep authorized local/inbox
  and privacy recovery irrespective of payment.

### OPT-C05 — Pure net-fueling-cost engine

- **Depends:** OPT-C01 and valid current B prices.
- **Inputs/areas:** existing vehicle/money types and bounded pure calculation.
- **Deliver:** candidate comparison with source/date, volume/consumption,
  incremental fuel/tolls, optional valued time and insufficiency reasons.
- **Tests/acceptance:** independent exact oracle, overflow/precision/rounding,
  wrong units, zero consumption, negative savings, return/on-route, absent
  price/point and different fuels.
- **Recovery:** disable detour ranking; preserve legacy tank-cost estimate.

### OPT-C06 — Bounded route evidence

- **Depends:** OPT-C05 and selected provider/terms/budget decision.
- **Inputs/areas:** Directory point provenance, indexed shortlist, transient origin
  and explicit routing port.
- **Deliver:** permitted extra-distance/time evidence with scoped freshness,
  candidate caps/cache and measured request budget.
- **Tests/acceptance:** unknown/stale/wrong point, no route, false straight-line
  fallback refused, provider timeout/429/redirect, concurrency/dedupe/memory/
  log redaction. Real provider evidence is required for routing claims.
- **Recovery:** explicit insufficiency/price-only comparison. Routing outage
  does not block history or local alerts.

### OPT-C07 — Transparent Android economy journeys

- **Depends:** OPT-C02/04/05/06 selected contracts; route UI remains gated if real
  route evidence is unavailable.
- **Inputs/areas:** vehicle/Home/station/history navigation and A cache.
- **Deliver:** explained net economy, distance/time/tolls, history and dates.
- **Tests/acceptance:** missing vehicle/point, stale price, changed origin/reference,
  late/offline responses, incompatible condition, large font/TalkBack and free
  existing journeys; affected JVM/lint/APK.
- **Recovery:** independently disable route comparison, preserve local records,
  history/basic alerts and estimates.

### OPT-C08 — Value, recovery and batch handoff

- **Depends:** selected preceding C acceptances; exclusions stated.
- **Inputs/areas:** pinned app/backend/data/provider/config manifest.
- **Deliver:** measured request/cost/latency, usefulness/duplicate outcomes,
  coverage limits and final affected A–C acceptance union.
- **Tests/acceptance:** real selected alert/comparison journeys, cancellation,
  expiry/privacy restore and due authorized native/accessibility matrix.
  Excluded flows are not complete; D is not required.
- **Recovery:** feature-specific disable and owned state recovery; no reset or
  fabricated route/price.

## Exit

Accepted C scope and truthful provider/native/retention limits; included known
local failures block acceptance. Retention/engagement gains remain hypotheses
to measure. D remains DEFERRED_FUTURE when C closes.

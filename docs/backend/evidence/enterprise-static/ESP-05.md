# ESP-05 — Freeze one enterprise data offering boundary

Status: LOCAL_DONE
Validation: PASS
Integration: INTEGRATION_PENDING
Date: 2026-10-24
Branch: `codex/enterprise-static-portal`
Parent: ESP-04 `3daa13c1`.

Rules: B-BR-ESP-03/16; no BUC runtime.
[Plan](../../../planning/ENTERPRISE_DATA_STATIC_PORTAL_PLAN.md#esp-05--freeze-one-enterprise-data-offering-boundary).
[Frozen specification](../../../product/ENTERPRISE_DATA_OFFERING.md).

## Before behavior

The plan listed regional price intelligence, monitored station reports and
licensed exports as candidates, but did not select one buyer, one decision, one
artifact or the evidence needed to sell it. Demand, costs and redistribution
rights were unresolved.

## Delivered

[Enterprise data offering boundary](../../../product/ENTERPRISE_DATA_OFFERING.md)
freezes:

- one buyer (regional distributor/association pricing analyst) and one recurring
  decision (regional sample comparison and coverage sufficiency);
- one artifact (periodic Regional Fuel Price Brief) with required period,
  geography, fuel/unit, sample count, exact milli-BRL min/max/spread, coverage,
  exclusions, source revision and Abastevo-derived label;
- coverage rules: one state initially, at least five priced stations per
  municipality, otherwise `INSUFFICIENT_SAMPLE`;
- cost inputs to measure later (build CPU/RAM/time, candidate/release bytes,
  transfer/egress, refresh/support), explicitly `UNMEASURED`;
- rights status: price/registry profiles remain `RIGHTS_PENDING`, so a
  commercial derived offer has no recorded permission yet;
- demand status: `DEMAND_UNVALIDATED`, with no invented customer, price,
  revenue or willingness-to-pay;
- explicit exclusions: tenant/team management, customer lists, fleet telemetry,
  alerts, billing, paid-access protection, station risk scores, legal advice,
  guarantees and Stage D participation.

No implementation, contract, customer, price or deployment follows.

## Checks executed

- Local Markdown path/link resolution for the specification and this evidence.
- `git diff --check` and scoped secret/PII inspection of the changed documents.
- No backend/landing/Android runtime test: this phase changes documentation only
  and enables no runtime or commercial behavior.

## Limitations

- `DEMAND_UNVALIDATED`: no buyer interview, letter of intent or usage evidence.
- `RIGHTS_PENDING`: the core price/registry data cannot be commercially
  redistributed from the current record.
- Costs are unmeasured; no price or margin is claimed.
- Private delivery/billing/organization management remains outside ESP and
  behind a separate contract/implementation decision.
- Next: ESP-06 — durable bounded data-to-export orchestration, reusing existing
  jobs and module ports without activating a schedule.

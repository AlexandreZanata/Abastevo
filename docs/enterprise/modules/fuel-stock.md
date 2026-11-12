# Internal fuel stock

Status: **PLANNED / RECONSTRUCTED**, 2026-11-11.
Recovered from the surviving enterprise requirements, shared rules and backlog.
The accepted F0 decisions and existing release gates govern any implementation.

## 1. Purpose

Track an organization's internal fuel inventory and attribute the recorded consumption cost to its assets without duplicating the purchase.

## 2. Scope

F6 adds tanks, receipts, dispensing, transfers, measured adjustments and reconciliation. It does not initiate provider payments. Metering and valuation depend on exact approved evidence.

## 3. Entities and ownership

The organization owns internal tank, product stock, movement, measurement and reconciliation. A dispense links one source tank and one same-organization asset fueling. Movements preserve their original identity and supporting references.

## 4. Business rules

- **BR-STOCK-001 — Exact compatible stock.** Quantity and value are exact and product units remain distinct.
- **BR-STOCK-002 — Moving average.** Valuation follows EDR-029, with the input basis retained. Missing quantity or valuation is not zero.
- **BR-STOCK-003 — One consumption cost.** Inventory purchase does not duplicate the cost recognized at the approved dispense (EDR-013).
- **BR-STOCK-004 — Audited reconciliation.** Measurements and adjustments record their reason and authority and never rewrite prior movements.

## 5. Use cases

**UC-STOCK-01 — Receive stock.** An authorized member records the tank, product, accepted quantity, source value and evidence. The transaction changes scoped inventory once. A retry reuses the result.

**UC-STOCK-02 — Dispense.** An authorized member records the asset and quantity. Intake checks the current source, records the movement and links the internal fueling. A concurrent shortage follows review or the approved policy and remains visible.

**UC-STOCK-03 — Reconcile.** An authorized member compares measured and recorded stock, records a reason and an approved adjustment. Difference alone does not establish loss.

## 6. States and transitions

Movements are append-only facts with reviewed corrections. Reconciliation follows open, explained or adjusted under the [state catalog](../08-state-model.md). An explanation records evidence and makes no change. An adjustment is a separate authorized movement. Retry cannot reopen a terminal decision or dispense twice.

## 7. Roles and permissions

Use [stock permissions](../07-permissions.md), asset scope and current organization authority. Receiving, dispensing, adjusting and viewing are distinct powers. A driver cannot adjust stock by recording a fueling. Private evidence access is checked independently.

## 8. Offline, concurrency, idempotency and corrections

Field dispensing follows the approved offline matrix. Cached availability does not prove current stock. A late or conflicting fact stays for review. Transactions serialize the affected product stock and preserve movement idempotency. In-place edits to approved movements are prohibited.

## 9. Events, notifications and integrations

The scoped stock module calls fueling and expense ports without crossing table ownership. Accepted movements notify projections through the existing job queue. Reports carry movement and method provenance. External purchasing and payment services are separate.

## 10. Data, formulas and examples

With recorded stock of 100 000 mL worth 60 000 centavos and a receipt of 50 000 mL worth 33 000, the pool is 150 000 mL worth 93 000. A 30 000 mL issue has a proportional cost of 18 600 centavos, leaving 120 000 mL and 74 400 centavos. Residual allocation follows exact rounding policy and source value is conserved. Different products are never combined in this calculation.

## 11. Screens

| ID | Surface | Objective | Actor | Data and actions | Validation |
|---|---|---|---|---|---|
| SCR-STOCK-01 | PANEL | Inspect tank movements | Scoped stock manager | Recorded quantity, value, history and reconciliation | Source basis, exact units and five shared states |
| SCR-STOCK-02 | APP | Record an authorized dispense | Scoped field member | Tank, asset and quantity | Durable save, stale availability labelled and accessible errors |

## 12. Acceptance, dependencies, risks, phase and microtasks

[EP12](../backlog/ep12-fuel-stock.md) owns the tasks. VAL-32 proves one-cost recognition. Test concurrent issues, exact value conservation, foreign tanks/assets, replay, rollback and measurement adjustment on real PostGIS before dependent indicators.

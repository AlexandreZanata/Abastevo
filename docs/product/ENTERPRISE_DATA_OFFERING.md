# Enterprise data offering boundary

Status: FROZEN SPECIFICATION / DEMAND_UNVALIDATED / RIGHTS_PENDING /
NO_PRIVATE_IMPLEMENTATION. Date: 2026-10-24. Task: ESP-05.
Owner: [enterprise data/static portal plan](../planning/ENTERPRISE_DATA_STATIC_PORTAL_PLAN.md#esp-05--freeze-one-enterprise-data-offering-boundary).
Rules: [B-BR-ESP-03/16](ENTERPRISE_DATA_STATIC_PORTAL_CONTRACT.md).

This document selects **one** buyer and **one** decision for a future enterprise
data offering. It does not implement accounts, organizations, billing,
entitlements, private delivery or paid access, and it does not change the free
public portal. No customer, revenue, price, willingness-to-pay or cost figure is
invented here.

## Buyer and decision

- **Buyer:** a pricing/market analyst at a regional fuel distributor or a
  regional retailer association. The buyer works with a state and its
  municipalities, not with individual consumer accounts.
- **Recurring decision:** “How do the station prices in the current ANP survey
  sample compare within our region, and where is the sample too thin to support
  a conclusion?”
- **Not the decision:** identifying a competitor's current live price, ranking
  individuals for enforcement, predicting a station's next price, certifying
  fuel quality or guaranteeing savings.

One named buyer and one decision are the whole scope. A second buyer, fleet
telemetry, personalized alerts or a private dashboard requires a new recorded
decision.

## Offered artifact

A periodic **Regional Fuel Price Brief**, derived only from the admitted public
snapshot and the ESP-04 metrics:

- period (survey start/end) and Abastevo publication time;
- state and municipality, fuel and unit;
- station sample count and source observation count;
- min, max and spread in exact milli-BRL, with displayed BRL formatting;
- national/state context from the same frozen revision;
- coverage, exclusions, missing municipalities and sample-change notes;
- source revision URL/checksum, methodology version and attribution.

Every number is labelled `Abastevo-derived from ANP station facts`. It is never
presented as an ANP official summary. The brief never says “cheapest station
currently in Brazil”; it says “lowest prices found in the identified ANP survey
sample and period”.

### Synthetic example (not real data, not a price promise)

```text
Region: São Paulo (SP), municipality 3550308
Fuel: GASOLINE_REGULAR, unit L
ANP survey: 2026-10-04 to 2026-10-10
Priced stations in sample: 4
Minimum: 5.799 BRL   Maximum: 6.099 BRL   Spread: 0.300 BRL
Abastevo-derived summary; not an ANP official statistic.
Coverage note: 4 of 4 exported stations in this geography had a price.
```

The values are illustrative fixtures. They are not an offer, a forecast or a
claim about the current market.

## Geography and coverage rules

- Start with one state and its municipalities; expand only after measured
  demand and a recorded scope decision.
- A municipality brief requires at least **five priced stations** in the
  selected revision. Below five, the brief reports `INSUFFICIENT_SAMPLE` and
  no ranking claim.
- Municipalities with missing identity or no observation stay counted as
  coverage gaps, not silently dropped.
- The survey is a sample. Coverage and period are mandatory on every artifact.
- A correction creates a new revision; the brief identifies the revision used
  and does not splice revisions.

## Cost and delivery boundaries (unmeasured)

The following are cost inputs, not estimates or commitments:

- export build CPU, memory, elapsed time and candidate bytes;
- retained release bytes and retention multiplier;
- public/private transfer and CDN egress;
- source refresh frequency and operator support;
- licensed-export support and correction handling.

No cost measurement has been made. The offering cannot be priced or promised
until ESP-16 and a separate commercial decision record actual values.

Possible delivery is a standardized licensed export or a report generated from
the same public snapshot. Private delivery, API access, tenant isolation,
personalization and billing are outside ESP and require a new contract and
implementation. Public static files cannot protect a paid dataset.

## Rights and redistribution

- The price and registry profiles are `RIGHTS_PENDING` in the
  [source admission register](ENTERPRISE_SOURCE_ADMISSION.md). A commercial
  redistribution of their derived rows therefore has no recorded permission.
- Enforcement and fine profiles have a declared CC BY path, but their runtime
  and safe station association remain gated; they are not part of this offer.
- A commercial brief requires a recorded compatible license or written
  permission, attribution to ANP, and clear separation between official facts
  and Abastevo-derived calculations.
- No ANP data may be sublicensed beyond the source terms. The free public
  portal remains free; the paid offer, if ever enabled, must not change public
  trust, ordering or access to official facts.

## Demand evidence

`DEMAND_UNVALIDATED`. No named prospective buyer, interview, letter of intent,
usage measurement or willingness-to-pay evidence exists. The product example is
a specification, not demand proof. Validation requires a named prospective
buyer and a documented recurring workflow before any private implementation.

## Explicit exclusions

Organization/team management, private customer lists, fleet telemetry,
personalized server alerts, billing, paid-access protection, account
entitlements, station risk scores, enforcement predictions, legal advice,
guaranteed savings, consumer reports/photos/GPS and iOS work are excluded. The
Stage D future-business gate remains deferred; this specification does not lift
it.

## Acceptance and rollback

Exit: one buyer, one decision, one artifact example, coverage rules, cost
measurement inputs, rights status, demand status and exclusions are frozen. No
implementation, contract, price or customer is claimed.

Rollback: withdraw this specification. The free public analytics and portal
remain unaffected.

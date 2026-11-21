# Organization subscriptions and product separation

## 1. Purpose

Evaluate reviewed organization plans and limits independently of individual accounts, membership and public participation.

## 2. Scope

F1 uses administratively controlled organization capabilities behind default-off flags. F8 adds approved catalog, billing, lifecycle and usage. Prices, trial periods, providers and fiscal documents require recorded decisions. The Android distribution remains consumption-only for this scope under the proposal and store review.

## 3. Entities and ownership

The organization owns subscription, capability evaluation and usage records. The catalog versions plan definitions and limits. The platform catalog stores immutable snapshots in
`enterprise_plan_catalog`, keyed by product, plan and revision. Its meter
ceilings are nullable integers for active vehicles, active stations, storage
bytes and API calls. A zero is an explicit ceiling. An absent ceiling or an
unregistered capability cannot grant access. The migration includes no plans.
Verified billing event identity and reconciliation preserve provenance. Personal Premium and the existing account ENTERPRISE tier remain separate sources with their current rights.

## 4. Business rules

- **BR-SUBS-001 — Plan grants no role.** Capabilities and limits do not create or widen membership or assignments (BR-AUTH-009).
- **BR-SUBS-002 — Confirmed source.** A client toggle or local payment record cannot activate a subscription. Only its approved server authority can do so.
- **BR-SUBS-003 — Retained history.** Plan change never silently deletes records. Archived resources preserve their history and permitted exports.
- **BR-PUB-001 — Private/public separation.** Publication requires a separate explicit permitted action. Personal and company queues and histories stay distinct.

## 5. Use cases

**UC-SUBS-01 — Evaluate access.** A request resolves current membership, scoped role, state and approved capability limits. Missing or disabled authority denies the relevant new action. Reads and exports use the recorded state policy.

**UC-SUBS-02 — Downgrade.** A permitted manager selects an admitted change. The recorded effective period applies. If 14 assets exceed a limit of 10, the authorized member chooses the 10 active assets and the remaining four retain archived history. Eligible earlier offline facts enter review.

**UC-SUBS-03 — Reconcile billing.** An admitted verified event is bound to its exact organization and offer. Repeated event IDs converge. Unknown or inconsistent records remain unreconciled and cannot grant roles.

## 6. States and transitions

The catalog uses trialing, active, past due, suspended, cancelling and cancelled from [the state catalog](../state-model.md). Validity and grace depend on recorded policy rather than invented durations. The server persists event, source and transition history. The scoped
`org_billing_subscription_events` ledger keeps the exact plan revision, state
before and after, term, event time and redacted evidence reference. Each new
revision must follow the immediately preceding state. Verified settlement can
recover a past-due term. Operator restoration returns to the state held before
suspension. Pending cancellation preserves its prior state until verified
cancellation or the recorded term boundary. Reactivation preserves prior periods. Provider events do not change station authority or public records.

## 7. Roles and permissions

The plan capability source is composed with the existing authorization gate
when `ANPFUEL_ENTERPRISE_BILLING_ENABLED=true` on an enterprise-enabled
deployment. Without subscription history, operator pilot flags remain the
source. A subscription with a current entitlement can grant a capability
only after role and proof checks. An explicit disabled operator flag can
still block ordinary writes. The transaction that applies a new write
rechecks the capability under the same organization lock used for subscription
changes and operator flags. An authorized retained export can be requested
after the subscription stops allowing new work. Existing export proof,
permissions, current organization state and operator denial still apply.
The switch defaults off.

Plan read and management require the current organization's named permissions. Fresh authentication applies where required. Billing authority does not authorize staff actions or private read scopes. Separate sensitive permissions still govern exports, documents and location.

## 8. Offline, concurrency, idempotency and corrections

Changes and purchases require the server. Existing field queues preserve creation context and current intake uses server history. Repeated provider events and manual adjustments are idempotent. Usage counters retain period and source and cannot be bypassed by merely rotating a credential.

## 9. Events, notifications and integrations

The organization settlement consumer can receive Stripe invoice events through
the existing authenticated payment inbox when both enterprise and organization
billing are enabled. The finalized subscription metadata identifies the
company subscription. The consumer retrieves the invoice and captured card
charge, compares the reviewed customer, contract, amount, price and term,
and atomically appends the transition and its audit receipt. Reconciliation
records conflicts in the inbox without applying an organization transition.

Manual pilot capability activation remains independent from selling plans.
Notifications carry safe links. Fiscal invoice issuance and accounting
connections are separate capabilities.

## 10. Data, formulas and examples

Commercial numbers in [the product vision](../product-vision.md) remain hypotheses. Catalog and effective periods are data. Limits name measurable units and periods. Usage includes active vehicles or stations, admitted storage and API calls, with no invented production capacity. The vehicle meter counts scoped fleet records of kind `VEHICLE` in state `ACTIVE`. The existing fleet root lock serializes changes that increase this count. A denied activation preserves the draft and creates no activation acknowledgement. Money remains exact and source-bound.

## 11. Screens

| ID | Surface | Objective | Actor | Data and actions | Validation |
|---|---|---|---|---|---|
| SCR-SUBS-01 | PANEL | Inspect own organization plan | Authorized manager | State, capability, limits and effective changes | Personal tier separate, no false payment success |
| SCR-SUBS-02 | PANEL | Resolve authorized overage | Permitted manager | Affected scoped resources and explicit selection | No deletion, exact limits and five shared states |
| SCR-PUB-01 | APP | Understand active product context | Account holder | Personal or named organization mode | Protected partition, explicit switch and accessible labels |

## 12. Acceptance, dependencies, risks, phase and microtasks

[EP16](../backlog/ep16-subscriptions.md) defines tasks. GOV-009 and the offer policy precede billing. VAL-18 and VAL-35 test verified-event replay and state effects. Keep public registration, ranking and social participation independent. The plan cannot certify provider or store readiness.

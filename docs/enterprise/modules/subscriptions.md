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

**UC-SUBS-02 — Downgrade.** A permitted manager selects an admitted change.
The recorded effective period applies. When the lower paid term leaves
more active vehicles than its limit, the company plan panel shows the
current count and allows a manager with fresh proof to select the exact
vehicles that stay active. The scoped command binds the current subscription
revision and fleet authorization. The remaining vehicles become inactive
and receive a plan archive reference to the audited fleet transition.
Fleet reads and exports retain their history. Open trips or work orders
block the whole selection. Reactivation checks the current fleet limit.

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
The company plan and billing panel uses `subs.plan.read` to show exact
catalog revisions, current term dates and scoped confirmed settlements. It
keeps the previous company's results out of the screen after a scope change.
It reads the plan under the same shared entitlement lock as the Data panel,
with the server clock sampled after the lock wait. The read remains available
when billing writes are disabled.

## 8. Offline, concurrency, idempotency and corrections

Changes and purchases require the server. Existing field queues preserve creation context and current intake uses server history. Repeated provider events and manual adjustments are idempotent. Usage counters retain period and source and cannot be bypassed by merely rotating a credential.

A matching keep-active retry returns its original selection. The signed
fueling intake resolves a currently plan-archived vehicle under the same
organization and proves the author's retained authority and the subscription
in effect when the fueling was declared. Eligible earlier fuelings produce
one `PLAN_CHANGED` review item and a replayable receipt. A new declaration
for the archived vehicle is refused while it stays out of active use.

The storage meter counts reserved and retained original bytes from the
company document registry. The plan panel displays that measured total
alongside the reviewed byte ceiling. Upload declarations from the company
panel and signed device share the same entitlement lock and audited
reservation. Replays keep the original allocation. Failed or deleted
documents release their logical allocation under the existing lifecycle.

A reviewed trial grant records its organization, plan revision, acceptance
window, duration and approval reference. The restricted operator path starts
it with `ops enterprise trial start --org <uuid> --grant-id <uuid>
--operation-id <uuid>`. The server clock establishes the original term.
Retry with the same operation ID to read the original result. Trial grants are
protected by organization scope. No grant, price or duration is seeded.

## 9. Events, notifications and integrations

A commercial review can authorize binding an existing Stripe invoice or Asaas
charge to the exact reviewed company offer. The approval fixes the company,
operator, subscription, customer, contract, resource and term. Asaas reviews
include the scoped external reference. The operator runs:

```sh
ops enterprise billing bind --org <uuid> --approval-id <uuid> --operation-id <uuid> [--operator <uuid>]
```

The command uses a dedicated ordinary PostgreSQL login and the existing
enterprise billing flags. It consumes an in-window approval and retains the
binding and audit together. A retry of the same operation returns the original
acknowledgement even after the approval expires. New bindings require a company
that has not closed. Binding creates no payment, fiscal document or entitlement
and the acknowledgement contains only internal identifiers and replay status.
The review authority provisions immutable offers and charge approvals separately
under GOV-009. The operator login can only read these reviews. The API and worker
have read access to the binding for reconciliation and cannot enroll charges.
After migration `000289`, the migration owner applies
[`infra/db/billing-binding-role.sql`](../../../infra/db/billing-binding-role.sql)
to a dedicated ordinary login. The restricted operator process uses that
login in `ANPFUEL_DATABASE_URL`; the API and worker keep their own login.

The organization settlement consumer can receive Stripe invoice events through
the existing authenticated payment inbox when both enterprise and organization
billing are enabled. The finalized subscription metadata identifies the
company subscription. The consumer retrieves the invoice and captured card
charge, compares the reviewed customer, contract, amount, price and term,
and atomically appends the transition and its audit receipt. Reconciliation
records conflicts in the inbox without applying an organization transition.

Asaas reconciliation retrieves the existing charge and uses its
`abv_org:<organization UUID>:<checkout UUID>` reference to locate the
reviewed binding. Manual Pix uses the one-off charge and automatic Pix
requires the linked authorization. Received, unadjusted charges must match
the exact approved BRL amount. The receipt preserves the provider's payment
day separately from the inbox observation instant. Advance payments on
either provider wait in the inbox until the approved term begins. A separate
charge for an already settled term enters reconciliation.

Manual pilot capability activation remains independent from selling plans.
Notifications carry safe links. The plan panel reads the receipt for its
exact term and displays its gross amount in BRL. It reports
`NOT_CONSULTED` for fiscal status while authorized fiscal consultation is
pending. Fiscal invoice issuance and accounting connections are separate
capabilities.

### Read-only provider sandbox replay

Use an existing synthetic charge from the reviewed test merchant. The Stripe
invoice's finalized subscription metadata names its organization and
subscription. An Asaas charge's external reference names the organization and
checkout. Use organization A from
[the shared synthetic fixture](../../../contracts/testdata/enterprise/fixture-two-orgs-v1.json)
and its `record_id` as the subscription ID. The case file contains exactly the
reviewed identifiers rather than deriving them from the retrieved payment.

Save one JSON object in a private file with mode `0600` and set
`ANPFUEL_SANDBOX_COMPANY_BILLING_CASE` to its path. The fields are:

- `channel` is `stripe_card`, `asaas_pix_manual` or `asaas_pix_automatic`.
- `organization_id`, `subscription_id`, `event_id`, `resource_id`, `customer_id`,
  `contract_id` and `offer_reference` are strings. For Stripe, use the invoice,
  customer, subscription and price references. For manual Pix, the resource
  and contract references are the same payment ID. For automatic Pix, the
  contract reference is the reviewed authorization ID. The Asaas offer
  reference is the reviewed internal reference.
- `asaas_external_reference` is the exact reviewed reference for Pix and an
  empty string for Stripe.
- `amount_centavos` is an integer. `term_start` and `term_end` are RFC 3339
  timestamps delimiting the current billable term. A Stripe case uses the
  exact interval on the invoice line.

Provide only the selected provider's test credential and merchant through the
private test environment. Stripe uses `ANPFUEL_SANDBOX_STRIPE_SECRET` and
`ANPFUEL_SANDBOX_STRIPE_MERCHANT`. Asaas uses `ANPFUEL_SANDBOX_ASAAS_KEY` and
`ANPFUEL_SANDBOX_ASAAS_MERCHANT` with a current-format sandbox credential.
[Stripe API keys](https://docs.stripe.com/keys) and
[Asaas authentication](https://docs.asaas.com/docs/authentication-2) document
the test credential formats.
With `ANPFUEL_TEST_DATABASE_URL` pointing to disposable loopback PostGIS, run
from `backend/` once per reviewed channel:

```sh
go test -race -tags=integration,sandbox -count=1 -run '^TestSandboxCompanyBillingReplay$' -v ./internal/modules/payments/adapters
```

The test submits a controlled inbox notification, reads canonical payment
evidence from the selected sandbox and checks duplicate delivery against the
disposable ledger. HTTPS GET requests are limited to the selected provider
host. A missing case file is reported as skipped and earns no sandbox evidence.

## 10. Data, formulas and examples

Commercial numbers in [the product vision](../product-vision.md) remain hypotheses. Catalog and effective periods are data. Limits name measurable units and periods. Usage includes active vehicles or stations, admitted storage and API calls, with no invented production capacity. The vehicle meter counts scoped fleet records of kind `VEHICLE` in state `ACTIVE`. The existing fleet root lock serializes changes that increase this count. A denied activation preserves the draft and creates no activation acknowledgement. Money remains exact and source-bound.

The station meter counts explicit admissions linking a company establishment
to an approved station representation and operator revision. The current
profile operator, representation grant, company membership and establishment
must still match that review. A withdrawn association or ended proof drops
out of the count. The organization lock serializes competing admissions.

```sh
ops enterprise station admit --org <uuid> --approval-id <uuid> --operation-id <uuid>
```

The company plan shows the observed vehicle and station counts with their
reviewed ceilings. The station admission affects the company billing meter.
Public prices, ranking, reviews and moderation continue to use the station
surface’s separate source rules.

## 11. Screens

| ID | Surface | Objective | Actor | Data and actions | Validation |
|---|---|---|---|---|---|
| SCR-SUBS-01 | PANEL | Inspect own organization plan | Authorized manager | State, capability, limits and effective changes | Personal tier separate, no false payment success |
| SCR-SUBS-02 | PANEL | Resolve authorized overage | Permitted manager | Affected scoped resources and explicit selection | No deletion, exact limits and five shared states |
| SCR-PUB-01 | APP | Understand active product context | Account holder | Personal or named organization mode | Protected partition, explicit switch and accessible labels |

## 12. Acceptance, dependencies, risks, phase and microtasks

[EP16](../backlog/ep16-subscriptions.md) defines tasks. GOV-009 and the offer policy precede billing. VAL-18 and VAL-35 test verified-event replay and state effects. Keep public registration, ranking and social participation independent. The plan cannot certify provider or store readiness.

### Independent personal Premium and company plan checks

EMP-SUBS-010 verifies BR-AUTH-009 and BR-TEN-010 against current payment
and membership facts. Personal Premium is authorized by the personal
subscription projection and account proof. Company billing and its plan
limits are evaluated under the selected company scope and current member
permission. A change in one projection leaves the other unchanged. A
retained company export and an earlier signed fueling are evaluated by
their own company rules when the plan blocks new work.

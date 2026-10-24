# Stripe/Asaas offer and lifecycle policy v1

Status: POLICY_FROZEN / CATALOG_UNRESOLVED / PRODUCTION_SALES_DISABLED.
Date: 2026-10-24. Task: PAY-01.
Owner: [payment plan](../planning/STRIPE_ASAAS_PAYMENT_PLAN.md).
Rules: [B-BR-PAY-01–24 and BUC-PAY-01–10](STRIPE_ASAAS_PAYMENT_CONTRACT.md).
Golden cases: [offer-policy-v1.json](../../contracts/testdata/payments/offer-policy-v1.json).

This policy freezes the technical offer and paid-through envelope needed by the
payment implementation. It does **not** approve a commercial price, tax/fiscal
workflow, provider account or live sale. No monetary value in this document is a
production price. The only amount is a clearly synthetic test fixture
(`1234` centavos) and must never be shown to a payer or registered at a provider.

All three selected sale channels remain independently disabled:

| Channel | New sales | Maintenance (when implemented) | Current blocker |
|---|---|---|---|
| `stripe_card` | OFF | required | merchant/Price IDs, webhook, tax/consent review |
| `asaas_pix_manual` | OFF | required | merchant/API/webhook, catalog, fiscal review |
| `asaas_pix_automatic` | OFF | required | merchant eligibility, authorization contract, sandbox proof |

Disabling new sales must not disable webhook intake, reconciliation,
cancellation, refunds, export or already-earned paid-through access. No secret
presence may enable a channel; activation requires the approved catalog and the
provider-proof gates in the plan.

## Catalog v1

Catalog version: `pay-catalog-v1`. Currency: `BRL`; money is integer centavos,
never floating point. The server, not the client, selects offer, amount, term,
capabilities and provider mapping.

### Premium

| Field | Frozen value |
|---|---|
| Offer ID | `premium.monthly` |
| Product family | `premium` |
| Payer | one authenticated account |
| Term | one calendar month (`P1M`), monthly anchor |
| Capabilities | `cloud.sync`, `cloud.backup`, `cloud.restore`, `alerts.advanced`, `professional.write`, `professional.report`, `vehicles.multiple` |
| Channels | card, conventional Pix, Pix Automático (independent) |
| Production amount | **UNRESOLVED** |
| Test amount | `1234` centavos, synthetic only |
| Sale | **DISABLED** |

The capability list is the existing verified Premium bundle. Web payments extend
the existing subscription/access owner; they do not create a second entitlement
or grant free features retroactively. Free status/export/deletion and existing
local data remain outside paid checks.

### Enterprise candidate

No Enterprise offer is sellable. The only candidate is a single accountable payer
for a versioned regional fuel-price brief derived from admitted public data.

| Field | Frozen value |
|---|---|
| Offer ID | `enterprise.regional_brief.v1` (candidate, not sellable) |
| Payer | one accountable legal entity/person |
| Deliverable | periodic regional fuel-price brief; no private portal or API |
| Capabilities | none in the app; no team/seat/station/tenant right |
| Term | **UNRESOLVED** |
| Production amount | **UNRESOLVED** |
| Sale | **DISABLED** |

The candidate remains blocked by: deliverable not implemented, source
rights/redistribution pending, demand unvalidated, price/term unresolved and
fiscal workflow unresolved. Enterprise payment must never create organization
membership, team access, station representation/management, a private portal or
cross-account capability. Premium and Enterprise remain independent.

### Common catalog rules

- Exactly one settled payment funds exactly one paid period. A payment may not
  fund two periods and a period may not be funded twice. A repeated settlement
  for the same order is excess even when a future-period slot exists.
- At most one future period may be held beyond the current paid-through period.
  A second early payment is `EXCESS_UNAPPLIED` and does not extend access.
- No trial, coupon, zero-price grant, installment, wallet credit or automatic
  proration is supported in v1. Requests for them are refused.
- Only the monthly term is supported. Annual or multi-month terms require a new
  policy version.
- A price/catalog change applies at the next term boundary; already earned
  paid-through is preserved. A quote carries the catalog version and expires.
- Underpayment, overpayment, wrong currency or a client-supplied amount is
  refused; it never creates a partial period or a wallet balance.
- Provider prices/plans are mapped by explicit IDs at PAY-02/05. Startup must not
  create products, prices, customers or webhooks.

## Term, anchor and paid-through

Billing civil dates use `America/Sao_Paulo`; stored instants are UTC. Periods are
half-open `[start, end)`.

- The **first** period starts at the authoritative settlement instant and ends one
  calendar month later. The local settlement day becomes the contract anchor.
- Adding a month clamps the anchor to the target month's last day (for example,
  day 31 becomes February 28/29) while preserving the original anchor for later
  periods.
- **Early renewal** (settlement at or before current `end`) starts at current
  `end`, not at payment time. It does not create a gap or overlap.
- **Late renewal** (settlement after current `end`) starts at the settlement
  instant and resets the anchor to that local day. It grants no retroactive
  access.
- Grace is **0 seconds** in v1. At `end`, access is no longer active; card
  retries or a provider past-due state do not extend local access.
- Cancellation stops future renewal. It does not erase the paid-through period,
  automatically refund, delete data or disable free export.
- Access also requires the existing live account proof and a verified settlement.
  A redirect, QR creation, authorization, invoice creation or unknown provider
  state grants nothing.

Exact examples are in the golden fixture. The first and last examples:

| Case | Start | End |
|---|---|---|
| Initial settlement on 2026-01-31 | `2026-01-31T12:00:00Z` | `2026-02-28T12:00:00Z` |
| Leap-year settlement on 2028-01-31 | `2028-01-31T12:00:00Z` | `2028-02-29T12:00:00Z` |
| Early renewal while paid through 2026-02-28 | `2026-02-28T12:00:00Z` | `2026-03-31T12:00:00Z` |
| Late renewal after 2026-03-31, paid 2026-04-02 | `2026-04-02T12:00:00Z` | `2026-05-02T12:00:00Z` |

## Refunds, disputes and excess money

- Refunds are supported only to the original provider payment route, bounded by
  the original gross amount. A refund request/loss is not a completed refund.
- A **partial** refund is a financial adjustment and does not shorten access.
- A **cumulative full refund** or confirmed chargeback/reversal of the payment
  that funded a period revokes the unexpired remainder of that period at the
  confirmation instant. Already elapsed time is not re-granted.
- A pending dispute is not a confirmed fraud finding and does not by itself revoke
  access. A confirmed chargeback applies only to its matched payment/period.
- An account-level provider hold without a matched payment does not revoke every
  payer; it is surfaced for review.
- A duplicate or excess settlement is recorded as `EXCESS_UNAPPLIED`, never as
  extra access or an automatic wallet credit. Its refund is handled by PAY-16.
- Cancellation and account deletion never create an automatic refund.

## Switching and recurring funding sources

One product family has at most one recurring funding source per account.

- Plan/term switching takes effect at the next term boundary. No implicit
  proration; immediate upgrades/proration remain unsupported.
- The old recurring source must be canceled/fenced with a confirmed provider
  outcome before a new automatic source may debit. An unknown cancellation blocks
  the new automatic activation.
- Card ↔ Pix Automático: cancel the old renewal, confirm it, activate the new
  source for the next term, and preserve the current paid-through period.
- Pix Automático → card: do not create a card renewal before the old mandate is
  confirmed stopped.
- Manual Pix → card or Pix Automático: the manual paid period remains valid until
  its end; the recurring source may activate for the next term only. A manual
  payment is not recurring consent.
- Concurrent manual Pix and automatic settlement may produce excess money; it
  produces at most one paid period. Excess follows the refund/excess policy.

## Pix modes and scheduler choice

Conventional Pix and Pix Automático are different products with independent
switches:

- **Conventional Pix** creates one owner-bound charge/QR per order. It has no
  implicit recurring consent. Only an authoritative receipt funds its period.
- **Pix Automático** requires an admitted merchant, explicit payer authorization,
  and a mandate. The initial payment and the mandate are independent: a refused
  authorization can coexist with a paid first period, but no future debit is
  allowed until authorization is ACTIVE.
- The selected cycle owner is **Asaas provider-owned SUBSCRIPTION**. The
  application does not run a second cycle scheduler. If PAY-02 cannot admit the
  provider-owned contract for the required terms, Pix Automático remains disabled;
  switching to application-owned MANUAL requires a new recorded decision, not a
  silent fallback.

## Consent, legal, fiscal and distribution gates

- Recurring consent must state price, term, renewal cadence, cancellation and
  support contact before checkout. Final Portuguese wording and legal review are
  **PENDING**; the channel stays off until approved.
- A provider invoice/receipt is not a Brazilian fiscal document. Fiscal issuer,
  tax treatment, invoice handling and retention are **UNRESOLVED**. No tax engine
  is built in this scope.
- CPF/CNPJ, billing address and contact data are minimized; no CPF/CNPJ is
  required merely to create a free account. Retention/erasure of financial
  metadata requires the privacy review at PAY-15/20.
- Provider email/SMS/WhatsApp notifications are **OFF/UNDECIDED** to avoid
  duplicate or unapproved messages. Application notification delivery is
  configured separately.
- Android purchase UI, store links or steering are **DISABLED** until the
  applicable Play distribution/policy decision is verified. Web backend work does
  not authorize an in-app purchase flow.

## Provider mappings still unassigned

Stripe Price IDs, Asaas product/plan IDs, merchant/account IDs, webhook endpoints,
API/SDK versions, allowed return origins and test/live separation are not defined
by this policy. They are admitted at PAY-02/05 and proven at PAY-21. No API key
alone closes these gates.

## Golden acceptance cases

[offer-policy-v1.json](../../contracts/testdata/payments/offer-policy-v1.json)
contains synthetic, versioned cases for: month-end/leap anchors, early and late
renewal, zero grace boundaries, partial/cumulative/full refunds, pending dispute
vs confirmed chargeback, duplicate/excess settlement, amount mismatch, method
switching with confirmed/unknown cancellation, and disabled Premium/Enterprise/
trial/annual/Android sale paths.

PAY-03 must implement these deterministic rules behind provider-neutral ports.
PAY-06/07/10/11/12/13/14/16 must consume them. A case is accepted only when the
actual implementation test passes; this document and fixture are not runtime
evidence.

## Remaining owner inputs before any sale

1. Approved public Premium price, term and any trial decision.
2. Approved Enterprise deliverable/capabilities and price, plus source rights,
   demand evidence and a permitted delivery model.
3. Tax/fiscal issuer and retention decision; approved recurring consent text.
4. Provider merchant/account approval, catalog IDs, webhook setup and test/live
   separation.
5. Pix Automático eligibility and actual provider cycle contract.
6. Distribution decision for any Android purchase surface.

Until all applicable inputs and the phase-specific provider proofs exist, the
catalog remains `PRODUCTION_SALES_DISABLED`.

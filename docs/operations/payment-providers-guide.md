# Stripe and Asaas setup guide

Step-by-step provider setup for the staging host `teste.abastevo.com.br`.
The [payments runbook](payments.md) owns rotation, preflight, queue recovery
and channel disable. The
[subscriptions module](../enterprise/modules/subscriptions.md) owns the
company billing rules. This guide covers only what an operator clicks or
calls at each provider.

Rules for every step:

- Use Stripe test mode (or a Stripe Sandbox) and the Asaas Sandbox only.
- Keys, webhook secrets and tokens live in the host's protected environment
  file, created from [`infra/env.staging.example`](../../infra/env.staging.example).
  Never put one in this repository, an issue, a chat or a screenshot.
- Provider screens change. Each step links the provider's own walkthrough,
  read on 2026-11-21, instead of copying it.
- Screenshots belong in `docs/assets/payments/stripe/` and
  `docs/assets/payments/asaas/`. Take them only in test mode or sandbox and
  redact account, customer, key and e-mail values before committing.
- Sales stay off. Provisioning credentials or webhook destinations activates
  nothing (see [Production checklist](#production-checklist)).

## Stripe test mode

1. **Select the test environment.** In the Dashboard account picker, open a
   Sandbox or switch to test mode, and confirm it before opening any other
   page. [Sandboxes](https://docs.stripe.com/sandboxes) ·
   [testing use cases](https://docs.stripe.com/testing-use-cases)
2. **Keys.** Under **Developers → API keys**, copy the test secret key
   (`sk_test_…`, or a restricted `rk_test_…`). Set, on the host only:

   | Variable | Value |
   |---|---|
   | `ANPFUEL_STRIPE_MODE` | `test` |
   | `ANPFUEL_STRIPE_SECRET_KEY` | the test secret key |
   | `ANPFUEL_STRIPE_MERCHANT_ID` | the account ID (`acct_…`) shown in account settings |

   The API refuses a key whose account differs from the merchant ID.
   [API keys](https://docs.stripe.com/keys)
3. **Product and Price.** In **Product catalogue**, add a Product and one
   recurring Price: currency BRL, billing period monthly, fixed amount. Set
   `ANPFUEL_STRIPE_PRICE_ID` to its `price_…` ID. A company offer pins the same
   Price ID, plan revision, merchant, mode, amount in centavos and monthly
   term in `enterprise_billing_offers`.
   [Manage products and prices](https://docs.stripe.com/products-prices/manage-prices)
4. **Webhook destination.** In **Developers → Webhooks**, add the endpoint
   `https://teste.abastevo.com.br/v1/payments/stripe/webhook` with API version
   `2026-09-30.endive`. Intake refuses any other API version and any Connect
   event. Copy the signing secret (`whsec_…`) to
   `ANPFUEL_STRIPE_WEBHOOK_SECRET`. Select exactly these events, the only ones
   the worker routes:

   - `invoice.paid`
   - `invoice.payment_failed`
   - `invoice.payment_action_required`
   - `customer.subscription.updated`
   - `customer.subscription.deleted`
   - `checkout.session.completed`
   - `charge.refunded`
   - `charge.dispute.created`
   - `charge.dispute.updated`
   - `charge.dispute.closed`

   Any other authenticated event is stored and acknowledged with no effect.
   [Webhooks](https://docs.stripe.com/webhooks)
5. **Test cards.** Use any future expiry and any three-digit CVC.

   | Card | Scenario |
   |---|---|
   | `4242 4242 4242 4242` | payment succeeds |
   | `4000 0000 0000 0002` | generic decline |
   | `4000 0025 0000 3155` | 3-D Secure challenge; approve and refuse it once each |

   [Test cards](https://docs.stripe.com/testing)
6. **Stripe CLI (local intake only).** The listener prints its own signing
   secret; use that one for the local API, never the dashboard secret.

   ```sh
   stripe listen --events invoice.paid,invoice.payment_failed,invoice.payment_action_required,customer.subscription.updated,customer.subscription.deleted,checkout.session.completed,charge.refunded,charge.dispute.created,charge.dispute.updated,charge.dispute.closed \
     --forward-to http://127.0.0.1:8080/v1/payments/stripe/webhook
   stripe trigger invoice.paid
   ```

   A triggered event proves delivery and signature verification only. It is
   refused when the account's default API version is not the pinned one, and
   it can never settle a company term.
   [Stripe CLI](https://docs.stripe.com/stripe-cli)

### Company subscription on Stripe

The worker treats an invoice as a company invoice only when the finalized
invoice's subscription carries this metadata, set on the **subscription**:

| Key | Value |
|---|---|
| `abastevo_billing_scope` | `organization` |
| `abastevo_organization_id` | the organization UUID |
| `abastevo_organization_subscription_id` | the organization subscription UUID |

Do not add `order_id` or `account_id`: those mark a personal checkout and the
invoice is refused. The invoice must be collected automatically by card, in
BRL, with one subscription line of quantity 1 at the reviewed Price and no
proration. [Metadata](https://docs.stripe.com/metadata)

Order matters:

1. Create the subscription so its first invoice exists and is still unpaid.
2. Record the reviewed binding for that invoice (see
   [Binding a reviewed charge](#binding-a-reviewed-charge)).
3. Collect the payment.

A first invoice paid before its binding ends as a terminal conflict in the
inbox. A Checkout session creates the invoice at payment time, so it cannot
be bound in advance. A renewal invoice paid before its binding waits in the
inbox and settles once the binding exists.

## Asaas Sandbox

1. **Account and key.** Create the Sandbox account, then open
   **Integrações → Chaves de API** as an administrator and generate a key.
   Set, on the host only:

   | Variable | Value |
   |---|---|
   | `ANPFUEL_ASAAS_MODE` | `test` (calls `https://api-sandbox.asaas.com/v3`) |
   | `ANPFUEL_ASAAS_API_KEY` | the Sandbox API key |
   | `ANPFUEL_ASAAS_MERCHANT_ID` | the account UUID that Asaas sends as `account.id` in every webhook |

   [Sandbox](https://docs.asaas.com/docs/sandbox) ·
   [API keys](https://docs.asaas.com/docs/chaves-de-api)
2. **Webhook.** Under **Integrações → Webhooks**, register
   `https://teste.abastevo.com.br/v1/payments/asaas/webhook`. Generate a
   dedicated authentication token of 32 to 255 printable characters without
   spaces and set it as `ANPFUEL_ASAAS_WEBHOOK_TOKEN`. Asaas sends it in the
   `asaas-access-token` header. The API key is never accepted as the token.
   The worker routes every `PAYMENT_…` and `PIX_AUTOMATIC_RECURRING_…` event;
   subscribe to these:

   - `PAYMENT_CREATED`, `PAYMENT_RECEIVED`, `PAYMENT_CONFIRMED`, `PAYMENT_OVERDUE`
   - `PAYMENT_REFUNDED`, `PAYMENT_PARTIALLY_REFUNDED`
   - `PAYMENT_CHARGEBACK_REQUESTED`, `PAYMENT_CHARGEBACK_DISPUTE`,
     `PAYMENT_AWAITING_CHARGEBACK_REVERSAL`
   - `PIX_AUTOMATIC_RECURRING_AUTHORIZATION_CREATED`, `…_ACTIVATED`,
     `…_REFUSED`, `…_CANCELLED`, `…_EXPIRED`
   - `PIX_AUTOMATIC_RECURRING_PAYMENT_INSTRUCTION_CREATED`, `…_SCHEDULED`,
     `…_REFUSED`, `…_CANCELLED`
   - `PIX_AUTOMATIC_RECURRING_ELIGIBILITY_UPDATED`

   Only `PAYMENT_RECEIVED` can settle a term.
   [Webhook setup](https://docs.asaas.com/docs/receba-eventos-do-asaas-no-seu-endpoint-de-webhook) ·
   [charge events](https://docs.asaas.com/docs/webhook-para-cobrancas) ·
   [Pix Automático events](https://docs.asaas.com/docs/eventos-para-pix-autom%C3%A1tico)
3. **Company Pix charge.** Create a synthetic customer with notifications
   disabled, then a charge with `billingType=PIX`, the reviewed amount and due
   date, and

   ```text
   externalReference=abv_org:<organization UUID>:<checkout UUID>
   ```

   Both UUIDs are lowercase. The checkout UUID is chosen at review; the
   organization subscription UUID is a valid choice. The reviewed binding
   stores the whole string and settlement requires an exact match. Bind the
   charge before paying it: a payment received before its binding ends as a
   terminal conflict.
   [Create a customer](https://docs.asaas.com/reference/criar-novo-cliente) ·
   [create a charge](https://docs.asaas.com/reference/criar-nova-cobranca)
4. **Pix Automático.** Create a monthly authorization for the same customer
   and follow the provider's sandbox flow until the authorization is `ACTIVE`.
   Each cycle is its own charge: it carries `pixAutomaticAuthorizationId` and
   the same kind of `abv_org:` reference, and needs its own binding before the
   debit. `ANPFUEL_ASAAS_PIX_AUTOMATIC_NEW_SALES_ENABLED` stays `false`.
   [Authorization API](https://docs.asaas.com/reference/criar-uma-autorizacao-pix-automatico) ·
   [implementation guide](https://docs.asaas.com/docs/pix-automatico-implementacao)
5. **Confirm a sandbox payment.** Use the charge's Sandbox detail page or the
   sandbox-only confirmation endpoint. A confirmed payment does not by itself
   prove that an authorization became active.
   [Sandbox balance and confirmation](https://docs.asaas.com/docs/como-adicionar-dinheiro-para-testes) ·
   [confirmation endpoint](https://docs.asaas.com/reference/confirmar-pagamento)

## Binding a reviewed charge

A commercial review approves one existing Stripe invoice or Asaas charge for
one company offer and term in `org_billing_charge_approvals`. The restricted
operator login from
[`infra/db/billing-binding-role.sql`](../../infra/db/billing-binding-role.sql)
then records the binding:

```sh
ops enterprise billing bind --org <uuid> --approval-id <uuid> \
  --operation-id <uuid> --operator <uuid>
```

`ANPFUEL_ENTERPRISE_ENABLED` and `ANPFUEL_ENTERPRISE_BILLING_ENABLED` must be
true for the command. Reuse the same operation ID to retry. Binding creates no
payment and no entitlement. After the provider reports the payment, the
plan and billing overview of the company web app shows the settled charge,
its gross BRL amount and its term.

## Sandbox replay per channel

`TestSandboxCompanyBillingReplay` reads one existing sandbox charge from the
provider and settles it twice in a disposable ledger. It writes nothing to the
provider. Run it once for each of `stripe_card`, `asaas_pix_manual` and
`asaas_pix_automatic`, following
[Read-only provider sandbox replay](../enterprise/modules/subscriptions.md#read-only-provider-sandbox-replay)
for the private case file.

```sh
cd backend
go test -race -tags=integration,sandbox -count=1 \
  -run '^TestSandboxCompanyBillingReplay$' -v ./internal/modules/payments/adapters
```

| Variable | Use |
|---|---|
| `ANPFUEL_SANDBOX_COMPANY_BILLING_CASE` | path of the `0600` JSON case for one channel |
| `ANPFUEL_SANDBOX_STRIPE_SECRET`, `ANPFUEL_SANDBOX_STRIPE_MERCHANT` | Stripe test key and account |
| `ANPFUEL_SANDBOX_ASAAS_KEY`, `ANPFUEL_SANDBOX_ASAAS_MERCHANT` | Asaas Sandbox key and account UUID |
| `ANPFUEL_TEST_DATABASE_URL` | disposable loopback PostGIS, never staging |

A channel reported as `SKIP` has no evidence. Only a `PASS` for the channel
counts toward provider acceptance.

## Production checklist

New sales stay disabled until the owner accepts M8
([milestones](../enterprise/testing.md)). Before that gate:

- All three sandbox replay channels pass, and one signed staging payment per
  provider is reconciled under owner control.
- Offers, prices, contracts and fiscal terms are reviewed (EMP-GOV-009).
- Production keys, webhook secret and token are created separately from the
  test ones and stored only through the host secret workflow.
- The production Stripe destination uses API version `2026-09-30.endive` and
  the event list above; the merchant ID is the production account.
- The Asaas production account has a registered Pix key and, for Pix
  Automático, confirmed merchant eligibility.
- `ANPFUEL_STRIPE_CARD_NEW_SALES_ENABLED`,
  `ANPFUEL_ASAAS_PIX_MANUAL_NEW_SALES_ENABLED` and
  `ANPFUEL_ASAAS_PIX_AUTOMATIC_NEW_SALES_ENABLED` are still `false`.

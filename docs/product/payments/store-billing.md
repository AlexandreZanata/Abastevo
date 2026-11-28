# Store billing: Google Play and App Store

Owner decision, 2026-11-23: the mobile apps sell Premium only through the
store that installed them. Android uses Google Play Billing; iOS and iPadOS
use In-App Purchase through StoreKit 2. Stripe (card) and Asaas (Pix) remain
web-only channels under the [payment contract](README.md). The same verified
account receives the same Premium capabilities from any channel.

This contract supersedes the earlier "Google Play as a later alternative"
note in the payment contract. Prices, trial, billing periods and tax remain
owner decisions recorded in the store consoles and the server catalog
([D-PR01](../premium/README.md#decisions-to-freeze-in-prm00)). Store setup and
provider API details live in the
[store billing integration guide](../../backend/payments/store-billing-integration.md).

## Rules

- **B-BR-STORE-01 — Channel per surface.** The Android app starts purchases
  only with Play Billing and the iOS app only with StoreKit. Neither app
  embeds a web checkout, shows a web price or links to a web purchase unless a
  store program allows it for the storefront and the owner enabled it. Both are
  off. The website sells through Stripe and Asaas and never through a store.
- **B-BR-STORE-02 — One entitlement.** Every channel grants the frozen PRO
  capability bundle. The status read merges Play, App Store, web and admin
  sources for the account. A purchase from one channel unlocks the account on
  every platform. The status lists each active source with its own paid-through
  time.
- **B-BR-STORE-03 — Server authority.** A client result, receipt, signed
  transaction or notification never grants access by itself. The server asks
  the store for the canonical purchase, checks the app identity (package name
  or bundle ID), environment and a product from the server catalog, and only
  then commits the state. App Store payloads are accepted only with a valid
  signature chain to Apple Root CA - G3.
- **B-BR-STORE-04 — Account binding.** Before a purchase, the app reads a
  binding derived by the server from the account with a secret key. Play
  receives it as `obfuscatedAccountId` (64 hexadecimal characters). StoreKit
  receives it as `appAccountToken` (a UUID). Verification denies a purchase
  whose binding is missing or belongs to another account. Restore never moves
  a purchase between accounts.
- **B-BR-STORE-05 — Stable purchase key.** A Play subscription is keyed by its
  purchase token, and `linkedPurchaseToken` retires the replaced token. An App
  Store subscription is keyed by `originalTransactionId`, which stays the same
  across renewals and restores. The database stores a SHA-256 digest of
  provider, environment, app identity and key, plus the key encrypted at rest.
  Logs, jobs, exports and responses never carry the key.
- **B-BR-STORE-06 — Lifecycle mapping.** Pending purchases (Play `PENDING`,
  StoreKit Ask to Buy) grant nothing. Play states keep their frozen mapping.
  App Store subscription status maps as: 1 active → `ACTIVE`, or `CANCELED`
  when auto-renew is off; 2 expired → `EXPIRED`; 3 billing retry → `HOLD`;
  4 billing grace → `GRACE`; 5 revoked → `REVOKED`. A revocation date also
  means `REVOKED`. Access ends at the transaction `expiresDate`, or at the
  grace period end while in `GRACE`. An unknown value maps to `UNKNOWN` and
  grants nothing.
- **B-BR-STORE-07 — Notifications trigger a refresh.** Play real-time
  developer notifications and App Store Server Notifications V2 only enqueue a
  refresh of an already bound purchase. The refreshed canonical state decides
  access, never the notification type or time.
- **B-BR-STORE-08 — Completion.** The server acknowledges a verified Play
  subscription within the Play window. The iOS app finishes a StoreKit
  transaction only after the server accepted or definitively refused it. An
  unavailable server leaves the transaction unfinished, so the app retries.
- **B-BR-STORE-09 — Management and exit.** Each app opens its own store's
  subscription management page. Account deletion keeps the free exit, retires
  stored purchase keys and tells the user to cancel the store subscription in
  the store, because deleting the account does not stop store billing.
- **B-BR-STORE-10 — Fail closed.** A store channel stays off until its
  credentials and catalog are configured. Without a configured offer for the
  app's store, the app shows the current status with no purchase button.
  When Google Play cannot return the listed plans, Android shows a retry action
  in the subscription section. Prices and the billing note appear with loaded plans.
  Payment never changes trust, moderation or free participation.
- **B-BR-STORE-11 — Play license test access.** In the `test` environment,
  Google Play offers, verification and restore are limited to explicitly
  listed Abastevo accounts. The backend uses the authenticated account ID.
  An empty, unreadable or invalid list denies all Play offers and owner
  purchase requests. Canonical verification still requires a Google Play
  test purchase. Existing acknowledgements, notifications and reconciliation
  continue when test access is removed.

## Use cases

- **BUC-STORE-01 — Buy in the app.** Signed-in owner → status lists the
  store offer and binding → store sheet → app sends the purchase key with a
  signed request → server verifies with the store → Premium is active → Play
  purchase acknowledged by the server or StoreKit transaction finished by the
  app.
- **BUC-STORE-02 — Restore.** Owner signs in on a new device → app lists the
  store's current entitlements → signed restore for each key → server verifies
  the binding → Premium is active. A purchase bound to another account is
  refused.
- **BUC-STORE-03 — Renew, lapse, refund.** Store notification → refresh job →
  canonical state → access extended, put on hold or revoked.
- **BUC-STORE-04 — Cross-platform.** A subscription bought on the web or on
  the other platform appears as an active source in the status. The app shows
  Premium as active without offering a second purchase.
- **BUC-STORE-05 — Test Play on the public app.** A listed Abastevo test
  account signs in and buys with a Google Play license tester and a test
  payment method. An ordinary account sees no Play offer and a signed attempt
  to verify or restore is denied before a purchase is recorded.

## API contract (v1)

All routes are `POST`, JSON, `Cache-Control: no-store`.

`/v1/premium/status` with the session proof (`family_id`, `access_token`)
returns the existing status plus `store_offers`, one entry per configured
store:

```json
{
  "store_offers": [
    {
      "provider": "play",
      "products": [{"product_id": "abastevo_premium", "plan_ids": ["monthly", "yearly"]}],
      "account_binding": "<64 lowercase hex characters>",
      "management_url": "https://play.google.com/store/account/subscriptions?package=com.anpfuel.app"
    },
    {
      "provider": "app_store",
      "products": [{"product_id": "com.anpfuel.app.premium.monthly", "plan_ids": []}],
      "account_binding": "<lowercase UUID>",
      "management_url": "https://apps.apple.com/account/subscriptions"
    }
  ]
}
```

`tiers[].source` is `play`, `app_store`, `web` or `admin`.

`/v1/premium/purchases/verify` and `/v1/premium/purchases/restore` take the
session proof, a fresh UUID `operation_id`, `purchase_token` and `provider`
(`play` when absent, or `app_store`). For `app_store`, `purchase_token` is the
transaction's `originalTransactionId` as a decimal string. The request must be
signed by the device key bound to the account (RFC 9421 profile of the
identity module). Errors: `400` invalid, `401` proof, `403` denied (binding,
product, environment or app mismatch), `409` retry, `429` quota, `503`
provider or channel unavailable.

`/v1/premium/provider-notifications` receives Play real-time developer
notifications through an authenticated Pub/Sub push.
`/v1/premium/app-store-notifications` receives App Store Server Notifications
V2 (`{"signedPayload": "..."}`); the signature chain is the authentication.

## Play free trial

- **B-BR-STORE-12 — Play free trial.** For the `abastevo_premium` monthly
  and yearly base plans, Android may show `mes-gratis` only when Play
  returns the offer for the current Google account. It must contain a
  single zero-price month followed by the paid recurring base-plan
  period. Show the full localized renewal price and period before
  purchase. Other offers and invalid trial terms fall back to the
  ordinary base plan. The free app tier requires no subscription.
- **BUC-STORE-06 — Start a Play trial.** An eligible first-time subscriber
  sees one free month followed by the full recurring price, then opens the
  Play checkout with that offer token. A subscriber without the trial
  receives the ordinary paid offer. Server verification and renewal
  handling follow the same rules for both.

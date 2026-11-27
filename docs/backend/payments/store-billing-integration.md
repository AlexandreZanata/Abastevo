# Store billing integration guide

How Abastevo sells Premium inside the apps through Google Play and the App
Store, and how the backend verifies those purchases. The rules are in the
[store billing contract](../../product/payments/store-billing.md). Web
payments (Stripe card and Asaas Pix) are in the
[payment contract](../../product/payments/README.md).

Primary sources were checked on 2026-11-23. Recheck the policy and fee
sections before each release; they change often.

## Architecture

```text
Android app ── Play Billing Library ──► Google Play ──► RTDN (Pub/Sub push)
     │  obfuscatedAccountId = binding                        │
     └─ signed POST /v1/premium/purchases/verify ─┐          ▼
                                                  │   /v1/premium/provider-notifications
iOS app ──── StoreKit 2 ──────────► App Store ──► App Store Server Notifications V2
     │  appAccountToken = binding                            │
     └─ signed POST /v1/premium/purchases/verify ─┤          ▼
                                                  │   /v1/premium/app-store-notifications
                                                  ▼
                       subscription module (Storefront)
            ┌──────── Play service ────────┬──────── App Store service ───────┐
            │ Android Publisher API        │ App Store Server API             │
            │ subscriptionsv2.get, :ack    │ /inApps/v1/subscriptions/{id}    │
            └──────────────┬───────────────┴──────────────┬───────────────────┘
                           ▼                              ▼
             subscription_purchases (provider, environment, app id, digest)
                           ▼
           POST /v1/premium/status merges play + app_store + web + admin
```

- `backend/internal/modules/subscription/adapters/play`: Android Publisher
  client and the service-account OAuth token source.
- `backend/internal/modules/subscription/adapters/appstore`: App Store Server
  API client, JWS verification pinned to Apple Root CA - G3, notification
  decoding.
- `backend/internal/modules/subscription/application/storefront.go`: routes
  commands, jobs and reconciliation to the store that issued each purchase.
- `backend/internal/modules/subscription/adapters/storefront.go`: builds the
  runtime from configuration. `cmd/api` and `cmd/worker` both use it.
- Migration `000300_subscription_store_provider.sql` adds
  `subscription_purchases.provider` (`play` for every earlier row).

### Purchase sequence (both stores)

1. The signed-in app calls `POST /v1/premium/status`. The response lists
   `store_offers` with the product IDs and the owner's `account_binding`.
2. The app shows the store's localized price and starts the store purchase
   with the binding (Play `setObfuscatedAccountId`, StoreKit
   `.appAccountToken(UUID)`).
3. On success the app sends `POST /v1/premium/purchases/verify` with
   `provider`, a fresh `operation_id` and the purchase key (Play
   `purchaseToken`, App Store `originalTransactionId`). The request is signed
   with the device key bound to the account.
4. The server reads the canonical purchase from the store, checks app
   identity, environment, product and binding, stores the encrypted key and
   returns the merged status.
5. Play: the server enqueues the acknowledgement job. App Store: the app
   calls `transaction.finish()` after a 200 or a definitive 403.
6. Renewals, cancellations, refunds and holds arrive as notifications. Each
   one enqueues `subscription-refresh`, which reads the canonical state
   again. The worker also reconciles due purchases every minute.

## Google Play

### Console setup (owner)

1. Play Console → the app → **Monetize with Play → Products →
   Subscriptions**: create product `abastevo_premium` with base plans such as
   `monthly` and `yearly` (auto-renewing). Prices are an owner decision.
2. **Payments profile** with tax and bank data must be complete.
3. Google Cloud: enable the **Google Play Android Developer API**, create a
   service account and a JSON key. Linking the developer account to a Cloud
   project is no longer required.
4. Play Console → **Users and permissions → Invite new users** → the
   service-account email with **View financial data, orders, and
   cancellation survey responses** and **Manage orders and subscriptions**.
5. Real-time developer notifications: create a Pub/Sub topic, grant
   `google-play-developer-notifications@system.gserviceaccount.com` the
   **Pub/Sub Publisher** role, create a **push** subscription to
   `https://<api host>/v1/premium/provider-notifications` with
   authentication enabled (a service account and an audience). Then **Play
   Console → Monetization setup → Real-time developer notifications**: topic
   `projects/<project>/topics/<topic>`, notifications for subscriptions and
   voided purchases, and **Send test message**.
6. Testing: add license testers (Play Console → Settings → License testing)
   and publish to an internal track.

### Client (Play Billing Library)

- Library: PBL 9.1.0 (2026-06-18). Apps and updates must use PBL 8 or newer
  since 2026-11-01 (PBL 7 deadline). The app uses the Java artifact
  `com.android.billingclient:billing` because `billing-ktx` 8.1+ requires a
  newer Kotlin standard library than the project uses.
- Client: `BillingClient.newBuilder(context).setListener(listener)
  .enablePendingPurchases(PendingPurchasesParams.newBuilder()
  .enableOneTimeProducts().build()).enableAutoServiceReconnection().build()`.
- Products: `queryProductDetailsAsync` with `ProductType.SUBS`; the result is
  a `QueryProductDetailsResult` (`productDetailsList`,
  `unfetchedProductList`). Do not cache `ProductDetails`.
- Launch: `BillingFlowParams.newBuilder().setProductDetailsParamsList(listOf(
  ProductDetailsParams.newBuilder().setProductDetails(pd).setOfferToken(token)
  .build())).setObfuscatedAccountId(binding).build()` and
  `launchBillingFlow(activity, params)` on the main thread. `OK` only means
  the sheet opened. The offer token comes from
  `subscriptionOfferDetails` (base plan, offer, pricing phases).
- `obfuscatedAccountId` is at most 64 characters and must not be cleartext
  PII; the server binding is 64 hexadecimal characters of an HMAC.
- Results arrive in `onPurchasesUpdated`. Grant nothing for
  `PurchaseState.PENDING`. Handle `USER_CANCELED`, `ITEM_ALREADY_OWNED`
  (restore), `BILLING_UNAVAILABLE`, `SERVICE_UNAVAILABLE`, `NETWORK_ERROR`
  and `ERROR` as recoverable states.
- Call `queryPurchasesAsync(QueryPurchasesParams.newBuilder()
  .setProductType(ProductType.SUBS).build())` at start, on resume and for
  Restore. Any `PURCHASED` and unacknowledged purchase is sent to verify
  again, so a server outage does not lose it.
- Do not acknowledge on the client. An initial subscription that is not
  acknowledged within three days is refunded and revoked; the server
  acknowledges after verification. In license testing the window is a few
  minutes.
- Suspended subscriptions (`Purchase.isSuspended()`, PBL 8.1+) grant nothing;
  send the user to the Play subscription center.

### Server (Android Publisher API)

- OAuth: JWT bearer grant (RFC 7523) signed RS256 with the service-account
  key, `scope=https://www.googleapis.com/auth/androidpublisher`,
  `aud=https://oauth2.googleapis.com/token`, at most one hour. The endpoint is
  pinned in code, not read from the key file.
- Read: `GET https://androidpublisher.googleapis.com/androidpublisher/v3/
  applications/{packageName}/purchases/subscriptionsv2/tokens/{token}`.
  Fields used: `subscriptionState`, `lineItems[].productId`,
  `lineItems[].expiryTime`, `lineItems[].offerDetails.basePlanId`,
  `acknowledgementState`, `linkedPurchaseToken`,
  `externalAccountIdentifiers.obfuscatedExternalAccountId`, `testPurchase`.
- State mapping: `SUBSCRIPTION_STATE_ACTIVE` → `ACTIVE`, `_CANCELED` →
  `CANCELED` (access until `expiryTime`), `_IN_GRACE_PERIOD` → `GRACE`,
  `_ON_HOLD` → `HOLD`, `_PAUSED` → `PAUSED`, `_EXPIRED` and
  `_PENDING_PURCHASE_CANCELED` → `EXPIRED`, `_PENDING` → `PENDING`. A
  revocation appears as `_EXPIRED`; a token Google no longer knows becomes
  `REVOKED` on refresh.
- `linkedPurchaseToken` (upgrade, downgrade, in-app resubscribe) retires the
  old token for the same account.
- Acknowledge: `POST …/purchases/subscriptions/{subscriptionId}/tokens/
  {token}:acknowledge`.
- RTDN push: Pub/Sub sends `Authorization: Bearer <Google OIDC JWT>`. The
  server checks RS256 with Google's published keys, `aud`, `email`,
  `email_verified` and the subscription name, then decodes
  `message.data`. `subscriptionNotification` and
  `voidedPurchaseNotification` with `productType` 1 (refund or chargeback)
  schedule a refresh. `testNotification`, one-time product events and
  envelopes older than ten minutes return 204 (reconciliation covers them).
  Notification types 1–22 carry no state; the refresh decides.

### Policy, fees and Brazil

- Digital subscriptions sold in the app must use Play Billing. The Payments
  policy forbids leading users to another payment method except under an
  enrolled program.
- Brazil: subscriptions pay a 15% service fee. The user choice billing pilot
  lists Brazil (fee reduced for alternative billing); external offers and
  external content links are not available in Brazil. CADE inquiry
  08700.002969/2024-61 is open with no remedy found. Abastevo does not enroll
  in any program: the Android app shows no web price or link.
- Testing renewals with license testers: 1 week or 1 month = 5 minutes, 3
  months = 10, 6 months = 15, 1 year = 30; at most 6 renewals; grace 5
  minutes, hold 10. Play Billing Lab can change country and accelerate
  states.

## App Store

### App Store Connect setup (owner)

1. The Account Holder accepts the **Paid Applications Agreement** and
   completes tax and banking (Business).
2. **Monetization → Subscriptions**: one subscription group (for example
   "Abastevo Premium") and products such as
   `com.anpfuel.app.premium.monthly` and `com.anpfuel.app.premium.yearly`
   with duration, price, localized display name and description, and a
   review screenshot. The first subscription is submitted with an app
   version.
3. **Users and Access → Integrations → In-App Purchase → Generate In-App
   Purchase Key**. Download the `.p8` once; copy the Key ID and the Issuer
   ID.
4. **App Information → App Store Server Notifications**: Production and
   Sandbox URLs `https://<api host>/v1/premium/app-store-notifications`,
   **Version 2**. Without a sandbox URL both environments use the production
   URL.
5. Optional: enroll in the App Store Small Business Program.
6. Sandbox testers: **Users and Access → Sandbox**. TestFlight builds also
   buy in the sandbox.

### Client (StoreKit 2, iOS 17+)

- Products: `try await Product.products(for: ids)`; unknown IDs are omitted.
- Purchase: `try await product.purchase(options: [.appAccountToken(uuid)])`.
  Result: `.success(VerificationResult<Transaction>)`, `.userCancelled`,
  `.pending` (Ask to Buy; grants nothing).
- Accept only `.verified(transaction)`; send `transaction.originalID` to the
  server. Call `await transaction.finish()` only after the server accepted it
  or refused it with 403.
- Start a `Transaction.updates` listener at launch; it delivers unfinished
  transactions, renewals, Ask to Buy approvals, offer codes and purchases
  from other devices.
- Restore: iterate `Transaction.currentEntitlements` and verify each
  subscription; call `AppStore.sync()` only from an explicit Restore action
  (it prompts for the Apple Account).
- Manage: `.manageSubscriptionsSheet(isPresented:)`; refunds:
  `.refundRequestSheet(for:isPresented:)`.
- The app shows the subscription name, period and full renewal price before
  purchase, plus links to the Terms of Use (EULA) and the Privacy Policy
  (guideline 3.1.2).
- Local testing: an Xcode `.storekit` configuration signs transactions with a
  local certificate, so the backend refuses them; use the sandbox for
  end-to-end tests.

### Server (App Store Server API)

- Hosts (recommended since 2026-05-05): `https://api.storekit.apple.com` and
  `https://api.storekit-sandbox.apple.com`. The older
  `*.itunes.apple.com` hosts remain supported.
- Authentication: ES256 JWT, header `alg=ES256`, `kid=<Key ID>`, `typ=JWT`;
  claims `iss=<Issuer ID>`, `iat`, `exp` (at most 60 minutes; the server uses
  20), `aud=appstoreconnect-v1`, `bid=com.anpfuel.app`.
- Read: `GET /inApps/v1/subscriptions/{originalTransactionId}` (Get All
  Subscription Statuses). The server picks `lastTransactions[]` with the
  same `originalTransactionId` and verifies `signedTransactionInfo` and
  `signedRenewalInfo`.
- Signed data: compact JWS, `alg=ES256`, `x5c` = leaf, intermediate, root.
  The server rebuilds the chain to the embedded Apple Root CA - G3
  (SHA-256 `63:34:3A:BF:B8:9A:6A:03:EB:B5:7E:9B:3F:5F:A7:BE:7C:4F:5C:75:6F:30:
  17:B3:A8:C4:88:C3:65:3E:91:79`, valid to 2039-04-30) and requires the
  intermediate OID `1.2.840.113635.100.6.2.1` and the leaf OID
  `1.2.840.113635.100.6.11.1`. The root sent in `x5c` is not trusted.
- Checks: `bundleId`, `environment` (`Production` or `Sandbox`), `type` =
  `Auto-Renewable Subscription`, `productId` in the server catalog,
  `originalTransactionId` equal to the submitted key, `appAccountToken`
  equal to the account binding.
- Status mapping: 1 active → `ACTIVE` (`CANCELED` when `autoRenewStatus` is
  0), 2 → `EXPIRED`, 3 billing retry → `HOLD`, 4 grace → `GRACE` (access to
  `gracePeriodExpiresDate`), 5 → `REVOKED`; a `revocationDate` is always
  `REVOKED`. Access ends at `expiresDate`. Dates are Unix milliseconds.
- App Review and TestFlight buy in the sandbox while talking to the
  production server. Apple's guidance is to call production first and the
  sandbox on `TransactionIdNotFound` (4040010). The server does this only
  when `ANPFUEL_APPSTORE_SANDBOX_FALLBACK=true`; enable it before submitting
  for review. TestFlight subscriptions renew daily at most six times.
- Notifications V2: body `{"signedPayload": "<JWS>"}` with
  `notificationType`, `subtype`, `notificationUUID`, `version` 2.0 and
  `data` (`bundleId`, `environment`, `signedTransactionInfo`,
  `signedRenewalInfo`, `status`). The server verifies the chain, refuses
  another app or environment, deduplicates on `notificationUUID` and
  schedules a refresh. `TEST` returns 200 without effect. Apple retries
  non-2xx replies at 1, 12, 24, 48 and 72 hours; the sandbox does not retry.
- Rate limits: about 50 requests per second per app for most endpoints.

### Policy, fees and Brazil

- Guideline 3.1.1: digital content and subscriptions inside the app use
  In-App Purchase. 3.1.2: auto-renewable subscriptions give ongoing value,
  last at least seven days, work on all the user's devices and show the
  required disclosures. 3.1.3(b): access bought on the web may be used in the
  app because the same Premium is also sold through In-App Purchase.
- Guideline 5.1.1(v): in-app account deletion; for subscribers, tell them
  billing continues through Apple and link the subscription management
  page.
- Brazil (CADE settlement 2025-12-23, effective with iOS 26.5): apps may use
  alternative in-app payment processing or out-of-app offers with link
  entitlements (`com.apple.developer.storekit.custom-purchase-link.allowed-regions`
  = `["br"]`), with In-App Purchase still offered at least as prominently and
  monthly reports through the External Purchase Server API. Abastevo does not
  request these entitlements; without them guideline 3.1.1(a) forbids calls
  to action toward other purchase methods, so the iOS app shows no web price
  or link.
- Brazil commission with In-App Purchase: 21% + 5% processing (standard) or
  10% + 5% (Small Business Program, or subscriptions after the first year).
  Outside Brazil's table the Small Business Program rate is 15%.
- Sandbox renewal (default rate): 1 week = 3 minutes, 1 month = 5, 3 months =
  15, 6 months = 30, 1 year = 1 hour; up to 12 renewals.

## Server configuration

Store credentials and the catalog are required to start each channel.
Configuration warnings omit secrets and account IDs.

| Variable | Meaning |
|----------|---------|
| `ANPFUEL_STORE_ENVIRONMENT` | `test` (Play license testing, App Store Sandbox) or `production`. |
| `ANPFUEL_STORE_TOKEN_KEY` | 32 bytes in base64: AES-GCM key for stored purchase keys. |
| `ANPFUEL_STORE_BINDING_KEY` | 32 bytes in base64: HMAC key for account bindings. Rotating it breaks restore of existing purchases. |
| `ANPFUEL_PLAY_PACKAGE_NAME` | Default `com.abastevo`. |
| `ANPFUEL_PLAY_SERVICE_ACCOUNT_FILE` | Path to the mounted service-account JSON key. |
| `ANPFUEL_PLAY_PRODUCTS` | `abastevo_premium:monthly,yearly` (entries separated by `;`). |
| `ANPFUEL_PLAY_TEST_ACCOUNTS_FILE` | In `test`, path to a private file with one lowercase Abastevo account UUID per line. Blank lines are allowed. Missing, empty or malformed files disable Play offers and app-initiated verification and restore. |
| `ANPFUEL_PLAY_PUSH_SUBSCRIPTION` | `projects/<project>/subscriptions/<name>` of the push subscription. |
| `ANPFUEL_PLAY_PUSH_AUDIENCE` | Audience configured on the push subscription. |
| `ANPFUEL_PLAY_PUSH_SERVICE_ACCOUNT` | Service-account email that signs push tokens. |
| `ANPFUEL_APPSTORE_BUNDLE_ID` | Default `com.anpfuel.app`. |
| `ANPFUEL_APPSTORE_ISSUER_ID` | Issuer ID of the In-App Purchase key. |
| `ANPFUEL_APPSTORE_KEY_ID` | Key ID of the In-App Purchase key. |
| `ANPFUEL_APPSTORE_PRIVATE_KEY_FILE` | Path to the mounted `.p8` key. |
| `ANPFUEL_APPSTORE_PRODUCTS` | Comma-separated product IDs. |
| `ANPFUEL_APPSTORE_SANDBOX_FALLBACK` | `true` lets production verify sandbox purchases (App Review, TestFlight). Default `false`. |

Mount key files as secrets; never commit them. Use the `test` environment on
a non-production deployment for sandbox and license-tester runs.

When the public app uses the test backend, mount the same tester file on the
API and worker and verify an ordinary account receives no Play offer before
activating a base plan. Keep out-of-app promotion disabled. On the tester's
device use the Google Play license tester account that installed the app and
a test payment method. Let test purchases expire or revoke them, then drain
their lifecycle jobs before changing the backend environment. When another
store channel is active, use a separate deployment because both channels
share `ANPFUEL_STORE_ENVIRONMENT`.

## Acceptance checklist

- Play: license tester buys, the status shows `play` PRO, the
  acknowledgement job completes, an accelerated renewal and a cancellation
  arrive through RTDN, a refund (voided purchase) revokes, Restore on a
  second device with the same account works and a different account is
  refused.
- App Store: sandbox tester buys, the transaction is finished after the
  server accepts, renewals, `DID_CHANGE_RENEWAL_STATUS`, `REFUND` and
  `EXPIRED` notifications change the status, Restore works on a second
  device, another account is refused, and a review build against production
  works with the sandbox fallback on.
- Cross-platform: a web purchase and a purchase from the other store show as
  active without a purchase button.

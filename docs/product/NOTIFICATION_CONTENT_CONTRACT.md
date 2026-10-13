# Notification content and OneSignal delivery contract

State: PLANNED / CONTENT PROPOSAL ONLY, 2026-10-13. User asked to add phases and message bodies now, applying real backend/Android delivery later. No SDK, live account, credential, send, backend runtime or Android change is performed by this document. [PRM11–PRM14](../planning/PREMIUM_BACKEND_APP_PLAN.md#onesignal-notification-amendment--planning-only-2026-10-13) own future implementation.

## Cost and architecture

Provider selected: OneSignal. Our existing backend hosts event eligibility, private inbox, templates, filters, quiet windows, quotas and durable jobs; OneSignal operates delivery. Official pricing checked 2026-10-13: Free mobile push currently applies to organizations with up to 1,000 monthly active users; verify counting/features before launch. No paid upgrade or paid-only targeting feature is authorized. Hosting a sender does not remove provider limits or our existing hosting costs. [Official pricing](https://onesignal.com/pricing).

Keep audience selection/advanced filters in our backend. Use the OneSignal Free API capabilities only, behind a provider port. Require explicit owner review if limits cease to fit; pause external sends and retain inbox/basic local alerts rather than incur unapproved charges.

## Body templates proposed for pt-BR

Values below are placeholders, not actual station prices/promotions. Render exact Brazilian currency and product unit; include public source/time/conditions in the detail. Reject missing required placeholders rather than fabricate them.

- **new-price-v1** — Title: “Preço atualizado em {posto}”. Body: “{combustivel}: {preco_formatado}/{unidade}. Informado às {hora_local}.” Requires a current supported projection; a pending photo/submission alone cannot trigger this wording. Tap: public station/fuel detail with current source and time.
- **price-drop-v1** — Title: “Preço caiu em {posto}”. Body: “{combustivel} passou de {preco_anterior} para {preco_atual}/{unidade}. Confira a atualização.” Compare the same fuel/unit/condition and eligible source/time baseline. This does not paywall the existing local weekly drop alert.
- **promotion-v1** — Title: “Promoção informada em {posto}”. Body: “{beneficio_resumido}. {condicao_resumida}. Válida até {validade_local}. Confira as regras.” Source, actual terms, expiry and separate promotion consent are mandatory. Loyalty/app/payment restrictions stay explicit; promotional price never competes as an unconditional price.
- **premium-threshold-v1** — Title: “Seu alerta encontrou um preço”. Body: “{combustivel} a {preco_formatado}/{unidade} em {posto}, dentro do limite de {limite_formatado}/{unidade}.” Requires current verified Premium rule and supported fresh price; threshold filters do not buy ranking/trust.
- **premium-combined-v1** — Title: “Preço encontrado para seu filtro”. Body: “{posto}: {combustivel} a {preco_formatado}/{unidade}. {condicao_resumida}. Confira os detalhes.” All selected rule predicates must match; use truthful condition copy and selected city/station scope, no contributor GPS.
- **premium-digest-v1** — Title: “Resumo dos seus alertas”. Body: “{quantidade} opções atendem aos seus filtros em {cidade}. Atualizado às {hora_local}.” Count only still-valid candidates; no empty or invented count, best-price guarantee or implied complete city coverage.
- **private-generic-v1** — Title: “Atualização no abastevo”. Body: “Há uma atualização para você. Abra o app para conferir.” Optional lockscreen-safe fallback; opening still authenticates the current account before private data.

## Proposed body envelope

Versioned internal fields: schema_version, event_id, event_type, template_id, locale, rendered title/body, created_at, expires_at, source reference/condition and an allowlisted typed navigation target. Recipient account/rule/device routing remains server-private and is translated to an opaque OneSignal alias/subscription set. Never send access/purchase tokens, email, precise GPS, receipt photos or financial records to the provider. No arbitrary web URL or HTML; unknown version/expired payload opens safe inbox recovery only.

OneSignal request fields are adapter-owned: configured app_id, target channel, localized headings/contents, minimal typed data, eligible opaque recipients, idempotency key and event-bounded TTL. Server App API key never enters the APK. Render content and target audience in our backend; no reliance on paid provider journeys/segmentation/timezone automation. Verify current API field support and Free availability in PRM11/12. [Message API](https://documentation.onesignal.com/reference/create-message).

Require server-proven device/account binding and supported identity verification before private alias targeting; client login/external_id alone is not ownership. Logout/revoke/account switch must unlink old routing, and tap rechecks current ownership/expiry. [Identity verification](https://documentation.onesignal.com/docs/en/identity-verification).

## Experience and acceptance

New-price opt-in and promotional opt-in are distinct; marketing is not implied by free account registration. Premium enables new advanced filter combinations/windows, while existing free basic/follow alerts stay free. Keep category mute, quiet hours, cooldown, dedupe, TTL and best-effort windows; no notification flood or exact-time guarantee.

Provider acceptance, device receipt, OS display and user open are different states. Backend inbox remains authoritative under push loss/delay. No always-on tracking/advertising SDK configuration is authorized. Use existing Android components/notification channel conventions, Portuguese strings and accessibility. Real new-price/promotion publication rules and sender/SDK/privacy/cost/device tests must pass in their owning phases before any send.

This contract is independent of Google Play RTDN billing notifications. Current first-ten backend tasks continue unchanged; notifications implementation is future scope.

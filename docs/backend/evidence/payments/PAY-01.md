# PAY-01 — offers and lifecycle policy freeze

Status: LOCAL_DONE
Validation: PASS
Date: 2026-10-24
Depends on: PAY-00
Branch: `codex/payments-stripe-asaas`
Base/branch parent: `5f66c0d1` (PAY-00) on `721582b8`.

Objective: freeze a deterministic, provider-neutral Premium/Enterprise offer and
paid-through/refund/switching policy, with versioned golden examples, without
inventing a production price or enabling any sale channel.

Rules covered: B-BR-PAY-01 (catalog authority), 03 (state separation), 04 (paid
evidence), 07 (two Pix modes), 08 (one scheduler), 09/10 (idempotency/ambiguity),
14 (period/access), 15 (cross-provider), 16 (refund/dispute), 17 (proportionate
risk), 18 (capability isolation), 23 (activation). Use cases: BUC-PAY-01/02/03/04/
05/07/08/09.

## Frozen decisions

- Premium offer `premium.monthly`, one authenticated account, one calendar-month
  term (`P1M`), capabilities exactly `cloud.sync`, `cloud.backup`, `cloud.restore`,
  `alerts.advanced`, `professional.write`, `professional.report`,
  `vehicles.multiple`.
- Production Premium price is **UNRESOLVED**. The only amount is the synthetic
  fixture value `1234` centavos; it is not a real price and cannot enable checkout.
- Enterprise remains non-sellable. Candidate only: `enterprise.regional_brief.v1`,
  one accountable payer, regional fuel-price brief, no app capability, team,
  seat, station, tenant, private portal or API. Blocked by unimplemented
  deliverable, source rights, demand, price/term and fiscal workflow.
- One settled payment funds exactly one period; at most one future period may be
  held. Duplicate/excess settlement is `EXCESS_UNAPPLIED`, never extra access.
- Periods are half-open `[start,end)` in `America/Sao_Paulo` civil dates with UTC
  storage; month-end anchors clamp to the target month's last day. Early renewal
  starts at current end; late renewal starts at settlement and resets the anchor.
  Grace is zero seconds.
- Partial refunds are financial-only. Cumulative full refund or confirmed
  chargeback of the funding payment revokes only the unexpired matched period.
  Pending disputes and unmatched account holds do not revoke access.
- One recurring funding source per product family. Switching takes effect at the
  next term boundary with no proration; an unknown old cancellation blocks the new
  automatic source.
- Conventional Pix and Pix Automático remain independent. Pix Automático uses
  **Asaas provider-owned SUBSCRIPTION** cycles; application-owned MANUAL is not a
  silent fallback. Initial payment and mandate activation remain independent.
- All three channels are OFF: `stripe_card`, `asaas_pix_manual`,
  `asaas_pix_automatic`. Maintenance (webhooks, reconciliation, cancellation,
  refunds, export, earned access) must remain available when sales are off.
- No trial, coupon, installment, wallet, proration or annual term in v1.
- Consent text, tax/fiscal issuer, provider mappings, merchant eligibility and
  Android distribution policy remain unresolved/disabled.

## Artifacts and shared paths

- New canonical policy: `docs/product/STRIPE_ASAAS_OFFER_POLICY.md`.
- New versioned fixture: `contracts/testdata/payments/offer-policy-v1.json`
  (26 synthetic cases: 6 period, 3 access, 3 refund, 2 dispute, 2 excess,
  2 amount, 3 switch, 5 sale).
- Additive shared edit: `docs/product/PREMIUM_PRODUCT_CONTRACT.md` D-PR01 now
  points to the PAY-01 policy and explicitly preserves owner price/term decisions
  and disabled sales. The PRM worktree remains clean and its head is an ancestor
  of `origin/dev`; MAP/ESP remained clean at validation and were not modified.
- Additive payment-owned docs: payment plan status/link, `ROADMAP.md` and
  `PROGRESS.md`.
- No backend code, migration, SQL, OpenAPI, config, CI, Compose/Caddy, Go module
  or subscription source was changed. No provider/runtime test is claimed.

## Validation performed

Commands run from the payment worktree:

```sh
python3 <offer-policy-v1.json recomputation/validation>   # period/access/refund/
                                                          # dispute/excess/switch/sale
git diff --check
python3 <added/changed Markdown link check>
bash scripts/scan-secrets.sh
```

Results:

- Golden fixture PASS: 26 unique cases. Period arithmetic was independently
  recomputed with Python `zoneinfo` (`America/Sao_Paulo`), including 2026 month-end,
  2028 leap year, early renewal from current end, late renewal anchor reset and
  zero-grace boundaries. Refund/dispute/excess/switch/sale expectations matched.
- The fixture asserts: version 1, `SYNTHETIC_TEST_ONLY`, null production amount,
  synthetic amount `1234`, all channel flags false, zero grace, one future period
  and provider-owned automatic scheduler.
- `git diff --check` PASS.
- Added/changed Markdown link check PASS.
- `bash scripts/scan-secrets.sh` PASS.
- No production price, provider key, customer data, charge URL or QR payload was
  added.

## Remaining obligations

- Owner must approve the real Premium price/term/trial decision. Until then
  `premium.monthly` cannot be sold.
- Enterprise needs an implemented deliverable, source-rights decision, demand
  evidence, price/term and delivery model; no tenant/team/station capability is
  authorized.
- Tax/fiscal issuer, retention, consent wording, notifications and provider
  merchant/catalog/webhook setup remain unresolved.
- Pix Automático merchant eligibility and the actual provider cycle contract must
  be admitted at PAY-02/21; unavailable proof keeps the channel disabled.
- PAY-02 must revalidate provider contracts/SDKs and dependency licenses before
  adding any dependency.

Next action: PAY-02 only after maintainer authorization.

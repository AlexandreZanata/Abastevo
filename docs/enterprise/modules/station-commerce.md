# Station commerce and fleet agreements

Status: **PLANNED / RECONSTRUCTED**, 2026-11-11.
Recovered from the surviving enterprise requirements, shared rules and backlog.
The accepted F0 decisions and existing release gates govern any implementation.

## 1. Purpose

Let a currently authorized station representative publish bounded commercial information and transact with a fleet under an explicit contract.

## 2. Scope

F7 follows the separate Stage D override, representation decision and staffing evidence. It adds commercial profile, announced prices, labelled promotions, replies and agreements. It cannot overwrite official records, moderate its own criticism or change trust.

## 3. Entities and ownership

The organization owns profile revisions, announced price and promotion under one station authority. An agreement names two explicit parties. A voucher belongs to its transaction scope. Each side owns its internal accounting record and sees only the agreed field allowlist.

## 4. Business rules

- **BR-STN-001 — Separate commercial source.** An announced price has product, unit, author, conditions and validity and never enters official or community consensus (BR-SRC-001).
- **BR-STN-002 — Neutrality.** Payment and promotions do not affect organic ranking, moderation, trust or confidence (BR-SRC-002).
- **BR-AGREE-001 — Narrow bilateral sharing.** An agreement grants only named transaction fields to named parties. It grants no fleet, employee, document or location browsing (EDR-023).
- **BR-AGREE-002 — Once-only redemption.** An eligible voucher is consumed online once under current authority and its validity and limits.

## 5. Use cases

**UC-STN-01 — Announce a price.** A current representative selects an authorized station, product and conditions and submits a revision. The server validates authority and validity. A superseded or withdrawn announcement retains history.

**UC-AGREE-01 — Establish terms.** Eligible representatives propose and explicitly accept an agreement. Suspension or ending stops new eligible transactions under its recorded policy. Neither party expands the allowlist unilaterally.

**UC-AGREE-02 — Redeem.** A permitted station actor submits the valid transaction and voucher. Current server checks and one transaction record the result. Identical retry returns the result. Competing use cannot consume it twice. Typed ports create only the permitted internal fueling and receivable effects.

## 6. States and transitions

Announced prices are active, expired, withdrawn or superseded. Promotions have scheduled, active, ended or withdrawn state. Agreements are proposed, active, suspended, ended, declined or expired. Vouchers are issued, redeemed, expired, cancelled or locked. [The state catalog](../08-state-model.md) defines names. Each transition checks current station authority and party scope and retains audit history. Commercial expiry never rewrites representation history.

## 7. Roles and permissions

[Station-scoped permissions](../07-permissions.md) require live station authority. Fleet and station views use separate party scopes. General agreement visibility does not permit employee, track or original document reads. Catalog corrections use Directory governance. Official replies use existing feedback moderation rules.

## 8. Offline, concurrency, idempotency and corrections

Profile management, agreements, pre-authorization and voucher redemption are online-only. Server time decides validity. Redemption locks its source authority and limits and persists one acknowledgement with the accepted effects. Corrections use reviewed revisions and preserve each party's history.

## 9. Events, notifications and integrations

Publication uses profile revision ports. Redemption invokes typed fueling and payable interfaces with minimal fields. Interaction metrics require admitted collection and thresholds. Notification payloads remain bounded. Fiscal invoicing and provider settlement require their separate reviewed integrations.

## 10. Data, formulas and examples

Negotiated prices retain product, quantity unit, payment and offer conditions, effective interval and source version. Integer centavos and milli-units follow BR-MONEY. A statement reconciles existing transaction records and never creates another expense. Comparison cannot silently substitute announced or negotiated values for official or community data.

## 11. Screens

| ID | Surface | Objective | Actor | Data and actions | Validation |
|---|---|---|---|---|---|
| SCR-STN-01 | PORTAL | Manage authorized station commerce | Current representative | Profile, separate price source and promotions | Authority, validity, labels and five shared states |
| SCR-AGREE-01 | PANEL, PORTAL | Review own contract side | Permitted party | Allowed terms and transaction facts | No general cross-party access, explicit acceptance |
| SCR-AGREE-02 | PORTAL | Redeem an eligible voucher | Authorized station actor | Safe voucher metadata and server result | Online requirement, bounded failures and accessible outcomes |

## 12. Acceptance, dependencies, risks, phase and microtasks

[EP15](../backlog/ep15-station-commerce.md) supplies tasks and dependencies. VAL-33 and VAL-34 prove neutrality and transaction-only sharing. Test revoked authority, expiry, competing redemption, field allowlists on API/jobs/exports and duplicate cost recognition before any activation.

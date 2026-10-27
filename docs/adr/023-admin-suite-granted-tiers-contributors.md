# ADR-023 — Administrative suite, granted tiers and project contributors

Status: PROPOSED (2026-10-27). Becomes ACCEPTED only when the user approves the
planning PR that introduces it. Owner plan:
[ADMIN_CONTRIBUTOR_PLAN](../planning/ADMIN_CONTRIBUTOR_PLAN.md). Rules:
[ADMIN_TIERS_CONTRACT](../product/ADMIN_TIERS_CONTRACT.md).

## Context

Premium capabilities exist today only through verified Play proof or verified
web payment grants (`payment_grants`), and all sales are disabled
([offer policy](../product/STRIPE_ASAAS_OFFER_POLICY.md)). During the test
period, and later for production test accounts, the owner must be able to give
Premium-equivalent access without a payment. The project also wants to credit
validated repository contributors publicly (app and landing page), and to
assign predefined staff responsibilities to accounts.

Until now there has been no public administrative path: moderation and repair
run through `cmd/ops` only (BUC-006). This ADR introduces a narrow, separately
authenticated administrative HTTP surface and explicitly keeps BUC-006 true for
every existing user-facing route.

## Decision

1. **Single owner key.** One administrative key, generated offline by
   `ops admin keygen`, never committed, never stored raw on the server. The API
   holds only its SHA-256 hash (`ANPFUEL_ADMIN_KEY_SHA256`, optional `_NEXT` for
   rotation). No hash configured means no admin routes (404).
2. **Admin session, not the key, authorizes requests.** Login exchanges the key
   for a short-lived opaque session (HttpOnly `__Host-` cookie + CSRF token,
   idle 30 min, absolute 8 h). Every login attempt and every mutation is written
   to an append-only audit table.
3. **Granted tiers are a third, independent entitlement source.** A new
   `entitlement_grants` owner is merged into the existing capability projection
   beside Play and web grants (`domain.Evaluate` / `RequireCapability`).
   Capabilities are derived from a code-frozen tier catalog; neither admin UI nor
   app client ever sends capability strings.
4. **Tier catalog v1:** `PRO` (the existing Premium bundle; "Pro" is the
   product name of Premium), `CONTRIBUTOR` (Premium bundle +
   `contributor.profile`), `ENTERPRISE` (Premium bundle; no organization, team,
   seat, station or cross-account right). Paid Enterprise sale stays disabled.
5. **Contributors** are accounts holding an active `CONTRIBUTOR` grant. A
   repository contributor is someone present in the GitHub contributors list of
   the configured repository. Account ↔ GitHub login linking is approved by the
   admin, optionally backed by a public-gist ownership proof.
6. **Staff roles are a code-frozen catalog.** The admin only assigns/revokes.
   A role grants no power until a specific consumer endpoint enforces it with
   its own tests. Roles never confer admin-key powers.
7. **Neutrality.** No tier, grant, contributor status or role may change trust,
   moderation outcome, confidence tier, ranking, feedback weight or placement
   in price results (OPEN_SOURCE_BUSINESS).
8. **Admin web UI** is a static, framework-free site (`admin-web/`) reusing the
   landing design tokens and fonts, served by Caddy under `/admin/` on the API
   origin so cookies and `connect-src 'self'` work without CORS.

## Consequences

- New modules: `admin`, `entitlement`, `contributor`, `staff`; new migrations
  in the reserved range (see plan). `subscription` reads `entitlement_grants`
  inside the caller transaction exactly like `payment_grants`.
- Capability checks must work when payment ports are not configured (test
  period). Composition in `cmd/api` must not hide admin grants behind payment
  configuration.
- `/v1/premium/status` gains an additive `tiers` field; existing fields keep
  their meaning.
- Public `GET /v1/contributors` is the only new anonymous read; it exposes
  consented display fields only and allows CORS GET from configured landing
  origins.
- Real-production use remains governed by G09/G24; this ADR authorizes
  construction and staging validation only.

## Rejected alternatives

- Inserting fake `payment_grants` rows: they require orders/settlements and
  would corrupt payment reconciliation.
- Runtime-editable roles/permissions: unbounded authorization surface.
- Admin as a flag on a normal user account: a stolen user session would become
  an admin session.
- Raw key in environment: a leaked env dump would leak the key; storing only the
  hash keeps the key in the owner's password manager.

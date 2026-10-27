# A0 — Admin suite backend recon

Date: 2026-10-27. Base: `origin/main` at `44b7b915`. Worktree
`.worktrees/adm-backend`. Rules: `docs/product/ADMIN_TIERS_CONTRACT.md`
(B-BR-ADM/ENT), ADR-023 and `docs/planning/ADMIN_CONTRIBUTOR_PLAN.md`.
The planning documents are introduced by PR #161 and are not yet on `main`;
this recon reads them from the `codex/admin-contributor-plan` worktree.

## Migration numbers

`origin/main` ends at `000132_charging_operational_commands.sql`. Open PRs
#152 (000133–000138) and #156 (000139) stay outside the reserved range.
Next free numbers inside `000140`–`000149`:

- A1: `000140_admin_sessions_audit.sql`
- A2: `000141_entitlement_grants.sql`

Every migration is append-only and gets a recovery/idempotency integration
test through the existing `migrate.Apply` chain (all migrations from empty).

## Plug-in points

### Configuration

- `backend/internal/platform/config/config.go`: `Config`, `load`, `LogValue`.
  New fields (never logged): `AdminKeyHashes`, `AdminOrigin`,
  `AdminAllowedCIDRs`. Malformed hashes/origin/CIDRs fail startup naming the
  variable and never echoing the value. Admin routes stay unmounted when no
  hash is configured (ADM-01).
- `infra/env.staging.example` and `infra/env.prod.example`: commented
  placeholders only.

### HTTP composition

- `backend/cmd/api/main.go`: `run` builds the chi router, the pgx pool, the
  account session validator and the trusted-proxy resolver used by identity.
  Admin composition is a separate `cmd/api/admin.go` function called after the
  premium wiring. It receives the pool, validated admin config, the existing
  trusted-proxy CIDRs, the request-ID source and the clock.
- Client IP for ADM-04/ADM-08 reuses the existing trusted-proxy logic
  (`identity/adapters/http.Resolver.ClientIP`) through a narrow closure; the
  admin module does not import identity adapters and never persists raw IPs.
- Admin route set is mounted only when at least one valid key hash exists.
  When unset, chi's default 404 answers every `/v1/admin/*` path (S1).
- Admin responses get `Cache-Control: no-store`, `X-Robots-Tag: noindex,
  nofollow`, `Referrer-Policy: no-referrer`, a 16 KiB body cap and the
  optional CIDR fence (ADM-08/ADM-09).

### Module

`backend/internal/modules/admin/`:

- `domain/`: key format/hash/fingerprint, constant-time matching, reason
  validation, session token hashing and idle/absolute expiry. Pure; no I/O.
- `application/`: `Login`, `Authenticate`, `Logout`, `RevokeAll` and the
  audit/idempotency `Mutation` port. Clock and random source injected.
- `adapters/`: pg store joining the caller transaction for sessions, login
  attempt counters, idempotency and append-only audit; HTTP adapter for
  cookie, CSRF, Origin, headers, body limit and errors.
- SQL lives in `backend/db/queries/admin/` with its own `sqlc.yaml` stanza
  (`000001`, `000020`, `000140`); `sqlc generate` output is committed.

### CLI

`backend/cmd/ops/main.go` gains `ops admin keygen`. It writes only to stdout
(key plus SHA-256), uses `crypto/rand`, and never touches the filesystem or
config (ADM-02).

### Granted tiers (A2)

- `backend/internal/modules/entitlement/domain/catalog.go`: frozen
  `tier-catalog-v1` (`PRO`, `CONTRIBUTOR`, `ENTERPRISE`) and capability
  derivation. Golden JSON: `contracts/admin/tier-catalog-v1.json`.
- `backend/internal/modules/entitlement/adapters`: `entitlement_grants`
  store through `backend/db/queries/entitlement/`.
- Subscription projection, minimal diff:
  - `subscription/domain/policy.go`: `Evaluate` merges play, web and admin
    sources; `Status` gains `tiers`; reason `verified-admin` for admin-only.
  - `subscription/adapters/web_grants.go`: pattern for `adminGrants` reading
    the grant table inside the caller transaction with `42P01` tolerance.
  - `subscription/adapters/capability.go`: `RequireCapability` also checks
    admin capabilities.
  - `subscription/adapters/store.go`: `Status` loads web and admin grants in
    the same owner transaction.
  - `subscription/application/service.go`: the current `payments-disabled`
    wipe only applies when no web/admin capability is active (ENT-10).

### Payments-not-configured composition (ENT-10)

`backend/cmd/api/main.go` currently short-circuits every capability check:

```go
if !premiumService.Configured() { return ... denied ... }
```

before `premiumStore.RequireCapability` for `cloud.sync`,
`vehicles.multiple`, `professional.write` (ledger and alerts). Those gates
must be removed so the projection is always evaluated: Play rows simply do
not exist while payments are unconfigured, web grants tolerate a missing
table (`42P01`), and admin grants are local rows. The A2 integration test
proves each `PRO` capability with only an admin grant and
`premiumService.Configured() == false`.

`/v1/premium/status` must keep `payments_configured=false` for an
admin-only account (no payment source), while returning `state=ACTIVE`,
`reason=verified-admin`, the `PRO` capabilities and the additive `tiers`
entry with `source=admin`.

### Account search/detail and erasure

- Accounts live in `accounts` (`id`, `alias`, `status`) and address lookups
  in `account_addresses.address_hash`; the hash comes from
  `account/domain.AddressHash`. Admin search is read-only and parameterized,
  max 20 results: exact UUID, exact alias, alias prefix (≥3 chars), exact
  email (hashed server-side). No `LIKE` wildcards from user input.
- ENT-11 erasure: the existing account deletion transaction is
  `account/adapters/pgstore.go` `DeleteAccount`, which updates
  `accounts.status='deleted'`. Existing modules hook that event with
  `AFTER UPDATE OF status` triggers (`000075_subscription_ownership.sql`,
  `000078_cloud_records.sql`). A2 follows the same pattern in
  `000141_entitlement_grants.sql`, revoking active grants with
  `revoke_reason='account-erased'` inside the same transaction.

### API contract

`contracts/openapi/v1.yaml` receives the admin session routes in A1
(`POST/GET/DELETE /v1/admin/session`,
`POST /v1/admin/sessions/revoke-all`) and the A2 account/grant routes.
`go test ./internal/platform/apicontract` validates the document.

## Risks / notes

- `admin_sessions` stores a deterministic lookup digest plus a salted hash so
  the cookie can be resolved without scanning; raw tokens never persist.
- Login rate-limit counters are serialized through a global row lock, so
  concurrent failures cannot lose updates; only real key mismatches count.
- Logout/revoke-all are mutations under ADM-12 (reason + idempotency) and are
  covered by the same transactional audit helper used by later grant
  mutations.

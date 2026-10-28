# Admin suite, granted tiers, contributors and staff roles — contract v1

Status: PROPOSED with [ADR-023](../adr/023-admin-suite-granted-tiers-contributors.md).
Plan: [ADMIN_CONTRIBUTOR_PLAN](../planning/ADMIN_CONTRIBUTOR_PLAN.md).
Rule IDs: `B-BR-ADM-*` (admin auth), `B-BR-ENT-*` (granted tiers),
`B-BR-CTB-*` (contributors), `B-BR-STF-*` (staff roles), use cases `BUC-ADM-*`.
Every rule below needs at least one test that cites its ID in the test name or
comment. A rule without a passing test is not implemented.

## 1. Vocabulary

| Term | Meaning |
|---|---|
| Admin | The project owner holding the single administrative key. Not an app account. |
| Admin key | `abv_admin_` + 43 base64url chars (32 random bytes). Lives only in the owner's password manager. |
| Admin session | Opaque server session obtained by presenting the key once. |
| Tier | Named bundle of capabilities from the frozen catalog (§3). |
| Grant | Admin-created, time-bounded or open-ended assignment of one tier to one account. |
| Repository contributor | GitHub login listed by the GitHub contributors API for `ANPFUEL_GITHUB_REPO`. |
| Contributor | Account with an `ACTIVE`, unexpired `CONTRIBUTOR` grant. |
| Staff role | Code-defined responsibility (§5) assigned by the admin to an account. |

## 2. Admin authentication (B-BR-ADM)

- **ADM-01** Configuration: `ANPFUEL_ADMIN_KEY_SHA256` = lowercase hex SHA-256
  of the full key string (64 chars). Optional `ANPFUEL_ADMIN_KEY_SHA256_NEXT`
  for rotation. A malformed value fails API startup. When neither is set, no
  `/v1/admin/*` route is registered (404 for every admin path).
- **ADM-02** The raw key never exists on the server: not in env, files, DB,
  logs, metrics, errors or audit. `ops admin keygen` prints the key and its hash
  once to stdout and writes nothing to disk.
- **ADM-03** Login is `POST /v1/admin/session` with JSON `{"key": "..."}` only.
  Keys in query string, path, cookies or headers are ignored and the request is
  `400`. Comparison: SHA-256 of the presented key against each configured hash
  with `crypto/subtle.ConstantTimeCompare`. Any failure is `401 admin.denied`
  with an identical body and no hint (wrong format, wrong key, disabled key).
- **ADM-04** Brute-force limits: per client IP (resolved with the existing
  trusted-proxy logic) at most 5 failed logins per 15 minutes → `429` with
  `Retry-After`; globally at most 50 failed logins per hour → all logins `429`
  for 15 minutes. Successful login does not reset the global counter. Limits are
  stored in PostgreSQL (survive restart) and are concurrency-safe.
- **ADM-05** Session: 32 random bytes, base64url, returned only as cookie
  `__Host-abv_admin` (`HttpOnly; Secure; SameSite=Strict; Path=/`). Stored as a
  salted hash in `admin_sessions` with `key_fingerprint` (first 12 hex of the
  configured hash that matched), `created_at`, `last_seen_at`, `expires_at`,
  `revoked_at`. Idle timeout 30 min, absolute lifetime 8 h. Login always issues
  a new session (no fixation).
- **ADM-06** CSRF: login also returns `{"csrf_token": "..."}` (random, stored
  hashed per session). Every non-GET admin request must carry header
  `X-Admin-CSRF` matching it **and** an `Origin` header exactly equal to
  `ANPFUEL_ADMIN_ORIGIN` (e.g. `https://api.example.org`). Missing/mismatch →
  `403 admin.csrf`. `GET` admin endpoints have no side effects.
- **ADM-07** Rotation: a session is valid only while its `key_fingerprint`
  matches a currently configured hash. Removing a hash invalidates its sessions
  on the next request. `POST /v1/admin/sessions/revoke-all` revokes every session.
- **ADM-08** Optional network fence `ANPFUEL_ADMIN_ALLOWED_CIDRS`: when set,
  admin requests from other client IPs get `404`.
- **ADM-09** Every admin response: `Cache-Control: no-store`,
  `X-Robots-Tag: noindex, nofollow`, `Referrer-Policy: no-referrer`. Admin
  request bodies are limited to 16 KiB and decoded with unknown fields rejected.
- **ADM-10** Audit: table `admin_audit_events` is append-only (a trigger raises
  on `UPDATE`/`DELETE`). Rows: id, at, session_id (nullable for failed login),
  action, target_type, target_id, reason, before (jsonb), after (jsonb),
  request_id, client_ip_hash. Failed logins are audited without any key material.
  Every successful mutation writes its audit row **in the same transaction**.
- **ADM-11** An account bearer/session token never authenticates an admin
  request, and an admin session never authenticates a user request.
- **ADM-12** Every mutation needs `reason` (10–500 Unicode chars after trim) and
  header `Idempotency-Key` (UUID). Same key + same body replays the stored
  result; same key + different body → `409`.

## 3. Tier catalog and granted tiers (B-BR-ENT)

Catalog `tier-catalog-v1` (Go constant in `entitlement/domain`, golden JSON in
`contracts/admin/tier-catalog-v1.json`):

| Tier | Capabilities | Notes |
|---|---|---|
| `PRO` | `cloud.sync`, `cloud.backup`, `cloud.restore`, `alerts.advanced`, `professional.write`, `professional.report`, `vehicles.multiple` | Same bundle as paid Premium. |
| `CONTRIBUTOR` | PRO bundle + `contributor.profile` | Public credit (§4). |
| `ENTERPRISE` | PRO bundle | Label only in v1. No org/team/seat/station/cross-account right. |

`FREE` is the absence of any capability; it is never stored.

- **ENT-01** Only an admin session can create or revoke a grant. No user,
  role holder, payment or webhook path writes `entitlement_grants`.
- **ENT-02** Target account must exist with status `active`; `suspended` or
  `deleted` → `409 entitlement.account-ineligible`.
- **ENT-03** At most one `ACTIVE` grant per (account, tier) — partial unique
  index. A second grant → `409 entitlement.exists` unless the request has
  `"replace": true`, which revokes the old grant and inserts the new one in one
  transaction (two audit rows).
- **ENT-04** `end_at` is optional. When present: `now + 1h ≤ end_at ≤ now + 5
  years`. `null` means until revoked. `start_at` is always the server time.
- **ENT-05** Capabilities are derived at evaluation time from (`tier`,
  `policy_version`) via the catalog. The table stores no capability array, and
  request bodies containing `capabilities` are rejected (unknown field).
- **ENT-06** A grant counts when `state='ACTIVE' AND start_at <= now AND
  (end_at IS NULL OR end_at > now)`. Expiry needs no job.
- **ENT-07** Revoke sets `state='REVOKED'`, `revoked_at`, `revoke_reason`.
  Revoked grants are never reactivated; a new grant is a new row. A capability
  check that starts after the revoke commits must deny.
- **ENT-08** Capability projection = union of Play, web payment and admin grant
  capabilities. Admin grants never change payment rows, and payments/refunds
  never change admin grants.
- **ENT-09** `/v1/premium/status` adds `tiers`: array of
  `{"tier","source","valid_until"}` where `source ∈ {play, web, admin}`; paid
  Premium reports tier `PRO`. Existing fields keep their meaning; when only an
  admin grant is active, `state="ACTIVE"` and `reason="verified-admin"`.
- **ENT-10** Admin grants must work when payment ports are not configured.
- **ENT-11** Account erasure revokes all active grants with reason
  `account-erased` inside the erasure flow.
- **ENT-12** Grants never alter trust, moderation, confidence tier, ranking,
  feedback weight or price-result placement (OPEN_SOURCE_BUSINESS).

## 4. Contributors (B-BR-CTB)

- **CTB-01** "Validated repository contributor" = login present in the
  server-side snapshot of `GET https://api.github.com/repos/{ANPFUEL_GITHUB_REPO}/contributors`
  (all pages, `per_page=100`). Snapshot refresh: at most once per minute on
  admin request, cached for 1 hour, 5 s timeout, 1 MiB response cap, optional
  read-only `ANPFUEL_GITHUB_TOKEN`. No new Go dependency (`net/http`).
- **CTB-02** Two ways to make a contributor: (a) admin grants `CONTRIBUTOR`
  directly to any active account (arbitrary, audited); (b) the user opens a
  claim with a GitHub login and the admin approves it.
- **CTB-03** Claim: signed-in user submits `github_login` (GitHub rules:
  1–39 chars, `[A-Za-z0-9-]`, no leading/trailing/double `-`). Server returns a
  one-time code `abastevo-contributor-` + 16 base32 chars. Max 1 open claim per
  account, max 3 claims per account per 24 h. Claims expire after 30 days.
- **CTB-04** Optional ownership proof: the user creates a **public gist** on
  that GitHub account containing the code and submits the gist ID. The server
  fetches `GET https://api.github.com/gists/{id}` and marks the proof `VERIFIED`
  only if `owner.login` equals the claimed login (case-insensitive) and any file
  content contains the exact code. Otherwise `UNVERIFIED`.
- **CTB-05** Admin approval of a claim creates, in one transaction, the
  `CONTRIBUTOR` grant (no end date by default) and the GitHub link. Approving an
  `UNVERIFIED` claim or a login absent from the snapshot requires
  `"override": true` plus reason; the audit row records the override.
- **CTB-06** One GitHub login ↔ at most one account, and one account ↔ at most
  one login, among active links (unique partial indexes). Conflicts → `409`.
- **CTB-07** Claim states: `PENDING → APPROVED | REJECTED | WITHDRAWN |
  EXPIRED`. Terminal states are final.
- **CTB-08** Public profile, editable only while the account holds
  `contributor.profile`:
  - `display_name`: NFC-normalized, trimmed, 2–40 runes; letters, marks,
    digits, space, `.`, `'`, `-` only; no `@`, `/`, `:` or control characters.
  - `links`: 0–2 items of `{platform, handle}`; one link per platform.
    Allowlist and handle rules:

    | Platform | Handle rule | Server-built URL |
    |---|---|---|
    | `github` | GitHub login rule | `https://github.com/{h}` |
    | `gitlab` | `[A-Za-z0-9_.-]{2,255}` | `https://gitlab.com/{h}` |
    | `linkedin` | `[A-Za-z0-9-]{3,100}` | `https://www.linkedin.com/in/{h}` |
    | `x` | `[A-Za-z0-9_]{1,15}` | `https://x.com/{h}` |
    | `instagram` | `[A-Za-z0-9_.]{1,30}` | `https://www.instagram.com/{h}` |
    | `youtube` | `[A-Za-z0-9_.-]{3,30}` | `https://www.youtube.com/@{h}` |
    | `bluesky` | DNS-like `[a-z0-9.-]{3,253}` with a dot | `https://bsky.app/profile/{h}` |

    Clients send only platform + handle; any URL or scheme in a handle is
    rejected. Clients must render only the server-built `url` and re-check that
    its host is in the allowlist before opening it.
  - `visibility`: `PUBLIC | HIDDEN`; default `HIDDEN` until the user saves with
    `public_consent: true`.
- **CTB-09** A profile appears in `GET /v1/contributors` only when: active
  `CONTRIBUTOR` grant + `visibility=PUBLIC` + not admin-hidden + account active.
  Revoking the grant removes it on the next read (cache ≤ 5 min).
- **CTB-10** Public item fields only: `display_name`, `links[] {platform,
  handle, url}`, `github_login` (only when linked; shown as "verified GitHub
  contributor"), `contributor_since` (month `YYYY-MM`), `roles[]` (public role
  labels, §5). Never `account_id`, alias, email, grant ids or reasons.
  Order: `contributor_since` ascending, then an opaque stable id; cursor
  pagination, max 50 per page. `Cache-Control: public, max-age=300`.
- **CTB-11** CORS: only `GET /v1/contributors`, only for origins listed in
  `ANPFUEL_PUBLIC_WEB_ORIGINS`; no credentials; no other route gains CORS.
- **CTB-12** Profile edits: max 10 per account per 24 h. Admin can `hide`
  (reason) and `unhide`; hidden profiles keep the data but are not listed.
- **CTB-13** Account erasure deletes the profile, links and claim codes and
  revokes the GitHub link; the public list stops showing it immediately.
- **CTB-14** Contributor status never alters trust, moderation, ranking or
  feedback weight (same as ENT-12).

## 5. Staff roles (B-BR-STF)

Catalog `staff-roles-v2` (Go constant in `staff/domain`, golden JSON
`contracts/admin/staff-roles-v2.json`; v2 adds `project_creator`, migration
000145, owner request 2026-10-28; `staff-roles-v1.json` stays as history):

| Role | Public label (pt-BR) | Intended responsibility (future consumer) |
|---|---|---|
| `project_creator` | Criador do projeto | Public badge for the project's author; no responsibility or permission |
| `community_moderator` | Moderação da comunidade | Review reports and apply moderation actions |
| `station_reviewer` | Revisão de postos | Review station suggestions and profile claims |
| `contributor_curator` | Curadoria de contribuidores | Pre-review contributor claims |
| `support_agent` | Suporte | Read-only account status lookup for support |

- **STF-01** Roles exist only in code. No endpoint creates, renames or edits a
  role or its permissions.
- **STF-02** Only an admin session assigns/revokes. Target account must be
  active. One `ACTIVE` assignment per (account, role); optional `end_at` with
  the same bounds as ENT-04; same reason/idempotency/audit rules.
- **STF-03** In v1 a role grants **no permission**. Each future consumer
  endpoint is its own task with its own authorization and negative tests.
- **STF-04** Roles never confer admin powers and never alter trust, moderation
  outcome or ranking by themselves.
- **STF-05** The owner sees their own active roles in an authenticated read;
  public role labels appear in the contributors list only when the account is
  a public contributor and `show_roles_publicly` is true (default false).
- **STF-06** Account erasure revokes all active assignments.

## 6. Use cases (BUC-ADM)

| ID | Actor | Flow | Key failures |
|---|---|---|---|
| BUC-ADM-01 | Admin | Logs in with key, gets session, logs out | wrong key 401, rate limit 429, disabled 404 |
| BUC-ADM-02 | Admin | Finds an account by UUID, exact alias, alias prefix (≥3 chars) or exact email (hashed server-side) | short query 400, no match empty list |
| BUC-ADM-03 | Admin | Grants PRO/ENTERPRISE to a test user for N days | ineligible account 409, duplicate 409, bad end_at 400 |
| BUC-ADM-04 | Admin | Revokes a grant | already revoked 409 |
| BUC-ADM-05 | Admin | Views GitHub contributors with link/claim state | GitHub down → cached snapshot + `stale=true` |
| BUC-ADM-06 | User | Opens contributor claim, adds gist proof | invalid login 400, open claim 409, quota 429 |
| BUC-ADM-07 | Admin | Approves/rejects claim, or links directly | unverified without override 409, link conflict 409 |
| BUC-ADM-08 | Contributor | Edits display name + up to 2 links, publishes | no capability 403, invalid link 400, quota 429 |
| BUC-ADM-09 | Anyone | Lists contributors in app / landing | none (empty list is valid) |
| BUC-ADM-10 | Admin | Assigns/revokes staff role | unknown role 400, duplicate 409 |
| BUC-ADM-11 | Admin | Hides/unhides a contributor profile | not found 404 |
| BUC-ADM-12 | Admin | Reads audit log with filters | — |

## 7. HTTP surface

Admin (cookie session + CSRF on mutations, all `no-store`):

| Method & path | Purpose |
|---|---|
| `POST /v1/admin/session` | Login `{key}` → cookie + `{csrf_token, expires_at}` |
| `GET /v1/admin/session` | Who-am-I: `{key_fingerprint, expires_at}` |
| `DELETE /v1/admin/session` | Logout |
| `POST /v1/admin/sessions/revoke-all` | Revoke all admin sessions |
| `GET /v1/admin/accounts?q=` | Search (max 20) |
| `GET /v1/admin/accounts/{id}` | Detail: alias, status, created_at, grants, paid summary (state only), roles, contributor link/profile/claims |
| `POST /v1/admin/accounts/{id}/grants` | `{tier, end_at?, reason, replace?}` |
| `GET /v1/admin/grants?tier=&state=&cursor=` | List grants |
| `POST /v1/admin/grants/{id}/revoke` | `{reason}` |
| `GET /v1/admin/github/contributors?refresh=` | Snapshot + link/claim state |
| `GET /v1/admin/contributor-claims?state=&cursor=` | Claims queue |
| `POST /v1/admin/contributor-claims/{id}/approve` | `{reason, override?}` |
| `POST /v1/admin/contributor-claims/{id}/reject` | `{reason}` |
| `POST /v1/admin/accounts/{id}/github-link` | Direct link `{login, reason, override?}` |
| `POST /v1/admin/contributor-profiles/{account_id}/hide` · `/unhide` | `{reason}` |
| `GET /v1/admin/roles` | Role catalog |
| `POST /v1/admin/accounts/{id}/roles` | `{role, end_at?, reason}` |
| `POST /v1/admin/role-assignments/{id}/revoke` | `{reason}` |
| `GET /v1/admin/audit?action=&target_id=&cursor=` | Audit read |

User (same proof pattern as `/v1/premium/status`: `family_id` +
`access_token` body and signed owner):

| Method & path | Purpose |
|---|---|
| `POST /v1/premium/status` | Additive `tiers` (ENT-09) |
| `POST /v1/contributor/claims` | Open claim `{github_login, operation_id}` → `{claim_id, code}` |
| `POST /v1/contributor/claims/read` | Own claims |
| `POST /v1/contributor/claims/{id}/proof` | `{gist_id}` |
| `POST /v1/contributor/claims/{id}/withdraw` | — |
| `POST /v1/contributor/profile/read` | Own profile + roles |
| `POST /v1/contributor/profile` | Upsert `{display_name, links, visibility, public_consent, show_roles_publicly}` |

Public: `GET /v1/contributors?cursor=` (CTB-10/11).

All routes must be added to `contracts/openapi/v1.yaml` in the same PR that
implements them (the `apicontract` test enforces it). Error codes follow the
existing `httpapi.WriteError` style: `admin.*`, `entitlement.*`,
`contributor.*`, `staff.*`.

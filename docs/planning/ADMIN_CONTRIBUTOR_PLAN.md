# Admin suite, granted tiers and contributors — execution plan

Status: PLANNED (2026-10-27). Nothing here is implemented until its PR is
merged with passing tests. Decision: [ADR-023](../adr/023-admin-suite-granted-tiers-contributors.md).
Rules (the source of truth for behavior): [ADMIN_TIERS_CONTRACT](../product/ADMIN_TIERS_CONTRACT.md).
Kickoff prompts: [ADMIN_CONTRIBUTOR_PROMPTS](ADMIN_CONTRIBUTOR_PROMPTS.md).

This plan is written to be executed by a less capable agent. Follow it
literally. When something is not covered, stop and ask the user instead of
inventing a rule.

## 0. Global rules for every front

1. **Isolation.** Each front works in its own git worktree and its own
   branches, created from `origin/main`. Never edit the main checkout at
   `/home/iiii/PESSOAL-PROJETOS-ALEXANDRE/brazil-fuel-prices` (it holds another
   person's uncommitted work). Never use `git stash`, `reset --hard`,
   `push --force`, `--no-verify` or admin merge bypass.
2. **One PR per feature.** Each task block below (`A1`, `A2`, …) is one branch
   and one PR to `main`. The user authorized feature branches from `origin/main`
   with PRs to `main` for this batch, because `dev` carries the unrelated open
   PR #154. If the previous PR of the same front is not merged yet, branch from
   the previous branch and open the PR with `--base <previous-branch>`; say so in
   the PR body. **Never merge.** The user merges after review.
3. **Do not touch other open PRs.** Open PRs at planning time: #152 (driver
   suite, touches `ProfileScreen.kt` and migrations 000133–000138), #154 (dev
   map), #155 (staging landing, `infra/caddy/staging-web.Caddyfile`), #156
   (migration 000139), #157, #160. Avoid editing their files; when unavoidable
   (only `ProfileScreen.kt`, `landing/static/_headers`), keep the diff minimal.
4. **Migrations.** This batch reserves `000140`–`000149`. Before creating one,
   run `git fetch origin && git ls-tree --name-only origin/main backend/db/migrations/`
   and check open PR branches; if a number is taken, take the next free number
   inside the reserved range. Applied migrations are append-only. Every
   migration gets a recovery/idempotency test like its neighbours.
5. **TDD.** Write the domain test from the contract rule first (RED), then the
   code (GREEN), then refactor. Test names or comments cite the rule ID, e.g.
   `TestGrant_ENT03_DuplicateActiveGrantConflicts`.
6. **Secrets.** Never commit a key, hash of a real key, token, cookie, email,
   real account id or screenshot of real data. Fixtures use synthetic values.
   Before every commit run `git diff --check` and grep the diff for
   `abv_admin_`, `ANPFUEL_ADMIN_KEY_SHA256=[0-9a-f]`, `ghp_`, `github_pat_`
   and e-mail addresses (`[A-Za-z0-9._%+-]+@[A-Za-z0-9.-]+\.[a-z]{2,}`).
7. **Checks per PR (backend).** From `backend/`: `gofmt -l .` (empty),
   `go vet ./...`, `go test -race -count=1 ./...`, and the PostGIS integration
   suite for touched modules
   (`go test -race -count=1 -tags=integration ./internal/modules/<m>/... ./cmd/api/...`
   with `ANPFUEL_TEST_DATABASE_URL` from `backend/README.md`), plus
   `go test ./internal/platform/apicontract -count=1`. `sqlc generate` output
   must be committed and unchanged on re-run. Run `make quick-verify` before
   opening the PR.
8. **Checks per PR (Android).** `./gradlew :app:testDebugUnitTest
   :application:test :data:test lintDebug assembleDebug` (adjust to the modules
   touched). No new lint errors. Strings for all 8 locales
   (`values`, `-pt-rBR`, `-es`, `-fr`, `-de`, `-ja`, `-ru`, `-zh-rCN`).
9. **Checks per PR (landing / admin-web).** `npm ci && npm run typecheck &&
   npm run build && npm run check && npm test` in the touched package.
10. **Commits.** English, Conventional Commits (`feat(admin): …`), atomic. PR body: scope,
    rules covered (IDs), tests run with results, what is not done.
11. **Progress.** Each PR adds one concise entry to the top of
    `docs/planning/PROGRESS.md` (task, branch, PR, tested revision, state
    `LOCAL_DONE / INTEGRATION_PENDING`, next action). Detail goes to
    `backend/evidence/admin/<TASK>.md`.
12. **Stop conditions.** Stop and report (do not improvise) when: a test you did
    not write fails on `origin/main`; a rule is ambiguous; a migration number
    conflict cannot be resolved inside the range; you would need a new
    dependency; you would need to change payment, trust or moderation behavior.
13. **No CI polling, no sleeping.** Push, open the PR, report, continue.

## 1. Fronts and order

```
Front A  Backend (first; everything depends on it)
  A0 recon ─ A1 admin auth+audit ─ A2 granted tiers ─ A3 contributors ─ A4 staff roles ─ A5 staging VPS
Front B  Admin web UI (starts after A1 PR exists; real use after A2)
  B0 recon ─ B1 scaffold+login+accounts/grants ─ B2 contributors/roles/audit ─ B3 Caddy+staging
Front C  Contributor UI (app + landing; starts after A3 PR exists)
  C0 recon ─ C1 app public list ─ C2 landing page ─ C3 app self-service (claim+profile) ─ C4 staging check
```

Worktrees (create with `git worktree add -b <branch> <path> origin/main`):

| Front | Worktree path | Branch prefix |
|---|---|---|
| A | `.worktrees/adm-backend` | `codex/adm-a<N>-<slug>` |
| B | `.worktrees/adm-web` | `codex/adm-b<N>-<slug>` |
| C | `.worktrees/adm-contributors-ui` | `codex/adm-c<N>-<slug>` |

## 1a. Consolidation and staging corrections (2026-10-27)

State: A1–A4 are delivered as stacked PRs #164 → #166 → #167 → #168, all
open; #164 conflicts with `main` (main moved to `d4dcf716`). Migrations
`000140`–`000143` do not collide with the map (`000139`) or driver
(`000133`–`000138`) reservations. Nothing of A1–A4 is on staging.

Staging facts that supersede the plan text above:

- Staging is single-node k3s in namespace `abastevo-temp`, not Docker
  Compose. `docs/operator/deploy.md` (Compose) does not describe it. The
  reviewed templates are `infra/k8s/staging/` (PR #169) and the deploy path is
  the CD agent (CD-03/CD-04). The API reads configuration from a Kubernetes
  Secret, not an env file.
- Staging currently runs the unmerged map branch (images `stg-7c7d776f`,
  schema `000139`). Deploying any single feature branch would drop the other
  branch's features, so admin code reaches staging only through `main` and the
  CD agent (or one supervised CD apply), after the map PRs are in `main`.

Decisions:

1. Front B starts now (rule above: after the A1 PR exists). It does not wait
   for A5. Local integration uses a backend built from the A4 head
   (`codex/adm-a4-staff-roles`, which contains A1–A3) on a disposable
   database; unit tests use a fake `fetch` against the OpenAPI contract.
2. A5 is split. A5a (code, now): `scripts/tests/test-admin-staging.sh` plus a
   run against the local A4 stack, as a normal PR stacked on #168. A5b
   (staging run): executes in the same staging window as B3, after A1–A4 and
   the map PRs are in `main` and deployed; its evidence goes in the B3 PR
   (`backend/evidence/admin/A5-STAGING.md`), not in a bookkeeping-only PR.
3. Keys: the user generates the staging admin key locally with
   `go run ./cmd/ops admin keygen`, keeps the raw key, and the hash goes into
   the staging Secret (`kubectl -n abastevo-temp` patch of the API Secret,
   run by the user or by an agent that only receives the hash). Rotation is
   the same command after the validation. No agent sees the raw key.
4. B3 serves `admin-web/dist` from the staging web tier (the Caddy ConfigMap
   and web Deployment templates in `infra/k8s/staging/`, a release directory
   switched by symlink like the landing), not from Compose files. Production
   wiring stays out of scope (G09).
5. Merge order: #161 (this plan) → #164 (after merging current `main` into
   it) → retarget #166, #167, #168 to `main` one by one as each base merges →
   A5a. Front B PRs target `main` and merge after #164.

## 2. Front A — Backend

### A0 — Recon (no PR)

Read: this plan, the contract, ADR-023, `backend/README.md`,
`docs/backend/TEST_STRATEGY.md`, and these files to copy patterns from:

- `backend/internal/modules/subscription/domain/policy.go` (`Evaluate`, `mergeWeb`)
- `backend/internal/modules/subscription/adapters/web_grants.go` and
  `capability.go` (`RequireCapability`)
- `backend/internal/modules/subscription/adapters/http/http.go` (user proof
  pattern: `family_id` + `access_token` + `SignedOwner`)
- `backend/cmd/api/main.go` (composition; `premiumStore`, payments wiring)
- `backend/cmd/ops/main.go` (CLI pattern)
- `backend/internal/platform/httpapi` (`WriteError`), `config`, `httpserver`
- `backend/internal/modules/account/domain/account.go` (`AddressHash`)
- An existing migration + its integration test, `backend/sqlc.yaml`.

Write `backend/evidence/admin/A0-RECON.md`: where each new piece plugs in,
the exact composition point for payments-not-configured (ENT-10), and the
migration numbers you will use. Include it in the A1 PR.

### A1 — Admin authentication, sessions, audit (PR 1)

Branch `codex/adm-a1-admin-auth`. Rules ADM-01…ADM-12.

1. Docs in this PR: ADR-023 status unchanged, contract unchanged unless a
   real gap is found (document it in the PR body).
2. Migration `000140_admin_sessions_audit.sql`: `admin_sessions`,
   `admin_login_attempts` (rate-limit counters by `ip_hash` + window, and a
   global row), `admin_idempotency`, `admin_audit_events` + trigger refusing
   `UPDATE`/`DELETE`.
3. Module `backend/internal/modules/admin/` with `domain/` (key format,
   fingerprint, reason validation, session timing — pure, no I/O),
   `application/` (Login, Authenticate, Logout, RevokeAll, Audit port),
   `adapters/` (pg store with sqlc queries in `backend/db/queries/admin/`),
   `adapters/http/` (routes, cookie, CSRF, Origin, headers, body limit).
4. Config: `ANPFUEL_ADMIN_KEY_SHA256`, `ANPFUEL_ADMIN_KEY_SHA256_NEXT`,
   `ANPFUEL_ADMIN_ORIGIN`, `ANPFUEL_ADMIN_ALLOWED_CIDRS` in
   `internal/platform/config`; startup failure on malformed values; add
   placeholders (never values) to `infra/env.staging.example` and
   `infra/env.prod.example`.
5. `ops admin keygen` in `backend/cmd/ops`: prints key and hash to stdout,
   writes no file. Test that two runs differ and that hash(key) == hash.
6. Route registration only when a hash is configured; a test proves 404 when
   unset.
7. Middleware `admin.RequireSession` usable by later PRs. Every handler that
   mutates must call one helper that validates reason + idempotency and writes
   the audit row in the same transaction.

Required tests (each a separate test function):

| # | Case | Expect |
|---|---|---|
| S1 | no hash configured, any admin path | 404 |
| S2 | malformed hash env | startup error |
| S3 | correct key | 200, `__Host-abv_admin` cookie with HttpOnly, Secure, SameSite=Strict, Path=/ |
| S4 | wrong key / empty / 10 KiB key / non-JSON / key in query | 401 or 400, identical 401 bodies |
| S5 | 6th failure from same IP within 15 min | 429 + Retry-After |
| S6 | 51 failures/hour from many IPs | 429 for all, also for correct key |
| S7 | concurrent 20 failed logins (goroutines) | counter exact, no lost update |
| S8 | session idle 31 min / absolute 8h01 (injected clock) | 401 |
| S9 | logout then reuse cookie | 401 |
| S10 | login twice | two different sessions; old remains until expiry; revoke-all kills both |
| S11 | rotation: session from removed hash | 401 |
| S12 | mutation without `X-Admin-CSRF` / wrong token / wrong Origin / no Origin | 403 |
| S13 | user account access token or signed-owner proof on admin route | 401 |
| S14 | admin cookie on user route | not accepted as user |
| S15 | unknown JSON field / body > 16 KiB | 400 / 413 |
| S16 | same Idempotency-Key replay / different body | same result / 409 |
| S17 | audit `UPDATE`/`DELETE` | DB error |
| S18 | log capture across S3–S12 | no key, token, cookie, CSRF value present |
| S19 | `/v1/admin/*` responses | `no-store`, `noindex`, `no-referrer` headers |
| S20 | CIDR fence set, request from outside | 404 |

### A2 — Granted tiers and capability projection (PR 2)

Branch `codex/adm-a2-granted-tiers`. Rules ENT-01…ENT-12, BUC-ADM-02/03/04.

1. Migration `000141_entitlement_grants.sql`: `entitlement_grants(id, account_id
   FK accounts, tier CHECK IN ('PRO','CONTRIBUTOR','ENTERPRISE'), policy_version,
   state CHECK IN ('ACTIVE','REVOKED'), start_at, end_at NULL, reason,
   created_at, created_session_id, revoked_at, revoke_reason)` + partial unique
   index `(account_id, tier) WHERE state='ACTIVE'` + owner index.
2. Module `backend/internal/modules/entitlement/` with `domain/catalog.go`
   (tier catalog v1 constant + `Capabilities(tier, policyVersion)`), grant
   validation, and store. Golden `contracts/admin/tier-catalog-v1.json` with a
   test asserting the Go catalog equals the JSON and that `PRO` equals the
   existing Premium bundle in `subscription/domain/policy.go`.
3. Subscription integration, minimal diff:
   - add `adminGrants(ctx, tx, accountID, now)` next to `webGrants`
     (same `42P01` tolerance);
   - extend `Snapshot` with `AdminCapabilities`, `AdminValidUntil`, `Tiers`;
   - add `mergeAdmin` after `mergeWeb`; reason `verified-admin` when only admin;
   - `RequireCapability` also checks admin grants;
   - `Status` gains `Tiers []TierStatus` (ENT-09). Update
     `contracts/premium/policy-v1.json` additively.
   - Confirm in `cmd/api/main.go` that capability checks and `/v1/premium/status`
     work with payments unconfigured (ENT-10); test it.
4. Admin endpoints: accounts search/detail, grant create/list/revoke (contract §7).
5. Erasure hook: revoke active grants on account erasure (find the existing
   erasure flow in `account`/`privacy`; add the call inside its transaction).

Required tests: domain table tests for ENT-02…07; integration (real PostGIS):
duplicate active grant under concurrency (two goroutines → one 201, one 409);
`replace` atomicity; expiry at the exact boundary (injected clock);
revoke-then-check denies; `RequireCapability` true for each PRO capability with
only an admin grant and payments unconfigured; status JSON contains `tiers`;
suspended/deleted target 409; body with `capabilities` 400; search by UUID /
alias / alias prefix / email (via `AddressHash`) and SQL-injection-like input
(`' OR 1=1 --`, `%`, `_`) returns nothing unexpected; erasure revokes; trust and
ranking code paths untouched (grep test: no import of `entitlement` from
`trust`, `moderation`, `community` ranking).

### A3 — Contributors (PR 3)

Branch `codex/adm-a3-contributors`. Rules CTB-01…CTB-14, BUC-ADM-05…09/11.

1. Migration `000142_contributor_profiles.sql`: `contributor_profiles`,
   `contributor_links` (max 2 by CHECK + trigger or app rule + test),
   `contributor_github_links` (unique partial indexes both ways),
   `contributor_claims`, `github_contributor_snapshots`.
2. Module `backend/internal/modules/contributor/`: domain validation
   (display name, platform/handle table, URL builder, claim state machine, code
   generation), GitHub client adapter (`net/http`, timeout, size cap, optional
   token; **tests use `httptest.Server`, never the real GitHub**), store, user
   HTTP routes, admin HTTP routes, public `GET /v1/contributors`.
3. CORS only for `GET /v1/contributors` from `ANPFUEL_PUBLIC_WEB_ORIGINS`.
   Add `/v1/contributors` to Caddy `@shared_cacheable` in `infra/caddy/Caddyfile`
   (identity is ignored by that route).
4. Erasure hook (CTB-13).

Required tests: every handle rule with valid/invalid examples
(`javascript:alert(1)`, `https://evil.com`, `user/../x`, Unicode look-alikes
such as Cyrillic `а`, 16-char X handle, empty, 3 links, two links same
platform); display name with `<script>`, RTL override `‮`, zero-width
joiner, 41 runes, emoji (rejected); claim quotas and states; gist proof
(owner mismatch, code absent, gist 404, GitHub 500/timeout → `UNVERIFIED` not
crash, oversized response); approval atomic (grant + link) and override rule;
link uniqueness under concurrency; public list never contains account id,
alias, email, reasons (JSON key allowlist assertion); revoked grant / hidden
profile / HIDDEN visibility / suspended account excluded; pagination stable;
CORS headers present only for allowed origin and only on that route; erasure
removes from list.

### A4 — Staff roles (PR 4)

Branch `codex/adm-a4-staff-roles`. Rules STF-01…STF-06, BUC-ADM-10/12.
Migration `000143_staff_roles.sql`; module `backend/internal/modules/staff/`;
golden `contracts/admin/staff-roles-v1.json`; admin endpoints for catalog,
assign, revoke; owner read through `POST /v1/contributor/profile/read`;
public labels in the contributors list per STF-05; audit read endpoint
`GET /v1/admin/audit`. Tests: unknown role 400, duplicate 409, inactive target
409, end_at bounds, no endpoint can modify the catalog (route table test),
holding any role does not authorize any admin route (S13-style test per role),
erasure revokes, audit filters and pagination.

### A5 — Staging VPS validation (PR 5)

Branch `codex/adm-a5-staging`. Follow `docs/operator/deploy.md` and
`docs/operator/access.md` exactly; staging only. Production is out of scope
until the user asks explicitly (G09/G24).

1. Add `scripts/tests/test-admin-staging.sh` (reads `ANPFUEL_ADMIN_KEY`,
   `ANPFUEL_STAGING_URL` and a synthetic test account from the operator's shell
   environment, prints no secret, `set +x`). It runs: S1-style disabled check
   is skipped on staging; login; who-am-I; wrong key 401; missing CSRF 403;
   search the synthetic account; grant `PRO` 1 day; `/v1/premium/status` shows
   tier; revoke; status no longer shows it; logout; reuse cookie 401;
   `GET /v1/contributors` 200 with CORS for the landing origin.
2. The user generates the key (`go run ./cmd/ops admin keygen`) and sets the
   hash in the staging env file on the VPS. The agent never sees or stores the
   raw key; it asks the user to export it in the shell that runs the script.
3. Record results (no secrets) in `backend/evidence/admin/A5-STAGING.md`.

## 3. Front B — Admin web UI

Visual standard: identical to the landing (`landing/static/assets/styles.css`
tokens, Manrope + Source Sans 3 from `landing/static/assets/fonts/`, same
buttons, cards, focus rings, spacing). Copy these at build time; do not fork
the token values by hand.

### B0 — Recon (no PR)

Read `landing/README.md`, `landing/scripts/build.mjs`, `check.mjs`,
`landing/static/assets/styles.css`, two landing tests, and contract §2/§7.

### B1 — Scaffold, login, accounts and grants (PR)

Branch `codex/adm-b1-admin-web`. New top-level package `admin-web/`
mirroring the landing layout (`static/`, `src/`, `scripts/`, `tests/`,
TypeScript 7.0.2 only, no framework, no runtime dependency).

Pages (`/admin/…`): `login/`, `accounts/` (search + detail with grants:
create with tier select, end date default +30 days, reason; revoke with
reason), shared header with session expiry and logout.

Rules: all data rendered with `textContent`/`createElement` — a test fails the
build if `innerHTML`, `outerHTML`, `insertAdjacentHTML`, `document.write` or
`eval` appears in `src/`; `fetch` with `credentials: 'same-origin'`; CSRF token
kept in memory only (never `localStorage`); every mutation sends
`Idempotency-Key` (crypto.randomUUID) and `X-Admin-CSRF`; on 401 go to login;
`<meta name="robots" content="noindex, nofollow">` on every page;
`static/_headers` with the CSP of ADR-023 + `frame-ancestors 'none'`.
Tests: build, no-unsafe-DOM scan, headers, API client unit tests against a fake
`fetch` (CSRF/idempotency headers present, 401 redirect, error mapping), form
validation (reason length, end date bounds).

### B2 — Contributors, claims, roles, audit (PR)

Branch `codex/adm-b2-admin-web-contributors`. Pages: `contributors/` (GitHub
snapshot with stale badge, link state, claims queue approve/reject with
override checkbox that requires reason), profile hide/unhide, `roles/` on the
account detail, `audit/` list with filters. Same tests pattern.

### B3 — Serve under Caddy and validate on staging (PR)

Branch `codex/adm-b3-admin-serving`. Serve built `admin-web/dist` at
`/admin/*` on the API origin from `infra/caddy/Caddyfile` (new `handle_path`
block with the admin CSP and `X-Robots-Tag`), mount in staging/prod compose,
extend the infra gate tests, deploy to staging, manual check with screenshots
of synthetic data only.

## 4. Front C — Contributor UI (app + landing)

### C0 — Recon (no PR)

Read `ProfileScreen.kt`, `ProfileActionRow.kt`, the Premium data path
(`GetPremiumStatusUseCase`, `PremiumViewModel`, its data source in `data/`),
navigation graph, one existing list screen, `landing/static/index.html`, nav
and footer, `landing/static/_headers`, `sitemap.xml`, landing tests.

### C1 — App: public contributors list (PR)

Branch `codex/adm-c1-app-contributors`.

- Data: `ContributorsRemoteDataSource` for `GET /v1/contributors` (cursor
  paging), domain model in `domain/`, use case in `application/`.
- UI: the **last** item of the Profile screen becomes a highlighted card
  "Contribuidores do projeto" (Premium-accent style like `PremiumEntryCard`),
  opening `ContributorsScreen`: list of name, verified-GitHub badge, role
  labels, up to 2 link chips. Empty, loading (skeleton), error-with-retry and
  offline states.
- Links: open only the server `url` after re-checking scheme `https` and host
  in the CTB-08 allowlist; otherwise do not render the chip. Use an
  `ACTION_VIEW` intent; no WebView.
- Tests: mapper (unknown platform dropped, non-https dropped, wrong host
  dropped), ViewModel states with fake repository, paging, strings in 8
  locales, Compose UI test for the card being last in Profile.
- `ProfileScreen.kt` is also touched by open PR #152: add only the new item at
  the end of the list; no reformatting.

### C2 — Landing: `/contribuidores/` page (PR)

Branch `codex/adm-c2-landing-contributors`.

- `landing/static/contribuidores/index.html` using the existing page shell,
  header, footer, typography and card components exactly. Static content
  without JS: what a contributor is, link to the GitHub repository and to the
  app. Progressive enhancement `landing/src/contributors.ts` fetches
  `https://<API>/v1/contributors` (API origin from a `<meta name="abastevo-api">`
  set at build time from an env var with a safe default of disabled) and
  renders cards with `textContent` only, links only after the same allowlist
  check as the app, `rel="noopener noreferrer nofollow ugc"`,
  `target="_blank"`.
- `_headers`: add the API origin to `connect-src` for `/contribuidores/*` only.
- Add to nav/footer, `sitemap.xml`, `llms.txt` if the pattern requires.
- Tests: page works without JS, no unsafe DOM API, CSP contains the API origin
  only on that path, rendering with fixture JSON (XSS payload names stay text),
  link allowlist, budget check passes.

### C3 — App: claim and profile self-service (PR)

Branch `codex/adm-c3-app-contributor-self`. Signed-in only.
"Sou contribuidor do projeto" flow: enter GitHub login → show code with copy
button and instructions to create a public gist → submit gist ID → claim
status. For accounts with `contributor.profile`: edit display name, up to 2
links (platform dropdown + handle), visibility switch with explicit consent
text, show-roles switch. Client validation mirrors CTB-08 but the server is the
authority (show server error codes). Tests: validators, ViewModel with fakes,
error mapping (403/409/429), strings in 8 locales.

### C4 — Staging check (no code unless a bug is found)

Point a debug build at the staging origin, verify list, claim and edit with a
synthetic account; verify the landing page against staging. Evidence in
`backend/evidence/admin/C4-STAGING.md`.

## 5. Definition of done for the whole batch

- All PRs A1–A5, B1–B3, C1–C4 merged by the user, each with its rule tests.
- Security matrix S1–S20 green; A2/A3/A4 negative and concurrency tests green
  on real PostGIS.
- Staging script green with evidence; no secret in Git history
  (`git log -p origin/main | grep -E 'abv_admin_[A-Za-z0-9_-]{20}'` empty).
- PROGRESS records each task state honestly; nothing claims production
  readiness.

# Admin suite (ADR-023) — what remains

Status: INTEGRATION_PENDING (2026-10-27). A1–A4, A5a, B1/B2 and C1–C3 are on
`main` (PR #176, plus #161/#164/#166/#167/#168). A5b, B3 and C4 are not
started; nothing of A1–A4 runs on staging yet.

## Context

- Plan and staging corrections: `docs/planning/ADMIN_CONTRIBUTOR_PLAN.md`
  (sections 1a, A5, B3, C4, 5); contract: `docs/product/ADMIN_TIERS_CONTRACT.md`;
  decision: `docs/adr/023-admin-suite-granted-tiers-contributors.md`.
- Phase evidence: `backend/evidence/admin/*.md` (A5a "Not done", B2 "Not done",
  C3 "Not done"). Staging is single-node k3s in `abastevo-temp`; admin code
  reaches staging only through `main` and the CD agent
  (`docs/backend/evidence/cd/CD-04.md`).

## Decision already taken (record, do not reopen)

- A2 payment gate: with payment ports unconfigured only admin grants unlock;
  Play and web rows keep unlocking nothing as before A2 (ENT-10,
  `TestAdminGrant_ENT10_UnconfiguredPaymentsOnlyAdminUnlocks`,
  `backend/evidence/admin/A2-GRANTED-TIERS.md`).

## Open contract questions (owner decision before or inside the code stage)

a. Account detail vs contract §7. §7 lists "roles, contributor
   link/profile/claims" in `GET /v1/admin/accounts/{id}`; the service type
   `AccountDetail` holds only account, grants and paid summary
   (`backend/internal/modules/admin/application/service.go`). B2 shows only
   session-scoped role assignments plus audit history, so an admin cannot
   revoke a pre-existing role from the panel (`B2-ADMIN-WEB.md`). Decide:
   implement §7 as written (recommended, contract already says it) or amend
   §7.
b. Landing `/contribuidores/`. The per-path CSP lives in
   `landing/static/_headers`, but the staging web tier sets one global CSP
   with `connect-src 'self'` in `infra/caddy/staging-web.Caddyfile` and does
   not read `_headers`; a cross-origin API fetch would be blocked. Decide the
   production API origin (same-origin vs separate API host) and where the
   path CSP is enforced. Also `landing/src/contributors.ts` fetches only the
   first page (no cursor follow-up); decide "load more" vs first page only
   (max 50 per page, CTB-10).
c. Contributor user routes (`backend/internal/modules/contributor/adapters/http/user.go`)
   authenticate with `family_id` + `access_token` only; §7 says "signed
   owner". Decide whether write routes (claims, proof, withdraw, profile
   upsert) require the SignedOwner device proof. Decide whether proof
   submission needs its own quota (CTB-03 caps claims, CTB-12 caps profile
   edits; nothing caps proof calls, each of which hits the GitHub API).
   Decide whether secret gists are acceptable: CTB-04 says "public gist", the
   client reads `GET /gists/{id}` and does not check visibility (verify).
d. ADM-03 says keys in query, path, cookies or headers make the request
   `400`; the login handler rejects only `?key=` and ignores other locations
   (`backend/internal/modules/admin/adapters/http/http.go`). Decide: enforce
   400 for header/cookie keys, or amend ADM-03 to "ignored". ADM-12: logout
   currently requires a reason (B1 sends a fixed reason); decide whether
   logout/revoke-all stay "mutations with reason" or are exempted.
e. Public role labels are pt-BR strings built in
   `backend/internal/modules/staff/domain/catalog.go` and shown verbatim in
   the app in all 8 locales. Decide: return role codes and localize in the
   app, or accept pt-BR labels for v1.

## Remaining work

1. Contract fixes per decisions a–e (code stage). Acceptance: each decision
   recorded in the contract/ADR; RED→GREEN tests for every changed rule
   (account detail with roles + contributor link/profile/claims and erasure
   negatives on real PostGIS; ADM-03 header/cookie case; proof quota 429 if
   adopted; landing paging and CSP test against the real serving tier);
   OpenAPI updated in the same commit; B2 panel lists and revokes existing
   roles from the detail.
2. B3 — serve `admin-web/dist` at `/admin/*` on staging. Plan 1a moves this
   from Compose to the k3s web tier (Caddy ConfigMap + web Deployment in
   `infra/k8s/staging/`, release directory switched by symlink). Today no
   Caddyfile has an `/admin` block and the signed release ships only the
   landing layer (CD-04 R7). Acceptance: `/admin/*` served with the admin CSP
   (`frame-ancestors 'none'`), `X-Robots-Tag` noindex and `no-store` HTML;
   artifact path decided (new OCI layer verified like the landing, or a
   reviewed manual release); infra gate tests extended; `kubectl diff` of
   the rendered namespace reviewed before apply.
3. Staging configuration (user-run). The user generates the key with
   `go run ./cmd/ops admin keygen`; only the hash enters the staging Secret;
   `ANPFUEL_ADMIN_ORIGIN`, `ANPFUEL_PUBLIC_WEB_ORIGINS`, `ANPFUEL_GITHUB_REPO`
   (optional read-only token) set. Acceptance: no agent sees the raw key;
   admin routes return 404 before the hash exists (ADM-01) and mount after.
4. A5b — run `scripts/tests/test-admin-staging.sh` against
   `teste.abastevo.com.br` (the only allowlisted remote) after A1–A4 are
   deployed from `main`. Acceptance: all 16 steps PASS, PASS-only output,
   synthetic account only, grant revoked and session logged out by the trap;
   results in `backend/evidence/admin/A5-STAGING.md`; key rotated afterwards.
5. B3 manual check — login → search → grant → revoke → contributors →
   roles → audit → logout through the served panel with synthetic data.
   Acceptance: screenshots of synthetic data only.
6. C4 — debug app build against staging: public list, claim + gist proof,
   profile edit, visibility; landing `/contribuidores/` against staging.
   Acceptance: `backend/evidence/admin/C4-STAGING.md`; CORS only on
   `GET /v1/contributors`; instrumented C1/C3 tests run on a device that
   accepts installs (local device refused, `INSTALL_FAILED_USER_RESTRICTED`).
7. Batch DoD (plan §5): `git log -p origin/main | grep -E
   'abv_admin_[A-Za-z0-9_-]{20}'` empty; PROGRESS states each task honestly;
   ADR-023 status line still reads PROPOSED although #161 merged (verify and
   update).

## Blockers and owner decisions

- Decisions a–e above.
- Deployment: depends on the CD acceptance (`CD_STAGING_ACCEPTANCE_REMAINING.md`)
  or one supervised CD apply of `main`; staging must first leave the maps-only
  images (`stg-7c7d776f`).
- Key custody stays with the user (generation, Secret patch, rotation).
- Production wiring stays out of scope (G09/G24).

## Delivery: one branch, one PR

- Stage ADM-S1 (code: items 1–2): branch `codex/adm-staging-b3`, one PR to
  `main` listing decisions a–e, B3 and their tests.
- Items 3–7 need ADM-S1 deployed from `main` (the CD agent only deploys
  signed `main` releases): a hard external dependency, recorded here as the
  split reason. Their evidence (A5-STAGING, C4-STAGING, screenshots) plus any
  fix they find ship as one follow-up PR; with no fix, the evidence rides in
  the next functional PR instead of a bookkeeping-only PR (AGENTS.md rule 8).

## Progress 2026-10-28

- B3 code: the signed staging release ships `admin-web/dist` under `admin/`
  in the landing layer (built, checked and tested in the release job), and
  the staging web Caddy serves `/admin/*` with `no-store` pages and immutable
  assets under the global CSP/noindex. Deployed: `/admin/login/` returns 200
  on staging; the configmap was applied with a diff showing only that block.
- The admin API stays inert (`/v1/admin/session` 404) until the owner adds
  `ANPFUEL_ADMIN_KEY_SHA256`, `ANPFUEL_ADMIN_ORIGIN` and
  `ANPFUEL_ADMIN_ALLOWED_CIDRS` to the staging API secret; the key is
  generated by the owner with `go run ./cmd/ops admin keygen` and never
  handled by an agent. A5b/C4 can run after that.
- A1–A4 runtime (contributors, grants, staff, migrations `000140`–`000143`)
  is live on staging since release `d181510a`; `/v1/contributors` returns 200.

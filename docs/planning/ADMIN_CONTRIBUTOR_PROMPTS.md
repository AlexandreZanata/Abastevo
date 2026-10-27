# Kickoff prompts — admin suite, granted tiers, contributors

Paste one prompt per agent session. Each prompt covers only the first phases
of its front; continue later with the "Continue" prompt at the end.

Prerequisite for all fronts: the planning PR that adds
`docs/adr/023-admin-suite-granted-tiers-contributors.md`,
`docs/product/ADMIN_TIERS_CONTRACT.md`,
`docs/planning/ADMIN_CONTRIBUTOR_PLAN.md` and this file is merged into `main`.

Order: start Front A first. Start Front B once the A1 PR is open. Start Front C
once the A3 PR is open (C1/C2 only need the public endpoint contract).

---

## Prompt — Front A (Backend), phases A0–A2

```text
You are working on the Abastevo repository (Go backend, PostgreSQL/PostGIS,
Android app, static landing). Talk to the user in Portuguese (pt-BR); write
code, docs and commits in English.

GOAL: implement Front A phases A0, A1 and A2 of
docs/planning/ADMIN_CONTRIBUTOR_PLAN.md: admin key authentication with
sessions and audit (A1) and admin-granted tiers PRO/CONTRIBUTOR/ENTERPRISE
merged into the existing Premium capability projection (A2). Stop after the A2
PR is open. Do NOT start A3.

SOURCE OF TRUTH, read fully before coding, in this order:
1. AGENTS.md
2. docs/product/ADMIN_TIERS_CONTRACT.md  (rules B-BR-ADM-*, B-BR-ENT-*)
3. docs/adr/023-admin-suite-granted-tiers-contributors.md
4. docs/planning/ADMIN_CONTRIBUTOR_PLAN.md sections 0, 1 and 2 (A0–A2)
5. backend/README.md and docs/backend/TEST_STRATEGY.md
If the code contradicts the contract, or a rule is missing, STOP and ask me.
Never invent rules, endpoints, fields or dependencies.

ISOLATION (mandatory):
- Do not edit /home/iiii/PESSOAL-PROJETOS-ALEXANDRE/brazil-fuel-prices
  directly: it has another person's uncommitted changes.
- From that repo run: git fetch origin
  git worktree add -b codex/adm-a1-admin-auth .worktrees/adm-backend origin/main
  and work only inside .worktrees/adm-backend.
- Never use git stash, reset --hard, push --force, --no-verify, or merge a PR.
- Migrations: reserved range 000140-000149. Check origin/main and open PR
  branches for the next free number before creating a file.

WORK METHOD:
- A0: write backend/evidence/admin/A0-RECON.md exactly as the plan says
  (plug-in points, composition for payments-not-configured, migration numbers).
- For every rule: write the failing test first (test name cites the rule ID,
  e.g. TestLogin_ADM04_RateLimitPerIP), then the minimal code, then refactor.
- Copy existing patterns: subscription/adapters/web_grants.go for reading
  grants inside the caller transaction; subscription/adapters/http/http.go for
  handlers and errors; cmd/ops/main.go for the CLI; existing migration tests.
- Domain packages stay pure (no I/O). SQL only through sqlc with parameters.
- The raw admin key must never be written anywhere except stdout of
  `ops admin keygen`. Tests use synthetic keys generated in the test.

A1 = PR 1 (branch codex/adm-a1-admin-auth):
- Implement everything in plan section A1, including all tests S1..S20.
- Checks: gofmt -l . (empty), go vet ./..., go test -race -count=1 ./...,
  PostGIS integration tests for the new module and cmd/api
  (-tags=integration, ANPFUEL_TEST_DATABASE_URL as in backend/README.md),
  go test ./internal/platform/apicontract -count=1, make quick-verify,
  git diff --check, and the secret grep from plan section 0 item 6.
- Add the routes to contracts/openapi/v1.yaml. Add one PROGRESS.md entry and
  backend/evidence/admin/A1-ADMIN-AUTH.md (commands + results, no secrets).
- Commit atomically (Conventional Commits), push, open PR to main with
  gh pr create --base main. PR body: rules covered, tests and results, not done.
  Do not wait for CI.

A2 = PR 2 (branch codex/adm-a2-granted-tiers):
- If PR 1 is merged: branch from origin/main. If not: branch from
  codex/adm-a1-admin-auth and open the PR with --base codex/adm-a1-admin-auth,
  stating it is stacked.
- Implement plan section A2 and every test listed there. Keep the subscription
  diff minimal; existing Premium tests must keep passing unchanged.
- Prove ENT-10: an account with only an admin PRO grant passes
  RequireCapability for each PRO capability while payments are NOT configured.
- Same checks, evidence file A2-GRANTED-TIERS.md, PROGRESS entry, PR.

FINAL REPORT to me (pt-BR): PR links, rules implemented with test names,
commands run with pass/fail counts, anything skipped and why, open questions.
If any check fails and you cannot fix it inside scope, stop and report it;
never weaken or delete a test.
```

---

## Prompt — Front B (Admin web UI), phases B0–B1

```text
You are working on the Abastevo repository. Talk to the user in Portuguese
(pt-BR); code, docs and commits in English.

GOAL: implement Front B phases B0 and B1 of
docs/planning/ADMIN_CONTRIBUTOR_PLAN.md: a new static admin web package
`admin-web/` with login, session header/logout, account search/detail and
grant create/revoke, using the admin API from Front A (PRs A1/A2). Stop after
the B1 PR is open. Do NOT start B2.

READ FIRST: AGENTS.md; docs/product/ADMIN_TIERS_CONTRACT.md sections 2, 3, 6,
7; docs/adr/023-admin-suite-granted-tiers-contributors.md; plan sections 0, 1,
3; landing/README.md; landing/scripts/build.mjs; landing/scripts/check.mjs;
landing/static/assets/styles.css; landing/package.json; two files in
landing/tests/. If the admin API on origin/main (or in the open A1/A2 PRs)
differs from the contract, STOP and ask me.

ISOLATION: from /home/iiii/PESSOAL-PROJETOS-ALEXANDRE/brazil-fuel-prices run
git fetch origin && git worktree add -b codex/adm-b1-admin-web
.worktrees/adm-web origin/main ; work only there. If the A1/A2 PRs are not
merged yet, you still build against the contract with a fake fetch in tests;
do not copy backend code. Never edit landing/ files except to read them
(the build copies tokens and fonts from landing/ at build time). No git stash,
force push, --no-verify or merge.

VISUAL STANDARD: exactly the landing look: same CSS tokens, Manrope + Source
Sans 3 local fonts, buttons, cards, spacing, focus rings, light theme. Reuse by
copying landing/static/assets/styles.css and the fonts into admin-web/dist at
build time; put admin-only styles in admin-web/static/assets/admin.css using
only existing tokens.

TECH RULES (tests must enforce them):
- TypeScript 7.0.2 strict, no framework, no runtime dependency, Node test
  runner only (same as landing).
- Render data only with textContent/createElement. A test fails if src/
  contains innerHTML, outerHTML, insertAdjacentHTML, document.write or eval.
- fetch with credentials 'same-origin' to relative /v1/admin/... paths.
- CSRF token from login kept in a module variable only (never localStorage or
  sessionStorage). Every non-GET sends X-Admin-CSRF and Idempotency-Key
  (crypto.randomUUID()). Any 401 returns to /admin/login/.
- Every page has <meta name="robots" content="noindex, nofollow">.
- admin-web/static/_headers: CSP default-src 'self'; script-src 'self';
  style-src 'self'; img-src 'self' data:; connect-src 'self'; font-src 'self';
  frame-ancestors 'none'; form-action 'self'; base-uri 'none'; plus
  X-Robots-Tag noindex, nofollow; Referrer-Policy no-referrer; no caching of
  HTML.
- Grant form: tier select (PRO, CONTRIBUTOR, ENTERPRISE), end date default
  today+30 days with "no end date" option, reason 10-500 chars, confirm dialog
  showing account alias + tier + end date. Revoke requires reason + confirm.
- Never display or log the admin key; the login input is type=password,
  autocomplete=off, cleared right after submit.

B1 = PR (branch codex/adm-b1-admin-web): pages /admin/login/,
/admin/accounts/ (search + detail + grants). Tests: build, check script,
unsafe-DOM scan, headers content, API client with fake fetch (headers present,
401 redirect, error code mapping), form validators. Checks: npm ci,
npm run typecheck, npm run build, npm run check, npm test, git diff --check,
secret grep (plan section 0 item 6). Add PROGRESS.md entry and
backend/evidence/admin/B1-ADMIN-WEB.md. Push and open PR to main (or stacked
on the A2 branch if the UI cannot build without it; say so). Do not wait for
CI.

FINAL REPORT (pt-BR): PR link, screens done, tests and results, what is
stubbed against the contract, open questions.
```

---

## Prompt — Front C (Contributor UI: app + landing), phases C0–C2

```text
You are working on the Abastevo repository (Android Kotlin/Compose app in
app/, application/, data/, domain/ modules; static landing in landing/).
Talk to the user in Portuguese (pt-BR); code, docs and commits in English.

GOAL: implement Front C phases C0, C1 and C2 of
docs/planning/ADMIN_CONTRIBUTOR_PLAN.md: the public contributors list in the
app (highlighted last item of the Profile screen opening a Contributors
screen) and the landing page /contribuidores/. Both only READ the public
endpoint GET /v1/contributors. Stop after the C2 PR is open. Do NOT start C3.

READ FIRST: AGENTS.md; docs/product/ADMIN_TIERS_CONTRACT.md sections 4
(especially CTB-08, CTB-10, CTB-11) and 7; plan sections 0, 1 and 4; then the
files listed in plan C0. If the public endpoint on origin/main or in the open
A3 PR differs from the contract, STOP and ask me.

ISOLATION: from /home/iiii/PESSOAL-PROJETOS-ALEXANDRE/brazil-fuel-prices run
git fetch origin && git worktree add -b codex/adm-c1-app-contributors
.worktrees/adm-contributors-ui origin/main ; work only there. Never edit the
main checkout. ProfileScreen.kt is also changed by open PR #152: add only the
new last item, no reformatting or reordering. No git stash, force push,
--no-verify or merge.

C1 = PR (branch codex/adm-c1-app-contributors):
- Follow the existing Premium status data path as the pattern
  (domain model -> application use case -> data remote source -> ViewModel).
- Profile screen: the LAST item is a highlighted card "Contribuidores do
  projeto" styled like PremiumEntryCard (PremiumColors accent). It opens
  ContributorsScreen: name, "GitHub verificado" badge when github_login exists,
  role labels, up to 2 link chips. Loading skeleton, empty, error+retry,
  offline states. Cursor paging.
- Links: render a chip only if url starts with https:// and its host is in the
  CTB-08 allowlist (github.com, gitlab.com, www.linkedin.com, x.com,
  www.instagram.com, www.youtube.com, bsky.app). Open with ACTION_VIEW. No
  WebView.
- Strings in all 8 locales: values, values-pt-rBR, values-es, values-fr,
  values-de, values-ja, values-ru, values-zh-rCN.
- Tests: mapper drops unknown platform / non-https / wrong host; ViewModel
  states with a fake repository; paging; Compose test that the card is the last
  Profile item. Checks: ./gradlew :app:testDebugUnitTest :application:test
  :data:test :domain:test lintDebug assembleDebug (only modules that exist),
  git diff --check, secret grep. PROGRESS entry + evidence file
  backend/evidence/admin/C1-APP-CONTRIBUTORS.md. Push, PR to main.

C2 = PR (branch codex/adm-c2-landing-contributors, from origin/main, it does
not depend on C1):
- landing/static/contribuidores/index.html built with the SAME shell, header,
  footer, typography and card components as the existing landing pages; copy
  structure from an existing page, do not create new visual styles beyond
  existing tokens.
- Without JavaScript the page still explains what a contributor is and links
  to the GitHub repository and the app.
- landing/src/contributors.ts (progressive enhancement): reads the API origin
  from <meta name="abastevo-api" content="...">, set at build time from an env
  variable; when empty, the script does nothing. Renders cards with
  textContent only; links only after the same allowlist check; anchors use
  rel="noopener noreferrer nofollow ugc" target="_blank".
- landing/static/_headers: add the API origin to connect-src only for
  /contribuidores/*. Add the page to nav/footer and sitemap.xml following the
  existing pattern.
- Tests: page content without JS, no innerHTML/outerHTML/insertAdjacentHTML/
  document.write/eval in src/contributors.ts, CSP change scoped to the path,
  rendering with fixture JSON including an XSS payload name (stays text),
  allowlist, existing budget/check tests pass. Checks: npm ci, typecheck,
  build, check, test, git diff --check. PROGRESS entry + evidence file. Push,
  PR to main.

FINAL REPORT (pt-BR): PR links, screenshots of synthetic data only (never real
users), tests and results, open questions.
```

---

## Continue prompt (any front, next phases)

```text
Continue Front <A|B|C> of docs/planning/ADMIN_CONTRIBUTOR_PLAN.md with phase
<ID> only (for example A3). Re-read AGENTS.md, the contract sections for that
phase, plan section 0 and the phase section. Work in the same worktree
(.worktrees/adm-backend | adm-web | adm-contributors-ui). First run
git fetch origin and check whether the previous PR of this front was merged:
if yes branch from origin/main, if not stack on the previous branch and open
the PR with --base <previous-branch>. Same rules: tests first citing rule IDs,
all listed tests, all checks, evidence file, PROGRESS entry, one PR, no merge,
no CI polling, stop and ask on any ambiguity. Report in pt-BR.
```

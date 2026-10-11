# LR00 — Landing redesign branch reconciliation

- Task: LR00 (reconcile branch and actual prior work)
- Date: 2026-10-11
- Branch: `landing` at `20bb603` (in sync with `origin/landing`)
- Status: LOCAL_DONE (docs audit only; no website/backend behavior change)

## Refs at audit time

| Ref | SHA | Note |
|---|---|---|
| `landing` (this worktree) | `20bb603` | clean tree, in sync with `origin/landing` |
| `origin/landing` | `20bb603` | no push needed |
| `dev` / `origin/dev` | `a853708` | in sync; occupied primary checkout, uncommitted CS edits untouched |
| `main` / `origin/main` | `a349d7c` | in sync |

- `landing` vs `dev`: 3 ahead (plan/fonts/LR00 commits), 7 behind (CS/backend chain). No backend files touched on `landing`.
- `landing` vs `main`: 20 ahead, 0 behind (17 inherited at opening + 3 preparation/reconciliation commits).
- Worktree: `.worktrees/landing-refresh` on `landing`; primary checkout stays on `dev` and was never switched, stashed or reset.

## Preserved prior work (no duplicate foundation)

- P25 static landing acceptance: `docs/release-evidence/p25-static-landing.md` present.
- P26 guides acceptance: `docs/release-evidence/p26-landing-guides.md` present.
- Preparation commits on `landing`: `61453e2` (redesign plan, prompt, 30-chapter catalog, authentic reference/media, local Manrope/Source Sans 3 WOFF2 with OFL/provenance/hash-copy regression, content-contract amendment, README/ROADMAP/PROGRESS notes), `1e353fd` (font-license whitespace) and `20bb603` (this LR00 reconciliation).
- Prepared artifacts verified present: `landing/static/assets/fonts/` (4 hashed WOFF2 + OFL notices + provenance), `landing/tests/local-fonts.test.mjs`, `docs/assets/landing/redesign-20261011/` (reference HTML/PNGs, manifest, 8 native captures, preparation evidence), `docs/product/LANDING_MOBILE_TUTORIAL_CATALOG.md`.

## Gates and limits

- Required `Quick verification` gate preserved; gate-selection tests cover `landing/`.
- No runtime suite for this audit per plan (docs-only). Artifact reconciliation re-verified now: 8/8 manifest SHA-256 + 1220x2712 dimensions OK, 4/4 font hashes + WOFF2 signatures + OFL notices OK, TU01–TU30 catalog present, reference v2 HTML + desktop/mobile PNGs present, P25/P26 evidence preserved. Preparation-reported build figures (typecheck/build/check green, 25 landing tests, gate selection 18/18) recorded as reported, not re-executed here.
- Validation: `git status --short --branch` clean, `git diff --stat` empty, `git log -5 --oneline`, `git worktree list`, `git diff --check` clean, `bash scripts/scan-secrets.sh` passed. No image-generation call spent; superseded AI concept stays excluded.
- No deployment, DNS/CDN/VPS, store listing, URL submission, issue/PR/merge/wiki, or production/Android acceptance follows from LR00.

## LR29 performance record — 2026-10-11

Measured on emitted `landing/dist` (not projections): CSS gzip 4,075 B
(budget 25 KiB), JS gzip 1,290 B (budget 10 KiB), fonts 128,916 B (new budget
150 KiB), 72 WebP derivatives 3,400,760 B total with largest 97,168 B
(new per-file budget 120 KiB, PNG masters refused by the checker). `npm run
check` green. No new images added; full Core Web Vitals still needs field
measurement after deployment (owner).

## LR36 delivery decision — 2026-10-11

INTEGRATION_PENDING, no merge performed. Reviewed: 39 landing-only commits
against `dev` touch only docs/landing-static/web assets/ROADMAP (no
backend/Android); PR #128 stays draft with no CI checks; `dev` has 30 unique
commits plus another contributor's uncommitted staged work; `dev` branch is
unprotected while required Quick verification applies at dev→main. Merging
now would risk conflicts with another contributor's active work. Owed, in order: human review of
PR #128, landing sync onto latest `dev` with conflict resolution, required
current-head verification, then merge via the guarded path.

## LR35 scoped union — 2026-10-11

Single union run at `3da311e` (no repeat without new failure/change):
landing typecheck green, build green (CSS `a63f6f37`, JS `72d6a45f`,
prelaunch), `npm run check` PASSED, landing tests 40/40, gate selection
18/18 (`test-gate-selection.sh`), `git diff --check` clean, secret scan
passed. No backend/PostGIS/Android suites: landing-only scope per plan.

## LR32 accessibility record — 2026-10-11

Measured/static (not browser) exit: text contrast navy/white 16.92,
`--text-muted` #44506b on white 8.06 / on surface 7.57, blue/white 8.12 both
ways (all ≥ AA 4.5); primary controls ship ≥44px hit areas with a visible
3px focus ring; smooth scroll yields to `prefers-reduced-motion`;
disclosure stays native `details/summary`. Live 200% zoom reflow and
keyboard-only traversal still need a real browser pass (no browser in this
environment); automated anchor/heading/skip-link coverage holds.

## LR37 deploy record — 2026-10-11

DEPLOY_PENDING, no remote change. No domain/SSL/hosting/publish approval was
requested or received in this batch; no preview deployment, DNS/CDN edit,
smoke run or rollback was executed. Owed, in order: landing→dev integration
(LR36), protected dev→main delivery with Quick verification, then owner
approvals for preview, publish, smoke and rollback evidence.

## LR38 index follow-up — 2026-10-11

Local sitemap↔dist parity verified by test (every emitted indexable page
listed; every listed URL resolves; no 404/lastmod/placeholders). Remote
index verification stays explicitly pending: no Search Console/Bing
permissions or keys available, site undeployed, nothing submitted, no
receipts. Owed after publish: ownership verification, crawler-access check,
sitemap availability and per-URL submission receipts.

## LR39 SEO/AI-search report — 2026-10-11

Verified locally (emitted dist, tests green): normal indexable HTML on 25
pages; semantic pt-BR text; alt/context on all gallery/tutorial images;
stable canonical URLs with sitemap↔dist parity and true lastmod-free
entries; matching JSON-LD (WebSite/Organization/WebPage/CollectionPage/
Article/BreadcrumbList, no fabricated rating/Offer/FAQ/HowTo); crawlable
internal links with zero dead ends; preview noindex guard on non-404 pages;
robots separates search crawlers (allowed, incl. OAI-SearchBot) from
training crawlers (GPTBot/ClaudeBot disallowed) with user fetchers named
transparently; llms.txt as optional factual aid (hub/chapters listed), not a
ranking instrument.

Owed per source, none claimed: Google/Bing indexing and rich results
(undeployed, no submissions); IndexNow key + submissions (owner action);
Search Console/Bing Webmaster ownership, diagnostics and receipts;
answer-engine citations (no guaranteed ranking or AI placement anywhere);
field Core Web Vitals; live zoom/keyboard browser pass. No doorway pages,
no keyword stuffing, no hidden bot-only content, no guaranteed ranking.

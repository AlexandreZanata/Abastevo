# Economical-agent prompt — landing redesign and complete mobile tutorials

Paste the following into the implementation agent. Replace only `NEXT_TASK` with
the next unaccepted LR ID. Keep the task/state ledger in the existing plan and
PROGRESS; do not reread the entire repository on every turn.

```text
Implement the next bounded task of the Abastevo landing redesign.
NEXT_TASK=LR00

MANDATORY BRANCH: use the existing branch landing, dedicated to the landing page.
The explicit user choice overrides the default dev development branch for this
bounded scope only. Prefer the already-created isolated checkout
/home/iiii/PESSOAL-PROJETOS-ALEXANDRE/brazil-fuel-prices/.worktrees/landing-refresh.
First verify git branch/status/worktree list and current commits. Do not switch,
stash, overwrite or reset the occupied primary dev checkout. Do not recreate
landing or import another contributor's uncommitted backend changes.

Read applicable AGENTS.md, docs/planning/FAST_EXECUTION.md, only current PROGRESS,
the selected task in docs/planning/LANDING_REDESIGN_EXECUTION_PLAN.md and its
listed source/tests. Read DELIVERY_WORKFLOW at phase opening/policy change.
Use docs/product/LANDING_MOBILE_TUTORIAL_CATALOG.md for TU01–TU30 and
LANDING_CONTENT_CONTRACT.md for claims, typography and source distinction.
Preserve existing P25/P26 acceptance/history; LR00–LR39 are follow-up tasks.

Work sequentially, one bounded LR task at a time. When the caller authorizes a
construction batch, continue from each tested LOCAL_DONE checkpoint to its next
explicit dependency; do not wait for PR/CI/merge during construction. A request
for one task only does not authorize the entire roadmap. Keep a concise ledger
of files, revision, checks, actual state, missing evidence and next action. Make
meaningful local commits. No agent spawning or paid external services by default.

Use the current visual reference:
docs/assets/landing/redesign-20261011/landing-reference-v2.html
and its desktop/mobile PNGs. The first AI-generated concept is superseded.
No phone/mockup in the opening hero. Create a separate app gallery and a further
separate tutorials section, linking to /como-usar/ and individual chapter routes.
Use ONLY real app screenshots in mockups and lessons. Never redraw app screens,
invent controls or ask imagegen to create app content. Original PNGs are in the
media package; check manifest.json, version, hashes, privacy and state before use.
Marketing captures stay clean. Tutorials add transparent numbered RED RECTANGLES
as responsive SVG overlays in the image's native coordinate system, with action,
expected result, recovery, Previous/Next and equivalent readable HTML text.
Never turn a demonstration click into an account/API write. Check alignment at
320/375/768/1440px and 200% zoom. Result frames may omit a rectangle.

Font choice is already integrated locally: Manrope headings/CTAs/navigation/cards/
prices/emphasis (700/800 headings, 700 prices), Source Sans 3 prose/FAQ/legal/info
(400/500). Reuse the official local variable WOFF2 assets, hash names, OFL notices,
swap/fallback and build tests. Preserve the brand vector. No Inter, Geist,
Poppins or Montserrat; no font CDN or mobile typography change.

Cover every actual mobile route/control via the 30-chapter catalog, including
empty/error/offline/permission/result states. Current primary auth is username
and saved account key; do not teach historical email/provider flows as working.
A source route or future plan is not runtime evidence. Missing media remains
MEDIA_PENDING; unavailable features remain explicitly pending. If backend/app
work is required, record an owner dependency instead of implementing it for a
marketing task. No invented account key, screenshot data or synthetic success.

The physical phone belongs to the user. Never run clearAppStorage seeders, reset
it, expose account keys/private photos/precise contributor GPS, submit mutations
or change persistent settings for a screenshot. Use a disposable emulator/profile
and clearly identified demo data for authenticated/destructive/permission flows.
Confirm actual device access when needed; locked/private frames are rejected.
Restore any changed preferences. The accepted package has eight 1220x2712 native
captures, debug 3.1.0/code4; installed source SHA unknown. It does not prove all
features or full Android acceptance. Do not capture the user's private media
notification overlay. Extend the manifest only after reviewing each frame.

SEO: inspect existing robots/sitemap/canonical/metadata/JSON-LD/legal/guides/tests;
extend rather than duplicate. Use /home/iiii/ZAFLAS/zaflas-landing/ READ-ONLY for
SEO/indexability/discovery patterns ONLY, never its visual design, identity,
runtime or deployment rules. Reverify current official Google/Bing/IndexNow and
OpenAI/Perplexity/Anthropic crawler docs linked in the plan. Normal indexable HTML,
accurate semantic text, images/alt/context, stable canonical URLs, true sitemap
lastmod, matching JSON-LD and crawlable links come first. Separate search bots,
user fetchers and training bots; do not broaden training permission silently.
llms.txt/Markdown are optional factual aids, not special ranking requirements.
No fabricated rating/Offer/FAQ/HowTo eligibility, hidden bot-only content,
keyword stuffing, duplicate city doorway pages or guaranteed AI/search ranking.
Audit preview noindex in actual emitted pages and headers. Ownership-dependent
Search Console/Bing submissions/IndexNow stay pending until separately authorized.

For behavior changes, update B-BR/BUC first, then meaningful failing regression,
implementation and refactor. Use targeted node tests during each task. Do not add
runtime dependencies/frameworks for a static requirement; any new dependency
needs recorded need/license/security review. Run affected typecheck/build/check,
diff-check and secret-surface review. Do not rerun backend/PostGIS/full Android
suites for docs/media/fonts-only changes. Run the scoped union once at LR35; do
not repeat aggregate runs without a new failure/change. Critical changes still
need owning negative/failure/concurrency tests immediately.

No deployment, DNS/CDN/VPS changes, store publication, remote trackers, URL
submission, PR/merge/wiki publication follows merely from this prompt. At final
authorized delivery, inspect the complete branch history, integrate reviewed
landing commits normally into maintained dev and use protected dev→main delivery
with required current-head Quick verification and guarded helper. Never force
push, bypass checks or send the branch's inherited unrelated commits unreviewed.
Keep landing for ongoing website work. Preserve MIT notices and com.anpfuel.

Communicate in Portuguese, keep code/technical docs/commits in English. Finish
with the actual task status, files, concise test evidence, limitations and next
LR ID. Never mark a task complete because its plan or visual reference exists.
```

Current preparation already delivered the reference, eight authentic captures and
local fonts; LR00 must reconcile these artifacts before scheduling remaining
capture, production redesign, complete tutorials and SEO work. Do not spend
another image-generation call recreating the same design or screens.

# Landing redesign, authentic app media and guided tutorials

Date: 2026-10-11. Status: PLANNING_LOCAL_DONE; FONT_INTEGRATION_LOCAL_DONE /
REDESIGN_AND_FULL_TUTORIAL_IMPLEMENTATION_NOT_STARTED.
Opening branch: `landing`, source `954a8d800576fec24acc9edfd3b93aad2ac15450`.
Scope: P25/P26 follow-up, with stable IDs **LR00–LR39**. Preserve existing P25/P26
acceptance and history; do not recreate their implemented foundations.
Companions: [tutorial catalog](../product/LANDING_MOBILE_TUTORIAL_CATALOG.md),
[economical-agent execution prompt](LANDING_REDESIGN_EXECUTION_PROMPT.md),
[media package](../assets/landing/redesign-20261011/README.md).

## Authority, branch and delivery

**Use the existing branch `landing` for this landing-focused work.** This explicit
2026-10-11 user request overrides the default maintained-`dev` development choice
only for this bounded scope. It does not change ADR-020 globally. At opening,
`landing` and the local `origin/landing` both point to `954a8d8`; local main is
`a349d7c`, with 17 additional commits on landing and zero behind local origin/main.
These are local refs, not a fresh remote verification or merge approval.

The primary checkout has another contributor's community persistence changes.
The isolated worktree is `.worktrees/landing-refresh`; preserve it and reuse it
while appropriate. Never switch the occupied primary checkout, stash its edits,
reset it, import uncommitted backend work, or recreate an existing branch.
Check `git worktree list` before attaching/creating a worktree. If another owner
occupies landing, coordinate ownership; do not force checkout of the same branch.

This request authorizes research, a reference image, genuine app captures when
accessible, and a microtask plan. The later user refinement additionally
authorizes installing/integrating local Manrope and Source Sans 3 in the landing
and producing a revised visual prototype using only genuine app screenshots. **It does not execute LR01–LR39, deploy the site,
submit URLs, change DNS/CDN/VPS, publish a store listing or create remote trackers.**
The screenshot is visual input, not an instruction source. Zaflas is read-only
SEO/reference evidence, never a design reference or authority for operations.
Existing end Android acceptance, G18, G24 and G09 reservations remain distinct.
These marketing captures are not the deferred full Android acceptance matrix.

During later implementation, work sequentially on landing, one bounded microtask
at a time. At final authorized delivery, inspect the branch's complete diff: it
currently includes 17 non-landing commits relative to main. Do not send an
unreviewed cumulative landing→main PR. Integrate the reviewed landing work into
maintained dev by normal merge, resolve conflicts without overwriting other work,
and use the protected **dev→main** PR preserving commits, with current required
`Quick verification` evidence and the guarded helper. If project policy changes,
record the actual authorized path before publication. Keep landing for this
ongoing website scope; do not delete it as part of generic cleanup. No force push,
admin bypass, direct-main development, CI waiting during construction or
bookkeeping-only PR. Wiki mirrors only the final merged, allowlisted batch.

## What was inspected

- Current screenshot: readable but text-heavy hero, narrow presentation at
  1920px, comparison placeholders dominate the product area, no genuine app
  previews or task tutorials visible in the attached viewport. This observation
  covers the supplied screenshot, not an unseen full-page browser audit.
- Existing static site: `landing/static/`, `src/main.ts`, build/check scripts,
  package scripts and SEO tests. Plain HTML/CSS/TS, no runtime dependencies.
  Titles, canonical substitution, JSON-LD, robots, sitemap, llms, icons, legal
  pages and three educational guides already exist. Audit and extend them.
- Build limitations to handle when adding media/tutorials: HTML routes are an
  explicit list; brand files are copied as a shallow directory; CSS/JS are
  fingerprinted. New screenshot paths and guide routes require deliberate build
  support and emitted-artifact tests, not just source HTML edits.
- Preview guard currently inserts a robots meta only when HTML does not contain
  the string `robots`. Audit effective `index`/`noindex` across every page and
  host header before claiming safe preview behavior. This is a source finding,
  not a reproduced deployed incident.
- Android inventory: `Routes.kt`, `AnpNavGraph.kt`, main screens, vehicle form,
  station experience, capture, account and settings, and pt-BR strings. The
  current primary account flow is **username + saved account key**. The older
  P25 contract's email/provider description is historical and must be reconciled;
  never teach an unavailable Google/Apple/email path from old docs alone.
- Existing instrumentation seeders call `clearAppStorage(context)`. Never run
  these against the user's connected phone. Use a disposable emulator/profile
  for deterministic fixtures and destructive tutorials.
- Read-only SEO reference: `/home/iiii/ZAFLAS/zaflas-landing/`, HEAD
  `5216ebc75f3c5e72b4cbc17a0882ae2a22f645fa`. Inspected SeoHead, SEO site/meta,
  JSON-LD graph, robots, llms, sitemap config and Markdown middleware. No observed
  ranking, traffic, indexation or performance result is inferred from that code.
- Physical Android discovered: 1220×2712, density 480, installed debug app
  version 3.1.0-debug/code 4. Installed build SHA is unknown. A black display
  capture was rejected while locked; the user subsequently unlocked the phone and
  explicitly authorized ADB navigation. See media manifest for subsequent
  accepted captures or pending obligations. Never label this installed APK as
  matching the current source without proof.

## Behavior contract

### B-BR-LR01 — Product claims follow demonstrated behavior

Keep official ANP survey values/dates distinct from community observations,
conditions, confidence and cache freshness. ANP is not a current pump quote.
No guaranteed savings, cheapest-nearby promise, national live coverage, fabricated
ratings/testimonials/user counts or downloadable Play release without evidence.
A debug screenshot proves the visible screen only. Label prerelease/beta,
fixtures, TEST_ONLY, stale and unavailable features honestly. Update the
[content contract](../product/LANDING_CONTENT_CONTRACT.md) before changing copy.
The generated reference is inspiration, never an authentic screen or final logo.

### B-BR-LR02 — Captures are authentic and safe to publish

Preserve original lossless PNG dimensions, pixels and aspect ratio. No AI repair,
upscaling, inserted prices, repainted controls or fake success overlays. Stage
screens with anonymous data before capture; reject screens containing account
keys, identity documents, personal identifiers, notifications, precise contributor
GPS or production photos. Public station addresses differ from contributor GPS.
Do not copy private device databases/logs into Git. Record feature/state, app
version, source SHA if known, capture size/date, checksum, fixture provenance,
publication eligibility and limitations. Never retain the phone serial in public
metadata. Device frames are separate authored CSS/SVG compositions; preserve
MIT/brand rights and record external template licenses if any.

### B-BR-LR03 — Tutorials show precisely where to act

Every action step includes the exact screen, prerequisite, Portuguese control
label, one numbered **red rectangle**, instruction and expected result. Use a
transparent SVG/HTML overlay on the unmodified capture. Store rectangle coordinates
in the original image coordinate space, not viewport pixels. Support responsive
scaling and letterboxing correctly. Never place boxes approximately from memory.
Use UI bounds or inspected image bounds and verify them at actual display sizes.
A number and text accompany red so color is never the only signal. Split steps
when several actions would obscure the screen; a final result step can omit a box.

### B-BR-LR04 — Demonstration has no remote side effects

The tutorial frontend is a local static walkthrough, not a second mobile client.
Buttons move between captured states only. No account creation, voting, uploads,
location access, camera access, deletion or moderation is sent from the website.
Label it `Demonstração guiada`; a synthetic flow is labelled as such. Do not build
fake phone functionality from inferred UI. Unavailable chapters show a factual
availability note and remain outside any claim of complete live functionality.

### B-BR-LR05 — Useful without JavaScript, accessible with it

Tutorial text, ordered steps, images, captions, navigation and legal links exist
in emitted HTML. JS may enhance Previous/Next, progress and filters. Keyboard,
visible focus, escape from any dialog, reduced motion, screen-reader announcements,
200% zoom and 320px reflow are mandatory. No autoplay, forced tour, scroll trap,
canvas-only copy or network-only lesson content. Never intercept browser shortcuts.

### B-BR-LR06 — Discovery and AI training are different decisions

Apply crawl/index controls per environment. Search visibility motivates Googlebot,
Bingbot, OAI-SearchBot, PerplexityBot and Claude-SearchBot access to **public site
content**; it does not authorize private API/media access. Audit inherited policy
and document GPTBot/ClaudeBot/Google-Extended training choices separately. Do not
silently opt into training to improve search. User-triggered fetchers have their
own documented behavior; robots is not an access-control boundary.

### B-BR-LR07 — Structured data is factual

Every JSON-LD statement must match visible current content. Reuse stable entity
IDs and canonical URLs. No copied Zaflas company/CNPJ/socials/ratings/offers,
LocalBusiness station impersonation, hidden FAQ stuffing or nonexistent locale
alternates. WebSite/WebPage/BreadcrumbList and factual image metadata suffice;
MobileApplication remains gated by the actual release facts. HowTo/FAQ markup
cannot be an acceptance requirement for Google rich results.

### B-BR-LR08 — Lightweight delivery and truthful acceptance

Retain HTML/CSS/TS, exact existing compiler/lockfile and zero runtime packages.
No Astro/React/CMS/backend service, analytics SDK, paid SEO service or live API
coupling is needed. Record need/license/security before any dependency addition.
Preserve current budgets: compressed CSS ≤25 KiB, compressed JS ≤10 KiB,
initial home transfer ≤500 KiB. Lazy-load the tutorial library; do not preload
all phone images. New page budgets must be explicit before implementation.

### Use cases

- **BUC-LR01:** visitor understands sources and unreleased Android status, then
  opens app previews or GitHub without creating an account.
- **BUC-LR02:** visitor selects a feature, reads prerequisites, advances through
  exact highlighted controls and recognizes the result and recovery path.
- **BUC-LR03:** maintainer selects a genuine, privacy-reviewed PNG, places it in
  a licensed/custom mockup, and traces it back to a manifest entry.
- **BUC-LR04:** search/AI crawler receives coherent public HTML, metadata,
  canonical links and factual documentation without JS or authentication.
- **BUC-LR05:** a feature unavailable in the captured build stays explicitly
  pending; its chapter cannot fabricate controls, receipts or production evidence.

## Visual direction to implement later

The initial AI-generated concept is SUPERSEDED and excluded from this delivery.
The user refinement requires **no generated app interface in any mockup or
tutorial**, and no phone mockup in the home opening. Use the
[revised authentic-screen prototype](../assets/landing/redesign-20261011/landing-reference-v2.html)
and its browser-rendered PNG as the current composition reference. It combines
HTML/CSS with unmodified screenshots from the actual connected app. The two-step
interactive preview demonstrates the treatment; it is not the complete tutorial
implementation. Preserve approved brand vectors verbatim.

Proposed order: compact header → product-message hero without phone mockups →
quiet principles strip → separate authentic app gallery → three action-oriented feature stories → tutorial teaser
→ source explanation → project/privacy → FAQ → availability → useful footer.
The tutorial section is separate from both the hero and the app gallery.
Mobile uses one column and a legible single phone within the app/tutorial sections. Avoid a long row of miniature
screens, excessive borders, oversized empty hero, generic stock photos or
repeated identical cards. Target a 1120–1200px content area, fluid H1 around
40–60px desktop/32–40px mobile, 16–18px readable body, 4px spacing grid and
16–24px card radii where appropriate. Measure rather than forcing these values.

Candidate pt-BR H1: `Preços de combustíveis. Fonte e data à vista.` Candidate
primary CTA: `Conheça o aplicativo`, to the real app gallery anchor; secondary
`Ver no GitHub`. Keep `Android em desenvolvimento` and store state factual.
Typography is **Manrope** for titles/CTAs/navigation/cards/prices/emphasis
(700/800 headings, 700 prices), **Source Sans 3** for prose/FAQ/legal/info
(400/500), self-hosted with OFL notices and swap/system fallback. This user
decision supersedes the old system-only family. Preserve the vector wordmark.
Do not introduce Inter, Geist, Poppins or Montserrat.

Use navy `#0b1849`, blue `#0042c5`/`#0052f7`, teal source accent and pale surfaces.
Any proposed soft mockup shadows, larger radii or section rearrangement explicitly
revise the old flat/no-shadow visual contract at LR03; do not silently conflict.
Dark marketing appearance remains out until approved artwork/contrast exist.

## Execution sequence and microtasks

All tasks below are **PLANNED**, except this request's limited research/media
preparation recorded separately. Read only the selected task and owning files.
Standard risk unless specified. A task normally touches one concern and 1–4
owning files plus focused tests/evidence. Split an unexpectedly large task using
an ID suffix (for example LR23a) rather than expanding scope silently.

### Phase LR-A — Reconcile and freeze

#### LR00 — Reconcile the branch and actual prior work

- **Depends:** none. **Rules:** B-BR-LR01/LR08; BUC-LR01.
- **Goal/microsteps:** inspect status/diff/log/worktrees; confirm landing ownership;
  compare main/dev/landing; inventory existing P25/P26 evidence and affected gates.
- **Files:** phase evidence `docs/release-evidence/landing-redesign.md` (create on
  execution), current PROGRESS. Do not import another checkout's uncommitted files.
- **Accept:** exact starting SHA, preserved changes, selected next task, no duplicate
  foundation work. Full branch comparison recorded before final delivery.
- **Validate:** `git status --short --branch`, `git diff --stat`, `git log -5
  --oneline`, `git worktree list`, `git diff --check`; no runtime suite for audit.

#### LR01 — Freeze actual capability and claim inventory

- **Depends:** LR00. **Rules:** B-BR-LR01/LR07; BUC-LR05.
- **Goal/microsteps:** map every route/control to the tutorial catalog; distinguish
  source-present, device-observed, unavailable and planned; reconcile username/key
  account flow, provider gating, photo rules and CS/offline obligations.
- **Files:** tutorial catalog and content contract; relevant Android code read-only.
- **Accept:** every reachable route/control has a chapter; pending helpfulness,
  offline extension, training consent and disabled native provider flows are
  explicitly gated. No content accuracy inferred from string resources alone.
- **Validate:** route-to-chapter and source-link check; privacy/copy review.

#### LR02 — Freeze the information architecture and copy

- **Depends:** LR01. **Rules:** B-BR-LR01/LR05; BUC-LR01/02.
- **Goal/microsteps:** define home anchors, `/como-usar/` hub and chapter slugs;
  revise concise pt-BR copy, availability labels and links; retain existing
  educational `/guias/` pages and legal destinations.
- **Files:** content contract and tutorial catalog. Scope is public app education.
- **Accept:** route inventory, section order and copy are consistent; no orphan
  lesson or duplicated city/keyword pages; no real download CTA before listing.
- **Validate:** source/path/link review; compare copy with current app behavior.

### Phase LR-B — Product art and capture masters

#### LR03 — Revise the visual contract

- **Depends:** LR02. **Rules:** B-BR-LR01/02/05.
- **Goal/microsteps:** use concept composition; keep original logo; specify revised
  type, widths, spacing, surfaces, device frames, buttons and responsive states.
- **Files:** content contract, two existing wireframes or versioned siblings.
- **Accept:** 375px/1440px compositions, declared old-token supersessions, AA
  contrast measurements and clear primary hierarchy; no generated UI as evidence.
- **Validate:** render/inspect wireframes, logo provenance and contrast.

#### LR04 — Prepare a safe capture environment

- **Depends:** LR01. **Rules:** B-BR-LR02; BUC-LR03. **Risk:** privacy.
- **Goal/microsteps:** inspect installed version/build and screen; choose disposable
  emulator for fixture flows; standardize pt-BR, theme, font scale and orientation;
  use original resolution, hide keyboard unless teaching input.
- **Files:** media README/manifest; existing seeders read-only at preparation.
- **Accept:** no user-phone reset, install, DB seed, logout or destructive rights
  flow. Explicitly record unknown installed SHA. Lock screen requires user unlock;
  never infer approval from elapsed time. Restore any changed device preferences.
- **Validate:** screen/privacy inspection before every saved public image; capture
  originals via `adb -s <selected-device> exec-out screencap -p > <output>.png`.

#### LR05 — Capture the public-read marketing set

- **Depends:** LR04. **Rules:** B-BR-LR02; BUC-LR03.
- **Goal/microsteps:** capture Explorar with useful source/date data, city picker,
  city search, station listing, station detail/source separation, community
  filters, ANP analysis and history where actual useful states exist.
- **Files:** `docs/assets/landing/redesign-20261011/screenshots/`, manifest.
- **Accept:** lossless native PNGs, no partial animation/spinner, crisp text, no
  system notification/PII, source/date uncropped. Empty/error states belong to
  tutorial recovery, not marketed as a populated successful feature.
- **Validate:** visually inspect each original at 100%; dimensions/checksums.

#### LR06 — Capture private and conditional feature states safely

- **Depends:** LR05. **Rules:** B-BR-LR02/04; BUC-LR03/05. **Risk:** privacy.
- **Goal/microsteps:** on a disposable fixture environment capture vehicle form,
  tank estimate, guest signup, masked-key account, photo review with synthetic
  owned image, outbox states, feedback/report forms and management when enabled.
- **Files:** screenshots, manifest and capture recipe. No production media.
- **Accept:** no real key/credential/document appears even briefly; fixture
  prices/usernames labelled; source-built but unavailable flows remain pending.
  Never submit real votes/reports/uploads/deletions to create marketing evidence.
- **Validate:** privacy audit and pixel inspection; fixture source/rights record.

#### LR07 — Build the capture manifest and select public assets

- **Depends:** LR05/06 for available states; unresolved rows remain pending.
- **Rules:** B-BR-LR02; BUC-LR03.
- **Goal/microsteps:** select sharp originals; record metadata/checksum/claim class;
  separate marketing, tutorial recovery and rejected candidates; list missing rows.
- **Files:** media README/manifest, scoped capture recipe.
- **Accept:** every selected file is traceable and eligible; no claimed capture
  for unseen screens. A route-wide complete library is blocked by missing states.
- **Validate:** file/dimension/hash integrity and human image review, not OCR alone.

#### LR08 — Export responsive media derivatives

- **Depends:** LR07/LR03. **Rules:** B-BR-LR02/08.
- **Goal/microsteps:** keep PNG masters; produce deterministic downscaled WebP/PNG
  variants (for example 360/720/1080px, capped by original); create CSS/SVG device
  frames separately; assign alt text and image dimensions.
- **Files:** approved `landing/static/assets/app/` derivatives and provenance.
- **Accept:** no stretching/upscaling/AI restoration; legible 1×/2× output, no
  branded unlicensed device templates, transfer-budget fit. Tutorial originals
  stay unobscured; perspective is for promotional compositions only.
- **Validate:** image visual comparison, dimensions, byte sizes, license review.

### Phase LR-C — Rebuild the landing presentation

#### LR09 — Extend the deterministic asset build

- **Depends:** LR08. **Rules:** B-BR-LR08; BUC-LR03/04.
- **Goal/microsteps:** add bounded public asset copying/fingerprinting; register
  chapter routes deliberately; rewrite all emitted references safely.
- **Files:** `landing/scripts/build.mjs`, relevant build tests.
- **Accept:** no private/source/master leakage; nested asset paths work; missing
  media fails loudly; identical inputs reproduce identical output.
- **Validate:** meaningful missing-file/path-traversal/reproducibility tests and
  emitted-reference checks; existing build tests stay green.

#### LR10 — Implement header and hero

- **Depends:** LR03/LR09. **Rules:** B-BR-LR01/05/08; BUC-LR01.
- **Goal/microsteps:** revise header/hero markup, mobile layout and CTA hierarchy;
  keep the opening free of phone mockups; use a strong text/brand composition
  with visible prerelease context and anchors to the separate gallery/tutorials.
- **Files:** home HTML, shared CSS; update affected content assertions meaningfully.
- **Accept:** readable 320–1440px, no clipped logo/H1, one clear product action,
  accurate Play state and source labels, image space reserved to prevent shift.
- **Validate:** typecheck/build/check, content-without-JS tests, visual review.

#### LR11 — Implement the app gallery and feature stories

- **Depends:** LR10. **Rules:** B-BR-LR01/02/05/08.
- **Goal/microsteps:** add Explorar/community/vehicles/source stories from manifest;
  choose static responsive layout; captions explain each screen and its status.
- **Files:** home HTML, CSS and approved media references.
- **Accept:** legible screen previews, meaningful alt/captions, no autoplay
  carousel, no false feature promise, below-fold media lazy-loaded.
- **Validate:** no-JS content, keyboard navigation, image request/size review.

#### LR12 — Implement source, privacy, project and footer sections

- **Depends:** LR11. **Rules:** B-BR-LR01/05/07.
- **Goal/microsteps:** restyle source comparison and concise privacy copy; preserve
  correct legal/contact/GitHub destinations, MIT notices and ANP non-affiliation.
- **Files:** home HTML, shared CSS; legal pages only if a verified copy change owns it.
- **Accept:** source distinction stays explicit, FAQ serves people, no fabricated
  testimonials or collection controls, legal links remain available without JS.
- **Validate:** links, source/claims, content tests, layout on legal/guide pages.

### Phase LR-D — Tutorial engine and complete lesson batches

#### LR13 — Define the tutorial data and annotation contract

- **Depends:** LR01/LR07/LR09. **Rules:** B-BR-LR02/03/04/05.
- **Goal/microsteps:** version lesson/step fields; bind image hashes and normalized
  rectangles to native bounds; define unavailable/recovery and next/back behavior.
- **Files:** tutorial catalog, a small build-time lesson schema/validator.
- **Accept:** unknown images, zero/out-of-bounds boxes, duplicate IDs, missing alt,
  missing results and credential-bearing examples are rejected.
- **Validate:** boundary/malformed data tests before implementation; document
  pixel→normalized→display conversion and `object-fit` offset rules.

#### LR14 — Implement one accessible lesson end to end

- **Depends:** LR13. **Rules:** B-BR-LR03/04/05; BUC-LR02.
- **Goal/microsteps:** implement TU02 city picker with real frames, SVG rectangle,
  numbered text, Previous/Next enhancement and static ordered fallback.
- **Files:** one chapter HTML, shared CSS, small optional TS and focused tests.
- **Accept:** red box tracks exact control at 320/375/768/1440px and 200% zoom;
  keyboard/focus/status correct; JS-off lesson remains complete; no API request.
- **Validate:** invalid coordinates, next/back bounds, focus and image mapping;
  real browser visual review; no implementation-mirroring snapshot-only test.

#### LR15 — Implement the tutorial index and landing teaser

- **Depends:** LR14. **Rules:** B-BR-LR04/05/08.
- **Goal/microsteps:** create `/como-usar/`, group driver/community/account/manager
  lessons; add deep links, estimated steps and availability status; link from home.
- **Files:** hub HTML, home teaser, CSS/optional TS.
- **Accept:** crawlable links, no empty placeholders, planned labels truthful,
  browser back and copyable chapter URL work without JS.
- **Validate:** route/link coverage, keyboard, JS-off content, initial byte budget.

#### LR16 — Author startup, city and discovery lessons

- **Depends:** LR15 and relevant accepted captures. **Rules:** B-BR-LR01–05.
- **Goal/microsteps:** TU01–TU04; one chapter per subtask if needed, exact
  onboarding/manual city/search/filter controls and success/recovery states.
- **Files:** selected chapter HTML/data, capture records only.
- **Accept:** new user can reach useful data without granting optional location;
  zero-result and denied location recover via real available controls.
- **Validate:** each instruction against matching build/frame, overlay inspection.

#### LR17 — Author official analysis, weeks and history lessons

- **Depends:** LR16; TU05–TU07 capture prerequisites.
- **Rules:** B-BR-LR01–05. **Goal/microsteps:** teach geographic/fuel comparisons,
  survey week download/selection, active week, history and missing-week outcomes.
- **Files:** three chapters, image/step metadata (split if >4 owning files).
- **Accept:** no ANP reference described as a current quote; historical data and
  insufficient-history states explicit; no huge download induced by a demo.
- **Validate:** date/source text and each transition; overlay/link/content checks.

#### LR18 — Author station and community browsing lessons

- **Depends:** LR17; TU08–TU10/TU23 captures.
- **Rules:** B-BR-LR01–05. **Goal/microsteps:** stations list, fuel/order/distance,
  station source/condition/unknown/disputed cards, feed filters/pages and public
  station profile/representation status. Include external navigation availability.
- **Files:** selected lesson group, split by chapter; no Android feature changes.
- **Accept:** coordinates/coverage/freshness never overstated; official/community
  cards and station authority are distinct; unavailable Maps/Waze has recovery.
- **Validate:** every stated option actually reachable; offline/error frames honest.

#### LR19 — Author safe account and rights lessons

- **Depends:** LR18; disposable account fixtures. **Rules:** B-BR-LR02–05.
- **Risk:** privacy/auth. **Goal/microsteps:** TU11–TU13, username registration,
  safe key backup/login/export, saved/offline session, logout and deletion preview.
- **Files:** chapters, synthetic capture records; no auth implementation.
- **Accept:** key always masked and synthetic; never ask visitor to share/paste
  key into website; no recovery promise beyond implemented rules; confirm dialog
  differs from server-completed deletion. Native provider appendix gated by proof.
- **Validate:** key/token pattern review and pixel audit; no outbound mutation.

#### LR20 — Author capture, review, send and activity lessons

- **Depends:** LR19; TU14–TU17 captures. **Rules:** B-BR-LR01–05.
- **Risk:** privacy/location. **Goal/microsteps:** permission/post selection,
  photograph/crop/OCR, edit detected fuels/remove subsets, explicit send, queued
  vs received vs refused/partial, My Contributions and available cancellation.
- **Files:** one chapter per subtask, fixtures with owned synthetic images only.
- **Accept:** GPS/mock/radius/accuracy, 24h media and pending validation correctly
  explained from the active version; pending is never shown as published success.
- **Validate:** source/receipt correspondence; no real capture/upload side effect;
  malformed/OCR/permission/offline outcomes each have a truthful path.

#### LR21 — Author participation and moderation lessons

- **Depends:** LR20; TU18–TU21 captures.
- **Rules:** B-BR-LR01–05. **Goal/microsteps:** ratings, fuel-specific comments,
  replies/votes, price confirm/dispute and report reasons; distinguish planned
  contribution HELPFUL/NOT_HELPFUL from current VALID/INVALID feedback votes.
- **Files:** selected chapter/metadata, split by journey.
- **Accept:** sign-in prerequisites, self/duplicate/stale/refused actions follow
  actual rules; report is a request for review, never immediate deletion. TU20
  cannot become available on a domain-only helpfulness implementation.
- **Validate:** actual labels/targets checked against app/backend contract; no
  synthetic votes or reports sent to public/production users for a screenshot.

#### LR22 — Author suggestion and station representative lessons

- **Depends:** LR21; TU22/TU24/TU25 captures and genuine feature availability.
- **Rules:** B-BR-LR01–05. **Risk:** privacy/authority.
- **Goal/microsteps:** suggestion fields/duplicate/status/cancel; claim roles,
  declaration export/signature import/status/cancel; scoped management and reply.
- **Files:** chapter group split as needed; no private claim PDFs in repository.
- **Accept:** requesting representation does not confer verified authority;
  representative replies do not edit official identity/prices/ratings. Unknown,
  expired or revoked grants have clear outcomes; never forge a signature document.
- **Validate:** scopes/labels/status evidence, private-document review, no mutation.

#### LR23 — Author vehicles, notifications and settings lessons

- **Depends:** LR22; TU26–TU28 captures.
- **Rules:** B-BR-LR01–05. **Goal/microsteps:** vehicle add/edit/delete preview,
  tank/fuel/reference, up-to-three limit, estimate; local alerts/OS permissions;
  language/theme/cache/retention/history/radius/sync/week/share if reachable.
- **Files:** one chapter subtask at a time, relevant media metadata.
- **Accept:** tank cost labelled estimate; local price-drop alert not guaranteed
  real-time; deletion/cache clearing shown using disposable data; system settings
  screenshots contain no notifications/account identity and are version-labelled.
- **Validate:** every displayed Settings control mapped; no unseen options invented.

#### LR24 — Author offline, failures and help lessons

- **Depends:** LR23; TU29–TU30 captures.
- **Rules:** B-BR-LR01–05. **Goal/microsteps:** cached ANP/community freshness,
  no local data, queued intentions, reconnect/expiry, help/rules/legal and retry.
- **Files:** two chapters, catalog coverage. Reuse each chapter's failure steps.
- **Accept:** enumerate per-route offline behavior; planned offline city packs,
  durable social queues and training-consent extension stay gated until implemented
  and captured. Never claim the current app works fully offline from this plan.
- **Validate:** cross-chapter no-connection walkthrough on a disposable emulator;
  no actual network disconnection/change on the user's phone without agreement.

#### LR25 — Close tutorial coverage and responsive annotation acceptance

- **Depends:** LR16–LR24; pending capabilities explicitly excluded from active set.
- **Rules:** B-BR-LR03–05; BUC-LR02/05.
- **Goal/microsteps:** map every reachable route/control to a lesson/step; inspect
  target rectangles on original and scaled frames; review guest/account/manager
  prerequisites and all success/empty/error/denied/offline outcomes.
- **Files:** catalog and tutorial evidence; fix affected lessons only.
- **Accept:** 100% of inventoried controls accounted for as available or explicitly
  pending, with zero false active lesson. Full-function demonstration requires
  all targeted actual states; missing capture means MEDIA_PENDING, not complete.
- **Validate:** automated coverage + manual representative/full chapter review;
  screen reader, 200% zoom, keyboard and no-JS walkthrough.

### Phase LR-E — Search engines and AI discovery

#### LR26 — Centralize factual per-page metadata

- **Depends:** LR12/LR15. **Rules:** B-BR-LR06/07.
- **Goal/microsteps:** unique pt-BR titles/descriptions, H1/headings, absolute
  canonical/og:url, social preview/alt, favicons and actual locale; synchronize
  home, legal, existing guides, tutorial hub and chapters.
- **Files:** build metadata source and selected page heads/tests, bounded batches.
- **Accept:** no localhost/staging/private URLs or invented hreflang; display
  lengths are editorial targets, not strict Google ranking rules.
- **Validate:** emitted HTML metadata and duplicate/canonical/path checks.

#### LR27 — Update sitemap, robots and environment guards

- **Depends:** LR26. **Rules:** B-BR-LR06/07.
- **Goal/microsteps:** add only canonical indexable public 200 pages; truthful
  lastmod only when sourced; enforce preview noindex even with existing robots
  meta; preserve public indexability, real 404/redirect status and asset access.
- **Files:** build/check scripts, robots/sitemap, affected indexing tests.
- **Accept:** preview negative cases fail closed; no sitemap for private/404/
  unavailable empty lessons; no robots block used as secrecy or deindex guarantee.
- **Validate:** existing index/follow vs preview noindex regression cases;
  artifact and later real-host header/status inspection.

#### LR28 — Extend factual JSON-LD and image discovery

- **Depends:** LR26. **Rules:** B-BR-LR01/07.
- **Goal/microsteps:** reuse WebSite/WebPage graph, breadcrumbs for chapters,
  image dimensions/alt/captions; connect real GitHub identity; inspect app-schema
  prelaunch/published branches without enabling publication.
- **Files:** selected HTML/metadata generator, JSON-LD tests.
- **Accept:** no rating/offer/business facts copied from Zaflas; no fake app rich
  result eligibility; red tutorial overlay text also present as HTML instructions.
- **Validate:** parsed graph consistency, visible-content comparison, Schema.org
  validation; Rich Results Test only for currently supported eligible types.

#### LR29 — Define public search and training bot policy

- **Depends:** LR27. **Rules:** B-BR-LR06.
- **Goal/microsteps:** audit Googlebot/Bingbot/OAI-SearchBot/PerplexityBot/
  Claude-SearchBot public-read access; keep training consent separate; record
  current official agent names and any current host/WAF challenge conflict.
- **Files:** robots, SEO decision record and focused policy tests.
- **Accept:** genuine search discovery supported; no broad WAF security bypass
  based on spoofable User-Agent; no private endpoint allowance. Training policy
  inherited until explicit owner choice, not silently broadened by reference copy.
- **Validate:** local rule parsing; actual bot acceptance remains external evidence
  until verified; do not mutate shared CDN/firewall/VPS from this task.

#### LR30 — Refresh llms and optional Markdown documentation

- **Depends:** LR28/29/LR25. **Rules:** B-BR-LR01/06/07/08.
- **Goal/microsteps:** refresh existing `llms.txt` using approved copy; link app
  lessons/source distinction/version/status; optionally generate `llms-full.txt`
  or static `.md` chapter companions from one source if a recorded need exists.
- **Files:** llms plus existing build/check tests; no server middleware dependency.
- **Accept:** no contradictions, keys/private URLs or unpublished claims; no
  duplicate content manually maintained in three places. Markdown content
  negotiation is not required; if later adopted, preserve HTML and HTTP Vary/cache.
- **Validate:** content/link parity and missing-reference tests. These files are
  documentation aids, not proven universal indexers or Google ranking factors.

#### LR31 — Plan verified webmaster submission and measurement

- **Depends:** LR27–30. **Rules:** B-BR-LR06/08.
- **Goal/microsteps:** prepare runbook for canonical-domain ownership, Google
  Search Console, Bing Webmaster Tools and optional IndexNow on real changes;
  define index coverage/queries/impressions/clicks/CTR and actual AI referrals.
- **Files:** landing README and SEO runbook/evidence. No external submission now.
- **Accept:** distinguish configured/discovered/crawled/indexed/displayed states;
  avoid obsolete sitemap ping APIs, Google general-purpose Indexing API misuse,
  fabricated traffic estimates or daily unnecessary URL submissions.
- **Validate:** official procedure links and prerequisite review. Do not put
  private verification credentials in source; only provider-required public tokens
  may be exposed with owner authorization. No new recurring automation implied.

### Phase LR-F — Quality and evidence

#### LR32 — Validate responsive presentation and accessibility

- **Depends:** LR25–30. **Rules:** B-BR-LR03/05/08.
- **Goal/microsteps:** inspect 320/375/768/1024/1440px, 200% zoom, long pt-BR
  labels, JS off, reduced motion, keyboard and screen reader; fix actual defects.
- **Files:** affected HTML/CSS/TS, browser evidence.
- **Accept:** no horizontal overflow, serious accessibility defects, clipped
  controls, hidden instructions or misaligned rectangles; visible focus and
  at least 44px touch targets where feasible; useful print stylesheet.
- **Validate:** real browser evidence and scoped automated a11y tool if already
  available; no new dependency solely to tick a checklist.

#### LR33 — Validate performance and asset delivery

- **Depends:** LR32. **Rules:** B-BR-LR08.
- **Goal/microsteps:** record lab tool/version/viewport/throttling; measure initial
  requests and compressed budgets; optimize largest actual culprit; reserve media
  dimensions and prioritize only actual above-fold LCP image.
- **Files:** CSS/media/build/check only where affected; lab evidence.
- **Accept:** home budgets remain met; tutorial chapter default budget ≤500 KiB
  initially with later steps loaded on demand; CSS/JS shared budgets unchanged.
  Lab targets ≥95 for performance/accessibility/best-practices/SEO, with actual
  measurement variability disclosed. No weakened gate to accommodate all PNGs.
- **Validate:** current build/check plus measured browser network/lab results;
  field LCP≤2.5s/INP≤200ms/CLS≤0.1 p75 remains pending without enough real traffic.

#### LR34 — Validate privacy, content and public artifact boundaries

- **Depends:** LR33. **Rules:** B-BR-LR01/02/04/06/07.
- **Goal/microsteps:** pixel-review all public media; audit outbound requests,
  copy/source consistency, private master exclusion, licenses and tutorial demo
  side-effect absence; compare emitted metadata with visible content.
- **Files:** evidence and targeted fixes only.
- **Accept:** zero exposed keys/PII/GPS/production photos, no tracking SDK or
  accidental write API, no build sources/secrets/private assets in dist.
- **Validate:** secret scanner plus manual image review, network inspection and
  build leak/claim tests; a text secret scan alone cannot certify screenshots.

#### LR35 — Run one scoped construction exit

- **Depends:** LR32–34. **Rules:** B-BR-LR08.
- **Goal/microsteps:** run existing typecheck/build/check/tests and affected gate
  selector tests; record exact tree/commands/results/unresolved external evidence.
- **Files:** release evidence and concise PROGRESS.
- **Accept:** no required failed/skipped check, no fake completed chapter, no
  production/Android release acceptance inferred; LOCAL_DONE distinct from
  INTEGRATION_PENDING and INDEXATION_PENDING. Failure gets fixed before closure.
- **Validate:** commands below once for unchanged executable inputs; docs-only
  updates afterward get docs checks, not another full backend run.

### Phase LR-G — Later authorized integration and public follow-up

#### LR36 — Review cumulative history and integrate through maintained dev

- **Depends:** LR35 and later delivery authorization.
- **Rules:** B-BR-LR08; ADR-018/020.
- **Goal/microsteps:** refresh actual main/dev/landing refs; audit all cumulative
  commits; merge reviewed scope into dev normally with ownership coordination;
  resolve conflicts and revalidate affected behavior before protected dev→main PR.
- **Files:** actual conflict owners/evidence only; no synthetic PR IDs.
- **Accept:** commits preserved, unrelated dirty work untouched, current head/base
  and required Quick verification green before guarded finish; no CI polling.
- **Validate:** `bash scripts/git-flow.sh finish --base origin/main --pr
  <actual-number> --required "Quick verification"` only when all guards apply.

#### LR37 — Publish the tested static artifact and verify hosting

- **Depends:** actual integration + separately authorized publication/host access.
- **Rules:** B-BR-LR06/08.
- **Goal/microsteps:** deploy immutable checked dist; verify TLS/canonical
  redirects/headers/cache/real404/previews and public asset URLs; retain rollback.
- **Files:** publication evidence/runbook; host operations only within authorization.
- **Accept:** published SHA/artifact hash known; preview and production indexing
  distinct; no API/private-media exposure; store state remains factual.
- **Validate:** real HTTPS responses, browser screenshots and rollback availability.

#### LR38 — Submit verified discovery signals

- **Depends:** LR37 + authorized ownership/admin actions.
- **Rules:** B-BR-LR06/07.
- **Goal/microsteps:** verify public property, submit sitemap in Search Console/
  Bing; optional IndexNow after proving correct key and changed URLs; inspect
  actual canonical page and bot accessibility, not only robots source.
- **Files:** sanitized SEO evidence. No DNS/console mutation without authority.
- **Accept:** submission receipt is not indexation, bot allow is not a citation,
  IndexNow is not Google submission; Bing/Copilot and direct AI search measured
  separately. No promises of ranking, instant recrawl or complete engine coverage.
- **Validate:** actual console/response evidence; unresolved discovery remains pending.

#### LR39 — Record useful postpublication evidence and maintain chapters

- **Depends:** LR38 and genuine elapsed public data, never a fixed wait loop.
- **Rules:** B-BR-LR01/06/08.
- **Goal/microsteps:** when data exists compare coverage/queries/CTR and real AI
  referrals; sample citations without claiming exhaustive coverage; invalidate
  screenshots/rectangles when app controls/build change; mirror merged docs once.
- **Files:** sanitized evidence, capture manifest and affected chapter updates.
- **Accept:** honest date range/source/sample limitations; WIKI_PENDING only if
  sync failed; no arbitrary growth/rank claim. No recurring monitor created by plan.
- **Validate:** data provenance and updated capture mapping; recheck only affected
  code/assets, never rerun backend suites because wiki/Google is still pending.

## Commands and acceptance cadence

Run from repository root; inspect actual current script interfaces first:

```sh
git status --short --branch
git diff --check
npm --prefix landing ci
npm --prefix landing run typecheck
npm --prefix landing run build
npm --prefix landing run check
npm --prefix landing test
bash scripts/tests/test-gate-selection.sh
bash scripts/scan-secrets.sh
```

`npm ci` is environment setup once, not before each microtask. Use targeted
`node --test` files during a task; run the specialized union once at LR35. No
backend/PostGIS or full Android suite for static/docs/media-only changes. If a
mobile regression is discovered, record it with its owning evidence; fixing
unrelated app behavior is not authorized by this landing plan. Critical changes
introduced later still require immediate owning negative/failure/concurrency tests.

## SEO research and adaptation record

Read 2026-10-11; primary sources only. Reverify changing guidance at execution.
Do not claim this plan installed every possible indexer.

- [Google AI features](https://developers.google.com/search/docs/appearance/ai-features):
  ordinary SEO/indexability remains the foundation; no special AI schema/file is
  required. Crawl/index/display are not guaranteed. Prefer this narrow guide over
  speculative AEO tactics and unverified search snippets.
- [Google robots](https://developers.google.com/search/docs/crawling-indexing/robots/intro):
  robots controls crawling, not authorization. Preview noindex must be observable.
- [Structured data](https://developers.google.com/search/docs/appearance/structured-data/intro-structured-data):
  markup supports understanding and must describe real content; eligibility does
  not guarantee enhanced display.
- [Google image guidance](https://developers.google.com/search/docs/appearance/google-images):
  use discoverable HTML images with contextual text, useful alt and responsive
  assets; keep masters separate from website delivery size.
- [Google search updates](https://developers.google.com/search/updates) and
  [HowTo change](https://developers.google.com/search/blog/2023/08/howto-faq-changes):
  deprecated rich-result features must not drive the tutorial design. The current
  FAQ documentation URL redirects to updates; recheck supported features rather
  than copying historical FAQ/HowTo eligibility from Zaflas or old plans.
- [OpenAI crawlers](https://developers.openai.com/api/docs/bots): OAI-SearchBot is
  search discovery, GPTBot is training, ChatGPT-User is user-initiated fetching.
  Each has a different purpose; a training allow is not needed for search allow.
- [Perplexity crawlers](https://docs.perplexity.ai/docs/resources/perplexity-crawlers):
  distinguish PerplexityBot from Perplexity-User; verify real infrastructure access
  using official current documentation, without a generic security bypass.
- [Anthropic crawler policy](https://support.claude.com/en/articles/8896518-does-anthropic-crawl-data-from-the-web-and-how-can-site-owners-block-the-crawler):
  distinguish Claude-SearchBot/Claude-User from training ClaudeBot.
- [Bing sitemap discovery](https://blogs.bing.com/webmaster/2025/7/Keeping-Content-Discoverable-with-Sitemaps-in-AI-Powered-Search/)
  and [IndexNow](https://www.indexnow.org/documentation): accurate sitemaps and
  change notifications assist discovery; only ownership-verified public URLs,
  with no guaranteed inclusion or ranking and no Google IndexNow claim.
- [Core Web Vitals](https://developers.google.com/search/docs/appearance/core-web-vitals):
  distinguish field user measurements from local lab diagnostics.
- LR28 re-verified 2026-10-11: OpenAI crawler docs still separate OAI-SearchBot
  (search answers, independent opt-out) from GPTBot (training) and ChatGPT-User
  (user-triggered, robots may not apply); a 2026 user-agent table attests
  ClaudeBot (Anthropic training) and Perplexity-User (on-demand agent); IndexNow
  serves Bing/Yandex/Seznam/Naver while Google declines it and requires key
  ownership. Implemented in `robots.txt`: search allowed, GPTBot/ClaudeBot
  disallowed, fetchers listed for transparency. Submissions stay pending owner
  action.

Adopt from Zaflas: centralized factual head fields; stable/deduplicated entity IDs;
route-aware metadata tests; sitemap/robots/llms consistency; optional clean Markdown
representations. Do not copy its visual design, runtime/framework, admin paths,
company identity, language routes, profiles, rating counts/offers or deployment
rules. Its source implementation is not measured evidence of professional ranking.

## Current preparation evidence and remaining work

This document is the requested plan, not LR implementation acceptance. The first discarded concept used the built-in image tool. The selected reference
is browser-rendered HTML/CSS with actual app PNGs; its complete source, rendering
recipe and capture provenance are in the media package. Planning checks cover links, task IDs,
route inventory, files, diff and secret surface. The explicitly requested local font integration is the only current website
code change; no redesign/tutorial implementation, runtime dependency, backend
behavior, deploy, issue/PR, sitemap submission or wiki publication is performed
by this planning/media task. The separate visual prototype is not production HTML. First execution action: LR00 in branch `landing`.

# Execution prompt — MAP-00–33

Current continuation: read [MAP_EV_RESUME](MAP_EV_RESUME.md) first. MAP-14
ENGINE_GATE passes for the versioned local extraction matrix; the persistent
admitted artifact is ready. Close retained MAP-15 integration duties and then
implement the real MAP-17 screen. Do not reopen resolved budget/linker/owner
stamp requests or confuse static builder metadata with the runtime receipt.

Paste the following into the implementation agent only when you want construction
and source delivery to begin. This planning turn does not execute the instructions.
The human-facing Portuguese companion contains the same scope and safeguards.

---

Implement the approved Abastevo fuel/electric map batch sequentially, MAP-00–33.
Use the complete local plan, never this prompt alone:

- docs/planning/MAP_EV_EXECUTION_PLAN.md
- docs/planning/MAP_EV_PHASES.json
- docs/planning/MAP_EV_SOURCE_REGISTER.md
- docs/assets/map-icons/README.md

If these files are in an isolated planning worktree rather than maintained dev,
locate that attached worktree, inspect its diff/base and integrate only the scoped
planning/artifact commit normally. Do not copy unrelated changes or lose later
commits. This plan was audited at b90a0cd1; the actual starting HEAD may be newer.

## Objective and limits

Deliver a professional Android map for fuel and electric stations, with bottom
navigation exactly Explorar | Comunidade | Mapa | Perfil. At the map top, ANP /
Comunidade selects the price source. A station marker opens a quick sheet with
source/date/product/unit prices, profile and route actions. Complete offline maps,
search, driving routes/guidance, private favorites/trips and EV stop/alternative
planning in the later phases. Reuse current visual standards and existing modules.

Only admit open-source/open-data resources permitting the actual commercial,
modified, offline and redistribution use. The committed architecture is an
Abastevo-owned map: vendor the pinned CoMaps-derived native C++ core at an exact
SHA, maintain a small documented patch set, build our own reproducible AAR and
expose a narrow `MapEngine` Kotlin port. Abastevo owns the Compose UI, app identity,
station identity, prices, community data and map style; the native core owns only
render/search/route. Do not replace the app with CoMaps, ship its UI/brand/CDN,
assume a drop-in SDK exists, add a WebView map as a shortcut or silently substitute
a second renderer. Preserve com.anpfuel, MIT and all third-party notices. Never
relicense OSM data as MIT; honor ODbL obligations and visible credit.

On receiving this implementation prompt, construction, meaningful local commits,
normal dev backup pushes and the final scoped protected dev -> main PR/guarded
merge are requested, subject to actual repository permissions/checks. Do not push
product changes to main directly. No VPS/production deployment, purchases,
external operator contact, accounts, public OSM writes, physical phone changes or
Play publication is included. You may use an owned disposable Android emulator
and disposable local PostGIS/storage for meaningful tests; provide a manual phone
checklist to the user. Android Auto host/certification and real staging/production
proofs stay explicit external obligations unless separately authorized.

## Start with evidence, then execute

1. Read repository AGENTS.md, docs/planning/FAST_EXECUTION.md, current PROGRESS,
   DELIVERY_WORKFLOW, selected ROADMAP scope and the four files above. Reconcile
   EE/MEX completed implementation and open obligations with actual code.
2. Inspect git status/diff/history/worktrees/origin. Use maintained dev synchronized
   normally from main. If occupied, isolate and preserve all other changes; do not
   stash/reset/force/rebase published history or develop in an old detached clone.
3. Record current source, deployed source and test/device evidence separately.
   Verify actual schema/Room versions, Gradle/Cargo targets and existing modules.
   Do not infer live readiness from old LOCAL_DONE labels.
4. Select the first phase lacking verified acceptance. Read every step, required
   check and exit. Execute one bounded task at a time. If a phase is broad, split
   it into MAP-NN-T01/T02/T03, retaining the phase's full acceptance obligation.
5. Do not start Android map functionality before MAP-13 DATA_GATE. Execute MAP-14
   as the committed extraction: pinned/vendored source, owned AAR, `MapEngine`
   port, real render/search/route and Abastevo overlay selection, licenses and
   measured budgets. The minimum usable map is MAP-20; it does not complete later
   offline/navigation and EV planning phases.

## Per-task execution loop

Document B-BR/BUC and risk -> reproduce a meaningful failing behavior -> implement
smallest correct change -> run targeted tests and immediate critical negative/
failure/concurrency tests -> compile/static/format -> git diff --check and secret
review -> write truthful evidence -> atomic English commit -> authorized normal
dev push -> continue without waiting for PR/CI between phases.

Use docs/backend/TEST_STRATEGY.md and backend/README.md for actual commands/pinned
tools/disposable DB variables. SQL/migration/auth/money/privacy/jobs require real
PostGIS/failure/ownership/concurrency proof immediately. Compile-only commands,
mock engines, skipped integration and APK assembly are not behavior acceptance.
Do not run the entire aggregate suite after every tiny change; run affected tests
now and the appropriate union at the declared final gates.

Evidence must name task/rules, tested revision/tree/artifacts, real commands,
counts/outcomes, environment, negative cases, unresolved limitations and next task.
Only use Status: LOCAL_DONE and Validation: PASS when that phase's required local
exit actually passed. INTEGRATION_PENDING, INTEGRATED, EXTERNAL_PENDING,
WIKI_PENDING and RELEASE_CERTIFIED remain distinct. Keep PROGRESS concise and
link phase evidence; preserve history. Never manufacture a test count or remote ID.

At a context boundary, leave current task/head/commit/tests/pending/next-action
notes so the next turn resumes without repeating completed work. Continue the
next authorized phase autonomously. Difficulty is not a reason to replace real
features with placeholders. Stop dependent work only for a demonstrated required
blocker; give exact evidence and a concrete bounded recovery proposal, while
finishing independent authorized work.

## Rules you must not violate

- Source acquisition belongs to Go; Rust parses local deterministic admitted
  artifacts without network/DB. Reuse existing producers, profiles and native
  bridges. CoMaps map generation uses a separately pinned native build tool, not
  a handwritten Rust implementation of its format.
- Public map files and prices have different generations/updates. Do not rebuild
  road maps for every price change. Native resources must match engine format,
  world/region/neighbor dependencies and styles/search/routing assets.
- Review exact upstream SHA/third-party licenses/services before copying code.
  Prove JNI/build/lifecycle/custom POI overlays on a real engine. Missing support
  requires a recorded architecture revision, not silent MapLibre/Google fallback.
- Preserve canonical station UUIDs, CNPJ strings and charging site/equipment
  identities. Proximity alone cannot merge stations, prices or access entrances.
- Missing/city-centroid locations are not exact pins. Approximate reference
  locations and reviewed entrances must be distinguishable.
- ANP station price must be an individual matching station fact for the selected
  product/period. No municipal average, wholesale price, other fuel/unit or
  community value disguised as ANP. Unknown price never means zero/free.
- ANP and Comunidade switches update pins, sheet/list/cache together and cancel
  stale results. Charging points remain visible in ANP mode with an explanation
  that ANP has no electric tariff and an explicit community-switch action.
- Keep connectors/power/access/hours/price/date/status independent and unknown
  where absent. A static OCM point or old community report is not live availability.
- Signed writes/private owner reads reuse real account proof. Public discovery
  remains anonymous. No public contributor GPS/home/routes, hidden telemetry,
  automatic tracking, paid trust or paid community participation.
- Use Room migrations and atomic downloads/imports; no destructive fallback.
  Verify authenticated manifest and digest, limits, paths, neighbor dependencies,
  restart/disk-full behavior and active-file deletion races.
- Use Android platform location and local voice fallback, not a new proprietary
  location/cloud voice dependency. Browsing works without GPS permission; active
  guidance has explicit start/stop and correct foreground-service behavior.
- Respect app theme, existing sheets/cards/typography/spacing and shared
  AnpContainerDefaults. Preserve 48dp targets, visible labels, accessible result
  list, 200% text and narrow-screen behavior. No complete app redesign.
- Use the supplied map pictograms/provenance. They are station-type icons, not
  new fuel-grade enums. Do not replace existing fuel icons or blindly recolor
  them. Keep SVG masters out of oversized Android VectorDrawable strings; use
  the prepared PNG pipeline where appropriate.
- EV routing uses road distance, useful energy, consumption, reserve, compatible
  stops, access and alternatives. Explain missing data/no reachable backup;
  never claim a guaranteed arrival or invent exact charging times.
- Do not admit closed/noncommercial-only feeds just to finish. Manual vehicle
  input and open reference discovery must work without PBE/manufacturer/operator
  access. Optional inputs remain optional, with honest source coverage.

## Finishing the source batch

At MAP-33, reconcile the scope and every phase exit once; inventory open PRs to
avoid losing scoped commits. Do not merge unrelated PRs merely because open.
Reconcile current main normally, revalidate affected changes, and open/reuse the
cumulative protected dev -> main PR after source acceptance. Fix CI/review issues
with new commits and inspect the actual current head/base and all applicable
checks. Required missing/failed/skipped/cancelled checks block merge; changed
head/base invalidates previous evidence. No admin/force/no-verify bypass.

When all guards actually pass, use the repository helper with the real PR:

bash scripts/git-flow.sh finish --base origin/main --pr ACTUAL_NUMBER --required "Quick verification"

ACTUAL_NUMBER is a placeholder; discover and substitute the verified PR number.
Never execute a made-up command/ID. Keep dev and verify all scoped commit ancestry
on main. Wiki publication follows its separately authorized allowlist once per
merged batch. Do not wait/poll CI during construction or create bookkeeping PRs.
Pending final CI is CI_PENDING, not a reason to pretend merge succeeded.

Final report in Portuguese: phases and actual status, commits/PR/check head/base,
implemented journeys, executed tests/native/manual evidence, admitted sources/
rights/coverage, artifacts, remaining external obligations and exact phone test
instructions. Do not call the app nationally complete, operationally certified or
production-ready from source merge alone. Begin MAP-00 now and continue in order.

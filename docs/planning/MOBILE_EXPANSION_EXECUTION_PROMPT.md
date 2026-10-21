# Execution prompt — mobile vehicles, performance and ANP discovery

**Selected extension:** use the [combined MEX/EV prompt](EV_SUPPORT_EXECUTION_PROMPT.md)
and [EE plan](EV_FULL_SUPPORT_EXECUTION_PLAN.md). The latest human request adds
BEV/PHEV/hybrid, Rust source preparation, community charging prices and Android
offline/private cost support. The original prompt below is historical base scope;
do not execute it alone while ignoring the EE dependency splice.

Submit the following prompt to the implementation agent. It selects local source
construction, not remote publication, source activation or commercial release.
User communication is Portuguese; repository code/docs/commits remain English.

---

Implement the complete authorized local scope of
docs/planning/MOBILE_EXPANSION_EXECUTION_PLAN.md, following
docs/product/MOBILE_EXPANSION_CONTRACT.md. Deliver working Android plus required
backend contracts/integration, not another plan or disconnected screen mockups.

Repository: /home/iiii/PESSOAL-PROJETOS-ALEXANDRE/brazil-fuel-prices.
The planning patch was prepared in .worktrees/mobile-expansion-plan on
codex/mobile-expansion-plan from dev fb85d4d1. Those are historical context:
inspect actual worktrees/commits before acting. The plan may need integration
from that branch before it is available on the current maintained head. Never
replace newer dev with the old planning base. Merge the planning commit normally
when safe and revalidate changed dependencies; preserve every other front.

## Start here

1. Read applicable AGENTS.md, docs/planning/FAST_EXECUTION.md, current
   docs/planning/PROGRESS.md and docs/planning/DELIVERY_WORKFLOW.md. Then read the
   selected MEX task, its rules/use cases and owning PRM/FQ/OPT source/tests.
2. Inspect status/diff/worktrees/log before editing. Use maintained dev only if
   safely available; otherwise use an isolated codex/ worktree from a verified
   committed head, following repository reconciliation rules. Do not stash,
   reset, overwrite, commit or delete another effort's work. Capture the real
   starting head and existing dirty paths in MEX-00 evidence.
3. Execute MEX-00 first. Reconcile actual existing implementation/evidence with
   the plan. Do not redo accepted PRM/FQ/OPT work or trust stale headers alone.
4. Execute one bounded task at a time in plan order. MEX-20 is a container:
   execute MEX-20a through MEX-20f as separate tested checkpoints. Its parent
   cannot pass early. Split any other oversized task before coding with explicit
   child acceptance and preserved parent dependencies.

## Required product outcomes

- One new free vehicle; additional vehicles require verified Premium. Keep
  existing technical ceiling three unless a later explicit amendment changes it;
  do not market unlimited slots. Preserve legacy vehicles/history and free
  retained-data exit. The owner changed slots only: preserve existing PRM paid/
  free rules for professional tools. Do not invent prices, manual grants or
  payment-provider configuration.
- Complete actual refuel/expense/odometer Android journeys, valid measured
  consumption versus configured estimate, cost/km, monthly reports and comparable
  periods using existing exact Go/Kotlin methodology. Unknown/incomplete data
  remains labelled; do not invent travelled distance or profit.
- Complete allowed Premium sync/backup/restore/device/conflict consumers with
  explicit consent, durable owner/origin-bound commands and actual server
  acknowledgements. A local save is not synced and pending edits are not backed up.
- Optimize local state/city lookup without full-catalog fuzzy work on each
  keystroke, N+1 enrichment, mandatory network or imported-week dependency for
  finding a location. Preserve unknown/no-price municipalities and minSdk.
- Add typed national price exploration and supported natural-question intents.
  Use visible interpretation/source/period/coverage, exact indexed bounded
  queries and measured reuse. No generative AI/search cluster needed by default.
- Add a national ANP source-record explorer AND safer station-list/detail
  occurrence filters. Keep assays, enforcement actions, explicitly reported
  infractions and fine records separate. Preserve unmatched source records.
  Current-company association is not proven historical site occurrence. No
  source-global fallback, fuzzy identity match, guessed fuel/status/guilt or
  "no infractions" claim from missing data.
- Filters run in the backend before pagination, with complete scoped traversal;
  do not filter just the phone's downloaded stations. Distinguish record/station/
  company counts and missing-source coverage. Preserve multiple distinct results
  sharing document/procedure IDs and do not count snapshots as independent fines.
- Complete missing FQ read acceptance, scoped HTTP/root/cursor/ETag wiring,
  actual enforcement XLSX pipeline and conservative Directory revalidation before
  exposing dependent mobile filters. Reuse owning FQ tasks, not parallel ledgers.
- Reconcile the other agent's OPT-A work for persistence/event-based refresh and
  remove fixed "every 15 seconds" copy across affected locales. Preserve manual
  refresh, useful source/age information and pagination windows.

## Non-negotiable safety and correctness

Use the project's architecture/design system, existing modules/ports and pinned
dependencies. Migrations are append-only with real upgrade/recovery acceptance.
Do not add services or dependencies without documented need/license/security.
Private financial data never enters public rankings, official counts or caches.
Do not commit raw source names, CPF, secrets, customer data or production photos.
No payment-based trust/removal. No arbitrary score thresholds or fabricated case
links/dates. No D receipts/private proof/business portal, iOS, trip tracking,
fleet/income/tax service, paid activation or production rollout.

Use actual source profile bytes and independent census for real enforcement
acceptance; passing synthetic adapter tests is insufficient. For company-scoped
reads prove lock behavior with observed concurrent writers, cancellation and
realistic new-query EXPLAIN. A race-detector run alone does not prove SQL locks.
ETags/cursors bind full filters, source revision vectors and current association
fingerprints; no stale station-scoped 304 after retirement/outage. Offline cached
association is explicitly last-checked, not currently reverified.

## Finish and commit every task

For EACH execution unit:
1. Record B-BR/BUC, expected behavior, risks, exact acceptance commands and rollback
   before behavior changes. Use RED → GREEN → REFACTOR for domain changes.
2. Implement only this unit. Reuse accepted existing code without duplicate work.
3. Run meaningful targeted tests. Critical auth/money/privacy/SQL/migration/jobs
   require immediate negative/failure/concurrency and real PostGIS/SQLite checks.
   Test the affected wire contracts and actual integration, not only mocks.
4. Run affected compilation/format/static checks, git diff --check and changed-file
   secret/PII review. Fix known failures; never weaken tests, accept zero selectors
   or use compile-only as behavioral acceptance.
5. Record tested tree/revision, commands/results, source/config, scope, limits and
   next action in docs/backend/evidence/mobile-expansion/MEX-NN.md (child suffixes
   for MEX-20a…f); update current PROGRESS concisely with links.
6. Review staged paths to exclude all foreign edits. Make one meaningful atomic
   English Conventional Commit for the completed unit, then continue from it.
   Only full local acceptance gets LOCAL_DONE / INTEGRATION_PENDING and PASS.

Do not stop after the first phase unless an actual dependency blocks further
authorized work. Record external blockers precisely and continue independent
units; do not fake provider/device/source proofs. Do not run all backend/Android
release suites after every small change. Run affected checks per task and the
combined final local acceptance once after reconciling actual inputs.

No per-task PR/CI/merge waits, CI polling, fixed sleeps or wiki updates. This
prompt authorizes LOCAL implementation and commits only: do not push, open PRs,
merge main, deploy, enable imports/payments, install/clear the user's phone or run
held native acceptance without separately valid explicit authority. The plan
describes guarded final delivery for a later expressly selected publication step.

## Final response in Portuguese

Report what now works, actual task/commit mapping, tested commands/results,
before/after measured performance and exact pending external obligations. Explain
free/Premium limits and retained-data behavior, national coverage and company/site
attribution. Separate implemented source from integrated/deployed/release states.
Do not claim the whole app ready or complete if any required local task remains.

---

The human can separately authorize protected final remote delivery after reviewing
the local result. Never infer that authorization from this instructional document.

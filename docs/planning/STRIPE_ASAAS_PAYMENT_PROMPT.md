# Implementation prompt — Stripe cards and Asaas Pix

Prepared 2026-10-24. Paste this prompt into the implementation agent when
starting construction. The current planning turn performs none of these actions.

---

Implement Abastevo PAY-00–PAY-23 sequentially. You own backend web payments:
Stripe for cards, Asaas conventional Pix and Pix Automático, Premium and a
defined single-payer Enterprise offer. Both Pix modes are required and have
independent activation. Google Play is a future adapter, not a dependency for
Stripe/Asaas. Use Go and the existing PostgreSQL architecture; no Java rewrite.

## 1. Read before editing

Locate the planning worktree at:
`/home/iiii/.codex/worktrees/stripe-asaas-plan/brazil-fuel-prices`.
Read these current repository files:

- AGENTS.md.
- docs/planning/FAST_EXECUTION.md.
- docs/planning/PROGRESS.md, current version only.
- docs/planning/DELIVERY_WORKFLOW.md.
- ROADMAP.md, Stripe and Asaas payments section.
- docs/planning/STRIPE_ASAAS_PAYMENT_PLAN.md.
- docs/planning/STRIPE_ASAAS_PAYMENT_PHASES.json.
- docs/planning/STRIPE_ASAAS_PROVIDER_REGISTER.md.
- docs/product/STRIPE_ASAAS_PAYMENT_CONTRACT.md.
- docs/planning/MAP_EV_EXECUTION_PLAN.md, ownership/dependency scope.
- docs/planning/ENTERPRISE_DATA_STATIC_PORTAL_PLAN.md, ownership scope.

Then read the current phase's exact code/tests and applicable existing PRM/FQ
evidence. Do not treat an old SHA, plan or passing compile as runtime readiness.
If planning files are not on dev, preserve/integrate only their scoped planning
commits through normal history. Do not copy an entire old worktree over new code.

## 2. Authorization when this implementation prompt is invoked

Implement the named backend scope, execute meaningful local tests, commit owned
work and push your exclusive branch as backup. At batch closure, coordinate dev
integration, create/reuse the protected dev → main PR, fix checks and merge only
through the actual passing guards. Prepare and validate an Abastevo-only VPS
TEST/SANDBOX deployment when the required access/environment is supplied and its
scoped topology is verified. Never charge/refund real money from this prompt.

No unrelated MAP/ESP implementation, native Android/iOS UI, provider enrollment,
paid services, DNS change, other hosted app, global cleanup or shared-edge scope
expansion. Public purchase UI/links in a Play-distributed app require a verified
policy/program decision; web backend work can proceed independently. Enterprise
payments do not authorize teams/tenants/station claims or deferred Stage D.

## 3. Isolate from the map and static portal

Start PAY-00 by inspecting status/diff/history, worktrees and actual dev/main
refs. Identify MAP, ESP and existing Premium owners. Use an exclusive payment
worktree and branch, suggested codex/payments-stripe-asaas. Never move occupied
dev, stash/reset others, force push or rewrite published commits.

Payment-specific code/tests and docs/backend/evidence/payments are your scope.
Leave Android map/CoMaps/charging, Directory/Official/Rust ingestion and static
landing consultation/export product changes to their respective owners.

Before changing ANY shared path, declare it and coordinate exclusive ownership:
subscription access, API/worker composition, OpenAPI, migrations/sqlc, Go modules,
CI, Caddy/Compose, ROADMAP and PROGRESS. A worktree or clean status alone is not
a lock. Use the actual agreed coordination mechanism; do not invent a lock the
other agents do not honor. If coordination is absent, finish independent code
and record the shared dependency rather than editing concurrently.

Reserve migration numbers only at a coordinated fresh-head boundary. Never
edit applied migrations or overwrite another workstream's generated outputs.
Revalidate semantic consumers after integration even when Git reports no conflict.
Do not merge unfinished MAP/ESP code to force payment delivery.

## 4. One phase, then one commit

Use the JSON registry as an ordered checklist. Start PAY-00 and advance in
numeric order, checking each depends_on. Do not spawn subagents or implement
several phases together. If a phase is too large, define sequential child phases
and their acceptance before coding; do not silently drop parent obligations.

For each phase:

1. State phase/objective, dependencies, exact owned/shared files, B-BR/BUC and risk.
2. Discover real test/build commands from current code and backend/README.md.
3. Reproduce a meaningful failing test for changed behavior.
4. Implement the smallest correct behavior using existing owners and ports.
5. Run targeted positive, negative, failure and concurrency checks immediately.
6. For SQL/migrations/money/jobs/auth/privacy, run real disposable PostGIS and
   actual relevant process/recovery tests now, not at the end of the batch.
7. Run affected compile/format/vet/sqlc/contracts, git diff --check and sensitive-
   surface review. Never delete/weaken/skip a known failing required test.
8. Write docs/backend/evidence/payments/PAY-NN.md with actual commands/results,
   tested tree/artifacts, failure cases, remaining obligations and next action.
9. Append a compact PROGRESS note without losing concurrent records. Commit
   only owned code/tests/evidence in English using the registry topic.
10. Continue the next accepted dependency without PR/CI waits during construction.

Do not mark LOCAL_DONE/PASS when required checks are missing. At interruption,
leave phase, branch/head, tests, uncertain operations and exact next action.
Difficulty is not permission to add placeholders or a deployable fake provider.

## 5. Essential implementation rules

- Reuse subscription/account/jobs. Existing Configured() and access currently
  assume Play ports; web payments must not require Play secrets or fake tokens.
- Catalog prices/BRL minor units/terms/capability target come from the server.
  Never trust client amount, customer ID, paid status or entitlement. Do not
  invent real prices or sell unimplemented Enterprise capabilities.
- Separate order, remote attempt, settled payment, contract, cycle, mandate,
  refund/dispute and source-specific grant. QR creation/redirect/checkout
  completion is not payment evidence. Unknown states grant nothing.
- Stripe uses hosted Checkout and maintained SDK/raw-body signature verification.
  No card PAN/CVV in application handlers, logs, tests or storage.
- Asaas webhook authentication is its separate authToken header, not the API
  key and not an invented HMAC signature. Verify remote resource bindings.
- Authenticate, persist inbox, then acknowledge. Asaas requires HTTP 200.
  DB failure cannot return success. No provider calls in the callback path.
- Use durable intents and local operation/cycle uniqueness. Stripe provider
  keys have limited retention; Asaas externalReference is not guaranteed unique.
  After timeout or lost reply, reconcile before retrying. If remote absence
  cannot be established, keep OUTCOME_UNKNOWN and escalate reviewed recovery.
  Never blindly repeat charge/customer/mandate/subscription/refund creation.
- Initial Pix payment can succeed with a refused automatic authorization.
  Do not debit future cycles before authorization is ACTIVE. Select exactly
  one cycle scheduler; do not confuse automatic MANUAL mode with conventional Pix.
- Permit only one recurring funding source per contract/product family. Safely
  fence/cancel the old source before switching; uncertainty blocks a second
  auto-debit. Duplicate money does not mean duplicate benefits.
- Refunding only the original payment route requires bounded refundable balance,
  fresh restricted authorization, reason/audit and confirmed provider outcome.
  Handle partial refunds, chargebacks, MED and unmatched account holds explicitly.
- Network I/O holds no long DB locks. Use bounded jobs/queries/retries/quotas and
  preserve safety/map workloads. Access reads use local verified state under
  defined freshness; no provider query at every page/view.
- Disabling new sales preserves maintenance, callbacks, cancellation, legitimate
  settled access and free exits. Deletion/restore cannot replay financial writes.
- No secret/customer PII/raw QR/payment URLs in Git/logs/artifacts. Use reviewed
  retention, encryption/rotation and test/live merchant isolation.
- Test owner isolation, forged requests, altered amounts, replay, parallel calls,
  reordered events, lost replies, crashes, stale leases, key rotation, expiry,
  refunds and restore. Follow the complete risk matrix; do not claim all abuse
  is impossible or replace real external proof with mocks.

## 6. Configuration and external proof

Build fully configurable code, safe example configuration, read-only preflight
and exact account/catalog/webhook setup instructions. API keys alone cannot
replace provider account approval, price/capability mapping, callback registration,
Pix Automatic eligibility, fiscal decisions or sandbox acceptance.

Keep all three sale channels independently off by default. Secret presence alone
must not enable live charging. Maintenance must remain operable when new sales
are off. Do not automatically create live products/webhooks/customers on startup.

Complete independent local work through PAY-20 without credentials, using test-
only fixtures and real local DB/process tests. At PAY-21, use actual securely
supplied Stripe test-mode and Asaas Sandbox access. Verify hosted card/SCA,
manual Pix, automatic Pix consent/cycles, callback, access, renew/cancel/refund
and recovery as supported. Missing access or unsupported cases remain PROOF_OWED.
Do not use real money to replace unavailable sandbox simulations.

## 7. Final PR, VPS and reporting

After actual required acceptance, coordinate dev/main and other workstreams,
run the necessary final affected suite once, open/reuse the protected dev → main
PR, fix failures in new commits and verify current head/base/all applicable checks.
Use the repository helper with the verified actual number:

bash scripts/git-flow.sh finish --base origin/main --pr ACTUAL_NUMBER --required "Quick verification"

ACTUAL_NUMBER is a placeholder. Missing/failed/skipped/canceled required checks
block merge. No admin bypass, force push or no-verify. Keep dev. If dev contains
unfinished unrelated MAP/ESP work, coordinate a safe accepted integration point;
do not silently include it. Wiki uses its existing separately owned scope.

PAY-23 deploys the pinned merged artifact in the named Abastevo test environment
after verifying scope/access/backup/rollback and a coordinated deployment window.
Test real HTTPS/provider callbacks and full financial/access/cancel/refund flow,
not just /health. Preserve other hosted services and app/map/static-site smoke.
Simulate outages only in owned isolated environments. Do not mutate shared edge
beyond the authorized diff or run unbounded/live-money load.

Keep production sales disabled pending actual commercial/account/distribution
gates and explicit live-money scope. If provider proof is missing, report it
precisely and preserve completed local work; never declare the entire payment
backend commercially ready. Provide exact remaining owner steps, not “add keys”.

Communicate in Portuguese. Code/docs/commits are English. Continue autonomously
within this authorization; ask only for genuinely missing decisions/access or
uncoordinated shared scope. After each phase report ID, commit, tests, status,
remaining dependencies and next action. Begin PAY-00 now.

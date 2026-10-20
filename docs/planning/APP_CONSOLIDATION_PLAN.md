# App consolidation: offline efficiency, community reliability and economy

Status: PLANNING_LOCAL_DONE / INTEGRATION_PENDING, 2026-10-14.
Implementation: A/B/C PLANNED; D DEFERRED_FUTURE / NOT_AUTHORIZED.
Authority: explicit maintainer request to plan A–D, with future-only D.

## Outcomes and sequence

1. [A — Home offline and efficiency](HOME_OFFLINE_EFFICIENCY_PLAN.md):
   preserve useful data, persist city summaries, refresh deliberately and reduce
   repeated transfer/computation.
2. [B — Community reliability without mandatory GPS](COMMUNITY_RELIABILITY_PLAN.md):
   improve existing media, independence, consensus, anomaly and reputation rules.
3. [C — Economy and retention](ECONOMY_RETENTION_PLAN.md):
   useful alerts, observed history and estimated net savings after a detour.
4. [D — Future evidence and business participation](FUTURE_EVIDENCE_BUSINESS_PLAN.md):
   fuel-purchase documents/NFC-e and representative/portal capabilities.
   D is excluded from the A–C implementation batch and core launch prerequisites.

This is planning only: no A–D feature implementation, provider activation,
document collection, issue/PR/wiki publication, device action or deployment.
A future implementation request selects the bounded range. No arbitrary trust
formula, geographic admission, new monetization or provider is adopted here.

[Target contracts](../product/APP_CONSOLIDATION_TARGET_CONTRACT.md) own proposed
B-BR-OPT-01–18 / BUC-OPT-01–10; the
[continuation prompt](APP_CONSOLIDATION_EXECUTION_PROMPT.md) bounds execution.

## Binding D restriction

**Do not implement D until the app is extremely consolidated and has thousands
of real clients, unless the maintainer explicitly requests earlier implementation
of the relevant D scope.**

This is an adopted scope restriction, not only a rollout flag: no D runtime
stubs, dormant APIs/migrations, dependencies, document collection or hidden
screens during A/B/C. Completing C, downloads, a deadline or generic instruction
to continue all current work is not an early override.

Normal reopening needs documented maturity AND sustained real adoption in the
thousands, plus explicit D selection. Define the client metric, observation
window and threshold at future reopening; do not invent them or require paying
customers now. Missing definitions/evidence keep D deferred. A direct explicit
early human request can waive maturity/adoption timing only for its named scope;
verification, privacy, critical tests and protected delivery remain mandatory.

Overlaps: FQ-10 private document/purchase verification, document attachment in
FQ-15, and new P30–P33 claim/proof/management/portal work are held by D regardless
of their older task labels. Official ANP history, non-document reports, public
station metadata, ordinary photo contributions, workflow capture receipts,
billing-provider tokens and existing private manual records are outside this
hold. Preserve existing source/history and maintain existing privacy/security
duties; the hold neither deletes implemented code nor certifies it.

## Current evidence and reuse

Planning source: isolated app-consolidation-plan worktree at
`91ff1f93650839a3a55a7239d14b737b25919d04`, preserving the occupied FQ worktree.
FQ-00–08 now have source evidence and FQ-09 is the recorded next candidate;
their historical LOCAL_DONE / INTEGRATION_PENDING records remain intact.
This plan changes future overlap timing, not their past acceptance.

The primary checkout remains detached at `e22824db` with another owner's
Evidence Store edit. Local dev `757c09ae`, main `a349d7c` and origin/main
`68483db9` were locally recorded refs; no remote fetch/protection refresh is
claimed. At the earlier read-only opening,
[PR #135](https://github.com/AlexandreZanata/Abastevo/pull/135) was verified merged
with head `e6fc6da7`; the separate community-first checkout reached
`8bc9e5ba` with later safety fixes. Reconcile its actual current integration
at implementation; old test/deployment records are not fresh runtime proof.

Inspected reuse boundaries:

- Home uses a 15-second resumed timer, clears in-memory summary at session start/
  failure and has expiry handling. A later periodic guard already avoids
  restarting an active slow request; preserve it rather than redo it.
- Anonymous city summary uses one full-population SQL snapshot, no-store and
  request-time generated_at. ETag alone does not avoid SQL aggregation.
- Room/Directory cache, shared origin, signed capture, media SHA-256/dHash,
  duplicate queries, Jobs, Trust and Moderation already exist.
- Basic weekly alerts, notification/history repositories, vehicle data,
  TankFillCostRule and price-history rules already exist. Advanced alert/inbox/
  provider work is in [PRM07 and later amendments](PREMIUM_BACKEND_APP_PLAN.md);
  source implementation and actual provider acceptance are distinct.
- [ADR-021 in merged source](https://github.com/AlexandreZanata/Abastevo/blob/e6fc6da7326dc576a2d1055ea78bbdd8089d5a4b/docs/adr/021-community-first-contribution.md)
  removes mandatory location admission. A/B add no GPS requirement, missing-GPS
  penalty, proximity advantage or new attestation SDK.
- [FQ official quality](FUEL_QUALITY_IMPLEMENTATION_PLAN.md), price confidence,
  contributor trust and business badges remain separate. A/B/C do not change
  FQ scoring or smuggle D into a different module.

Before OPT-A00, reconcile the planning patch and accepted source with maintained
dev synchronized from main, using isolation when occupied. Existing
[ADR-019](../adr/019-android-vps-integration-first.md) app-first obligations remain;
A→B→C refines the selected app work after prerequisites, without restarting
completed P phases. The new D restriction overrides older timing that made new
private proof/portal expansion part of the early app construction sequence.

## Architecture and delivery contract

Reuse existing Go modular monolith, PostgreSQL/PostGIS, worker/transactional Jobs
and Android domain/application/data/app modules, Room, com.anpfuel and notices.
Pure rules stay independent of storage/network. A owns public summary read models;
B extends Evidence/Community/Trust/Moderation through explicit ports; C consumes
source-aware read and vehicle/history/alert capabilities. No duplicated catalog,
new always-on service, Redis/graph database, ML or provider dependency by default.
Any measured dependency need requires maintenance/license/security/cost review.

Task ranges: OPT-A00–A07, OPT-B00–B08, OPT-C00–C08, OPT-D00–D06.
Execute one bounded task at a time, splitting oversized tasks into letter-suffixed
checkpoints. Document rules before behavior; pure RED → GREEN → REFACTOR.
Every task records dependencies, inputs/areas, output, meaningful acceptance,
failure/recovery and exact executable commands when its tests/packages exist.

Auth/money/privacy/SQL/migrations/jobs need immediate negative/replay/concurrency/
real-PostGIS checks; affected private object work also needs actual disposable
S3 boundary tests. Run affected compilation/format/static/contracts, diff and
secret review. Known local failures block dependent source; do not weaken tests.

Freeze operational budgets from actual measurements: requests/session, bytes,
SQL calls/plans, queue age, app memory/latency, provider requests and review
false positives/cost. Priority labels and mathematical request reductions are
not measured battery, capacity or retention gains.

Source checkpoints are LOCAL_DONE / INTEGRATION_PENDING and can advance the next
already authorized A/B/C task without PR waits. Native/provider/deployment/
retention proof stays separate. Routing/provider unavailability blocks its
affected real flow, not unrelated history/local alerts.

At selected batch closure, run the affected end acceptance union once and due
Android/native/accessibility checks. Follow [DELIVERY_WORKFLOW](DELIVERY_WORKFLOW.md),
[CI_PLAN](CI_PLAN.md), ADR-018/020: protected dev→main, actual current-head/base
Quick verification/reviews, guarded finish and owned merged-SHA wiki when
authorized. No CI polling, per-stage merge wait, direct-main or bookkeeping PR.
G24/G09/P10 retain their scoped gates; iOS remains deferred.

D is not an unmet A–C exit criterion. Record DEFERRED_FUTURE, never implemented,
completed or permanently cancelled merely to close a batch.

## Validation and next action

Docs-only checks: changed/new Markdown paths/anchors, rule/task IDs, dependency
order, D restrictions across owning plans, whitespace and scoped secret/PII
review. No backend/Android runtime suite or live readiness claim for this edit.

Future checks reuse [backend/README](../../backend/README.md) and
[TEST_STRATEGY](../backend/TEST_STRATEGY.md). Building blocks:

```sh
./gradlew :domain:test :application:test :data:testDebugUnitTest :app:testDebugUnitTest :app:lintDebug :app:assembleDebug
git diff --check
bash scripts/scan-secrets.sh
```

Select actual affected Go/sqlc/PostGIS/S3 commands at task opening. Never run
destructive fixtures on staging/shared dev or claim nonexistent package tests.
The isolated full backend runner is
`bash scripts/tests/test-local-backend.sh`, when justified at batch closure.

First future implementation candidate: OPT-A00 after explicit selection and
current-head reconciliation. Every D task remains DEFERRED_FUTURE.

## Local planning acceptance

Documentation validation: PASS, 2026-10-14, isolated source base 91ff1f93.
The scoped Python inspection checked 50 added/new local links, all 33 task
sequences and dependency/input/output/test/recovery fields, 18 rules and 10 use
cases, and D restriction propagation across the overlapping plans. New/added
whitespace and high-confidence secret scans, git diff --check and the repository
tracked-secret scan passed. Prose review found no private documents, credentials,
precise contributor GPS or production media.

Scope: seven new and ten existing Markdown documents only. No runtime suite,
source feature, migration, provider/native/production test, remote publication
or deployment was executed. A/B/C remain PLANNED; D remains DEFERRED_FUTURE.

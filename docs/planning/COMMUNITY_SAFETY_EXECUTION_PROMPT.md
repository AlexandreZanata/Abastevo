# Execution prompt: community safety and privacy

Date: 2026-10-11. Status: READY_FOR_USER_HANDOFF / NOT_EXECUTED.
Companion: [canonical phased plan](COMMUNITY_SAFETY_EXECUTION_PLAN.md).
The prompt below is intended for the user to paste into the implementation AI.
Its presence in the repository is not execution, publication or deployment approval.

---

Implement the Abastevo community evaluation, abuse protection, moderation,
bounded feeds and optional training-image privacy plan in this repository.
Include the offline extension CS15–CS25: saved community prices, bounded city
downloads, local pagination, durable offline contributions/social actions,
safe capture/OCR, synchronization, privacy and failure/abuse acceptance.
Communicate with me in Portuguese; write code, technical docs and commits in
English. Preserve Portuguese product/legal copy. Do not substitute a new plan
for implementing the selected tasks.

Read AGENTS.md, docs/planning/FAST_EXECUTION.md, only current PROGRESS.md,
DELIVERY_WORKFLOW.md at opening, and
docs/planning/COMMUNITY_SAFETY_EXECUTION_PLAN.md. Then read only the selected
ROADMAP/phase rules and relevant owning code/tests. Reconcile actual git status,
diff and commits first; preserve other contributors' work. Use maintained dev,
synchronized from main according to ADR-020; if occupied, use an isolated
worktree rather than overwriting, stashing or resetting anyone's changes.

Preserve task IDs; execute by dependencies **CS00–CS12 -> CS15–CS25 -> CS13 -> CS14**,
not numeric sorting. Select the first genuinely pending microtask; do not repeat
accepted tasks unless inputs changed. Split phases into named A/B/C tasks with
independent tests/evidence. In microtask mode, execute exactly one per invocation,
record its checkpoint and stop with the next task. Continue automatically only
when the user explicitly selected continuous execution; no CI/PR polls or repeated
permission questions. CS13/CS14 follow only after their actual
prerequisites pass and the user has authorized the specific staging/device
execution; reuse applicable authorization already given without asking again,
but never infer runtime access from a document. Do not perform deployment
or phone changes as part of merely reading this prompt.

CS00 must produce a concise reused/missing inventory, actual target/card mapping,
current route/schema/index/DI owners, exact limits, supported orders, threat
coverage, versioned API/error contracts and frozen measurement/stop budgets.
Do not invent endpoints, quotas or existing completion. Domain B-BR/BUC and
acceptance examples precede behavior. Use RED -> GREEN -> REFACTOR. Implement
with existing module ports, Go/chi/pgx/sqlc/slog, PostGIS, jobs, private S3 and
Android transports. New dependencies/services need recorded need/license/security
review; no default Redis, microservice, paid fraud SDK or UF partition change.

Non-negotiable product and engineering rules:

1. Likes/dislikes mean **contribution helpful / not helpful**, UI `Contribuição
   útil` / `Contribuição não útil`. They do not validate a price or affect price
   confidence, consensus, station ratings, payment privileges or comment
   VALID/INVALID votes. Bind one account vote to one stable contribution/revision,
   across device keys; reject self-votes and ineligible targets. Change, remove,
   lost-response retry and concurrent commands must reconcile exact tallies.
   Do not treat a changing aggregate price card as a contribution identifier.
2. Anonymous users can browse. Voting/reporting/uploads use existing free-account
   signed-write rules. Reuse app-scoped resettable key/installation proof for
   technical admission; a device ID is not a person or authentication. No IMEI,
   advertising ID, cross-app fingerprint, permanent raw-IP logging or GPS
   telemetry. Cap issuance/key farms and limiter cardinality. Layer per-route,
   account, verified installation, trusted IP, concurrency and global cost
   budgets; preserve shared-NAT usability and accessible 429 recovery.
3. Prevent heavy work before admission. Bound bodies, headers, cursor decoding,
   image dimensions/decode CPU, DB connections/timeouts, jobs/retries/cache/metric
   labels and all storage copies. Atomically reserve bytes/session capacity before
   upload grants; reconcile actual provider bytes, abandoned/multipart/versioned
   objects and release reservations idempotently. Fail closed for new costly
   intake when quotas/storage/deletion cannot enforce limits; preserve bounded
   reads, cleanup and status. Never run a real DDoS against the shared VPS.
4. Reuse reports, existing private operator CLI and moderation audit. A dislike
   or report count does not automatically hide/delete or prove abuse. Preserve
   bounded cases, reporter privacy, author deletion priority and appeal rules.
   Do not invent public admin endpoints or an unrelated moderator console.
5. Reuse signed filter-bound keyset cursors, limit+one and safe DTOs. The shared
   current default/max are 20/100; verify before any deliberate versioned change.
   No deep OFFSET, per-page full COUNT, all-history fetch or N+1 queries. Cap
   cursors before decoding. Keep caller's private vote overlay out of public
   caches. Define live-update behavior honestly: unchanged-data traversal is
   exact; mutable feeds need deduplication/refresh and may move between pages.
   LIMIT alone does not prove bounded query cost. Measure real PostGIS plans,
   dense/sparse/empty cities, every sort, maximum page and deep cursors.
6. Implement P37-PC05 once. Training consent is optional, default OFF, versioned
   and server/account/per-image bound. ON applies to future disclosed photos,
   never old ones. OFF stops local new permission immediately, submits priority
   signed revocation, and shows pending until acknowledged. Server revocation
   invalidates old grants and starts deleting owned training derivatives/labels;
   stale offline ON cannot re-enable. Re-enabling needs fresh explicit consent.
   Include a separate delete-images action and owner-only privacy rights APIs.
7. Ordinary original/quarantine/thumbnail/outbox media expire within the existing
   24-hour contract, without retry/crop/consent resetting deadlines. Training
   permission only allows a separately minimized private derivative; uncertain
   sanitization prevents promotion. Every copy/version/export needs ownership
   and deletion inventory. Proposed revocation purge target is 24h from server
   receipt, a project SLO rather than a legal deadline: freeze and prove it
   before public promises. No transient-media backups. Replay expiry/deletion
   before restored traffic. Test real storage, worker outages and promotion/
   revoke/export races; virtual time does not replace an actual 24h soak.
   A powered-off phone cannot physically clean until it executes again.
8. Wire docs/product/COMMUNITY_PRIVACY_TERMS_PT_BR.md only after controls match.
   Settings must show consent status, pending/error/delete receipts and actual
   terms version. Verify controller/contact/processors/transfers/retention facts
   before activation; .invalid test contact is never a production rights channel.
   Do not start model training, send photos to third-party trainers, publish a
   gallery or promise file deletion automatically untrains a model.
9. Offline reads use persisted local data immediately, including city/station
   community prices after process death. Audit existing ANP tools and all actual
   routes in CS15; test every row's offline outcome. Bound downloaded city packs,
   local pages, cache and pending work. Show saved coverage/source/observation time;
   expired last-known prices are historical, not current cheapest-price claims.
   Never promise data that was not downloaded or unseen remote changes.
10. Offline writes save owner/origin-bound durable intents, not server success.
    Reuse the outbox and receipts, fresh proof/nonces at send, immutable identity,
    bounded due queries, conflict/dependency handling and finite retry/dedup age.
    Quotas/admission/account state are checked server-side on reconnect. No bulk
    exemption, private overlay cache leakage, stale grant or priority starvation.
    Logout/key/origin/backup changes cannot migrate or silently replay intent.
11. The current online two-minute camera permit is not full offline authorization.
    CS20 must freeze a safe versioned bounded offline-capture contract before
    Android consumption. Evaluate server-preissued single-use permits without
    claiming client time/GPS is infallible. Refuse trusted claims for absent/
    expired permission, mock fixes, clock/reboot uncertainty or unknown stations;
    preserve an explicitly unverified draft instead. Do not use reconnect GPS or
    refresh capture dates. Local image expiry remains capture+24h even unsent.
    Audit offline OCR model availability and manual fallback; no silent new SDK.

Run immediate targeted meaningful domain, HTTP, failure, negative and concurrency
tests. Auth/money/privacy/SQL/migrations/jobs also require real isolated
PostGIS/storage/recovery/least-privilege checks in the same task. Follow
docs/backend/TEST_STRATEGY.md and backend/README.md, including the actual Rust
preparer for affected integration tests. Run affected compilation/format/static,
contract, secret-surface and diff checks. Do not skip/weaken tests, use mocks as
runtime proof, or run the whole aggregate after each small change.

For CS07 freeze reproducible seeds/data/arrival profiles and SLOs before results.
Separate actual HTTP, Reader and SQL numbers. Include maximum current page size,
all sorts/city densities, vote/report/ingest contention and limiter false positives.
Use independent measured windows per the plan, correct elapsed throughput,
p50/p95/p99, drops/errors/429, RSS/cgroup memory/CPU/OOM/restarts, pool/locks,
rows scanned/IO/WAL/temp spills, queue and physical storage/deletion lag. Retain
raw hashes and environment ceilings. Do not extrapolate requests/s into a claim
of maximum simultaneous users, or call isolated synthetic traffic real adoption.

For later CS13/CS14, only the authorized Abastevo container/namespace may change
on the VPS described by /home/iiii/I9-PROJETOS/VPS-HOSTINGER-I9-32GB/. Read its
instructions, discover actual topology and use https://teste.abastevo.com.br.
Never alter other images, workloads, data, shared proxy/firewall or host settings;
no global prune/down or unbounded load. Preserve media volumes and deadlines,
prove recovery/resource reserve and stop on the frozen safety thresholds. The
old preview cutoff is 2026-10-15 07:45:50 America/Cuiaba; do not extend it silently.

CS14 is the final device acceptance phase. Normal/release Android must refuse
Fake GPS as trusted location. Controlled near/boundary/far checks may use only
nonrelease, server-authorized TEST_ONLY receipts and synthetic isolated data;
never turn off mock checks in release. Test real PostGIS 149.9/150/150.1m and
unknown station coordinates. ANP station presence alone is not reviewed precise
GPS. Genuine near/far phone fixes remain separate evidence; mark OWED if unavailable.
Preserve phone accounts/data, verify release excludes test controls, restore the
mock setting changed by this test, and leave unrelated developer settings alone.

For each phase, make a meaningful atomic local commit when the applicable
workflow permits; record rule IDs, exact commands/counts, source fingerprint/SHA,
evidence, limits and next task in the canonical evidence and concise PROGRESS.
Use LOCAL_DONE, INTEGRATION_PENDING, INTEGRATED and RELEASE_CERTIFIED accurately.
No publication follows from planning. When final delivery is explicitly authorized,
use the protected cumulative dev -> main PR, preserve commits, run the configured
guarded finish and publish wiki once from merged SHA. No main development push,
force push, admin bypass, no-verify, bookkeeping-only PR or CI polling.

Opening snapshot: at 0f3601c PR127 backend/Quick passed but Android test failed
with UncaughtExceptionsBeforeTest in CommunityPricesViewModelTest; local reruns
passed and do not establish a fix. Reverify actual state and diagnose any still
known failure before merge. Do not merge on a repeated lucky green run.

At handoff provide: completed/partial/owed phase checklist, rules-to-test mapping,
source diff/hashes, migrations/OpenAPI/query plans, immediate test reports,
actual HTTP manifests/raw hashes, real all-copy 24h purge and restore evidence,
APK/backend hashes, safe private synthetic device traces, residual risks and
rollback. Include the CS15 capability manifest, all offline scenario outcomes,
Room recovery/query evidence, permit decision, exact receipt/effect reconciliation,
reconnect/resource metrics and offline expiry proof. No required FAIL/OWED row may
be labelled full offline acceptance; network-dependent capabilities stay explicit.
I will return for independent validation; do not waive it or claim
G09/public production certification. If a required owner fact or external
boundary is unavailable, stop only dependent work, state the exact blocker and
continue useful authorized independent tasks.

## Microtask continuation prompt (Portuguese, eight lines)

```text
Continue docs/planning/COMMUNITY_SAFETY_EXECUTION_PLAN.md: execute somente a próxima microtarefa pendente na ordem CS00–CS12 → CS15–CS25 → CS13 → CS14.
Leia AGENTS.md, FAST_EXECUTION.md, PROGRESS.md e apenas os contratos/códigos relevantes; preserve alterações existentes.
Confira as dependências e defina regras, escopo e critérios de aceitação antes de editar; não repita tarefas já aceitas.
Implemente só essa microtarefa, reutilizando módulos e seguindo RED → GREEN → REFACTOR quando houver comportamento.
Valide offline real, falhas, abuso, concorrência e persistência conforme o risco, além de compilação, estática, segredos e diff.
Não confunda cache com preço atual nem fila com envio confirmado; preserve autorização, localização, quotas e expiração das fotos.
Atualize PROGRESS.md com evidências reais, pendências e próxima microtarefa; na VPS, altere exclusivamente o Abastevo autorizado.
Pare após a microtarefa e reporte resultados e bloqueios; não publique, faça merge ou declare funcionamento integral sem a matriz passar.
```

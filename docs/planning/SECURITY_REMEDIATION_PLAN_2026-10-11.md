# Security remediation plan — 2026-10-11

Status: PLANNED / IMPLEMENTATION_NOT_STARTED. Owner evidence: [source audit](../security/SECURITY_AUDIT_2026-10-11.md), pinned commit `954a8d800576fec24acc9edfd3b93aad2ac15450`. This request authorized audit and planning, not implementation/deployment/publication. Findings concerning provider/proxy configuration must be reconciled with actual authorized deployment evidence before claiming a live exploit.

## Execution rules for an economical implementation agent

Work one microtask at a time. Read AGENTS, FAST_EXECUTION, current PROGRESS and only the selected task's evidence/code/tests. Reconcile current dev/main and ongoing CS02 work; do not modify/stash another effort's changes. Use an isolated worktree when required. This plan supplements [CS00–CS25](COMMUNITY_SAFETY_EXECUTION_PLAN.md); reuse its identity, feedback, privacy, queue and storage components rather than constructing another framework/service.

Before changing behavior, document the applicable B-BR/BUC contract and bounded acceptance. Implement a failing regression test, then the smallest fix, then refactor only if necessary. Audit characterization probes currently assert the flawed behavior: **invert them into safety regressions**, do not permanently bless that behavior. Source acceptance requires auth/privacy/negative/concurrency tests immediately, real disposable PostGIS for SQL changes, affected compilation/format/static checks, secret review and `git diff --check`.

Each completed task records actual revision, commands/results, failure/recovery cases, limitations and next task as LOCAL_DONE / INTEGRATION_PENDING. No live readiness inference. Follow ADR-018/020 delivery at the final authorized construction batch, with protected dev → main and the configured required Quick verification check. No direct main push, force/admin bypass or bookkeeping-only PR. Planning alone never publishes.

## Priority and dependencies

Default order: **SEC00 → SEC01 → SEC02 → SEC03 → SEC04 → SEC05 → SEC06 → SEC07 → SEC08 → SEC09**. SEC01–03 address the three HIGH risks and the related atomic-budget defect first. Privacy and offline safety remain release gates even after availability fixes.

### SEC00 — Reconcile source, reproduce and freeze the threat/budget contract

Maps to AUD-01…12; integrate with CS00/CS03–04.

- SEC00-A: record current maintained dev commit, concurrent changes and which findings still reproduce. Parallel construction reached CS07A (`d04c96f`) during the audit; review the later helpfulness/moderation HTTP/store/migration changes and unfinished Android work before treating them as covered.
- SEC00-B: create a route/capability inventory distinguishing anonymous reads, anonymous signed price writes, account-only social writes, private owner reads, upload grants and restricted operator actions. Do not turn intentional anonymous price contribution into an alleged auth bypass.
- SEC00-C: freeze versioned item/byte/time/concurrency budgets for every intake and queue. Use existing limits and measurements first; document finite global protection, NAT fairness, expiry and 429/503 semantics. Numeric ceilings not supported by evidence remain proposed, not advertised capacity.

Exit: current-source reproductions and explicit B-BR/BUC/owner map. No VPS or phone mutation.

### SEC01 — Bound public account and feedback resource admission

Maps to AUD-01; reuse CS03–04 and the existing atomic quota adapter.

- SEC01-A: regression for distinct-email/username/session creation and repetitive comments. Separate per-subject fairness from global item/byte/concurrency admission; cover invalid-request cost as well as successful writes.
- SEC01-B: gate unavailable email delivery by deployment configuration; remove the unbounded memory mailbox from production wiring. Keep a bounded deterministic test adapter. Do not add an SMTP dependency without an explicit need/license/security record or expose a debug-code route.
- SEC01-C: atomically enforce applicable admission before persisting account/session/comment/revision rows. Add bounded cleanup/retention for codes/sessions and storage accounting; keep free registration/social participation free. Address and IP identifiers stay minimized/hashed with documented retention.
- SEC01-D: meaningful real-SQL tests for parallel requests, quota rollback, duplicate intent/retry, exhausted global budget, rotating subjects and a legitimate second client. Return stable retry metadata and useful frontend states.

Exit: unique-address/username floods cannot bypass aggregate ceilings, one account cannot create unlimited comments, and unavailable dependencies do not leave unlimited retained issuances. Demonstrate bounded growth rather than only counting HTTP statuses.

### SEC02 — Repair identity fairness behind the actual trusted proxy

Maps to AUD-03; reuse CS03 network-abuse controls.

- SEC02-A: regression with two independent clients behind one proxy, direct untrusted forwarding headers, IPv4/IPv6, malformed/multiple headers and NAT-shared clients.
- SEC02-B: implement one narrow client-address resolver with explicit trusted immediate-proxy allowlist and header-chain ownership. Preserve raw IP log/database exclusion. Do not trust all private peers or arbitrary X-Forwarded-For by default.
- SEC02-C: split deliberate global overload protection from per-client/key registration/challenge limits; verify ordinary signed multi-step photo/sync operations have a viable budget.

Exit: real **local** Caddy → API integration demonstrates independent quotas, bounded global protection and spoof rejection. An actual shared VPS edge is outside this task's mutation scope; any required shared-edge change must be proposed separately, never applied under Abastevo-only authorization.

### SEC03 — Make media bytes, reservations and retries atomically bounded

Maps to AUD-02/04; reuse CS04 and existing evidence session/idempotency ports.

- SEC03-A: move contributor/day quota and natural-key admission into the same transaction; add atomic byte reservations and correct release/expiry semantics. A replay of the winner remains readable at full quota; divergent bodies conflict.
- SEC03-B: select an enforceable transfer boundary compatible with the deployed provider: proven exact-length signed PUT or bounded streaming through the existing authorized application/edge. Document why the chosen mechanism caps accepted bytes, repeated transfers and incomplete writes. A returned JSON maximum is insufficient. Do not assume R2 supports S3 POST content-length-range policies.
- SEC03-C: provider-compatible integration tests for declared/actual mismatch, missing/chunked length, max and max+1, repeated URL reuse, expired/revoked session, wrong owner/key/MIME/hash, interrupted transfer and concurrent budget admission. Use small synthetic payloads and the actual protocol, not only a permissive fake.
- SEC03-D: verify bytes never enter a public prefix, worker reads/decodes remain capped, failed objects are reconciled, and monitoring reports accepted/reserved/rejected bytes plus oldest incomplete reservation without secrets.

Exit: no transfer bypasses the effective byte cap; parallel fresh reservations cannot exceed the configured count/bytes; retries do not multiply storage or extend media lifetime. Real PostGIS and real local S3-compatible service required; provider-specific production acceptance remains separate.

### SEC04 — Connect user deletion to resumable privacy erasure

Maps to AUD-05; reuse CS09–11 and existing account/feedback/privacy erasure functions and deletion ledger.

- SEC04-A: freeze the complete account/contributor/social/media/queue/credential/alias/backup inventory and lawful/contractual retention exceptions. Capture linked owners durably before dropping bindings; define a receipt rather than pretending acknowledgement is completion.
- SEC04-B: atomically record the user erasure intent and revoke relevant session/key authorities; enqueue owned scope cleanup through explicit ports. Reuse existing erasure jobs and ledger, extending them only as necessary. Avoid a distributed transaction or an unrelated service.
- SEC04-C: tombstone/anonymize public attribution and minimize credentials/links according to the contract; rebuild feedback/community aggregates without deleting other authors' replies/history. Reconcile consent withdrawal and training inventory separately from operational-data expiry.
- SEC04-D: local cleanup must refuse/send no pending operation under the erased owner; include all private media/queue copies. Test lost response, worker failure in each scope, duplicate deletion, concurrent writes, partial restore and ledger replay.

Exit: actual account HTTP route → durable job → public/private reads → restore evidence. Existing operator-only erasure tests are necessary but insufficient. Terms/settings must describe the demonstrated state, with optional training off by default and separate revocation.

### SEC05 — Prove every-copy media expiry and cleanup health

Maps to AUD-06/07; reuse CS10–11/CS21–22.

- SEC05-A: extend bounded overdue inventory and intake health to quarantine originals, finals and every provider copy/version. Failed original deletion must produce a policy failure and stop new intake; preserve successful per-copy progress.
- SEC05-B: replace hourly-after-expiry assumptions with deadline-aware scheduling plus measured safety margin/catch-up. Add bounded retry and oldest-overdue alarms. Test storage outage, growing overdue backlog, process death, active verifier, late upload, clock skew and restored state. Never overwrite applied migrations; add the next actual unused migration with recovery tests if needed.
- SEC05-C: Android cleanup handles abandoned `.tmp` files and ownership without racing a live writer. Delete/discard and sweep cover both temporary/final sealed paths. Reject rollback/future timestamps under a versioned policy; preserve the remaining lifetime across restart/reboot without claiming powered-off deletion.
- SEC05-D: assess OS backup/device transfer for private Room metadata separately from cacheDir photos and excluded auth preferences. Prove selective exclusion/no-backup handling and ownership on restore; do not destroy public offline packs unnecessarily.
- SEC05-E: isolated short fake-clock tests first, then an authorized real 24h observation of every copy and deletion receipt. Worker/API health green is not physical-deletion evidence; inability to delete is an explicit release failure.

Exit: zero unexplained overdue copies, retry/recovery evidence, actual every-copy deadline compliance where execution is possible, and an honest offline/powered-off limitation in the user notice.

### SEC06 — Bound official-data synchronization and contribution outbox work

Maps to AUD-10/11; reuse CS15–18/CS21/CS23–24.

- SEC06-A: enforce approved source origins/HTTPS/redirects and compressed/download/listing limits; atomically publish completed files and purge failures. Test redirects to foreign/private/plaintext destinations and filenames with query/traversal-like components without fetching them.
- SEC06-B: implement XLSX coordinate checked arithmetic/range validation before allocation; cap expanded ZIP entry/total bytes, worksheets, rows/cells, shared strings and per-string text. Close resources if initialization fails. Use a small crafted column fixture and small compression-expansion fixtures; never rely on catching OutOfMemoryError.
- SEC06-C: SQL selects/claims one requested outbox command or one bounded eligible page, with appropriate indexes; loading all payloads to select one is forbidden. Keep owner/origin/frozen revision and lease semantics. Add paged local history and explicit item/byte/history retention budgets.
- SEC06-D: persist terminal versus transient refusal; honor server Retry-After, use bounded backoff/jitter and per-run concurrency/attempt/byte budgets. Preserve the same operation ID and fresh request proof across retries. Test logout, two accounts, environment change, process death after server commit, lost ACK and cancel races.
- SEC06-E: measure growing synthetic histories (for example 1/100/1,000/10,000 records under fixed local resource ceilings), query counts/scanned rows/allocated bytes/peak RSS/latency, offline start and reconnect. These are test sizes, not promised supported users. Demonstrate cost bounded by requested batch and no quadratic full-history scanning. Include honest large official XLSX samples within agreed ceilings.

Exit: malformed tiny inputs fail safely before large allocation, partial imports do not corrupt the prior usable snapshot, offline reads remain available, and reconnect cannot produce unbounded local work or backend intake.

### SEC07 — Harden refresh failure handling and per-issuer JWKS lifecycle

Maps to AUD-08/09; reuse existing account/identity components.

- SEC07-A: replace random-invalid-token family revocation with reviewed proof-based reuse detection and bounded hashed history, preserving rejection of real replay. Immediate real-PostGIS parallel refresh/revocation tests; never log tokens or family identifiers.
- SEC07-B: per-issuer JWKS timestamps, bounded refresh on unknown/removed keys, explicit redirect/HTTPS policy and bounded failure behavior. Test alternating providers past TTL, key removal, provider outage and malicious redirect locally.
- SEC07-C: verify Android serializes refresh, does not transparently replay unsafe refresh requests after an ambiguous response, and distinguishes unavailable authority from a permanent denial. Keep account/session/owner revocation checks fail closed.

Exit: a random bad token cannot revoke a known valid family, proven replay still triggers the frozen security response, and one provider's cache refresh cannot prolong another provider's TTL.

### SEC08 — Make location assurance explicit and adversarially tested

Maps to AUD-12; reuse CS13/CS19 and the location policy. This phase contains a policy decision, not an instruction to add a speculative attestation service.

- SEC08-A: document the difference between request/key authenticity, OS mock detection, coordinate geometry, station-point quality and physical presence. Ensure self-declared metadata alone cannot manufacture an independent HIGH-trust signal. Version any contract change; preserve existing user-facing 150m geometry and honest unknown/manual outcomes.
- SEC08-B: construct a separate synthetic client/key with internally consistent fabricated location metadata. Exercise signed HTTP capture/price routes under a local test environment; test replay, teleport, stale/coarse/missing signal and unreviewed station coordinates.
- SEC08-C: record a measured decision on additional platform assurance and its unsupported/offline-device behavior; attestation does not prove GPS by itself. No device fingerprint, blacklist or precise-GPS retention is silently introduced.
- SEC08-D: only after the offline/privacy implementation dependencies pass, perform the authorized final real-phone staging matrix: normal Fake GPS must refuse; isolated TEST_ONLY geometry cannot enter public consensus; genuine location can pass at a reviewed station. Preserve evidence without real photos/coordinates in Git/logs.

Exit: stated assurance matches measured behavior. No “100% fake GPS prevention” or physical-presence claim based on geometry alone.

### SEC09 — Independent validation, bounded staging and protected delivery

- SEC09-A: current-head review of all corrections and post-audit additions (including CS02–CS07A and subsequent Android work); meaningful negative/concurrency/idempotency/crash-after-business-commit-before-ledger-completion tests through real HTTP. Verify queue/pool saturation cannot deadlock critical privacy/auth paths.
- SEC09-B: current Go/Rust plus Android resolved-dependency and container/base-image advisory scans with pinned versions/SBOM. Review licensing/security records for any new dependency. Confirm release excludes debug injection/private export surfaces.
- SEC09-C: complete affected immediate/local integration checks once, then release-scoped fault/recovery evidence. Existing clean scans/static gates do not replace business-logic regressions. Record changes to the tested head; prior evidence does not cover later commits.
- SEC09-D: prepare an explicit Abastevo-only staging runbook and rollback before any future authorized rollout. Bound CPU/memory/IO/request rate with current measured spare capacity, use existing guards and stop on latency/backlog/overdue/OOM/reserve breaches. Do not change other workloads, host/shared proxy, images, volumes or data. Historical 5 req/s windows are not an automatic current safety budget or maximum-user estimate.
- SEC09-E: final protected dev → main PR, required current-head/base checks, guarded merge preserving commits and one allowlisted merged-SHA wiki mirror, only within the authorized implementation batch. G09/G24/live acceptance statuses remain distinct.

Exit: zero unresolved HIGH findings in the deployed scope, MEDIUM items fixed or explicitly accepted with owners/expiry/compensating evidence, every-copy privacy criteria met, measured bounded reconnect behavior, and no unverified production/readiness claim.

## Continue prompt (eight lines)

```text
Continue the next pending microtask in docs/planning/SECURITY_REMEDIATION_PLAN_2026-10-11.md.
Read AGENTS, FAST_EXECUTION, current PROGRESS, the selected contract and relevant code/tests only.
Reconcile current dev/main and preserve ongoing CS02/other changes; isolate the checkout when occupied.
Document the B-BR/BUC acceptance, reproduce the defect with a failing safety regression, then apply the smallest fix.
Run negative/failure/concurrency tests immediately and real disposable PostGIS for SQL changes.
Preserve owner/origin, idempotency, offline behavior and privacy; no speculative service/dependency or trust bypass.
Record the actual revision/results/limits and next action as LOCAL_DONE / INTEGRATION_PENDING; follow final-batch delivery.
Do not touch VPS/phone/shared edge or publish unless the current authorized execution scope explicitly includes it.
```

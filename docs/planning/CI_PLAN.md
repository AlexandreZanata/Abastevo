# CI cadence: local task, phase integration, release certification

Policy adopted from Goyim-Arena, adapted to the fuel backend. **Target configuration, pending G01-FLOW**. As inspected at `3540011eab38e1287ef344bdd9d457d9caae2606`, `backend.yml` still runs fast/vulnerability/PostGIS integration jobs on matching pushes/PRs, and `ci.yml` runs Android tests on its selected paths. This edit changes the plan; it does not claim those YAML triggers were changed or remote protection configured.

## Level 1 — each microtask, local and specific

Run required task acceptance and relevant risk checks before its implementation commit. Docs/process: links, task/dependency consistency, changed templates/rules, whitespace and secret-surface checks. Go: changed package/consumers' actual tests, formatting, compilation and relevant vet. SQL/PostGIS/migrations: real DB schema/upgrade/transaction tests. Authentication/privacy/price precision/idempotency/jobs: positive and adversarial/failure cases; concurrency/race where relevant. OpenAPI/sqlc: affected contract vectors and generation drift. Infra: syntax plus smoke of the changed control. Android work after G09: affected module/Room/permissions/offline tests, without unrelated backend load campaigns.

Risk class `critical` includes identity/replay/authorization, precise prices/units, observation immutability/consensus, PII/media, transactions/migrations/jobs, backups and destructive-operation controls. `standard` covers ordinary noncritical logic; `docs` is nonexecutable documentation. A workflow/migration/script disguised as documentation is classified by behavior. Failures block the corresponding commit/integration; retrying away flakiness is not a fix.

## Level 2 — phase integration, bounded quick CI

Target command: `make quick-verify` (introduced in P01-T13); one implementation shared locally/remotely, composed from small scripts rather than duplicating logic in YAML. Initial target budget: <5 min with warm tooling cache; remote hard timeout 12 min. These are budgets to measure, not timings already achieved.

Target required job name: **Quick verification**. Trigger on all PRs including draft open/synchronize/reopen/ready and push to main, with no workflow-level path filter that could leave a required check missing. Use per-ref concurrency/cancel superseded runs, read-only token, pinned actions/tools and dependency caches keyed to lockfiles. Avoid privileged `pull_request_target` and production credentials. Cancelling an obsolete run is normal; the current required run must finish SUCCESS.

Quick contents:

- Validate docs/task references and changed-file secret detection, including untracked local files and PR committed diff. Do not rely solely on working-tree `git status` in a clean CI checkout; that would scan nothing. Report filenames/redacted diagnostics rather than raw matched secrets. Parser/tool errors fail instead of being hidden by `|| true`.
- Go format/build/vet for the backend and compile its tests. If using `go test -run '^$'`, explicitly label that step compile-only.
- Real short behavioral suites for critical domain packages as introduced, plus config, health, redaction, transport and API-contract vectors already present. Maintain an explicit package/check manifest: required missing package or zero matched tests fails; a not-yet-implemented module is recorded as future, not silently skipped success.
- Relevant sqlc vet/diff and OpenAPI checks when their inputs/generated outputs change, with deterministic manifest-based selection and tested unknown-path fallback. Keep at least contract/reference integrity in the always-on part.
- No complete PostGIS/race/E2E/Android/image/restore/load sweep here. Required focused DB/security tests still run at Level 1 and specialized phase exit; their evidence is required for phase closure.

Source-wide vulnerability/image/security certification remains in the release matrix, with targeted dependency scans immediately when dependencies change or a relevant advisory/finding is known. Do not defer a known vulnerable addition until release. Slow tooling should be pinned/cached; avoid installing the same tool separately for each check.

Phase exit = declared specialized checks + quick local + required quick remote and configured reviews on the exact current PR head/base. A full suite named in an old task is interpreted by purpose: a directly required migration/auth/restore/load acceptance test remains mandatory; only redundant whole-project repetition moves to Level 3. Record each task's evidence scope explicitly to avoid waiving its acceptance criteria.

## Level 3 — full matrix on an immutable release candidate

Backend candidate: after P01–P08 and all phase fixes are merged, P09 selects a clean checkout of a pinned commit. Run the union of full unit/static/coverage, real PostGIS/race/migration upgrade, OpenAPI/compatibility/golden, auth/privacy/evidence/moderation E2E, dependency/secret/image scans, image/startup, deployed edge/cache/TLS, fault/load, encrypted off-host restore and privacy-deletion replay checks. Include the existing Android regression baseline needed to preserve the shipped app. Record expected jobs and external-environment checks up front; a missing result prevents certification.

Reuse previously collected expensive specialized evidence only when it demonstrably covers the same artifact/config/environment and unchanged relevant inputs. Do not rerun a 30-minute load or restore exercise because a documentation field changed, but never label evidence for one binary as certification of a different one. A code/config change invalidates affected candidate evidence; a changed release SHA requires reconciliation of the full matrix before approval.

Full workflows are manual release-candidate dispatch and stable version tag verification, never every ordinary PR/push. A manual run must explicitly select/verify the intended candidate SHA after all required phase merges. No candidate/stable tags merely to trigger early full CI. A stable tag points to the certified SHA; any tag-triggered checks must pass before release publication/deploy. No automatic production deploy follows phase merges. P10/P11 releases each have a separate appropriate full matrix.

The full entry point grows as modules and environment checks are implemented. During P01, its harness proves selection and failure behavior against available foundation checks; absent future suites remain explicit outstanding work. Only the complete P09 expected-result manifest can certify G09. A tool exiting zero with only the initial foundation subset cannot satisfy the release gate.

## Safe transition from current CI

P01-T13 implements local quick/full entry points and proves the selection/error cases; P01-T14 builds the safe Git phase controller; P01-T15 prepares idempotent issue/milestone reconciliation; P01-T16 prepares manifest-based wiki preview/publication; P01-T17 integrates remote workflow/protection settings and demonstrates the complete phase lifecycle.

Switch without a protection gap: introduce/observe the new quick job before making it required; preserve existing required checks until their replacements are verified; update required-status names and trigger changes in an ordered, reviewed rollout. If a required old check would disappear, do not mark activation complete or bypass protection. Where remote settings cannot be inspected/changed, report G01-FLOW BLOCKED with the exact missing permission/check; do not claim main is protected.

Activation tests: draft PR runs quick; docs-only PR still reports required quick success with meaningful docs checks; committed secret/change is detected in clean checkout; missing/failing/skipped required check prevents merge; changed head/base invalidates result; missing PostGIS fails a selected critical integration test; full suite is not automatic per PR; explicit candidate dispatch runs full expected matrix; no unauthorized remote operation in dry-run/local-only mode. Use synthetic/fake remotes first, then a controlled authorized PR. Never weaken runtime/phase tests to hit the quick budget.

## Evidence and state

Task LOCAL_DONE → phase READY_FOR_INTEGRATION → INTEGRATED (wiki separately PENDING/SYNCED) → candidate RELEASE_CERTIFIED. Current backend workflows are evidence of the old cadence, not proof of G01-FLOW. G09 retains all security/privacy/restore/capacity acceptance: faster iteration changes when broad suites run, not what a release must prove.

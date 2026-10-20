# Mobile pagination and maintained-dev delivery reconciliation

Status: IN_PROGRESS
Validation: PENDING
Integration: INTEGRATION_PENDING

User authorized reconciliation and protected main integration on 2026-10-20. Reuse PR138 (dev→main), preserving its quality batch and both histories. Main base08a54dc4, published dev901c432c, committed local quality tip9737b22f (advanced from the initial91ad38c0 during reconciliation), mobile/native review tipd0918d03. An independent local clone with a private dev ref avoids taking over the dirty maintained-dev checkout. The uncommitted quality109 work and the original evidence-store edit stay in their original directories; no stash/reset/force update is used.

Normal merge changes only the combined progress record on conflict. All Android/domain/application/data/build inputs are byte-identical to d0918d03; its1,726 JVM executions and38 native passes remain applicable. The additional backend source is the committed quality continuation through FQ-07R6B, with its checkpoint evidence and still-default-off sources. No uncommitted future milestone or whole-feature acceptance is imported.

Before final integration, validate the combined backend on a separately named tmpfs PostGIS, actual migration chain109, the four pinned Rust executables and the two actual SQLite harnesses. Reverify actual GitHub head/base and all applicable signals. Main protection observed: strict required Quick verification (GitHub Actions app15368), administrators enforced, zero required approvals, no force-push/deletion. No protection changes or override are authorized. Run git-flow finish with explicit origin/main and required Quick verification only when remote acceptance is successful.

Existing remote failures were not treated as accepted: quick refused the unclassified .dockerignore Docker build input; integration reported alert fixture/no-candidate failures and a concurrent cloud snapshot duplicate. The first combined local run additionally exposed stale quality/source-index fixture expectations and strict preparer mode requirements. Diagnose each failure before claiming acceptance; no assertion is removed merely to turn CI green.

Review contracts for integration repairs:

- B-BR-DELIVERY-01: commits from main, published dev, captured local dev and mobile review remain ancestors; foreign dirty files and local branch remain unchanged.
- B-BR-GATE-DOCKER-01: .dockerignore is a full-code gate input, never docs-only; unknown top-level paths still fail closed and secret scanning remains active.
- B-BR-SNAPSHOT-ONCE-01: concurrent snapshot ticks for the same admitted owner/revision/policy do not publish duplicate ready snapshots; failed writes remain recoverable, private ownership/consent fences remain mandatory.

Final test counts, CI head/base/check URLs and integration/wiki outcome belong in this record or post-merge PR metadata once actually observed. No release, deploy, source activation or certification follows from source integration.

## Reconciliation repairs and local evidence

- Gate failure reproduced by executing the actual Bash selector: .dockerignore refused before, full selection after; three selector tests PASS, existing code/docs routing and unknown-path refusal retained. Harness invokes the regression. No required context/workflow is disabled.
- The old CI ran alerts on Sunday with a Monday–Wednesday schedule; wall-clock integration fixtures correctly produced no candidates. Fixed the injected evaluation time to Tuesday10:00 Sao Paulo and retained explicit Sunday/outside-window negative tests; no production schedule changed.
- The historical Directory index test still hardcoded the complete applied list ending105. It now requires the first pending index migration102 plus unchanged replay/index-plan/retained-row assertions, allowing later independent append-only migrations.
- The initial local Rust artifacts inherited group-write mode775 and were correctly refused by the trusted executable port. Applied755 to this run's temporary preparers, matching CI's explicit chmod; no port was relaxed. Five repaired race/PostGIS packages PASS: alerts/adapters, directory/adapters, quality/adapters/native, quality/adapters/jobs and cmd/worker.
- Original cloud race deterministically reproduced two uploads/ready snapshots with the first object write paused. Fixed scheduled ticks with a per-owner PostgreSQL try-advisory session lease over admission/upload/receipt. A separate connection (bounded by the30s tick/5s connect/close deadlines) keeps the publication pool and owner-row transactions available during object I/O. Busy owners skip this tick; pending metadata/recovery behavior remains unchanged. All cloud adapters/HTTP/domain race/PostGIS tests PASS, including a one-connection publication pool and cancellation releasing the lease for retry; vet/staticcheck PASS. No migration109 conflict or dependency addition.
- Both actual SQLite harnesses PASS (six cases each). Android/build inputs remain identical to d0918d03, so prior1,726 JVM and38 native executions are retained as exact-source evidence.

The initial broad backend matrix executed2,715 passing cases plus13 failing nested cases across four packages (all repaired/environment causes above), four explicit optional case skips and26 packages without tests. Its passing package evidence is not a claim of an all-green aggregate. The final combined109 matrix is recorded only after it actually completes. The occupied other front advanced to its committed9737b22f checkpoint; it was normally merged before publication. Its subsequent uncommitted FQ-07R6C remains outside this candidate and untouched.

# Forum planning evidence — 2026-11-05

Status: LOCAL_DONE
Validation: PASS
Delivery: INTEGRATION_PENDING / PR_PENDING / CI_PENDING

## Authorized scope and source reconciliation

The maintainer requested the complete Forum plan/documentation, a step-by-step
implementation handoff and a GitHub PR. The attachment explicitly prohibits
feature implementation, API/migration/Android edits, dependency installation and
deployment. The subsequent instruction requires **GitHub main as the starting
point**. This package modifies documentation only.

Original checkout: detached `e22824db`, with unrelated tracked/untracked work.
No file there was edited/stashed/committed. Maintained local dev is occupied in
another worktree and was not switched/reset/updated. Created isolated detached
`/tmp/abastevo-forum-plan` from fetched GitHub main `b8e93a41` (PR #190), then
normally fast-forwarded to `3fc77acc6d3dc85819cbc9d00cadaec056448d0a` after
PR #191 merged during research. The final plan uses that reconciled main.
Reviewed the new station-name projection/OpenAPI changes, retained the same
Community card integration seam, and updated migration inventory to 000156.

Verified remote: `git@github.com:AlexandreZanata/Abastevo.git`. Read actual
GitHub repository permissions (push allowed) and main protection: strict
`Quick verification`, GitHub Actions app ID 15368, enforce_admins true;
zero required approving reviews, stale review dismissal enabled. Publication
does not authorize merge or protection changes.

Delivery uses an isolated main-based documentation commit and a normal
fast-forward push to maintained remote dev only if current ancestry permits
and the resulting PR delta is still this documentation package. The occupied
stale local dev checkout remains untouched. Concurrent remote work must be
reconciled/inspected, never overwritten or silently bundled as Forum changes.

## Investigation performed

Read actual current AGENTS, FAST_EXECUTION, current PROGRESS, delivery policy,
ADR-019/020 and selected ROADMAP context. Inspected Community/UI/navigation/
theme/Room/client/test seams, backend module inventory, account/session/proof/
binding, feedback comments/votes/quotas, moderation actions/CLI, existing admin
web/security, job scheduler/worker, account erasure wiring, Official SQL and
alerts/subscription. Exact code references and reuse limits are in the plan's
traceability matrix. A current source file/test is not runtime certification.

Researched official source pages, candidate RSS URLs, publisher terms, CC BY-ND,
ANP price methodology, current ANPD guidance/STF primary materials and PostgreSQL
locking/FTS documentation. Research failures/unsupported RSS rendering and
unverified permissions are retained in the source register. A short guessed ANP
series URL returned 404; it was replaced with the actual official canonical path.
No source was approved/enabled and no publisher contacted.

## Documentation checks and their limits

- Verified FOR-00 through FOR-29 are ordered, unique and present (30 tasks).
- PASS: eight new documents, 58 local links/anchors resolved, 30 ordered unique
  microtasks, 16 B-BR rules and nine BUCs; Markdown fences/newlines/whitespace
  verified. ROADMAP/docs index/current PROGRESS link the package (11 changed files).
- PASS: `git diff --check`, `bash scripts/scan-secrets.sh` and an additional
  direct scan of the eight new Markdown documents for high-confidence credential
  patterns. Manual review found public source/terms URLs only; no private
  account/GPS/provider/storage data. The generic scanner excludes Markdown.
- Initial restricted `make quick-verify` stopped in existing loopback HTTP
  fixture tests: six socket PermissionError failures. This was an environment
  restriction, not a product regression or an accepted pass.
- Repeated with loopback permission and the installed Go 1.27.3/tool binaries;
  **PASS on initial `b8e93a41`**, docs-only selection, 90 seconds. Existing
  OpenAPI lint returned zero errors, 727 warnings and 116 informational findings.
  This is preliminary evidence; the base subsequently advanced, so current-base
  affected validation must supersede it. No warning suppression or code fix.

Actual local invocation (log is local-only, not committed):

```sh
env PATH='/home/iiii/go/pkg/mod/golang.org/toolchain@v0.0.1-go1.27.3.linux-amd64/bin:/home/iiii/go/bin:/usr/local/bin:/usr/bin:/bin' \
  GOCACHE=/tmp/abastevo-forum-go-cache GOTOOLCHAIN=local make quick-verify
```

Current-main validation on `3fc77acc`:

- The repeated aggregate invocation was interrupted by session interruption;
  no uninterrupted current-main `make quick-verify` success is claimed.
- Its local release-workflow, deploy status/probe/surge/agent harnesses and
  Kubernetes manifest checks passed before interruption. Agent summary:
  **229 assertions passed, zero failed**; manifest suite: three tests passed.
  All deployment commands in that harness are disposable stubs, not live calls.
- Completed the remaining affected docs-only contract checks separately:
  `go test -count=1 ./internal/platform/apicontract/...` PASS (0.818s),
  using the PATH/GOCACHE/GOTOOLCHAIN above; `vacuum lint -r
  contracts/openapi/vacuum-rules.yaml contracts/openapi/v1.yaml --no-update-check`
  PASS, zero errors, 727 existing warnings and 116 informational findings.
- Required remote Quick verification remains CI_PENDING until GitHub reports
  its actual current-head result. No merge is attempted in this request.

Final tested revision: documentation diff on `3fc77acc` plus the exact paths
listed by this PR. Content-only evidence annotations do not invalidate matching
code/contract inputs. The local documentation acceptance is complete; external
CI and integration remain separate.

No Forum behavioral/backend/PostGIS/Android/device/load tests were executed:
there is no Forum implementation. Those commands in the validation document are
future obligations, not fabricated passes. No runtime, external permission,
moderator staffing, capacity, staging success or RELEASE_CERTIFIED claim.

## Completion and next action

This planning package is LOCAL_DONE / INTEGRATION_PENDING for documentation
acceptance only. Feature tasks remain PLANNED; the remote required gate and
actual source integration are not accepted by this local status.
Publish one substantive documentation PR through protected dev → main, leave
remote CI/merge truth in the PR and report its actual URL. Stop after planning;
future implementation is selected with the short handoff prompt. No wiki until
actual merge and its independently verified publication scope.

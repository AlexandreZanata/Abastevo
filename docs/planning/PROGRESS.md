# Current execution state

- Updated: 2026-09-28, P01-T17 LOCAL_DONE; G01-FLOW COMPLETE (activation).
- Branch: `codex/phase-01-delivery-flow`, based on `07e7d6d`; clean.
- Main protection ACTIVE: strict, required `[Quick verification]`, PR required (0 approvals, dismiss-stale), enforce-admins, no force/deletion, merge-commits allowed. Switched from `[fast,integration,test]` only after observing Quick verification SUCCESS — no gap. `backend.yml`/`ci.yml` untouched, still path-triggered signal. Red non-required integration/test alone no longer blocks; that evidence stays mandatory at task/phase-exit level.
- Plan rule: after a verified merge, always delete the phase branch locally and remotely, verified. `finish` enforces it (test-flow 23/23).
- P01-T01…T16: LOCAL_DONE. P01-T16: wiki.sh preview/export/publish, test-wiki 31/31; real preview 83 pages; publish deferred (WIKI_PENDING).
- P01-T17: LOCAL_DONE — `quick.yml` (validated schema: all-PR+main triggers, no path filter, read token, concurrency, 12min timeout, fetch-depth 0, `make quick-verify`) committed in `27a5ee8`; controlled phase PR #2 head `27a5ee8`/base `3540011`: Quick verification SUCCESS 5m32s, fast 4m34s, integration 1m17s, test 4m1s; protection switched; PR #2 `clean` under new rules; evidence comment recorded. G01-FLOW activation COMPLETE. Phase READY_FOR_INTEGRATION — merge of PR #2 pending explicit approval (main mutation).
- G01-FLOW: COMPLETE. P02-T01 unblocked after PR #2 merge (needs G01 + G01-FLOW on main).
- Next: **merge PR #2 (explicit approval) or P02-T01 — Shared ANP fixtures** after merge. P02-T01 follows G01 + G01-FLOW.
- Issues/milestone/PR/wiki: flow draft PR pending; wiki once per merged phase; no invented IDs.
- G09 NOT STARTED; P10 BLOCKED BY G09; roadmap 78 tasks.

## Evidence pointers

- [Prior P01 command outcomes preserved verbatim](history/P01_FOUNDATION_EVIDENCE.md).
- [Android baseline re-validation](BASELINE_VALIDATION.md#p01-t01-re-validation--2026-09-28).
- [Workflow policy](DELIVERY_WORKFLOW.md), [CI plan](CI_PLAN.md), [fast execution card](FAST_EXECUTION.md), [ADR-013](../adr/013-fast-phase-delivery.md).
- [Phase record template](templates/PHASE_RECORD.md): use one compact state and task evidence entries, not a growing full transcript in this file.

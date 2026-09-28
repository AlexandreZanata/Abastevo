# Current execution state

- Updated: 2026-09-28, P02-T01 LOCAL_DONE; P01 INTEGRATED.
- Branch: `codex/phase-02-official-catalog`, based on `8c21945`; clean.
- Phase P01+G01-FLOW INTEGRATED: PR #2 merged `2706ba4` → `8c21945` (match-head-commit, required Quick verification SUCCESS); PR #1 auto-closed (commits contained); branches `codex/phase-01-delivery-flow` and `codex/phase-01-delivery-plan` deleted locally + remotely, verified; merge recorded in PR #2 metadata; wiki WIKI_PENDING. Post-merge main CI runs automatically.
- Main protection ACTIVE: strict, required `[Quick verification]`, PR required, enforce-admins, no force/deletion.
- P02 entry G01 + G01-FLOW satisfied. Exit G02 pending; no Android modifications.
- P02-T01: LOCAL_DONE — fixtures + loader tests; quick-packages extended.
- P02-T02: LOCAL_DONE — kernel values, 94 subtests, RED→GREEN.
- P02-T03: LOCAL_DONE — directory repository on real PostGIS, RED→GREEN.
- P02-T04: LOCAL_DONE — stdlib parser, D06 SELECTED, 8 suites RED→GREEN.
- P02-T05: LOCAL_DONE — allowlisted fetch, 11 suites RED→GREEN.
- P02-T06: LOCAL_DONE — `db/migrations/000003` (runs with identity unique, revisions with lifecycle checks, per-survey pointer, station prices with row+natural-key indexes, summaries) + `db/queries/official` + `official/domain` (ReviewGate strict 1%/20% boundaries, tally with bounded samples, stdlib-only) + `adapters` Importer (same-bytes NoOp, superseding corrections, batched staging with conflict quarantine + identical restage, single-tx validate/mark/switch, pointer-gated visibility); integration on real PostGIS: no-op, corrected revision, partial invisible + retry, empty refused, 30% drop review with pointer held, 199/200 conflict quarantine publishing; RED proven by forced-publish (unit + integration), GREEN on restore; manifest extended.
- P02-T07: LOCAL_DONE — geocoder port + throttle/cache, RED→GREEN.
- P02-T08: LOCAL_DONE — contract-first 5 anonymous endpoints (Station/List/Nearby/Groups/History schemas, spec back to 100/100, 7 golden vectors incl. negatives); `platform/httpapi` (HMAC cursors bound to filters + expiry, ApiError envelope, limit/cursor parsing, ETag/304, per-route cache headers) + `ANPFUEL_CURSOR_SECRET` (32B min, per-boot fallback documented); directory/official application validation (bounds, enums, UUIDs, opaque keys) + owned read adapters (keyset pagination, LIKE escaping, WKT mapping, latest-published grouping, community null) + chi handlers + cmd/api wiring with closure-based existence (no cross-module reads); integration on real PostGIS: search/pagination/wildcard/order, nearby exclusion/order/plan-via-GiST/no-leak/no-store, detail ETag/404/honest-nulls, groups current-only + filters + 404, history walk across revisions + scope + tamper-proof cursors; RED proven by dropping the NOT NULL filter (adapter + HTTP), GREEN on restore; manifest extended.
- G02: COMPLETE pending phase integration (PR #3 merge + exit evidence).
- Next: **integrate P02 (merge PR #3) or P03-T01** per authorization.
- Issues/milestone/PR/wiki: flow draft PR pending; wiki once per merged phase; no invented IDs.
- G09 NOT STARTED; P10 BLOCKED BY G09; roadmap 78 tasks.

## Evidence pointers

- [Prior P01 command outcomes preserved verbatim](history/P01_FOUNDATION_EVIDENCE.md).
- [Android baseline re-validation](BASELINE_VALIDATION.md#p01-t01-re-validation--2026-09-28).
- [Workflow policy](DELIVERY_WORKFLOW.md), [CI plan](CI_PLAN.md), [fast execution card](FAST_EXECUTION.md), [ADR-013](../adr/013-fast-phase-delivery.md).
- [Phase record template](templates/PHASE_RECORD.md): use one compact state and task evidence entries, not a growing full transcript in this file.

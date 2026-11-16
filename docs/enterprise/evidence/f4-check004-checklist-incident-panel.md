# EMP-CHECK-004 — Panel: checklists, damages and incidents

Date: 2026-11-16.
Status: LOCAL_DONE
Validation: PASS
Integration: INTEGRATION_PENDING

One bounded F4 web task (module CHECK, P1, type WEB, size M) on
`codex/enterprise-f4-check004` (worktree `/data/dev/worktrees/f4-check004`),
based on the F4 head `e7cbf724` (TRIP-022 integrated). Real dependencies used:
the EMP-CHECK-002 run read, release and `CHECKLIST_RUN` availability cause;
the EMP-CHECK-003 incident queue, read and handling; the EMP-FLEET-007 asset
list and availability read; the EMP-DOC-002/PRIV-005 audited content path;
EMP-PLAT-007/008 company session and shell; the IDN-016, ORG-014 and TRIP-022
panel conventions. Nothing is faked on the server side; synthetic data only.
No migration, Android, job or deployment change.

Read: the EMP-CHECK-001..004 blocks ([EP11](../backlog/ep11-maintenance.md));
[maintenance](../modules/maintenance.md) §§3–12 (BR-CHECK-010/011/016/017,
UC-CHECK-04/05, §7 permissions, SCR-CHECK-04/05); [07-permissions](../07-permissions.md)
BR-AUTH-011 and the §7.3 matrix (`check.incident.manage`, `fleet.asset.read`,
`doc.document.view_content`); [13-ux-web](../13-ux-web.md) §§3–6;
[04-business-rules](../04-business-rules.md) BR-TRUTH-006; EDR-238..241,
283..286 and 304..306; the evidence of PLAT-007, PLAT-008, IDN-016, ORG-014,
DOC-002, PRIV-005, CHECK-001, CHECK-002 (whose Retained list names the panel
lists of runs and of `review_required` runs for this task) and CHECK-003, and
TRIP-022 (module layout, transport allowlist, error mapping, Chromium check).

IDs used: B-BR-EMP-506..510, BUC-EMP-258..260, EDR-328..330 — all of them; no
pre-allocated ID is left unused. Migration 000254 was not needed and is not
used.

## Rules frozen before behaviour

Observed before any change: no run list existed (the CHECK-002 run read needs
the asset and the run id), so neither an organization-wide run list nor a
`review_required` queue could be composed; the incident queue is ORG-only
`check.incident.manage` and orders by id; the release is fresh
`check.incident.manage` with revision CAS, its author `SELF_APPROVAL` (409); the
report read carries attachments with their storage state and never content;
document content is `content/access` (audited capability) then `content/read`
(`content_base64`, JPEG up to 5 MiB) under `doc.document.view_content`, and
probing it would itself issue and record an access; `contexts/read` hints carry
only `idn.*` keys, so the panel had no way to know whether the member holds the
content permission; the company transport bounds JSON at 64 KiB, maps 409 to
`conflict` and drops unknown codes (`SELF_APPROVAL` was not a safe code); the
company CSP is `img-src 'self'` (no `blob:`/`data:`); the TRIP-022 availability
parser refuses the `CHECKLIST_RUN` missing source.

Rules B-BR-EMP-506..510 and use cases BUC-EMP-258..260 are recorded in the
[product contract](../../product/ABASTEVO_EMPRESAS_CONTRACT.md) section
"EMP-CHECK-004 — Panel: checklists, damages and incidents (2026-11-16)";
decisions EDR-328 (the run queue read), EDR-329 (the content hint and the
content action) and EDR-330 (panel placement, composition and retained items)
in [16-decisions](../16-decisions.md). They are the recommended routine
decisions, taken unattended:

- **One minimal backend read (EDR-328).** `POST
  /v1/orgs/{org_id}/checklists/runs/list` under the new `RunQueue` company
  action whose policy is exactly the incident queue's
  (`IncidentPolicy(IncidentQueue)`: `check.incident.manage`, read, not fresh,
  the organization root): an ORG grant covers it, structural, SELF, AUDITOR,
  FINANCE and DRIVER members are refused. Items are the run read's projection;
  order BLOCKING → `review_required` → the rest, newest declared completion
  first, then the run id, with a sealed cursor carrying that key. No
  migration, table, index or permission key; the authorization matrix is
  unchanged (an existing routed permission).
- **The content hint (EDR-329).** `contexts/read` adds
  `doc.document.view_content` when an ORG assignment covers it (a read hint,
  not gated by freshness). The panel offers "Baixar foto" only with the hint
  and a `STORED` attachment, through `content/access` + `content/read` on a new
  transport method (`privateContent`, only those two paths, 8 MiB bound); the
  bytes are handed to the browser as a saved file (the CSP admits no inline
  image) and the capability and bytes are never kept.
- **Server block only (EDR-330).** The opened record's asset availability is
  always the FLEET-007 read, re-read after every acknowledged write; the panel
  has its own availability parser that accepts `CHECKLIST_RUN`. A report's
  `blocks_use` is shown as the reporter's declaration: the report block is not
  applied by the server yet (retained).

## Shape

- Backend (Go): `maintenance/application/runs.go` — `RunQueue`,
  `RunGroup`/`RunQueueKey` (`KeyOf`, `Same`, `Before`, `Cursor`,
  `ParseRunQueueKey`), `RunQueueFilter`, `RunQueuePage`,
  `RunRepository.ListRuns`, `RunService.List` (policy bound before and after;
  every item valid, strictly in queue order after the key, a next key only on a
  full page); `maintenance/adapters/runs.go` — `ListRuns` (queue authority
  before and after in one transaction, each item through the run read's
  `readRun`); `db/queries/maintenance/runs.sql` — `ListChecklistRunQueue`;
  `maintenance/adapters/http/runs.go` — `RunQueuePath`, `RegisterRuns` gains the
  queue authorization, sealed cursor `checklists.runs.queue.v1:<limit>`;
  `organization/adapters/run_authority.go` — the `RunQueue` case (organization
  root only); `enterprisesuite/routes.go` — the incident queue authorization
  passed to `RegisterRuns`; `orgaccess/domain/context.go` — the content hint.
- Contract: one path and two schemas (`EnterpriseChecklistRunListRequest`,
  `EnterpriseChecklistRunPage` whose items are `EnterpriseChecklistRun`) and
  the context hint enum + `maxItems: 4` in `contracts/openapi/v1.yaml`
  (additive, +127/−1 lines); inventory `check.incident.manage` +1 route;
  router test paths +1; route count 161 → 162; AUTH-011 auditor walk 162 / 33
  / 129; apicontract binds the queue route and checks the closed shapes and
  the hint vocabulary.
- Web (TypeScript, no new dependency): `src/checks.ts` (`CheckPanelApi` port
  and strict parsers: a run must satisfy the stored invariants — revision 1
  current, revision 2 released BLOCKING, `review_required` = `outcome_differs`
  = device ≠ server; a report's history must lead to its state and revision; a
  release acknowledgement must be the released run at the next revision; a
  decision acknowledgement must answer the decision — and
  `CheckPanelController` with generation fencing, immutable intents, conflict
  re-read, `SELF_APPROVAL` handling and the content action), `src/checkpanel.ts`
  (DOM, pt-BR copy, focus, availability list, download), the `#checks-panel`
  section in `static/index.html` (existing classes only, no CSS change), the
  transport in `src/api.ts` (panel allowlist; `privateContent`; `SELF_APPROVAL`
  safe code; per-call JSON bound) and the mount in `src/app.ts`. Dist grows
  from 27 to 29 allowlisted files (`check.mjs`, `build.test.mjs`).
- Tests: `organization/adapters/checklist_run_queue_integration_test.go` (2
  PostGIS tests), `TestContextHintsOfferDocumentContentOnlyWithOrganizationCoverage`
  (domain), `TestChecklistRunQueueAndContentHintContractsAreExact`
  (apicontract), `TestRunQueueKeysFiltersAndPolicyAreBounded` (application),
  `business-web/tests/checks.test.mjs` (8 Node tests over the real parsers,
  controller and transport), `business-web/tests/checkpanel.py` (local Chromium
  DOM check with intercepted synthetic routes).

## RED

Logs in `/data/dev/worktrees/f4-logs/check004-*`. Rules, the exported shape
and the frozen tests came first; the store entry point was a zero-page stub
(`ListRuns` returning an empty page), `ContextHints` was the base body, the
web controller and API were compiled stubs (empty methods, API methods
throwing `unavailable`, `privateContent` throwing `denied`, `SELF_APPROVAL` not
a safe code). No expectation was weakened between RED and GREEN.

- Domain hint (`go test -race -json -run TestContextHints
  ./internal/modules/orgaccess/domain/`) → `check004-go-hint-red.jsonl`: 2
  top-level, 1 failing (`TestContextHintsOfferDocumentContentOnlyWithOrganizationCoverage`:
  "ORG content holder without the hint"), the existing hint test passing.
- Contract (spec at the base, `-run TestChecklistRun ./internal/platform/apicontract/`)
  → `check004-contract-red.jsonl`: 3 top-level, 2 failing (the binding test on
  the undocumented queue route; the new shape/hint test), 4 subtests passing.
- Router (route wired, inventory and route lists at the base, `./cmd/api/`) →
  `check004-cmd-red.jsonl`: 3 top-level failing
  (`TestAuthorizationInventoryCoversFullProductionRouter`,
  `TestCompanyProductionCompositionIsOptInAndContractChecked`,
  `TestFullProductionRouterEnterpriseContractAndDefaultOff`), 1 subtest, 21
  passing.
- PostGIS (`-race -tags=integration -p 2 -run '^TestChecklistRunQueue'`,
  OpenAPI and inventory in place, store stubbed) → `check004-db-red.jsonl`:
  2/2 failing at their first behaviour check ("queue order []" after the five
  fixture runs were stored with the expected outcomes; "first queue page []").
- Web (`node --test tests/checks.test.mjs`) → `check004-web-red.log`: 8
  top-level, 8 failing, 0 passing.

## GREEN and REFACTOR

- Domain → `check004-go-hint-green.jsonl`: orgaccess/domain 36/36 top-level,
  1,161 subtests under race.
- Contract → `check004-contract-green.jsonl` 3/3 top-level, 16 subtests;
  router → `check004-cmd-green.jsonl` 24/24 top-level, 25 subtests.
- PostGIS focused (`-run '^(TestChecklistRun|TestAccountContext|TestReviewCorrectionsActualCompanyHTTPMatchesContract)'`)
  → `check004-db-green.jsonl`: 18/19 top-level. The one failure was an
  expectation of the composed HTTP test inconsistent with the frozen rule, not
  a behaviour defect: after the release the run moves to the "other" group,
  where the newer plain run comes first; the assertion now checks that order
  and the released run's history at index 1 (nothing weakened). Rerun
  `check004-db-green2.jsonl`: 2/2.
- The new hint changes two existing expectations, updated with the rule and
  not weakened: the stale-proof context read of an OWNER now lists exactly the
  read hints `idn.employee.read`, `doc.document.view_content` (was exactly the
  first), and the suspended organization's read lists only read hints (was
  only `idn.employee.read`); both still refuse every write control.
- Web → `check004-web-green1.log` 6/8, then `check004-web-green2.log` 7/8,
  then `check004-web-green.log` 8/8. Fixed on the way: the conflict copy now
  carries SCR-CHECK-05's "já atualizado" (implementation); the duplicate
  candidates expectation listed only the open report although B-BR-EMP-509
  admits any listed report of the same asset, so it now expects the open and
  the resolved one (the foreign-asset and unlisted references are still
  refused before transport); the lost-acknowledgement fixture stored an
  `ACKNOWLEDGED` report without its history step, which the strict parser
  rightly refused (fixture fix). Whole suite 67/67 (`check004-web-final.log`).
- Characterization (not counted as RED): `TestRunQueueKeysFiltersAndPolicyAreBounded`
  (the policy is the incident queue's; group/newest/id order; canonical cursor
  round trip and nine non-canonical cursors refused; filter bounds).
- The Chromium check passed at its first run (`check004-web-browser.log`).

## Acceptance and negatives mapped to tests

| Item | Evidence |
|---|---|
| AC: the panel shows the actual asset block | web `the block is never hidden…`: availability only from the server read, read when a run opens and again after the release; a block that remains after the release (another cause) stays visible; a failed read is `null` with "não pôde ser confirmada" and no `AVAILABLE` anywhere in state; Chromium: "Bloqueado", "Checklist bloqueante — este checklist", then "Disponível" only after the server read; PostGIS/HTTP: `BLOCKED` with the `CHECKLIST_RUN` cause naming the run through the composed availability route, still `BLOCKED` after a refused self-release, `AVAILABLE` after the release |
| AC: an authorized release without altering historical evidence | PostGIS/HTTP `TestChecklistRunQueueThroughComposedCompanyHTTP`: release 200 at revision 2, a second 409 `CONFLICT`; the queue item keeps device and server outcome `BLOCKING`, issues and critical issues `[2]`, `released_at`; web: the run is read again (revision 2, `recheck` `BLOCKING`, issues `[2]`, released instant); parsers refuse a released run at revision 1 and a revision 2 still `BLOCKING`; Chromium: "Liberado em", "Resultado conferido pelo servidor … Bloqueando o uso" |
| Negative: self-release where forbidden | web `a self-release surfaces SELF_APPROVAL…`: one write, "outro membro", run still `BLOCKING` revision 1, block still visible, not retryable, not a lost authority; the same for closing an own report; transport keeps the `SELF_APPROVAL` code of a 409; PostGIS/HTTP: the author's release 409 `SELF_APPROVAL` and the queue unchanged; Chromium: "outro membro", retry hidden |
| Negative: hidden block | see the first row; the list keeps the server's attention order (BLOCKING first) and outcome labels |
| Negative: document permission bypass | web `document content is offered only with the content hint…`: without the hint no content action and no request; with it only the `STORED` attachment (never `UPLOADING`, never an unlisted id); exactly `content/access` then `content/read` with the issued capability; no capability or bytes in state; a `403` hides the action and keeps the report; another organization does not inherit the hint; transport: content paths refused by `privatePost`, `privateContent` admits only the two content paths; domain + HTTP: the hint only with an ORG assignment holding the key (SUPERVISOR, UNIT FLEET_MANAGER, SELF DRIVER, expired OWNER: none); Chromium: no button without the hint, a saved `foto-1.jpg` with the exact bytes and nothing kept in the page with it |
| Negative: foreign incident | web `a foreign or vanished report clears the panel…`: unlisted references never sent; a `404 NOT_FOUND` read or decision clears runs, reports, the record and the availability and reveals no id or text; a read answering another report is unusable; a late answer of the previous organization never renders; Chromium: the detail disappears, no private text, focus on the panel heading; PostGIS: another organization's run is never listed |
| Negative: CONFLICT re-reads | web `a CONFLICT re-reads…`: one write, "já atualizado", the newer revision and state from the server, the report and its asset read again, not retryable, the list row refreshed; a stale release re-reads the run; Chromium: "já atualizado" and "alteração concorrente" |
| Scoped list and refusals | PostGIS `TestChecklistRunQueueOrdersAttentionFirstAndIsOrganizationScoped`: exact order of five runs (two BLOCKING newest first, the disagreement, the released and the plain run newest first), every item equal to the run read, pages of 1/2/3 partition the same order with the next key equal to the last item's, a key past the end is empty, invalid filters refused, UNIT FLEET_MANAGER, UNIT SUPERVISOR, AUDITOR, FINANCE and SELF DRIVER refused with no item, the read changes no row (runs, answers, releases, audit, sync operations, sensitive accesses, every run's outcome/revision/updated_at); HTTP: cursor bound to the limit and the route, forged cursor, limit 101 and an unknown field `MALFORMED`, AUDITOR and DRIVER `403` naming `check.incident.manage` |
| Commands keep their contracts | web `commands send exact schema-1 bodies…`: release and the four steps with the revision read, `ACKNOWLEDGE` without note, `DISMISS` with a listed duplicate of the same asset, five distinct operations, only allowed steps offered, a lost acknowledgement blocks new decisions and retries the identical body, a mismatched acknowledgement is unusable, `VALIDATION_FAILED` keeps the record and is not retried |
| Privacy and isolation | no browser storage (Chromium); private responses must be `no-store` (transport); content bounded separately (64 KiB ordinary JSON refused at 200 KiB, content admitted at 200 KiB and refused at 9 MiB); the queue item shape is closed (HTTP, apicontract) |

## Validation

Environment: worktree at `e7cbf724` plus this task, Go 1.27.4 toolchain,
`GOCACHE=/data/dev/cache/f4-go-build`, `GOTMPDIR`/`TMPDIR=/data/dev/cache/f4-tmp`;
shared PostGIS control `127.0.0.1:32775` used only through orgtest isolated
child databases; a throwaway MinIO project `f4-check004-s3` for the S3-backed
suites (torn down at the end); Node 26.3.1 with the existing TypeScript 7.0.2
(installed offline from the lock); Python Playwright Chromium; the reviewed
digest-pinned Caddy image.

- Toolchain `gofmt -l ./internal ./cmd ./db` empty; `go build ./...`,
  `go vet ./...`, `go vet -tags=integration ./...` clean; `sqlc vet` and
  `sqlc diff` clean after `sqlc generate`; `GOTOOLCHAIN=go1.27.1 staticcheck`
  on maintenance, orgaccess, organization, enterprisesuite, apicontract,
  cmd/api and `db/queries/maintenance`, and with `-tags=integration` on
  maintenance, orgaccess, organization/adapters and enterprisesuite: no
  findings (`check004-staticcheck.log`).
- OpenAPI `vacuum lint -r contracts/openapi/vacuum-rules.yaml`: 0 errors,
  1,127 warnings / 682 informs vs 1,126 / 682 at the base `e7cbf724`
  (`check004-vacuum*.log`); the diff is additive apart from the hint enum's
  `maxItems` 3 → 4.
- Route counts on this tree: normal 162 (routed 30 / signed 1 / not routed
  54), specials 26; AUTH-011 auditor walk 162 routes / 33 positive / 129
  FORBIDDEN; authorization matrix unchanged (300 cells, 121 allowed / 179
  denied).
- Affected race/integration union `go test -json -race -tags=integration
  -count=1 -timeout=150m -p 2 ./internal/modules/organization/adapters
  ./internal/modules/maintenance/... ./internal/modules/orgaccess/...
  ./internal/platform/enterprisesuite/... ./internal/platform/apicontract/...
  ./cmd/api/...` with PostGIS and the throwaway MinIO →
  `check004-db-union.jsonl`: 903 top-level / 7,596 subtests passed, 12
  top-level failed before any enterprise code because the log environment
  file named the storage secret variable wrongly ("explicit disposable
  `ANPFUEL_TEST_STORAGE_SECRET_ACCESS_KEY` required"); with the variable fixed
  the 12 (auditor walk, document upload/content/links, exports, signed
  attachment sessions, three `cmd/api` public-process tests) passed 12/12, 131
  subtests (`check004-db-union-rerun.jsonl`). About 30 min on a machine shared
  with other agents (`organization/adapters` 1,757 s, `apicontract` 1,585 s,
  `orgaccess/adapters` 64 s); it includes the authorization matrix (300 cells,
  unchanged) and the AUTH-011 auditor walk (162 / 33 / 129).
- Untagged whole module `GOFLAGS=-p=2 go test -count=1 -timeout=45m ./...`
  → `check004-untagged-all.log`: exit 0, 230 packages ok, 86 without tests, 0
  FAIL.
- Web: `npm run typecheck`, `npm test` (67/67), `npm run build`, `npm run
  check` (29 files) → `check004-web-final.log`; gzip sizes: `checks` 7.5 KiB,
  `checkpanel` 5.3 KiB, `api` 3.8 KiB, `index.html` 7.1 KiB (budget 15 KiB);
  `python3 tests/checkpanel.py` PASS (`check004-web-browser.log`);
  regressions `python3 tests/missionboard.py` PASS
  (`check004-web-missionboard.log`) and `python3 tests/caddy.py` PASS
  (`check004-caddy.log`).
- `cmd/worker` integration tests were not run (untouched; their known
  failures need pinned Rust preparer CLIs this machine lacks).
- The actual-API HTTPS browser harness (`tests/browser.py`) was not extended
  or run for the panel: it would need the checklist, incident, fleet and
  document grants and fixtures in the harness; the actual routes are proved by
  the PostGIS/HTTP suite and the DOM by the Chromium check (retained below).
- `python3 docs/enterprise/tools/check_plan.py` → 0 problems; `git diff
  --check` clean; staged diff reviewed for secrets/PII: synthetic identifiers,
  names, labels and a four-byte synthetic JPEG only; the MinIO validation
  credentials live only in the log environment file (not committed).

## Retained

- The asset block from open damages/incidents with `blocks_use` and a release
  without resolution (BR-CHECK-010/011 for reports: a FLEET-007 source owned by
  the maintenance module with its OpenAPI cause kind and PostGIS proofs).
- The review acknowledgement of a run disagreement (`review_required` stays a
  server flag; acknowledging it needs persisted state and a migration).
- Structural (UNIT/COST_CENTRE/ASSET) run and report queues and the
  SCR-CHECK-04 filters by asset, day, author and template version; an index for
  the queue order when volume evidence exists.
- Run photos (DOC-003 `CHECKLIST_RUN` link kind, EMP-CHECK-002 retained),
  work-order links and "open work order" (EMP-MAINT-003), notifications
  (NOTIF); an inline photo viewer (needs a company CSP decision for
  `blob:` images).
- The actual-API HTTPS browser flow of the panel in `tests/browser.py`.
- Finding for the coordinator, not changed here: the TRIP-022 mission board's
  availability parser refuses the `CHECKLIST_RUN` missing source and labels a
  `CHECKLIST_RUN` cause "Outro motivo"; the agenda would need the same
  vocabulary as this panel.
- Production grants (none new; README subsection) and live composition review.
- No milestone, phase or gate is claimed.

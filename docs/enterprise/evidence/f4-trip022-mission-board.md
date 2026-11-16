# EMP-TRIP-022 — Panel: mission planning and assignment board

Date: 2026-11-16.
Status: LOCAL_DONE
Validation: PASS
Integration: INTEGRATION_PENDING

One bounded F4 web task on `codex/enterprise-f4-trip022` (worktree
`/data/dev/worktrees/f4-trip022`), based on the F4 head `2f031276` (EXP-002
integrated). Real dependencies used: the EMP-TRIP-008 mission store, authority
(`MissionStructure`, `EligibleAssigneeTx`) and routes; EMP-TRIP-009 plans;
EMP-TRIP-013 vehicle change; EMP-FLEET-002/007 asset list, availability,
reservations and blocks; ORG-006 cost-centre list; EMP-PLAT-007/008 company
session and shell; EMP-IDN-016/ORG-014 panel conventions. Nothing is faked on
the server side; synthetic data only. No migration, Android, job or deployment
change.

Read: the EMP-TRIP-022 block and the TRIP-008/009/013/021/023 blocks
([EP06](../backlog/ep06-trips.md)); [trips](../modules/trips.md) §§3, 6, 11
(SCR-TRIP-03); [fleet](../modules/fleet.md) §6.2 and SCR-FLEET-06;
[13-ux-web](../13-ux-web.md) §§3–6; [07-permissions](../07-permissions.md)
BR-AUTH-004/013 and the §7.3 matrix (`trip.mission.manage`, `trip.trip.start`,
`fleet.asset.read`, `org.structure.manage`, `idn.employee.read`);
[04-business-rules](../04-business-rules.md) BR-TRUTH-006;
[17-traceability](../17-traceability.md) REQ-07.2; EDR-057, 064/065, 192..195,
228..231, 246..251; the evidence of PLAT-007, PLAT-008, IDN-016, ORG-014,
TRIP-008, TRIP-009, TRIP-013 and FLEET-007 (whose Retained list names the
SCR-FLEET-06 calendar for this task).

IDs used: B-BR-EMP-470..474, BUC-EMP-239..241, EDR-304..306 — all of them; no
pre-allocated ID is left unused. Migration 000248 was not needed and is not
used.

## Rules frozen before behaviour

Observed before any change: no read tells which employees the assignment
command would accept (`employees/list` shows every employee including
FINANCE/AUDITOR-only members; `roles/list` needs `auth.role.assign`, held only
by OWNER/ADMIN); `contexts/read` hints carry only `idn.*` keys, so the panel
has no `trip.mission.manage` hint; `units/list` and `cost-centres/list` need
`org.structure.manage`, which FLEET_MANAGER and SUPERVISOR planners do not
hold, while every planner role holds `fleet.asset.read` and the asset read
carries the asset's current `unit_id`; `missions/list` is ORG-level; mission
commands are fresh `trip.mission.manage` with revision CAS and their strict
decoders refuse unknown fields; the company transport maps 401
`FRESH_AUTH_REQUIRED` to `fresh`, 403/404 to `denied` and 409 to `conflict`.

Rules B-BR-EMP-470..474 and use cases BUC-EMP-239..241 are recorded in the
[product contract](../../product/ABASTEVO_EMPRESAS_CONTRACT.md) section
"EMP-TRIP-022 — Panel: mission planning and assignment board (2026-11-15)";
decisions EDR-304 (the eligible-assignee read), EDR-305 (board placement and
composition) and EDR-306 (SCR-FLEET-06 as a read-only agenda list view) in
[16-decisions](../16-decisions.md). They are the recommended routine
decisions, taken unattended.

Decisions worth reading first:

- **One minimal backend read (EDR-304).** "Only eligible assignees are shown"
  cannot be met by composition, so `POST
  /v1/orgs/{org_id}/missions/{mission_id}/assignees/list` lists, under the
  mission read policy, exactly the employees for which the assignment
  command's own `EligibleAssigneeTx` is true, scanning candidates in employee
  order inside one read transaction between two authority checks. Items carry
  only `employee_id`, `display_name` and `registration_number`. No migration,
  table or permission key.
- **Opened on demand, the server decides (EDR-305).** With no permission hint,
  the board sends nothing until the member asks; a `403`/`404` on any mission
  read or command clears every private board record and shows fixed copy.
  Creation binds the unit to the chosen vehicle's current unit; destinations
  are text; a refused cost-centre or asset list disables only its picker.
- **Agenda, not calendar commands (EDR-306).** The mission vehicle's
  availability (every cause, evaluation instant), reservations and blocks are a
  read-only list view; purposes, notes and reservation people are not kept.

## Shape

- Backend (Go): `trip/application/missions.go` — `MissionAssigneeFilter`,
  `MissionAssignee`, `MissionAssigneePage`, `MissionRepository.ListMissionAssignees`
  and `MissionService.Assignees` (mission read policy before and after, result
  ordered, bounded, canonical, text bounds of the 000176 profile);
  `trip/adapters/missions.go` — `ListMissionAssignees` and `readTx` (the read
  helper with the transaction, so the authority runs per candidate);
  `db/queries/mission` — `ListMissionAssigneeCandidates` (the stanza now lists
  000176 for `org_employee_profiles`); `trip/adapters/http/missions.go` —
  `MissionAssigneesPath`, sealed cursor bound to route, mission and limit.
- Contract: one path and three schemas (`EnterpriseMissionAssigneesRequest`,
  `EnterpriseMissionAssignee`, `EnterpriseMissionAssigneePage`) in
  `contracts/openapi/v1.yaml` (purely additive, +125 lines); inventory
  `trip.mission.manage` eight routes; router test paths +1; route count 153 →
  154 and the AUTH-011 auditor walk 154 / 31 / 123; apicontract checks the read
  metadata and the closed item shape.
- Web (TypeScript, no new dependency): `src/missions.ts` (`MissionBoardApi`
  port and strict parsers; `parseMission` refuses `ACCEPTED`/`DECLINED`
  without the matching answer; `parseAck` refuses an acknowledgement that does
  not answer the decision; `MissionBoardController` with generation fencing,
  immutable intents and conflict re-read), `src/missionpanel.ts` (DOM, pt-BR
  copy, focus, agenda), the `#mission-board` section in `static/index.html`,
  board styles in `static/assets/business.css` (existing tokens only), the
  transport allowlist in `src/api.ts` (13 board paths; reservation/block
  writes, `mine/list` and units stay refused) and the mount in `src/app.ts`.
  Dist grows from 25 to 27 allowlisted files (`check.mjs`, `build.test.mjs`).
- Tests: `backend/internal/modules/organization/adapters/trip_mission_assignees_integration_test.go`
  (3 PostGIS tests), `TestMissionAssigneeFilterIsBounded` (application),
  `business-web/tests/missions.test.mjs` (8 Node tests over the real parsers
  and controller), `business-web/tests/missionboard.py` (local Chromium DOM
  check with intercepted synthetic routes).

## RED

Logs in `/data/dev/worktrees/f4-logs/trip022-*`. Rules, the exported shape
and the frozen tests came first; the store and the web controller were
compiled stubs (zero page; empty methods and API methods throwing
`unavailable`). No expectation changed between RED and GREEN.

- Router RED (route wired, spec and inventory at the base) `go test -json
  ./cmd/api/` → `trip022-cmd-red.jsonl`: 4 top-level failing
  (`TestAuthorizationInventoryMatchesRegisteredMeterDescriptor`,
  `TestAuthorizationInventoryCoversFullProductionRouter`,
  `TestCompanyProductionCompositionIsOptInAndContractChecked`,
  `TestFullProductionRouterEnterpriseContractAndDefaultOff`), 1 subtest; 20
  passing. Contract RED (`-run TestTripMission ./internal/platform/apicontract/`)
  → `trip022-contract-red.jsonl`: 1/2 failing (missing
  `EnterpriseMissionAssigneesRequest`).
- PostGIS RED (`-race -tags=integration -p 2 -run '^TestTripMissionAssignees'`)
  against the zero-page store stub: `TestTripMissionAssigneesScopeIsolationAndAuditor`
  ("the unit planner lists the eligible assignees of its unit's mission") and
  `TestTripMissionAssigneesThroughComposedCompanyHTTP` ("HTTP eligible
  assignees 0") → `trip022-db-red-4.jsonl`;
  `TestTripMissionAssigneesListExactlyTheEligibleEmployees` ("an eligible
  member is missing from the list") → `trip022-db-red-6.jsonl`. Earlier
  attempts of that first test failed for fixture reasons, not behaviour, and
  were fixed before the genuine RED: the runtime role lacked SELECT/INSERT on
  `org_employee_profiles` (`trip022-db-red.jsonl`), the owner connection is
  refused by `WithOrgTx` and by forced RLS (`-2`, `-3`), a SUSPENDED
  membership needs `suspension_origin` (`-4`) and a merged employee must be
  inserted ACTIVE then terminated and merged (`-5`).
- Web RED (`node --test tests/missions.test.mjs`) → `trip022-web-red.log`:
  8 top-level, 8 failing (assertions and null selections against the stub
  controller; the allowlist test on `denied` paths), 0 passing.

## GREEN and REFACTOR

- PostGIS → `trip022-db-green.jsonl`: 3/3 at the first implementation
  (3.3 s each under race).
- Router and contract after the OpenAPI blocks and the inventory line →
  `trip022-cmd-green.jsonl` 24/24 top-level, `trip022-contract-green.jsonl`
  2/2.
- Web → 8/8 at the first implementation; whole suite 59/59
  (`trip022-web-green.log`, `trip022-web-final.log`); typecheck, build and
  artifact check (27 files) pass; gzip sizes: `missions` 6.7 KiB,
  `missionpanel` 5.0 KiB, `index.html` 6.5 KiB (budget 15 KiB).
- Refactor: `parseMission`'s ASSIGNED invariant simplified; the failed-read
  message after an acknowledged write says the write was confirmed.
- Found by the Chromium check and fixed (`trip022-web-browser.log`): the detail
  heading lost its focus move because the first render of a newly opened
  mission happens while its reads are still pending; focus now moves once per
  opened mission after the reads settle. The check itself first sampled the
  focus too early; it now waits with `to_be_focused`. Then PASS.
- Characterization (not counted as RED): `TestMissionAssigneeFilterIsBounded`
  (bounds, canonical cursor, non-canonical mission and invalid filter refused
  before any repository call); the build test's board accessibility strings.

## Acceptance and negatives mapped to tests

| Item | Evidence |
|---|---|
| AC: only eligible scoped assignees are shown | PostGIS `TestTripMissionAssigneesListExactlyTheEligibleEmployees`: for every employee of the organization, listed ⟺ the assignment command accepts it on a fresh mission with the same unit and vehicle (north and south missions); FINANCE, AUDITOR, SUSPENDED, ENDED+INACTIVE, account-less and merged employees are absent; a UNIT-scoped SUPERVISOR is listed for its unit's mission and not for the other unit's; web: options are exactly the server list, the current assignee is not offered again |
| AC: only eligible scoped assets are shown | web parser keeps only ACTIVE, non-`IMPLEMENT` assets with a unit (`DRAFT` and implement fixtures dropped); the Chromium check shows two vehicle options plus the placeholder; vehicle change never offers the current vehicle |
| AC: the board reports current server state | web: state is the last list/read with `listedAt`/`readAt`; every acknowledged command is followed by a read that replaces the row; Chromium: "Lista consultada em …", "Situação consultada no servidor em …", agenda "avaliada em …" (BR-TRUTH-006) |
| Foreign assignment hidden | web `foreign or stale choices are refused before transport and never shown`: an employee outside the eligible list, an implement, a draft asset, the current vehicle and an unknown cost centre send nothing; a read answering another mission is unusable and not shown; a late answer of the previous organization never renders; PostGIS: another organization's mission `NOT_FOUND`, HTTP 404 through the other organization's path, the item shape is closed (apicontract) |
| Concurrent update not hidden | web `a concurrent update surfaces as CONFLICT…`: one write, `conflict` shown, the newer revision and assignee read from the server, no retry possible, the list row refreshed; Chromium: "A missão mudou" with "Revisão 3" and "alteração concorrente" |
| Unconfirmed state never presented as accepted | web `an unconfirmed state is never presented as accepted`: the pending command leaves DRAFT shown; the ack shows "Designada — aguardando resposta"; an ack claiming `ACCEPTED` or another employee is unusable (state unchanged, same-operation retry only); a lost ack blocks a new decision and retries the identical body; `ACCEPTED` appears only from a read with the answer and a read claiming it without one is refused; Chromium: no "Aceita" after assignment, "Aceita pela pessoa designada" only after the server read |
| No write without the permission (server 403) | web `no write without the permission…` over the real `CompanyApi`: a `403 FORBIDDEN` on cancel and on the list clear missions, selection, assets, assignees and agenda and keep no mission, vehicle, employee or title text; `NOT_FOUND` the same; `FRESH_AUTH_REQUIRED` ends the session view; Chromium: the detail disappears and focus moves to the board heading; PostGIS/HTTP: DRIVER, AUDITOR and an out-of-scope UNIT planner `FORBIDDEN` naming `trip.mission.manage` on the read |
| Exact command contracts | web `commands send exact schema-1 bodies…`: create/plan (three load shapes, cost centre omitted when empty)/assign/vehicle/cancel bodies match the strict decoders, revisions are the ones read, seven distinct operation ids |
| Read changes nothing; pagination | PostGIS: asset/trip/reservation/block/review snapshot and every mission head, transition, receipt and audit count unchanged by the read; limit 1 yields empty pages with an advancing cursor; one large page lists the same set; HTTP: sealed cursor refused on another mission or limit, forged cursor, limit 101 and an unknown field `MALFORMED` |
| Privacy | agenda keeps no purpose, note or reservation employee (web and Chromium); assignee items have exactly three fields (HTTP, apicontract); no browser storage (Chromium); private responses must be `no-store` (transport) |

## Validation

Environment: worktree at `2f031276` plus this task, Go 1.27.4 toolchain,
`GOCACHE=/data/dev/cache/f4-go-build`, `GOTMPDIR`/`TMPDIR=/data/dev/cache/f4-tmp`;
shared PostGIS control `127.0.0.1:32775` used only through orgtest isolated
child databases; a throwaway MinIO project `f4-trip022-s3` for the S3-backed
suites (torn down at the end); Node 26.3.1 with the existing TypeScript 7.0.2
(installed offline from the lock); Python Playwright Chromium.

- Toolchain `gofmt -l ./internal ./cmd ./db` empty; `go build ./...`,
  `go vet ./...`, `go vet -tags=integration ./...` clean; `sqlc vet` and
  `sqlc diff` clean after `sqlc generate`; `GOTOOLCHAIN=go1.27.1 staticcheck`
  on trip, apicontract, cmd/api and `db/queries/mission`, and with
  `-tags=integration` on trip and organization/adapters: no findings
  (`trip022-staticcheck.log`).
- OpenAPI `vacuum lint -r contracts/openapi/vacuum-rules.yaml`: 0 errors,
  1,113 warnings / 681 informs vs 1,109 / 681 at the base `2f031276`
  (`trip022-vacuum*.log`); the diff is purely additive (+125 lines).
- Route counts on this tree: normal 154 (routed 30 / signed 1 / not routed
  54), specials 26; AUTH-011 auditor walk 154 routes / 31 positive / 123
  FORBIDDEN; authorization matrix unchanged (300 cells).
- Affected race/integration union `go test -json -race -tags=integration
  -count=1 -timeout=120m -p 2 ./internal/modules/organization/adapters
  ./internal/modules/trip/... ./internal/platform/enterprisesuite/...
  ./internal/platform/apicontract/... ./cmd/api/...` (with the throwaway
  MinIO) → `trip022-db-union.jsonl`: 8 packages ok, 820 top-level / 7,052
  subtests passed, 0 failures, 0 skips, about 63 min on a machine shared with
  two other unions (`organization/adapters` 3,778.6 s, `apicontract`
  2,948.3 s, `cmd/api` 244.0 s); it includes the authorization matrix (300
  cells, 121 allowed / 179 denied, unchanged) and the AUTH-011 auditor walk
  (154 / 31 / 123). A comment-only tidy of the store's doc comment was made
  while it ran (no code change).
- Untagged whole module `GOFLAGS=-p=2 go test -count=1 -timeout=45m ./...`
  → `trip022-untagged-all.log`: exit 0, 229 packages ok, 87 without tests, 0
  FAIL.
- Web: `npm run typecheck`, `npm test` (59/59), `npm run build`, `npm run
  check` (27 files) → `trip022-web-final.log`; `python3 tests/caddy.py` PASS
  (`trip022-caddy.log`, the reviewed digest-pinned Caddy image); `python3
  tests/missionboard.py` PASS (`trip022-web-browser.log`).
- `cmd/worker` integration tests were not run (untouched; their known
  failures need pinned Rust preparer CLIs this machine lacks).
- The actual-API HTTPS browser harness (`tests/browser.py`) was not extended
  or run for the board: it would need the mission, fleet and cost-centre
  runtime grants and fixtures in the harness; the actual routes are proved by
  the PostGIS/HTTP suite and the DOM by the Chromium check (retained below).
- `python3 docs/enterprise/tools/check_plan.py` → 0 problems; `git diff
  --check` clean; staged diff reviewed for secrets/PII: synthetic identifiers,
  names and labels only; the MinIO validation credentials live only in the
  log environment file (not committed).

## Retained

- The actual-API HTTPS browser flow of the board in `tests/browser.py`
  (mission/fleet/cost-centre grants and fixtures in the harness).
- SCR-FLEET-06 reservation request/confirm/cancel and block/unblock commands,
  the grid calendar rendering and trips in progress as their own rows; more
  than one page of reservations/blocks in the agenda (EDR-306).
- Board filters, the mission transition history read and a plan history
  (TRIP-008/009 retained lists); a unit destination picker and missions
  without a vehicle from the panel (EDR-305); a `trip.mission.manage` context
  hint.
- The trip link and `IN_PROGRESS`/`COMPLETED` (EMP-TRIP-021/023); the app
  inbox (EMP-TRIP-021); notifications (NOTIF).
- Production grants (`SELECT ON org_employee_profiles` for the company
  principal, README subsection) and live composition review.
- No milestone, phase or gate is claimed.

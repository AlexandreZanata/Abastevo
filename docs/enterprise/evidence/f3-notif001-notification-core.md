# EMP-NOTIF-001 — Notification core: inbox, preferences, de-duplication, limits and history

Date: 2026-11-14.
Status: LOCAL_DONE
Validation: PASS
Integration: INTEGRATION_PENDING

One bounded F3 DB/API task in its own worktree
(`codex/enterprise-f3-notif001`). The coordinator owns the shared docs, backlog
state, PROGRESS, pushes and the PR. This task adds the module
`backend/internal/modules/orgnotify` (domain, application, adapters, HTTP),
migration `000204_org_notifications.sql`, its sqlc package, four member-facing
self routes, their OpenAPI contract and route-gate/inventory entries. No
Kotlin/Android (the app centre is EMP-NOTIF-004), no external provider (EDR-032),
no production producer. Synthetic data only.

Rules read: REQ-17.2 ([traceability](../17-traceability.md)); EDR-032
([decisions](../16-decisions.md)); BR-PRIV-003, BR-PRIV-008
([privacy](../11-privacy-security.md)); BR-AUTH-014
([permissions](../07-permissions.md), §9 notification duties, catalog keys);
BR-NOTIF-001, UC-NOTIF-01, §§6–10 and SCR-NOTIF-01
([module](../modules/imports-notifications-integrations.md)); BR-TEN-007;
Notification `PENDING`/`DELIVERED`/`FAILED`/`SUPPRESSED`
([state model](../08-state-model.md)); the EMP-NOTIF-001..004 task blocks
([EP18](../backlog/ep18-imports-notifications-integrations.md)) so the API serves
them. Reused, not modified: `orgevent` (EMP-PLAT-010, [evidence](f1-plat010-transactional-events.md)),
`orgaudit`, `orgaccess` current authority (EMP-AUTH-003) and its permission
catalog, the job queue, `companysession`, `orgquota`. Extended additively:
`orgapi` (one `SelfNoticeRule` descriptor), the `apicontract` route gate, the
`enterprisesuite` composition, `cmd/api` authorization inventory oracle,
`contracts/openapi/v1.yaml`, `authorization-routes-v1.json`, `sqlc.yaml` and the
quick manifest. Precedent: EMP-FLEET-002
([evidence](f2-fleet002-asset-persistence.md)).

## Rules frozen before behaviour

Pre-allocated IDs: B-BR-EMP-215..218, BUC-EMP-110..111, EDR-150..151. All are
used; none remains reserved.

Proposed contract section title: "Enterprise notification core (EMP-NOTIF-001)".

Rules B-BR-EMP-215, B-BR-EMP-216, B-BR-EMP-217, B-BR-EMP-218, BUC-EMP-110, BUC-EMP-111 are recorded in the [product contract](../../product/ABASTEVO_EMPRESAS_CONTRACT.md) section "EMP-NOTIF-001 — Notification core: inbox, preferences, de-duplication, limits and history" (frozen before behaviour; text moved verbatim at integration).

Decision rows (EDR-150..151), recommended routine decisions taken unattended:

Decisions EDR-150, EDR-151 are recorded in [16-decisions](../16-decisions.md).

## Shape frozen before behaviour

Module `backend/internal/modules/orgnotify` (EDR-150):

- `domain`: closed `Catalog` of composition-registered `Kind`s (name, optional
  catalog `Permission`, `Mandatory`, template keys, typed `Param`s of
  `ParamID`/`ParamCode`(enumerated)/`ParamCount`/`ParamDate`, optional relative
  `Link` template over UUID params); `Request` → `Prepared` (`Prepare`,
  `PrepareAll`) with canonical params JSON, rendered link and content digest;
  `Screen` (eligibility before preference, mandatory ignores mute); `Channel`
  (only `IN_APP` enabled; `ANDROID_LOCAL`, `PUSH`, `EMAIL` disabled);
  `Outcome`/`Reason`; `CommandDigest`; bounds `MaxRecipients=50`,
  `MaxRequestsPerEvent=10`, `MaxParams=8`, `HourlyLimit=20`; `ConsumerName =
  "notif.dispatch"`; redacted fmt/slog/JSON views of requests.
- `application`: `Service` (List, MarkRead, Preferences, SetPreference) over a
  `Repository`; the `Producer` port `RequestTx(ctx, tx, scope, orgevent.Event,
  []Request)` called inside the producer's own audited orgevent transaction.
- `adapters`: `Store` (producer port, `Dispatch` orgevent effect, member
  commands) using AUTH-003 `LoadAuthorityTx` in the same scoped transaction;
  `Migration = "000204_org_notifications.sql"` (tests derive the version from
  it, so a coordinator renumbering touches the file name, this constant and the
  sqlc stanza only).
- `adapters/http`: four `SELF_NOTICE` routes (EDR-151) registered from
  `enterprisesuite` with a dedicated `notif.self` write admission; new
  `orgapi.SelfNoticeRule` descriptor pinned by the route/contract gate and the
  authorization inventory. `enterprisesuite.Options.NoticeKinds` is empty in
  production; producers add kinds and compose `notif.dispatch` later.

Migration 000204 (append-only, additive, forced RLS on all seven tables, no
seeded kind/job/grant): `org_notification_requests` (immutable; FK to the
source `org_audit_events` operation; unique `(org_id, idempotency_key)`;
`dispatched_at` set once; deferred check that recipients match
`recipient_count`), `org_notification_recipients` (FK to `org_memberships`),
`org_notification_notices` (identity `position` keyset; only `read_at` set once;
one per request and recipient), `org_notification_deliveries` (append-only
history, `channel='IN_APP'`, DELIVERED with notice or SUPPRESSED with
INELIGIBLE/MUTED/LIMITED), `org_notification_preferences` (revision starts at 1,
advances by one, no delete), `org_notification_budgets` (fixed UTC-hour windows,
`used` 1..20, only counts up) and `org_notification_commands` (append-only member
receipts with request digest).

Notification states (08-state-model): PENDING = request not yet claimed;
DELIVERED/SUPPRESSED = per-recipient IN_APP history; FAILED is reserved for the
external channels of EMP-NOTIF-002 and is not producible now.

## RED → GREEN

Stubs compiled with the final exported API (zero bodies / `ErrUnavailable`,
`NewCatalog` returning `ErrInvalid`, `Register` mounting nothing), then tests,
then implementation. Logs in `/data/dev/worktrees/f3-logs/`:

- RED unit `notif001-go-red-unit.jsonl`: 14 top-level / 24 subtests; 13 fail,
  1 pass (`TestCatalogRejectsInvalidDefinitions` passes against the
  always-refusing stub), 0 skip.
- RED contract `notif001-go-red-contract.jsonl` (`-run
  'TestSelfNotice|TestActualCompanyRoutes'`): 3 top-level; 2 fail (routes in the
  contract but not composed), 1 pass (schema examples already in the contract).
- RED inventory `notif001-go-red-inventory.jsonl`: 3 top-level / 15 subtests;
  `TestAuthorizationInventoryCoversFullProductionRouter` fails (inventoried
  SELF_NOTICE routes missing), 2 pass.
- RED integration `notif001-go-red-integration.jsonl` (real PostGIS, stub
  store): 12 top-level, 12 fail, 0 skip. Eleven fail on the stub catalog/store;
  the migration test failed on its own fixture (identity-only organization root
  without scope, 42501). The fixture was corrected to create the populated root
  through the real organization store and ordinary runtime role; no assertion
  was weakened.
- GREEN unit `notif001-go-green-unit.jsonl` (`-race`): 14 / 81, 0 fail, 0 skip.
- GREEN integration `notif001-go-green-integration.jsonl` (`-race
  -tags=integration`, real PostGIS 3.6 / PostgreSQL 18 via the owned disposable
  control DB on 32775 and orgtest child databases with ordinary
  NOBYPASSRLS roles): 24 / 106, 0 fail, 0 skip; adapters 16.0 s, http 6.6 s.
- GREEN contract `notif001-go-green-contract.jsonl` (`-race`
  apicontract/orgapi/enterprisesuite): 89 / 641, 0 fail; apicontract 480.7 s.
- GREEN inventory `notif001-go-green-inventory.jsonl` (`-race`): 3 / 23, 0 fail.
- Coverage (`-coverpkg=./internal/modules/orgnotify/...`, integration):
  domain 97.7 %, application 100 %, adapters 81.7 %, adapters/http 84.0 %,
  total 87.0 % (`notif001-go-coverage.out`).

REFACTOR: dispatch now evaluates all claimed (request, recipient) pairs in one
global (membership, kind, request) order so budget-row locks cannot deadlock
across concurrent events; a cross-event concurrent dispatch case was added and
the full integration run re-passed.

Integration cases (BUC-EMP-110/111): end-to-end test producer through orgevent,
the real job queue and handler with a `/reports/{id}` panel link to the
authorized OWNER/FINANCE recipients; lost role, never-held permission, ended
and suspended members get INELIGIBLE history and no notice (no late notice
after reactivation); repeated and redelivered dispatch change nothing; muted
optional kind → MUTED, mandatory kind delivered despite a stored mute; 1+21
alerts → 20 DELIVERED + 2 LIMITED with no silent drop; idempotency key returns
the first request and refuses changed content with the whole producer
transaction rolled back; 12 concurrent duplicate producers → one request; 12
concurrent dispatchers → one notice per recipient; 6 concurrent events → exact
budgets; attachment/free-text/foreign/unknown recipients, unaudited source,
mismatched scope and forged source refused; forced RLS hides all seven tables
without or with a foreign scope and refuses unscoped/foreign inserts; append-only
and single-transition guards refuse rewrites even for the migration owner;
replay/reuse/revision/ownership of member commands incl. 12 concurrent identical
and 12 concurrent competing preference commands; keyset pagination; populated
previous chain → 000204 failure/recovery/retry/previous-compatibility/no-seed.
HTTP (real company cookie, composed `enterprisesuite` router, contract-validated
responses): own list/unread/cursor binding, wrong member/org/account, missing
CSRF/cookie, mark read replay/reuse/foreign, preferences list/mute/replay/
reuse/stale/mandatory/unknown, ended membership refused at the next request.

## Validation

Commands from `backend/` with the pinned environment (`PATH` go1.27.4,
`GOCACHE=/data/dev/cache/f3-go-build`, `GOTMPDIR`/`TMPDIR=/data/dev/cache/f3-tmp`,
`STATICCHECK_CACHE=/data/dev/cache/f3-staticcheck`,
`ANPFUEL_TEST_DATABASE_URL=postgres://anpfuel:…@127.0.0.1:32775/anpfuel`):

```sh
gofmt -l ./internal/modules/orgnotify ./internal/platform ./cmd/api      # empty
go build ./... && go vet ./...                                          # notif001-build-vet.log: OK
go test -json -count=1 ./...                                            # notif001-go-untagged-all.jsonl
go test -json -count=1 -race -tags=integration -coverpkg=./internal/modules/orgnotify/... ./internal/modules/orgnotify/...
go test -json -count=1 -race ./internal/platform/apicontract ./internal/platform/orgapi ./internal/platform/enterprisesuite
go test -json -count=1 -race ./cmd/api -run TestAuthorizationInventory
go test -json -count=1 -race -tags=integration ./internal/modules/organization/adapters ./internal/modules/orgaccess/adapters/http ./internal/platform/enterprisesuite
GOTOOLCHAIN=go1.27.1 staticcheck ./internal/modules/orgnotify/... ./internal/platform/apicontract ./internal/platform/enterprisesuite ./internal/platform/orgapi ./cmd/api
GOTOOLCHAIN=go1.27.1 staticcheck -tags=integration ./internal/modules/orgnotify/...
sqlc generate && sqlc vet && sqlc diff
(repo root) vacuum lint -r contracts/openapi/vacuum-rules.yaml contracts/openapi/v1.yaml --no-update-check
(repo root) python3 docs/enterprise/tools/check_plan.py
```

Results: build/vet OK; untagged full suite 199 packages pass (74 without
tests), 2487 top-level / 6863 subtests pass, 4 pre-existing skips, 0 failures;
staticcheck clean (`notif001-staticcheck.log`); sqlc vet/diff clean
(`notif001-sqlc.log`); vacuum 0 errors (warnings 901 → 909 and infos 570 → 579,
the same requestBody-example and duplicate-description classes already raised
by the existing self-device routes; `notif001-vacuum.log`); check_plan 0
problems; `git diff --check` clean; secret/PII scan hits only identifiers
(`LeaseToken`, `CursorSecret`, `csrf_token`), runtime-generated test session
values and synthetic negative strings (`secret-token`); no real secret, name,
CNPJ, contact or coordinate.
Affected composition consumers (`-race -tags=integration`, real PostGIS):
organization adapters, orgaccess HTTP and enterprisesuite, which all mount the
composed router with the new routes: 345 top-level / 2379 subtests pass, 0 fail
(organization adapters 664.3 s; `notif001-go-affected-composition.jsonl`).

## Proposed shared-doc changes

- `backend/README.md`, new section "Organization notification core
  (EMP-NOTIF-001)": "Migration204 installs forced-RLS notification requests,
  recipients, notices, delivery history, preferences, hourly budgets and member
  receipts. The runtime principal needs `GRANT SELECT,INSERT ON
  org_notification_recipients, org_notification_deliveries,
  org_notification_commands TO runtime_role;` and `GRANT SELECT,INSERT,UPDATE ON
  org_notification_requests, org_notification_notices,
  org_notification_preferences, org_notification_budgets TO runtime_role;`
  (UPDATE only for the single dispatch claim, read mark, revisioned preference,
  budget count and row locks; guards refuse every other change), plus the
  existing AUTH-003 current-authority reads. No kind, producer or dispatch
  worker is enabled; producers compose `notif.dispatch` in the existing
  Dispatcher before activation."
- `docs/enterprise/07-permissions.md` §9: "Own inbox and notice preferences use
  the special SELF_NOTICE boundary (account-bound current membership, no
  catalog permission); a dedicated inbox permission is an AUTH-owned future
  decision."
- `docs/enterprise/08-state-model.md`, Notification row note: "PENDING =
  registered request not yet dispatched; DELIVERED/SUPPRESSED are per-recipient
  IN_APP history (reasons INELIGIBLE, MUTED, LIMITED); FAILED belongs to the
  external channels of EMP-NOTIF-002."
- `docs/enterprise/modules/imports-notifications-integrations.md` §12: cite
  EDR-150/151 and the B-BR-EMP-215..218 rules.
- `docs/enterprise/09-architecture.md` module list: add `orgnotify` (in-app
  notification core on orgevent and the existing queue).

## Retained obligations

- Production producers (EMP-RPT-004 report jobs, approvals, alerts) register
  their kinds and compose the `notif.dispatch` effect in the existing
  Dispatcher; no producer, kind or worker is activated here.
- An inbox catalog permission (if wanted) belongs to the AUTH owner; the
  permission catalog was not edited.
- Android signed inbox entry for managed-access members and lock-screen rules:
  EMP-NOTIF-004. Panel/app notification centres: EMP-NOTIF-004.
- External channels, attempts, retries and FAILED outcomes: EMP-NOTIF-002 with
  its delivery decision (EDR-032). Alert rules/subscriptions: EMP-NOTIF-003.
- Retention/erasure of notices, history, receipts and expired budget windows
  (privacy/retention owner); production quota measurement for `notif.self`;
  runtime grant provisioning and backup/restore rehearsal (README proposal).
- Operator delivery-history views and dead-job operations remain with their
  owning tasks. No F3/M3 acceptance, deployment or release is implied.

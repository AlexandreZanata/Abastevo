# F1 EMP-PLAT-013 private request telemetry

Status: LOCAL_DONE
Validation: PASS

2026-11-11. Scoped task acceptance; INTEGRATION_PENDING through
[PR202](https://github.com/AlexandreZanata/Abastevo/pull/202). F1/M1 remain open.

## Entry and related correction

The owner requested fixing home_open_map lint, implementing the next eligible
task and publishing. Started from clean ec92ab38, matching remote dev/PR202;
main ef96cd13 remains an ancestor. The occupied local dev and primary
checkout's unrelated Android/source changes are untouched.

First, da8ee090 adds all six missing translations. Complete app lint passed
offline with zero errors and 347 existing warnings, without suppression,
baseline or threshold changes.
[Localization evidence](../../mobile/home-open-map-localization.md).
Earlier failed-lint records retain their historical truth and are superseded
for current readiness by this successful run.

EMP-PLAT-012 still requires EMP-GOV-014's external-channel decision.
EMP-PLAT-013 is the next eligible platform task, depending on locally accepted
EMP-PLAT-004. B-BR-EMP-27, B-BR-EMP-28 and BUC-EMP-15 were written before code.
No external channel, provider contact or sending authority is inferred.

## Implemented scope and sink inventory

- **Existing private metrics registry/listener.** cmd/api composes one
  orgapi.Observe middleware. org_requests_total and
  org_request_seconds_total use only source route template/unmatched,
  success/refused/unavailable and allowlisted orgapi codes or OK/REFUSED.
  WriteError records its normalized code in request-local state. No body,
  response content, raw provider error or private identifier is inspected
  for telemetry. The existing 512-series cap and dropped-series counter
  apply. Seconds are cumulative, not a latency histogram.
- **HTTP access/recovery.** The existing factory encloses private handlers
  in private context and assigns fresh server correlation instead of trusting
  inbound request IDs. Access logs retain source route, bounded method,
  actual status and elapsed time. Unknown wire methods fold to OTHER.
  Private unmatched/method errors produce one access record, not duplicates.
  Panic value/stack content is not logged; existing private recovery remains
  responsible for the response.
- **Generic slog calls.** Private context discards arbitrary messages,
  attributes and inherited client/job/bound-route metadata. Only fixed
  event codes and server correlation survive. A separate transport-owned
  logger method emits access fields. Public logging policy remains intact.
  Callers must use the supplied request context, and raw URLs never belong
  in the transport route-template parameter.
- **Default formatting.** Shared session/command/actor/membership/principal/
  cursor/gateway values redact fmt and slog. Embedded credentials propagate
  that default to module DTOs; JSON wire serialization is unchanged. Existing
  orgmedia, companysession and orgevent redaction remains in its own code.
  EMP-PLAT-010 already bounds persisted queue errors.
- **Edge and future sinks.** Canonical Caddy access/runtime filters remove
  request and response-header objects; staging static configuration has no
  access-log directive. Reviewed configuration only, with no proxy change
  or live certification. Feature-specific attachment/review/tracking metrics
  and alert thresholds remain owned by their modules. No trace exporter or
  parallel logging/metrics service is introduced.

The existing global HTTP observer shares the bounded method vocabulary.
Response wrappers now preserve first committed status, implicit writes,
flush and ResponseController access, so later attempted headers cannot
rewrite access/metric status. Organization observation runs inside recovery:
panic is counted safely, then repanicked for the existing response handler.
Private application errors remain distinct from append-only audit authority.

## Executed checks

Linux amd64, Go 1.27.4, pinned staticcheck v0.8.1 with the documented
go1.27.1 compiler workaround. PostgreSQL18/PostGIS3.6 used the unique tmpfs
Compose project enterprise-plat013-20261111 with loopback-only access.
Fixtures own disposable databases/logins. The project was removed afterward.
All identities, coordinates, credentials and document markers are synthetic;
no staging/provider/production data was used.

- RED: tests failed because private-context logging and orgapi.Observe did
  not exist. GREEN: seeded sinks pass, including arbitrary method, client
  trace, bound route, raw provider error, principal, cookie/CSRF/signature,
  coordinates, cursor, document body and panic content.
- `go test -race ./internal/platform/telemetry/... ./internal/platform/orgapi ./internal/platform/httpserver ./internal/modules/companysession/... ./internal/platform/apicontract ./cmd/api -count=1`
  passed on the final runtime implementation. Contract tests also passed;
  no OpenAPI route/security declaration changes.
- The complete HTTP pipeline verifies success, conflict, unavailable provider,
  panic, malformed body, disabled route, unknown/method-mismatched route,
  exact single access count, implicit/explicit status and flush.
  One hundred concurrent requests across two organizations produce exactly
  fifty OK and fifty FORBIDDEN observations without private values.
  Public client correlation is not promoted to trusted private metadata;
  the final boundary/cmd/api race regression also covers that provenance guard.
- Factory tests reject nil/conflicting metric registration, exclude public
  requests and normalize unknown typed/private provider errors. DTO fmt/slog
  tests exercise all shared redactors, including promoted embedded-session
  redaction.
- `go test -race -tags=integration ./internal/platform/orgapi ./internal/modules/companysession/... ./internal/platform/database -count=1`
  passed with real PostGIS. Existing live account/key/session checks cover
  nonce races, changed body, wrong signer/owner, foreign/ended membership,
  revoked key/family and suspended account. The signed gateway test now
  also verifies safe result-code exposition and absence of fixture IDs/tokens.
- Final cross-package race/integration coverage passed for telemetry,
  metrics, orgapi and httpserver, using -coverpkg for those same four packages.
  New private-context/logger file: 100%; observer: 93.0%; shared redactors:
  100%; private access boundary: 100% statement coverage. Percentage does
  not replace the failure/concurrency checks above.
- Affected go vet and pinned staticcheck passed; API/worker/ops builds passed.
  No SQL, dependency or schema change; no sqlc/OpenAPI regeneration required.
- The actual new API/migrator ran against a separate disposable database.
  It returned ready 200, private unknown-route 404 with server-generated
  trace and exactly one safe private metric. Private /metrics exposed that
  observation; public /metrics returned 404. Captured API logs and metrics
  contained none of the injected private markers/organization ID. The
  process stopped with exit 0; discovery and inherited ANPFUEL flags were
  cleared/disabled. This is local composition evidence, not VPS acceptance.
- Enterprise checker/generator regressions, new relative links, explicit
  gofmt, whitespace and tracked/staged secret review passed. Plan counts
  remain 348 tasks, 545 rules, 167 use cases, 145 screens, zero problems.

Runtime implementation blobs:
telemetry/private.go `4347d54aafba5625dd4d5f32dc2eb3a4e5d2752d`;
telemetry/telemetry.go `67fac10368c7fcf591690c6801d61f0425e0aa7c`;
orgapi/observe.go `de0453fecf5d1867dea715bf09b8933089d4a6b9`;
orgapi/redact.go `2a9fc55d0cbafd9128e2e46b7a888f40eb67191c`;
httpserver/private.go `06c5ad738c92b8d0cc2b229046b2545e2755cd60`;
httpserver/httpserver.go `634cdfff04a1769a5cc99e5bf91b704a7f120fbe`;
cmd/api/main.go `8fd7161f14fd074dd8bc670c188e628b5ad7de8f`.
Code, tests and evidence travel in one task commit above da8ee090.

## Retained gates

The app lint error is resolved. The separate aggregate Kotlin domain coverage
failure is still open, as recorded by EMP-PLAT-009/011; no domain code changed
or coverage threshold was altered here. EMP-PRIV-003's protected Android
store/device evidence and Claude's AUTH, revocation, runtime-role, transfer,
bucket, backup and route-composition activation obligations remain owed.
No enterprise business route/capability or production feature is activated.

F1/M1 remain open. EMP-PLAT-014 is the next eligible platform task;
EMP-PLAT-012 retains its EMP-GOV-014 dependency. Remote checks/review must
apply to the new head before protected integration. No CI polling, merge,
deployment, wiki publication or release certification is performed.

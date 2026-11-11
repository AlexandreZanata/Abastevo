# F1 EMP-PLAT-014 scoped write-rate admission

Status: LOCAL_DONE
Validation: PASS

2026-11-11. Scoped source acceptance, INTEGRATION_PENDING through
[PR202](https://github.com/AlexandreZanata/Abastevo/pull/202).
F1/M1 remain open; no production policy or business route is activated.

## Entry and scope

The owner requested the next eligible task and GitHub publication. The
enterprise worktree was clean at published 97298631, matching remote dev
and PR202, with main ef96cd13 already an ancestor. The occupied local dev
and primary checkout's unrelated changes are preserved.

Read the task, current progress, API/permission decision order, existing
identity limiter/query and forum transaction precedent. B-BR-EMP-29,
B-BR-EMP-30 and BUC-EMP-16 were documented before behavior. EMP-PLAT-012
retains its external-channel decision gate. No policy for that channel,
new service, dependency, personal quota or global request cap is inferred.

## Implemented behavior

orgapi.Register now requires WriteAdmission for a write rule and validates
a matching source policy at registration. Current session/proof, membership,
permission/capability and web freshness precede admission. Invalid commands,
foreign/denied authority and fresh-auth failure spend no target budget.
Reads and pre-organization company sessions retain their existing paths.

orgquota composes the existing identity Limiter.CheckTx. One short
ordinary-login WithOrgTx consumes organization, account and device counters,
in that lock order, on the existing identity_rate_windows write operation.
Quota denial or transaction failure rolls back every counter; no module
work runs. Admission commits before module rules, so an admitted conflict
still spends request budget. This is not billing, a record limit, or an
atomic business/authorization transaction. Cancellation prevents dispatch;
there is no promise to refund an admission already committed.

Source definitions map permissions to a class, three explicit budgets and
a recorded measured-limit rationale. Source slices/maps and key bytes
cannot mutate active policy after construction. A class shares its budgets
across its registered permissions. No production default is selected:
there are no mounted enterprise business routes to measure here. Each
owning feature must measure/register its class before activation. The small
test caps are synthetic boundary vectors, not production capacity claims.

Subjects use framed HMAC over class/axis/organization, adding account and
verified Android key only where applicable. Account/organization counters
survive device rotation; all axes are organization-scoped. Web has a stable
account/web bucket rather than a fabricated hardware identifier or rotating
cookie subject. Storage contains only the opaque orgquota:v1 digest and
existing operation/window/count/expiry metadata. Guard/config formatting
redacts keys and policy internals.

Exhaustion returns the existing private RATE_LIMITED/429 with positive
ceil-second Retry-After; the shared OpenAPI error documents that header.
Missing policy fails registration. Invalid principal, unsafe role, cancellation
or unavailable storage fails closed without raw provider/SQL detail.
Authorization/audit, telemetry and business mutations remain separately owned.

The inherited windows are fixed, not sliding. Whole-second windows up to
24 hours fit the existing quota-window horizon and retry protocol.
No migration or sqlc change is required. The existing Cleanup API removes
expired windows without deleting current windows; no active caller was
found in source. Cleanup scheduling/cadence must therefore be wired and
verified before enterprise activation; this evidence claims no live retention
guarantee. Replicas also require consistent policies, synchronized clocks
and a stable privacy key across active windows. Class/window/key changes
need a reviewed rollout rather than silent counter reset.

## Executed verification

Linux amd64, Go 1.27.4 and pinned staticcheck v0.8.1 under the documented
go1.27.1 workaround. Real PostgreSQL18/PostGIS3.6 used the unique tmpfs
Compose project enterprise-plat014-20261111, exposed only on loopback.
Fixtures create their own databases and ordinary runtime logins; concurrent
tests use a 16-connection pool. All identities/keys/policies are synthetic.

- RED: policy tests failed with undefined Definition/Guard/New. GREEN:
  source validation, subject isolation, device/web scopes, immutable policy/
  key and default fmt/slog privacy tests pass.
- `go test -race ./internal/platform/orgquota ./internal/platform/orgapi`
  passed. Registration refuses absent/unknown policy. HTTP tests prove
  authority/freshness before admission, no write quota on reads, cancellation
  refusal, static 503 on provider failure and ceil-second retry boundaries.
- `go test -race -tags=integration -coverprofile=/tmp/enterprise-plat014-coverage.out ./internal/platform/orgquota ./internal/platform/orgapi ./internal/modules/identity/domain ./internal/platform/database -count=1`
  passed. New orgquota statement coverage is 100%; percentage does not
  replace the failure/concurrency proofs below.
- Real PostGIS tests exhaust device, account and organization independently;
  rotate devices/accounts without resetting aggregate caps; isolate a second
  organization/class; and preserve the stable web bucket. Thirty simultaneous
  requests admit exactly three and reject twenty-seven, spending exactly
  nine counter units across the three axes.
- Failure injection at account/device insertion, missing grant and deferred
  commit rolls back all counters. An unsafe migration-role pool is refused.
  Cancellation is observed while a query waits on a held real counter lock;
  it returns unavailable, leaves counts unchanged and succeeds on retry.
- Fixed-window rollover creates new live counters while expired full
  windows exist; the existing Cleanup API removes only the three expired
  windows. Current quota remains exhausted afterward.
- Normal gateway/DB integration proves foreign/revoked scope consumes
  nothing, success/conflict each spends one admission, a third call receives
  no-store 429/Retry-After without entering module code, and quota exhaustion
  cannot override subsequent membership denial.
- The real signed-account/key gateway now uses the actual admission guard
  with an ordinary pool. Only its two successful proof/authority admissions
  spend six units; nonce races, changed body, wrong owner, foreign/ended
  membership, revoked key/family and suspension spend no additional units.
- Existing company-session and identity-adapter real-PostGIS suites passed.
  After fixing the signed fixture's import/registration setup, the complete
  orgapi integration package was rerun successfully. No test was weakened.
- API/HTTP/telemetry and route-contract race regressions passed. Affected vet,
  pinned staticcheck and API/worker/ops builds passed. OpenAPI lint passed
  with the existing 785 warnings/179 informs; no extra lint debt.
- The new behavioral package is in the quick manifest. Enterprise verifier/
  generator regressions, new links, explicit gofmt, whitespace and tracked/
  staged secret checks passed. Counts stay 348 tasks, 545 rules, 167 use
  cases, 145 screens, zero structural problems.

Runtime blobs: orgquota/quota.go `d3e058da23f3046dc2f65727955355244e5e3ec1`;
orgapi/admission.go `0118320be08fc455c0886e4e238c9b953c3defcc`;
orgapi/gateway.go `2afe27c9fda6a19e231ad5044c6404e7dde1de0f`;
orgapi/errors.go `52bd82393a2a4828bd5ea60300f49f6828110b6a`.
Implementation, tests, contracts and evidence travel in one task commit.

## Retained obligations

No feature-specific measured production budgets, live limiter activation,
cleanup scheduler, new AUTH provider or ordinary runtime login is configured.
Those prerequisites must be reviewed with the owning feature before rollout.
This request limiter does not resolve the review's per-member transfer/
concurrency gate. Protected Android storage, aggregate Kotlin domain coverage
and other [review obligations](f1-pr202-review.md) remain owed. The app
translation lint correction remains accepted; no Kotlin/resource source changed.

EMP-PLAT-015 is next eligible; EMP-PLAT-012 awaits EMP-GOV-014.
F1/M1 remain open. Remote checks/review must validate the new head before
protected integration. No CI polling, merge, deployment, wiki publication
or release certification is performed.

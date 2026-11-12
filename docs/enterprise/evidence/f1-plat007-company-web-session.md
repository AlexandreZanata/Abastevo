# F1 EMP-PLAT-007 company account web session

Status: LOCAL_DONE
Validation: PASS

Date 2026-11-11. Delivery state INTEGRATION_PENDING; F1/M1 remain open.
The owner requested implementation and publication into existing
[PR202](https://github.com/AlexandreZanata/Abastevo/pull/202), head dev,
base main. Construction starts at published `277fd470` in the isolated
`codex/enterprise-f0-f1` worktree. PR202 still has that head and main
`ef96cd13` at the pre-publication read. The occupied local dev and other
worktrees are preserved. EMP-GOV-003 and EMP-PLAT-004 are present.

## Implemented behavior

B-BR-EMP-17, B-BR-EMP-18 and BUC-EMP-10 were recorded before behavior.
The new `companysession` module reuses account validation, SHA256Hasher,
cryptographic token generation, UUID generation and the existing private
error envelope. No dependency, service, admin authority or tenant privilege
is introduced.

The production API mounts three typed POST operations only when
`ANPFUEL_ENTERPRISE_ENABLED=true`:

- `/v1/org-access/session/login`: current fresh account family/access proof
  issues a dedicated company cookie and CSRF value.
- `/v1/org-access/session/read`: empty JSON, cookie and CSRF return account
  and fixed authentication/expiry metadata.
- `/v1/org-access/session/logout`: empty JSON, cookie and CSRF revoke and
  clear only that company session.

The canonical configured HTTPS Origin and Host must match exactly.
The host-only `__Host-empresas-session` cookie is Secure, HttpOnly,
SameSite=Strict, Path=/ and has no Domain. Ambiguous headers/cookies,
cross-site fetch metadata, signed/bearer credentials, query parameters,
unknown/duplicate/case/Unicode JSON aliases, null proof and bodies over
16 KiB are refused. HTTP reads have a 15-second deadline; service calls
have a ten-second context. Every private response is no-store without ETag.

Only lookup digests and separately salted, domain-separated cookie/CSRF
verifiers persist. The raw company cookie appears only in Set-Cookie;
the CSRF value is returned to the private login client. Account proof is
not stored. Result, proof and application records redact default, Go and
structured log formatting. Errors never forward database/provider detail.

Issuance uses the existing current account proof's issuance time, with a
strict less-than-15-minute freshness window. A second verifier check binds
the account proof snapshot; the transaction locks the active account and
exact current family hash/expiry before insertion. Refresh or revocation
between validation and insertion refuses issuance. Existing company
sessions remain usable after the source access token expires, subject to
current account/family liveness.

Idle expiry is exactly 30 minutes; absolute expiry is at most eight hours,
bounded by the source family's lifetime. Activity never extends freshness
or absolute lifetime. Re-login with fresh proof atomically rotates a
presented same-account cookie and CSRF; failed insert/commit preserves the
old session. Account locking serializes the separate ten-company-session
limit. Invalid family/session rows are pruned before admission.

Company logout leaves personal and admin sessions alive. Admin logout
leaves company sessions alive. Account suspension/deletion or source-family
revocation invalidates company access. The existing admin CIDR fence,
cookie, table and middleware are unchanged.

Organization entry uses `WebSession` to verify without idle renewal,
resolves current organization membership/permission, then calls mandatory
`WebSessionConfirm` to recheck cookie/CSRF and declared freshness before
renewal. Foreign scope returns the existing safe denial before freshness.
The separate middleware uses its own context key and refuses admin/public
namespaces. Account identity alone never authorizes organization data.

Migration `000167` adds only the company table and the source-family
composite ownership index/FK. Existing account/family cleanup cascades;
no credentials or activation are seeded. Applied migrations and the
`000132` compatibility floor are unchanged. The module owns its sqlc
package; explicit account/family schema dependencies support live authority.

The existing daily privacy-retention job includes `company-web-sessions`,
with 500-row SKIP LOCKED batches and oldest-overdue-before-delete metrics.
It runs even when the enterprise surface is off. Reads deny expired rows
immediately. Cleanup tolerates clock rollback without destroying otherwise
unexpired future-dated credentials.

Tested source blobs: domain `76a13b26bd1b6fe7e6160471a960b6229ca8d2b1`,
application `932b0094192958e971c8c4e42efff28d4a2a671f`,
store `d74ba57e37450c44f753d979829eebc5e7f2649c`,
account adapter `e1756c8cd07acb1f87f8e58c243ac0565ada6111`,
HTTP `28758e46fb6747fd9f84a0b72a79982754d47849`,
migration `094f98bc75dea42fadfb3f208c3c237efd58e1dd` and gateway
`c1b0291503f46807284744873cc17684611800dc`.
Source, tests and evidence travel in the same atomic task commit.

## Executed checks

Linux amd64, backend Go 1.27.4. Commands below ran from backend unless
noted, with `GOCACHE=/tmp/enterprise-go-build`. The disposable tmpfs
Compose project `enterprise-plat007-20261111` used the repository's
pinned PostGIS 18/3.6 fixture, loopback port 32781. Real SQL tests used
explicit `ANPFUEL_TEST_DATABASE_URL`, unique databases, synthetic accounts
and ordinary runtime logins. No persistent/staging database was used.

- RED/GREEN caught revoked source families occupying admission slots,
  cleanup deleting unexpired credentials during clock rollback, and
  application acceptance of expired/future rows from a persistence port.
  Final production SQL and domain checks reject these cases.
- `go test -race -tags=integration -coverpkg=./internal/modules/companysession/... -coverprofile=/tmp/enterprise-plat007-coverage.out ./internal/modules/companysession/... -count=1`
  passed on the final source. The shared coverage includes actual HTTP
  journeys from the adapter package: total 85.4%, domain 100%, application
  89.3%, HTTP 89.7%, persistence/account adapters 73.7%.
- Real PostGIS proved fixed freshness, exact idle/absolute boundaries,
  current suspension/deletion/revocation, source-access expiry and refresh,
  no renewal after invalid CSRF/freshness, and bounded cleanup convergence.
- Twelve concurrent logins admitted exactly ten and refused two. Refresh
  after proof validation refused stale issuance. A deferred commit failure
  returned no new credentials, preserved the old cookie, and allowed retry.
  Pruning revoked families freed admission slots.
- Actual TLS handlers proved company/admin cookie separation (including
  renamed credentials), both logout directions, unchanged personal access,
  secure attributes, exact/ambiguous origin and CSRF denial, no-store/noETag
  and middleware context/namespace isolation.
- The actual company ports through orgapi denied foreign scope with 404
  and stale fresh-required access with 401, without extending idle time.
  Missing confirmation refuses gateway registration.
- Migration rehearsals applied the previous chain through 000166 with
  populated account/family/catalog data, injected a DDL failure, verified
  no leftover table/index/ledger, retried successfully and reapplied with
  zero work. Composite ownership refused cross-account families; the old
  family DELETE cascaded; public rows and compatibility floor survived.
- Unit failure tests covered absent ports, invalid/stale proof, private
  persistence errors, ID/token/salt generation failure, failed logout,
  credential formatting, ambiguous transport, JSON/body limits and disabled
  registration. Failure never returns credentials or claims successful logout.
- Affected `go test -race -tags=integration` passed for account, admin,
  privacy (all subpackages), orgapi, apicontract and migrate. The account
  and admin SQL/auth/erasure/refresh regression suites used real PostGIS.
  `go test -race ./cmd/api ./cmd/worker -count=1` passed.
- The actual production company factory/registration is checked against
  canonical OpenAPI. Bidirectional route checks reject missing operations,
  unguarded handlers, cookie-plane/security/CSRF/freshness drift and missing
  cache policy. The complete apicontract regression suite passed.
- `go vet -tags=integration` and pinned staticcheck v0.8.1 passed for
  companysession, orgapi, apicontract, cmd/api and cmd/worker. Staticcheck
  used the documented `GOTOOLCHAIN=go1.27.1` workaround; runtime builds
  retain Go 1.27.4. `sqlc generate`, `sqlc vet`, `sqlc diff` and
  `go build ./cmd/api ./cmd/worker ./cmd/ops` passed.
- OpenAPI vacuum lint passed with 785 warnings/179 informs, below the
  prior 786/179 baseline. No new lint debt was introduced.
- The actual previous API was built from `git archive 277fd470 backend`
  with `-buildvcs=false`. Binary SHA-256:
  `7187925a9df2d5f526f197ab8d2ce13b6cf617bf60941f071f9c1d9aef0bb1bc`.
  After applying the current chain through 000167 to a separate disposable
  database, it returned HTTP 200 `{"status":"ready"}` and stopped with
  exit 0. Inherited ANPFUEL configuration was cleared and external discovery
  was disabled.
- The enterprise verifier/regressions, explicit gofmt, whitespace and
  changed/new-file secret review passed. The quick manifest includes all
  four new behavioral packages.

## Retained delivery and product gates

This task adds account session lifecycle operations, not organization
business routes, membership policy or the panel. AUTH supplies concrete
organization authority; EMP-PLAT-008 supplies the approved company shell.
No Android feature, Caddy route, live flag activation or real-receipt
intake/retention is added. Existing legal, runtime-role, deployment and
production certification obligations remain open.

Delivery reuses the authorized dev-to-main PR202 with preserved commits.
Required remote checks/review must validate the new head before protected
integration. This evidence does not claim a merge, CI result, staging/VPS
deployment, wiki publication or F1/M1 completion. Next bounded task:
EMP-PLAT-008, upon authorization.

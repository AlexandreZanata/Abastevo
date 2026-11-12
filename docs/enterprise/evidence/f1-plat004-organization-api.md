# F1 EMP-PLAT-004 organization API conventions

Status: LOCAL_DONE
Validation: PASS

Date 2026-11-11. Delivery state INTEGRATION_PENDING, F1 IN_PROGRESS.
Source dependency EMP-PLAT-001 is present through `4c4d6e3b`. The task builds
on published audit checkpoint `090207ad` on `codex/enterprise-f0-f1`.
Fetched main is `d8bdc642` and dev is `231e63b7`. The isolated construction
checkout preserves occupied dev and other worktrees. GitHub publication
was explicitly requested. Protected dev-to-main integration is pending.

## Implemented scope

B-BR-EMP-11, B-BR-EMP-12 and BUC-EMP-07 were recorded before implementation.
The `orgapi` entry registers typed organization POST handlers, validates a
bounded strict DTO and passes resolved account, device, membership and
organization scope to the module callback. The new quick-manifest package
contains behavioral tests.

Android uses narrow ports to the existing live account session and
owner-bound signed-device proof. Company web has a separate session port
and exact-origin guard. Partial credentials, explicit empty/null credentials,
or signed headers cannot fall back to web authentication. The company port
owns its dedicated cookie and CSRF verification in EMP-PLAT-007.
Current membership/role/fresh-auth/capability resolution is an explicit
server port owned by the AUTH tasks.

Commands use schema version 1, canonical non-nil operation IDs and positive
revisions when required. Unknown/duplicate fields, Unicode/case aliases in
top-level names, reserved null scalars, query parameters, ambiguous content
types and bodies over 64 KiB are rejected. Exact input bytes are restored
for signature verification. Errors use registered codes and static safe
text. Successes and failures are no-store with no ETag.

The actual production router now gives unmatched private namespaces and
unsupported methods the same safe NOT_FOUND envelope. Private panics use
the safe UNAVAILABLE envelope. Public and personal route behavior keeps its
existing handlers and transport policy.

The cursor wrapper reuses the existing HMAC implementation with a
JSON-framed binding for origin, organization, verified account, membership,
route and canonical filters. Pages use 20/100 limits and bounded tokens/keys.
The existing expiry helper was corrected to reject at the deadline itself.

## Contract and route gate

The canonical OpenAPI adds a conventions-only organization section, common
request/identifier/error models, private cache headers and the error response.
Twelve shared vectors exercise schemas and the HTTP entry.

`ValidateOrganizationRoutes` walks actual chi registrations and compares
them with OpenAPI in both directions. It checks POST, guarded handler
descriptors, matching permission/write/fresh/revision/success metadata,
required JSON bodies, path scope and all shared error/cache responses.
It rejects unregistered, undocumented and unguarded routes, missing scope or
body, changed permissions, missing errors, ETags and cacheable responses.
Module composition tests reuse this gate as their feature routes land.

There are zero published business operations in this foundation task,
consistent with its exclusion of feature handlers. Positive test operations
and deliberately broken registrations make the gate tests non-vacuous.
The concrete authority provider and company-session implementation stay with
AUTH and EMP-PLAT-007. This checkpoint records the tested shared boundary.

Tested source blobs are `46281643183f924da359d994a8bf9180d15acdc5` for the
HTTP entry, `0a16c95cc9f10061078721527fefd128b00401f4` for cursor binding,
`4803fbad235e43d104cc4c6e3e7f383158b14dbb` for the route validator and
`20bb04f9da475c2bd9267cb2edeea5b2abb37f43` for OpenAPI.
Source and evidence travel in the same task commit.

## Executed verification

Linux amd64, Go 1.27.3, pinned staticcheck v0.8.1 and vacuum v0.30.6.
The unique tmpfs Compose project `enterprise-plat004-20261111` supplied
PostGIS 18/3.6 on an ephemeral loopback port. Tests created and removed only
their own databases and synthetic logins. No new migration was introduced.
Go commands used `GOCACHE=/tmp/enterprise-go-build`.

- RED reproduced absent implementation. Later regressions reproduced cursor
  acceptance at exact expiry and Go JSON's Unicode credential-name alias.
  Both failed before their respective fixes and passed afterward.
- `go test -race -tags=integration -coverprofile=/tmp/enterprise-plat004-coverage.out ./internal/platform/orgapi -count=1`
  passed with 93.4% coverage on the final source.
- Real PostGIS tests used the actual account service/store, registration
  ceremony, account/key binding and signed-request verifier. Correct proof
  reached the callback. Reused nonce, wrong signed authority, changed body,
  expired proof, another account's session, revoked key/session and suspended
  account were refused. Current-policy fixture rows proved foreign/offboarded
  scope denial on the next request. Two concurrent requests sharing a nonce
  produced one callback and one authentication refusal. The policy fixture
  supplies the AUTH port while its production membership schema is pending.
- Scoped `go test -race` passed for httpapi, httpserver, apicontract and
  every existing direct cursor consumer under admin, charging, cloudsync,
  community, contributor, directory, driver, forum, ledger, mapdata, official,
  quality and subscription. Socket-based suites ran with local networking.
- `go vet -tags=integration` and pinned staticcheck under `GOTOOLCHAIN=go1.27.1`
  passed for orgapi, apicontract, httpserver and httpapi.
- `go build ./cmd/api ./cmd/worker ./cmd/ops` passed.
- Canonical OpenAPI validation, schema/HTTP vectors and drift regressions
  passed. Full pinned vacuum lint passed with 773 warnings/160 informs,
  compared with 774/160 on `090207ad`. Added descriptions/examples resolved
  the new lint findings. The existing rule configuration is unchanged.
- Explicit gofmt, plan checker and checker regressions, diff/new-file
  whitespace and changed-file secret review passed.

Next bounded task is EMP-PLAT-005. Feature handlers, account-scoped
organization-access operations, concrete authorization/session wiring and
release acceptance retain their owning tasks and gates.

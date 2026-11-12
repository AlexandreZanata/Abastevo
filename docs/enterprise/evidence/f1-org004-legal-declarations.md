# F1 EMP-ORG-004 private legal declarations

Status: LOCAL_DONE
Validation: PASS

2026-11-12. Source above ORG-003 8a95c07f in the owned enterprise worktree,
targeting [PR202](https://github.com/AlexandreZanata/Abastevo/pull/202).
F1/M1, completed-phase publication and final current-head/base CI remain open.
This checkpoint does not authorize actual CPF collection or legal acceptance.

## Contract and choices

B-BR-EMP-55..58 and BUC-EMP-29 preceded behavior. EDR-037 adopts the module's
recommended OWNER/ADMIN org.legal_entity.manage row as explicit permissions
v2. Only those two of 1,092 matrix cells change; v1 remains historical evidence.
No role inheritance, verification, representation, public station or payment
right is inferred. Current role, membership, proof, record and capability
checks still apply. EDR-038 resolves the source grammar mismatch for specified
snake_case audit/event names, retaining exact registrations and length bounds.

ORG-004 runs before ORG-005 to supply actual scoped establishment references
for optional unit links. Fleet setup still does not require a legal party.
The synthetic org.legal admission class inherits identity v1's 200/hour on
each organization/account/web-or-device axis; this is an explicit bounded
synthetic setting, not a measured production capacity. No dependency, service,
provider credential, production grant or flag is introduced.

Primary identifier rules were checked before implementation and linked in
[references](../18-references.md). The existing CNPJ kernel supports numeric
and alphanumeric forms using Receita's ASCII-minus-48 algorithm. The new CPF
value checks both modulo-11 digits and rejects constant sequences. Tests
calculate identifiers at runtime from synthetic bases; no person's CPF is
copied into source, documentation or output. Format validation says nothing
about registration status, representation or legal consent.

## Domain, privacy and guarded API

A COMPANY declaration stores an eight-character CNPJ root and atomically
creates its first establishment from the full validated identifier. Later
establishments must match that root. HEAD_OFFICE/BRANCH/NOT_INFORMED are
explicit declarations; order 0001 does not infer a head office. Municipality
is an optional seven-digit declared IBGE reference, without address/provider
verification. Mutable establishment metadata uses CAS and leaves CNPJ,
parent and identity immutable.

NATURAL_PERSON requires CPF and the pinned VERIFICATION_AND_AGREEMENTS
purpose; establishment input is refused, including an empty object. The
HTTP regression first reproduced 201 for that invalid combination, then
passes with 422 after the boundary correction. The OpenAPI discriminates
purpose/establishment fields by party kind. Runtime checks remain responsible
for actual checksums and normalized identity. All created parties are
DECLARED, with the explicit display label "Declarado pela empresa". There
is no verifier or route to produce VERIFIED in this task.

Default reads and lists always mask CPF, including for managers. A separate
fresh management operation writes an identifier-access audit and returns the
identifier only after commit. Failed or deferred audit commits return no
content. Domain, application and DTO values redact fmt/JSON/slog; explicit
response mappings release only their intended fields. Logs, errors, receipts
and audit contain no raw CPF or legal name. Identifier errors expose only
allowlisted fields and INVALID_CPF/INVALID_CNPJ/DUPLICATE codes.

The production enterprise factory mounts eight POST operations under
/v1/orgs/{org_id}/legal-entities: create/list, parent read/identifier-read/
withdraw and establishment create/list/change. All require the dedicated
PANEL company cookie, exact HTTPS Origin/Host, CSRF, current membership and
org.legal_entity.manage. Configuration writes additionally require fresh
proof and the current organizations pilot flag; explicit identifier reads
require fresh proof. These entries accept only roots initialized with the
source-pinned synthetic terms receipt. The global surface remains default-off.
Normal reads retain the previously authorized capability-off behavior.

Commands carry version, operation and expected revision where applicable;
no client identity, credentials, verified status or reveal flag is accepted.
Lists use bounded UUID keyset pages and opaque cursors bound to account,
organization, membership, route, parent/filter and limit. Foreign/unknown
references share a bounded 404. Every response is no-store without ETag.
The fully composed production-router/OpenAPI gate now checks every path
selector, including nested parent and establishment IDs, in both directions.
Cookie, authority metadata and missing-selector drift regressions pass.

## Transaction, schema and recovery

Migration 000173 adds forced-RLS legal parties, establishments and immutable
command acknowledgements. Bounded trigger refusals avoid PostgreSQL failing-row
DETAIL for CPF validation/duplicates. Current identifier uniqueness is local
to the organization. Revoked history can coexist with a new explicit current
declaration under a new identity, following the module's correction rule.
Deferred company presence checks prevent orphan roots without establishments.

Ordinary-role commands lock account/proof, the shared capability and root,
then resolve current membership/role and recheck proof after lock waits.
Content, receipt and borrowed-transaction audit commit together. Changed
intent conflicts; matching operations return their original acknowledgement.
Failed commits return zero results and no successful replay marker.

Withdrawal first attempts deletion inside a savepoint. Actual database FKs,
including DEFERRABLE INITIALLY DEFERRED references forced to validate within
the savepoint, decide whether content must instead be retained as REVOKED.
There is no invented reference service. A failed cascade is rolled back before
revocation, preserving its establishments. Verification history also requires
retention. Unreferenced content without that history is removed. The receipt
retains IDs/hash/outcome without an FK to removable private content. Its scoped
creation reference permits authorization of exact retry after removal, while
content reads/new distinct commands still return 404 and identity cannot be
reused. HTTP and direct scoped-reference regressions cover this distinction.

The populated upgrade starts at 000172 with real initialized root/unit/member/
OWNER, audit/event rows and a public station. An ordinary NOSUPERUSER/
NOBYPASSRLS owner applies the migration. Injected SQLSTATE 22012 rolls back
new tables/functions/ledger and restores the exact old audit/event constraints
and populated rows. Retry applies once, then no-ops; the prior schema-version
compatibility check and an actual post-upgrade declaration pass. This checks
schema compatibility, not execution of an old container image. Recovery keeps
the additive schema/history, disables admission and uses a compatible previous
binary; no applied migration edit or destructive down is supplied.

## Validation

Disposable PostgreSQL 18/PostGIS 3.6 uses the pinned repository image on local
loopback 32771. Privileged pools prepare fixtures only; application commands
run with separate ordinary runtime logins and forced RLS. This is not evidence
of production credentials/grants. Go 1.27.4, sqlc 1.31.1, staticcheck v0.8.1
(Go 1.27.1 toolchain) and vacuum 0.30.6 were used.

Meaningful critical cases include:

- One thousand independently generated CPF values, normalization/checksum
  mutations, masks, invalid types/purpose, corrupt storage and explicit roles.
- Twelve competing distinct duplicate declarations produce one creation;
  twelve equal commands produce one creation and eleven original replays.
- Foreign declaration/establishment and receipt identities are isolated;
  independent organizations can declare the same identifier without an oracle.
- Metadata CAS, immutable CNPJ/parent, same-root checks, exact retry and private
  keyset cursor replay with changed scope/filter/parent.
- Current cookie/family revocation, elapsed authentication, capability disable,
  ordinary masked read preservation, malformed authority and no bearer/admin
  fallback. Exact established gateway refusal codes are retained.
- Failed/deferred content/receipt/audit commits, no provisional success,
  unreferenced removal, deferred-FK revocation and exact HTTP retry after removal.
- Fresh audited CPF reveal, no identifier returned on audit failure, and no
  raw private fields in receipts/events/default/error responses.
- Populated ordinary-owner migration/failure/recovery and existing
  organization, membership, authorization, audit/event, quota and session flows.

Commands (ANPFUEL_TEST_DATABASE_URL must name a disposable local fixture):

```sh
cd backend
GOCACHE=/tmp/enterprise-go-build go test -race -tags=integration \
  ./internal/modules/organization/... ./internal/modules/orgaccess/... \
  ./internal/modules/orgaudit/... ./internal/modules/orgevent/... \
  ./internal/modules/membership/... ./internal/modules/companysession/... \
  ./internal/platform/orgapi ./internal/platform/apicontract \
  ./internal/platform/orgquota ./internal/platform/enterprisesuite -count=1
GOCACHE=/tmp/enterprise-go-build go test -race -tags=integration \
  -coverpkg=./internal/modules/organization/... \
  ./internal/modules/organization/... -count=1 \
  -coverprofile=/tmp/enterprise-org004-final-coverage.out
GOCACHE=/tmp/enterprise-go-build go test -race ./cmd/api -count=1
sqlc vet
sqlc generate
sqlc diff
```

The affected packages also pass go vet, pinned staticcheck, API/worker/ops
compilation, strict gofmt, git diff --check, added local Markdown links and
staged secret-surface review. Canonical plan validation reports 348 tasks,
545 BRs, 167 UCs, 145 screens and zero structural problems; its three tests
pass. OpenAPI has zero errors and the same 785 pre-existing warnings; 229
informational findings include 50 additional duplicate-description notices.
A sandbox-denied loopback socket in the existing API suite was rerun with
local socket authorization and passed; it was not classified as a code defect.

The final composed organization coverage profile records domain 95.17%,
application 80.92%, adapters 76.36% and HTTP 78.39%. Unlike the default
per-test-package profile, -coverpkg counts production HTTP/application paths
executed by the external real-PostGIS tests. This is not a whole-project or
Kotlin coverage claim. After reproducing the empty-establishment defect,
the complete affected organization suite, focused contract drift/examples,
static checks and API/worker/ops builds pass again on the corrected source.
The broader authorization/audit/event/membership/session/quota suites passed
before that HTTP-only correction and their inputs did not change.
No Android behavior changes, so no emulator acceptance is claimed. Actual
IDN/device providers, ordinary production grants, vault/staging, GOV-014/
PLAT-012, Kotlin coverage and real-data/legal gates remain independent.

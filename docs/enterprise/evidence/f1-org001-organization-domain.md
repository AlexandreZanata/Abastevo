# F1 EMP-ORG-001 organization aggregate and lifecycle

Status: LOCAL_DONE
Validation: PASS

2026-11-11. Scoped domain acceptance, INTEGRATION_PENDING through
[PR202](https://github.com/AlexandreZanata/Abastevo/pull/202).
F1/M1 remain open. This is a value model, not organization onboarding,
current actor authority, persistence or actual closure.

## Entry and bounded scope

The owner requested the next enterprise task and GitHub publication.
The isolated worktree entered clean at published ef8bfff9, matching open
dev → main PR202; main ef96cd13 remains an ancestor. Occupied local dev and
unrelated primary-checkout work are preserved.

EMP-PRIV-003 has implemented source but remains IN_PROGRESS for the owner's
deferred native batch and current AUTH/IDN composition. Its production
persistence/contact bindings stay unavailable. ORG-001 depends only on the
accepted PLAT-001 source, so this independent F1 domain work does not
claim the native gate closed or jump to F3 PLAT-016. Next ORG task is ORG-002.

Read ORG-001, organization business rules/state graph, current progress and
the kernel scope precedent. B-BR-EMP-36/37 and BUC-EMP-19 were recorded before
behavior. The [102-row oracle](../../../contracts/testdata/enterprise/organization-lifecycle-v1.psv)
was fixed from the specification before implementation. The initial
pending-to-active RED case returned transition-not-allowed; the completed
implementation then passed the full oracle and invariants.

## Implemented behavior

The proposed module name is now confirmed as organization/domain. NewPending
accepts explicit validated kernel.OrgScope and starts PENDING_SETUP at
revision 1. It creates no unit, employee or owner; the owning creation
transaction must allocate a fresh ID, refuse an existing root (including
CLOSED) and atomically write the complete BR-ORG-025 records. No global
uniqueness or persistence claim is made by a pure constructor.

Snapshot, ConfigurePending and Transition check original scope. Changes
check expected revision and reject exhaustion without wrapping. Values
are immutable; profiles, kind slices and missing-checklist results are
copied. Same-name organizations remain separate identities. Descriptive
kind tags are validated/sorted/deduplicated with no invented default;
they never choose a role, price, plan, limit or related legal entity.

Pending setup may represent incomplete configuration in the domain. Actual
onboarding in ORG-003 has stricter input/transaction requirements. Activation
requires nonblank name, server-known IANA zone, at least one active unit and
owner, and recorded terms. All missing items are returned in fixed order as
bounded codes. Name/zone text is not rewritten. Invalid nonempty zone/kind
or invalid hydrated states fail. Server IANA aliases are accepted; no new
client/time-wire model or cross-platform timezone parity is asserted.

Only documented state edges are available. Positive confirmed conditions
are mandatory for pending cancellation, suspension, cause resolution,
archive blocking-item clearance and restore/closure schedule state.
Unknown/default conditions deny. CLOSED has no outgoing edge, including a
self-edge and metadata configuration. Rehydrate is for trusted storage,
never client DTO decoding; it does not prove cross-record facts or history.

The owning application commands must verify current actors and freshness,
resolve the facts from scoped data, enforce CAS, persist/audit changes,
handle timers and perform membership/device/grant/notification effects.
Closure periods, abandoned-setup jobs, retention and actual closure are not
implemented or activated. No HTTP route, SQL/migration, dependency, service,
client feature or runtime provider is added. Default formatting and slog
redact aggregate/profile/snapshot contents.

## Executed checks

Go 1.27.4 module toolchain, pinned staticcheck v0.8.1 using Go 1.27.1,
Linux amd64. GOCACHE=/tmp/enterprise-go-build; staticcheck uses
XDG_CACHE_HOME=/tmp/enterprise-staticcheck. Commands from backend:

- `go test -race -coverprofile=/tmp/enterprise-org001.cover ./internal/modules/organization/domain ./internal/modules/kernel -count=1`
  passed. New organization package: **100% statement coverage**; existing
  kernel 94.4%. The combined total is not represented as 100%.
- The 102 fixed rows cover all 25 state pairs, all 32 activation combinations,
  known/unknown reasons/kinds/zones, cancelled/pending/elapsed/unknown closure
  facts, immutable CLOSED and revision bounds. Header/count/unique IDs,
  known operation and complete expected maps are checked with no skips.
- Six additional tests cover zero/foreign scopes, pending configuration,
  hydrated invariants, stale/maximum revisions, copied inputs/results/errors,
  no default evidence, identical descriptions across IDs and private
  fmt/slog output. Thirty-two concurrent value decisions pass under race
  detection; this is not repository/CAS concurrency proof.
- `go vet` and pinned `staticcheck` on organization/domain and kernel pass.
  `go build ./cmd/api ./cmd/worker ./cmd/ops` passes; roots remain unmounted.
  Explicit fail-on-output gofmt and git diff whitespace checks pass.
- The quick behavioral manifest includes the new package. Enterprise
  checker/generator and three checker regression tests pass:
  **348 tasks / 545 rules / 167 use cases / 145 screens / 0 problems**.
  These are specification counts, not completed product features.
- Added relative links and tracked/staged secret review/Gitleaks pass.

No SQL or migration changed, so no disposable PostGIS run is claimed or
required for this pure task. No Android source/contract input changed, so
the prior domain/application/data/native compilation/lint suites are not
repeated. The existing Android aggregate coverage gate remains below 90%
(80.3115% at unchanged ef8bfff9 inputs), and PRIV-003 native acceptance,
current providers and other [review activation gates](f1-pr202-review.md)
remain owed. No required failing gate is reclassified as passed.

SHA256 of tested inputs:

- Implementation: ce1c0105f167f6b9155baa083e5950e1cbf9d6295c35d715d9a0a78eddac5bfd.
- Invariants: f9431c86b69dca5e7ae9d44ff6ee61a4582d8922cd8c93eed646af58810a141a.
- Oracle consumer: c794a6bc3027d9c2844711eb2a8000b6d8a673516a90e337c45be6945518a786.
- Oracle: ebba563196d90452f4e3bee165aed688bb0f30f76666fa0836bdccad724837ea.

## Recovery and remaining work

The pure model has no persistent side effect; rollback preserves the stored
state and removes only future entry wiring. Do not reconstruct CLOSED as
a new pending root or bypass the existing scope/audit conventions.
ORG-002 owns root persistence, migration/upgrade isolation and real PostGIS
proof; ORG-003 owns atomic onboarding. Current actor integration and actual
lifecycle commands keep their planned ownership. No live readiness, merge,
wiki publication, deployment or release certification is inferred.

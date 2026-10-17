# FQ-05W5 — Native worker composition and isolated claims

Status: LOCAL_DONE
Validation: PASS

Completed 2026-10-17 on maintained dev, parent db8ef2ca. The tested behavior is
the W5 worker/config/SQL/source-serial changes included in this atomic checkpoint;
delivery remains INTEGRATION_PENDING. No remote checks or deployment claimed.

Before behavior: B-BR-FQ-01/02/03/04/05/07/12/13/16, BUC-FQ-01/02/03/04.
Compose existing verified HTTPS source Fetcher through worker root, bounded PMQC
file/archive/process ports, Directory-owned read-only identity and transaction
guard, shared publication/watermark and a whole-attempt cross-worker serial
lease acquired before fetch. The default-off native import flag must exclude
queued source work from the general/safety dispatchers without charging attempts.
When enabled it receives one fixed serial slot; safety loops remain independent.
Reserve quality-discovery from general claims now; live discovery comes next.

The trusted preparer path is bundled /prepare_quality, never job input. A runtime
preparer outage fails only its native job with closed errors; it does not stop the
safety loops. Each attempt resolves current Directory evidence once per CNPJ,
retains source observation clocks, collects only active exact binding guards and
never creates identity. Guard/replay is checked inside the publication transaction.

Use a separate bounded connection for the source advisory lease, so a waiting
source job cannot exhaust the publication/safety pool. Close this connection on
all paths; whole attempt <=210 seconds, below the existing five-minute job lease.
Private fetched workspaces and files are owned and cleaned; verified public-IP/
TLS/redirect/source allowlist from the existing Fetcher is preserved. No live
activation, unrelated schedules, business portal, D, iOS or production release.

Immediate acceptance: default/explicit/bad configuration; disabled queue attempts,
fixed independent slot composition and safety handler requirements; real advisory
serialization/cancellation/release/failure; source preflight/private-address/TLS/
redirect/size/cleanup negatives; actual Rust→Directory→PostGIS→frozen-read replay
and identity retirement. Static Rust image packaging/live discovery remain owed
until separately tested, not inferred from this composition code.

## Delivered and verified

Worker root now composes native FileImport with the existing HTTPS Fetcher,
bounded Rust runner, Directory-owned read ports/live guard and existing atomic
publisher. Directory evidence is cached only within one attempt; retries resolve
again. Exact active bindings alone enter the sealed transaction guard. A source
advisory session is held on an independent connection across fetch/prepare/publish
and released by closing that connection on success/error/cancellation. Connection
establishment is bounded to five seconds; whole attempt to210 seconds. SQL uses
the fixed source lock key, not untrusted input; sqlc regeneration is stable.

`ANPFUEL_QUALITY_IMPORTS_ENABLED` is independently default-off and documented in
the example environment. Invalid booleans fail config loading. Both native kinds
are excluded from general claims even when handlers exist. Explicit import
enablement adds exactly one fixed slot; existing four required safety slots are
retained. Unsupported/duplicate slot kinds and missing required handlers fail.
No discovery handler or schedule is installed. The root uses trusted
`/prepare_quality`, never a job-controlled executable/path.

Meaningful local acceptance:

- Real Rust plus disposable PostGIS: a2016 full source clock binds to a station
  operationally created today without interpreting that timestamp as a business
  validity window; identical replay converges. Retirement after actual prepare
  fails the publication guard with the prior revision untouched. A fresh retry
  resolves the retired identity to review and publishes an association correction;
  the prior frozen revision remains reachable and no station is created.
- Real queue: disabled import/discovery jobs remain queued at zero attempts,
  while ordinary/safety jobs complete; an explicitly enabled import slot claims
  only imports and leaves discovery uncharged.
- Real advisory-lock barrier: a waiting second worker cannot enter its callback;
  a main pool limited to one connection remains available while it waits. Failed
  callbacks and cancelled waiters release their session; subsequent attempts run.
- Foreign/file/query source selections are refused before I/O with redacted
  errors; a cancelled allowed fetch leaves no owned workspace. Existing real
  HTTPS Fetcher checksum/cleanup, conditional/status, size, timeout/cancellation,
  redirect, loopback/private-address and forbidden-URL tests remain green.
- Existing publication, provenance, rollback/replay and job failures pass in the
  affected quality suites. Temporary synthetic source files are cleaned.

The first end-to-end test incorrectly expected historical observation age to
make a newly published lease stale. The assertion was corrected to the existing
publication-age contract; no runtime staleness rule was changed. Historical sample
age still needs independent methodology/coverage handling in FQ-12.

Commands from backend/:

```sh
ANPFUEL_TEST_QUALITY_PREPARER_PATH=/tmp/fq-native-rust/debug/prepare_quality GOCACHE=/tmp/fq-completion-go-cache go test -count=1 -race -tags=integration ./cmd/worker ./internal/modules/quality/adapters ./internal/modules/quality/adapters/jobs ./internal/platform/config ./internal/modules/official/adapters/source
GOCACHE=/tmp/fq-completion-go-cache go vet ./cmd/worker ./internal/modules/quality/adapters/... ./internal/platform/config
GOCACHE=/tmp/fq-completion-go-cache go build ./cmd/worker
XDG_CACHE_HOME=/tmp/fq-completion-staticcheck GOTOOLCHAIN=go1.27.1 GOCACHE=/tmp/fq-completion-static-cache staticcheck ./cmd/worker ./internal/modules/quality/adapters/... ./internal/platform/config
sqlc vet
sqlc generate
```

PASS: worker2.635s, quality adapters24.819s, quality jobs8.560s, config1.009s,
HTTPS source5.617s, all with race detection; vet/build/staticcheck/sqlc PASS.
Generated files were hashed before/after regeneration: no drift. Built worker
output was removed from the workspace. Format, changed docs links, whitespace
and tracked/changed/untracked secret/PII review PASS. No migration or dependency
addition; no unrelated aggregate release run.

## Explicit next obligations

This is composition acceptance only. Keep imports OFF. The live guard does not
automatically refresh a previously completed publication when Directory changes
later. Frozen active links can therefore remain visible until a new attempt
re-resolves them. Define durable invalidation/revalidation before activation,
preserving historical access and lease/ETag/cursor semantics. The tested manual
fresh retry above is not proof of an automatic completed-job refresh mechanism.

Bounded live discovery/scan progress, non-starving created-versus-deduped enqueue,
explicit scheduling and actual static Rust worker image/container proof remain
owed in FQ-05W. FQ-06/07 runtime coverage also requires actual reconciliation,
not an inference from Rust-only adapter acceptance. Then FQ-12–17 and the final
protected dev→main batch remain pending; D is NOT_AUTHORIZED.

The maintainer requested stopping after this microstage and handing the remaining
work to another agent: [continuation prompt](../../../planning/FUEL_QUALITY_REMAINING_HANDOFF.md).

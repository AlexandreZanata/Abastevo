# FQ-07R6D — Bounded Directory company evidence in the fine worker

Status: LOCAL_DONE
Validation: PASS

Parent 5c60d5e6, maintained dev. B-BR-FQ-01/02/06/07/11/14/16;
BUC-FQ-02/03/04. One runtime composition checkpoint, activation stays off.

Before behavior: actual fine worker preparation uses complete validated accepted
observations to plan company-only associations through bounded fresh Directory
read ports. Share lookups per exact CNPJ and batch512; use a Directory-owned
bounded foreign-branch query, never one query per observation or per unknown
identifier. Retain exact/retired/ambiguous history and all review outcomes. Base
matching only routes unassociated review. Query/index plan must remain bounded
against realistic retained identifiers; no speculative index/dependency addition.

The file importer preserves its explicit older nil-association mode for existing
callers, while the actual worker always composes planning and a typed Directory
transaction guard receiving actual derived bindings. When association planning
is configured, a failure/nil/partial/malformed plan refuses the whole attempt;
there is no silent fallback. Validate native source before identity I/O. Keep
planning under the existing whole-attempt lease and raw cleanup before publication.
Watermark/recompute/replay/superseded/dead budgets stay unchanged. Quality import
never creates or writes a Directory station/identifier.

Immediate real Directory batch/single equivalence, pre-I/O invalid input, closed
pool/cancellation, chunk/EXPLAIN, actual worker positive/review/no-write/retirement
race/cleanup/ack recovery and whole-source worker acceptance are mandatory. No
public station filter/live-read attribution, revalidation/automatic rebound,
Android source journey, activation, D or deployment follows. Follow with separately
accepted scoped current/frozen reads and revalidation; FQ-06/Android/FQ-16/17 owed.


## Delivered and verified, 2026-10-20

Directory owns SourceOtherBranchesBatch/OtherBranches: canonical full identifiers
validate before any query; requests deduplicate/sort and chunk at512. Results
contain only requested full CNPJs that have foreign-branch review evidence, never
a foreign station assignment. Explicit bytewise pattern_ops prefix bounds plus
the original LIKE predicate reuse the retained-identity index from102, including
alphanumeric/leading-zero identifiers. No new migration/index/dependency exists.
Read-only queries remain in Directory's generated package; quality does not write
or import its SQL. Invalid, closed-pool and cancelled calls fail with redacted
errors and no partial result. Empty requests require no storage.

The actual nativeFineImport factory always composes fresh company planning and
a typed Directory Guard. PlanCompanyAssociations requests exact evidence in512-
CNPJ batches; only identifiers with no exact retained evidence need the bounded
foreign-branch query. Known retired/ambiguous candidates retain their actual
review semantics. One injected UTC operational day belongs to the whole attempt,
independently of source DAY/MONTH/due facts. The typed guard translates actual
source-derived bindings to Directory's existing transaction port. No station or
identifier is created, changed or retired by quality import.

FineFileImport optionally supports explicit pre-association callers for backward
compatibility; the production factory always installs the new port. Native
source input validates before identity lookup. A configured association port
that fails or returns nil/partial/malformed decisions refuses with no fallback;
all raw artifacts are cleaned on failure. Planning stays under the common source
lease, then source re-verification/confirmed raw cleanup precede atomic guarded
publication and existing watermark/recompute acknowledgment.

Tests prove scalar/batch branch equivalence, numeric/alphanumeric/leading-zero
branches, duplicate elimination, invalid-before-I/O, closed-pool/cancelled errors,
and514 distinct identifiers with exactly512+2 query batches. Actual512-prefix
EXPLAIN over20,001 retained identifiers uses the existing owned history index
without a sequential identifier scan. The old Directory index-upgrade test was
found stale at105; its expected suffix alone now includes106–109. Baseline scan,
post-index plan, replay and prior assertions remain unchanged; full Directory
package passes against the current chain.

Actual discovery/dispatcher→Rust→Directory→PostGIS→read/correction passes for2025,
2024,2021 and grouped2016–2019. A preseeded matching company yields three frozen
bound decisions across the original/corrected revisions, with exactly one existing
station/identifier and zero dated PMQC links; import performs no identity writes.
Both source contracts additionally refuse association error/nil/partial output
while closing raw artifacts exactly once. Real retirement after planning but
before publication refuses the stale prepared binding, preserves pointer/watermark
and cleans raw files. A fresh retry records unassociated succession review; an
injected watermark failure then replays its exact frozen revision, restores the
ack and creates no duplicate source/decision/identity rows.

Whole private official corpus acceptance passes the actual current worker,
including real Directory lookup/planning, typed publication, replay and full public
keyset traversal:35,911 source records =35,612 accepted observations +293 CPF
quarantine +6 metadata, zero duplicates. The deliberately empty Directory yields
35,612 explicit unmatched company decisions and seven policy contexts, with zero
station/identifier creation. This tests complete loss accounting, not national
station coverage. All private raw workspaces are removed. PASS74.313s; each
whole-attempt remains below its210-second budget in this local acceptance.

## Commands and results

From backend/, trusted preparer environment from backend/README.md and local
loopback5434 disposable PostGIS, GOCACHE=/tmp/fq-completion-go-cache:

- go test -race -tags=integration ./internal/modules/directory/adapters
  -run 'TestSourceOtherBranchesBatch|TestSourceIdentityHistoricalIndexPlan'
  -count=1 -v:PASS3.179s, actual retained-index plans.
- Native fine worker/jobs regression including actual discovery/correction:
  PASS(worker4.150s,jobs2.727s); retirement/ack-loss test PASS1.762s.
- go test -race -tags=integration,qualitysource ./cmd/worker
  -run TestOfficialSevenFileWorkerPipeline -count=1 -v:PASS74.313s.
- go test -race -tags=integration ./internal/modules/quality/... ./cmd/worker
  ./internal/modules/directory/adapters ./internal/platform/migrate
  ./internal/platform/apicontract -count=1:12 packages PASS
  (quality adapters62.464s,http28.941s,jobs23.939s,native1.230s,prepared1.159s,
  read14.311s,application1.115s,domain1.025s,worker12.068s,
  Directory31.540s,migrate6.962s,apicontract6.855s).
- go build ./...; affected go vet/staticcheck:PASS.
- sqlc vet/generate stable whole generated-query byte hashes:PASS.
- gofmt, diff/local-link and staged secret-surface review:PASS.

INTEGRATION_PENDING. Both import/discovery controls remain off;109 was not applied
to VPS. Actual public station filtering/live association and historical labels,
Directory revalidation/rebound policy, linked Android journeys, FQ-06 actual XLSX,
full FQ-16/17 and final protected delivery remain OWED. No production/device/D
acceptance or deployed image is inferred. Next FQ-07R6E: lease-bound station-scoped
company observation reads with honest live/frozen attribution and actual HTTP/
privacy/cursor/ETag/Directory-change tests; then revalidation and app integration.

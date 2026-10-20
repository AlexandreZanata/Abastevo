# FQ-07R6C — Atomic scoped company publication

Status: LOCAL_DONE
Validation: PASS

Parent 9737b22f, maintained dev. B-BR-FQ-01/02/06/07/11/14/16;
BUC-FQ-02/03/04. One immutable publication prerequisite; runtime/read follow.

Before behavior: native2025 and six historical fine inputs optionally carry one
complete typed company decision census. Nil preserves all prior publication-key
bytes and means no association evidence; explicit empty census freezes the policy
for an empty native source. Validate exact source keys, one decision per accepted
observation, same policy/operational decision day, canonical station IDs, strict
kind/reason/scope/unknown-historical-window and one consistent company verdict
per full source CNPJ before I/O. No partial/subset/foreign decision is admitted.

Freeze the sorted policy/scope/company decisions in an optional publication-key
member, excluding operational decision day. A later-day identical replay keeps
its original frozen day. Changed association creates a correction, never mutates
old facts; superseded replay cannot resurrect it. Preserve legacy/2025/history
keys when no new policy input is supplied.

Stage source facts, company context and complete decisions in the existing
transaction; verify full final decision count. Any context/row/guard/census error
rolls everything back with the prior current pointer intact. Derive unique full-
CNPJ/station bindings from source rows and validated decisions; invoke an explicit
typed Directory guard with those actual bindings on every bound publication and
replay. Bound input without this port fails before database I/O; a generic unrelated
guard cannot authorize a mismatched declared station. Keep existing guards active.

Immediate application negative/identity/replay-key and real PostGIS native/history/
empty/replay/correction/superseded/rollback/concurrent convergence/actual Directory
retirement and mismatched-station guard tests are mandatory. No public visibility,
runtime source linking, activation, D, VPS/device/production acceptance is inferred.


## Delivered and verified, 2026-10-20

NativeFineInput and HistoricalFineInput optionally carry CompanyAssociationInput.
Before I/O, Bindings validates the complete accepted source-key census, kernel-
checked canonical full CNPJs, exact policy/day, canonical station IDs and allowed
kind/reason/scope/false-historical-window. Repeated observations of the same CNPJ
must share one consistent verdict; a missing, duplicate, foreign or contradictory
decision refuses the whole input. Bound bindings are derived uniquely from the
actual source CNPJ/declared station, sorted and passed explicitly to a typed
WithCompanyGuard port. A missing typed guard refuses bound requests before pool,
ID maker or generic guard access; an unrelated generic guard cannot substitute.
WithGuard/WithCompanyGuard preserve each other in immutable publisher copies.

The sorted publication identity includes policy/scope and complete decisions but
excludes the operational day. Its optional JSON member is omitted when nil,
preserving old memberless/2025/historical key bytes. Empty explicit policy is
distinct from missing evidence and freezes a zero denominator. Identical later-
day replay retains the original context day. Changed verdict creates a new
correction key; old provenance/association rows remain unchanged and superseded
input cannot resurrect them. Paths without company evidence avoid new map work.

Both fine publishers stage original clock/source events/notes first, then company
context and all decisions in the same existing publication transaction. Every
stage must return its row and final decision count must match the accepted
census. A later insert failure or corrupt-stage count divergence rolls back the
source/context/preceding decisions/current pointer. Generic guards stay active;
the typed Directory guard runs inside that transaction on every bound creation
and replay, before source mutations. The storage policy grants no public/current
station attribution until actual read/runtime composition is accepted.

RED application tests and frozen-key tests first failed for absent typed symbols.
Fourteen application negative cases, sorted/day-independent identity, repeated
CNPJ deduplication, explicit-empty/nil behavior and source-level pre-I/O refusal
PASS. Existing legacy/native/history publication key compatibility tests PASS.
Actual Directory fixtures, not stubs, prove all seven fine profiles can publish,
replay and correct company associations, refuse a mismatched declared station
and refuse retired-identity replay; original unassociated and bound revisions
stay frozen. Generic+typed guard composition is exercised. Eight simultaneous
publishers per2025/2021/grouped profile converge. A real later-row trigger failure
and a second trigger corrupting the staged census both roll back completely;
recovery and superseded refusal PASS. All seven explicit empty profiles retain
zero count, source precision and original operational decision day across replay.

The reused real Directory guard separately passes retirement/status rejection,
observed concurrent retirement/status locking through transaction completion,
and bounded chunk coverage; no new Directory writes/query ownership were added.

## Commands and results

From backend/, trusted preparer environment from backend/README.md, disposable
loopback5434 PostGIS databases, GOCACHE=/tmp/fq-completion-go-cache:

- go test -race ./internal/modules/quality/application
  ./internal/modules/quality/adapters -count=1:PASS.
- go test -race -tags=integration ./internal/modules/quality/adapters
  -run TestCompanyPublication -count=1 -v:PASS; expanded final cases are included
  in the final matrix below.
- go test -race -tags=integration ./internal/modules/quality/... ./cmd/worker
  ./internal/platform/migrate ./internal/platform/apicontract -count=1:
 11 packages PASS (adapters52.426s,http23.968s,jobs20.828s,native1.228s,
 prepared1.160s,read12.256s,application1.118s,domain1.017s,worker9.956s,
 migrate6.675s,apicontract6.194s).
- go test -race -tags=integration ./internal/modules/directory/adapters
  -run TestSourceBindingGuard -count=1 -v:PASS6.237s.
- go build ./...; affected go vet/staticcheck:PASS.
- gofmt, diff/local Markdown-link and staged secret-surface review:PASS.

INTEGRATION_PENDING. No new migration in this slice (109 remains local only).
No actual fine-worker association producer, public station filter/live binding
read, revalidation, linked Android flow, activation/deployment/native-device/
production certification or D work is claimed. Next FQ-07R6D: bounded per-attempt
Directory evidence and typed guard composition in the actual fine file worker,
then current/frozen source reads and revalidation without dead-budget resets.
Actual FQ-06 XLSX, full linked Android/FQ-16/17 and protected delivery remain OWED.

# FQ-07R6B — Compatible scoped company association ledger

Status: LOCAL_DONE
Validation: PASS

Parent 91ad38c0, maintained dev. B-BR-FQ-01/02/06/07/11/14/16;
BUC-FQ-02/03/04. One storage prerequisite; runtime/publication follow separately.

Before behavior: append migration109 only. Preserve all existing source events,
revision provenance/current pointers, dated PMQC links and108 source contexts.
Do not backfill unknown company associations. Separate revision context freezes
current-company-v1, CURRENT_COMPANY_ONLY, operational decision day, unknown
historical business window and accepted-observation denominator. Separate rows
bind one same-revision native fine observation key to a resolution kind/reason
and optional canonical station. Bound decisions alone carry a station; all review
outcomes remain unassociated. Source company CNPJ is derived from the event,
never copied/reinterpreted as historical ownership or a chemistry/legal finding.

Both context and row operations require the correct native/historical sanction
source and staging revision, lock/recheck its state against publication/withdrawal
and fail closed on collisions. Context count must equal staged native observations;
empty native revisions retain an explicit zero denominator. FK/unique/policy/
calendar/scope/reason/kind/station constraints must refuse malformed/foreign rows.
No event/due/extraction date substitutes for the decision clock. Read/count queries
remain private storage ports, with no public station visibility or current identity
claim until actual guarded publication/read composition is accepted.

Immediate real108→109/replay/legacy-preservation, native/historical/empty source,
negative constraints/foreign stream, eight-writer convergence, observed revision-
lock withdrawal barriers and failed-stage atomic rollback tests are mandatory.
Existing migration acceptance assertions advance their expected suffix only;
applied SQL/checksums and prior assertions remain intact. Generated sqlc output
must reproduce cleanly. No new dependency, source activation, D, deployment or
native-device/production acceptance. Next atomic immutable publication/live guards.


## Delivered and verified, 2026-10-20

Append-only109 adds separate quality_company_association_revisions and
quality_company_associations. No old table is altered or backfilled. Revision
context freezes exact policy/scope, finite supported-calendar operational decision
day, false historical-business-window knowledge and0–500000 accepted-observation
count. Decisions reference their same-revision source event and context, with
one unique observation key and optional Directory station FK. Only bound plus its
exact policy reason can carry a station; all four review kinds require NULL and
their specific reasons. A partial station/revision/key index supports future
scoped history. Source CNPJ is derived by joining the original fine event.

Context insertion requires a native2025 or verified historical source context,
correct sanctions stream and staging revision, exactly counts the existing staged
native observations and refuses legacy synthetic rows. Decision insertion requires
that context and the same-revision native event. Both operations lock/recheck the
revision FOR SHARE, refuse conflicts without false acknowledgment and expose no
update operation. Internal context/decision/count reads keep the five resolution
denominators distinct. Future atomic publication must stage all source events
first, stage the complete decision census, verify final counts and apply actual
Directory live guards; these storage queries alone do not certify current identity
or create public station visibility.

Six real-PostGIS test functions pass:

-108→109 migration/replay keeps complete frozen JSON hashes of15 old tables
 (source manifests/resources/provenance/periods/events/assays/PMQC links/current
 pointers/native clocks/history contexts/notes) identical. Both new tables remain
 empty; old native publication replay keeps its revision ID and all legacy/native/
 historical leases remain reachable.
- All seven source profiles and empty revisions retain exact context/census;
 five resolutions each sum to the accepted-observation denominator, with original
 source CNPJ and no station on review cases.
- Fifteen negative context/row cases refuse policy/scope/invented historical
 window/calendar/census/kind/reason/station/foreign record/revision violations.
 Missing context/native profile, foreign stream and non-staging revisions refuse.
- Eight concurrent context writers and eight decision writers converge to exactly
 one row each; losing statements return no row rather than false success.
- Two observed pg_blocking_pids barriers prove context and decision stages wait
 on the revision lock, recheck withdrawal and persist no new facts afterward.
- A later invalid bound decision rolls back source clock, both source events,
 company context and preceding good decision in the same transaction.

RED first failed on absent query operations. sqlc's initial operator parsing of
unspaced =@ failed the typed compile check before any database execution; explicit
parameter spacing fixed it. Real storage tests then passed. Five older upgrade
assertions only append109 to their expected migration suffix; previous checks and
all applied SQL remain unchanged. No tests were weakened.

## Commands and results

From backend/, trusted preparer environment from backend/README.md and local
loopback5434 disposable PostGIS databases, GOCACHE=/tmp/fq-completion-go-cache:

- go test -race -tags=integration ./internal/modules/quality/adapters
  -run TestCompanyAssociationStorage -count=1 -v:PASS4.275s.
- go test -race -tags=integration ./internal/modules/quality/...
  ./internal/platform/migrate ./internal/platform/apicontract -count=1:
  ten packages PASS (adapters44.716s,http21.683s,jobs17.661s,native1.195s,
  prepared1.127s,read9.771s,application1.098s,domain1.017s,migrate5.165s,
  apicontract6.094s). This includes the final expanded15-table upgrade snapshot.
- go build ./...; affected go vet/staticcheck:PASS.
- sqlc vet and generate, whole generated-query byte hashes unchanged on repeat:
  PASS; only the owned quality package gains models/queries.
- gofmt, diff, local Markdown links and staged secret-surface review:PASS.

INTEGRATION_PENDING. No migration was applied to VPS; no public association,
company publication key, runtime Directory lookup/live fence, station history,
revalidation, Android source journey or source activation is claimed. Next
FQ-07R6C: validate full typed decision census, optional frozen publication identity
preserving old keys, one atomic source/context/decision publication and live
transaction guard/replay/rollback/concurrency acceptance; then actual worker/read/
revalidation composition. FQ-06 XLSX, full FQ-16/17 and protected final delivery
remain OWED. D stays held; device/production certification is not inferred.

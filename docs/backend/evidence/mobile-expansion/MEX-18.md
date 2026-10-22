# MEX-18 — Scoped reader prerequisite acceptance

Status: LOCAL_DONE
Validation: PASS

Parent: `fa7632f4`, maintained dev. B-BR-MEX-11/14/17; BUC-MEX-07–09.
Before behavior: positive page/detail holds actual Directory binding/status locks
and source-revision validity through its read transaction. Withdrawal, cancelled
or failed reads never return partial station attribution. Fresh empty scopes,
legacy/missing/invalid association evidence remain distinct; exact fingerprints
change with fresh allowed bindings. Global history remains source-only.
Observe concurrent actual retirement/status writers waiting until page/detail
commit/cancel, not just a pre-read guard call. Explain actual generated queries
with realistic unrelated rows and exercise all six historical native profiles.
No HTTP/source activation until MEX-19 independently passes.


## Delivered and proved

Real RED exposed cancelled-empty page/detail acceptance and positive page/detail
from a withdrawn lease. Both scoped reads now check context first and hold a
sanctions/source/stream revision FOR SHARE lock, accepting published/superseded
history only, through Directory guards, actual generated row reads and commit.
No cross-module SQL, schema/dependency or weakened constraint was introduced.

Eight actual PostGIS scenarios (page/detail × retirement/status × commit/cancel)
park the reader in its real generated SQL behind an exclusive event-table lock,
then observe pg_blocking_pids for the actual Directory writer. Writers stay
blocked through the read, resume on commit/cancel and leave no partial result or
lock leak. Fresh resolution changes the fingerprint and returns an honest empty
scope after retirement/status change; global source facts remain accessible.

Missing/failing identity ports, guard failure, real SQL table-fault injection,
divergent census and missing legacy association evidence refuse without positive
attribution. A verified zero-row source is separately available/empty. All six
historical profiles preserve reference labels/refs, unknown paid=NULL, due/source
transit/clock precision and unlabelled values in both scoped page and detail,
without manufactured incident/decision dates.

EXPLAIN(ANALYZE,BUFFERS,FORMAT JSON) uses the exact captured generated page/detail
queries against20,004 synthetic event rows. The20,000 unrelated events/associations
are excluded by indexed access; structural assertions forbid sequential scans
of those large tables or returning more than the four fixture observations.
The initial load fixture used an invalid content-key prefix; its real constraint
refused it, then the fixture was corrected without changing any constraint.

Final quality/all + Directory/adapters integration race matrix9 packages PASS;
added verified-empty case PASS afterward. go vet/staticcheck(sql analysis uses
existing Go1.27.1 workaround), sqlc vet/diff, gofmt, diff and secret review PASS.
Logs `/tmp/bfp-mex18-{red,green,locks,acceptance,plans,full,empty}.log`.
MEX-17's formatted-identity scan initially flagged only an intentionally invalid
all-one synthetic checksum-shape test; manual review/recheck confirms no private
source identity in committed census/metadata. No source/HTTP activation or
production certification follows; next MEX-19.

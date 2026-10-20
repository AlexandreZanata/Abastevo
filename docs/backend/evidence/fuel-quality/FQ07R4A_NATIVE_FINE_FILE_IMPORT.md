# FQ-07R4A — Bounded native fine file/process/job boundary

Status: LOCAL_DONE
Validation: PASS

Parentf4bed458, maintained dev. B-BR-FQ-01/02/06/07/14/16;
BUC-FQ-03/04/06. One bounded import prerequisite; no schedule/runtime activation.

Before behavior: accept only the exact official HTTPS MAV CSV URL/year profile,
canonical planned hash and strict versioned payload. Fetch capability owns its
private artifact/path/cleanup; job payload never chooses a filesystem path or
native binary. Hold the existing separate-connection source serial lease across
fetch, snapshot/process/verified typed load, atomic native publication and durable
watermark. Verify planned bytes before and after preparation. Native process uses
an immutable operator-owned path, bounded context/time/bytes, a private copied
snapshot and private output artifacts; discard process output and remove owned
workspaces on all outcomes. No partial batch/publication/watermark on pre-publication failure; a later acknowledgement failure must replay the committed revision.

Use the native typed publisher/source clock contract, not legacy SanctionInput.
Replay and failed watermark recovery converge without stale resurrection. No
station association is inferred here: Directory identity binding/guards and
station-specific read journeys remain explicit subsequent work, and due dates
cannot be used as occurrence/identity clocks. Tests must prove strict input before
I/O, trusted binary/source integrity, cancellation/redaction/cleanup, actual
Rust→PostGIS publication/replay/correction and no partial failure/foreign stream
mutation immediately. Worker/discovery/image wiring follows after this boundary.
D, source activation, VPS and production/native-device certification remain held.

## Delivered and verified, 2026-10-19

FineRunner accepts only www.gov.br HTTPS MAV annual CSV filenames and the exact
resource year, with no credentials/query/fragment/port/path-encoding escape. It
checks the operator-owned executable permissions and a private regular source
file, pins the open descriptor/hash/bytes, creates a private verified process
snapshot with the official filename, and checks both snapshot and original again
after execution. Context bounds cover hashing/copy/process; default90s,
maximum120s, inside the existing210s source serial attempt. Process environment
is minimal, stdout/stderr discarded, manifest/artifact reads bounded and typed.
No process or source failure returns partial observations. Owned workspace
cleanup failure invalidates the returned batch.

FineFileImport validates strict version/source/stream/year/URL/memberless hash
payload before fetch/prepare. Paths, executable and workspace roots never come
from job input. The existing separate-connection native source advisory lease
covers the whole attempt. Download cleanup is confirmed after verified typed
preparation and before publication, so raw names/CPF do not survive successful
intake. Only minimized typed observations proceed to the native atomic publisher.
Publication/watermark/derived-intent acknowledgements converge on retry; a
superseded source replay acknowledges without resurrecting a revision or moving
the watermark. No station association is inferred or stored here.

New integration tests require the actual trusted fine CLI, not an optional skip.
The affected backend CI build now compiles prepare_sanctions with --locked,
sets mode755 and exports ANPFUEL_TEST_SANCTIONS_PREPARER_PATH. The current backend
README reproducer includes the same prerequisite. This edits CI configuration
only; no remote CI run or activation is claimed. Worker image/runtime/discovery
registration remains FQ-07R4B.

## Validation

RED new native/file-job contracts absent, then GREEN:

- Native boundary cases: exact URL/year and unsafe variants; private source/
  hash/symlink/trusted executable checks; cancelled context and failed process
  redaction/workspace cleanup; real deadline cancellation; a successful fake
  process that modifies only its copied snapshot is refused by integrity checks
  before any artifact load, leaving the original untouched.
- File-job cases: invalid/missing/ambiguous/oversize payload/ports before I/O;
  fetch, preparation, source mutation, cancellation and cleanup failures remove
  owned source material once and return redacted errors without publication.
- Actual Rust→typed boundary→PostGIS scenario (no mocked parser/publisher/lease):
  three synthetic source records reconcile1 accepted+1 duplicate+1 CPF quarantine;
  source month/due/paid fields survive. Cleanup failure blocks even a fully
  parsed revision. Injected post-publication watermark failure leaves the valid
  committed revision but no false watermark acknowledgement; removing the
  failure and replaying converges to that same revision. Correction creates
  one new revision; stale bytes fail before prepare; replay of verified old
  bytes cannot resurrect superseded facts; original historical facts remain.
  Malformed source cannot publish or leak raw error text. All eight attempts
  clean both source and process workspaces; exactly two revisions and zero
  invented station links remain.

```sh
# backend/, disposable local PostGIS only
ANPFUEL_TEST_QUALITY_PREPARER_PATH=/absolute/trusted/prepare_quality ANPFUEL_TEST_SANCTIONS_PREPARER_PATH=/absolute/trusted/prepare_sanctions ANPFUEL_TEST_PREPARER_PATH=/absolute/trusted/prepare_registry GOCACHE=/tmp/fq-completion-go-cache go test -race -tags=integration ./internal/modules/quality/adapters/native ./internal/modules/quality/adapters/jobs ./internal/modules/quality/adapters/prepared -count=1
GOCACHE=/tmp/fq-completion-go-cache go vet ./internal/modules/quality/adapters/native ./internal/modules/quality/adapters/jobs
GOCACHE=/tmp/fq-completion-go-cache go build ./cmd/api ./cmd/worker
XDG_CACHE_HOME=/tmp/fq-completion-staticcheck GOTOOLCHAIN=go1.27.1 GOCACHE=/tmp/fq-completion-static-cache staticcheck ./internal/modules/quality/adapters/native ./internal/modules/quality/adapters/jobs
```

PASS logs `/tmp/fq07r4a-final-integration.log`: native1.128s, jobs9.638s,
prepared1.058s; focused actual native recovery scenario1.666s
(`/tmp/fq07r4a-integration.log`), scoped unit race/static/build PASS.
Workflow YAML parse, gofmt/diff/secret/new-file/link checks PASS. Existing PMQC
native/file-job regressions pass; shared artifact-name allowlist adds only the
native sanctions output, with each loader still enforcing its own manifest.
No new dependency, SQL/migration/API/Android change, production photo or private
source artifact is committed.

## Recovery and next

The new kind has no producer/runtime registration yet. Keep it disabled until
FQ-07R4B reserves/registers it, composes verified HTTPS fetch/discovery, ships the
static preparer and tests disabled-claim/image behavior. No native source claim
is enabled by this checkpoint. A confirmed publication followed by failed
watermark/queue acknowledgement is retryable, not rolled back or falsely hidden;
all pre-publication failures preserve the old visible revision. Cleanup failure
blocks publication and needs successful owned-file cleanup before retry; never
remove unrelated files or reset a database. Source/link/clock privacy constraints
and D deferral remain intact. Safe Directory associations, FQ-06 XLSX, linked
Android/FQ-17/protected batch delivery remain OWED.

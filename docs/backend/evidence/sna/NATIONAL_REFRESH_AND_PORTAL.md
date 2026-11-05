# National registry continuity and portal discovery — 2026-11-05

Status: LOCAL_DONE
Validation: PASS

## Behavior contract

B-BR-NAT-01: The fiscal portal exposes labeled state and municipality selectors,
with municipalities constrained to the selected state. Place filtering applies
to the full admitted edition, independent of its HTML pagination. Names/CNPJ
can further narrow results. Clearing or changing state clears an incompatible
city. Late asynchronous responses cannot replace a newer selection. Empty
coverage is explicit; unmatched/ambiguous fines never acquire an inferred
city or station. Retain source dates, status and safe association limits.
The website remains static: browser filtering reads immutable release-addressed
JSON files only, never an API/database search or per-visitor backend rendering.

B-BR-NAT-02: Every station heading in the public portal uses the canonical
public display name. A known distinct CPF-minimized legal name appears below
at lower emphasis. Equal/unknown secondary names are omitted. Both names are
searchable. Snapshot hashes and Directory change detection include this
metadata so a name-only correction regenerates the static portal. Preserve
legacy snapshot compatibility, official amounts, IDs and source attribution.

B-BR-NAT-03: National refresh discovers complete official editions, freezes
source/name/reference bytes, admits only validated rows and publishes through
owned idempotent ports. Compare edition membership, record additions/changes/
missing identities and expose last success/staleness. Absence from one remessa
is not proof of closure: retain history and independent community facts;
explicit official revocation or accepted correction owns operation status.
Failed/partial/suspicious editions retain the previous successful publication.
New ANP identities must participate in the automatic name-enrichment step,
without relying on a manually prepared list of today's establishments.

B-BR-NAT-04: The user selected a VPS-local monthly OpenCNPJ snapshot. A
short-lived scheduled updater checks the official source for a newer complete
edition, verifies it before use and retains the previous snapshot. Daily ANP
refresh joins its current CNPJ membership against that snapshot automatically;
unchanged source pairs reuse a hash-bound minimized cache. Source update and
name export share a filesystem lock. Bounded process/disk/deadline guards and
private permissions protect the shared host. Logs contain only stage/count
metadata; no OpenCNPJ company/partner/contact output. The website remains
static and consumes only newly generated immutable portal publications.

BUC-NAT: Exercise place changes, out-of-order responses, no-result cities,
both-name search and unknown/equal names; verify real PostGIS export/change
detection. Replay national additions/removals/changed names and interruptions
with deterministic source fixtures and actual owned loaders. Publish one
cumulative protected dev → main batch; only observed runtime receipts establish
deployment and freshness.

## Portal checkpoint

Portal status: LOCAL_DONE / INTEGRATION_PENDING.

The static generator now carries the minimized optional legal name through
Directory export, snapshot hashing, station pages, rankings, fiscal records
and all search files. Prefixes/search text include both names. Unknown/equal
secondary labels are omitted. Source hashes remain in a disclosure below the
visible edition/date. State/city filters read immutable JSON only; no website
API/search service is introduced. Working filters hide the unfiltered HTML
fallback, which remains usable without JavaScript or after a file failure.

Validation: 78 landing tests, TypeScript/build, portal/snapshot/job Go suites,
real PostGIS export and name-only fingerprint cases, Go vet, sqlc generate/
vet/drift, whitespace and secret scan passed. The browser selected MT/Sorriso
and displayed the linked fixture's trade/secondary legal name; at 390px there
was no horizontal overflow. The new filter regression was red before behavior
and green afterward. National source automation is the next local phase;
this checkpoint does not claim staging publication.


## National continuity checkpoint

National status: LOCAL_DONE / INTEGRATION_PENDING. Tested source is the
code in this checkpoint, based on merged `3fc77acc` (PR #191). Maintained dev
is occupied by the independent forum implementation; this work remains on
`codex/national-refresh-portal` in an isolated persistent worktree. No forum
code is included or authorized for publication by this task.

The updater separates mutable RFB build files from immutable hash-addressed
published snapshots, checks source ZIPs before pointer replacement and keeps
an independent previous snapshot. The daily worker compares membership,
checks admission accounting before load, automatically minimizes all current
CNPJs and binds pending recovery/cache to both complete source hashes. Source,
cache and state locks cover overlapping writers. Retention and interrupted
copy cleanup refuse links/unowned artifacts and preserve neighboring data.
See the [operator guide](../../../../infra/deploy/registry/README.md).

Immediate validation passed:

- Go race tests for station-sync, registry-sync, portal, snapshots and export
  jobs; affected vet/staticcheck, sqlc generate/vet, four staging render tests,
  systemd unit syntax, whitespace and tracked-secret review. The CLI recovery
  compile error was fixed before the accepted worker run.
- Negative cases: empty/truncated/duplicate CSV, ANP literal quotes, suspicious
  source drop, excessive/inconsistent quarantine, frozen report/source/cache
  corruption, changed monthly source with unchanged ANP, interrupted load,
  source/cache/worker overlap, failed verification, same-month correction,
  cancellation, source disk cap and owned retention boundaries.
- Real Rust/PostGIS daily recovery and overlap, canonical lifecycle/replay,
  export and legal-name-only invalidation. The national publication below used
  another fresh disposable database, never staging or a development DB.
- 78 landing tests, TypeScript, build/check and static-file-only request/race
  regressions passed. Browser checks verified MT/Sorriso, a distinct secondary
  legal name, hidden unfiltered fallback/more button and no horizontal overflow.

The current official ANP download on 2026-11-05 contains 45,807 unique CNPJs,
SHA-256 `3214cbf2a3bfc470a0922f6403fc85e162cb167d86e2ff1e773ce458eaf5fbee`.
Against the VPS's frozen preceding edition, the report records 0 added,
42 changed, 9 missing and 45,765 unchanged identities. No missing identity
causes automatic closure or deletion.

The real OpenCNPJ snapshot (2026-10), SHA-256
`76f36078c1e5b727af8416262309d1285e39f054514218a75a46206d5786037b`,
passed complete verification against official ZIPs and matched all 45,807
current ANP CNPJs. The minimized export reports 45,778 active and 32,218 active
nonblank names. The full adapter completed in 202.86 seconds in a networkless,
read-only-root container limited to 512 MiB and 500m CPU; an observed sample
used about 330 MiB. These measured limits replace the insufficient 256 MiB
job limit and fit the existing 8 GiB namespace quota without expanding it.

Rust admitted 45,695 assertions and quarantined 112 (0.245%). The complete
national batch published, independently verified and replayed successfully
in disposable PostGIS in 241.48 seconds: 45,695 stations/legal names and
31,634 usable trade names. Raw enterprise rows and source files remain outside
Git and logs; test output records counts only.

The source working directory was lost during this session. Source edits were
recovered from this chat's own tool history into the persistent worktree and
all affected local acceptance was repeated there before this checkpoint.
The new official download and full national acceptance above supersede the
earlier 45,816-row trial; no old receipt is claimed for new bytes.

## Observed staging preparation

The actual signed staging receipt still reports `3fc77acc`, schema floor 156.
The 8,333,664,200-byte snapshot was transferred privately, independently hashed
on the VPS and prepared in the owned UID-1000 source directory. Its receipt
explicitly records `scheduled_update_activated: false`. No new database edition,
portal filters, CronJob arguments or systemd timer is claimed active yet.

Next: resolve the occupied maintained-dev delivery order, protected integration,
verify the exact signed deployed revision, install the tested immutable tools
and source schedule, run a guarded registry publication, then read names and
static fiscal filters through staging and the actual Android app. No release
certification or unrelated forum delivery is implied.

## Signed landing search input continuity

B-BR-NAT-05: The portal worker reads the search program through a stable
filesystem input in each signed landing release. Its bytes equal that
release's fingerprinted search asset. Browsers continue using immutable
fingerprinted portal URLs, and website search remains static. A changed
TypeScript build must not require manually replacing a hash in worker config.

Observed staging config still names `assets/search.f37c980d.js`; the new
filter build emits another hash. A stable `portal-search.js` build input,
with revalidation caching, will move atomically with the signed landing
`current` pointer. Configure the worker and operator render input to use it
only after verifying that the deployed signed artifact contains it.

Alias checkpoint: LOCAL_DONE / INTEGRATION_PENDING. The actual old-hash
configuration was observed read-only before activation. A regression first
failed with ENOENT for `portal-search.js`, then passed after adding the alias
and no-cache header. All 78 landing tests, typecheck, build/check, byte-identical
build checks, whitespace and secret review pass. No backend or schema behavior
changes in this follow-up. The user-authorized isolated main delivery continues
without including the forum branch. Runtime config changes remain gated on
verification of the signed artifact's alias and fingerprinted bytes.

B-BR-NAT-06: Independent read-only Kubernetes pod, metrics and database probes
are collected concurrently within their existing eight-second individual
budgets and the existing twelve-second SSH snapshot deadline. Any missing,
failed or timed-out probe still refuses publication and stops only the exact
owned Job. Preserve pressure thresholds, identity checks and the frozen import.
The first live attempt was correctly stopped after the sequential probe chain
exceeded the snapshot deadline; no guard timeout is relaxed for the retry.

Telemetry checkpoint: LOCAL_DONE / INTEGRATION_PENDING. The old sequential
collector failed the three-probe barrier regression. The parallel collector
passed all 11 guard/accounting tests, including missing metrics, timeout,
pressure and ownership failures. Three real read-only VPS samples completed
in 3.327, 1.821 and 1.350 seconds with no resource violations. Individual
8-second and outer 12-second timeouts, pressure thresholds, source Job limits
and exact-owned-Job stop behavior are unchanged. After the interruption,
last success still referenced the previous ANP hash; there was no pending
publication or published minimized cache. The old Job was confirmed absent.

B-BR-NAT-07: Reuse verified minimized rows by full CNPJ and exact monthly RFB
snapshot hash across ANP editions. Changed ANP legal/address fields do not
require re-querying every RFB establishment. Query only identities not covered
by matching verified projections, retain deterministic output and reject
conflicting or malformed caches. The daily worker requires a matching source
projection before an uncached full-edition build. The existing DB-free monthly
oneshot prepares that projection under its existing larger resource budget;
its completion includes name preparation. No HTTP name service is installed.

The second live attempt was correctly stopped at host load1 5.382, above the
unchanged 5.2 reserve. Neither stopped attempt promoted a pending publication
or a minimized cache. A verified first cache may be bootstrapped from the
already accepted complete offline export, bound to the identical ANP and RFB
hashes. This is initialization; subsequent monthly/daily projections remain
automatic and new ANP CNPJs are queried rather than silently omitted.

Projection checkpoint: LOCAL_DONE / INTEGRATION_PENDING. The first regression
failed because an ANP legal-only change re-exported an existing CNPJ. Matching
RFB projections now supply verified rows across ANP editions, and only uncovered
CNPJs reach the CLI. Global CNPJ ordering keeps the descriptor/name bytes stable.
Daily mode refuses a new RFB snapshot without a matching projection; the DB-free
source oneshot downloads ANP and prepares names within its existing 110-minute,
8-GiB/one-CPU budget. No deadline, pressure reserve or daily limit was increased.

Race tests cover monthly/daily cache overlap, source writer exclusion, missing
seeds, changed RFB bytes, conflicting valid projections, forged provenance,
private fields, false counts, cancellation and failed downloads/cleanup. The
complete 45,807-CNPJ ANP set with one metadata-only change passed in a read-only,
network-disabled container limited to 512 MiB/0.5 CPU in 12.66 seconds. The CLI
wrapper refused every `export` command. Both outputs had name SHA-256
`3517e0f062154831bf9e477b435b7ff1319fb870a2520b4ac4f7f21415cdc160`,
with 45,807 matches and no missing identities. The earlier real complete
PostGIS publication/verification/replay therefore uses identical minimized
name bytes; this follow-up does not change SQL or publication policy.
Go race/vet and the repository-pinned Go 1.27.1 staticcheck pass.

Runtime continuation: the signed staging receipt confirms `5bf9d08a` and schema
156. The identical accepted offline projection was initialized privately in both
cache roots. A later external sampling attempt stopped before pending publication
when its SSH round trip exceeded 12 seconds. The same probes and ownership guard
were moved to a bounded VPS-local monitor, preserving each 8-second probe and
12-second sample deadline; initial host load 5.714 then refused starting the Job.
These are failed/refused attempts, not publication evidence. No unrelated service
was stopped, no names were logged, and the previous success remains intact.

B-BR-NAT-08: A replay whose every input was already complete and bound to the
identical admitted manifest verifies all immutable output bytes/row counts,
stored input accounting and membership counts, then resumes publication without
repeating per-row staging writes/lookups. Hold the existing transaction advisory
fence throughout. Unbound legacy/incomplete inputs still take full admission;
changed manifests, missing membership, altered streams and cancellation fail
closed. Preserve completion clocks, canonical identity and community facts.
This avoids repeating the completed national staging phase after a guarded stop.

Recovery checkpoint: LOCAL_DONE / INTEGRATION_PENDING. A real PostGIS trigger
first rejected the old completed replay because it attempted a membership INSERT.
The optimized path passes with that trigger enabled, while corrupted streams,
missing membership, changed stored accounting, cancellation and unbound legacy
inputs still refuse. Existing prepared input/manifest/rollback/disconnection,
concurrent replay and station-name/fact tests pass under race detection.
The actual 45,695-assertion national batch was admitted into disposable PostGIS;
its exact completed replay, with membership writes forbidden, took 580.339 ms
and retained 45,695 accepted / 112 quarantined. It reads all three streams and
keeps the original completion clock. No migration or publication rule changes.

Exact minimized-cache hits also now verify full ANP/RFB hash binding, competence
and descriptor provenance. The forged-owner regression failed before the fix,
then passed together with race, vet and pinned staticcheck. These checks apply
even when no name rebuilding is needed.

The last guarded VPS attempt committed the input staging phase and began
publishing source facts before load pressure stopped it. It is not a complete
edition: 250 trade names were observed at that point. The frozen pending batch
remains intact; recovery must continue it rather than fetch or erase it.

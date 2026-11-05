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

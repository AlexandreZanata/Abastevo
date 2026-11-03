# EVC-02 — Canonical source normalization

Status: LOCAL_DONE
Validation: PASS
Integration: INTEGRATION_PENDING

Bounded children, in order: EVC-02A socket/current and exact power primitives
plus the directly discovered legacy current/power decoding corrections;
EVC-02B the common minimized site model and source eligibility readers;
EVC-02C bounded CLI, real normalization/census and reproducibility. Finish all
before EVC-03. No source publication, DB import or staging change in this task.

B-BR-EV-NORM-02A: Socket mapping is frozen as contract data, keyed by explicit
OSM socket type and pinned OCM connection code. Unknown/ambiguous Tesla/domestic
labels never prove compatibility. GB/T AC/DC and CCS1/CCS2 remain distinct.
OCM current 10 and 20 are AC (single/three phase); 30 is DC. Never infer a
Brazilian domestic socket merely from country. Preserve group quantity and raw
code, never add across sources. Convert source decimal kW exactly to integer W;
unknown, unsupported fractional W, invalid or out-of-bounds power is null,
never rounded into a fabricated source assertion.

BUC-EVC-02A: Verify mapping and numerics from synthetic records and the pinned
reference metadata. Correct the existing reader's 20→DC error, with a RED test
that also covers 30→DC, and eliminate its f64 rounding of PowerKW. Keep source
text/lexemes private and preserve missing values. No new runtime service.

Primary references: [OSM sockets](https://wiki.openstreetmap.org/wiki/Key:socket),
[OSM charging sites](https://wiki.openstreetmap.org/wiki/Tag:amenity%3Dcharging_station),
[charge-point model](https://wiki.openstreetmap.org/wiki/Tag:man_made%3Dcharge_point),
[OCM pinned reference metadata](https://github.com/openchargemap/ocm-export/blob/8e3bedca48ca94807d96b2f5e7ee02cffe63ae54/data/referencedata.json).
The explicit NBR14136 spelling is recognized only when reported as such; the
pinned OCM reference has no NBR14136 code. Generic domestic and deprecated Tesla
keys are ambiguous and remain UNKNOWN. OSM documentation specifies unitless
socket output as kW; source-specific group data is retained without totals.

Dependency feature review (before implementation): enabling serde_json's
`raw_value` feature preserves exact per-connection numeric lexemes without
converting through f64. Existing pinned serde_json version/license (MIT OR
Apache-2.0), no new package/network dependency; Cargo.lock should stay unchanged.
Bounded source parsing, duplicate-field refusal and all existing Rust goldens
must pass before accepting the feature. This does not enable arbitrary-precision
behavior globally or change historical committed artifact bytes.

## EVC-02A checkpoint

Status: LOCAL_DONE
Validation: PASS

Tested parent fa9e43cf plus this child. RED reproduced the legacy AC-three-phase
bug (`DC` instead of `AC`); the new mapping/exact-power tests also failed before
the module existed. GREEN: frozen source lookup covers Type1/2, CCS1/2, CHAdeMO,
GB/T AC/DC, NACS, explicitly reported NBR14136 and a distinct legacy Tesla code;
unknown/ambiguous families stay UNKNOWN. Contract tests enforce unique source
codes and exercise every mapping. No capability is inferred from country.

The existing reader now maps OCM 20→AC and 30→DC. It binds PowerKW raw JSON tokens
to their own connection using serde_json RawValue, refuses duplicate known
numeric fields, and converts with checked decimal integer arithmetic. Missing,
invalid, out-of-range and fractional-W values remain null; no f64 rounding.
Synthetic regression tests cover multiple groups, unrelated nested/root values,
scientific notation and duplicate tokens. Raw values/unknown quantities remain
source-specific. Existing connector wire vocabulary is unchanged; the broader
family contract is for the upcoming canonical model.

Full Rust fast gate passes (fmt, clippy all targets, locked complete suite),
including prior byte/identity goldens and fourteen offline Python groups.
serde_json 1.0.151 MIT OR Apache-2.0 `raw_value=[]` adds no dependency; lockfile
unchanged. Diff and secret checks pass. No Go/SQL/API/UI behavior or staging
change. EVC-02B canonical model/eligibility and EVC-02C real normalization remain.

## EVC-02B opening

Execution bounds: EVC-02B1 common minimized site model + pure OCM reader/context;
EVC-02B2 OSM site/equipment reader and source lineage. Both precede EVC-02C.

B-BR-EV-NORM-02B: Each accepted source site preserves its own source identity,
revision/record hash/object version when present, source clocks, geometry,
license/provider attribution and connector groups. No cross-source merge or
quantity addition occurs here. A missing name/address remains absent. Private
OCM usage 2/3/6 and removed 200/210 or planned 150 records are excluded; public
1/4/5/7 retains its access condition. Unknown codes remain explicitly unknown.
OSM private/no and lifecycle prefixes/construction are excluded. Source status
is dated static metadata, never current availability. Unsupported connector,
power and quantity fields remain unknown with explicit issue counts. Conflicting
duplicate connection IDs are all excluded, independent of input order. Contacts,
comments, photographs and arbitrary free text do not enter the common model.

BUC-EVC-02B1: Verify the supplied pinned reference hash and all three provider
license strings before constructing OCM context. Decode numeric coordinate and
power tokens only at their actual field paths; validate finite WGS84 bounds and
exact power. Reject malformed/foreign/unlicensed/private/deleted/planned sites
without rejected coordinates in the decision stream. Normalize each accepted
site to the shared typed model, preserving source group units and explicit
field issues. Synthetic privacy, identity, access, status, numeric, duplicate,
ordering and changed-rights tests precede behavior. No raw-source dump or DB use.

### EVC-02B1 checkpoint

Status: LOCAL_DONE
Validation: PASS

Tested parent d610ca8d plus this child. The common source-site DTO carries source
identity/revision/record hash/clocks/rights, point provenance, nullable source
name/operator/address/access/fee/status and distinct connector groups. OCM
context verifies reference SHA and the three exact provider licenses/hashes
against the new source admission; duplicate provider/operator identities refuse.
The bounded reader permits 1 MiB/record, 8 MiB/reference, 2,000 groups/record;
no national source limits were raised. Names/addresses remain absent when absent.

Seven synthetic OCM test groups pass, plus socket/exact-power and legacy reader
regressions (12 focused tests total). RED exposed the missing reader, nullable
Connections handling and invalid source clocks; GREEN preserves null connectors
as unknown, excludes private/foreign/unlicensed/removed/planned/bad-geometry
records without rejected coordinates, strips contacts/comments/media, rejects
duplicate numeric fields, omits every conflicting duplicate group ID regardless
of input order, and retains exact per-path coordinate/power lexemes. Public
membership/notice/payment conditions remain explicit; no fee is guessed for
ordinary public usage. Static source status never claims live availability.
Source clock syntax/calendar is validated without assigning a missing timezone;
invalid/free-text clocks are omitted with issues. Unknown/invalid socket/power/
quantity fields have explicit reasons; groups are never summed across sources.

Fmt and clippy all targets with warnings denied pass; diff and secret checks
pass. No new dependency, Go/SQL/API/UI change, real normalization run or staging
action. Next EVC-02B2 adds OSM normalization into this same model, then EVC-02C
binds both source inputs and executes the private real normalization census.

### EVC-02B2 opening

B-BR-EV-NORM-02B2: OSM nodes/ways/relations preserve identity, actual object
version/timestamp and geometry provenance. Missing version/geometry stays
unknown or quarantined, never fabricated. `man_made=charge_point` is equipment,
not another site; only explicit source membership can supply parent keys here.
Fuel objects qualify as combined only with an explicit charging indication or a
positively reported known EV socket. Ordinary fuel rows are ignored as non-EV.
Private/no, deleted, disused/abandoned and construction objects are excluded.
Socket `yes` means a reported family with unknown quantity, never one connector;
`no`/zero does not prove a family. Unknown socket names stay UNKNOWN, and multiple
power alternatives are not reduced to a guessed maximum. OSM contact tags and
arbitrary tags never enter the common model; a digest of the decoded evidence
plus the pinned PBF identity retains auditability without contributor metadata.

BUC-EVC-02B2: Normalize synthetic site/equipment/fuel/lifecycle objects using the
same model as OCM, typed source units, stable sorted groups, explicit field issues
and source-only address components. Validate parent source identities without
proximity guesses or linking across sources. Source reader/CLI will construct
these evidence records from the admitted PBF in EVC-02C; no live source calls.

### EVC-02B2 checkpoint

Status: LOCAL_DONE
Validation: PASS

Tested parent 62366afb plus this child. OSM normalization now produces the same
common source model, with an enum distinguishing CHARGING_SITE, COMBINED_SITE
and CHARGE_POINT. Ordinary fuel evidence is ignored. Explicit electric-fuel or
positive known EV socket evidence identifies combined source objects; generic
domestic/no/unknown-presence tags do not fabricate an EV site. Lifecycle-only
site/equipment candidates remain visible to excluded-reason accounting.

Six synthetic OSM groups pass, along with all seven OCM groups after the final
filter/enum refactor (13 focused tests). RED exposed missing normalization,
inconsistent geometry provenance, parent-order sensitivity and uncounted
inactive equipment; GREEN validates source type/identity/version/timestamp,
finite geometry/provenance, explicit source parent identities, stable parent
sets/digests and sorted groups. Contact tags are discarded. A reported socket
`yes` retains null quantity; no/zero is absent; alternative powers remain null
with an issue. Names, brands, address components, fee and hours are source-only.
Missing name/address never becomes a coordinate or an invented label.

The record digest hashes canonical decoded evidence (sorted tags/parent set),
not a claimed raw PBF blob. Lineage revision includes the exact PBF SHA-256;
actual source versions/clocks and centroid/node provenance remain explicit.
The full Rust fast gate passed; after the final bounded candidate/enum changes,
affected OSM/OCM tests and all-target clippy with denied warnings pass. Fmt,
diff and secret checks pass. No dependency/SQL/API/UI change, real normalization
execution, import/backfill or staging action. Next: EVC-02C artifact contract,
bounded file preparation and real source reconciliation before EVC-03.

## EVC-02C opening

B-BR-EV-NORM-02C: A private normalized generation binds the frozen source profile,
PBF/IBGE bytes, complete OCM inventory and each POI/reference hash. No partial or
foreign input replaces a prior generation. Reuse the frozen national OSM limits
and resource fences; no silent cap raise or truncation. Scan/select EV site and
equipment evidence, preserve actual versions/timestamps and explicit structural
parent membership, resolve geometry with the bounded PBF index, then normalize.
Rejected output holds source identities/reasons only, never rejected coordinates
or contact data. Counts remain per source, UF and typed unit; equipment is not
counted as a site and sources are not summed before conflation. The output is a
private preparation format, not the public import/publication contract.

BUC-EVC-02C: Before real national execution, prove deterministic synthetic PBF +
OCM + boundary preparation, explicit child membership, privacy and complete
inventory/hash admission; test changed/missing/unexpected input, cancellation,
resource caps and existing-output preservation. Write owner-only hash-bound
streams/manifest to a new directory, with measured execution receipts outside
deterministic artifacts. Run only on already authorized local files, then record
actual source/UF counts and repeated output hashes. No DB, network or staging.

### Real-input unit audit before acceptance

The first real prototype reconciled all 1,775 source decisions (1,298 OCM +
477 OSM): 1,655 source sites, 74 equipment records, 46 exclusions. It is not the
accepted final normalization: cross-checking the EVC-01 charging keys found ten
objects carrying both amenity=charging_station and man_made=charge_point, all
without explicit parent membership. Preserve both explicit markers, and retain
standalone site evidence unless a source parent actually makes it equipment.
An unparented equipment-only object still does not become an invented site.
Record available closed-way footprints for EVC-03 containment/parent review;
unsupported/incomplete footprints remain absent, never synthetically closed.
This is a data-audit correction within the unaccepted EVC-02C prototype. Preserve
the prototype outputs privately; only the corrected tested replay will be final.

## EVC-02C accepted local checkpoint

Status: LOCAL_DONE
Validation: PASS
Integration: INTEGRATION_PENDING

Tested parent ccc52734 plus this child. The new private
[source format](../../../../contracts/ev/source-site-format-v1.md) and frozen
[normalization profile](../../../../contracts/ev/normalization-profile-v1.json)
bind both source pins, raw file inventory/hashes, resource profile and typed
records. The native CLI refuses cap/profile overrides and production/staging
contexts. The owner-only resource wrapper checks exact profile/format/required
file hashes, retains private child logs and records executable SHA separately.
PBF is hashed before and after processing; OCM directory coverage and each
file/reference hash are exact. Cancellation during output removes only that
unpublished generation; prior output is never replaced.

RED covered absent preparation, changed/missing/unexpected sources, integer
inventory-total overflow, cancellation during output and mixed source roles.
GREEN: full Rust fast gate passes (fmt, clippy all targets, locked suite),
including six file/CLI failure groups, seven OCM groups, seven OSM groups and
16 offline Python groups. Synthetic closed-way footprint/explicit-child output
is reproducible and private. No dependency added or lockfile changed. Diff and
secret checks pass. No Go/SQL/API/UI operation, import/backfill or staging change.

Corrected real preparation processed 363,988,420 PBF elements and all 1,298
OCM country files. [Independent reconciliation](EVC-02-reconciliation.json):

- OCM: 1,259 accepted source sites; 37 private and two planned excluded.
- OSM: 400 charging + six combined source sites, 64 equipment-only objects;
  seven private excluded. All ten dual-marked explicit sites are preserved.
- Total: 1,775 source decisions exactly reconcile; 1,665 source-site assertions,
  64 separate equipment records and 46 exclusions. These are source assertions
  before conflation, not 1,665 unique physical locations or complete coverage.
- All 44 available closed-way footprints are preserved. No accepted relation
  objects occur in these pins; fragmented relation footprints are unsupported
  and remain absent, not fabricated. Equipment parent containment/review is
  EVC-03 work; unparented equipment does not silently create a public site.
- All original OCM inventory IDs match decisions, artifact hashes verify, and
  contact/comment/media field scan passes. Raw rows remain outside Git.

Two corrected real executions are byte-identical for all five artifacts;
[hashes](EVC-02-reproducibility.json), manifest SHA-256
f7b43d21a294d2a7552ac3b8abb2589ca246dd0ff581e4a494db935fea07f560.
The accepted [replay receipt](EVC-02-execution.json) measures 70.747 s, peak RSS
370,352,128 bytes and owned working disk 2,111,670,020 bytes, within frozen
budgets. [Per-source/UF census](EVC-02-uf-census.json) includes all 27 UFs and
UNASSIGNED, with equipment explicitly separate. No cross-source quantity sum.

Next: EVC-03 deterministic conflation, parent containment/aggregation and actual
matched/single/probable/conflict counts. The owner imagery spot check and later
public rights/import/staging gates remain owed; this checkpoint does not claim
those acceptances.

## EVC-02C1 country-consistency correction opening

B-BR-EV-NORM-02C1: A declared OCM Brazil country does not override a point outside
the whole admitted IBGE country extent. Such a source assertion is quarantined,
never coordinate-swapped/repaired or assigned a nearest UF. Points inside that
extent without a unique municipality polygon keep UNASSIGNED status for review.
The already-committed v1 profile/artifacts remain historical; v2 adds this guard
and supersedes v1 for dependent conflation. No new source download or DB action.

BUC-EVC-02C1: Derive country extent from the pinned municipality geometry, test
an otherwise valid BR-labelled source point outside the extent, retain all
inventory decisions/reasons, and regenerate/reconcile two private real runs
under a new immutable profile hash. The independent real audit identified OCM
311700, provider 1; no rejected coordinates or personal source text enters Git.

### EVC-02C1 accepted successor checkpoint

Status: LOCAL_DONE
Validation: PASS
Integration: INTEGRATION_PENDING

Tested parent 886e8eae plus this child. RED proved an otherwise valid BR-declared
point outside the admitted boundary extent was accepted. V2 derives a cached
country extent from the pinned polygons and quarantines those points before
record/census publication. Existing v1 profile and receipts remain unchanged;
new dependent work must bind normalization-profile-v2 / ev-normalization-v2.
The stream format and input source pins remain compatible. Full Rust gate and
16 offline Python groups pass; no dependency, SQL/API/staging/import change.

Real [independent reconciliation](EVC-02-v2-reconciliation.json) removed two
out-of-extent source assertions: OCM 311700 and OSM node 11524381344. It repaired
neither coordinate. OCM now has 1,258 source sites; OSM 399 charging + six combined
source sites, 64 separate equipment records. All 1,775 decisions reconcile:
1,663 source-site assertions + 64 equipment + 48 exclusions (44 private, two
planned, two outside extent). The full OCM file-ID inventory still matches all
decisions. All accepted points are inside the independently calculated IBGE
extent; eleven OCM sites still lack a unique UF polygon match and remain
UNASSIGNED, not nearest-state guesses. [Per-source UF census](EVC-02-v2-uf-census.json).

Both corrected v2 real runs are [byte-identical](EVC-02-v2-reproducibility.json)
for all five artifacts, manifest SHA-256
61a0d64aaf2eda85dae247b3948a9d0df49a49baa1c019a83bab2bca1af4877b.
Accepted [replay](EVC-02-v2-execution.json): 118.780 s, peak RSS 370,384,896 bytes,
working disk 2,111,667,977 bytes, within unchanged frozen resource limits. Native
binary SHA and both profile identities are retained. Diff/secret review pass.
No national raw rows/coordinates were committed. Continue EVC-03A2 from this
successor, not the historical v1 normalization artifacts.

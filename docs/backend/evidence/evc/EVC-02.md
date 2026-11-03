# EVC-02 — Canonical source normalization

Status: IN_PROGRESS

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

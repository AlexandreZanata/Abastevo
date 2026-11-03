# Private EV source-site preparation v1

Format: `abastevo.ev-source-sites-v1`. Accepted parser: `ev-normalization-v2`.
Behavior profile: `normalization-profile-v2.json` (SHA-256 bound in the manifest).
The original v1 parser/profile remains historical; see the successor section.
Exposure: PRIVATE_PREPARATION. This is the EVC-02 input to EVC-03 conflation,
not a public import/publication format and not proof of complete Brazil coverage.

## Binding and limits

The manifest binds the exact source profile ID/SHA, PBF revision/bytes/SHA,
complete OCM inventory SHA/count, each OCM file byte count/SHA, reference SHA and
IBGE SHA. Production CLI uses only `national-source-pins-20261103-v1.json` and
`osm-national-limits-v1.json`; runtime source/cap overrides are refused. The pure
file adapter accepts explicit synthetic pins/limits for offline integration tests;
synthetic profile IDs are not public admission authority.

OCM limits: 5,000 country files, 1 MiB/file, 128 MiB total, 8 MiB reference and
inventory; native record normalization caps 2,000 groups per record. OSM retains
the frozen 3 GiB/400M elements/200K candidates/64 tags/256-byte tags and the same
wall/CPU/memory/file/disk fences. Artifact records cap at 1 MiB, files at 512 MiB.
No source truncation or silent budget increase is permitted.

## Deterministic private files

- `sites.ndjson`: one minimized source object per line, sorted by qualified key.
- `decisions.ndjson`: one source identity/state/reason per line, sorted by key;
  rejected records contain no point/address/contact payload.
- `issues.ndjson`: field/group issues sorted by key/reason, never raw free text.
- `uf-census.json`: 27 UFs plus UNASSIGNED, separated by OSM/OCM and typed unit.
- `manifest.json`: format/parser/profile/input identities, limits, summary,
  quarantine reasons and byte/SHA descriptors for the four files above.

JSON is UTF-8 without wall-clock measurements in deterministic files. NDJSON
records end in LF. The manifest is written last. Output must be a new directory;
0700 directories/0600 files. Cancellation/failure removes only this unpublished
attempt. Existing output is never replaced. The approved execution path is the
local Python resource wrapper, which keeps measured receipts and child logs
outside the deterministic generation and refuses production/staging contexts.

## Common source record

Qualified keys are `osm:{node|way|relation}:{id}` and `ocm:poi:{id}`. Kind is the
closed enum CHARGING_SITE, COMBINED_SITE or CHARGE_POINT. Equipment stays separate
from source sites until EVC-03 applies explicit parent membership. An unparented
charge point must not silently become another independently counted site.

Both explicit site/equipment markers are preserved. An unparented dual-marked
object retains explicit site evidence; equipment-only objects do not become
invented sites. Available closed-way footprints preserve source rings for
EVC-03 parent containment/review; incomplete/fragmented footprints remain absent.

Each record preserves source namespace/external ID, revision, record digest,
actual object version when provided, bounded validated source clocks, provider,
license and attribution. OCM digest is the original JSON bytes. OSM digest is
canonical decoded evidence (sorted tags/parent set); its revision also binds the
PBF SHA, so it is never mislabeled as a raw PBF blob hash. PBF contributor metadata
is not part of the decoded evidence. The point carries decimal strings and an
explicit node/way-centroid/relation-centroid provenance; it is never an address.

Name/operator/network/brand, structured address components, access, fee and hours
are nullable source fields. Status is static reported metadata, never live
availability. Groups preserve a qualified source ID/raw socket, frozen family,
exact integer watts or null, reported quantity or null, and reported current/
status. Source groups are not summed. Missing values, ambiguous socket names and
unsupported fractional watts stay unknown. The tested socket mapping is
`socket-families-v1.json`; UNKNOWN never establishes shared-family confirmation.

Public publication additionally needs EVC-03 conflation, EVC-05 rights/source
offer, EVC-06 guarded atomic import and the later staging acceptance gates.

## Accepted parser successor

`ev-normalization-v2`, bound to `normalization-profile-v2.json`, supersedes the
v1 parser for dependent work after the EVC-02C1 real-data audit. Stream format
v1 and source pins remain unchanged. V2 quarantines points outside the whole
pinned IBGE country extent despite a declared Brazil country. Polygon misses
inside that extent remain UNASSIGNED; no coordinate repair/nearest UF. The v1
profile and historical artifacts are retained unchanged and are not accepted
inputs for the subsequent conflation generation.

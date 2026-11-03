# EVC-07 Android national charging catalog

## EVC-07A public data contract

B-BR-EVC-07A. Preserve an absent name and raw connector code when the
national API supplies no value. Legacy places and connector groups still
require their historical fields. Preserve the national source key on each
connector group and the exact supported connector family. A two-source
claim requires both OSM and OCM attribution. Expose the linked station ID
only when present, with its API identity format. Preserve the national
API coordinate precision of up to 16 decimal places.

BUC-EVC-07A. Decode a synthetic two-source location with no name, distinct
source-owned connector groups, and an optional linked station ID. Refuse
inconsistent confidence and malformed source ownership. Exercise legacy
decoding and connector handling alongside the new transport contract.

The synthetic regression initially failed while decoding an empty name.
After the national shape was admitted, the expanded test also reproduced
the legacy precision limit. The pinned replay contains 276 unnamed sites
and 631 with a coordinate beyond 12 decimal places. The largest location
has three source assertions or twelve connector groups, within the existing
client bounds. Legacy name and coordinate guards remain in force.

The five targeted domain tests and eighteen snapshot data tests passed.
The national profile stays gated until offline storage can round-trip the
new fields. Its aliases can overlap the legacy profile, so storage must
scope row identity and accounting to the profile before enabling refresh.

Status: LOCAL_DONE / INTEGRATION_PENDING. Android debug Kotlin
compilation, whitespace and secret-surface checks also passed.

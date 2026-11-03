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

## EVC-07B offline snapshot storage

B-BR-EVC-07B. The offline place identity includes profile, generation and
public ID. A write, retirement or size check is scoped to one profile.
Migration preserves old rows and attribution. A national pack retains
confidence, links, exact coordinates, absent names and source-owned
connector groups through the storage mapping.

BUC-EVC-07B. Upgrade a Room database with a legacy place and write a
national row with the same ID and generation. Retire only the national
row and check both profiles. Round-trip a national pack with synthetic
facts and reject a place with the wrong profile shape.

The RED test reproduced a national row accepted by a legacy pack. The
round-trip regression now passes with a profile check on import and read.
Room 35 rebuilds the charging place table with a profile-scoped key and
copies all Room 34 fields, including source attribution. The schema diff
shows one changed table. The primary key also serves profile/generation
reads. Tombstones and budget checks use the same profile scope.

Twenty-three snapshot and discovery repository tests passed. The
33-to-34 and 34-to-35 instrumented tests passed on the Android emulator.
The new test opens the migrated database with Room, writes the same
identity in both profiles and verifies scoped retirement and counts.
Repository refresh still gates national packs pending the revision-bound
payload and rights checks.

Status: LOCAL_DONE / INTEGRATION_PENDING. App debug unit-test Kotlin
compilation, whitespace and secret-surface checks passed.

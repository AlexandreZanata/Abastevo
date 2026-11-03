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

## EVC-07C national page payload

B-BR-EVC-07C. Each national page includes that page's source-owned
connector groups, as observed in the same database snapshot as the
places and cursor. Emit an empty groups array for a national place
without groups. Publish only admitted operator text and allowlisted
address components. Legacy page shape remains compatible.

BUC-EVC-07C. In real PostGIS, read two pages with a sourced group
on the first row and no groups on the second. Confirm the group, address,
and operator agree with detail. A cursor from a replaced generation
still fails, and revision filtering cannot mix the generations.

The RED PostGIS fixture received a national page with the group absent.
The national reader now loads groups for the selected physical IDs in
one query inside the same repeatable-read transaction. Public list and
detail agree on each source key, power, operator and address. The test
also verifies an empty array, strips an unknown address field and
checks that an obsolete revision and cursor cannot read the new batch.
Legacy pages still omit groups.

Status: LOCAL_DONE / INTEGRATION_PENDING. Charging HTTP integration with
race detection, vet, staticcheck, OpenAPI lint with zero errors,
whitespace and secret review passed.

## EVC-07D national offline refresh

B-BR-EVC-07D. A national refresh uses only its active coverage revision,
same-origin revision-qualified offer paths and source-owned page groups.
Require exact place and group counts and recheck the active revision before
commit. An interrupted or stale refresh preserves the previous cache;
compare its base inside the Room transaction.

BUC-EVC-07D. Refresh a synthetic two-page generation, then refuse a
short generation, foreign offer URL and a revision change during refresh.
The original groups and cached generation remain available offline.

RED failed at the national profile gate. GREEN verifies exact counts
across the two-page fixture, keeps source-owned groups offline and
refuses a missing page, group count mismatch, foreign offer path and a
revision change. The codec also refuses missing national page groups and
fractional counts. An obsolete writer cannot overwrite the committed
generation. Offer paths are verified against the active revision; the
backend's publication gate serves the pinned artifacts.

Twenty-five focused Android data tests and three domain tests passed.
UI activation, the localized detail presentation and map/station
integration remain part of the following app tasks.

Status: LOCAL_DONE / INTEGRATION_PENDING. App debug Kotlin compilation,
whitespace and secret-surface checks passed.

## EVC-07E offline filter windows

B-BR-EVC-07E. A connector, current and minimum power filter must
match one reported connector group. Unknown power cannot satisfy a
minimum. Filter a complete local generation without stopping at an
empty raw window; continuation resumes at the next matching row.

BUC-EVC-07E. Refuse a site where the requested family and current
belong to distinct groups. Traverse a local snapshot where early
database rows do not qualify and check stable continuation and
power thresholds.

RED reproduced a false match across a Type 2 AC group and a CCS2 DC
group. The filter now evaluates every requested connector fact on the
same group. A cache test skips 101 nonmatching rows before returning the
first hit, then resumes at the second without duplication.

Explore prefers a cached national generation. Initial or manual network
refresh tries the national profile, with legacy bootstrap only when the
national request is unavailable and national data has not been selected.
The screen reuses the eight-locale minimum-power label and offers 22,
50 and 150 kW thresholds. Filter changes reset the local continuation.

Status: LOCAL_DONE / INTEGRATION_PENDING. Fourteen domain, repository
and ViewModel tests, app compilation, whitespace and secret review passed.
Map pins and national detail presentation remain open.

## EVC-07F national charging detail

B-BR-EVC-07F. Preserve the source operator and the shared, coordinate-safe
address line offline. A nameless location uses a localized presentation
label only. A two-source badge is shown only for confirmed matches.
Display each connector group's own power and provider attribution without
rounding or combining groups. Retain the existing community report route.

BUC-EVC-07F. Decode and persist synthetic operator/address facts, reject
coordinate text as an address, and show offline details with an unnamed
place, two sources and distinct powers. Legacy rows retain their fields.

RED reproduced omitted operator and address fields. The codec now
admits only supported address components and uses the SNA formatter.
Room 36 stores the resulting address line and operator as nullable
fields. Existing caches keep their rows and gain neither value until
the next refresh. The offline mapping round-trips both fields.

The detail shows exact source-attributed powers, the admitted operator
and address, and a localized fallback label when no name is available.
A two-source match shows the confirmation badge. The existing community
route receives the same public place ID. The four UI labels are present
in all eight locales.

Twenty-one snapshot unit tests, two Room migration tests and three
Compose detail tests passed. The Compose tests exercised both themes
and verified distinct reported powers and the report action.

Status: LOCAL_DONE / INTEGRATION_PENDING. Light and dark emulator
captures were exported after the dialog animation and visually checked.
Both use synthetic data. App/test APK builds, whitespace and
secret-surface checks passed.

## EVC-07G national map pins

B-BR-EVC-07G. Prefer the saved national map catalog and retain legacy
fallback when it is absent. National filters use one source group with
known minimum power. Coordinates determine pin eligibility only. Map
selection carries the two-source badge without implying manual review,
and missing coordinates remain list-only.

BUC-EVC-07G. With only the national cache available, render the
confirmed pin, keep the single-source pin unbadged and omit an unknown
power site under a minimum-power filter. Preserve legacy unknown-power
behavior and verify the selected pin's address and confidence.

RED reproduced the map ignoring a national-only cache. The map now uses
the national profile first and follows the cached row continuation.
The two-source fact reaches the selected marker, native label and
accessible list. The source-location label and unnamed fallback are
localized in eight languages. The national filter rejects unknown
power; the legacy map policy still keeps unknown power visible.

Status: LOCAL_DONE / INTEGRATION_PENDING. Twenty-nine map ViewModel
and fourteen native KML tests passed. The tests include the national-only
cache, strict minimum power, no map refresh request, per-pin confirmation
and legacy pagination. App compilation, whitespace and secret review passed.

## EVC-07H chargers linked to fuel stations

B-BR-EVC-07H. Fuel station detail reads only active national cache
rows carrying its exact canonical station ID. A canonical route can
read those public links offline. A legacy CNPJ must resolve to a
matching directory identity first. No proximity inference is made.

BUC-EVC-07H. Read only the station's linked rows, omit other and
retired rows, and open the same offline charging detail with its
attribution and report action. An unresolved legacy CNPJ reads no links.

RED reproduced the missing station cache lookup. The Room query joins
the complete national generation with its exact station ID in one read.
A SQLite replay of the generated schema and query excluded a different
station, a previous generation, a tombstone and an incomplete pack.
Both fuel station routes open the shared offline charger dialog and
preserve the correct community route for the selected charger.

The seven charging repository and eleven station ViewModel tests passed.
The ViewModel tests covered canonical offline links and unresolved legacy
CNPJs. The linked-site section label is present in all eight locales.

Status: LOCAL_DONE / INTEGRATION_PENDING. Four Compose tests passed
in a combined rerun, including selection and the reported charger ID.
The first device run was interrupted during emulator memory pressure.
App/test builds, whitespace and secret-surface checks passed.

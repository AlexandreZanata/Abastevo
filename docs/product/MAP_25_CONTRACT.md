# MAP-25 implementation contract

Scope: private favorites, saved trips, safe KML/GPX/KMZ exchange and an optional
explicit local track recorder. MAP-26+ and any new server dataset are outside this
phase. Written 2026-10-26 before behavior.

B-BR-MAP25-01 (ownership): saved places and trips belong to exactly one owner
(`guest` or an account id) and one API origin, like the ledger drafts. Every read,
write, delete, export and import is filtered by that owner; another account or the
guest never sees, exports, overwrites or deletes the record. Logout hides nothing
destructively and deletes nothing; signing in as another account shows only that
owner's rows. A file or envelope never carries authority to choose an owner: imports
always land in the current owner, and a record that names another owner is not
honoured. No automatic cross-account or guest-to-account restore exists.

B-BR-MAP25-02 (data): a station favorite keeps the station UUID (compatible with the
existing cloud-sync `favorites` dataset `{station_id}`), name, position, source kind
(ANP, COMMUNITY, MANUAL, IMPORT) and the original source id. A trip is a ROUTE_PLAN
(ordered origin/stops/destination plus avoid options, never the computed geometry), a
RECORDED_TRACK or an IMPORTED_TRACK. Coordinates are exact micro-degrees; invalid,
non-finite, out-of-range or over-precise (>9 decimals) input is refused, never
clamped. Bounds: 5,000 places, 500 trips, 10,000 points per track, 102 route-plan
points, 120-character names, 500-character notes. Quotas refuse; they never evict.
No home/work classification and no server sync is created: this phase does not claim
cloud backup. The cloud-sync client for `favorites` is not wired in the app yet, so
cross-device restore, stale-restore and signed private backup are NOT delivered here.

B-BR-MAP25-03 (privacy): everything is local. Nothing is uploaded, logged, sent to
analytics or placed in the SDK bookmark store. Export is an explicit user action
through the system file chooser and warns that files can reveal home addresses and
routes. Deleting a record removes it and its points; a tombstone of the record id only
(no name, position or note) prevents a later import from silently resurrecting it.
Re-importing a tombstoned id requires an explicit second confirmation.

B-BR-MAP25-04 (safe exchange): GPX 1.1 and KML 2.2 are read by an in-repo strict
subset reader (no dependency is admitted). DOCTYPE, ENTITY, processing instructions
other than the leading XML declaration, non-UTF-8 encodings, user-defined entities,
external references, nesting deeper than 64 levels, more than 200,000 elements,
oversized files (>5 MiB) and unknown top-level formats are refused before any record
is created. KMZ is read in memory from a zip: at most 64 entries and 20 MiB expanded,
no nested archives, no absolute/`..`/backslash/drive names, only a root `.kml`
document is used, nothing is written to disk. Import is all-or-nothing per file and
idempotent by canonical content hash (re-importing the same file creates nothing).
Original source ids are kept in `abv:` extensions; exported files carry a license
notice for public data (ANP open data; community contributions under the Abastevo
terms; OpenStreetMap map data attribution when a route plan is exported).

B-BR-MAP25-05 (recording): a track is recorded only after an explicit start with
precise foreground location, a visible notification and an explicit stop. Starting
guidance never starts recording and recording never starts at boot or from a map
view. Fixes follow the guidance quality policy (finite, accuracy <= 50 m, fresh).
Points are appended to an owner-scoped local row; stopping saves a RECORDED_TRACK,
discarding deletes it. Process death never restarts recording; an interrupted
recording is offered for explicit save or discard.

BUC-MAP25: save a station -> restart -> appears only for its owner -> logout/other
account/guest isolation -> export GPX/KML -> malicious file refused with no row -> import
valid file once (second time no-op) -> delete -> same file cannot resurrect without
confirmation -> record a short track with explicit start/stop -> export it.

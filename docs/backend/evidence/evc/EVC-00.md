# EVC-00 — Runtime inventory and source-register reconciliation

Status: LOCAL_DONE
Validation: PASS

Tested parent a25f1832 plus this documentation-only inventory. Read the station/
EV execution plan, EE source/contract/backend flow, EE-04, MAP-EV register/T07
and EVS-06..09. No product behavior, migration or source acquisition changed.
The owner's explicit read-only staging authorization covers this inventory.

## Measured staging baseline

At 2026-11-03T12:40:23Z the charging catalog tables contain zero batches,
zero places/groups and zero active reviewed/unresolved/rejected station links.
Directory has 48,770 rows: 45,767 with a UF and 3,003 without one; 40,388 have a
point. These are database station-row units, not a certified ANP or EV census.
[Per-UF baseline](../sna/staging-database-baseline-2026-11-03.json).

At 2026-11-03T13:45:55Z the active map generation is national-20261027,
profile national-open-map-v1: input/accepted 66/66, no duplicates/quarantine.
All 66 active assertions are OSM FUEL_STATION. There are zero active
CHARGING_SITE or COMBINED_SITE overlay assertions and no linked-UF EV rows.
The preceding 565-row generation is SUPERSEDED. Generation naming does not
establish national coverage. [Sanitized aggregate receipt](EVC-00-staging-overlays.json).
Native MWM embedded POIs were not counted; these facts describe the separately
stored charging catalog/map overlay, not every object embedded in a native map.

The signed automatic image update to d0eb9c79 was independently verified in
[ZD-08](../cd/ZD-08.md). The last verified image receipt reports schema 145; SNA migrations 146..148,
new campaign and public API have not been deployed or exercised there.

## Register reconciliation and concrete holds

EE-04 records owner source admission on 2026-10-22 for OCM commit
8e3bedca48ca94807d96b2f5e7ee02cffe63ae54, not a generic approval of every future
snapshot. Its measured local preparation used 1,298 POIs and 2,089 connection
groups (different units) from providers 1/40/57. The map register's old
no-subset/PENDING row was stale and now links that scoped ADMITTED_FOR_SOURCE
record. This does not imply live activation, fresh coverage or API access.

A review of contracts/ev/preparation-profiles-v1.json found provider 57's
provider_license_sha256 is 47 hexadecimal characters, unlike a SHA-256's 64.
The root reference-data digest and other two provider digests have 64. Do not
invent the missing digest or silently rewrite old artifact identities. EVC-01
must independently verify the pinned reference bytes/license text and record
correct rights metadata in a versioned profile before fresh admission.

T07 records a regional MWM Derived Database and a separate Abastevo overlay.
It does not pin a national Geofabrik input or waive obligations for the new
conflated OSM/OCM artifact. SRC-OSM remains PENDING for that national artifact;
EVC-01 owns approved acquisition, exact pins and measured streaming budgets.
No public Overpass, OCM or Nominatim production queries are authorized.

## Publisher documentation checked on 2026-11-03

- [OSM copyright](https://www.openstreetmap.org/copyright): retain attribution
  and applicable ODbL derived-database/share-alike duties for OSM-derived data.
- [Geofabrik download policy](https://www.geofabrik.de/data/download.html):
  public extracts omit personal contributor metadata and retain ODbL duties.
- [OCM terms](https://www.openchargemap.org/about/terms): contributor CC BY 4.0
  does not cover every imported provider; visible provider/license credit is
  required. Provider rights must be inspected separately.
- [OCM export README](https://github.com/openchargemap/ocm-export): the export
  is experimental and country-selective; reference data carries provider rights.
  Export/API are two transports of OCM, not two independent confirmations.

These are source-policy observations, not blanket legal certification or new
admission. MIT software notices and data licenses remain separate.

## Checks and next action

Verified the read-only SQL transaction, sanitized counts/no personal data,
EE-04 commit/profile/provider correspondence and exact digest lengths. Reviewed
changed links, source-register scope, diff-check and secret surface. No runtime
suite is warranted for this documentation-only task. Next: EVC-01 national
source pins and bounded extraction, with the owner's required approval before
national downloads, any OCM key acquisition or live import/staging step.

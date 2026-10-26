# Abastevo map acceptance runbook (staging-scoped)

Scope: `https://teste.abastevo.com.br` only after separate staging authorization and access.
Production/G09, store publication, public pilot, DNS/tags, payments, enterprise portal and
ANP/PMQC publication are explicitly out of scope. Use the existing operator credential
mechanism; never print credentials, tokens, keystores or private GPS.

## Frozen candidate manifest

- App release candidate: `app-release-unsigned.apk`, 78,396,538 bytes (74.76 MiB), SHA-256
  `feadd9f34d60c26b57a2025e96a1b799c04b52c90da9d47c9784a430363536e0`; arm64-v8a + armeabi-v7a,
  minSdk 26, every packaged native LOAD `0x4000`, `zipalign -P 16` PASS. Signed test install
  must be produced from this artifact; unsigned is not a published release.
- Debug/device candidate: `app-debug.apk` SHA-256
  `18f283c6ebbc62ad69e7b0e8408f533e49eaede5db737050d56621d6a0cfd915`; test APK
  `bc04e1ed3ad5cf7d729434f9f7b5928e27121b8177085802615eb9f7d9bb338b`.
- Pinned engine candidate `7ad828883c4aca8a`: `classes.jar`
  `071120e96b1c27be14d166183dd96b37751119fc4da45c74cb76a3647e0ee66a`, arm64 core
  `bb2aed7b…`, armv7 core `7d61b573…`; patches `ndk-r28-lld.patch`,
  `private-diagnostics.patch`, `explicit-guidance-voice.patch`; source offer, notices and
  SBOM are in `maps/engine/vendor/`.
- Admitted regional fixtures (private runtime, not shipped): `Brazil_Parana_East.mwm`
  `d1e9b2ce81cc10335b6b082a64d78a8241390f42c262e2832f400c981b200a3e`, `World.mwm`
  `77a35c16…`, `WorldCoasts.mwm` `d42a71b1…`. Other generations do not become compatible by
  filename.
- Database: latest local migration 132; local disposable restore rehearsal reported 132
  migrations / 163 public tables on both source and restore. Migrations are append-only; do
  not demonstrate rollback with a destructive down migration.

## Preconditions

1. Confirm the current staging SHA/image, schema version, config/defaults, active map package
   generation, engine AAR and database size; compare with the manifest above.
2. Confirm a tested backup exists and the restore target is known. Do not use a production
   database, real contributor content or an unsanctioned load campaign.
3. Use synthetic records for failure cases. Fuel/charging official data may be used only when
   already admitted for staging; never silently substitute ANP with community data.
4. Verify the app feature gate/rollback switch and the map last-good generation before deploy.
5. Check staging service coexistence: payments, security, worker queues and enterprise/static
   pages must not be interrupted by the map update.

## Local rehearsal already performed

On the disposable synthetic PostGIS database `anpfuel_map2831` (not staging):

```sh
pg_dump -U <synthetic-user> -d anpfuel_map2831 -Fc -f /tmp/map.dump
createdb -U <synthetic-user> anpfuel_map2831_restore
pg_restore -U <synthetic-user> -d anpfuel_map2831_restore /tmp/map.dump
# both databases: 132 migrations, 163 public tables
dropdb -U <synthetic-user> anpfuel_map2831_restore
```

This proves the local restore shape only; it is not a staging backup/restore.

## Read-only staging status at batch time

The origin responded to a read-only request, but `/`, `/health`, `/healthz`, `/v1/health` and
`/api/health` all returned HTTP 404. No authenticated operator session, DVCS/IP access,
database access or deploy credential was available in this environment, so no staging change
or staging PASS is claimed. Status: `STAGING_PENDING`.

Exact retake condition: authorized operator access (existing mechanism), a current-version
backup, explicit staging window, current SHA/schema/config inspection, and acceptance of the
rollback target. Without all of them, only this local runbook/candidate applies.

## Apply sequence (staging, when authorized)

1. Record current app/API SHA, DB migration head, active map generation and service health.
2. Take/verify a backup and a restore target; record size/hash only, never credentials.
3. Publish the exact candidate image/artifacts through the normal mechanism; do not reuse
   PR #150 images or an older AAR hash.
4. Apply append-only migrations; if migration fails, stop and restore the verified backup
   rather than attempting a destructive down.
5. Activate the admitted map generation/overlay atomically; keep the previous last-good
   generation and a rollback lease. Do not delete a generation in use by a reader/navigation.
6. Run the smoke matrix below; record every result in the manual sheet.
7. On any mandatory failure, roll back the app/map generation and restore the DB backup; keep
   the failed evidence.

## Smoke matrix

| Case | Required result |
|---|---|
| Fuel / electric / combined | Correct pins/services and exact source switch; no fabricated centroid/unknown pin |
| ANP / community | Source, date, unit and condition stay exact; absence is explicit; no silent fallback |
| Station detail/profile | Reviewed geometry/entrance and price sections load from the admitted source |
| Search / route / stops | Real offline/native search, car route, ordered stops, avoidance, ETA/distance |
| Downloads | Region/size/dependency, Wi-Fi policy, pause/cancel/resume; active generation not deleted |
| Airplane restart | Map/reference/price/search/route/guidance work from admitted local data; missing region refuses |
| No GPS | Browse/search/planning remain usable; guidance start reports permission/GPS state |
| Neighbor region | Complete route only when neighbor is admitted; missing neighbor never becomes success |
| Primary charger failure | Alternative from remaining SOC; no charge from failed unit; no false guarantee |
| Private restore | Owner-scoped saved plan/trip and charge history restore only for the same owner; deletion stays deleted |
| Accessibility | TalkBack/semantics, 48 dp, 200% font, narrow screen, dark/light and existing languages |
| Attribution | OSM/ODbL and engine notices visible; no CoMaps branding/CDN/service key |

## Rollback

- App: restore the prior signed artifact and configuration through the normal mechanism.
- Map: reactivate the previous complete last-good generation; do not delete the active
  generation or an active navigation lease. Stop readers before replacing an mmap file.
- DB: restore the verified pre-change backup into the target; migrations remain append-only.
- Private data: preserve owner scoping and tombstones; never expose a restored foreign account
  or resurrect a deleted item.
- After rollback, repeat the affected smoke cases and record the exact SHA/generation restored.

## Manual result sheet

For each row record: date/time (UTC), operator, staging URL, app build/hash, engine AAR hash,
map generation/hash, source/revision/date, database migration head, device/emulator, test
inputs, expected result, actual result, evidence location, PASS/FAIL/OWED and rollback state.
Pending cases remain pending; they never become PASS by omission.

## Stop conditions

Stop and roll back on: missing/mismatched backup, schema/migration failure, active-generation
deletion, private-data crossing owners, wrong price/source/unit, unreviewed geometry used as a
pin, routing success on a missing region, or a required check that is missing/failed/skipped/
cancelled. Do not weaken validation, signature, location review or tests to make staging green.

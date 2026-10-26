# MAP-32 — Scoped staging and manual acceptance runbook

Status: LOCAL_DONE (runbook + local restore rehearsal) / **STAGING_PENDING**
Validation: PASS for the local runbook and restore rehearsal
Integration: INTEGRATION_PENDING
Gate: no staging PASS; production/G09 is not authorized

Runbook: [MAP_ACCEPTANCE_RUNBOOK](../../operations/MAP_ACCEPTANCE_RUNBOOK.md).

## Delivered

- Frozen candidate manifest: final release APK 78,396,538 bytes / 74.76 MiB, SHA-256
  `feadd9f3…`, both ARM ABIs, 16 KB packaging; debug/test hashes; pinned AAR/classes/core
  hashes; admitted MWM hashes; migration head 132/163 tables.
- Staging-scoped apply/backup/rollback/smoke sequence for `https://teste.abastevo.com.br`
  only, with feature/package last-good and active-reader protections, append-only migrations,
  owner-scoped private restore and explicit stop conditions.
- Local disposable PostGIS restore rehearsal: dump/restore of the 132-migration database
  produced 132 migrations / 163 public tables on both source and restore. This is not a
  staging backup.
- Read-only origin probe: the origin responded, but `/`, `/health`, `/healthz`, `/v1/health`
  and `/api/health` returned 404. No authenticated operator access, SSH/DVCS access or
  database credential was available, so **no staging update or smoke was executed**.

## STAGING_PENDING

Exact retake condition: authorized staging access through the existing operator mechanism,
current SHA/schema/config inspection, verified backup/restore target and an explicit staging
window. The staging origin is not production; no production, store, public pilot, tag or
payment/portal publication is authorized by this record.

The manual smoke matrix covers fuel/electric/combined, exact source switching, detail/profile,
search/route/stops, downloads, airplane restart, missing GPS, neighbor regions, failed primary
charger alternative, private restore, accessibility/attribution and rollback. A missing case
stays OWED; it is not converted to PASS.

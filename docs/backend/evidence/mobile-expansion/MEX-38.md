# MEX-38 — Operational and external acceptance handoff

Status: LOCAL_DONE
Validation: PASS
Integration: INTEGRATION_PENDING

Parent: `113d6df7`. Date: 2026-10-23.
Depends MEX-37; owns PRM10/FQ-17 handoff only.

## Handoff

The immutable source-ready candidate, config/schema/source/policy/
license/census manifest is [EE-30-MANIFEST.json](../ev-support/EE-30-MANIFEST.json).
It is a local acceptance artifact, not a deployment authorization.

- Candidate code head: `887d5a71` (later Markdown-only commits).
- Local runbook/rehearsal: restore/erase/retry and source replay are
  covered by MEX-33/MEX-36 and MEX-34; flags stay OFF.
- External proofs OWED: Play sandbox, signed staging two-device,
  private R2/S3 deletion receipts, licensed operator ACCESS_PENDING,
  physical Android accessibility/Room/offline, production deploy/
  migration/load/restore and public pilot.
- No VPS/namespace mutation, paid activation, source activation,
  deployment, tag or customer data is included.
- D/iOS/G09/public pilot remain outside this acceptance; iOS stays
  archived.

Recovery: disable the affected producer/feature, retain published and
private data, restore a verified backup or redeploy the prior image;
append-only migrations are repaired forward, never forced down.

Next: EE-31/MEX-39 combined overview.

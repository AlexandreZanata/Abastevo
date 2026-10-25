# Public portal publication gate (ESP-18)

Status: **NOT PUBLISHED — EXTERNAL AUTHORIZATION REQUIRED**
Date: 2026-10-25

This runbook is the only authorized path to a public Abastevo portal. It does
not publish by itself and it does not authorize DNS, CDN, VPS, TLS or edge
changes. The current implementation is a local preflight and post-publication
smoke; no public release was executed.

## Hard prerequisites

All must be explicit and current before any remote mutation:

- a named public `origin` (HTTPS, no path) and the owned host/root;
- host ownership/DNS/TLS attestation and the CDN decision (none or provider);
- an approval reference and a named approver, plus a bounded maintenance window;
- a selected public source scope. Under
  [ENTERPRISE_SOURCE_ADMISSION](../product/ENTERPRISE_SOURCE_ADMISSION.md), only
  the official fines slice (`S-ESP-04`) is publicly admitted; price, registry and
  PMQC public slices remain `RIGHTS_PENDING`. A candidate containing
  `/precos/`, `/postos/` or price download CSVs is **not publishable** until the
  rights register changes.
- a candidate rebuilt with the approved origin. The current full candidate uses
  the placeholder `https://portal.example` in its sitemap/robots; it is a
  certification/preview artifact, not a public artifact.
- the exact candidate content hash and a previous safe rollback hash.

The static website publication does **not** certify the Android app, API,
private media, national source activation, public pilot or production G09.

## Scope file

The preflight accepts a JSON scope like:

```json
{
  "schema_version": "abastevo-portal-publication-scope-v1",
  "origin": "https://...",
  "host": "...",
  "root": "/...",
  "cdn": "none|...",
  "tls": "acme|...",
  "approval_ref": "...",
  "approved_by": "...",
  "window_start": "RFC3339",
  "window_end": "RFC3339",
  "candidate_content_sha256": "sha256:...",
  "rollback_content_sha256": "sha256:...",
  "source_slices": ["official-sanctions"],
  "source_admission_ref": "...",
  "rights_evidence": ["..."],
  "host_ownership_attestation": "...",
  "source_revision_id": "...",
  "source_lease_id": "...",
  "source_checksum": "...",
  "source_edition": "2025",
  "source_rows": 3375
}
```

Placeholders (`example`, `invalid`, `localhost`, `TBD`, `<...>`), private IPs,
HTTP origins, paths with `..`, price/registry/PMQC public slices, missing
rights references and missing source pins are rejected. The scope is an
operator declaration; the script cannot authenticate the approver or ownership.

## Local preflight

```sh
python3 infra/scripts/load/publication_preflight.py \
  --artifact <verified-candidate> \
  --scope <publication-scope.json> \
  --out <preflight.json>
```

Exit codes: `0` only for non-public helper modes; `1` local gate failed; `2`
usage/scope refused; `3` local preflight passed but execution approval and
external ownership verification are still required. The script never contacts
the public origin, VPS, API, database or ANP.

Checks include: candidate hash, source revision/lease/checksum/edition/rows,
official census, sitemap origin, robots indexability/sitemap, page
`index,follow`, objectionable price-page presence and the selected public
source scope. It writes `preflight.json` and always records
`publication_executed: false`.

## Publication sequence (only after approval)

1. Rebuild the candidate for the approved origin and the admitted official-only
   scope; exclude `RIGHTS_PENDING` price/registry/PMQC pages and downloads.
2. Run `portalcheck` and the preflight; require exit `3` (local pass) and a
   reviewed external attestation.
3. Copy/atomic-switch the release on the owned host; do not overwrite a live
   `current`; keep the previous release for rollback. Configure the public edge
   without the preview `X-Robots-Tag: noindex`, with HSTS/TLS, cache headers,
   sitemap/robots indexability and the CDN policy.
4. Run the public smoke with the exact content hash:

```sh
bash infra/scripts/smoke_portal_public.sh \
  --origin https://<approved-origin> \
  --expected-sha sha256:<candidate> \
  --pages /consultar/,/fiscalizacao/,/metodologia/ \
  --out <smoke-summary.txt>
```

5. Only after smoke PASS, publish/refresh `sitemap.xml` and indexability, then
   purge the CDN if selected. Record the exact release, source pin, counts,
   smoke output and remaining rights limits.
6. Monitor and rollback to the last safe release if the candidate, source,
   edge or smoke fails. Withdraw or rebuild outputs with corrected/withdrawn
   official records; never leave a withdrawn record served.

The smoke is read-only and refuses localhost/example/private hosts. It was
self-tested but not run against a public origin because none is approved.

## Current blockers (2026-10-25)

- No named public origin, host/root/CDN, ownership attestation, approver or
  maintenance window is recorded.
- The current B candidate is internally valid at `https://portal.example`
  (`portalcheck` PASS) but its sitemap/robots use that placeholder origin.
- B contains synthetic price/station/download pages; public price/registry/
  PMQC rights are `RIGHTS_PENDING`. The admitted official slice is event-level
  and all selected records are currently `unmatched`.
- The preview A is deliberately noindex and is not a public candidate.
- ESP-16 capacity was local loopback only; no CDN or VPS-direct campaign was
  run. Public capacity remains OWED.

## Rollback

Keep the prior public release root and its content hash. On failure: stop
promotion, switch `current` back to the last safe release, restore the previous
edge/indexability state, and run the public smoke against the rollback hash.
If the official source is withdrawn, use the publisher's `withdraw` path
(fallback or 404) and remove/rebuild affected sitemap entries. Cross-release
retention of old content-addressed JS/sitemap remains OWED.

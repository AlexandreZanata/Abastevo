# ESP-18 — Public portal publication gate

Status: BLOCKED_EXTERNAL_AUTHORIZATION
Validation: PASS (local gate/self-tests)
Integration: NOT_PUBLISHED
Date: 2026-10-25
Branch: `codex/enterprise-static-portal`
Parent: ESP-17 `103dcf6b`

Rules: B-BR-ESP-13–16; BUC-ESP-06/07.
[Plan](../../../planning/ENTERPRISE_DATA_STATIC_PORTAL_PLAN.md#esp-18--publish-the-selected-public-portal).
[Publication runbook](../../../operator/PORTAL_PUBLICATION.md).
[Source admission](../../../product/ENTERPRISE_SOURCE_ADMISSION.md).

## Outcome

**No public portal was published.** ESP-18 requires an explicit named
publication scope, protected delivery and an approved public host. None is
present in this session. Publishing the current artifact would also violate the
frozen source-rights register. The local publication gate, read-only public
smoke and runbook are implemented and validated; remote publication remains an
external, approved operation.

This phase therefore cannot claim `ESP-PUBLISHED`, a public release, API/app
certification, public pilot, G09 certification or production capacity.

## Candidate and source pin

The current full candidate (ESP-17 B) is internally valid at its declared
example origin:

```text
content      sha256:914d8b9fab77c807fbf15ef2829b7edc42e6620e4caeb3b022eb2b9694179a64
shape        3,409 pages / 3,421 files / 15,932,892 bytes
input hash   sha256:ef427fb5…
sitemap      3,408 URLs, origin https://portal.example
official     revision fc18397c-cf1c-41aa-a155-a3d5c4bbfa5c
             lease    a18283c151938ddbb56574ddbd897629cda050c887fefbc8ce81e0dd6c150a88
             source   277f6cf5…91fe96, edition 2025, 3,375 rows
             census   3,375 unmatched / 0 linked / 0 ambiguous
```

`portalcheck` on this artifact at `https://portal.example` PASSES (manifest,
hashes, allowlists, sitemap, assets, official census, privacy). That is integrity
evidence, **not** publication readiness: the origin is a placeholder and the
artifact contains source slices that are not publicly admitted.

## Hard blockers

1. **No named public scope or approval.** The host/root, origin, DNS/TLS, CDN
   decision, ownership attestation, named approver, approval reference and
   bounded maintenance window are absent. The existing
   `https://teste.abastevo.com.br` is a staging origin, not a public portal
   approval; `abastevo.com.br` ownership is unverified in the landing contract.
2. **Source rights pending.** Under the frozen source admission register,
   S-ESP-02 prices, S-ESP-01 registry and S-ESP-05 PMQC are `RIGHTS_PENDING`
   for public derived release. The current B contains `/precos/`, `/postos/`
   and price download CSVs from the synthetic fixture. Only the official fines
   slice S-ESP-04 is publicly admitted (CC BY, event-level); a public official-
   only artifact does not yet exist.
3. **Candidate origin.** B's sitemap and robots use `https://portal.example`.
   A public candidate must be rebuilt with the approved origin; the existing
   hash cannot be reused after that rebuild.
4. **Preview is not public.** ESP-17 A is deliberately noindex and has no
   public TLS/host. It is a preview artifact, not a publication candidate.

## Implemented gate

- `infra/scripts/load/publication_preflight.py`: validates a named scope
  (public HTTPS origin, host/root, CDN/TLS, approval, window, candidate and
  rollback hashes, source pin, admitted slices, rights references, ownership
  attestation), then checks the artifact hash, official pin/census, sitemap
  origin, robots indexability, page `index,follow`, price-page presence and
  official-only scope. It never contacts the public origin or mutates anything;
  it writes `preflight.json` with `publication_executed: false`.
- `infra/scripts/smoke_portal_public.sh`: read-only post-publication smoke for
  an approved public origin. It verifies the exact content hash, HSTS/security
  and cache headers, gzip, absence of `X-Robots-Tag: noindex`, robots
  `Allow`/origin sitemap, sitemap origins, configured pages, custom 404 and
  `/v1`/`/health` 404. It refuses localhost/example/private origins.
- `docs/operator/PORTAL_PUBLICATION.md`: scope schema, approval prerequisites,
  preflight/publication/smoke/indexability sequence, rollback and external
  obligations.
- Makefile targets `check-publication-gate`,
  `preflight-portal-publication`, `smoke-portal-public`.

## Preflight result (synthetic scope, not an approval)

A syntactically valid **test** scope (`https://portal.abastevo.com.br`, no
ownership/DNS/TLS claim) was used only to exercise the gate against B. It was
not treated as an approval and no network call was made.

```text
status: FAILED_LOCAL_GATE
hard failures:
- RIGHTS_PENDING: artifact contains price/station/download pages; selected public slice is official-sanctions only
- sitemap origin mismatch: expected https://portal.abastevo.com.br, found https://portal.example/consultar/
- robots.txt sitemap does not match the public origin
passing local checks:
- candidate hash, official pin, official census, official slice, page index,follow
```

The gate is working as intended: it blocks the current candidate instead of
publishing a placeholder-origin, rights-pending full portal.

## Validation

```sh
make check-publication-gate
```

PASS: publication preflight self-test and public smoke self-test (origin
guards, placeholder/private-host refusal, sitemap parsing). `python3 -m
py_compile`, `bash -n`, `portalcheck` B, and `git diff --check` PASS. The public
smoke was **not** executed against a public origin because none is approved.

## Publication requirements (next authorized action)

1. Product/owner supplies a named public scope and approval, host/root, origin,
   DNS/TLS/CDN ownership attestation and window.
2. Rights are resolved for S-ESP-01/02/05, **or** an official-only public
   artifact is selected and built without price/registry/PMQC pages/downloads.
3. Rebuild the candidate with the approved origin and admitted source slices;
   run `portalcheck` and the preflight.
4. After explicit execution approval, atomically publish the release on the
   owned host, configure the public indexable edge, publish sitemap/robots and
   purge CDN if selected.
5. Run `smoke_portal_public.sh` with the exact hash; record the public release,
   source pin, counts, smoke output and remaining limits.
6. Roll back to the last safe public release on any failure; withdraw/rebuild
   corrected or withdrawn official records.

## Limits

- No VPS, DNS, CDN, TLS, shared edge, Caddy, Compose, API, database or ANP
  mutation occurred. No public smoke was run.
- ESP-16 capacity is local loopback only; no CDN or VPS-direct capacity result
  exists. Public capacity remains OWED.
- Real prices/registry/PMQC public rights remain pending; synthetic price pages
  are not publishable. The official 2025 slice is real, event-level and all
  unmatched; no station link is claimed.
- Cross-release retention of old content-addressed JS/sitemap remains OWED.
- Website publication would not certify the Android app, API, private media,
  public pilot or production G09.
- No `main` merge, PR or release occurred.

## Next

External authorization and rights resolution are required before ESP-18 can be
executed. No further portal phase is claimed complete here.

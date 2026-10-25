# ESP-STAGING-01 — Staging FQ-07 ingestion and official-only /fiscalizacao

Status: STAGING_DONE
Validation: PASS
Integration: STAGING_ONLY / NOT_PUBLIC
Date: 2026-10-25
Branch: `codex/enterprise-static-portal`
Parent: ESP-18 `f1591b82`

Scope authorized by the user 2026-10-25: real (non-synthetic) FQ-07
2016–2025 ingestion served through the staging API, plus the official-only
`/fiscalizacao` route on `https://teste.abastevo.com.br`. Staging only:
noindex everywhere, no sitemap, no price/station/search/download routes, no
public release, no app/API/G09 certification.

Rules: B-BR-ESP-01/04/07–09/16; BUC-ESP-05/06.
[Source admission](../../../product/ENTERPRISE_SOURCE_ADMISSION.md) (S-ESP-04
event-level admitted; S-ESP-01/02/05 `RIGHTS_PENDING`).
[Publication gate](../../../operator/PORTAL_PUBLICATION.md) (public path;
this phase executes the staging subset only).

## Outcome

Staging now serves the admitted official sanctions slice from real ANP data:

- API: `/v1/quality/revisions?stream=sanction-v1` lists all seven editions
  (`2016-2019`, `2020`–`2025`), each `VERIFIED`; current lease is edition
  `2025`, revision `a91aff79-994e-4cf9-b673-bb71da19b6b6`, published
  `2026-10-25T01:14:25Z`, reconciling `3395 input = 3375 accepted +
  0 duplicates + 20 quarantined`.
- Static route: `/fiscalizacao/` (index + pagination + 3,375 record pages),
  `/metodologia/`, `/assets/*` and `/portal-manifest.json`, all
  `noindex, nofollow` with revalidation cache policy.

No synthetic row was ingested or published. No CPF, price, registry or PMQC
slice was published. Linked records (1,669 in 2025) render their decided
association state with **no station link**; station pages do not exist in
this slice. Full linked station journeys remain OWED with the full portal.

## Ingestion (namespace `abastevo-temp` only)

Pre-ingest backup first (isolated Job `abastevo-fq07-backup-20261025`):

```text
dump  29,268,493 bytes, mode 600
sha256 74524b592c446b629e062279407bbca2a9661010d260f0bcda9f04abf3e25434
```

A bounded worker Job (`abastevo-fq07-worker-20261025`, existing deployed
image, only `ANPFUEL_QUALITY_IMPORTS_ENABLED` +
`ANPFUEL_QUALITY_SANCTIONS_DISCOVERY_ENABLED` true, ANP/PMQC/enforcement
discovery explicitly false) drained seven manually enqueued
`quality-sanctions-discovery` rows (unique `manual:fq07-discovery:1..7`
dedupes). Final queue state: `7/7 discovery done`, `7/7 import done`,
zero failures. The bounded worker Job was deleted afterward; the live
deployment (`abastevo-temp-api`, zero restarts) was never patched.

The deployed worker image predates the enterprise-export handler, so the
official export was built by a temporary staging-only helper
(`fq07export-tmp`, static binary
`sha256:64d12dcb6d6a24a5aa9be664007f81add88927d213ae4ba6946494687ec052d0`,
never committed, removed from the repo): read-only repeatable-read
transaction → `ExportPin`/`BuildOfficialExport` (limit 500) → minimal
price-empty snapshot → `portalgen --official-only` → `portalcheck
--official-only`. Raw ANP bytes never entered Git; private export files
stay under the owned VPS path with `0700/0600`.

Implemented in this branch (committed): `portalgen --official-only`
(emits only fiscalizacao/metodologia/404/assets/robots/manifest, delinks
station hrefs, requires an official dataset), matching
`portalcheck`/`portalrelease --official-only` enforcement and release
`OfficialOnly` mode threading. Full-portal behavior is unchanged
(`enterpriseexport` suites green).

## Candidate and release

```text
content  sha256:460d4b51ca4a908fdd4c064fb239edd741530d1acca26147585e6fb53f7a7711
shape    3,394 pages / 3,397 files / 14,867,109 bytes
official edition 2025, rows 3375 (linked 1669 delinked-display, unmatched 1656, wrong_branch 50, ambiguous 0)
manifest official_only=true, noindex, no sitemap
```

Promoted with `portalrelease promote --official-only` (`current=
sha256:460d…`, no previous release). Staging binaries used:
`portalgen sha256:ccfdcdc5…`, `portalrelease sha256:fc8b4268…`
(hash-verified after transfer).

## Edge (additive only, `abastevo-temp` server block)

Backup before mutation (mode 600, outside Git):

```text
/opt/abastevo-temp/fq07-staging-20261025/private/nginx-abastevo-temp.conf.bak-20261025-124307
```

Four additive locations (`/portal-manifest.json`, `/fiscalizacao/`,
`/metodologia/`, `/assets/`) serving a readable copy of the verified
release through the atomic `serve/current` symlink; `nginx -t` PASS and
graceful reload. No other app server block touched; `/health/*`,
`/v1/*` and `/consultar/` behavior unchanged (API 200/200/404).

## Smoke (2026-10-25, TLS verified, no bypass)

- `portal-manifest.json` 200, content hash equals the promoted release,
  `official_only=true`, 3,394 pages.
- `/fiscalizacao/` 200, `noindex` meta + `X-Robots-Tag` header,
  revalidation cache policy, ANP attribution, zero `href="/precos|/postos|/consultar"`.
- `/metodologia/` 200; `/fiscalizacao/pagina/2/` 200; one record page 200
  with the negative-certificate limit, decided association state and no
  station link.
- `/assets/` directory listing refused (403); unknown path 404;
  `/consultar/` still API 404; `/health/live` 200 `{"status":"ok"}`;
  `/health/ready` 200; `/v1/quality/sanctions?limit=1` 200, edition 2025.

## Rollback drill (executed on staging)

`portalrelease withdraw --official-only` → `current` removed →
`serve/current` removed → `/fiscalizacao/` 404, API untouched. Then the
same artifact was repromoted (`current=sha256:460d…`), `serve/current`
relinked, `/fiscalizacao/` 200 with the identical manifest hash.
Standing rollback: restore the edge backup + reload, `withdraw` (or
`rollback` once a second release exists), repoint/remove
`serve/current`.

## Limits

- Historical editions (2016–2024) are API-readable through revision
  leases; the static route presents the current 2025 edition only.
- Linked records display the decided association without station
  navigation; station/price/search/download pages do not exist here.
- Price/registry/PMQC public rights remain `RIGHTS_PENDING`; the full
  portal and ESP-18 public publication stay blocked.
- No public indexing (no sitemap, noindex edge+meta), no pilot, no
  production/G09/app certification, no `main` merge or PR.
- The temporary export helper is intentionally uncommitted; rerunning it
  requires rebuilding from this branch.

## Next

None for this phase. The next authorized staging step is a second
official-only release (replay promote/smoke/rollback); the next
portal-scope decision stays with rights/public-origin approval.

## Addendum 2026-10-25 — review corrections (history preserved above)

A review reproduced three defects in the tooling this phase relied on; the
recorded rollback drill exercised the first one. Nothing above is deleted;
what follows corrects the interpretation and records the remaining work.

1. **Withdrawn releases could be re-promoted.** `Promote` only checked the
   withdrawal mark when the hash was already current, so withdraw →
   repromote of the same artifact succeeded. The drill above therefore did
   **not** prove a valid rollback: it proved the resurrection path. Fixed
   in this branch: `Promote` refuses withdrawn content
   (`ErrReleaseWithdrawn`) before any copy/switch and preserves the mark;
   `revoked-source` withdrawals additionally record the official
   revision/lease identity, and any artifact carrying it is refused by
   promote, rollback and fallback (`ErrOfficialRevoked`). A valid
   operational rollback is A → B → A across two candidates, with fallback
   to a safe release or 404 on withdrawal. The staging A → B → A drill is
   OWED on the next explicitly authorized staging execution (no VPS
   mutation under a local correction prompt).
2. **Staging access posture.** `noindex` means not indexable; it does not
   mean private. The staging route answered anonymous GET during this
   review. No access control is applied; an edge-compatible control scoped
   to staging consumers is prepared in the runbook (NOT_APPLIED, no
   secrets, API untouched).
3. **API history is not static history.** The seven editions are readable
   through revision-pinned API leases; the static route presents the
   current 2025 edition only. Do not cite API coverage as static coverage.
4. **Obligation ledger for the portal line.** Implemented locally,
   accepted with real data, running in staging, integrated, published and
   externally dependent are different states:

   | Obligation | State |
   | --- | --- |
   | ESP-00–ESP-06, ESP-08–ESP-11, ESP-13–ESP-15, ESP-17 | LOCAL_DONE |
   | ESP-07 real prices (880 over-precision rows) | BLOCKED_OVER_PRECISION |
   | ESP-12 official insights/downloads | PARTIAL (price slice only) |
   | ESP-11 live association revalidation/rebound | OWED |
   | ESP-16 public capacity | OWED (local loopback only) |
   | ESP-18 public publication | BLOCKED_EXTERNAL_AUTHORIZATION |
   | ESP-STAGING-01 continuous operation | BLOCKED until the staging A → B → A drill and the corrections above ship |

It is therefore false that only ESP-18 remains: the table above owns the
rest. Continuous-operation readiness for ESP-STAGING-01 stays blocked
until the authorized staging drill replays promote/smoke/withdraw with
two valid candidates under the fixed publisher.

## Addendum 2026-10-25 (2) — retention acceptance vs 404 rehearsal

The rollback drill above proved withdrawal semantics (404 + no
resurrection), not asset retention: after the switch the old release's
exclusive assets were simply unserved. Retention acceptance now exists
separately and locally: `portalrelease retain` rebuilds a hardlinked
`retained/` asset store from the newest non-withdrawn releases within a
byte budget (unit tests: byte-exact hardlinks, keep limit, budget
fail-closed, no pages retained, withdraw purges exclusive assets), and
the Caddy rehearsal proves over HTTP that a rolled-back release keeps
serving its retained assets while pages come from the current release
with no mixing, in both full and official-only modes.

Operational diff for the next authorized staging execution: run `retain`
after every promote/rollback in the same step, map the edge `/assets/*`
to the retained store (pages stay on `serve/current`), and re-run the
retention assertions; the current staging serve copy predates retention
and must be re-cut through this path, not patched in place.

## Addendum 2026-10-25 (3) — second-review correction of this contract

The retention contract in Addendum (2) is superseded operationally:
`promote`, `rollback`, `refresh` and `withdraw` now prepare and verify the
retained store automatically, so `retain` is a repair command rather than a
mandatory post-step. The current release is mandatory even beyond `keep`;
the store is an immutable generation selected by an atomic symlink, with
crash recovery and failed-switch preservation. Pause recovery for identical
source bytes and the serialized official publication/frozen-revision rules
are recorded in [ESP-CORRECTIONS-02](ESP-CORRECTIONS-02.md). The staging
serve copy still predates this correction and must be re-cut through the
publisher when staging work is authorized; no staging execution is claimed
here.

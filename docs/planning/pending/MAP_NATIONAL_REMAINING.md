# National fuel-station map — what remains

Status: INTEGRATION_PENDING (2026-10-27). Phases 1–4 ran nationally and are
APPLIED on staging (`abastevo-temp`) with D1 = yes, D2 = yes (exact number,
CNEFE species 6, levels 1–3), D3 = evidence only. PR #162 (contains #156 and
#155) is merged into `codex/phase-integration-20261027`, not yet into `main`.

## Context

- Plan: `docs/planning/MAP_NATIONAL_STATIONS_PLAN.md`; final receipt with
  "Not done / still open": `docs/planning/MAP_NATIONAL_FINAL_RECEIPT.md`.
- Earlier receipts: `docs/planning/MAP_NATIONAL_APPLY_RECEIPT.md`,
  `docs/planning/MAP_NATIONAL_STATIC_OVERLAY_RECEIPT.md`; operator guide
  `docs/operator/map-static-overlay.md`; device evidence
  `docs/backend/evidence/maps/map-national-device-20261027/`.

## Already closed (do not redo)

- A1.a HTTP 429: closed by `uf=<UF>` requests without `municipio`
  (29 requests, AC 60/60 identical); the national run is no longer a sample.
- D1/D2/D3 decided; 40,388/45,688 registry stations (88.4%) pinned across 27
  UFs; migration `000139`, `station-locate`, overlay release
  `national-20261027-b3fec38c`, verified rollback path recorded.

## Remaining work

1. Land the integration on `main`. Acceptance: integration PR merged with
   required checks green; superseded PRs #155, #156 and #162 closed with a link
   to the merged PR (DELIVERY_WORKFLOW "One PR per planning stage"); the
   stale bodies of #156 (A1.a "blocked", D1/D2 "open") not reused as status.
2. Stale planning text. `docs/planning/MAP_NATIONAL_APPLY_GATE.md` "Still
   blocked" still lists D1/D2 and A1.a as open. Acceptance: a superseded note
   pointing to the final receipt; history kept.
3. Staging runs the integrated release. Staging images are `stg-7c7d776f`
   (hand-deployed); after item 1, the CD agent (or one supervised apply)
   must roll out `main` without dropping the map. Acceptance: `/v1/stations/nearby`
   smoke returns tiered pins in the four receipt cities and
   `current/manifest.json` still resolves after the rollout (tracked in
   `CD_STAGING_ACCEPTANCE_REMAINING.md`).
4. Overlay refresh. Plan §4 says the overlay is built "after each import";
   the operator guide documents a manual `mapoverlay` run and no scheduled
   job exists in `infra/k8s/staging/` or the worker (verify). Acceptance: the
   overlay is rebuilt when the active generation or official week changes,
   switched atomically, old release retained; a receipt per rebuild.
5. National pipeline rerun cadence. A1–B3 ran once (2026-10-27) from
   private VPS inputs. Acceptance: documented rerun procedure and cadence
   (registry/API/CNEFE/RFB refresh), byte-identical reruns on unchanged
   inputs, polite API spacing kept (`MAP_NATIONAL_SOURCE_LIMITS.md`).
6. L5 stations (5,300: 3,441 no CNEFE establishment at the exact address,
   1,758 without usable number). Path is the community location flow, no
   centroid. Acceptance: product task opened against the existing
   contribution flows; L5 list published as `coverage.json` per plan B3
   (verify whether it was published).
7. App consumes the static overlay. The app still reads pins from
   `/v1/stations/nearby`; overlay consumption is a separate product step.
   Acceptance: own task with offline/size budgets; nearby kept for taps.
8. Base map distribution (BLOCKED_CONFIG). `mapDistBaseUrl`,
   `mapDistRegion`, `mapDistKeys` are empty in every build and no staging
   distribution origin exists; base tiles render only for the admitted
   Paraná fixture. Acceptance: owner-approved distribution origin and keys
   outside Git, a device run showing base tiles outside Paraná.
9. Dev map-UI device failures. `MapRouteInstrumentedTest` and
   `NativeRouteScreenInstrumentedTest` fail identically on pure `origin/dev`
   `6f8b82b5` (persistent-host and compact-panel UI). Acceptance: both green
   on a device, fixed in the dev map-UI line (PR #154 is still open
   `dev → main`).
10. Nearby API capacity. At ~60 req/s the nearby p95/p99 tail
    (207/658 ms warm) is attributed to the single CPU limit of the API
    container under the namespace quota. Acceptance: owner decision on quota
    or replicas, or accepted as the staging limit; precompressed `.gz`
    serving stays rejected on Caddy 2.10.2 (206 to plain GET) until a CDN.
11. Production. No production namespace, DNS or release was touched.
    Applying `000139`, `station-locate` and the overlay to production is G09
    scope and needs an explicit owner request; not part of this front.

## Blockers and owner decisions

- Item 8: distribution origin and keys (owner).
- Item 10: namespace quota/replica budget (owner).
- Item 11: G09 production release (owner, separate request).
- Item 3 depends on the CD staging acceptance.

## Delivery: one branch, one PR

Items 2, 4 and 5 (docs + overlay automation) form one stage on branch
`codex/map-national-followups`, one PR to `main`. Items 6–9 are separate
product/UI stages with their own plan entry each; item 1 is the current
integration PR; items 10–11 are owner decisions, no PR until decided.

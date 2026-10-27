# DRV-11 — Revisioned reports and private HTTP contract

Date: 2026-10-26. B-BR-DRV-12/15/17; BUC-DRV-06/09/10. Freeze before behavior.

## Scope

This phase exposes private driver earnings pages and revisioned reports over
HTTP, backed by owner-scoped storage. It does not add a mobile UI, cloud opt-in,
provider integration, checkout or public endpoint. The feature is off unless
`ANPFUEL_DRIVER_ENABLED=true`; disabled routes return 404 without touching
storage and the existing ledger/app routes are unchanged.

## Report projection

- Windows: DAY, WEEK (Monday start), MONTH and CUSTOM in an explicit IANA
  timezone. The window is half-open [from, to); CUSTOM is bounded to 366 days.
- Earnings are owner/vehicle scoped, optionally platform-filtered, and must be
  fully contained in the window. Partial overlap is explicit coverage, never a
  silent clipped sum. Current adjustments are included only when their target
  revision is current; stale/obsolete adjustments leave the result unknown.
- Costs use the newest cost-profile version effective at the window start.
  Only items whose effective period is fully inside the window are included;
  partial/open-ended items or unknown amounts make coverage incomplete and the
  recorded result unknown. Actual and modeled costs stay separate. No fuel
  purchase is treated as consumed fuel and no rate/tax/depreciation is invented.
- Distance selects exactly one documented basis/window observation. GPS and
  odometer are never added. Unknown distance prevents per-km results.
- Work time is not projected in this phase (shift wall-clock start/end are not
  persisted); time ratios are explicitly unavailable, never zero.
- Money is integer BRL centavos, distance integer metres; ratios use exact
  rational intermediates and are returned as strings. Generated reports carry a
  SHA256 source revision computed from earnings revisions, adjustments, cost
  profile and distance content.
- Source reads are bounded to 1000 rows. More rows fail with
  `driver: report source exceeds bounded limit`; lists are never silently
  truncated. An expected revision that no longer matches fails with
  `driver: stale report revision` before writing a snapshot.

## Snapshots and HTTP

- `driver_report_snapshots` stores owner/vehicle/window/timezone/platform/basis,
  source revision, increasing snapshot version, private JSON payload and
  generation time. Account soft/physical deletion erases it. A worker cleanup
  removes snapshots older than 400 days in bounded batches (1000/run).
- `POST /v1/driver/reports/build` requires live account session, exact-body
  signed owner key and `professional.report`; it recomputes and upserts.
- `POST /v1/driver/reports/get` is a free retained read; it returns no-store
  data only when the stored source revision still matches, otherwise 409 stale.
- `POST /v1/driver/earnings` is a free retained read with 1..100 items,
  owner-bound HMAC cursor, no-store and no ETag.
- All routes require signed owner proof. Errors are no-store and do not leak
  owner data. Cursors are bound to owner/origin; cross-owner use fails.

## Evidence boundary

Real disposable PostGIS must prove window/revision recomputation after late
edits, stale snapshot refusal, cost/distance coverage, retained reads after
capability lapse, snapshot erasure, bounded cleanup, HTTP no-store/cursor/auth
and feature-disabled 404. OpenAPI golden vectors preserve exact units. Real
provider identity/payment proof, mobile/device acceptance and production
certification remain later gates.

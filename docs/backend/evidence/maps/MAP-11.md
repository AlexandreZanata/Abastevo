# MAP-11 — Freshness, moderation and source lifecycle

Status: LOCAL_DONE / INTEGRATION_PENDING
Validation: PASS (revocation, kill switch, clocks and cursor generation)
Integration: INTEGRATION_PENDING

Tested tree: `codex/map-ev-implementation` at `721582b8` plus the uncommitted
MAP-09–11 changes. Date: 2026-10-24.

## Behavior frozen

- Observed, published and fetched clocks are stored and returned separately.
  No code derives a fixed expiry, a confidence percentage or live availability
  from them. The open-only launch keeps `capabilities.live_status=false`.
- A withdrawn/corrected source record is revoked through an append-only
  `map_assertion_revocations` row. Its historical assertion and prices remain
  in every generation for audit, but the record is excluded from all reads.
- Repeating the same assertion revocation is idempotent.
- A generation kill switch marks the generation `REVOKED`, records an
  append-only `map_generation_revocations` row, and removes the active read
  target. Replaying the same revoked content is refused rather than silently
  reactivating it.
- Cursors are bound to the active generation. A new generation or kill switch
  invalidates old cursors; a stale cursor cannot read a newer or revoked
  generation.
- Static OCM/OSM status, community reports and future dynamic feeds remain
  separate. This phase does not turn a static point into a live-availability
  claim and does not invent an operator feed.

## Verification executed

Real PostGIS integration in `internal/modules/mapdata`:

- publish a generation, revoke assertion `11111111-…`, query the same viewport:
  the revoked assertion is absent while its historical row and prices remain;
- revoke the same assertion twice: idempotent;
- kill-switch the active generation: `ActiveGeneration` returns no active row,
  the API returns an empty page with `generation=null`, and republishing the
  same content returns `ErrRevokedGeneration`;
- stale cursor after a new generation returns HTTP 400 (shared with MAP-10);
- cancel a publish before the transaction: zero generations and no partial
  overlay.

CLI proof on the disposable proof database:

```sh
go run ./cmd/map-revoke --assertion 11111111-1111-4111-8111-111111111111 \
  --reason 'source withdrawn' --source-ref 'proof-2026-10-24'
# assertion=... revoked
# revocation rows=1; historical assertion rows=1

go run ./cmd/map-revoke --generation 6d884633-7819-4a49-81e8-4d7222db2417 \
  --reason 'operator kill switch' --source-ref 'proof-2026-10-24'
# generation=... revoked
# API: {"generation":null,"items":[],"next_cursor":null}
```

## Limits

- Community report/moderation concurrency, signed owner operations and
  operator live feeds are owned by the existing community/charging modules;
  this public overlay phase only consumes their admitted output.
- Revocation is an operator CLI action, not an owner-facing endpoint.
- Clock-boundary behavior for future timestamps is preserved as data, not
  rejected; no liveness is inferred. `mapcontract` tests cover a future
  `fetched_at` and reject a malformed timestamp.
- No production source activation, staging or release claim.

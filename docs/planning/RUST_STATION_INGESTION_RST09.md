# RST-09 — City/profile handoff

Status: IMPLEMENTED — additive read projection only; no schema
migration, no publication change, no new service. Date: 2026-10-09.
Scope: `RUST_STATION_INGESTION_PLAN.md` RST-09. Language: English
document; user communication in Portuguese.

## 1. What was added

- Source-separated provenance on directory reads: `Station.official`
  carries the latest complete official anchor (source, display,
  municipality, state, run finish time) or nil for community-only
  stations. The anchor reuses `FindOfficialAssertion` (extended with
  the run finish time as freshness signal) inside the existing
  per-row `Reader.station` mapping; official rows never absorb
  community reports and community stations never borrow trust.
- Municipality reads stay code-exact by construction (aliasing lives
  at write time); existing keyset pagination is unchanged.
- OpenAPI `Station` gains the nullable `official` object to match the
  DTO; vacuum reports no new warnings.

## 2. Evidence (revision `018e91f` + working tree)

- Real disposable PostGIS: staged rows project with `registry-csv`
  anchors and fresh timestamps; a community-only CNPJ resolves with a
  nil anchor; point-less stations list in Search with nil coordinates
  and unknown quality while Nearby returns none; two-page city reads
  union without overlap or loss; unaccented name queries do not match
  accented names while code filters ignore display text entirely.
- `go build ./...`, `go vet`, `gofmt` clean; apicontract tests pass;
  directory/community unit suites green. sqlc regenerated with the
  pinned v1.31.1 and `sqlc diff` clean.

## 3. Rollout/rollback runbook

- Rollout: deploy normally; the projection is query-time, so already
  reconciled stations gain anchors immediately with zero backfill and
  no migration window. Monitor per-row lookup latency on large pages
  (one anchor query per row inside the existing per-row pattern).
- Rollback: redeploy the previous binary. Nothing is stored by this
  change — no table, view, column or backfill exists to remove — so
  canonical identities and historical facts survive removal
  untouched. The OpenAPI addition is nullable and backward
  compatible.
- Next (smallest): RST-10 protocol freeze from the benchmark campaign
  (separately authorized), or staged live-source verification for the
  disabled discovery/reconcile schedules.

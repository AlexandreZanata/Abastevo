# P30-T02 — Canonical operator link and public unclaimed profile

Status: LOCAL_DONE on `codex/phase-30-station-profiles`. Task: P30-T02.
New `stationprofile` module (pure domain, explicit ports, owned
sqlc, no cross-module adapter imports); public read-only profile;
no duplicate identity, no ownership assumption, no private data in
public DTOs.

## Behavior (TDD, critical-first)

- Migration `000036_station_profiles.sql` (append-only):
  `station_operator_revisions` (station→CNPJ + source/validity, one
  open revision via partial unique index) + `station_profiles`
  (station PK, policy version, JSONB projection, revision). No
  prices/grants/private data.
- sqlc owner stanza `db/queries/stationprofile` (schema 000001 +
  000002 + 000036); `sqlc vet` + `generate` clean.
- Domain (`profile.go`, stdlib-only): frozen policy version,
  operator sources (registry/dou/review — suggestions never qualify),
  terminal claim states, bounded public business keys (no CPF/keys/
  GPS/prices ever public).
- Application: `EnsureUnclaimed` (idempotent), `LinkOperator`
  (same-CNPJ replay, close-and-rotate, permitted sources, race
  convergence on the unique guard), `ReadProfile` (honest unclaimed
  shape, operator link without badge — grants arrive in P31).
- Adapters: PG store (replay-safe ensure, 23505→typed busy error),
  anonymous `GET /v1/stations/{id}/profile` (public cache headers,
  404 unknown, private-field scan in tests); `cmd/api` wiring via
  reader closure (modules never cross-read).
- OpenAPI path + `StationProfile` schema (`vacuum` PASS,
  `apicontract` + `check-compat.sh` PASS).

## Validation

- Unit (`-race`): domain 4/4, application ensure/link/rotate/source
  guards, handler public/honest/no-leak/no-premature-badge — PASS.
- Integration (real PostGIS, `-race`): ensure idempotence, rotate
  with exactly-1-open revision, unclaimed read shape, 4-way race
  converging on 1 open revision. Fresh disposable DBs (migrations
  incl. 000036).
- Fixed from real failures (not weakened): replay no-rows handling,
  race FK/test-identity issues (real canonicalizer + Reader now
  prove identity).
- Regression: full `stationprofile` + `directory` unit + integration
  PASS (zero failures); `go vet` clean; `go build ./...` PASS.
- `git diff --check` PASS; `scan-secrets.sh` PASS.

## Limits and next

- Claims/proofs/grants/verified replies are P30-T03…P33; review
  tooling consumes the frozen ports.
- Next: P30-T03 account-bound claim and one-use declaration.

# P16-T03B — Backend intake, persistence and proximity wiring

Status: LOCAL_DONE on `codex/phase-16-location-integrity` (phase PR
#44, draft). Issue: #42 (slice 2 of P16-T03; issue stays open
until the phase PR merges). No camera, live-provider or polling
contact in this slice.

## Slice acceptance (frozen before coding)

- T03B (this commit) — verified intake end to end: optional
  `location` block on observation submit (strict parse, additive;
  old bodies byte-identical behavior), server verification at
  submit (forged verdicts refuse `community.location-forged`,
  nothing stored), PostGIS distance against the precise station
  point with the transient fix (bands only survive), teleport
  review against the contributor's last site, persisted bands
  (`000030`: verdict/proximity/reason, never coordinates),
  DeriveSignals merge (stored VERIFIED + precise site carries
  the band; teleport routes review; everything else keeps the
  honest nil-claim path), consensus HIGH still needs quorum +
  proximity photos (single verified flag stays LOW per new
  golden fixture), additive OpenAPI `LocationEvidence` + 2
  vectors. sqlc cross-owner note: no table sharing — community
  owns its columns, directory owns its distance queries, wired
  in the composition root.

## What changed

- `000030_community_location_bands.sql` + community sqlc stanza
  + regen (Insert/Get/NaturalKey/List carry bands;
  `LastObservationSite`; `EligibleAnchors` regen fallout is a
  compatible row-type rename, mapping untouched).
- `directory/stations.sql` + regen (`FixDistanceM`,
  `StationPairDistanceM`) + `Repository.FixDistance` /
  `StationPairDistanceM` (fail-closed unknown handling).
- `application/submit_location.go` — `LocationEvidence`,
  `ErrLocationForged`, `attachLocation` (verify → proximity →
  teleport; infra failures degrade to UNKNOWN/unflagged, never
  fabricate NEAR); `submit.go` DTO field + 3 nil-safe ports.
- `application/derive.go` — `mergeStoredLocation`;
  `signals.go` — `RiskTeleportSuspect`.
- `domain/observation.go` + `adapters/store.go` — band fields,
  persist/map/conflict/`LastObservationSite`.
- `adapters/http/http.go` — strict location parse (canonical
  numbers, ranges, RFC3339; verdict strings pass through for
  later forged refusal) + 400 `community.location-forged`.
- `contracts/openapi/v1.yaml` — optional `location` +
  `LocationEvidence` schema (verdict enum); vectors
  with-location-valid + bad-verdict-invalid.
- `cmd/api/main.go` — Locate/LastSite/StationDistance closures
  (directory repo + community store, narrow ports).
- Tests: 8 attach suites (forged/NEAR/FAR/UNKNOWN/centroid/
  silent/teleport/baseline/degrade), 3 derive merge suites,
  HTTP parse + forged mapping, signed-stack E2E (201 + stored
  bands, 400 forged + nothing stored), PG E2E (PostGIS NEAR,
  50 km teleport flag, schema has no coordinate columns),
  `single-verified-flag-stays-low` golden.

## RED → GREEN

- RED proven by mapping forged refusals to silent pass: the
  forged unit case fails (raw rejection leaks instead of
  `ErrLocationForged`); GREEN on restore.
- Incidental fixes in-task: `submitBody` helper collision
  across same-package test files; `clock_skew_s` float→int64
  boundary; sqlc single-column scalar type.

## Validation (exact commands, this host)

```sh
cd backend && GOTOOLCHAIN=go1.27.1 go build ./... && sqlc vet && sqlc generate
gofmt -l internal/modules/community internal/modules/directory cmd/api
GOTOOLCHAIN=go1.27.1 go vet ./internal/modules/community/... ./internal/modules/directory/... ./cmd/api/
GOTOOLCHAIN=go1.27.1 go test -count=1 ./internal/modules/community/... ./internal/modules/directory/... ./internal/platform/apicontract/
export ANPFUEL_TEST_DATABASE_URL=postgres://anpfuel:anpfuel@127.0.0.1:5434/anpfuel?sslmode=disable
GOTOOLCHAIN=go1.27.1 go test -count=1 -race -tags=integration ./internal/modules/community/... ./internal/modules/directory/...
vacuum lint -r contracts/openapi/vacuum-rules.yaml contracts/openapi/v1.yaml --no-update-check
git diff --check
bash scripts/scan-secrets.sh
```

Outcome: build + `sqlc vet` clean (`generate` converges);
unit green (8 attach + 3 derive + HTTP parse/map + apicontract
incl. 2 new vectors + golden incl. new LOW fixture); `-race`
integration green (submit E2E, PostGIS proximity/teleport,
schema coordinate scan, signed HTTP stack); `vacuum` 0/0/27;
`diff --check`/secrets clean.

## Limits (not claimed)

- No device acceptance (T04); provider wiring unproven until
  hardware run (release horizon).
- Teleport baseline reads the contributor's own fact history
  (station IDs + times, no fixes stored); within-window fix
  reuse still converges on observation idempotency.
- `high-quorum` HIGH reachability unchanged (existing golden).

## Rollback

Migration `000030` is additive (nullable columns); code
rollback = revert intake/derive/store/HTTP/spec/wiring and
docs. Old bodies submit identically with nil location.

## Next

P16-T04 location device and recovery acceptance (#43, same
branch). Issue #42 stays open until the phase PR merges.

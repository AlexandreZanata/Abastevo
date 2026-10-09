# RST-05 — Owned Go loader with staging tests

Status: IMPLEMENTED — staging only; no publication change, no new
service, no new credentials. Date: 2026-10-09. Scope:
`RUST_STATION_INGESTION_PLAN.md` RST-05. Language: English document;
user communication in Portuguese.

## 1. What was added

- Migration `000049_registry_station_prep_source.sql` (append-only):
  `registry_source_runs.source` accepts `station-prep` alongside the
  two existing sources. Existing rows are unaffected; official-lookup
  reads keep their current source scope until a tested publication
  change says otherwise.
- `registry/loadbatch.go` — `LoadBatch` validates a `station-batch-v1`
  manifest plus its three JSONL streams without touching the store
  (format/UUID/versions, stream SHA256/sizes/row counts, strict JSON
  schemas, kernel CNPJ checks, CHECK-set membership, per-input
  accounting), then stages one run per manifest input through the
  existing `Store`: snapshot `station-prep:<run_id>:<input_key>`,
  idempotent replay via the unique snapshot key, live count agreement,
  succession links resolved to staged identities, `complete` only on
  full agreement. Anything else finishes the run as failed, never
  complete, so the existing complete-run reconciler stays blind to
  partial work.
- Hard gates: assertion streams must stay `unknown` quality without
  coordinates; candidate streams must stay `pending` review without
  accuracy claims; candidate points re-validate against national
  bounds; quarantine reasons stay in the frozen set. Derived candidate
  checksums use the same canonical recipe as station-prep.
- `Store` gains `SetAssertionSuperseded` (PG + fakes); no query text
  changed, so no sqlc regeneration was needed.

## 2. Evidence (revision `a131ce6` + working tree)

- Unit (fake): valid batch stages 3+2 rows with exact counts and one
  succession link; six invalid batches (bad format, tampered stream,
  skewed counts, bad CNPJ, unknown reason, reviewed smuggling) fail
  before any run exists; replay converges; store errors and silent
  count skews finish runs as failed.
- Real disposable PostGIS with `-race`: empty batch completes with
  zero rows; pre-migration CSV source coexists and bogus sources still
  violate the CHECK; 4-way concurrent reloads converge on stable run
  IDs and row counts; tampered streams complete nothing; replay keeps
  assertion identities; mid-load failure completes nothing new; a
  least-privilege role (CONNECT + USAGE + SELECT/INSERT/UPDATE on the
  two staging tables) loads successfully yet gets permission-denied on
  canonical stations. Credentials only via `ANPFUEL_TEST_DATABASE_URL`.
- `go build ./...`, `go vet` on the module and `gofmt` clean. Local
  `staticcheck` binary predates the pinned toolchain (export-data
  mismatch) and was not runnable here; the pinned CI staticcheck owns
  that signal.

## 3. Limits and next task

- `FindOfficialAssertion` source scope is unchanged: staged
  station-prep rows reconcile through existing runs but official reads
  need their own tested publication decision.
- Loader trusts manifest input hashes as recorded provenance (raw
  bytes are verified upstream at fetch/emit); it proves stream bytes
  against the manifest itself.
- Next (smallest): RST-06 location review gate — tested CRS
  transform, independent reference sample and evidence-backed
  promotion/rejection with 150m/fix-integrity cases on real PostGIS.

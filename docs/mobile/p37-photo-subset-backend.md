# P37-PC04A capture-bound shared evidence backend

Status: LOCAL_DONE
Validation: PASS

Backend portion of PC04 only; Android outbox integration remains IN_PROGRESS.
Parent e41936c on maintained dev. Behavior is frozen in
[the subset contract](../backend/PHOTO_SUBSET_SUBMISSION_CONTRACT.md).
Source/test/contract fingerprint: `a90c5f48f2d7641bbe8d69d76e92829a4fc508e583ef6e8192b4baa84d2af4ed`.

Signed upload intake accepts an optional complete capture envelope, checks the
existing owner/key/station/time receipt before reserving, durably binds it before
returning a PUT authorization and caps reservation/credential expiry. Opposite
session-to-capture and capture-to-session uniqueness prevents competing bindings.
Expired/non-ISSUED reservations cannot mint credentials; expired photo completion
cannot enqueue work or expose a READY reference. Ordinary byte retention remains
first receipt plus 24 hours; a reservation deadline never renews those bytes.

Each selected product is one immutable observation and validation job. The new
capture/product/condition unique key prevents new random ids from duplicating the
same reviewed fact; existing ids converge and changed capture timestamps conflict.
Ready owner evidence must resolve to the receipt's exact session. An exclusive
capture binding lane shares one object across distinct fuels; legacy objects retain
one-observation binding. Missing authority fails closed. Async validation checks
readiness, owner and expiry, then uses the capture lane. Bounded receipt cleanup and
rights erasure remove the ephemeral owner/key/station association.

## Immediate validation

- RED: expired reservation replay minted a credential; concurrent two-receipt
  binding yielded two winners. GREEN: both now refuse/converge correctly.
- New pure/HTTP tests cover complete envelope parsing, immutable time, unavailable
  authority, eligibility/binding refusal before presign, owner/key/station/session
  changes, deadline boundaries and expired completion/READY status.
- Real PostGIS tests cover exclusive binding races, reservation capture stripping,
  eight concurrent shared-object claims, legacy-lane refusal, two distinct fuel
  facts, eight row retries/one job, duplicate product/new id refusal, changed time,
  bounded expiry, owned erasure and migration ledger replay.
- `ANPFUEL_TEST_DATABASE_URL=<local disposable admin DSN> GOMAXPROCS=2 go test -p 2
  -race -count=1 -tags=integration ./internal/modules/community/...
  ./internal/modules/evidence/... ./cmd/api ./cmd/worker` passed. Every suite used
  isolated temporary databases. Actual API-process signed writes, nonce replay,
  IDOR and canonical money regressions passed. No private storage endpoint was
  provided: the actual S3/worker media branch remains NOT_EXECUTED.
- Affected `go vet` and `staticcheck`, `sqlc generate && sqlc vet` and OpenAPI vacuum
  passed (zero lint errors; 62 warnings/40 informs). `git diff --check` passed.
  The initial aggregate failed on an explicit missing test DSN and the old SQL
  mutation allowlist. The rerun supplied disposable infrastructure and pinned the
  new exclusive owner/capture/expiry guards; no failed test was skipped/weakened.

No historical photos/prices, precise fixes or training consent were uploaded.
No staging deployment, account proof acceptance, collection activation or release
certification is inferred. Current staging profile/media prerequisites remain
unaccepted. Next: transactional frozen Android review, signed PUT/complete/status
consumer, durable owner receipts/restart recovery; then consent and device acceptance.

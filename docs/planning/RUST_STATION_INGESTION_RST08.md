# RST-08 — Incremental discovery operations

Status: IMPLEMENTED — handler plus disabled schedule; no live fetch, no
Rust networking, no new credentials. Date: 2026-10-09. Scope:
`RUST_STATION_INGESTION_PLAN.md` RST-08. Language: English document; user
communication in Portuguese.

## 1. What was added

- `directory/adapters/jobs/discover.go` — `registry-discover` handler
  wrapping the existing bounded `Discover`: snapshot-scoped idempotent
  snapshots (complete snapshots replay without touching the network),
  terminal outcomes (complete/quarantined) completing the job, and
  transport/provider failures returning errors so the dispatcher backs
  off and eventually parks jobs dead. No Rust networking: Go owns fetch
  under the existing allowlisted discovery policy.
- Worker wiring (`cmd/worker/main.go`): handler registered with the
  production API config plus a daily `registry-discover-daily`
  schedule entry, disabled with reason exactly like the existing
  `registry-reconcile-daily` — the schedule exists, live firing waits
  for verified source access.

## 2. Evidence (revision `2ba0fc7` + working tree)

- Unit (loopback provider, replay-capable fake): bounded snapshot
  completes with one provider hit; replay fetches zero more;
  persistent 429/500 each stop at exactly the 3-attempt retry budget
  with the run failed; page-quota quarantine completes the job as a
  terminal data outcome; bad envelopes reject before any run.
- Real disposable PostGIS with `-race`, through the real queue and
  dispatcher: full cycle completes with 1 hit and empty backlog;
  re-enqueue converges with no refetch; crashed leases re-pool after
  TTL expiry and the snapshot still converges; closed-port outage with
  a single attempt dead-letters (0 queued/1 dead) while the run stays
  failed; persistent 429 re-queues with backoff (1 queued/0 dead)
  after exactly 3 hits.
- `go build ./...`, `go vet`, `gofmt` clean; directory unit suites
  green. Circuit behavior reuses the platform attempts/dead-letter
  path — no new state, no new table.

## 3. Limits and next task

- Live schedule firing stays disabled pending verified source access
  (same gate as reconcile-daily); enabling is a config decision with
  its own bounded live smoke, not a code change.
- Next (smallest): RST-09 city/profile handoff — source-separated
  profile projection through existing Directory/Profile ports with
  municipality/nearby pagination and a staged rollout/rollback
  runbook.

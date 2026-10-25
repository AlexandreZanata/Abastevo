# ESP-06 — Orchestrate bounded data-to-export intents

Status: LOCAL_DONE
Validation: PASS
Integration: INTEGRATION_PENDING
Date: 2026-10-24
Branch: `codex/enterprise-static-portal`
Parent: ESP-05 `46fee9e0`.

Rules: B-BR-ESP-10/11/13–15; BUC-ESP-02/06.
[Plan](../../../planning/ENTERPRISE_DATA_STATIC_PORTAL_PLAN.md#esp-06--orchestrate-bounded-data-to-export-intents).

## Before behavior

Official publication had no durable export intent. A new or corrected revision
would not trigger a candidate build; repeated notifications had no coalescing,
no single-active-build guard and no recovery after a lost notification. A
Directory identity/status change could also leave the last export stale with no
bounded reconciliation.

## Delivered

- **Directory export fingerprint** and **Official latest/current revision** ports:
  a cheap station+active-CNPJ hash and the newest current published survey
  revision. Both are owned read ports; no parallel data store.
- **Private workspace** (`enterpriseexport/adapters/jobs`): file lock
  (`flock`), atomic `state.json`, unique candidate directories and cleanup of
  stale `.export-*.tmp-*` directories. The state records the candidate path,
  export content hash, analytics hash, Directory fingerprint/station count,
  survey window and pinned Official revision IDs/checksums.
- **Build handler** (`enterprise-export-build`): acquires the single build lock,
  opens a read-only `REPEATABLE READ` transaction, resolves the newest Official
  window (current week plus configured prior weeks), coalesces unchanged work
  through the state, builds the export and `analytics-v1.json`, then writes the
  state atomically. A busy lock is a retryable error; a failed/cancelled build
  removes the candidate and leaves the previous state.
- **Reconcile handler** (`enterprise-export-reconcile`): compares the current
  Official pins and Directory fingerprint with the state and enqueues one build
  when stale/incomplete. Its dedupe key includes the schedule period, fingerprint
  and pins, so a changed Directory identity is not swallowed by a completed job
  and a dead build can be retried in a later bounded period.
- **After-publication intent:** `officialjobs.Import` has an optional
  `AfterPublish` hook. The worker wires it only when the feature is enabled and
  enqueues `enterprise-export-build`. A hook failure is visible to the job but
  never rolls back the already-published revision; reconciliation recovers the
  export.
- **Worker isolation and defaults:** `ANPFUEL_ENTERPRISE_EXPORT_ENABLED` defaults
  false; when enabled it requires an absolute `ANPFUEL_ENTERPRISE_EXPORT_ROOT`
  and a 1–12 week window (default 5). The build gets a dedicated serial
  dispatcher and is excluded from the general queue, so it cannot consume the
  safety/API slots. The 6-hour reconcile schedule is disabled unless explicitly
  enabled.
- **No migration:** the durable state is a private file on the configured
  workspace. This avoids taking a migration number while the map worktree holds
  an uncommitted `000122` claim; the state is not canonical data.

## Tests and actual results

Unit/race and static, from `backend/`:

```sh
go build ./...
go test -race -count=1 ./internal/modules/enterpriseexport/... \
  ./internal/modules/official/... ./internal/modules/directory/... \
  ./internal/platform/config ./cmd/worker
go vet ./internal/modules/enterpriseexport/... ./internal/modules/official/... \
  ./internal/modules/directory/... ./internal/platform/config ./cmd/worker
GOTOOLCHAIN=go1.27.1 staticcheck ./internal/modules/enterpriseexport/... \
  ./internal/modules/official/... ./internal/modules/directory/... \
  ./internal/platform/config ./cmd/worker
sqlc vet
sqlc generate   # byte-stable
gofmt -l ...
git diff --check
```

All PASS. New unit cases cover state validity/round-trip, exclusive lock,
stale-temp cleanup without deleting candidates, unique candidate paths, survey
window math, state matching (window/fingerprint/pins), revision pin pagination
and budget, config defaults/bounds/absolute root, and dispatcher isolation.

Real PostGIS, disposable local database:

```sh
go test -race -count=1 -tags=integration \
  ./internal/modules/enterpriseexport/... \
  ./internal/modules/official/adapters/jobs \
  ./internal/modules/official/adapters/read \
  ./internal/modules/directory/adapters/read
```

All PASS. `TestBuildCoalescesCorrectionsAndReconciles` proves: no state without
a published revision; first build creates one valid candidate/state/analytics;
an unchanged build is a no-op; a same-week correction creates exactly one new
candidate and updates the pinned revision; a held lock returns `ErrBuildBusy`;
a cancelled context leaves the state/candidate count unchanged; a current
reconcile does not enqueue; a Directory status change enqueues with a new
period/fingerprint key; a second status change produces a different key.
`TestImportCallsAfterPublishAndKeepsPublishedOnFailure` proves the hook runs once
on publication, not on a checksum no-op, and that a hook failure leaves the
published revision/current pointer intact for reconciliation.

## Limitations

- The file lock and state assume one host with a persistent private workspace;
  multi-host `flock` semantics are not proven. This is a VPS-single-host design
  until ESP-14/16 select and measure the deployment topology.
- The worker's dedicated dispatcher uses the existing 5-minute lease. The V1
  build is bounded (100k stations, 500 revisions, 500k prices, five weeks), but a
  lease expiry can retry the job. The file lock and state prevent duplicate
  promotion; no measured worst-case build duration exists yet (ESP-16).
- Analytics are computed in memory from the candidate JSONL; the 500k price cap
  bounds it. A larger national/history build needs a streaming analytics path and
  measured memory limits.
- The feature is default off. No schedule, source, public route, release pointer
  or deployment was activated. The candidate directory is private.
- Quality/enforcement/fine export intents are deferred to ESP-11.
- Next: ESP-07 — local data-foundation acceptance for the selected slices,
  including real-source fixtures, replay/correction/restore and the exact
  accepted editions.

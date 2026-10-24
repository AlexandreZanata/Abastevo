# MAP-02 — Existing electric runtime gaps

Status: LOCAL_DONE / INTEGRATION_PENDING
Validation: PASS (local source and disposable PostGIS)
Integration: INTEGRATION_PENDING

Tested source: `codex/map-ev-implementation` after MAP-01, tree
`0f7ad1fa` plus the changes below. Date: 2026-10-24.

## Behavior rules and use cases

- Charging community commands use free account sessions; forged, expired or
  foreign sessions never reach storage.
- Public charging history and tariff ranking remain anonymous and follow the
  existing `ANPFUEL_CHARGING_READS_ENABLED` gate (404 when disabled).
- Private ledger `charge` rows are owner-only, revision-guarded and idempotent.
  List and delete accept `charge` without changing refuel/expense filters,
  consent or report behavior.
- A charge tombstone is not listed; repeating the same delete is idempotent.
- OpenAPI must describe `charge` as an admitted ledger dataset, not only
  `refuel|expense`.

## Reproduction (RED)

The deployed test environment record
`docs/backend/evidence/TEST_ENV_UPDATE_2026-10-24.md` already documents
`POST /v1/charging/observations` and its owner-list/rank/history surfaces
answering 404, and `POST /v1/ledger/records` with `dataset=charge` answering
400. The source handlers existed but were not mounted; the ledger store admitted
charge writes but not reads/deletes.

Local RED before the fix:

- `TestChargeListDeleteOwnerIsolation` failed with
  `charge list [] ledger: invalid input`.
- The new composition test referenced a missing registration helper, so the
  charging command/history/rank routes had no composition-level guard.

## Implementation

- `backend/cmd/api/main.go`: one `registerChargingSurfaces` composition helper
  mounts the existing command, history and ranking handlers. Commands use
  `accountService.ValidateAccess`; history/ranking use the existing read gate.
- `history.go` / `rank.go`: optional `Enabled` gate returns 404 without touching
  storage, matching the existing charging read handler.
- `ledger/adapters/store.go`: `List`/`Delete` admit `charge` alongside
  `refuel|expense`; category filtering and unknown datasets remain refused.
- `contracts/openapi/v1.yaml`: `LedgerCharge`, `charge` dataset enums, consent
  max three and charge payload unions.
- Tests: composition/feature-gate tests in `cmd/api`, real-process route probes,
  disposable PostGIS owner/isolation/list/delete/tombstone/idempotency cases.

## Verification performed

Environment: Go `1.27.3`; disposable PostGIS
`postgres://anpfuel:***@127.0.0.1:5434/anpfuel`; no persistent/staging database.

```sh
GOTOOLCHAIN=go1.27.3 go test -race ./cmd/api ./internal/modules/charging/... ./internal/modules/ledger/...
# ok: cmd/api, charging http/jobs/licensed/prepared/domain, ledger adapters/http/domain

ANPFUEL_TEST_DATABASE_URL=<loopback disposable> \
GOTOOLCHAIN=go1.27.3 go test -race -tags=integration -count=1 \
  -run 'TestObservationCommandsAuthQuotaReplay|TestObservationCommunityVoteReportAndOwnerReads|TestChargeSessionCRUDRevisionIdempotency|TestChargeListDeleteOwnerIsolation|TestLedgerListFiltersPaginationAndDelete' \
  ./internal/modules/charging/adapters/http ./internal/modules/ledger/adapters
# ok: both packages

ANPFUEL_TEST_DATABASE_URL=<loopback disposable> \
GOTOOLCHAIN=go1.27.3 go test -tags=integration -run '^TestPublicProcessIdentityAndSignedWrites$' -count=1 -v ./cmd/api
# PASS: real API process; rank 400 (mounted), history 200, forged command 401

GOTOOLCHAIN=go1.27.3 go vet ./cmd/api ./internal/modules/charging/... ./internal/modules/ledger/...
GOTOOLCHAIN=go1.27.3 go test ./internal/platform/apicontract/...
vacuum lint -r contracts/openapi/vacuum-rules.yaml contracts/openapi/v1.yaml --no-update-check
# Passed with existing 489 warnings/106 informs, no errors
gofmt -l cmd internal/modules/charging internal/modules/ledger   # empty
git diff --check                                                # empty
```

## Limitations and state separation

- The test environment still runs deployed source `53719336`; this source fix
  is not deployed and the recorded live 404/400 remain until a separately
  authorized deployment.
- Kotlin ledger code already emits `charge`; no Android source change was needed
  in this task. Device/manual verification remains OWED.
- EE-22/24/25/26–31 external/source obligations remain as recorded; this task
  does not re-certify the whole EV plan.
- No national coverage, production, release or public pilot claim.

Next: MAP-03 map artifact and cross-language contracts.

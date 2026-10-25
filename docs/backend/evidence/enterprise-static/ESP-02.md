# ESP-02 — Close bounded discovery and correction gaps

Status: LOCAL_DONE
Validation: PASS
Integration: INTEGRATION_PENDING
Date: 2026-10-24
Branch: `codex/enterprise-static-portal`
Parent: ESP-01 `f1ec13ad`.

Rules: B-BR-ESP-02/09/10/15; BUC-ESP-01/02.
[Plan](../../../planning/ENTERPRISE_DATA_STATIC_PORTAL_PLAN.md#esp-02--close-bounded-discovery-and-correction-gaps).
[Admission register](../../../product/ENTERPRISE_SOURCE_ADMISSION.md#s-esp-02--anp-weekly-prices-and-historical-series-reuse-official).

## Before behavior

The existing `official` discovery had three concrete gaps:

1. It built only `revendas_lpc_YYYY-MM-DD_YYYY-MM-DD.xlsx` and
   `resumo_semanal_lpc_YYYY-MM-DD_YYYY-MM-DD.xlsx`. The ANP page currently
   lists the newest week as `..._2026_10_11-2026_10_17.xlsx`, so the newest
   survey was undiscoverable.
2. Without an ETag it deduped on the URL alone, so replaced bytes were never
   re-imported by the scheduled run. The source does not expose a reliable
   validator to trust blindly.
3. A later correction for an older week superseded whichever revision had
   published most recently, and `StationCurrentPrices` selected the revision
   with the newest `published_at`. An old correction could therefore replace
   newer prices as the station's "current" value.

## Delivered

- `source.ProbeContent` hashes the bounded response body, returns the SHA-256,
  maps 404 to `ErrNotFound`, keeps 429/5xx as retryable `ErrBadStatus`, and
  rejects a body above `MaxBytes`. `Probe` remains as a validator-only wrapper.
- `official` discovery accepts the current filename shape first and the legacy
  shape second, stopping at the first existing resource. The survey window still
  comes from the verified filename.
- Discovery dedupes by `URL + sha256` instead of `URL + ETag`. Unchanged bytes
  converge on the existing job; replaced bytes create one new job even when the
  URL and ETag are unchanged. The import fetches the bytes and treats an
  identical checksum as a no-op.
- The scheduled run keeps the existing current+previous week window (two weeks),
  so no daily source-load increase. An operator correction sweep may set
  `{"weeks":N}` in the job payload; N is clamped to five. Older corrections
  remain a controlled backfill.
- `BeginRun` supersedes the latest published revision **for the same survey
  window**, never a newer week. `FinishRun` compares a first publication with
  the previous chronological week, not the most recently published revision.
- `StationCurrentPrices` resolves the latest survey window that contains the
  station, then reads that window's current pointer. An older corrected week is
  still visible in history but cannot become the station's current price.

No migration, dependency, source switch or new schedule was added.

## Tests and actual results

Unit/race, from `backend/`:

```sh
go test -race -count=1 ./internal/modules/official/adapters/jobs ./internal/modules/official/adapters/source
go test -race -count=1 ./internal/modules/official/...
go build ./...
go vet ./internal/modules/official/...
GOTOOLCHAIN=go1.27.1 staticcheck ./internal/modules/official/...
sqlc vet
gofmt -l internal/modules/official db/queries/official
git diff --check
```

All PASS. New cases cover current/legacy filename variants, same content with
unchanged and deliberately unreliable ETags, replaced bytes, one job per change,
404 as unpublished, 429 as retryable, five-week inclusion/exclusion, the
five-week clamp, bad payload refusal, body hash/limit/404/429/304 in
`ProbeContent`, and the existing parser/import regression suites.

Real PostGIS, disposable local database (`ANPFUEL_TEST_DATABASE_URL` on
`127.0.0.1:5434`):

```sh
go test -race -count=1 -tags=integration \
  ./internal/modules/official/adapters \
  ./internal/modules/official/adapters/read \
  ./internal/modules/official/adapters/jobs
```

All PASS. `TestOlderWeekCorrectionKeepsNewerWindowCurrent` publishes week 1,
week 2 and then a week-1 correction. It proves the week-1 pointer moves to the
correction, the week-2 pointer is untouched, the correction's
`supersedes_revision_id` is the week-1 original (not week 2), and
`StationCurrentPrices` returns the week-2 amount. Existing same-week correction,
row-drop/review, history pagination and end-to-end import tests remain green.

`sqlc generate` is byte-stable across a repeat run. `git diff --check` is clean
and the scoped secret/PII scan found nothing.

## Limitations

- No live ANP workbook was downloaded or written to the database. The newest
  filename and page state come from the ESP-01 bounded page inspection; the
  deterministic tests use synthetic bytes and a TLS fixture. A bounded live
  source smoke remains an operator action and is not claimed here.
- The correction sweep is not scheduled or activated. The daily schedule keeps
  its previous two-week scope; `weeks` is an explicit operator payload.
- The content probe downloads the body instead of trusting the ETag. That is
  intentional for an unreliable validator and does not increase the default
  request count versus the previous no-ETag path. The five-week sweep is
  operator-triggered and capped.
- No source artifact, schedule, deployment or public output was enabled.
- Next: ESP-03 — freeze module-owned normalized export snapshots from the
  accepted Official/Directory/Quality read contracts, with real PostGIS
  concurrency/correction tests.

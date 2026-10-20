# FQ-07R5E — Bounded seven-resource fine worker

Status: LOCAL_DONE
Validation: PASS

Parent 08bef884, maintained dev. B-BR-FQ-01/02/06/07/14/16;
BUC-FQ-03/04/06. One runtime composition checkpoint; no activation.

Before behavior: admit only the seven actually verified exact official resources,
including one grouped2016–2019 CSV and2021 CSV served under .xlsx. Period comes
from the verified resource profile, never an invented annual URL/anchor label.
Discovery walks one supported href per existing durable cursor step; no reset of
dead jobs. Preserve2025 typed/v1 identities and dispatch the six historical
profiles through their separate trusted native CLI/manifest/application publisher.

Use the same pinned private source descriptor/copy/hash-before-and-after checks,
trusted immutable executable, minimal process environment, bounded time/bytes/
artifacts and confirmed cleanup on every outcome. Raw name/CPF artifacts must be
removed before publication. Hold the shared separate-connection source lease for
the whole fetch/prepare/cleanup/publish/watermark/recompute attempt. Strict payload
and profile-specific ports fail before I/O. Superseded replay/acknowledgment loss
converge without changing pointers or budgets. No queue path chooses a binary or
filesystem path. Keep both existing import/discovery switches default off.

Ship the static locked history CLI only in the nonroot worker image and reproduce
its build/env in affected CI. Immediate trust/cancel/tamper/cleanup/timeout/schema/
profile negative tests, actual Rust→worker→PostGIS correction/replay/ack recovery,
complete seven-resource discovery/cursor/disabled claims and actual image tests
are mandatory. No new dependency, station association, D, deployment or native-
device/production acceptance. Live FQ-06/Android/FQ-16/17 remain owed.


## Delivered and verified, 2026-10-20

Exact source URL admission covers seven inspected resources only: grouped2016–2019,
annual2020,2022–2025 and the2021 CSV actually named multas-2021-junho22.xlsx.
Unsupported years/alternate formats/query/fragments/period mismatches refuse before
fetch. FineRef now carries Period; no grouped source is split into invented years.
Discovery still inspects exactly one href per durable cursor step, retains lost-
ack/correction/concurrency recovery and preserves completed/dead job budgets.
A seven-resource/two-cycle real queue test proves exact payload periods and no
starvation/duplicate enqueue/dead-attempt reset. Existing default-off reserved
claim/scheduling tests remain green; no source control was enabled.

FineHistoryRunner shares the proven trusted-executable/private-descriptor/copy/
before-and-after digest/process/deadline/artifact/cleanup boundary with2025, then
loads only the separate strict history manifest. The artifact opener now admits
history.jsonl/source_notes.jsonl as required, while loaders retain exact stream
contracts. Nine historical failure modes cover timeout, snapshot mutation, process
output redaction, writable binary, symlink, identity, cancellation, wrong profile
and invalid manifest. The2025 trust/negative regression matrix remains green.

FineFileImport uses profile-specific required preparation ports before I/O and
builds a validated typed publication closure under the same whole-attempt source
lease. Confirmed raw-file cleanup precedes publication; source verification,
watermark/recompute acknowledgment and superseded replay remain common. Actual
Rust→PostGIS recovery runs for both2025 and2024: cleanup refusal publishes zero,
watermark failure leaves one replayable revision, retries converge, correction
freezes old facts, changed bytes refuse and superseded input cannot resurrect.
Fetch/prepare/tamper/cancel/cleanup failure tests run for both contracts.

The actual worker factory/discovery/dispatcher→Rust→PostGIS→public read journey
passes for2025,2024,2021 and the grouped source, with correction/replay/source
cleanup and zero inferred station links. CI and backend README build/chmod/export
the history preparer alongside existing binaries. No dependency was added.

Manual private whole-source worker acceptance (qualitysource tag) passes for all
seven pinned candidates: real trusted runner, typed file job, source lease,
publication/replay, watermark/recompute and complete public keyset traversal.
35,911 source records =35,612 accepted observations +293 CPF quarantine records
+6 source metadata records; zero duplicates, seven revisions, one watermark,
seven derived intents, zero station links, no retained raw artifacts. Original
DAY/MONTH clocks and exact source counters persist. Final PASS55.989s. An initial
manual traversal harness incorrectly supplied the joined continuation as one
keyset component; that owned attempt was terminated and its scratch DB removed.
The corrected harness decomposes both components and rejects repeated event IDs;
final complete acceptance passes. No product query was relaxed.

Live official fetch boundary, 2026-10-20: seven actual hrefs remain listed. Exact
pinned2025 source450,504bytes/SHA277f6cf5… reconciles3,395=3,375+20CPF;
misnamed2021 source720,842bytes/SHA85122d0d… reconciles4,976=4,929+44CPF+3metadata.
Both traverse the production secure fetch/private cleanup and actual native runner.
Final network test PASS33.328s. Full immutable hashes remain in source tests and
preceding source evidence; raw files/rows are excluded from Git and logs.

Local worker image build PASS, immutable image ID
sha256:668ebd17f63a719dc3e4881b5f969c2fbea28617a5ed997317e99d2b6ab0a2e4.
The pinned musl build ships /prepare_fine_history as static ELF alongside existing
preparers. Actual synthetic source mode600/owner65532 ran under configured
65532:65532 with network none; exact one-row minimized history artifacts and source
clock verified; disposable container removed. No registry push or deployment.

## Commands and results

From backend/, GOCACHE=/tmp/fq-completion-go-cache, private preparer environment
variables point at trusted755 local debug binaries. ANPFUEL_TEST_DATABASE_URL uses
only loopback5434 disposable PostGIS databases. Required variables/reproducer are
in backend/README.md; no credentials beyond the committed local fixture are needed.

- go test -race -tags=integration ./internal/modules/quality/... ./cmd/worker
  ./internal/platform/jobs ./internal/platform/config -count=1:11 packages PASS
  (adapters54.714s,http23.268s,jobs19.925s,native1.212s,prepared1.132s,read11.270s,
  application/domain1.018s,worker8.687s,platform jobs13.950s,config1.011s).
- go test -race -tags=integration,qualitysource ./cmd/worker
  -run TestOfficialSevenFileWorkerPipeline -count=1 -v:PASS55.989s.
- Same tags, -run 'TestOfficial(Historical)?FineWorkerNetworkBoundary':PASS33.328s.
- go build ./...; affected go vet and staticcheck:PASS.
- Actual docker build --target worker plus nonroot/network-none CLI smoke:PASS.
- Final affected unit/format/diff/link/YAML/secret review:PASS, recorded at closure.

INTEGRATION_PENDING. No runtime activation/current staging schema acceptance,
production/device certification, safe Directory station links, FQ-06 actual XLSX,
full linked Android journeys, FQ-16/17 or final protected merge/wiki is claimed.
D remains held. Next bounded task: safe current Directory company-observation
association without treating fine due/source-extraction dates as incident/ownership
dates; then remaining source/app/acceptance and final protected delivery.

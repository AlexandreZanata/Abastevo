# FQ-05W7 — Bounded live PMQC discovery and non-starving enqueue

Status: LOCAL_DONE
Validation: PASS

Completed 2026-10-17 on maintained dev, parent 11d7a2b0 (FQ-05W6). The tested
behavior is the W7 listing/discovery/cursor/enqueue-budget changes in this
atomic checkpoint; delivery remains INTEGRATION_PENDING. No schedule
registration, import activation, deployment or live network call claimed.

Before behavior: B-BR-FQ-03/04/05/13/16, BUC-FQ-01/02/03. Discovery plans
from a verified period inventory, inspects at most one listed archive per
run through the trusted HTTPS fetch and native member inventory, and walks
the listing with a durable cursor kept separate from completed-import
watermarks. Periods come only from verified archive hrefs: anchor text can
never invent a missing month, duplicate hrefs collapse to one archive, and
annual archives yield their monthly CSV members as separate editions.
Created jobs alone charge the per-run budget, so repeated runs make forward
progress through later members instead of re-charging deduplicated ones.

## Delivered and verified

- `native.ParseListingPage` (pure, stdlib-only) accepts only hrefs that pass
  the existing archive URL contract (host, prefix, `pmqc_*.zip` shape,
  no query/fragment), derives the period from the filename, resolves
  relative hrefs against the verified page URL, dedupes by URL, and refuses
  a page that yields no archive or exceeds the 4 MiB listing bound. The
  historical February-anchor-to-July-file case resolves to exactly one
  2026-07 archive and never fabricates 2026-02.
- Migration 104 adds `quality_discovery_cursors`: operational scan
  progress separate from watermarks, publication keys and lease clocks.
  The live handler inspects the first listed archive after the durable
  cursor and wraps only after the whole listing was covered.
- `LiveDiscovery` (kind `quality-discovery`) fetches the verified page,
  inspects exactly one archive per run, plans its members against the
  ledger, and enqueues frozen-provenance imports. The cursor advances only
  after the inspected archive was fully planned; if a defensive enqueue cap
  is hit, the same archive is re-inspected next run, where converged jobs do
  not count and the remaining members flow.
- The pre-existing injected-lister `Discovery` handler now consumes the
  platform created/existing result and charges only created jobs. A real
  queue test with a two-job budget covers six editions in three runs.
- Real-PostGIS acceptance: one archive per run with a held cursor and
  created-only budget (three members, cap two), the third member never
  starves, following archives are each visited once, wrap converges without
  forks, a recorded member replays stable, republished bytes create exactly
  one correction job pinning the new hash, a dead converged job never forks
  and the audited operator replay recovers it in place.
- Worker composition (imports enabled) wires the verified page fetch (owned
  HTTPS Fetcher entry: `www.gov.br`, dados-abertos prefix, listing kind,
  bounded bytes/redirects), one bounded archive inspection through the same
  fetch policy plus `native.Inventory`, and the shared platform enqueue used
  by revalidation. `quality-discovery` gains its own fixed slot alongside
  import and revalidation; general claims still exclude all three source
  kinds.

Named limits: each archive is re-downloaded when its turn returns to the
cursor (bounded to one archive per run; that is also how silent content
changes are noticed), discovery has no schedule yet (FQ-05W8 owns explicit
scheduling and image proof), dead discovery jobs are recovered through the
existing audited operator `ReplayDead` path rather than automatic resets,
and no live ANP page was fetched in this checkpoint: the parser is tested
against the page shape verified live in FQ-00, while the real fetch path is
the already-tested Fetcher plus archive inventory.

Commands from backend/:

```sh
ANPFUEL_TEST_QUALITY_PREPARER_PATH=/tmp/fq-native-rust/debug/prepare_quality ANPFUEL_TEST_PREPARER_PATH=/tmp/fq-native-rust/debug/prepare_registry GOCACHE=/tmp/fq-completion-go-cache go test -count=1 -race -tags=integration ./internal/modules/quality/... ./cmd/worker ./internal/modules/directory/adapters
GOCACHE=/tmp/fq-completion-go-cache go test -count=1 ./internal/modules/quality/adapters/native/
GOCACHE=/tmp/fq-completion-go-cache go build ./...
GOCACHE=/tmp/fq-completion-go-cache go vet -tags integration ./cmd/worker ./internal/modules/quality/... ./internal/modules/directory/...
XDG_CACHE_HOME=/tmp/fq-completion-staticcheck GOTOOLCHAIN=go1.27.1 GOCACHE=/tmp/fq-completion-static-cache staticcheck ./cmd/worker ./internal/modules/quality/...
sqlc vet
sqlc generate
```

PASS: quality adapters 27.1s, quality http 13.3s, quality jobs 11.6s (all
with race detection), native unit 0.1s, prepared/application/domain ~1.0s,
quality read 9.3s, cmd/worker 3.7s, Directory adapters 17.5s, all with race
detection. vet/build/staticcheck/sqlc PASS; generated files hashed
before/after `sqlc generate`: no drift. Four migration-head assertions were
updated for 000104 (000099–104, 000101–104, 000100–104, 000102–104 plus the
dynamic 000097+ list). gofmt, `git diff --check` and
`scripts/scan-secrets.sh` PASS; the diff carries only synthetic URLs,
fixtures and hashes. No new dependency.

## Explicit next obligations

FQ-05W8: package the pinned Rust `prepare_quality` executable in the worker
image (mode 755, distroless/static nonroot, narrow `.dockerignore` source
allowances, locked/reviewed dependencies) and add bounded explicit source
scheduling/configuration without activating unrelated ANP/registry imports;
reconcile FQ-06/07 runtime publication/discovery. Then FQ-12–17 and the
final protected dev→main batch; D is NOT_AUTHORIZED.

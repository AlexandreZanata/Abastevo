# ESP-13 — Verify reproducible versioned publication artifacts

Status: LOCAL_DONE
Validation: PASS
Integration: INTEGRATION_PENDING
Date: 2026-10-24
Branch: `codex/enterprise-static-portal`
Parent: ESP-11 `af615284`

Rules: B-BR-ESP-03/11/13/14; BUC-ESP-04/05.
[Plan](../../../planning/ENTERPRISE_DATA_STATIC_PORTAL_PLAN.md#esp-13--verify-reproducible-versioned-publication-artifacts).
[ESP-11 event-level official pages](ESP-11.md).

Exit: **ESP-STATIC-LOCAL_READY** for the artifact structure and the selected
event-level official slice. Real price/catalog publication remains blocked by
ESP-07 over-precision and ESP-01 rights; it is not claimed here.

## Delivered

- `portalgen` artifacts now use `enterprise-portal-v2`:
  - the manifest content digest binds the complete file list **and** the frozen
    official pin, so a changed source/revision/lease/census cannot hide behind
    unchanged HTML;
  - CSS and optional search JavaScript are content-addressed
    (`assets/portal.<8hex>.css|js`), preventing a browser from mixing an old
    page with new styling/code;
  - an indexable build requires a validated public origin and writes
    `sitemap.xml` plus a matching `Sitemap:` line in `robots.txt`;
  - noindex candidate builds still emit `Disallow: /`, no sitemap and no
    indexable page.
- `portalgen.Verify` checks one complete artifact without database/API access:
  - manifest schema/digest, sorted unique file list, exact on-disk file set,
    SHA-256/size per file, page count and private file modes;
  - HTML/JS gzip budgets (100 KiB each), regional search-shard gzip budget
    (250 KiB), page/file/byte budgets (50,000 pages, pages+1,000 files, 2 GiB);
  - sitemap XML shape, absolute origin, URL count/bytes, lastmod, no duplicate
    URLs, every sitemap URL resolves, and every indexable page appears exactly
    once (noindex pages are excluded);
  - content-addressed CSS/JS names and hashes, immutable search release paths,
    and no mixed-version release reference;
  - public-field allowlists for JSON keys, search-shard keys and CSV headers;
    CPF-shaped values, private keys, raw scripts/attributes and broken internal
    links fail;
  - official pin/source/census/page count and per-record station-link safety
    (`linked` may link; `unmatched`/`ambiguous` never link) plus the
    negative-certificate/limitation wording;
  - optional `--compare` second build with byte-identical file digests.
- `backend/cmd/portalcheck` is the operator check command. `portalgen` gained
  `--site-url`; `--indexable` now refuses a missing/invalid origin.

## Real artifact acceptance

The selected official slice is the real FQ-07/FQ-08 2025 export already present
under `/tmp/fq-portal-exports` (private, outside Git): 3,375 rows, revision
`fc18397c-cf1c-41aa-a155-a3d5c4bbfa5c`, lease
`a18283c151938ddbb574ddbd897629cda050c887fefbc8ce81e0dd6c150a88`, source
checksum `277f6cf5…91fe96`, payload SHA
`efe058ca7025094c73f3110a8339648a2919f67a09a8db3ef198668f15abc930`, census
`3,375 unmatched / 0 linked / 0 ambiguous`. Revision/payload IDs are run-local
(the disposable acceptance database was recreated); the source checksum and
counts are the fixed FQ acceptance.

The station/price side remains the existing seven-station synthetic fixture
because real prices are still blocked by ESP-07/rights. This proves the combined
artifact machinery and the real official slice, not a real price publication.

Two private candidate artifacts were built under `/tmp/fq-portal-ready`:

| Artifact | Mode | Pages | Files | Bytes | Content SHA-256 |
| --- | --- | ---: | ---: | ---: | --- |
| `real-2025-portal` | noindex preview | 3,409 | 3,419 | 15,231,739 | `sha256:bd6649fd5f9c1a3be5972a07cc5cfd1dc07329c681cedb39e3ae32ab510002f2` |
| `real-2025-portal-full` | indexable + real search JS | 3,409 | 3,421 | 15,932,636 | `sha256:75bc8a1a21702982ebf651aa2c14b8e8b1cd9005c7e68dbea7f80f80fd935682` |

The full artifact adds `sitemap.xml` (3,408 URLs, 466,272 bytes) and the compiled
search script. Its assets are:

```text
assets/portal.fab88ae7.css   5,466 bytes  SHA-256 fab88ae7…
assets/portal.2828d19c.js    9,404 bytes  SHA-256 2828d19c…
```

`robots.txt` in the full artifact:

```text
User-agent: *
Allow: /
Sitemap: https://portal.example/sitemap.xml
```

Sitemap URLs use the example origin `https://portal.example`, include the
official index/detail pages, exclude `404.html` and any noindex page, and every
URL maps to an existing page. The noindex artifact has no sitemap.

Actual measured budgets:

| Check | Budget | Actual |
| --- | ---: | ---: |
| Largest HTML gzip | 100 KiB | 10,023 bytes |
| Largest script gzip | 100 KiB | 3,030 bytes |
| Largest CSS gzip | measured | 1,820 bytes |
| Largest regional shard gzip | 250 KiB | 376 bytes (fixture); 23,639 bytes (6,000-station test) |
| Initial view gzip (HTML + CSS + JS) | measured | 6,678 bytes |
| Sitemap | 50,000 URLs / 50 MiB | 3,408 URLs / 466,272 bytes |
| Pages / files / bytes | 50,000 / pages+1,000 / 2 GiB | 3,409 / 3,421 / 15.9 MB |

`TestRegionalSearchShardIsBounded` independently measured the 6,000-station
regional shard at 346,084 bytes / 23,639 gzip (national comparison 1,020,084 /
70,029 gzip), below the 250 KiB budget.

## Reproducibility and negative coverage

- Two full builds from the same snapshot were byte-identical:
  `portalcheck --artifact real-2025-portal-full --compare
  real-2025-portal-full-2 --site-url https://portal.example` passed with the same
  `sha256:75bc8a1a…` and all 3,421 file digests equal.
- `TestVerifyReproducibleBuildsAndTampering` proves a changed comparison file is
  refused.
- Tamper matrix: changed file hash, extra file, symlink, HTML/file/byte budgets,
  private JSON key, CPF-shaped value, CSV header, broken internal link,
  non-content-addressed CSS, mixed `data-release`, missing sitemap/sitemap
  mismatch and indexable-without-origin are refused.
- Hostile source text (`<script>`, `<img onerror>`, `javascript:`) injected into
  station and official fields is HTML-escaped; the verifier passes the page and
  refuses any unexpected raw script/attribute.
- Official page checks prove 3,375 record pages, 17 index pages, the
  negative-certificate/limitation wording, and no station link for the all-
  unmatched real edition.

## Commands and actual results

From `backend/`, GOCACHE `/tmp/fq-portal-ready/go-cache`:

```sh
go build ./...
go test -race -count=1 ./internal/modules/enterpriseexport/...
go test -race -count=1 -tags=integration ./internal/modules/enterpriseexport/...
```

PASS: unit package run 1.016/1.021/1.011/4.099 s; integration
1.711/2.409/1.014/4.261 s (enterpriseexport, adapters/jobs, analytics,
portalgen).

Real official snapshot (real FQ export + synthetic station/price fixture;
indexable + real search JS):

```sh
ANPFUEL_TEST_REAL_OFFICIAL_DIR=/tmp/fq-portal-exports \
ANPFUEL_TEST_PORTAL_INDEXABLE=1 \
ANPFUEL_TEST_PORTAL_SITE_URL=https://portal.example \
ANPFUEL_TEST_PORTAL_SEARCH_JS=landing/dist/assets/search.2828d19c.js \
go test -race -count=1 -v ./internal/modules/enterpriseexport/portalgen \
  -run 'TestVerify|TestHostile|TestGenerate|TestOfficial|TestSearch|TestDownloads|TestInsights|TestReal'
```

PASS 42.465 s. The real test reported 3,409 pages / 3,421 files / 15,932,636
bytes / `sha256:75bc8a1a…`, html gzip 10,023, shard gzip 376, initial 6,678.

Operator checks on the retained private artifacts:

```sh
go run ./cmd/portalcheck --artifact /tmp/fq-portal-ready/real-2025-portal
go run ./cmd/portalcheck --artifact /tmp/fq-portal-ready/real-2025-portal-full \
  --site-url https://portal.example
go run ./cmd/portalcheck --artifact /tmp/fq-portal-ready/real-2025-portal-full \
  --compare /tmp/fq-portal-ready/real-2025-portal-full-2 \
  --site-url https://portal.example
```

All PASS; the noindex check reported `sitemap=0`, and the full check reported
`sitemap=3408, gzip(html=10023 js=3030 css=1820 shard=376 initial=6678)`.

Static:

```sh
go vet ./internal/modules/enterpriseexport/... ./cmd/portalgen ./cmd/portalcheck
GOTOOLCHAIN=go1.27.1 staticcheck ./internal/modules/enterpriseexport/... ./cmd/portalgen ./cmd/portalcheck
sqlc vet
sqlc generate          # stable, no query change
gofmt -l <changed files>
git diff --check
```

All PASS. No credential, PII, raw source row or generated artifact was
committed.

## Limitations and OWED

- **Real prices/catalog remain blocked** by ESP-07 over-precision and ESP-01
  rights; the real artifact uses the existing synthetic station/price fixture
  plus the real official 2025 slice. No price publication is claimed.
- The FQ portal snapshot is one current edition (2025), not the complete
  2016–2025 multi-edition history; a multi-edition export remains OWED.
- Fine association live revalidation/rebound remains OWED (FQ-07R6D); the
  official snapshot is frozen at revision `fc18397c…` and lease `a18283…`.
- The sitemap origin is an example; the actual host/topology and precompressed
  serving are ESP-14. No VPS, DNS, Caddy, Compose or public route was changed.
- The verifier does not run a real browser or device; mixed-version safety is
  proved by content-addressed assets/immutable release paths and static checks.
  Browser/performance acceptance remains ESP-16/17.
- `enterprise-portal-v1` artifacts are not accepted by the new verifier; a v2
  artifact must be rebuilt. The generator now emits v2.
- No `main` merge, PR, production activation or publication occurred.

## Next

ESP-14 — scoped static hosting and safe promotion of this verified candidate.
No hosting or release switch is implemented here.

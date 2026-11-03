# EVC-01 — National source pins and bounded preparation

Status: IN_PROGRESS

Bounds declared before behavior: EVC-01A implements tested local acquisition
and provenance; EVC-01B versions national OSM resource limits and per-UF census;
EVC-01C performs approved real acquisition/preparation and records actual pins.
Finish all before EVC-02. Source-code completion is not national data acceptance.

B-BR-EV-SRC-01A: Use fixed publisher HTTPS origins, country BR and admitted
OCM providers only, exact byte/record/page budgets and immutable source hashes.
API keys enter through a private file or environment/header, never URL, Git,
stdout or exception bodies. Raw source material stays in an owner-only directory
outside the checkout. No public source request is added to production services.
A failed/truncated/changed response preserves prior output and removes only
this attempt's partial file. National processing limits must be explicit and
versioned, with no silent cap increase/truncation.

BUC-EVC-01A: Prepare an auditable download/collection command, verify pin headers
and source bytes, refuse redirects/foreign origins/unbounded responses, record
source timestamps/rights and counts without contacts/comments/media in logs.
Use synthetic transports for failure/secret-surface tests. Ask owner before
national acquisition; ask separately before a DB import or staging action.

## Read-only preflight

2026-11-03: Geofabrik Brazil HEAD reports 2,102,091,708 bytes, Last-Modified
Tue, 03 Nov 2026 01:53:43 GMT and ETag "7d4b5fbc-65f5588430a54". No PBF bytes
were downloaded. Publisher URL:
https://download.geofabrik.de/south-america/brazil-latest.osm.pbf.
Local available /tmp space was 42.0 GiB; physical RAM 31.0 GiB (16 CPUs).
These are preflight observations, not a successful national parser measurement.

OCM export main still resolves to 8e3bedca48ca94807d96b2f5e7ee02cffe63ae54,
committed 2026-04-22T10:59:34Z. Do not label the Oct-2026 profile name as the
source's update date. The owner authorized use of an existing OCM key and
local Brazil-only collection; no key has been located/provided yet. Account/
new-key creation was not authorized by that existing-key answer.

Only publisher reference metadata (no national POI extract) was read locally:
563,770 bytes, SHA-256 71c7ea6f70e8f360ff13cc64ada578d07c05af0bf9e92b5efeda591ff695f7e8,
Git blob 715fc4b47040bd8dcf6e63dd20fce13073053e46, matching the historical
reference pin. Re-hashing the exact provider license strings verifies the
first two hashes and yields provider 57's correct 64-character digest:
cd754a95896760f420a2eacb8508e6dfc8f7764c6f280e84365a7076218b3944.
The old profile has 47 characters. Preserve old artifact identity and correct
this in a versioned admission profile after the source collection is accepted.

## EVC-01A acquisition-code checkpoint

Status: LOCAL_DONE (code only)
Validation: PASS

Tested parent 5f1797b6 plus this child. Five stdlib unittest groups cover pin
changes, truncated/oversize bytes, prior output preservation, private file mode,
HTTPS origins/redirect refusal, header-only key use, changed provider rights,
country/provider scope, duplicate/stale pagination, page/cumulative caps and
main/linked-checkout output refusal. The primary/linked repository guard was
strengthened after RED exposed the occupied primary checkout boundary. Syntax,
diff and secret-surface checks pass; no dependency added. Raw payloads are
private acquisition inputs, not product artifacts or Git fixtures.

The owner specifically authorized the 2,102,091,708-byte Brazil PBF download
to /tmp/abastevo-evc01-osm-20261103. This covers acquisition only, not a DB import
or staging mutation. Existing-key OCM collection was authorized separately;
the owner has not yet supplied/created a key. Code preparation proceeds while
that credential input remains pending. EVC-01 as a whole remains IN_PROGRESS.

### Publisher alias retry

The approved PBF attempt refused Geofabrik's HTTP 307 latest alias without
downloading data. Metadata identified the same-origin Brazil-261102 daily
file. RED/GREEN added the narrow latest→dated-Brazil redirect rule; every OCM,
foreign-origin, other-country or further redirect stays refused. Header pin
checks still run on the final response and the resolved URL is recorded. Six
acquisition test groups pass; no change to the authorized byte cap/scope.

### Administrative boundaries

The owner separately approved the approximately 4 MB IBGE municipal malha for
local UF classification. The source collector now has an 8 MiB, 6,000-feature
country-scoped GeoJSON path; synthetic shape/identity, private-output and receipt
tests pass. No boundary bytes have been used in a claimed national census yet.
Seven acquisition test groups run in the Rust fast gate with only synthetic
transports, no live network. The checkout test handles both normal and linked
checkouts. IBGE CC BY 4.0 evidence: [technical/legal note](https://www.ibge.gov.br/biblioteca/visualizacao/livros/liv102152.pdf).

## EVC-01B opening

B-BR-EV-SRC-01B: National parsing uses a frozen opt-in profile, refuses bbox/
limit overrides and records its profile hash. Raw regional output remains
compatible. Country/UF counts are object/site units, never connector totals.
Municipality polygons determine UF without fabricating a name/address; holes,
exclaves, overlapping/border ambiguity and missing geometry must remain honest.
Only uniquely classified points receive a UF; unassigned records are counted.

BUC-EVC-01B: Reuse the bounded PBF reader and its per-blob index/cache; apply
explicit input/candidate/tag, memory/file/disk/wall-time ceilings. Emit a
reproducible per-UF census tied to the PBF and IBGE byte hashes, and a separate
measured execution receipt. No national file runs before local negative,
geometry and deterministic-fixture tests. No database access or public runtime
source queries. Raw preparation candidates are not yet public eligible catalog
acceptance; EVC-02 owns access/status normalization and private/disused exclusion.

### EVC-01B code checkpoint

Status: LOCAL_DONE (code only)
Validation: PASS

Tested parent 188e7bea plus this child. RED first failed on absent geography,
national profile and resource runner modules. GREEN: full Rust fast gate passes
(fmt, clippy all targets with denied warnings, locked suite) and ten offline
Python test groups. Synthetic end-to-end CLI runs are byte-identical including
profile and source/boundary hashes, 27 UF buckets plus UNASSIGNED, separate
fuel/charging/combined object units and ambiguity reasons. Unknown profiles,
overrides and existing output are refused before parsing; last good output is
preserved. Polygon holes/exclaves, overlaps, borders and bad identities/points
are covered. Legacy regional output remains unchanged by optional fields.

The Linux stdlib runner loads the frozen profile, fences parser address space,
CPU and per-file size, monitors wall time/owned working disk and kills only its
own process group on failure. Tests exercise nonzero exit, timeout, memory/file/
disk refusal, private output and symlink refusal. It measures peak RSS/elapsed
separately from deterministic artifacts. No dependency was added. The national
CLI's cooperative deadline supplements this runner; use the runner for actual
national inputs, never the bare CLI as resource-acceptance evidence. No DB or
staging operation is part of this checkpoint.

EVC-01C remains IN_PROGRESS. Owner-approved downloads completed: OSM PBF
2,102,091,708 bytes, SHA-256
176a899d86f1bf2f77964577c482c119a3dcc772b8ee214e78387a5dfe4d3366;
IBGE 3,998,504 bytes / 5,571 features, SHA-256
5491bd548fe381cd388ba81c9edfbf4cf2749891e2fd32c1079ac71f7aba1c10.
The owner now explicitly authorized downloading/using the dated OCM export
(2026-04-22) instead of supplying an API key. No key/account creation is needed.
Do not relabel the snapshot as fresh or infer public/live coverage from raw
preparation counts. Source pins and real preparation measurements follow.

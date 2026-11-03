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

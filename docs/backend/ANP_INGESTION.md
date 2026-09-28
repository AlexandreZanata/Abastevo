# Server-side ANP ingestion and shared contracts

Source of official observations: ANP. Source of canonical platform station IDs: Directory. Source of published server official revisions: Official. Android's cached ANP data remains authoritative for its existing offline screens until an explicitly tested migration; do not merge the stores silently.

## Import stages

1. Discover allowlisted HTTPS ANP listing links and survey dates; conditional fetch with ETag/Last-Modified where supported. Scheduled daily discovery with a single active import lock, plus restricted manual command. Retry with bounded backoff; no scraping in public handlers.
2. Validate every redirect host/path, reject private/reserved network targets and enforce time/size limits. Initial limits: 30 MiB compressed file, 250 MiB total uncompressed content, 100 worksheets, 500k rows; adjust only with a measured ANP sample. Guard ZIP/XML bombs, external entities, traversal and workbook formula evaluation.
3. Download to bounded temporary storage, hash bytes and record source/version. Same dataset checksum/parser version is idempotent. Retain URL, checksum, discovery time and parser version.
4. Detect sheet/header layout by validated names/signatures, not only fixed row offsets. Parse streaming; normalize known labels/units/dates using fixtures. Store raw decimal text; price values up to three decimal places convert exactly to milli-BRL; greater nonzero precision quarantines the row pending a documented contract change. Never silently round.
5. Resolve CNPJ (numeric or alphanumeric with correct check digits) to stable station ID. Preserve source address components and leading zeroes. Unknown fuel/CNPJ/invalid required fields enter typed quarantine; do not coerce them. Recognized old numeric XLSX values may be padded only under a documented source rule and valid checksum.
6. Validate source row counts/duplicates, survey boundaries and previous-run deltas. Unknown headers or missing essential columns fail the import. Initial publication review threshold: >1% quarantined rows or >20% unexplained row-count drop compared with comparable dataset requires operator review; thresholds versioned. Every skipped row counts and produces a reason, so parser breakage cannot silently publish an empty week.
7. Publish complete validated revision and directory updates through owned interfaces/short transactions. Large imports stage rows in batches, then atomically switch publication pointer. Partial batches are never visible as the current official revision.
8. Changed source bytes create a new revision with supersedes reference, keeping previous prices. Reprocess errors safely; no truncating current tables. Cleanup staging/temp data separately.

## Geolocation plan

ANP sample station files do not contain station coordinates. Keep missing positions explicit. Use a provider/dataset that permits the required geocoding volume and attribution, with global quota, cache, timeout and retry. The public [Nominatim policy](https://operations.osmfoundation.org/policies/nominatim/) imposes shared-service limits and restricts systematic queries; it is not assumed suitable for national background geocoding.

Initial backend coverage may target one pilot municipality. Geocoded results record provider/time/quality/address match; ambiguous/city-centroid results cannot establish precise proximity. Manual reviewed location revisions are audited. Failed geocoding does not block textual station lookup or official prices. No geocoding request on every nearby read or observation. Provider selection is D05 before P02-T07; the rest of backend development can use deterministic fixtures.

## Shared fixture format and rollout

`contracts/testdata/anp/` will contain a manifest with format_version, provenance, source hash, expected normalization and `compatibility` classification. Each case has an ID, input, expected backend output and expected legacy-Android output where they intentionally differ. No production user data.

Required cases: all seven fuel products; legacy PREMIUM/aditivada mapping; unknown/accent/space labels; units L/M3/KG_13; 5.999 precision; empty/negative/zero prices; Excel numeric dates and leap/day-boundary cases; survey-week range; numeric CNPJ/leading zeros/new alphanumeric/checksum invalid; structured address components; header shifts; duplicate rows; same file retry; corrected source revision; malformed ZIP/XML; missing sheet; national versus state/municipal summaries.

P02 adds fixtures and Go consumers, records expected Android divergences using the imported tests/source, but changes no Android production or test source before G09. P10 adds the Kotlin fixture harness and resolves intentional incompatibilities one at a time. Full Android↔Go contract certification is therefore a P10 integration gate, not falsely claimed at G09. G09 requires frozen fixtures, Go verification and documented compatibility deltas.

Parser/library selection is D06: compare bounded streaming implementation with a maintained Go XLSX library on representative input, memory, security and licensing. Go standard ZIP/XML APIs may suffice, but do not casually build a complete spreadsheet engine. Select/pin the minimum necessary dependency before P02-T04.

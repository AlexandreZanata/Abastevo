# SEC06-A — Source-origin enforcement and transfer bounds

Date: 2026-10-11. Status: LOCAL_DONE. Scope: approved origins,
redirect-chain ownership, capped transfers, atomic publish,
sanitized names — no parser change (B), no outbox change (C).
Head: maintained `dev` + this task.
Plan: [remediation plan](../../../planning/SECURITY_REMEDIATION_PLAN_2026-10-11.md)
(SEC06, AUD-10). Contract: UC-001 discovery (approved gov.br
hierarchy). Constraint: tests never fetch (loopback seam only);
production seam stays empty (gov.br HTTPS only).

Validation: PASS

## Changes

- `data/.../AnpEndpoints.kt`: `isApprovedUrl` (HTTPS + exact
  `www.gov.br` host + approved path prefix, no userinfo,
  malformed refuses) with a 13-case offline matrix.
- `data/.../AnpFileDownloader.kt`: origin gate before fetch plus
  FINAL-URL re-check after redirects (listing compromise cannot
  bounce the client out); 64 MiB capped copy (a JSON maximum
  alone caps nothing); atomic publish (`.part` + rename, partials
  purge on every failure path); sanitized file names (query/
  fragment stripped, traversal/blank/control refuse pre-fetch).
  Explicit loopback `extraAllowedHosts` seam (default empty).
- `data/.../AnpListingScraper.kt`: origin gate plus 4 MiB capped
  listing read (declared length refuses without reading; the
  read itself stays capped). Constructor seam documented;
  Hilt `@Inject` removed (provided via NetworkModule — the seam
  has no binding).
- Tests: foreign/private/plaintext/userinfo refusals with zero
  recorded requests; cross-origin redirect refuses with no file;
  same-origin redirect still completes; oversize transfer refuses
  with nothing left behind; traversal refuses pre-fetch and
  query names sanitize; oversized listing refuses; all prior
  downloader/scraper/parser suites re-green.

## Validation (actual)

- `:data:testDebugUnitTest` (full) green, incl. 8 new tests
  (1 policy matrix + 5 downloader + 2 scraper).
  `git diff --check` clean; no secrets/PII (synthetic TEST-NET
  hosts and loopback only).
- RED pre-GREEN: foreign/private/userinfo URLs fetched or
  resolved before the gates (pattern-only classification);
  oversize streamed unbounded; traversal/query names landed
  verbatim or beside partial files.

## Limits / owed

- XLSX expansion ceilings (shared strings, dense rows, ZIP
  bombs): SEC06-B. Outbox SQL bounds: SEC06-C. Retry/retention
  budgets: SEC06-D. Measured histories: SEC06-E.
- Honest-large ANP samples run against the ceilings in SEC06-E;
  no live-source fetch occurred here. No production claim.

Next microtask: SEC06-B XLSX ceilings.

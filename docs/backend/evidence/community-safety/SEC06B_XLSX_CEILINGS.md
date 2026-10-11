# SEC06-B — XLSX expansion ceilings

Date: 2026-10-11. Status: LOCAL_DONE. Scope: coordinate/range
validation, entry/text/count/catalog caps, init-failure close —
no outbox change (C), no retry change (D). Head: maintained `dev`
+ this task.
Plan: [remediation plan](../../../planning/SECURITY_REMEDIATION_PLAN_2026-10-11.md)
(SEC06, AUD-10). Contract: Excel grid maxima (XFD/1048576).
Constraint: every bound refuses before allocation — never by
catching OutOfMemoryError; honest large ANP samples run against
the ceilings in SEC06-E.

Validation: PASS

## Changes

- `data/.../XlsxCellReference.kt`: references validate first —
  ≤3 letters, A–Z, XFD max, rows 1..1048576, blank refuses. The
  `ZZZZZZ1` audit repro (748 bytes → 300M-entry row) refuses in
  microseconds; XFD1048576 still parses (16 384-wide boundary).
- `data/.../StreamingXlsxParser.kt`: `XlsxLimits` (256 MiB entry,
  1M shared strings, 32 KiB text runs, 1024 worksheets); row
  numbers validate; shared/inline text accumulate capped;
  entry streams cap declared size up front plus actual inflated
  bytes (lying headers cannot smuggle expansion); catalog
  collects whole (bound holds wherever the target sits);
  missing entries refuse (no NPE); init failure closes the ZIP.
- `data/.../CappedInputStream.kt` (new): counting cap stream.
- Tests (`XlsxLimitsTest`, crafted zips, no live fetch): column
  bomb, grid edges, widest-valid boundary (16 384 cells),
  strings-count/text bombs, catalog bomb, corrupt-init close +
  non-ZIP refusal, missing entry. All small and fast.

## Validation (actual)

- `:data:testDebugUnitTest` (full) green, incl. 8 new ceiling
  tests; honest fixture suites (summary/station incl. 19 676-row
  file) re-green inside the ceilings.
  `git diff --check` clean; no secrets/PII (synthetic zips).
- RED pre-GREEN: column/edge/catalog bombs allocated or
  scanned past bounds before the gates (audit OOM reproduced in
  shape); the early-return catalog scan bypassed the count.

## Limits / owed

- No live 10 GB expansion run occurred (declared + actual caps
  bound it by construction); honest-large ANP samples measured
  in SEC06-E. Outbox SQL bounds: SEC06-C. No production claim.

Next microtask: SEC06-C outbox SQL bounds.

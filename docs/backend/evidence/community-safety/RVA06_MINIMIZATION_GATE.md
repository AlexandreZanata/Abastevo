# RVA06: honest training minimization gate

Status: LOCAL_DONE / INTEGRATION_PENDING for the fail-closed correction.
Validation: PASS. Content qualification remains PENDING; collection stays OFF.

## B-BR / BUC

- B-BR-RVA06-1: metadata removal and JPEG re-encoding do not prove that faces,
  plates, contact text or non-price regions are excluded. No complete frame
  receives a content-minimization PASS on that basis.
- B-BR-RVA06-2: only a separately qualified price-region path may return PASS.
  Until it exists, structurally acceptable input returns UNCERTAIN and no
  bytes; malformed/oversize/cancelled input refuses. No decode/re-encode or
  source retention is needed for an unavailable capability.
- B-BR-RVA06-3: the runtime source loader refuses collection explicitly. No
  environment switch can bypass the unqualified content gate. Existing purge
  and erasure paths remain available independently of collection.
- BUC: a valid image containing a price sign plus identifying-content
  stand-ins must not create inventory or READY output; an uncertain crop,
  price-only image or metadata-free image also refuses pending qualification.

Future activation needs a synthetic region-exclusion corpus, proven bounds,
reviewed dependency needs/licenses, source ownership and RVA04–08/terms gates.
This task chooses the fail-closed option allowed by RVA06.B; it does not claim
the future minimizer is implemented or that training is operational.

## Validation and remaining work

Tested delta on parent `32ae2c4`. RED: an 8×8 synthetic valid JPEG returned
604 bytes and PASS. GREEN: valid tiny/gradient/price-layout stand-ins and
synthetic EXIF input all return UNCERTAIN with no bytes. Invalid/truncated/
trailing input, >3 MiB and a pixel-bomb header refuse; cancelled work refuses.
No full-frame decode/re-encode runs. Production source loading now uses the
explicit unavailable adapter; there is no configurable collection bypass.

Privacy/evidence-media and worker/ops race suites, affected vet and format/diff
checks passed. Real PostGIS `TestUnqualifiedContentCannotCreateInventoryOnSQL`
passed: the production sanitizer produces no inventory/READY output. The first
uncached unit run was sandbox-blocked at a local httptest listener; the exact
scope passed with local-socket permission, without test changes or skips.
Staged secret-surface review passed. No new dependency or provider operation.

Future source/producer/content activation is not implemented or accepted.
Historical CS10C soak evidence/harness retains its old revision and cannot
qualify this changed pipeline; existing running fixtures were not touched.
RVA07 owns a new qualifying deadline run. UI must show unavailable collection
truthfully until all activation prerequisites pass (RVA08/CS23).

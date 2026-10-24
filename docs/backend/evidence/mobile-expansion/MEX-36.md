# MEX-36 — Privacy, access and all-copy recovery acceptance

Status: LOCAL_DONE
Validation: PASS
Integration: INTEGRATION_PENDING

Parent: `887d5a71`. Date: 2026-10-23.
Depends MEX-33/34/35; PRM/privacy ownership unchanged.

## Negative proofs executed

- Erasure/restore fences: `TestErasureBlocksReuseAndPurgesObjects`,
  `TestErasureOutageRestoredBytesAndRetry`,
  `TestRestoreCommitDoubleConfirmAndStale` PASS on disposable PostGIS:
  erased IDs cannot be recreated, outage bytes are not advertised as
  success, retry preserves fences, double confirm is idempotent and a
  stale preview refuses.
- Privacy campaign: `TestPrivacyCampaignDeleteDuringActivityAndRestore`
  PASS (delete during activity, restore, no resurrection).
- Owner isolation/free exit:
  `TestCloudFreeReadExportErasureInventoryAndForeignSession`,
  `TestLedgerStaleCascadeExpiryForeign`,
  `TestJourneyNegatives` PASS.
- CSV: `TestCSVEscapeFormulas` PASS; formula-leading fields are
  neutralized, quotes/newlines escaped, no private totals leak.
- Secret/PII review: `bash scripts/scan-secrets.sh` clean; public
  charging/occurrence fixtures contain no owner IDs, precise GPS,
  private home coordinates, photos or tokens. EE-22 client tests assert
  no GPS/route/home/photo keys in the wire body.

## Limits

- Real S3/object deletion receipts, signed staging two-account runs and
  provider push receipts remain OWED; local fakes/PostGIS do not prove
  remote object purge.
- No production customer traces, logs or photos were used.
- Owner export/delete/correction after paid lapse is covered by
  backend tests; physical Android journey remains OWED.

Next: MEX-37 accessibility/compatibility final union.

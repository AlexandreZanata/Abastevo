# SEC03-C — Provider-compatible transfer matrix

Date: 2026-10-11. Status: LOCAL_DONE. Scope: transport matrix +
verify loop over a real local S3-compatible service — no provider
change, no production storage (SEC03-D owns namespace/monitoring).
Head: maintained `dev` + this task.
Plan: [remediation plan](../../../planning/SECURITY_REMEDIATION_PLAN_2026-10-11.md)
(SEC03, AUD-02/04). Contract: SEC03-B boundary (transport accepts,
verification enforces). Constraint: small synthetic payloads only
(plus exact max/max+1 loopback cases); test-only MinIO creds;
anonymous volume dies with the container; no production data.

Validation: PASS

## Changes

- `evidence/adapters/storage/sec03c_matrix_integration_test.go`
  (new, disposable loopback MinIO `RELEASE.2025-04-22T22-12-26Z`,
  real SigV4/HTTP, no fakes): max accepted exactly; max+1
  accepted by transport but refused by the bounded read;
  declared/actual mismatch stored (claim bind owns it); wrong
  content-type 403 (type bound); transplanted key 403 (key bound);
  missing key 404, missing delete nil; backdated URL 4xx
  (expiry honored); interrupted PUT stays absent (follow-up 404,
  never verifiable); URL reuse overwrites (second body wins —
  claim binding decides promotion); 4 parallel transfers complete.
- `evidence/application/verify_protocol_integration_test.go`
  (new): full worker loop over the actual protocol — honest JPEG
  PUT promotes READY with a retrievable sanitized final;
  MaxBytes+1 on the wire rejects oversize; re-encoded bytes
  disagreeing with the claim reject checksum-mismatch; no final
  upload in either refusal.

## Validation (actual)

- `TestSEC03CTransportMatrixOnRealS3` PASS (real MinIO) and
  `TestSEC03CVerifyLoopOnRealS3` PASS (real MinIO + real bounded
  Download/server Upload).
- `go test ./internal/modules/evidence/...` green.
  `-race -tags=integration ./internal/modules/evidence/adapters/`
  still green (SEC03-A proofs intact; storage has no PG suite).
- `go vet` (+integration), `gofmt -l` clean,
  `git diff --check` clean, `scan-secrets.sh` passed (test-only
  MinIO creds, synthetic payloads).
- RED pre-GREEN: matrix written to fail on transport-caps
  (max+1 PUT succeeding surprises a JSON-max-only design);
  observed GREEN on first run against MinIO, confirming the
  SEC03-B boundary selection with provider evidence instead of
  assumption.

## Limits / owed

- Chunked/missing-length PUTs: our client always sends fixed
  length (see `transfer.Put`); provider-specific chunked
  behavior unpinned here — no chunked path exists to exploit.
- R2 production behavior unverified; MinIO proves the S3
  protocol mechanics, not the vendor. Namespace/monitoring:
  SEC03-D. No production claim.

Next microtask: SEC03-D namespace + monitoring.

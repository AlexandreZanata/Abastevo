# RVA01 — enforced photo ingress

Status: LOCAL_DONE / INTEGRATION_PENDING. B-BR-005 / B-BR-M04, evidence reservation/upload.

Production reservation issues a short-lived HMAC capability for the owned API
PUT route, never a storage PUT URL. Domain separation binds session and expiry;
persisted facts bind exact MIME/size/hash. Only ISSUED, unexpired sessions accept
bytes. Four transfers per API process; five durable attempts per session with a
bounded lease. Accepted transfers replay without rewriting storage. Failed or
ambiguous attempts can retry under the same hash; attempts never refund.

Content-Length and declared+1 bounded read reject oversize before storage. Read
deadline 15s, overall budget 20s. Storage credentials/PUT signatures stay private.
Lease fencing protects receipts. Owner completion/verification/retention remain
required. Expired capabilities cannot extend reservation deadlines.

RED: size/chunking/truncation/hash/type/token/expiry/admission/storage failures.
Real SQL: attempt exhaustion, lease fencing and receipt replay. Real private S3:
accepted bytes and rejected object absence. Provider-neutral API enforcement
does not imply an unrun real-R2 or release acceptance.

## Validation

Tested tree: synchronized `9e0a831` plus this RVA01 runtime/test/migration set.
- RED: gateway contract tests failed before the gateway existed.
- GREEN: evidence unit/HTTP/media/storage/application/domain, API unit, HTTP
  server and OpenAPI contract suites passed under `-race` (bounded `-p 2`).
- Three real-SQL ingress cases passed: 12 racers yield one lease, failed attempts
  exhaust at five, stale nonce cannot finish, receipt replay retains actual bytes,
  migration 000061 alone preserves an existing session and replays empty.
- Actual API/worker/private-S3 process tests passed for standard and development
  photo flows, now including oversized transfer refusal then exact retry.
- Gateway four-transfer concurrency and no-rewrite receipt replay passed.
- Route-specific body cap test passed; other endpoints keep their original cap.
- sqlc generation/vet, affected Go vet, gofmt and diff/secret checks passed.
  OpenAPI vacuum lint passed with existing nonblocking warnings; no claim of
  zero warnings or a fresh advisory scan.

Services: exclusive local Compose project `abastevo-rva-20261011-9e0a831`,
loopback PostgreSQL 32788 and private S3 32789. No VPS or existing soak changed.
The validation Compose storage fixture used a retired MinIO repository; replaced
it with the identical immutable Bitnami fixture already reviewed in PR129.
Existing migration-000060 upgrade proof now freezes its target at 000060, so
future append-only migrations cannot invalidate that historical upgrade test.

Envelope: at most 4 × (3 MiB + 1) buffered ingress bytes per API process;
5 charged attempts × 10 reservations caps successful-body transfer attempts at
150 MiB/day per contributor, independent of URL reissuance. Receipt bytes cannot
exceed the reservation. HTTP/edge admission and provider staging qualification
remain separate later gates. Startup/restart invalidates capabilities if the
configured secret changes; clients reserve again under the same operation.

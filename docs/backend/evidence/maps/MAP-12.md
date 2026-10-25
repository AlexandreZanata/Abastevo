# MAP-12 — Signed regional manifests and recoverable package downloads

Status: LOCAL_DONE / INTEGRATION_PENDING
Validation: PASS (signed envelope, tamper refusal, Range/ETag and actual API process)
Integration: INTEGRATION_PENDING

Tested tree: `codex/map-ev-implementation` at `721582b8` plus the uncommitted
MAP-09–12 changes. Date: 2026-10-24.

## Behavior frozen

- A distribution directory contains exactly one `distribution.json`, the
  native-map manifest (base64url inside the envelope) and the immutable package
  files named by the manifest.
- The envelope is `abastevo.map-distribution-v1`, Ed25519, with `key_id` and
  signature over the exact manifest bytes. The API is configured with pinned
  public keys only; it never receives a private key.
- Startup verifies the envelope, manifest schema, package paths, sizes and
  SHA-256 digests. Any missing/changed/unsafe package fails closed and the
  process does not serve a partial distribution.
- `GET /v1/map/regions/{region}/manifest` serves only the manifest whose
  `region.id` matches. ETag/If-None-Match are supported.
- `GET /v1/map/packages/{package_id}` serves only package IDs present in the
  signed manifest. It never joins user input to a filesystem path. Before every
  response it re-reads and SHA-256-verifies the opened bytes against the signed
  manifest; changed bytes return `409 map.package-changed` instead of serving
  new content under the old immutable ETag/URL. Responses include immutable
  caching, ETag, `Accept-Ranges: bytes` and `206` Range support.
- No private S3 media, redirect, upstream CDN or arbitrary path is reachable
  from these routes. The map distribution is optional and unmounted when the
  env pair is absent.

## Implementation

- `contracts/map/map-distribution-v1.schema.json`.
- `backend/internal/modules/mapdist`: sign/verify/load and HTTP handler.
- `backend/cmd/map-sign`: offline Ed25519 envelope builder.
- `backend/internal/platform/config`: optional paired
  `ANPFUEL_MAP_DIST_DIR` / `ANPFUEL_MAP_DIST_PUBLIC_KEYS`.
- `contracts/openapi/v1.yaml`: manifest and package paths.

## Verification executed

Built a real distribution from the MAP-08 Curitiba package plus test-only
`World.mwm`/`WorldCoasts.mwm`, signed it with a generated Ed25519 key, and ran
the actual API process:

```text
GET /v1/map/regions/brazil-parana-east/manifest -> 200
format=abastevo.map-distribution-v1 key_id=map-test-2026-10
ETag present; If-None-Match -> 304

GET /v1/map/packages/brazil-parana-east (Range: bytes=0-31) -> 206
Content-Range: bytes 0-31/10058648
Content-Length: 32
ETag: "d1e9b2ce…00a3e"
body starts: 1d7a9900000000004d574d00…

GET /v1/map/packages/nope -> 404
```

Unit tests cover signature tamper, untrusted key, changed package bytes (startup
and request-time `409`), missing package, unsafe paths, key-list parsing,
manifest ETag/304 and package Range.
`go test -race ./internal/modules/mapdist` and
`go test ./internal/platform/config ./internal/platform/apicontract` pass.

## Limits

- One pilot region; World/WorldCoasts are test inputs, not a production CDN
  entitlement. No national set, signed key rotation rehearsal, staging deploy
  or Play distribution.
- The Android downloader/activation is MAP-15; this phase proves the wire
  protocol and server boundary only.
- Range resumption is HTTP-level; interrupted-transfer recovery and disk-full
  behavior are tested in MAP-15.

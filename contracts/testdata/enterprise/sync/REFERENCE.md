# Reference feed client profile v1

EMP-FLEET-013 / B-BR-EMP-348..352, EDR-224..227. `reference-feed-v1.psv` pins the
signed reference feed of EMP-OFFL-006 (`POST /v1/orgs/{org_id}/sync/reference`,
frozen in `contracts/openapi/v1.yaml`) between the Go server and the Kotlin
client, and the client's local cache format.

Header `id|operation|input_hex|context_hex|outcome|expected_hex`. Operations:
`REQUEST` (raw request body; Kotlin parses and re-encodes it canonically, Go runs
the real server request parser and its 4 KiB body bound), `RESPONSE` (raw
response bytes as Go's `encoding/json` emits the server wire struct; context is
the canonical `{"held_revision","request"}`), `APPLY` (a response applied to a
prior cache; context is the canonical `{"cache","received","request"}`) and
`CACHE` (local cache bytes, version 1). Outcomes are `OK`, `INVALID`, `LIMIT` and
`BINDING`; `expected_hex` is the canonical result for `OK` and `-` otherwise.

The Go consumer proves that every response the client accepts (`OK`) or refuses
only as foreign (`BINDING`) is byte-for-byte what the server wire struct
re-encodes and satisfies the server delta invariants, and that every response
the client refuses as `INVALID` or `LIMIT` is not server-producible. `APPLY`
and `CACHE` semantics are client-only (EDR-227); Go checks only their embedded
responses. The generator is an independent Python oracle kept with the task
logs; no expected bytes come from either consumer.

129 rows: REQUEST 23 (OK 6, INVALID 16, LIMIT 1), RESPONSE 79 (OK 18, INVALID
52, LIMIT 1, BINDING 8), APPLY 16 (OK 10, BINDING 5, INVALID 1), CACHE 11 (OK 3,
INVALID 8). SHA-256
`ec39616cd9d58d0b54bb7ed9129e023fe91f8cb868c8d4a8546cf73e0ffd2098`.

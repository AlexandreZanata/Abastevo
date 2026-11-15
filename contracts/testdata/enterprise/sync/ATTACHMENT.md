# Attachment session profile v1

EMP-OFFL-005 / B-BR-EMP-415..420, EDR-267..270. `attachment-session-v1.psv`
pins the signed attachment session (`POST /v1/orgs/{org_id}/sync/attachments`,
frozen in `contracts/openapi/v1.yaml`) between the Go server and the Kotlin
client.

Header `id|operation|input_hex|context_hex|outcome|expected_hex`. Operations:
`REQUEST` (raw request body; Kotlin parses it strictly and re-encodes it
canonically, Go runs the real server parser with its 4 KiB bound) and
`RESPONSE` (raw response bytes in the server wire-struct field order; context
is the canonical request the response must bind to). Outcomes are `OK`,
`INVALID`, `LIMIT` and `BINDING`; `expected_hex` is the canonical (sorted-key,
compact) re-encoding for `OK` and `-` otherwise.

The request is flat: schema 1, request/family/membership/enrollment
identifiers, graphic ASCII access token (1..512), the declaration, document
and link identifiers (pairwise distinct), kind `RECEIPT`, lowercase SHA-256,
size 1..5242880, `image/jpeg`, a target kind of the closed EMP-DOC-001
vocabulary (the DOC-003 registry, not the wire, decides which are linkable)
and the target identifier. The response carries schema 1, the request,
membership, enrollment and document identifiers, one of the seven storage
states, a positive int64 storage revision and `upload`, which is a
capability (graphic ASCII 1..4096) with a canonical UTC expiry only when the
storage is `UPLOADING`, or null. Responses are at most 8 KiB.

The Go consumer proves that every request the client accepts is accepted by
the server parser with the same values, every refused one is refused, and
every response the client accepts (`OK`) or refuses only as foreign
(`BINDING`) is byte for byte what the server writes for that request and
session, while every `INVALID` or `LIMIT` response is not server-producible.
The generator is an independent Python oracle kept with the task logs; no
expected byte comes from either consumer.

89 rows: REQUEST 53 (OK 15, INVALID 37, LIMIT 1), RESPONSE 36 (OK 11,
INVALID 20, LIMIT 1, BINDING 4). SHA-256
`1996b45fb008044a04df5715315ac0d82f82f5d942f36a8c64b89dbf4f170b9c`.

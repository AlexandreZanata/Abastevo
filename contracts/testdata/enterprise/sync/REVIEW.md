# Review resolution profile v1

This additive profile never changes operation, acknowledgement or batch v1 bytes.
Resolution outcomes are `ACCEPTED` and `REFUSED`, with one immutable item revision
1 to 2 decision. The original final `ACCEPTED_FOR_REVIEW` acknowledgement stays
unchanged. The reason is exact, valid Unicode, control-free and at most280 scalar
values; refusal requires a nonblank reason, acceptance may use an empty reason.
Server instants are canonical UTC, excluding the unset minimum. Entity revision
is positive exact int64 for acceptance and explicit null for refusal.

The signed review-results request has schema1, request/family/member/enrollment
IDs, graphic ASCII proof and at most50 distinct operation/digest selectors. Its
64KiB bootstrap parser confers no source authority. The independently captured
owner/origin/org/member/enrollment tuple and request ID bind the complete128KiB
response. Each ordered selector has exactly one slot, with explicit null or a
resolution of at most4KiB. Local expected original review IDs bind every returned
resolution before any result is published. No missing, duplicated or reordered
suffix may expose a valid prefix. Secrets and reviewer proof are not outcomes.

`review-v1.psv` has137 rows: RESOLUTION66, REQUEST33, RESPONSE38. The exact header,
IDs, operations, group counts, hex grammar and complete canonical bytes are
checked by both consumers. `input_hex` is literal transport bytes, intentionally
including invalid UTF8. Context is `-` outside RESPONSE; response contexts contain
an independent source tuple, exact original request and original review selectors.
Negative rows use `-` canonical bytes; outcomes are OK/INVALID/LIMIT/BINDING/DIGEST.
Fixture dispatch names are grammar, never production policy. The SHA256 is
`db1e7499590d6c5e226f77be4f5962ec43ad0411356fcb4c1cdecfb8a5683dd8`.

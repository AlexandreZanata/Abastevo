# Go-signed distribution fixture for Kotlin MAP-15 tests

`distribution.json` is a real `mapdist.Sign` envelope produced by Go
`crypto/ed25519` (see the removed-by-design generator: ephemeral key,
`Sign` + `Verify` round-trip green at generation time). The package files
under `2601/` are deterministic synthetic bytes (NOT real `.mwm` map data);
only their SHA-256 digests and the Ed25519 signature are meaningful.

- `key_id`: `test-key` (test-only; no operational meaning, never pinned).
- Public key (`test-key.pub`, base64url raw 32 bytes).
- The private key was ephemeral and is NOT stored anywhere.
- `TestCommittedKotlinFixture` (`backend/internal/modules/mapdist/`)
  `Load`s this directory and rejects unknown keys; Kotlin JVM/device tests
  verify the same envelope through the platform provider and the owned
  Monocypher bridge.

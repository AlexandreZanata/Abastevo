// Package http serves the owner-only training-consent endpoints
// (CS09C): signed grant/revoke writes plus the owner status read.
// Authentication already happened upstream: Authenticate resolves the
// caller from the verified proof, so this package never sees keys,
// signatures or device material. Owner identity travels only in the
// caller, never in a body-supplied field.
package http

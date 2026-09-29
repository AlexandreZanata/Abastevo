// Package http serves private upload reservation and owner status
// (P05-T04): strict reserve parsing with canonical integer sizes,
// idempotent completion intents and owner-only status reads. Every
// response carries no-store; status bodies never contain object keys or
// URLs. Authentication already happened upstream: Authenticate resolves
// the caller from the verified proof, so this package never sees keys or
// signatures.
package http

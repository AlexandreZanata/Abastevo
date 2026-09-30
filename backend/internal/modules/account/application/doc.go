// Package application owns FREE account use cases (P13-T02A): email code
// enrollment, code consume with signup/login, refresh rotation with reuse
// detection and revocation. Every decision funnels through the frozen domain
// policy; persistence and mail stay behind the Store and MailSender ports so
// unit tests run on a mutex-guarded memory store with a fake clock and the
// Postgres adapter (P13-T02B) and HTTP transport (P13-T02C) reuse the exact
// same semantics. Plaintext codes and tokens never cross a port boundary
// into storage: only salted hashes persist.
package application

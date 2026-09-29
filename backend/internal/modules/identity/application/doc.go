// Package application owns anonymous identity use-cases (P03-T04): the
// idempotency decision policy shared by every mutating handler. Pure policy
// only — execute, replay, conflict or re-reserve from stored versus
// presented hashes and expiry. Persistence and the business transaction stay
// behind the Store port in identity/adapters, so a retry replays the stored
// outcome instead of executing twice and a changed body conflicts.
package application

// Package storage mints short direct-upload authorizations (P05-T02) for
// private S3-compatible object storage (ADR-010): presigned PUT URLs bound
// to one server-generated quarantine key, one content type and a short
// expiry, with SigV4 computed from the standard library only — no new
// dependency, no credential ever logged.
//
// The presigned URL authorizes transport, nothing else: it does not
// enforce the declared size and stays reusable until expiry, so the
// worker (P05-T03) downloads a bounded snapshot of the exact bytes,
// validates them and writes sanitized bytes to a different server-only
// key. The bucket stays private by construction: this package never emits
// ACL grants, and object keys outside the quarantine namespace are
// refused at mint time.
package storage

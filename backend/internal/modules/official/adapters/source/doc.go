// Package source fetches only approved ANP origins with checksum (P02-T05).
// Every URL — initial or redirect — must match the allowlist (exact HTTPS
// host, approved path prefix, no credentials/queries, no unlisted ports,
// filename shape for downloads), and every dialed IP must pass the unicast policy, which
// refuses private, shared, benchmark, documentation, loopback, link-local,
// multicast and unspecified ranges. DNS is resolved in the dial path and the
// validated address is the one dialed, so a rebinding answer cannot slip a
// private host past the check. Conditional requests (ETag/Last-Modified)
// make re-downloads and discovery probes cheap; bodies stream to owned
// temporary files that are always cleaned up, and SHA-256 is computed over
// the exact stored bytes. No scraping, no HTML parsing, no external calls
// from public handlers: listing-link extraction stays out of scope until a
// documented format exists.
package source

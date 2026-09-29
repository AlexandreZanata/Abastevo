// Package media validates and sanitizes image snapshots (P05-T03) with
// the standard library only: bounded size, JPEG magic, strict structure
// with no trailing data, dimension and pixel caps against decoder bombs,
// full decode, server hashes (SHA-256 plus dHash) and EXIF-free
// re-encode to a server-owned final key.
//
// Every check runs on one in-memory snapshot: a single download whose
// exact bytes are hashed, so a HEAD-then-act race cannot swap the
// validated object (TOCTOU). Polyglot and oversized inputs reject before
// any expensive decode; only the re-encoded bytes ever become READY.
package media

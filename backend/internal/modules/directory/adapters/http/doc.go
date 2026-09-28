// Package http exposes the anonymous directory reads (P02-T08): station
// search, nearby search and station detail. Handlers parse and validate
// HTTP input, seal opaque cursors bound to their filters and render the
// contract envelopes with explicit cache policies; nearby answers carry
// no-store and never echo the request point. Coordinates and raw query
// text never enter logs: handlers emit no log lines at all, and failures
// carry stable machine codes. Depends on the directory application port
// only.
package http

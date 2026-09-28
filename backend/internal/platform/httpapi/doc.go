// Package httpapi holds shared anonymous-read HTTP plumbing (P02-T08):
// opaque HMAC pagination cursors bound to their filters, the ApiError
// envelope, limit/cursor query parsing and cache/ETag headers. Coordinates
// and raw input never enter logs or error bodies here: handlers pass only
// validated scalars, and failures carry stable machine codes for client
// localization. Cursor integrity substitutes server state, so pages stay
// stateless; tampered, expired or filter-changed cursors fail closed.
package httpapi

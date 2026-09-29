// Package http exposes the community write and owner reads (P04-T04):
// signed observation submit with a safe RECEIVED acknowledgment, owner-only
// status with its decision timeline, and owner history pagination. All
// responses carry no-store; errors share the ApiError envelope without
// distinguishing missing from foreign resources. Depends on the community
// application use-cases only.
package http

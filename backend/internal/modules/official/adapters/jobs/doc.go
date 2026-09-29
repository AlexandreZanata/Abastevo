// Package jobs connects durable handlers for ANP discovery and import
// (P03-T08). The discovery handler probes the documented weekly file URLs
// and enqueues one import job per changed etag; the import handler fetches,
// parses, normalizes, resolves and publishes a single file through the
// existing pipeline. Both run behind the platform dispatcher with payload
// versions; unknown versions fail toward DEAD instead of running blind.
package jobs

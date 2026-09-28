// Package application owns anonymous directory read use-cases (P02-T08):
// station search, nearby search and station detail. Use-cases validate
// semantic bounds (geo ranges, query lengths, UUID shapes) and return
// opaque sort positions; HTTP cursor sealing, envelopes and cache headers
// stay in the handlers. Application depends on its domain, the shared
// kernel and explicitly declared ports only — never on adapters, SQL or
// other modules.
package application

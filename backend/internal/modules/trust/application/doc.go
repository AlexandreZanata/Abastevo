// Package application records audited trust verdicts (P06-T03): one
// decision, its current-tier projection and the affected-key recompute
// job, composed through explicitly declared ports. Recording is
// deliberately auditor-driven, never automatic: no consensus count,
// volume metric or paid plan may promote through this path, and the
// Evaluate policy stays a reviewer-side function until moderation (P07)
// authenticates its callers. Depends on its domain and ports only —
// never on adapters, SQL or other modules.
package application

// Package http exposes the anonymous official reads (P02-T08): current
// price groups and revision-aware price history. Community sections stay
// null until P04; unknown stations resolve to 404 through an injected
// existence check so modules never cross-read. Depends on the official
// application port only, plus a station-exists function the composition
// root provides.
package http

// Package metrics is the private service-metrics surface (P08-T05).
//
// Counters and gauges with strictly bounded label sets, exposed as
// Prometheus text on the loopback-only metrics listener. Label names
// validate; label values come from fixed vocabularies (HTTP method,
// chi route template, status class) so cardinality cannot grow with
// contributor, station or observation IDs (B-BR-011). A per-metric
// series cap drops runaway combinations loudly instead of OOMing the
// process. Stdlib only.
package metrics

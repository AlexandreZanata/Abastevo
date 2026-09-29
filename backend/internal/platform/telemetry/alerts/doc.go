// Package alerts evaluates bounded service rules with cooldown and a
// tested notification route (P08-T05). Rules read snapshots through
// injected checks (database, queue depth, backup age); the evaluator
// deduplicates within cooldown so a stuck firing rule pages once per
// window instead of flooding the operator. Alert payloads carry rule
// name, severity, stable detail and timestamp only: never contributor,
// station or observation identifiers, request bodies, tokens or URLs
// (B-BR-011). Stdlib only.
package alerts

// Package jobs connects the bounded geocoding worker (P03-T08): each run
// takes the oldest stations still missing a reviewed projection and
// resolves them through the injected geocoder service, which records
// unknown or city-centroid revisions with attribution and never projects
// on its own. Without a configured provider (D05 pending) the handler
// refuses loudly instead of fabricating coordinates, and its schedule
// stays disabled until a live provider exists.
package jobs

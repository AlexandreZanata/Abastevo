// Package geocoder resolves station coordinates behind a provider port
// with global throttle, quota, cache and full provenance (P02-T07). No live
// provider is wired: D05 stays pending, so development and tests use the
// deterministic FixtureProvider and every provider result records at most
// city-centroid quality. Reviewed projections happen only through the
// explicit directory Store path (manual review), never from a provider
// response; unknown and centroid results can never establish precise
// proximity (B-BR-014). Attribution (provider, source reference, time) is
// stored on every revision. Rollback is provider disablement: throttle to
// zero or swap the port, last reviewed projections stay untouched.
package geocoder

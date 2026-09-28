// Package read implements the official PriceReader port on the owned
// generated queries (P02-T08). It owns the official queries package; no
// other module may import it. Current values resolve through the latest
// published revision holding the station; history spans every published
// revision newest-first with keyset pagination. Unknown stations yield
// empty results here — the HTTP layer decides 404 through an injected
// existence check so modules never cross-read.
package read

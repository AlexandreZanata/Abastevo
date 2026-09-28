// Package domain owns canonical station identity (P02-T03): stations with a
// source-independent ID, historied CNPJ identifiers and append-only location
// revisions with an explicit missing state (B-BR-014). Only reviewed points
// may become the projected current location; city-centroid and unknown
// results are recorded but never projected as precise positions.
//
// Boundaries: stdlib plus the shared kernel only (TestDomainStdlibOnly
// covers this package too). No I/O, no SQL. Persistence lives in
// directory/adapters behind the Resolver and Store ports; alias corrections
// retire identifiers instead of deleting history.
package domain

# EVC-05 generation rights

## EVC-05A opening

B-BR-EV-RIGHTS-01. Derive a rights manifest from every retained source
assertion, including attached equipment. Bind it to the catalog digest,
the admitted OSM extract, and the admitted OCM inventory and provider
licences. A missing, unknown or altered provider or attribution refuses
the whole generation. An OSM-derived generation requires a derived
database offer before public publication.

BUC-EVC-05A. Build a rights manifest from a synthetic two-source catalog.
Reject missing source attribution, unknown OCM providers, changed licence
text or source edition, and repeated lineage across sites. Verify that
provider-specific attribution remains distinguishable for each site.

Status: LOCAL_DONE
Validation: PASS

Both initial rights assertions failed on the absent manifest. The
manifest now binds the catalog SHA-256 to the pinned OSM extract and
OCM reference and inventory. Every root and attached equipment record
is checked. OCM provider licences and attributions retain distinct text
and counts. Missing or changed rights and duplicate lineage refuse the
whole output. Three focused tests, all Rust targets, all-target Clippy,
format, whitespace and secret checks passed. The result declares the
offer requirement; it does not by itself authorize publication.

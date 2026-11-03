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

## EVC-05B opening

B-BR-EV-RIGHTS-01. Keep the derived-database offer and its terms bound to
the exact rights-checked catalog. The release candidate must refuse absent
or modified rights or offer bytes. The offer contains admitted catalog
records only, excluding private review files.

BUC-EVC-05B. Prepare an offer from a hash-checked synthetic conflation
manifest, revalidate it, and refuse removed rights, missing data, and
changed offer bytes before release.

Status: LOCAL_DONE
Validation: PASS

The local command verifies the exact upstream manifest and canonical
catalog before writing an owner-only bundle. Its notice binds the
download bytes, ODbL and OSM attribution, and distinct OCM provider
terms. The verifier rejects missing rights or offer data, changed
offer bytes or terms, and unrecognised record fields even when a new
manifest digest is supplied. The initial offer test failed before
implementation. All Rust targets, Clippy, format, whitespace and
secret checks pass. This evidence uses synthetic inputs. Runtime
publication and the nationwide offer await the admitted catalog and
EVC-06 activation gate.

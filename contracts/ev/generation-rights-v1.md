# Charging generation rights v1

The local command accepts an existing EVC-03 catalog directory and the
expected SHA-256 of its conflation manifest.

```text
prepare_ev_rights prepare CATALOG_DIR MANIFEST_SHA256 NEW_BUNDLE_DIR
prepare_ev_rights verify CATALOG_DIR MANIFEST_SHA256 BUNDLE_DIR
```

`rights.json` uses `abastevo.ev-generation-rights-v1`. It identifies the
exact catalog and upstream manifest, the frozen source profile, the OSM
extract, the OCM revision, reference and inventory, and every retained
OCM provider's exact terms and assertion count. Counts include attached
equipment. Source-level attribution is also retained on each catalog
record.

A bundle containing OSM source assertions must include
`derived-database.ndjson` and `derived-database-offer.json`. The NDJSON
contains the admitted derived catalog in its canonical framing and has
the same digest as that catalog. The offer identifies the download,
ODbL licence, OSM attribution and distinct OCM provider terms. Review
queues and raw source archives are outside the offer. A bundle without
OSM assertions has `offer: null`.

A public generation must validate its rights bundle and make its
required data and notice available together with its API and offline
pack. Missing, changed or extra files are refused. The local command
only prepares and verifies files.

## Geography sidecar

```text
prepare_ev_rights geography CATALOG_DIR MANIFEST_SHA256 IBGE_FILE OUTPUT_FILE
```

The separate `abastevo.ev-catalog-geography-v1` document binds the
catalog SHA-256 and pinned IBGE boundary SHA-256 to one classification
per canonical key. The importer takes an independently pinned sidecar
digest, refuses missing or repeated keys, and derives the census from
the joined rows. `UNASSIGNED` is explicit and conveys no address.

# Map contracts

Frozen MAP-03 formats (JSON Schema 2020-12):

- `poi-assertions-v1.schema.json` — source-specific public site/price assertions.
- `native-map-manifest-v1.schema.json` — pinned engine/generator, region,
  packages, dependencies and attribution.
- `prepared-envelope-v1.schema.json` — reference/overlay/private artifact
  envelope reusing the existing manifest/hash/census discipline.

Goldens: `contracts/testdata/map/`. These are contracts, not published API
schemas or evidence that a map/native engine exists. Unknown rights, missing
dependencies or mixed formats are rejected before activation.

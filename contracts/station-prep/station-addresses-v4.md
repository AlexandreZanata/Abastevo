# Station assertion v4: sourced addresses

The offline registry preparer opts in with `--address-inputs DESCRIPTOR`.
`--name-inputs DESCRIPTOR` remains optional and can appear in either order.
Without the address flag, the existing v2/v3 bytes and parser IDs are unchanged.

```sh
umask 077
prepare_registry REGISTRY.csv IBGE_ALIASES.json NEW_DIR SOURCE_REF EDITION RUN_UUID UTC_ISO \
  --name-inputs NAMES.json --address-inputs ADDRESSES.json
```

Use a private directory outside Git. The command consumes existing local bytes;
it does not fetch, stage, publish or backfill. Keep the prepared manifest with
its streams for later authorized loading.

The `registry-address-inputs-v1` descriptor contains up to three `inputs`.
Each pin has `source`, `profile`, `file`, `reference`, `edition`, `sha256`,
`bytes`, `rows`, `license` and `attribution`. `file` is a regular basename next
to the descriptor; symlinks and escaping paths are refused. Descriptor cap:
128 KiB. Per input: 64 MiB, 500,000 JSONL records, 64 KiB per line, final newline.
At most 256 address observations per normalized CNPJ across admitted sources.

- `rfb-station-addresses-v1`: `schema_version`, `cnpj`, `active`, `address`,
  `observed_at`. `active` comes from the establishment's own status, not a
  default. A newer inactive observation cannot select an older address.
- `osm-station-addresses-v1`: `schema_version`, `cnpj`, `object_type`,
  `object_id`, `match_reference`, `address`, `observed_at`. Use an existing
  reviewed or source-bound station match. Coordinates alone are not a match.
- CNEFE is recorded as skipped by the frozen policy while dataset reuse remains
  unverified. The preparer does not read that input's rows. The separate pure
  selector already exercises exact establishment evidence in synthetic tests.

Observation dates are ISO calendar dates. The address map permits only `street`,
`number`, `complement`, `district`, `postal_code`, `municipio`, `municipio_ibge`,
`uf` and `raw`. Contact, coordinate and other keys are refused. Do not put
private values, credentials or credential-bearing URLs in source references.

The [frozen source policy](address-source-policy-v1.json) binds profile, rights
and attribution, independently of input approval. The metadata reports RFB's
unversioned Creative Commons Attribution declaration and OSM ODbL 1.0. A local
hash proves byte identity, not source authenticity. Retain the original source
receipt/match evidence separately. Public OSM-derived publication also requires
its attribution and derived-database offer; preparation is not publication.

`station-assertion-v4` preserves the original ANP `address_raw` and
`address_normalized` fields, and adds `address_facts` with one `source`, a whole
`components` object and nullable `provenance`. External provenance binds input
profile/reference/edition/hash, date, rights policy, license, attribution and
match proof. ANP/municipality/unknown have no external provenance. The original
station identity/authorization and absent coordinates remain unchanged. The Go
loader adds canonical municipality identity metadata to the projection and
retains the external proof in `registry_assertions.address_provenance`.

Parser `station-prep-v0.3.1+addresses-v1` adds `address_policy_sha256`,
`address_inputs`, `address_skips`, and `address_coverage` to `station-batch-v1`.
The latter counts the same current accepted distinct CNPJs before and after
selection, nationally and by registry UF. Superseded assertions and repeats do
not enter that denominator. Before is the same file's ANP/municipality/unknown
baseline, not a measurement of the live database. Reasons are aggregate counts;
raw addresses, CNPJs and contact fields do not belong in coverage evidence.

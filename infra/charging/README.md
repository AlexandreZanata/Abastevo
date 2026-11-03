# Charging catalog review

Run these units on an operator-controlled preparation host. Build
`backend/cmd/charging-catalog` and install the binary at
`/usr/local/bin/charging-catalog`. Create the dedicated system account
`abastevo-charge-review` with read access to its owner-reviewed inputs.
Use an input directory outside home and temporary paths so the service
can read it with home protection and a private temporary directory.

The OCM timer reviews weekly at 04:10 UTC on Monday. The OSM timer reviews
monthly at 04:20 UTC on the first day. Each reads its own configuration
under `/etc/abastevo/charging-review-ocm.json` or
`/etc/abastevo/charging-review-osm.json`. Install the service template and
the corresponding timer when the operator authorizes scheduling. The
service has an isolated network namespace and cannot fetch new sources.

Each configured generation directory contains `rights/`, `geography.json`,
`conflated/generation/` and `normalized/generation/` from the local
preparation workflow. The JSON configuration has this form. Replace
each placeholder with the independently reviewed SHA-256.

```json
{
  "candidate": {
    "directory": "/srv/abastevo/charging/candidate",
    "rights_sha256": "<rights receipt>",
    "manifest_sha256": "<conflation manifest>",
    "geography_sha256": "<classification sidecar>",
    "boundary_sha256": "<IBGE boundary>"
  },
  "baseline": {
    "directory": "/srv/abastevo/charging/active",
    "rights_sha256": "<rights receipt>",
    "manifest_sha256": "<conflation manifest>",
    "geography_sha256": "<classification sidecar>",
    "boundary_sha256": "<IBGE boundary>"
  }
}
```

Use the published generation as the baseline. Omit it only before the
first publication. Rotate the configuration when the owner has approved
the next sources and reviewed their pins. Do not commit the private
configuration or report artifacts.

The command also runs directly.

```sh
charging-catalog review --config "$REVIEW_CONFIG" --output-dir "$REPORT_DIR"
```

Reports are written atomically with mode 0600 and retain their candidate
and baseline digests. With no output directory, the report goes to standard
output. If a verified transition has review items, the report is still
written and the command exits with an error. Resolve the review items
before requesting activation. Activation is a separate authorized command.

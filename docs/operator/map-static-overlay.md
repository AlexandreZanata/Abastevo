# Static map overlay hosting

The static map overlay is an immutable, release-addressed per-UF JSON view of
the active map generation, with the latest published official week's prices
embedded when available. It lets the CDN/web tier absorb map traffic while the
API only answers price taps.

## Release layout

```text
<map-root>/
  current -> <release>          atomic symlink, the only served path
  <release>/
    manifest.json                generation, price week, counts, file hashes
    AC.json, AL.json, ...        one file per UF; `??.json` for unknown state
```

`<release>` is `<sanitized-generation>-<content-sha256-prefix>`. Old releases
stay in place; the overlay is never rewritten in place.

## Builder

`backend/cmd/mapoverlay` reads the active `map_generations` row, its
`map_assertions`, the active CNPJ identifiers, and the latest
`official_current_revisions` week. It runs in a read-only transaction and never
writes to PostgreSQL.

```sh
# inside the staging cluster (DSN injected from the secret, never printed)
mapoverlay --output /srv/portal/map
```

On the staging VPS the host path is
`/opt/abastevo-temp/enterprise/portal/map` (uid/gid 65532, mode 700). The
builder writes `0644` JSON files and updates `current` atomically.

The manifest records:

- `generation` / `generation_id` / `source_revision` / `activated_at`;
- `price_week` (`survey_week_start`, `survey_week_end`) or `null`;
- per-file bytes, SHA-256, assertion count and priced assertion count;
- `content_sha256` over the release metadata and per-file hashes.

Each assertion carries source, kind, name, CNPJ, OSM identity, coordinates,
quality, display point, services, rights, capabilities and a `prices` array.
An absent price is explicit as an empty array; the overlay never claims live
status.

## Staging web tier

The same-origin web tier mounts the portal root at `/srv/portal`. Add this
handler before the portal `/dados/*` handler in the
`abastevo-temp-web-caddyfile` ConfigMap:

```caddyfile
	# Static map overlay releases are content-addressed and never change.
	handle_path /dados/mapa/* {
		root * /srv/portal/map
		header Cache-Control "public, max-age=31536000, immutable"
		file_server
	}
```

Then restart the web deployment:

```sh
kubectl -n abastevo-temp rollout restart deployment/abastevo-temp-web
kubectl -n abastevo-temp rollout status deployment/abastevo-temp-web
```

The handler strips `/dados/mapa`, so:

- `/dados/mapa/current/manifest.json`
- `/dados/mapa/<release>/PR.json`

resolve under `/srv/portal/map`. The edge keeps `X-Robots-Tag: noindex` and the
other security headers.

## Validation

```sh
curl -sSI https://teste.abastevo.com.br/dados/mapa/current/manifest.json
curl -sSI https://teste.abastevo.com.br/dados/mapa/current/PR.json
```

Expect `200`, `Content-Type: application/json`,
`Cache-Control: public, max-age=31536000, immutable` and
`X-Robots-Tag: noindex, nofollow`.

## Rollback

Old releases are retained. To roll back, atomically point `current` at the
previous release directory; the Caddy route does not need a restart:

```sh
ln -sfn <previous-release> /opt/abastevo-temp/enterprise/portal/map/.current.tmp
mv -T /opt/abastevo-temp/enterprise/portal/map/.current.tmp \
  /opt/abastevo-temp/enterprise/portal/map/current
```

Never edit a release directory in place. Rebuild with `mapoverlay` if the
generation or official week changes.

## Operational limits

- The builder is read-only against PostgreSQL; it does not import, publish or
  migrate.
- Run it as uid 65532; the map root and release files stay private on the host
  and readable by the Caddy process.
- Do not log the DSN. The staging apply uses a temporary pod with only
  `ANPFUEL_DATABASE_URL` from `abastevo-temp-secrets`.
- The current staging sample has 565 assertions, 19 UFs and 102 priced
  assertions for the 2026-10-18..2026-10-24 official week. This is pipeline
  coverage, not a national coverage claim.

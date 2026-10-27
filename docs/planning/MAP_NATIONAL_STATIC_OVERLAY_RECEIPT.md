# Static map overlay staging receipt

Status: **SERVED on staging only**. Namespace `abastevo-temp` on
`sorrimobi-vps`; no production namespace, deployment, DNS or app was changed.

## Release

- Release: `phase3-coverage-sample-20261026-5e8d5a74`.
- Map generation: `02ea6e7d-19ce-4942-a369-1aa7efdd03dd`.
- Source revision: `phase3-coverage-sample-20261026`.
- Activated at: `2026-10-26T23:04:03Z`.
- Assertions: 565; UFs: 19; assertions with official prices: 102; skipped
  unknown state: 0.
- Official week embedded: `2026-10-18`..`2026-10-24`.
- Release root on the VPS:
  `/opt/abastevo-temp/enterprise/portal/map/`.
- `current` symlink:
  `phase3-coverage-sample-20261026-5e8d5a74`.
- `manifest.json` SHA-256:
  `1856ce2c7503aeb9e6803d0792fa715bd3c0ce5dbda1cd8ab705fc156df6a0bc`.
- The release has 19 per-UF JSON files plus `manifest.json`; files are `0644`,
  the release root is private to uid/gid 65532.

The builder (`backend/cmd/mapoverlay`) reads the active generation and the
latest published official week in a read-only transaction. It did not write to
PostgreSQL, import data or apply a migration.

## Web tier

The `abastevo-temp-web-caddyfile` ConfigMap received a
`handle_path /dados/mapa/*` handler rooted at `/srv/portal/map`, with
`Cache-Control: public, max-age=31536000, immutable`. The web deployment was
rolled out.

- Caddyfile before SHA-256:
  `432f09123f40ee11778f52b1438eed94a112d3dd729a811bb675ff6927e9d1f8`.
- Caddyfile after SHA-256:
  `e333eb215139be2fe421575c78eaabadf3255ae698d7e4f78302d7ef7dc89011`.
- ConfigMap backup:
  `/opt/abastevo-temp/map-national-20261026/work/caddyfile-configmap-before.yaml`
  (SHA-256 `228035ef0eb53141113c8e0bb8548d10f09f3d748a24810da2c7ef69b7d57f51`).
- The first rollout attempt was blocked by the namespace CPU quota
  (`abastevo-temp-quota`, 4 CPU limit). Deleting the temporary overlay pod freed
  the quota; the second rollout completed and the old web pod was terminated.
  No other workload was scaled or changed.

## HTTP validation

- `https://teste.abastevo.com.br/dados/mapa/current/manifest.json`: `200`,
  `application/json`, `Cache-Control: public, max-age=31536000, immutable`,
  `X-Robots-Tag: noindex, nofollow`.
- `https://teste.abastevo.com.br/dados/mapa/current/PR.json`: `200`,
  `application/json`, immutable cache, `27621` bytes in the sampled response.
- The same headers were confirmed through the web service port-forward
  (`abastevo-temp-web:8080`) before the edge check.
- The overlay serves the OSM `osm-mapped` assertions from the active
  generation. The `/v1/map/assertions` price endpoint still accepts only
  `ANP`/`COMMUNITY`; the static overlay is the low-cost map path for these OSM
  pins. Consuming the overlay in the map UI remains a separate product step.

## Rollback

Old releases are retained. Rollback is an atomic `current` symlink switch to a
previous release; no Caddy restart is required. The release directories are
never edited in place. No rollback was needed for this receipt.

## Evidence

- Private VPS root:
  `/opt/abastevo-temp/map-national-20261026/work/`.
- Caddyfile before/after text and the ConfigMap backup are in that directory.
- The temporary overlay pod `abastevo-map-overlay-20261026` was deleted.
- No database DSN or credential was printed, committed or stored in Git.

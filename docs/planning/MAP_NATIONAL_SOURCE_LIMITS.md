# National source limits for bots and workers

Status: reference for Phase 2+ ingestion and matching workers. Observed on
2026-10-26. This document records the limits we actually hit or verified; it does
not invent an official limit where the publisher does not state one.

## ANP Revendedores API

- Endpoint: `https://revendedoresapi.anp.gov.br/v1/combustivel`
- Credentials: none. No 401/403 was observed.
- Observed behavior on 2026-10-26: 12 municipality requests at roughly one
  request per second succeeded; the 13th (`AC / MANCIO LIMA`) returned HTTP 429
  on all three attempts with 10/30/90 s backoff. The exact request and STOP
  record are in `inputs/api/STOP.txt` and `inputs/api/harvest-index.ndjson` on
  the Phase 1 VPS root.
- Published official limit: not found. Treat the limit as unknown and
  conservative.
- Worker policy:
  - one process and one request at a time; never parallelize this API;
  - start at one request every 5 s; if a 429 appears, stop the lane rather than
    increasing the rate;
  - retries only with 10/30/90 s backoff, then STOP on repeated 401/403/429;
  - cache every raw response by URL and SHA-256, and resume from the saved
    index;
  - never use `station-discover` for harvesting (it stages rows in the
    database); Phase 1/2 workers are file-only;
  - never print or commit credentials; this API needs none.
- Do not retry a stopped lane automatically. A new run needs an owner decision
  and a fresh observation window.

## OSM / Geofabrik

- `brazil-latest.osm.pbf` was 2,098,807,156 bytes on 2026-10-25; OSM timestamp
  2026-10-25T21:21:21Z.
- Download once per batch, verify the Geofabrik MD5 and our SHA-256, and keep
  one copy per private root. No credentials.
- `prepare_osm` default limits: 8 GiB input, 400,000,000 elements, 200,000
  matched. The observed Brazil run used 363,382,865 elements, 875,904 KiB peak
  RSS and 7m14s. Do not raise these limits without a measured need and a new
  review.
- One `prepare_osm` worker per PBF; do not re-parse the PBF per municipality.

## IBGE CNEFE 2022

- Official per-UF CSV ZIPs under
  `https://ftp.ibge.gov.br/Cadastro_Nacional_de_Enderecos_para_Fins_Estatisticos/Censo_Demografico_2022/Arquivos_CNEFE/CSV/UF/`.
- Pilot sizes: SP 1,082,015,432 B, PR 242,066,031 B, MT 37,772,175 B; data rows
  were 22,953,725 / 6,122,025 / 1,985,314 (header excluded).
- Public files, no credentials. Download each UF once, hash it, and stream the
  CSV; do not load a full UF into memory.
- Keep only the station municipalities' rows in a derived extract. The raw ZIPs
  stay in the private root, never in Git.

## Receita Federal CNPJ - Estabelecimentos

- Official landing page: `https://www.gov.br/receitafederal/dados`. The current
  public SERPRO+ share is a monthly folder (`2026-10` in this batch).
- The ten `Estabelecimentos0..9.zip` partitions are split by CNPJ root, not by
  UF, so all ten are needed to cover PR/MT/SP. The 2026-10 batch was
  5,416,929,969 bytes and 73,809,386 rows.
- Public share, no account; do not log the public share token. Four parallel
  file downloads completed in this batch; do not increase concurrency without
  a new observation.
- Cache by SHA-256 and month. Do not re-download a month for every worker.

## Overture Places

- Release used: `2026-10-21.0`; places theme 11,895,717,186 bytes in 16 Parquet
  files. In this release only parts 4 and 5 contained Brazil places.
- Use DuckDB with a bbox filter and the IBGE UF polygons; extract once to a
  private GeoParquet. D3 is undecided: Overture is evidence only, never sole
  proof.
- Do not scan all 16 files per worker when the release layout already prunes to
  the Brazil parts.

## National matcher (`prepare_national_matches`)

- Local files only: no network, database, clock or native engine.
- Hard bounds: 200,000 API records, 500,000 OSM references, 200,000 registry
  rows, 10,000 map assertions (the current `abastevo.map-assertions-v1`
  contract cap), 10,000 review rows and 4 MiB `reviews.ndjson` (the current
  `station-review` caps), 64 MiB per API JSON file, 1 GiB OSM/registry file.
- Matching: 0.0025 degree grid (about 250 m), 3x3 query, CNPJ and normalized
  address indexes, Haversine distance. L1 address radius 50 m; L1 CNPJ review
  distance 200 m (the `station-review` contract cap); L2 address uses 50 m when
  an ANP point exists and no distance when it does not.
- Ambiguous OSM ties are never resolved by picking one candidate. L3 ANP points
  remain decisions only while D1 is open; L5 never gets a pin.
- The sample Phase 1 inputs produced 361 L1 reviews, 565 L2 map assertions,
  1,574 L3 decisions and 43,188 L5 decisions in 1.12 s wall and 142,644 KiB
  peak RSS. A 1M-point index test completed in 2.98-8.72 s in release mode with
  594,360 KiB peak RSS.

## Stop conditions for any worker

Stop and report, do not raise limits or retry blindly, when:

- a source requests credentials or returns repeated 401/403/429;
- a hash, size or row count cannot be recorded;
- an input exceeds the documented bound;
- `prepare_osm` exceeds 8 GiB or 400M elements;
- the map assertion corpus exceeds 10,000 records;
- a proposed action would write to the database, apply a migration, deploy, or
  change nginx/k8s/app in a files-only phase.

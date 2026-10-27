# MC-A3 official point validation (B-BR-MAP-15)

Status: LOCAL_DONE. Date: 2026-10-27. Base: `23bc459b` plus this task.
Time: ~40 minutes.

## Rule and sources

- **B-BR-MAP-15** added to `docs/product/MAP_DISCOVERY_CONTRACT.md` before the
  code: finite WGS84, Brazil bounds, no zero/sentinel, SRID 4674/4326, no
  silent lat/lon swap, exact IBGE municipality, 500 m polygon buffer, shared
  cluster (>= 5 stations at the same 6-decimal coordinate), optional address
  candidate within 2,000 m. A failure removes the point; validation never
  upgrades a tier and never invents a replacement.
- IBGE malha municipal (simplified, v4 API):
  `https://servicodados.ibge.gov.br/api/v4/malhas/paises/BR?formato=application/vnd.geo+json&intrarregiao=municipio&qualidade=minima`
  -> 3,998,504 bytes, 5,571 features, SHA-256
  `5491bd548fe381cd388ba81c9edfbf4cf2749891e2fd32c1079ac71f7aba1c10`.
  The IBGE Malha Municipal Digital note licenses the product under
  **CC BY 4.0**; attribution "Fonte: IBGE" is retained.
- Municipality name/code table: the existing
  `portalgen/assets/ibge-municipalities.json` (5,571 entries; SHA-256
  `58192ed456ed2fabb18104887b6ddabce54295b0b431e7f77952d21cef727923`).
  Raw malha and the name table stay under the private VPS root; no raw data is
  in Git.

## Implementation

`tools/station-prep/src/anp_validation.rs` + `validate_anp_points`:

- loads the malha and resolves `(UF, folded municipality name)` to the IBGE
  code (accent-insensitive);
- builds the polygon rings and applies ray casting plus a 500 m point-to-edge
  distance buffer;
- counts shared rounded coordinates across the whole registry;
- has optional CNEFE/address candidates (`{cnpj, latitude, longitude}` NDJSON)
  with a 2,000 m maximum;
- writes deterministic `anp-validation.ndjson` and a manifest with input
  hashes, counts and per-reason totals. The output has no `quality`/`tier`
  field: validation can only reject a point.

## VPS results

Input: the MC-A2 normalized snapshot (45,807 rows).

| Status | Rows |
| --- | ---: |
| valid | 37,415 |
| no_point (L5 path) | 6,790 |
| rejected | 1,602 |

Rejected by reason:

| Reason | Rows |
| --- | ---: |
| `outside_municipality` | 1,318 |
| `shared_coordinate_cluster` | 196 |
| `municipality_unknown` | 87 |
| `outside_brazil` | 1 |
| `lat_lon_swapped` | 0 |
| `zero_or_sentinel` | 0 |
| `srid_invalid` | 0 |
| `address_candidate_too_far` | 0 (no candidate file in this run) |

Top rejected UF/reason pairs: RJ outside 649, MT outside 152, SP outside 108,
BA outside 51, MA outside 50, MG outside 48, RS outside 44, RS shared 34,
PA outside 31, BA shared 30. Valid pins per UF range from RR 142 to SP 6,888;
the full table is in the VPS manifest/NDJSON and the aggregate is reproducible
from it.

Validation took 1 s; the rerun output SHA-256
`e7da5cb8579435ee66861466220817482566bf1c70b84acd3f5798c1ac97e50b` was
identical. The RJ outside cluster is the main finding; those points do not pin
and fall to the address tiers/residual queue.

## Verification

- `cargo test --lib` -> 112 tests (2 new validation tests covering every
  reason, deterministic rerun and the no-tier property).
- `scripts/check-rust-fast.sh` -> `check-rust-fast: ok`.
- No database, deployment or app change.

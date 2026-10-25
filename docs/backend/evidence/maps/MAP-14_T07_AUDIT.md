# MAP-14 T07 — Dependency/license/SBOM/endpoint audit

Status: **LOCAL_SOURCE_ADMISSION_PASS**
Validation: PASS for exact local code/resource admission, notices, expanded runtime-code scan and update recipe. Current hashes and qualified engine acceptance are in [MAP-14_ENGINE_GATE](MAP-14_ENGINE_GATE.md).
Date: 2026-10-24. Upstream: `4587b8ad4d12c9f59982709fdaf03cc925948827`.

Current admission supersedes the older owner-signoff request below. The exact
asset profile excludes Code2000/KhmerOS, admits 193 assets and embeds 231 source
license files/full upstream terms. Both ABI builds are independently
reproducible. The runtime scan expands compressed Java/JNI entries: 36 combined
hosts, none unknown, no secret-pattern hits. The five SDK social/source URLs
are documented unused references. The retained source offer distinguishes
OSM-derived reference assertions from independently obtained ANP/community
records; separate storage alone is not a license boundary. Public source-offer
publication remains required before public distribution. The just_gtfs MIT
grant is present in ancestor `083d98e`, not merely a later tree. Updated durable
JSON/notices below are current; numeric hashes and owner requests in the earlier
chronology are historical, superseded by the versioned receipt.

## Earlier audit chronology

## Source inventory

- `git submodule status --recursive`: 193 entries, all clean at their pinned revisions.
- 22 additional vendored `3party` directories are not submodules (for example
  `monocypher`, `agg`, `kdtree++`, `minizip`, `stb_image` and
  `vulkan_wrapper`).
- 215 components total: 204 with a license/notice file and 11 resolved from an
  exact source header or a retained repository license. **0 unresolved.**
- Upstream `LICENSE`, `NOTICE` and `data/copyright.html` are retained beside the
  candidate; the Abastevo repository MIT license is included in the bundle.

Resolved items:

| Component | Resolution | Evidence |
| --- | --- | --- |
| `3party/glm` | MIT (dual Happy Bunny/MIT; MIT selected) | `util/conan-package/lib_licenses/LICENSE2.txt` |
| `3party/just_gtfs` | MIT | Pinned tree `7516753` predates `LICENSE.MIT`; upstream `comaps/just_gtfs` `b5b1df6` has MIT `LICENSE.MIT`, retained in `maps/engine/upstream/licenses/just_gtfs-MIT.txt` |
| `3party/monocypher` | BSD-2-Clause (BSD-2-Clause or CC0 offered) | `3party/monocypher/LICENCE.md` |
| `3party/GL` | MIT (Khronos) | `3party/GL/glcorearb.h` |
| `3party/agg` | `LicenseRef-AGG` | `3party/agg/agg_vcgen_smooth_poly1.h` |
| `3party/ankerl` | MIT | `3party/ankerl/stl.h` |
| `3party/kdtree++` | Artistic-2.0 | `3party/kdtree++/region.hpp` plus retained `Artistic-2.0.txt` |
| `3party/minizip` | `Zlib AND Info-ZIP` | `3party/minizip/src/MiniZip64_info.txt` plus retained `Info-ZIP.txt` |
| `3party/opening_hours` | MIT | `3party/opening_hours/opening_hours.hpp` |
| `3party/robust` | `LicenseRef-PublicDomain` | `3party/robust/predicates.c` |
| `3party/skarupke` | BSL-1.0 | `3party/skarupke/__flat_hash_map.hpp` |
| `3party/stb_image` | MIT (MIT or public domain; MIT selected) | `3party/stb_image/stb_image.h` |
| `3party/vulkan_wrapper` | Apache-2.0 | `3party/vulkan_wrapper/vulkan_wrapper.h` |

The full 219-license-file notice bundle is reproducible from the command below. The
committed durable artifacts are:

- [THIRD_PARTY_NOTICES.md](map14-t07/THIRD_PARTY_NOTICES.md), SHA-256
  `b8c837e4e7ecb2d960a1f52a1a29e0c3068f2479145309f7dd09cceee9ad1294`;
- [sbom.cdx.json](map14-t07/sbom.cdx.json), CycloneDX 1.5, 219 components,
  SHA-256 `63d468c4de8306e9838fab8a0b8da25f25b924411b54d986b52da7d880596f08`;
- [audit.json](map14-t07/audit.json), SHA-256
  `91a8247621679aa25a79204f8b2b71f885249ed01c3594a9a6b6c80846d0bd60`;
- [SOURCE_OFFER.md](map14-t07/SOURCE_OFFER.md), SHA-256
  `1c9b4ee84bfa8d6e739bc4b6fa4652e8de26dbdc344baf967b860cccb07e5cdf`.

The generated bundle contains 219 license files (226 files total, 3.0 MB); its
deterministic tar SHA-256 is
`c2d84594a4e32bc9f8d8909fc57c908fb8b579500d869a9f616764a491a401a8`.
Regenerate it with the audit script; the source tree retains every file.

## Artifact scan and endpoints

The scanner examined the actual release candidate (`sdk-release.aar`,
`classes.jar`, `jni/arm64-v8a/liborganicmaps.so`):

- no private-key/API-token pattern hits;
- 30 URL hosts in the native library, 0 unclassified:
  - 10 `UPSTREAM_SERVICE_UNUSED` (comaps.app/at, CDN/mapgen hosts,
    `comaps.openstreetmap.fr`, `comaps-cdn.s3-website.cloud.ru`);
  - 12 `OSM_DATA_OR_SCHEMA` (OSM/API, OGC/W3C, Wikimedia, Panoramax,
    MapComplete, Mangrove, Garmin/Earth reference);
  - 8 `SOCIAL_OR_REFERENCE`.
- Decision: `DOCUMENTED_UNUSED`, not neutralized. The Abastevo adapter does not
  configure a CoMaps account/key or invoke the upstream download service; the
  map packages are local. The T08 airplane-mode render/search/route proof
  demonstrates that none of these endpoints is required for the tested offline
  path. The strings remain in the native binary and must stay covered by this
  decision until a later cleanup changes the binary.
- No private service or key is required for the tested path.

## ODbL / derived database

The regional `.mwm` package is derived from OpenStreetMap data under ODbL 1.0.
The map displays `Map data © OpenStreetMap contributors`; the exact ODbL text
is retained as `ODbL-1.0.txt` in the notices bundle. The package is a Derived
Database; the exact generator SHA, patch, build recipe and package bytes are
retained. The public source/derived-data offer URL at the distribution endpoint
remains a release/deployment obligation.

The Abastevo overlay (station identities, prices, charging/community records) is
a separate Abastevo database. It is not merged into the OSM data and is not
relicensed as ODbL; it is displayed as a separate layer. This is a technical
determination, not a legal opinion.

## Patch reapplication and rollback

`maps/engine/upstream/README.md` records the exact-pin patch rules. The builder
applies `ndk-r28-lld.patch` only when the source is unpatched, or accepts it when
the reverse check succeeds; it refuses patch drift. Rollback:

```sh
git -C /data/dev/src/comaps-4587b8ad apply --reverse --check \
  /data/dev/worktrees/abastevo-map-ev/maps/engine/upstream/ndk-r28-lld.patch
git -C /data/dev/src/comaps-4587b8ad apply --reverse \
  /data/dev/worktrees/abastevo-map-ev/maps/engine/upstream/ndk-r28-lld.patch
```

A changed upstream SHA or ABI set invalidates this audit, the notices/SBOM and
the T08 evidence. Re-run the builder and this audit from the new pin.

## Commands

```sh
python3 scripts/maps/audit-comaps-release.py \
  --source /data/dev/src/comaps-4587b8ad \
  --candidate /tmp/abastevo-comaps-release-arm64 \
  --out /tmp/abastevo-comaps-release-arm64/audit.json \
  --notices-dir /tmp/abastevo-comaps-release-arm64/notices \
  --sbom /tmp/abastevo-comaps-release-arm64/sbom.cdx.json
# 193 submodules clean, 22 vendored, 215 components,
# 204 license files + 11 resolved, 0 missing, 30 endpoints, 0 unclassified.
```

## Remaining owner/legal work

- Human legal acceptance of the resolved `glm`/`just_gtfs`/vendored choices and
  the ODbL derived-database/overlay determination.
- Publish the durable source/derived-data offer at the release distribution
  endpoint.
- Final per-asset font/symbol/style/data admission review against
  `data/copyright.html` and the admitted resource manifest.
- Re-run after any engine SHA/ABI/patch change; this audit is not a substitute
  for ENGINE_GATE runtime evidence.

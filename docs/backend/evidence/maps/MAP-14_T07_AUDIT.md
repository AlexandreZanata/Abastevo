# MAP-14 T07 — Draft dependency/license/endpoint inventory

Status: IN_PROGRESS / AUDIT_DRAFT_NOT_LEGAL_ACCEPTANCE
Validation: PARTIAL (inventory and artifact scan only)
Date: 2026-10-24. Upstream: `4587b8ad4d12c9f59982709fdaf03cc925948827`.

This is a mechanical draft for the remaining T07 review. It is **not** a legal
opinion, a transitive right admission or an ENGINE_GATE closure.

## Source and submodules

- HEAD matches the pinned upstream revision.
- `git submodule status --recursive`: 193 entries, all clean at their pinned
  revisions.
- 191/193 submodules have an own or inherited license/notice file under the
  audit heuristic. Missing explicit files: `3party/glm` (readme only in this
  checkout) and `3party/just_gtfs` (no root license file found); legal review
  must resolve these two before admission.
- Upstream `LICENSE`, `NOTICE` and `data/copyright.html` were copied beside the
  release candidate by the builder. The candidate report still says
  `licenses: AUDIT_PENDING`.

## Artifact scan

Scanned the actual release candidate (`sdk-release.aar`, `classes.jar`,
`jni/arm64-v8a/liborganicmaps.so`):

- no private-key/API-token patterns were found by the bounded scanner;
- the native library contains default upstream endpoint strings, including
  `comaps.app`, `cdn-*.comaps.app`, `comaps-cdn.s3-website.cloud.ru`,
  `comaps.openstreetmap.fr`, `api.openstreetmap.org`, social/identity hosts and
  OGC/W3C schema URLs;
- the Abastevo adapter does not configure a CoMaps account/key and does not use
  the upstream map-download service, but these defaults remain in the binary and
  must be explicitly neutralized or documented as unused before release;
- OSM/ODbL attribution and every shipped third-party notice still need a
  human-reviewed inventory.

## Commands

```sh
python3 scripts/maps/audit-comaps-release.py \
  --source /data/dev/src/comaps-4587b8ad \
  --candidate /tmp/abastevo-comaps-release-arm64 \
  --out /tmp/abastevo-comaps-release-arm64/audit.json
# 193 submodules clean; 191 with license/notice evidence; legal_review=PENDING
```

## Remaining T07 work

- Resolve `3party/glm` and `3party/just_gtfs` licenses against the exact shipped
  source.
- Produce the final SBOM and NOTICE bundle, including asset/font/symbol/data
  licenses and ODbL derived-data determination.
- Decide and record whether upstream endpoint strings are removed, overridden or
  declared unreachable.
- Record patch reapplication/update/rollback and the human license sign-off.

# Map/EV planning and prepared asset validation

Date: 2026-10-24. Status: PLANNING_LOCAL_DONE / IMPLEMENTATION_PLANNED.
Validation: PASS for planning structure and prepared artwork only.
Audited source/base: b90a0cd1277f0728d60fdf47bb1b62bc91613416.

## Scope

The user requested data-first complete phases, a precise execution prompt for a
less capable agent, commercial/open-only resources, a CoMaps-inspired/native
reuse direction, a fourth Android Mapa tab and two generated station pictograms.
The user then explicitly requested removing an unwanted hole in the electric SVG.

No product runtime implementation, source admission/activation, generator build,
map data acquisition, device/VPS mutation or remote delivery occurred. MAP-00–33
are all PLANNED. Native extraction, dependency admission, real route behavior,
minimum map and production/device gates remain unproven.

## Planning checks executed

- Parsed MAP_EV_PHASES.json: 34 ordered unique IDs MAP-00–33, all PLANNED;
  dependencies exist and precede consumers; task steps/checks/exits/evidence set.
- Matched 34 Markdown phase definitions to registry; checked internal companion
  links and explicit DATA/ENGINE/MINIMUM/OFFLINE/PERFORMANCE/DELIVERY gates.
- Reviewed baseline navigation/source/data code and current progress; compared
  runtime EV gaps with the recorded actual API deployment evidence.
- Reviewed official CoMaps source/license/notices/build/feature documentation,
  OSM/Geofabrik, official ANP survey pages and OCM rights. Exact upstream SHA,
  transitive/per-provider admission remains an execution requirement, not a
  claimed licensing clearance for every candidate input.
- Parsed both new build-time Python scripts with ast.parse; reviewed local-only
  subprocess argument lists, paths and no network/runtime dependency behavior.
- Verified git diff --check and absence of credential/private-key token patterns
  on new docs/scripts/artifacts. All art is generic; no contributor PII/GPS/photo
  or operator logo was added.

## Artwork checks executed

Two distinct built-in ImageGen calls produced raw transparent PNGs. Raw images
and exact prompts were retained, not overwritten by normalization. The existing
fuel/home pipeline informed one category ink + white details and actual SVG paths.
No embedded bitmap, script, font, gradient or external reference is in either
icon master. These are station service symbols, not new fuel enum artwork.

Ran trace-map-icons.py for both source images and exported with Inkscape. Both
pre-correction traces passed silhouette/ink/white mask IoU >=99% against their
normalized source-derived masks (final trace scores 1.0/1.0/1.0). An earlier
electric white-mask result was below99%; increasing trace sampling resolved it
without weakening the threshold.

The requested electric correction removed exactly one enclosed accidental teal
path subpath. The guarded repair asserted its signature and bounds, re-rendered
the final SVG and proved changes were confined to normalized pixel bounds
[296,548,411,651]. All pixels outside that region stayed unchanged. Interior
RGBA was verified as (0,121,107,255); display, bolt, plug and other geometry stayed.
Pre-correction normalization and raw source remain retained; the final normalized
PNG is explicitly rendered from the corrected SVG. Old mask IoU is not passed
off as fidelity of the intentional edited shape.

Reproduced into a fresh temporary output using these real commands:

```sh
python3 scripts/assets/trace-map-icons.py --source docs/assets/map-icons/sources/ic_map_station_electric.png --name ic_map_station_electric --color '#00796B' --output /tmp/abastevo-map-reproduce
python3 scripts/assets/fix-map-electric-svg.py --assets /tmp/abastevo-map-reproduce
```

Compared SHA-256: corrected SVG, corrected normalized PNG and Android PNG were
byte-identical to the delivered copies. Raw/SVG/Android hashes match provenance.
Inspected exported 24/48/128px preview on light/dark neutral backgrounds before
and after correction; the unwanted hole is gone in both. Inkscape's sandboxed
recent-file/log warnings did not prevent rendering or affect output checks.

Actual map-background/20px/selected-state/device accessibility checks stay owned
by MAP-18. No app compilation, runtime test or national coverage claim is made:
no app source was changed, and APK/device execution would not validate a plan.

## Delivery state

Planning documents, ROADMAP/PROGRESS links, art and reproducible build-time scripts
are isolated from other worktrees. A local planning/artifact commit is permitted;
no planning-only push/PR/merge/wiki is requested here. The future implementation
prompt explicitly describes its own construction/delivery scope when the user
chooses to run it. Start that work with MAP-00 actual-baseline reconciliation.

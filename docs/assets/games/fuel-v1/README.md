# Fuel game assets — V1

Status: ASSET_DRAFT_LOCAL_DONE / INTEGRATION_PENDING, 2026-10-09.
Scope: GME-00 in [the separate Games plan](../../../planning/GAMES_OFFLINE_PLAN.md).
These are genuine static 3D meshes, not PNGs placed inside a GLB container.
Game implementation, sprite atlases, animation and device acceptance are planned.

Open [preview.html](preview.html) in a WebGL browser to select, rotate, zoom and
download each embedded GLB. The file is self-contained and works offline without
a CDN or web service. [Contact sheet](contact-sheet.png) is the static fallback.
The preview uses simplified directional lighting; the GLBs carry PBR materials.

## Contents and measured budgets

Eight models total **731,128 bytes (714.0 KiB)** and **13,940 triangles**:

- [Common gasoline](gasoline_regular.glb): 1,868 triangles / 97,932 bytes.
- [Additive gasoline](gasoline_additive.glb): 1,912 triangles / 99,928 bytes.
- [Premium gasoline](gasoline_premium.glb): 1,892 triangles / 99,092 bytes.
- [Ethanol](ethanol.glb): 456 triangles / 27,768 bytes.
- [Diesel S500](diesel_s500.glb): 2,336 triangles / 120,908 bytes.
- [Diesel S10](diesel_s10.glb): 2,024 triangles / 105,592 bytes.
- [CNG](cng.glb): 2,448 triangles / 126,472 bytes.
- [LPG P13](lpg_p13.glb): 1,004 triangles / 53,436 bytes.

All fit the GME-00 art budget: 128 KiB and 3,000 triangles per model; pack
under 1 MiB. These are measured art sizes, not a game/APK/memory benchmark.
There are 2–3 primitives/materials per model, no external references, textures,
fonts, skins, cameras, lights, extensions or animations. All positions, normals
and indices are embedded in the GLB. Indexed flat normals preserve the low-poly
style; material grouping avoids a draw call for every decorative part.

## Provenance, categories and license

Original procedural modeling adapts the repository's existing fuel PNG artwork:
rounded pump/plinth, right hose/nozzle, droplet, premium crown, ethanol leaf,
CNG tank and LPG cylinder. It is a stylized 3D adaptation rather than a
pixel-faithful trace. No image-generation model, purchased model, copied game
character or new runtime dependency was used. MIT, with repository notices
preserved. Names are generic fuel categories, not commercial fuel-brand assets.

Actual normalized PNGs under `docs/assets/fuel-icons/transparent/` are the
references, with SHA256 in [manifest.json](manifest.json). In particular, the
current CNG PNG is gold while its historical SVG has green paths; this pack
follows the visible PNG, not that stale SVG ink. Existing sources are untouched.
Category accents: common gasoline orange, additive gold, true premium ruby,
ethanol green, S500 blue, S10 violet, CNG yellow-gold and LPG orange. White and
charcoal are neutral. Silhouettes/badges and text labels supplement colors;
color alone must never identify fuel in gameplay or accessible UI.

The legacy enum `GASOLINE_PREMIUM` means **additive gasoline**. The distinct
premium asset binds to `GASOLINE_PREMIUM_GRADE`; manifest mappings preserve that
existing domain meaning. Never derive enum names from the display filenames.

## Import and rendering contract

- glTF 2.0 binary `.glb`, right-handed, Y up, front +Z, base Y=0, centred near
  X=0. Units are metres at artistic prop scale, not engineering dimensions or
  literal cylinder capacity. Bounds are recorded per file in the manifest.
- Static render props; shader uses category/ivory/charcoal PBR materials, opaque,
  metallic 0 and roughness 0.38/0.55. No transparency sorting or texture upload
  required. The browser viewer approximates the PBR appearance.
- Relief details and pump parts intentionally intersect; these are render
  assemblies, not certified single watertight solids or physical collision
  meshes. Games must define simple fair collision primitives separately.
- For 2.5D, render selected views into tightly cropped transparent atlases at
  actual target pixel size; add animation/LOD only as a separate selected task.
  Do not ship this documentation PNG/HTML or both unused GLB masters and atlases.
- For real 3D, first select a measured renderer/loader, verify import/materials
  and lifecycle on actual Android devices, and budget resident geometry/GPU memory
  and instance count. This desktop preview is not Android performance evidence.

## Reproduction

From the repository root:

```sh
python3 scripts/assets/build-fuel-game-assets.py
python3 scripts/assets/build-fuel-game-assets.py --preview
```

The first command uses only the Python standard library for GLBs, manifest and
HTML. The optional contact-sheet renderer uses existing workstation Pillow
12.2.0 / NumPy 2.4.6 (documentation-only tooling; nothing added to app/build
dependencies). Generation is deterministic for the recorded Python 3.12.2
environment, including the source hashes. Re-run from a clean owned working
tree and inspect changes instead of overwriting another task's art. The viewer
template is [here](../../../../scripts/assets/fuel-game-viewer.template.html).

## Local verification and limits

At the recorded dev baseline `4725ebcb65bc065f6c6db3c40929b38b7548880a`:

- Generated every GLB and repeated generation to check byte-identical outputs.
- Checked glTF header/chunks/alignment, accessor/index ranges, finite positions,
  unit normals, stored bounds, materials, triangle/byte budgets, no external
  resources, exact category coverage and source/output SHA256 values.
- Independently loaded all eight models with existing workstation trimesh;
  verified triangle totals, winding consistency and positive oriented volumes.
  This is not a Khronos-validator or Blender/device certification claim.
- Inspected the contact sheet and actual browser WebGL rendering; selected
  categories and camera/rotation controls; no engine or service dependency.
- Checked Python/JavaScript syntax, local Markdown links, whitespace and scoped
  secret surface. Active app resources and code are unchanged; no aggregate
  Android/backend tests or release APK were run for this art/docs-only task.

The exact manifest pins the delivered artifacts. Current execution state is in
[PROGRESS](../../../planning/PROGRESS.md); local art completion does not certify
gameplay, fraud prevention, backend integration, national ranking or release.

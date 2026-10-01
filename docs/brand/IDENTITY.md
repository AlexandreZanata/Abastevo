# abastevo brand identity

Decision date: 2026-10-01. Name and supplied logo: SELECTED BY MAINTAINER. README V2 composition: APPROVED FOR PUBLICATION by maintainer request 2026-10-01. Scope: publish the selected identity, native vector assets and README through the guarded phase workflow.

## Name and mark

Write the project name **abastevo**, in lowercase. The source logo is the maintainer-supplied blue gradient A crossed by a green ribbon. [Original logo](../assets/brand/abastevo-logo-source.png) is copied byte-for-byte; generation never replaces that source. The banner arranges the supplied mark with the wordmark; P00-T03 supersedes the first lowercase composition with the supplied uppercase lettering. It is not a new logo proposal.

## README proposal

Prepare a wide, minimalist header with a white background, generous whitespace, the supplied mark and a dark navy uppercase wordmark traced from the P00-T03 reference. Preserve the logo's contour, blue/green palette and proportions. No slogan, icon collection, fake app screenshots, feature/release badges or product claims inside the artwork. The name must remain legible at GitHub README width. Use a responsive Markdown image with descriptive alt text after visual approval.

The [first generated composition](../assets/brand/abastevo-readme-banner-v1.png) is preserved as the earlier draft. The current approval draft is the [native-vector V2 composition](../assets/brand/abastevo-readme-banner-v2.svg), combining the unchanged vector logo with the reference lettering. [README visual preview](../../README.brand-preview.md) shows placement; the canonical README name is updated locally, but the maintainer approved V2 publication on 2026-10-01 and the canonical README now uses the combined SVG. The [exact generation prompt](../assets/brand/readme-banner-prompt.txt) and generation-provenance.json live with the draft assets. Built-in imagegen was used; no paid CLI/API fallback was invoked. Keep approved assets under docs/assets/brand so repository and wiki references do not depend on a local generation cache.

## Boundaries

Keep com.anpfuel, existing modules, binary identifiers, repository/API addresses, historic release names, upstream attribution and MIT notices. Current project docs use abastevo; historical evidence retains the names used at the time. No claim of trademark registration, domain ownership, affiliation or a changed logo license follows this selection. Existing TRADEMARKS and LICENSE notices remain the rights references.

## Task P00-T01

Bounded task: local project identity documentation plus one reviewable README artwork proposal. Branch codex/phase-00-abastevo-brand-preview, isolated from the ongoing P17 effort. Risk: docs/assets. Validate source-byte preservation, exact wordmark, legibility, local image links, unchanged code/license, whitespace and secret surface. No domain/runtime behavior changes, dependency additions or broad app/backend test campaign. Final step is the maintainer's visual approval; no GitHub issue, PR, merge or wiki publication is created for this preview.

## Local validation

2026-10-01: banner inspected visually (2172 × 724, RGB PNG, 769117 bytes); exact lowercase wordmark and supplied-mark identity recognizable. Original RGBA logo source hash matches the attachment byte-for-byte. README/identity relative links, provenance hashes, prompt record, naming, unchanged code/LICENSE and `git diff --check` pass; secret scan passes. The inherited broken git-ignored plan link was replaced with the canonical ROADMAP link in both README versions. No domain/backend/Android behavior changed and no aggregate test/remote publication was run for this local preview.

## Task P00-T02: original-logo vector assets

User request 2026-10-01: vectorize the supplied original logo for future real app use, retaining its recognizable contours, blue/green gradients and transparent background. Scope: native SVG paths/gradients plus platform-ready vector exports when supported; no embedded bitmap masquerading as vector, no active app-icon/package/runtime change. Work stays in the existing isolated brand worktree; README banner approval is still separate and pending.

Acceptance: inspect silhouette and internal folds against the original; render small and large sizes on light/dark backgrounds; check SVG safety and absence of raster/external references; preserve original PNG bytes. Document formats, optical differences from raster reconstruction and future Android/iOS import boundaries. Asset creation does not certify a shipped app icon.


2026-10-01 outcome: P00-T02 LOCAL_DONE. The [native SVG master](../assets/brand/abastevo-logo.svg), [Android VectorDrawable](../assets/brand/abastevo_logo_android.xml), [iOS vector PDF](../assets/brand/abastevo-logo-ios.pdf) and [preview](../assets/brand/abastevo-logo-preview.png) are saved with [import instructions and evidence](../assets/brand/README.md) and [vector provenance](../assets/brand/vector-provenance.json). Original bytes preserved; transparent paths/gradients contain no bitmap. SVG rendered at 32–4096 px; original comparison/light-dark inspection, SVG safety, Android geometry/gradient parity and aapt2 compile/link passed; PDF has zero raster images. Silhouette IoU 0.994204 is an outline metric, not pixel-exact/color equivalence. PNG-derived curves/gradients are reconstructed, so small optical differences remain. No app/runtime resources, dependencies or package identifiers changed; device adoption remains future work. README banner still awaits approval; no remote publication.


## Task P00-T03: reference wordmark and vector README composition

User request 2026-10-01: reproduce only the ABASTEVO lettering from the new reference; ignore its logo. Trace the eight uppercase letter shapes into native paths, without font substitution. Compose that independent SVG with the unchanged P00-T02 logo master in the existing horizontal README layout; retain the white background and navy text. Scope: wordmark/vector composition/preview assets only, no runtime branding or remote publication. Acceptance: all eight glyphs and counters preserved; no raster/font/external content in either SVG; logo master bytes unchanged; render and visually inspect the composition, then present for approval. The project name in prose remains lowercase; uppercase applies to this artwork.


P00-T03 outcome 2026-10-01: LOCAL_PREVIEW_DONE / AWAITING_VISUAL_APPROVAL. [Wordmark SVG](../assets/brand/abastevo-wordmark.svg) contains the eight reference glyphs as paths, with counters preserved and no font dependency. [Combined README SVG](../assets/brand/abastevo-readme-banner-v2.svg) retains the existing white horizontal layout and navy palette; [PNG preview](../assets/brand/abastevo-readme-banner-v2.png) is only a raster render. Original logo master hash and copied path/gradient geometry are unchanged. SVG safety, local renders and reference lettering silhouette overlap (IoU 0.990820 at 415 × 50 px) passed. Raster-derived contours are reconstructed rather than mathematically exact. No runtime change or remote publication. [Provenance](../assets/brand/wordmark-provenance.json) records source hash and checks; README.brand-preview.md now points to V2.


Publication authorization 2026-10-01: the maintainer requested updating GitHub with the artwork and README. V2 is approved; earlier pending-approval notes are historical task evidence. P00-T01–T03 are LOCAL_DONE / AWAITING_PHASE_MERGE; no runtime resources, release tags or deployment are in scope.

## P00-T04 — Native app launcher icons

Maintainer request 2026-10-01: adopt the existing vector A alone, unchanged in shape/gradients, on a white background as the app icon. Scope: Android adaptive/color/themed/fallback launcher resources and manifest round icon; iPhone/iPad AppIcon asset catalog plus explicit host wiring instructions. Preserve master SVG, identifiers and runtime behavior. Current iOS host is an SPM shell with no Xcode application project; asset catalog preparation is not a compiled/shipped iOS launcher claim. Validate crop-safe placement, resource compilation, opaque iOS sizes/catalog coverage, independent SVG source preservation and visual previews. No screenshot/banner wordmark in the icon.

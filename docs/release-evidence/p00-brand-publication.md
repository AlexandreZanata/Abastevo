# P00 approved brand publication

Maintainer selected abastevo/original mark, requested native vector assets and reference lettering, and authorized GitHub artwork/README publication on 2026-10-01. Task commits: P00-T01 b3b7de6 (#63); P00-T02 9903f70 (#64); P00-T03 9dd10f4 (#65). Base integrated by normal merge of origin/main 047a347; phase draft PR #66, milestone 11. Runtime resources/packages/licenses preserved. No release/deployment in scope.

Acceptance evidence: [identity](../brand/IDENTITY.md), [asset import/check record](../assets/brand/README.md), vector-provenance.json and wordmark-provenance.json. Original source bytes preserved; native SVG/Android geometry and gradients checked; Android resource compiled/linked; iOS PDF has zero raster images; SVG rendered 32–4096 px; lettering IoU 0.990820. Current publication tree passes local links, artifact hashes, native SVG safety/no raster/font/external references, whitespace and scoped secret review. Required quick gates and guarded merge remain pending on the final head/base.

## Preserved local preview history

The following records predate approval/publication; their pending states are historical.

- Task P00-T01: selected name abastevo and supplied logo; local documentation plus README artwork awaiting visual approval.
- Branch codex/phase-00-abastevo-brand-preview; worktree .worktrees/abastevo-brand-preview; base origin/main be680d1. Ongoing P17 checkout remains separate.
- Publication scope LOCAL_PREVIEW_ONLY; no remote issue/PR/merge/wiki update. This preview does not advance a functional or production gate.
- Artifact: docs/assets/brand/abastevo-readme-banner-v1.png; README.brand-preview.md. Original logo copied byte-for-byte; no runtime code changed.
- Evidence/approval source: docs/brand/IDENTITY.md. State LOCAL_PREVIEW_DONE / AWAITING_VISUAL_APPROVAL; not INTEGRATED.

- P00-T02 LOCAL_DONE on the same isolated branch/base (uncommitted): native SVG (~23 KB), Android XML and iOS vector PDF saved under docs/assets/brand; original PNG preserved. Evidence/imports: docs/assets/brand/README.md and vector-provenance.json.
- Vector checks: 32–4096 px render/visual inspection, SVG safety, SVG/Android path-gradient parity, Android aapt2 compile/link and PDF zero-raster inventory PASS; no device/runtime integration. Next: review brand preview, then authorized publication separately; banner approval remains pending.

- P00-T03 LOCAL_PREVIEW_DONE / AWAITING_VISUAL_APPROVAL (same isolated branch; uncommitted): reference uppercase text traced as independent SVG; V2 README combines both native SVG assets. Original-logo bytes/geometry unchanged; SVG safety, renders and glyph IoU 0.990820 PASS. Evidence: docs/brand/IDENTITY.md / wordmark-provenance.json. No remote publication or runtime change.

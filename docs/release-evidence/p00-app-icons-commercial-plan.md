# P00 continuation — icons and commercial direction

Scope authorized on 2026-10-01: finish the approved A launcher icon, plan the commercial community experience, explicitly defer iOS, and align wiki Home with README. Branch `codex/phase-00-app-icons`; base `9c090c6577b9cedb6119f9d5f8f8de96f2c6aa96`; milestone 12. Primary checkout contains unrelated changes and remains untouched.

## P00-T04 — native launcher adoption

- Default icon: white background, unchanged approved vector A, no wordmark. Android native adaptive foreground, API33 monochrome layer, round reference and five density fallbacks; iOS opaque AppIcon catalog prepared.
- Source SHA256: `706fc79d36df750838256dab85f05791eeb8476dc031717dd6c3559c6d57fe0d`. Byte equality against integrated master PASS; native paths/clips/gradient equality PASS; no raster embedding or external SVG resources.
- Alpha silhouette fitted inside 32.5dp radius, below the 33dp safe-zone radius; square/rounded/circular previews visually inspected.
- Ten Android density exports and eighteen iPhone/iPad/marketing slots: dimensions, RGB opacity and white corners PASS.
- `./gradlew :app:assembleDebug --no-daemon`: BUILD SUCCESSFUL, 50s, 67 tasks. Whitespace and scoped secret-surface review PASS.
- iOS assets are prepared, not runtime accepted: no Mac and no Xcode application target. Workstream deferred by explicit user request; only explicit resumption authorizes further iOS implementation.
- No identifiers, permissions, runtime data, business rules or license notices changed. No broad backend suite for launcher assets.

## P00-T05 — planning and wiki Home

Pending planning task: commercial community UX, phased backend/app contracts, Android release prerequisites, preserved iOS backlog and explicit Home adoption. This task does not implement the planned redesigned experience or certify production.

## Integration

Task issues/PR, exact tested head/base, required checks, guarded merge and wiki snapshot will be recorded at batch closure. No deploy/tag/pilot.

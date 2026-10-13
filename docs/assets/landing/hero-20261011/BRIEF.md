# Landing artwork and theme — LH01–LH03

Status: LOCAL_DONE
Validation: PASS
Integration: INTEGRATION_PENDING
Base: dev 9fa7ad1; isolated codex/landing-hero-art because security remediation occupies primary dev.
Scope: landing presentation and asset build only; no backend/mobile runtime or deployment.

## Authority and behavior

The user's 2026-10-11/12 instructions replace the illustrative hero comparison
with an authentic Android-home mockup, align desktop artwork with the headline
and description, move ANP/community badges outside the screen, add a stylized
Brazil with symbolic station lights, and illustrate six information cards.
The latest instruction replaces navy theme colors with black/grayscale and
white emphasis, fixes mutually visible sun/moon icons, and regenerates the
check/clock and contribution-camera illustrations.

B-BR-L01/L02: original screen pixels and source/week stay unchanged; generated
map lights are artistic, not station coordinates or coverage evidence.
BUC-L01/L02: source distinction and prelaunch status remain semantic HTML.
BUC-LH01/LH03: 44px keyboard-accessible control shows exactly one action icon;
system default, explicit local choice, blocked storage and cross-tab reset work.
Dark theme tokens are all neutral grayscale and meet AA contrast.

## Selected assets

Built-in image_gen generated [Brazil master](brazil-map-master.png) and the
six [raw pictograms](icons/sources). The final [prompt set](prompts) contains
all seven selected prompts; check/camera were regenerated on 2026-10-12.
No new dependency or CLI image-generation fallback was used.

The original Android explore-dark derivatives are untouched. CSS provides the
device frame; HTML provides source badges and the illustrative caption.
Desktop copy/stage share a grid row and identical height. Mobile stacks them.
Two hash-named Brazil WebPs ship (26,180 and 56,136 bytes).

[scripts/assets/trace-landing-icons.py](../../../../scripts/assets/trace-landing-icons.py)
normalizes generated PNGs to category ink/white, repairs enclosed alpha dropouts,
and traces genuine SVG paths. Raw generator files remain unmodified inputs;
normalized PNGs document the cleanup, including removal of the clipboard hole.
Every vector passes rendered silhouette/ink/white IoU above 99% against its
normalized input. Maximum compressed SVG is 75,286 bytes. The build verifies
hashes/budgets and refuses embedded raster/script/external SVG content.
Exact hashes and metrics: [icon provenance](icons/provenance.json),
[shipping manifest](../../../../landing/static/assets/art/provenance.json).

## Validation

- `npm --prefix landing run typecheck`: PASS.
- `npm --prefix landing run build`, `run check`, `test`: PASS; 42/42 tests.
  Build initially hit sandbox EPERM spawning tsc; ordinary escalated retry passed.
- Theme tests execute emitted JS and cover automatic/explicit/invalid preference,
  denied storage, cross-tab reset, single-icon visibility and grayscale AA tokens.
- `python scripts/assets/trace-landing-icons.py`: all six vectors pass >99% metrics.
- Earlier scoped `bash scripts/tests/test-gate-selection.sh`: 18/18 PASS.
- Browser LH02: 320/390/768/1024/1440, zero horizontal overflow, phone contained,
  badges never overlap screen; copy/stage heights equal at desktop widths.
- Final LH03 browser: 320/390/768/1024/1440 widths have no positive horizontal
  overflow, exactly one theme icon, complete phone containment and no badge
  overlap. At 1024 and 1440 the art/copy heights match exactly (386.0625 and
  323.34375px). Sun-only dark and moon-only light inspected; console warnings/
  errors empty. [Measured proof](responsive-proof.json).
- [Desktop dark](desktop-dark.jpg), [desktop light](desktop-light.jpg),
  [six dark cards](cards-dark.jpg),
  [mobile dark](mobile-dark.jpg). These are browser previews, not app masters.
- `git diff --check` and `bash scripts/scan-secrets.sh`: PASS.
- Browser capture first hit a transient automatic-review 429; later ordinary
  retry succeeded. Full-page sticky-header capture had a renderer artifact;
  regular viewport capture verified the actual desktop layout.

## Delivery

This is a local checkpoint in the isolated worktree. Integrate the atomic commit
into maintained dev after its active security work is saved, then use protected
dev → main delivery. No push, PR, merge, hosting, DNS, device install or release
certification is claimed.

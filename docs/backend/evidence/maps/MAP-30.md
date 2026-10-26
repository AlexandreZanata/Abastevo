# MAP-30 — Optional in-car integration disposition

Status: EXTERNAL_PENDING (local disposition PASS; no car adapter delivered)
Validation: PASS for the disposition only
Integration: N/A (optional host feature)
Gate: not applicable to the phone; no Android Auto/CarPlay certification claim

Dependency: MAP-29 local proxy checkpoint. The phone map/navigation remains usable without
any car host. iOS/CarPlay stays archived.

## License and dependency assessment

- The only candidate considered is the AndroidX Car App Library (`androidx.car.app`), whose
  published AndroidX artifacts are Apache-2.0. No exact revision is admitted or pinned in this
  repository, and no dependency/SBOM/notice entry was added. Adding one would require a
  recorded exact version, transitive license/dependency review, ProGuard/R8/host manifest
  review and an SBOM/notice update in the same change.
- The host/runtime (Android Auto, DHU and head-unit distribution/certification) is a separate
  proprietary/external surface. Its source license is not the AndroidX client license. Strict
  FOSS host requirements cannot be established from the available workstation.
- No closed map/navigation SDK, Google location dependency, external intent navigation or
  WebView substitute is added. The existing pinned CoMaps core and `MapEngine` remain the only
  phone navigation engine.

## Host availability and decision

No authorized DHU, head unit, emulator car host or distribution channel was available in this
batch. Therefore the optional adapter is **not implemented**: there is no host lifecycle,
navigation-intent, start/end metadata, stop/reconnect or driving-time text-entry regression to
claim. This is an explicit external disposition, not a hidden pass and not a blocker for the
phone.

Retake conditions (all required): an authorized host/DHU or head unit, an exact admitted
AndroidX Car App Library revision with license/dependency/SBOM review, owner authorization to
touch that host scope, and a host-safe UI/lifecycle design that cannot start phone GPS/TTS or
record a private trace merely by connecting the car. If any condition is absent, the feature
stays EXTERNAL_PENDING and the phone remains the delivered product.

No files, dependencies or permissions were changed by this phase. No iOS/CarPlay work was
started.

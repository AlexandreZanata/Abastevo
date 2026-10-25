# Resume the Abastevo map after native engine acceptance

Checkpoint review: 2026-10-25. **ENGINE_GATE PASS / LOCAL_DONE / INTEGRATION_PENDING** for
[the accepted extraction matrix](../backend/evidence/maps/MAP-14_ENGINE_GATE.md).
The [receipt](../backend/evidence/maps/map14-final/acceptance.json) pins exact
artifacts, source files, tests and runtime qualifications. Earlier PARTIAL/owner
requests are preserved in [history](history/MAP_EV_RESUME_BEFORE_ENGINE_ACCEPTANCE_20261024.md).

Reuse `/data/dev/worktrees/abastevo-map-ev`, branch
`codex/map-ev-implementation`, from the checkpoint commit containing this file.
Inspect actual HEAD/status first. Preserve other checkouts and all existing
MAP-00–16 work. No source/phone/VPS activation or remote delivery is implied.

## Already resolved — do not ask the owner again

- The accepted ceiling is 80 MiB. The universal release is 74.19 MiB with both
  ARM ABIs. Do not raise the ceiling to pass a future failure.
- No manual gold/lld workaround or hybrid AAR remains. The source patch retains
  ThinLTO, uses NDK LLD and strips unshipped metadata before the build ID.
- Both ARM ABIs have byte-identical independent-checkout AAR/classes/core
  comparisons. The exact resources, full notices/SBOM and expanded code audit
  pass. Code2000 and KhmerOS are excluded; no arbitrary human legal-owner stamp
  remains a local development prerequisite. Public source-offer publication is
  still mandatory before public distribution.
- Actual ARM64/16 KB/OpenGLES3 and ARM32/API30/Vulkan render/search/route,
  native lifecycle and canonical overlay selection pass. Failed ARM32 OpenGL
  emulator configurations are recorded and remain unaccepted; they are not
  the recipe for repeating the accepted ARM32 test. No physical all-device or
  production certification was inferred.
- Static builder reports deliberately say `NOT_EVALUATED_BY_STATIC_BUILDER`
  for runtime/gate fields. Use the versioned receipt for the phase result.

## Use the existing admitted artifact

Persistent candidate (outside Git and `/tmp`):
`/data/dev/artifacts/abastevo-map-engine/d2ec37881ede4e22`.
AAR SHA-256:
`d2ec37881ede4e2288e1a929f01a1eabab193e7a6d518c76ff4342f085f2b10c`.
Pass its absolute path as `-PcomapsSdkDir=...`. The SDK BuildConfig stays intact;
never substitute debug classes, remove it, copy raw unadmitted SDK assets or
edit generated Ninja files. Both advertised APK ABIs must contain the core.

Rebuild only when needed using [the builder recipe](../../maps/engine/upstream/README.md).
Pinned source is `/data/dev/src/comaps-4587b8ad`; a second independent source
checkout and prepared-input hashes are documented in the evidence. Changes to
SDK pin/patch/native ABI/resources invalidate affected artifact acceptance;
ordinary Compose work does not require repeating a clean C++ build.

## Next bounded work — real integration, not more owner decisions

1. Reconcile retained MAP-15 integration items before claiming real downloads:
   API-26 Ed25519 verification, WorkManager retry/cancel, schema-25 migration
   and active-package-to-native storage layout. Reuse existing patterns and
   allocate an append-only schema change only if the actual schema needs one.
   Java provider absence is an engineering failure state, not an owner decision.
   The pinned source already has Monocypher/BSD-2-Clause Ed25519; expose it
   behind the existing verification port (or a small owned JNI bridge), retain
   source/license hashes and prove exact Go bytes/RFC vectors, wrong lengths,
   key/message/signature mutation, trust/rotation and fail-closed behavior on
   API 26. No new provider or handwritten cryptographic algorithm is needed.
2. Connect the actual activated package generation to the SDK's readable
   internal versioned map directory. Preserve the prior active generation on
   verification/native activation failure. Do not use an adb-seeded directory
   or the probe fixture as the product downloader.
3. Continue MAP-17: a process-scoped singleton engine, the actual native
   MapHost and four tabs in order Explorar | Comunidade | Mapa | Perfil.
   Complete error/loading/no-package/first-fix-recenter states with real adapter
   behavior. The source host now waits for readiness, releases retired owners
   and forwards the full SDK surface lifecycle; a bare Activity without an
   AndroidX lifecycle owner is rejected. Test the real Compose/native host,
   repeat tab/back/deep links/recreation and the minified product path.
4. MAP-18/19/20 own viewport policy, dense OSM/bookmark disambiguation, final
   native artwork/targets, localized accessible list, genuine API stations,
   exact source-specific prices and minimum-map acceptance. Never pin unknown
   or city-centroid locations as exact stations, expose debug KML identity
   labels, or substitute a fixture/fake map/button for the real screen.

These later UI duties cannot circularly require completed MAP-17/18 UI before
those phases may start. The native extraction proof is ready; the production
map tab is not yet delivered. Full search UX, guidance/trips/EV and broader
physical/driver/performance QA retain their original owning phases. Read only
current PROGRESS, selected task and relevant code/tests; archived chronology is
not a new blocker. Run immediate scoped checks and truthful atomic local
checkpoints; no PR/CI/merge waiting during construction. No push, PR, merge,
phone installation, public distribution, deployment or certification here.

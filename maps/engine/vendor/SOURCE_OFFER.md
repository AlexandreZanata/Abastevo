# Source access for the pinned native build input

This directory distributes the MAP-22 privacy revision of the MAP-14 development dependency,
not an Abastevo application release or a map-data package.

- AAR: `sdk-release.aar`, 40,063,602 bytes.
- SHA-256: `9cdd3cda00e1025a4ba986bd385f07cf013b0584b41c9c14091ba2b4277c352c`.
- Corresponding upstream source: [CoMaps commit 4587b8ad4d12c9f59982709fdaf03cc925948827](https://github.com/comaps/comaps/tree/4587b8ad4d12c9f59982709fdaf03cc925948827).
- All recursively pinned submodule commits are preserved in `submodules.txt`;
  their repository URLs come from the upstream `.gitmodules` at those commits.
  `sbom.cdx.json` identifies the shipped components and selected notices.
- Abastevo's source modifications are supplied in
  [ndk-r28-lld.patch](../upstream/ndk-r28-lld.patch) and
  [private-diagnostics.patch](../upstream/private-diagnostics.patch). The owned host/adapter
  source is in this repository under `maps/engine/src/`.
- The full license texts and component notices are retained in the AAR under
  `assets/abastevo-map-licenses/`, alongside its Android resources. The core
  LICENSE and NOTICE are also adjacent to this document; the copyright
  inventory is retained inside the AAR.
  Project MIT does not replace these component/data terms.

Retrieve the corresponding source without an account or payment:

```sh
git clone https://github.com/comaps/comaps.git comaps-source
git -C comaps-source checkout 4587b8ad4d12c9f59982709fdaf03cc925948827
git -C comaps-source submodule update --init --recursive
# Apply both patches in maps/engine/upstream/ (ndk-r28-lld and private-diagnostics).
```

The build/preparation commands, toolchain versions, admitted resource hashes,
excluded fonts and independent-checkout comparison are retained in
[the build recipe](../upstream/README.md),
[resource admission](../upstream/resource-admission.json) and
[MAP-14 acceptance](../../../docs/backend/evidence/maps/map14-final/acceptance.json).
Keep the precise upstream pin, submodule commits and patch when redistributing
this input. No Code2000/KhmerOS font, CoMaps branding, credential, MWM map region
or private community data is admitted by adding this build dependency.

Gradle extracts the vendored archive into its ignored build directory through
`scripts/maps/provision-comaps-aar.py`; it verifies the whole archive and cached
files. Explicit `comapsSdkDir` overrides still undergo the existing per-core
verification. No developer-specific filesystem path is needed by CI.

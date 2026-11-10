# Third-party licenses and notices

The [Abastevo project license](LICENSE) applies to material the project owner
can license. It does not replace the following notices or the terms of data
sources, dependencies, fonts and other third-party material.

- Original project/origin MIT notice: [LICENSES/MIT-original.txt](LICENSES/MIT-original.txt).
  The notice is retained exactly as shipped before this license transition;
  historical revisions keep their accompanying license.
- CoMaps native SDK core: [Apache-2.0 license](maps/engine/vendor/LICENSE) and
  [upstream copyright/notice](maps/engine/vendor/NOTICE). The exact admitted
  third-party inventory and full retained terms are indexed in
  [the native engine notices](docs/backend/evidence/maps/map14-t07/THIRD_PARTY_NOTICES.md).
  Bundled MIT, BSD, Artistic, Zlib, Boost and other notices remain unchanged.
- Fuel icons, legacy Material Design Icons and source data:
  [third-party attribution](docs/attribution.md).
- OpenStreetMap map data and other geocoding/map providers retain their required
  on-screen attribution and source terms; see [data sources](docs/data-sources.md).
- OpenCNPJ is a separate offline backend preparation tool, not part of the APK.
  Its selected MIT notice and pinned binary/source provenance are preserved by
  [the registry installation](infra/deploy/registry/README.md).
- CameraX (`androidx.camera` 1.4.2) is Apache-2.0; its `camera-core` native
  image library bundles libyuv under BSD-3-Clause. Review:
  [live board framing](docs/mobile/p37-ocr-live-framing.md).
- Kotlin, Android Jetpack, Room, OkHttp and other dependencies retain their own
  license files and notices. The Abastevo visible credit does not replace them.

ANP, IBGE, RFB and community data are not relicensed as Abastevo source code.
Their source, privacy, attribution and redistribution rules remain separate.

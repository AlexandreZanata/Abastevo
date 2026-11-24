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
  [the native engine notices](release/notices/comaps/THIRD_PARTY_NOTICES.md).
  Bundled MIT, BSD, Artistic, Zlib, Boost and other notices remain unchanged.
- Fuel icons, legacy Material Design Icons and source data:
  [third-party attribution](docs/assets/attribution.md).
- OpenStreetMap map data and other geocoding/map providers retain their required
  on-screen attribution and source terms; see [data sources](docs/architecture/data-sources.md).
- OpenCNPJ is a separate offline backend preparation tool, not part of the APK.
  Its selected MIT notice and pinned binary/source provenance are preserved by
  [the registry installation](infra/deploy/registry/README.md).
- CameraX (`androidx.camera` 1.4.2) is Apache-2.0; its `camera-core` native
  image library bundles libyuv under BSD-3-Clause. Review:
  [live board framing](docs/android/ocr-live-framing.md).
- Google Play Billing Library (`com.android.billingclient:billing` 9.1.0) is
  governed by the [Android Software Development Kit License](https://developer.android.com/studio/terms.html);
  its bundled `third_party_licenses.txt` covers its own dependencies. Review:
  [Premium area](docs/android/premium-area.md#dependency-review-google-play-billing-library).
- AndroidX Credential Manager and its Play services bridge use Apache 2.0.
  The Google ID SDK uses the Android Software Development Kit License.
  Review: [Android sign-in](docs/android/sign-in.md#dependency-review).
- Kotlin, Android Jetpack, Room, OkHttp and other dependencies retain their own
  license files and notices. The Abastevo visible credit does not replace them.

ANP, IBGE, RFB and community data are not relicensed as Abastevo source code.
Their source, privacy, attribution and redistribution rules remain separate.

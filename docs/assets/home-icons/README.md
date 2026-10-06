# Home pictograms

Original native SVGs created for the 2026-10-06 reference-inspired Home.
Style follows the existing fuel pictograms: filled silhouettes, transparent
backgrounds, a green/blue/orange palette and small colored highlights.
SVG sources contain only paths, no scripts, fonts, embedded images or external
references. Android VectorDrawables share the exact 64 × 64 geometry and fills.
The app uses `Color.Unspecified` so theme tinting never removes their colors.
Existing fuel SVGs/bitmaps and logo masters are unchanged.

- [Savings / piggy bank](ic_home_savings.svg)
- [Community](ic_home_community.svg)
- [Trust / shield](ic_home_trust.svg)
- [Stations / map](ic_home_stations.svg)
- [History / chart](ic_home_history.svg)
- [Location](ic_home_location.svg)

Android exports: `app/src/main/res/drawable/ic_home_*.xml`.
Icons are decorative beside native accessible text; the shield represents
source context and does not assert certified/verified price quality.
These project-native additions follow the repository MIT license; no new
third-party icon/font/library dependency was introduced.

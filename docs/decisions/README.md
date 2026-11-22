# Architecture decisions

Decisions that still constrain the code. Numbers are kept from the original
records; gaps are process decisions that were retired with the old phase
workflow (git history has them). A decision is changed by a new record that
supersedes it, never by editing history.

| # | Decision |
|---|----------|
| [001](001-kotlin-compose-stack.md) | Kotlin + Jetpack Compose for the Android app |
| [002](002-price-import-immutability.md) | Imported official prices are immutable |
| [003](003-nominatim-reverse-geocode.md) | Nominatim for optional reverse geocoding |
| [005](005-modular-monolith-and-postgresql.md) | One Go modular monolith and one PostgreSQL/PostGIS database |
| [006](006-official-catalog-and-exact-contracts.md) | Official revisions, canonical stations and exact shared contracts |
| [007](007-anonymous-proof-of-possession.md) | Anonymous proof of possession for signed writes |
| [008](008-privacy-and-retention.md) | Data minimization and privacy-aware immutable history |
| [009](009-community-consensus-and-projections.md) | Immutable observations and conservative current-price projection |
| [010](010-private-object-evidence.md) | Private object storage for photo evidence |
| [012](012-licensing-and-hosted-business.md) | Licensing and hosted convenience kept apart from community trust |
| [015](015-kotlin-multiplatform-shared-domain.md) | Shared Kotlin logic with native device adapters |
| [016](016-android-commercial-community-ios-deferred.md) | Android first; iOS deferred |
| [017](017-station-profile-and-verified-representation.md) | Station profiles and scoped verified representation |
| [021](021-community-first-contribution.md) | Address-first community contribution without mandatory proximity |
| [022](022-comaps-derived-native-map-engine.md) | CoMaps-derived native map engine behind a narrow Android port |
| [023](023-admin-suite-granted-tiers-contributors.md) | Administrative suite, granted tiers and project contributors |
| [025](025-private-abastevo-empresas-organization-boundary.md) | Organization boundary for private Abastevo Empresas data |
| [026](026-multilingual-public-site.md) | Multilingual public site, pt-BR first, reviewed `en`/`es` translations |
| [027](027-ai-reviewed-landing-translations.md) | AI-reviewed translations for the `en` and `es` landing, disclosed to readers |
| [028](028-ai-reviewed-portal-translations.md) | AI-reviewed English and Spanish portal catalogs and localized city eligibility |

Enterprise-suite decisions are recorded in [enterprise/decisions.md](../enterprise/decisions.md).

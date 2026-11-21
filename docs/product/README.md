# Product rules

Behavior contracts the code implements. Rules use stable identifiers
(`B-BR-*` business rules, `BUC-*` use cases) that tests and commits reference.
When code and a contract disagree, fix one of them in the same change; the
code and its tests are the final word on what ships today.

## Core

- [Product overview](overview.md): what the product is and its invariants.
- [Open source and business boundaries](open-source-business.md).
- [Admin suite, tiers, contributors and staff roles](admin-tiers.md).
- [Notification content](notification-content.md).

## Stations and official data

- [National station catalog](stations/catalog.md), [registry sources](stations/registry-sources.md).
- [Station and fuel feedback](stations/fuel-feedback.md), [fuel quality and reliability](stations/fuel-quality.md).
- [Station profiles](stations/profile-representation.md) and the [frozen profile contract](stations/profile-contract.md).

## Community and forum

- [Community experience](community/experience.md), [community-first validation](community/community-first.md), [metrics](community/metrics.md).
- [Moderation](community/moderation.md), [safety: report and block](community/safety.md).
- [Terms text shown to users (pt-BR)](community/terms-pt-br.md): user-facing copy, intentionally in Portuguese.
- [Forum contract](forum/contract.md) and [forum policies](forum/policies.md).

## Map, vehicles and EV

- [Map discovery](map/discovery.md), [reports, routing and guidance](map/reports-routing-guidance.md), [favorites and trips](map/favorites-and-trips.md).
- [Vehicles and official occurrences](vehicles-and-occurrences.md), [vehicle registration](vehicle-registration.md), [exploration journeys](exploration.md).
- EV: [support contract](ev/README.md), [typed data](ev/typed-data.md), [preparation](ev/preparation.md), [energy model](ev/energy-model.md), [charging-stop planner](ev/charging-stop-planner.md), [journey UI](ev/journey-ui.md), [cost method](ev/financial-method.md).

## Driver suite, Premium and payments

- [Individual driver suite](driver-suite/README.md) and its slices in [driver-suite/](driver-suite/).
- [Premium](premium/README.md), [financial method](premium/financial-method.md), [source policy](premium/source-policy.md).
- [Payments (Stripe/Asaas)](payments/README.md) and [offer policy](payments/offer-policy.md). Sales are disabled.

## Website

- [Landing content](landing/content.md), [educational guides](landing/guides.md) and the [organic growth and multilingual contract](landing/organic-growth-i18n-plan.md).

Enterprise (Abastevo Empresas) rules live in [docs/enterprise/](../enterprise/README.md).

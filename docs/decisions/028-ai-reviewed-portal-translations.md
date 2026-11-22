# ADR-028: AI-reviewed portal translations for English and Spanish

The owner extends [ADR-027](027-ai-reviewed-landing-translations.md) to the
English and Spanish portal message catalogs. The first AI pass drafts from
the Portuguese source. A separate pass checks terminology, data and unit
fidelity, source and internal links, and placeholders. Catalog metadata names
the actual AI reviewer. Each localized page discloses the AI review and states
that Portuguese prevails. Other locales still follow ADR-026.

National prices, fuel hubs, states, fuel by state, methodology, downloads and
insights have localized routes. City variants use a closed IBGE-code allowlist
of the 27 capitals and border cities already covered by the Portuguese portal.
The same snapshot must supply the Portuguese page before it can have a variant.
Station, fuel by city, enforcement, forum and search pages remain Portuguese.

B-BR-SEO-01 applies the locale and catalog gates together with the existing
page facts. A disabled locale or unreviewed catalog emits no pages. Emitted
variants share the indexability of their Portuguese source. Reciprocal
hreflang carries only indexable routes from the same group, including the
Portuguese default. Price data stays in BRL and uses the survey unit, normally
per litre. The portal keeps the canonical survey units for CNG and LPG. Currency
and gallon conversions are outside this delivery.

# ADR-028: AI-reviewed portal translations for English and Spanish

The owner extends [ADR-027](027-ai-reviewed-landing-translations.md) to the
English and Spanish portal message catalogs. The first AI pass drafts from
the Portuguese source. A separate pass checks terminology, data and unit
fidelity, source and internal links, and placeholders. Catalog metadata names
the actual AI reviewer. Each localized page discloses the AI review and states
that Portuguese prevails. Other locales still follow ADR-026.

National prices, fuel hubs, states, fuel by state, methodology, downloads and
insights have localized routes. English covers all municipality summaries
published in the same snapshot. Spanish municipality summaries use the
closed IBGE-code list of capitals and border cities. Both depend on the
Portuguese source route. Station details, enforcement, the editorial forum
and search also have localized routes. Individual fuel-by-city pages
keep their Portuguese routes.

B-BR-SEO-01 applies the locale and catalog gates together with the existing
page facts. A disabled locale or unreviewed catalog emits no pages. Emitted
variants share the indexability of their Portuguese source. Reciprocal
hreflang carries only indexable routes from the same group, including the
Portuguese default. Price data stays in BRL and uses the survey unit, normally
per litre. The portal keeps the canonical survey units for CNG and LPG. Currency
and gallon conversions are outside this delivery.

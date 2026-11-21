# ADR-027: AI-reviewed translations for the `en` and `es` landing

Accepted by the owner on 2026-11-21. This decision amends rule 5 of
[ADR-026](026-multilingual-public-site.md) ("reviewed translations only", by a
fluent human) for the landing pages of the `en` and `es` locales. It answers
the open inputs I18N-D05 of the
[multilingual contract](../product/landing/organic-growth-i18n-plan.md) and
G-D01 of the
[expansion plan](../product/landing/organic-traffic-expansion-plan.md).

## Context

The multilingual landing build was finished and switched off. ADR-026 required
a named, fluent human reviewer for each locale, and none was named, so
`contracts/i18n/locales.json` kept `en` and `es` disabled and `/en/` and `/es/`
answered 404.

The owner wants both locales published and has no human reviewer for them.

## Decision

1. **The review is made by AI and recorded as such.** The `en` and `es`
   dictionaries and landing translations ship after an AI review. The locale
   registry, the dictionaries and the front matter record the reviewer as
   `AI review (Claude)`. No person is named for a review that no person made.
2. **Two passes.** One AI pass drafts the translation. A separate pass reviews
   it against the pt-BR source and checks that:
   - every number, formula, date, table and source link equals the source;
   - the glossary terms are used;
   - no fact, number or claim was added;
   - internal links point to the same locale where a translation exists.
3. **Disclosure.**
   - Every translated guide says, in its editorial line, that the version was
     translated and reviewed with AI and that the Portuguese original prevails.
   - The editorial policy states the rule in the three languages.
   - The statement "no text is published without human review" covers original
     Portuguese texts only.
4. **The build gates do not change.**
   - A translation needs `translationStatus: reviewed`, `reviewedBy` and
     `reviewedAt`.
   - A locale needs a reviewed dictionary and a reviewer in the registry.
   - A newer pt-BR source marks the translation stale: it shows a note, turns
     `noindex` and leaves hreflang and the sitemap until it is reviewed again.
   - The hreflang, canonical, switcher and no-redirect rules of ADR-026 hold.
5. **Scope.** The landing only: the home, the about page, the editorial
   policy, the calculators and the localized guides, in `en` and `es`. The
   portal, legal pages and any other locale need their own decision before
   they ship under this rule.
6. **A human review replaces the AI review at any time.** The page then names
   that person in `reviewedBy`.

## Alternatives

- **Wait for a human reviewer, as ADR-026 said.** This keeps the stronger
  guarantee and keeps both locales off for an unknown time.
- **Record the owner as the reviewer.** The owner did not review the texts, so
  the record would be false.
- **Publish the drafts without a review pass.** This is the raw machine output
  that the contract excludes.

## Consequences

- A mistranslated fact can reach a reader before any person has read it. The
  number check, prices rendered from data, the Portuguese-prevails rule and
  the disclosure limit the damage; they do not remove the risk.
- A correction to a translation follows the editorial policy: an issue in the
  repository, then a fix with a new `reviewedAt`.
- The pt-BR editorial policy changed with this decision, so its translations
  must follow every later change to it.

## Verification

Landing tests prove that every production translation is reviewed and not
stale, that the build emits the locale pages with reciprocal hreflang, that
the editorial policy states the translation rule, and that the emitted site
passes the dist check.

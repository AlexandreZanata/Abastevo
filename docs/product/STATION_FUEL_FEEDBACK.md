# Station and fuel feedback target contract

Status: ADOPTED target, NOT IMPLEMENTED. Owner phases P13/P14/P17. User confirmed 280 characters and email-code/Google/Apple registration on 2026-09-30.

## B-BR-F01…F08

- **F01:** Feedback targets an existing station plus a catalog fuel product. Ratings describe personal experience, not a laboratory quality measurement or a pump-price fact. Public reads may be anonymous; every rating, comment, reply, validity vote and report requires an active authenticated app account. Payment never affects permission to participate, vote weight or trust.
- **F02:** Rating is an integer 1–5, one current rating per account/target. Edits are versioned and idempotent; aggregate count and mean are rebuildable. No fractional/out-of-range stars. Account deletion/suspension has an explicit contribution/projection policy before schema implementation.
- **F03:** Comments and replies are plain text, nonempty and at most **280 Unicode scalar values**, counted after trimming and CRLF normalization. Same vectors on Go/Kotlin/Swift cover 280/281, accents, emoji, combining sequences and invalid UTF-8/surrogates. Never count UTF-16 units on one platform and code points on another. HTML is not rendered; no silent truncation. Only the author edits their text; revision audit excludes unnecessary PII.
- **F04:** Replies attach to a comment at the same station/fuel. Initial bounded depth is one level; this is a proposed MVP restriction, frozen by P14-T01. Replies share text, ownership, moderation and voting rules. Pagination and write quotas are bounded.
- **F05:** A validity vote is VALID or INVALID, at most one active vote per account/comment revision, excluding the author. Change/removal is idempotent. Edits create a new revision with a new vote denominator; old votes remain restricted audit history. Unique constraints and concurrent transaction tests enforce this, not client UI flags.
- **F06:** Community agreement = 100 × valid / (valid + invalid). Use integer basis points (floor(10000 × V / total)); UI displays one decimal by truncation. Example: 2 valid/1 invalid → 66.6%, 0/3 → 0%, 3/0 → 100%. Zero votes → null/“No votes”, never 0% certainty. Show valid, invalid and total counts alongside the percentage. This is community agreement, not factual probability; small samples and coordinated accounts can mislead. No payment weighting or mixing with price-consensus confidence.
- **F07:** Moderation states: visible, flagged, hidden, removed. Reports do not automatically grant moderation power or let one user delete another's comment. Server roles authorize decisions with bounded reasons and actor/action audit. Public reads exclude hidden/removed text and private identifiers; author/account controls include deletion, report and block. Abuse limits, appeal route, safe display and retention policy are specified before launch.
- **F08:** Account/session revocation blocks new social writes. Display opaque public aliases, never email/provider subject, precise GPS or signed media URLs. Feedback cannot overwrite official ANP records or manipulate price facts.

## BUC-F01…F05 and tests first

F01 rate/edit/remove; F02 comment/reply/edit/delete; F03 vote/change/remove; F04 report/block/moderate/appeal; F05 paginate source-separated feedback and rebuild projections. Each gets contract vectors before implementation, positive/negative ownership/session cases, duplicate/replay/parallel writes, stale revisions, suspension/erasure and projection reconciliation on real PostGIS.

P14 publishes additive versioned OpenAPI schemas only after freezing these decisions; the implemented v1 is not silently changed by this document. P17 consumes the same fixtures on Android and iOS. Anonymous contributor proof alone cannot authorize social writes. No new account or feedback endpoint is claimed to exist today.

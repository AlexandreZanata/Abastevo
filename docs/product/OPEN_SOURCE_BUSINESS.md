# Open source and business boundaries

Status: planning. Preserve the existing MIT license and attribution. Product/brand “Postô” is provisional; do not rename packages, publish a new app identity or register marks/domains in this task.

## Free product and hosted value

Core official/community lookup, anonymous contribution, essential offline behavior, current local vehicles/navigation and basic confirmation/dispute stay free. Paid plans may sell optional hosted backup, recovery/migration, cross-device sync, advanced statistics/alerts and convenience. Payment must never buy trust, moderation immunity, a better confidence tier or misleading placement in cheapest-price results.

A future API/fleet/merchant offering needs validation of demand, data licensing, privacy, cost and abuse constraints. Design provider-neutral boundaries now; do not build multi-tenancy, fleet services or merchant dashboards before those use cases exist.

## License and contributions

The actual [MIT license](../../LICENSE) is authoritative; preserve its copyright and permission notices. A repository hyperlink alone does not replace those notices. The [OSI MIT text](https://opensource.org/license/mit) supports commercial reuse under its terms. Past MIT distributions retain those granted rights; a new licensing decision cannot erase them.

Current recommendation: keep MIT during backend MVP development to avoid an accidental mixed-license boundary. Decision pending before accepting contributions under any changed terms: MIT throughout versus Apache-2.0 client/new code versus AGPL-3.0 backend. Evaluate network-copyleft obligations, dependency compatibility, copyright ownership and contributor consent with qualified review before any change. No relicensing is performed here.

Use a simple contribution guide and consider DCO sign-off for provenance (recommended before external backend contributions). CLA only if a concrete future licensing/business need justifies it. Do not claim existing outside contributions can be relicensed solely because the repository owner wants to. Inventory contributions and preserve upstream history/provenance.

## Build in public

Publish architecture, source, tests, SQL migrations, consensus algorithm, OpenAPI, release notes and reproducible synthetic benchmarks. Publish progress with actual completion evidence, not aspirational feature claims. README points to roadmap, local setup and known limitations. Add screenshots only when the UI exists and reflects the current release; no invented communities, support links or sponsors.

Keep private: production database, personal data, evidence photos, signing keys, provider tokens, private operational access, and sensitive deployment/anti-abuse runtime settings. Public algorithm changes should remain explainable and versioned even when specific production abuse thresholds are private. Aggregated statistics need privacy review and sufficiently large groups.

## Commercial extension guardrails

Play Billing integration is LATER; backend verifies purchase state against the provider and translates it to Entitlement. Never accept a client `isPremium` flag as authorization. Handle renewals, revocations, replay and reconciliation before selling hosted access. Account linking preserves contributor identity only after proofs; a paid account does not inherit unrelated reputation.

Brand/trademark is separate from software license. [TRADEMARKS.md](../../TRADEMARKS.md) records provisional status without claiming an unverified registration. Revenue experiments must cover measured hosting/operation costs and maintain a credible free product. No prices or revenue forecasts are invented in this plan.

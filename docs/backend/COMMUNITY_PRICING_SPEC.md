# Community pricing specification

Status: deterministic v1 proposal. Numerical defaults are conservative hypotheses to test on synthetic and labelled pilot data, not claims of fraud-proof accuracy. Changes require fixtures and a new policy version. Rules: [B-BR-001…016](DOMAIN_MODEL.md).

## Observation input and admissibility

Required: client_submission_id, station_id, fuel_product, price {amount_milli_brl, currency, unit}, condition {kind, qualifier_id when applicable}. Optional: evidence_id, capture timestamp, location with accuracy/fix age, OCR candidates and user-confirmed selection, supersedes_observation_id. No contributor_id, confidence, reputation or server timestamps from the client.

Server records received_at, resolved contributor/key, policy version and server-computed signals. Missing photo/GPS is allowed for accessibility/offline contribution; such observations alone cannot produce HIGH confidence. No location or integrity signal proves truth. A capture claimed more than 24 h old or in the future by over 5 min is historical-only pending review; receipt time is not advertised as capture time. Missing/untrusted capture time is labelled as received time.

Plausibility checks compare regional/product ranges and ANP only as signals. ANP is too delayed to veto every legitimate price change. Invalid amount/unit/station/ownership is a structural rejection. Unknown station location yields unknown proximity. Suspicious but structurally valid content enters review and is excluded from consensus until released; each reason is recorded.

## Conditions, product and evidence

Compute independent prices for `(station_id, fuel_product, unit, condition_kind, qualifier_id)`. STANDARD must be explicitly selected by the contributor. APP/LOYALTY require a recognized qualifier for aggregation; otherwise retain the fact pending classification. Never select the smallest OCR number automatically. OCR result and confidence are client claims, only useful with human confirmation and other signals.

Evidence is bound to one observation. Exact repeated bytes from other contributors do not count as independent photographic support; the same session retry is idempotent. Perceptual similarity raises review risk, not automatic guilt. Private media are never included in public prices.

## Confirmations and disputes

A confirmation references one VALIDATED, still-eligible observation and affirms its product, amount, unit and full condition. It cannot change the price. Contributor must differ from its author. Unique `(observation_id, contributor_id)` prevents retry amplification. The vote ledger allows at most one latest eligible support vote per contributor per price key across observations and confirmations. Ordering uses received_at then event ID, not client time. Linked keys resolve to the same contributor.

Dispute reasons: PRICE_CHANGED, WRONG_STATION, WRONG_PRODUCT, WRONG_CONDITION, EVIDENCE_MISMATCH, OTHER. PRICE_CHANGED should reference a replacement observation if possible. Disputes are reports, not negative votes that can erase a price on demand. Quotas and one open case per contributor/target/reason stop repeated flooding. A substantiated review can exclude facts; contradictory independent price support can set the projection to DISPUTED without operator intervention. Moderators' votes do not receive arbitrary numeric multipliers.

## Consensus policy `consensus-v1`

1. Evaluate at injected UTC time T. Fetch only the last 48 h of eligible validated observations and confirmations for one price key; remove revoked, quarantined, blocked-contributor votes and exact-media duplicates from photographic independence.
2. Reduce to one latest eligible vote per contributor. Confirmation weight is never added to that same contributor's observation weight. A confirmation cannot refresh the anchor observation's expiration.
3. Group by **exact milli-BRL amount**; no fuzzy clusters in v1. Weight for each vote = recency factor × trust factor. Recency: 4 if age ≤6 h; 2 if >6…24 h; 1 if >24…48 h. Trust: 1 for new contributors, 2 for established contributors. Blocked contributors are excluded. Evidence changes eligibility/quality gates, not an unbounded multiplier.
4. Sort groups by total weight, independent contributors, newest eligible anchor receipt, then amount ascending, then smallest observation UUID. Tie-breaking is deterministic; it does not bypass the conflict rule below.
5. A group must contain at least one eligible direct observation, not only confirmations. If none, return UNKNOWN. If two leading groups each have ≥2 independent contributors and runner-up weight is ≥60% of winner weight, return DISPUTED with no recommended current amount. An unresolved substantiated moderation case also yields DISPUTED.
6. Otherwise select the leading amount. LOW: any eligible anchor. MEDIUM: ≥2 independent supporters and ≥1 server-validated unique photo with acceptable proximity. HIGH: ≥3 independent supporters, ≥2 established contributors, ≥2 distinct validated photo anchors within 6 h, acceptable proximity on those anchors, and runner-up weight <25% of winner. Missing or questionable geolocation cannot satisfy the HIGH gate.
7. `expires_at` = most recent eligible direct anchor receipt +48 h. Freshness: FRESH up to 6 h; AGING >6…48 h; STALE thereafter. HIGH additionally decays to MEDIUM once its 6 h anchor requirement fails. Confirmations may improve support but never move this expiry by themselves.
8. Store algorithm_version, policy_config_version, computed_at, input cutoff/version, counts, reason codes, supporting event IDs in restricted audit data and next_recompute_at. Public output includes explanatory counts and freshness, not contributor IDs or sensitive risk thresholds.

These rules stop “last request wins” but do not solve Sybil attacks. A new-price minority remains LOW; organized multiple identities can still deceive. Rate limits, evidence independence, conservative trust maturation and review are part of the MVP gate. Do not market HIGH as a guaranteed live price.

## Trust policy `trust-v1`

New identity starts NEW. ESTABLISHED requires account-key age ≥14 days and ≥10 independently reviewed successful observations across ≥5 days with no upheld abuse decision. Ordinary consensus success alone does not promote trust: circular reinforcement would let colluding identities train their own reputation. Moderator review or independently validated pilot labels establish outcomes. Store contributing case IDs and policy version.

BLOCKED is an audited abuse decision, not a numeric punishment for one discrepancy. Rehabilitation/reversal creates a new decision and recomputes affected prices. Paid entitlement, account email, device cost and purchase status are absent from the algorithm. The initial system may remain mostly LOW/MEDIUM while reliable support accumulates; that is acceptable.

## Abuse signals and admission controls

Initial configurable test defaults: register ≤5/hour/IP; authenticated writes ≤10/min and 50/day/contributor; photos ≤10/day/contributor, 3 MiB each; one contributor vote per price key. Edge IP limits and DB-backed identity/operation quotas are complementary. NAT/shared connections require graceful 429s and review, not automatic mass banning.

Signals: suspicious velocity/impossible travel using short-lived derived distance/time, poor GPS accuracy (initial acceptable ≤100 m), station distance (initial acceptable ≤300 m plus accuracy), duplicate media, repeated identical values, implausible regional deviation, identity clusters and dispute history. These thresholds are tunable experiments. Large deviations alone do not reject real price changes. Play Integrity is optional future corroboration, never required proof or a substitute for anonymous access.

## Concurrency and recomputation

Persist observation and validation job atomically. One durable job per affected price-key/version is deduplicated. Acquire a per-key DB lock before loading votes and publishing the next projection. Recompute on validation, confirmation, moderation, trust change and timed weight/expiry boundaries. If a worker crashes, lease/fencing plus projection versions prevent old work overwriting a newer result. Replay produces the same price for the same input/clock/policy. API applies expiry independently of worker health.

## Required golden cases

No data→UNKNOWN; one new observation→LOW; two independent supporters with validated photo→MEDIUM; strict HIGH quorum; self-confirmation rejected; retries do not add votes; one contributor switching amount changes only its own vote; duplicate image does not establish independence; two supported conflicting prices→DISPUTED; missing coordinates blocks HIGH; APP qualifier separation; ANP remains unchanged; expiry despite worker outage; confirmation does not revive stale anchor; entitlement changes have zero effect; input permutation yields identical output; policy-version replay; moderation invalidation and contributor blocking trigger recomputation.

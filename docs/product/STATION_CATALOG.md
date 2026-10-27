# National station catalog target contract

Status: **PLANNED target**, 2026-10-02. This document does not describe new runtime behavior. Owner: [P25–P29 catalog plan](../planning/STATION_CATALOG_PLAN.md); task definitions are in [ROADMAP](../../ROADMAP.md#station-catalog-expansion). Existing Directory, free-account, evidence, location, moderation and price contracts remain authoritative until compatible extensions are implemented and tested.

## Purpose and vocabulary

Catalog fuel retailers independently of participation in ANP's price survey. A business registration, ANP authorization, observed physical opening and a mapped location are different facts with different dates. Authorization publication must never become an inferred inauguration date. Absence from a feed means verification is unavailable, not proof of unlawful activity.

The initial public catalog accepts records whose identity/address and authorization evidence satisfy the frozen P25 policy. Community-only suggestions remain private to their submitter and authorized operators until reviewed. P27 freezes a separate reviewed-unverified publication policy before any such visibility; no unverified suggestion is automatically published by this plan. A station without reviewed coordinates can be text-searchable; it cannot claim precise proximity or routing.

## B-BR-D01–D12

- **D01 — Source separation:** Directory owns stable station identity, source assertions and reviewed location. Official owns dated ANP price observations; Community owns submitted prices and consensus. Catalog insertion creates neither a price nor trust, and never fabricates an ANP price-survey observation.
- **D02 — Identity and succession:** Validate full branch CNPJ as text using existing numeric/alphanumeric rules. Use SIMP/authorization as additional source identifiers after their scopes are verified. Enforce unique active identifiers transactionally; retain validity history. Different identifiers at one address, or nearby points, are review candidates, never automatic merges. Any succession, duplicate consolidation or identifier reassignment needs an audited, reversible decision preserving references to prices/social facts.
- **D03 — Provenance and clocks:** Record source, source identifier/reference, content checksum, parser/policy version, published/effective dates when supplied, fetched time, validated time and catalog publication time. Unknown timestamps stay unknown. Replaying identical input/policy is idempotent; corrections retain supersession history. Restricted owner/contributor data never enters public provenance.
- **D04 — Independent states:** Model authorization verification, operating status, publication eligibility and location quality separately. P25 freezes wire values and allowed transitions before migrations/API. An active CNPJ, map point, vote or photograph alone proves none of the other states. Unknown/stale status is displayed honestly; paid status does not affect verification.
- **D05 — Publication and withdrawal:** Publish only validated records from complete reconciled input with explicit atomic boundaries. A broken, truncated or missing page cannot retire records. A single missing row does not mean closure. Closure/revocation requires sourced status evidence or an audited review. Late contradictory evidence cannot silently replace newer accepted facts.
- **D06 — Location:** Reuse append-only location revisions and the existing reviewed-point projection. Validate latitude/longitude, CRS/SRID, source accuracy and address match; missing/ambiguous/centroid positions stay unreviewed. A user pin or capture GPS is a private suggestion to review, not a canonical station point. Do not publish the contributor's precise GPS.
- **D07 — Account and ownership:** Suggestions, corrections and owner-status reads require an active free app account plus existing signed-request proof. Anonymous contributor proof alone is insufficient. All public catalog reads may remain anonymous. Knowing a public CNPJ or submitting a station grants no ownership, moderation privilege or exclusive editing rights.
- **D08 — Submission and abuse:** Freeze bounded structured inputs, account quotas, idempotency keys and safe error/status projections before coding. Concurrent submissions for one station converge without losing each author's private request status. Evidence of suspicious activity routes to review; community vote counts do not grant publication power. Reports/appeals reuse moderation authority, never auto-delete another user's station.
- **D09 — Media and privacy:** Reuse P15/P16 controls and local lightweight capture/encoding. A suggested photo is optional unless the frozen review policy explicitly requires it for a particular unresolved case. Every photo copy expires within the existing maximum 24 hours; no reset on rebinding/retry. Durable decisions retain only policy-approved minimal metadata. After expiry, insufficient cases require fresh evidence or remain unresolved. No raw photos, credentials, private documents, submitter GPS or contact/provider identity in Git, fixtures, logs or public APIs.
- **D10 — Source access:** Fetch only approved HTTPS origins with redirect/DNS/IP validation, deadlines, byte/row/page limits, bounded retries and global quotas. No caller-provided fetch URL or remote request in a public search handler. Disable XML external entities/DTD fetching and formula execution. Credentials stay in operator secrets, not task payloads or logs. A provider timeout leaves the last validated state intact and its freshness visible.
- **D11 — License and representations:** Record storage/redistribution/attribution rights per source before enabling it. OSM and commercial-provider assertions retain provenance and may require separate storage/views; physical separation alone does not settle license obligations. No copying of a restricted provider into an unrestricted permanent catalog. Partner messages require agreed authority and independent authorization verification.
- **D12 — Measured freshness:** Distinguish opening-to-source delay, source-publication-to-fetch delay, eligibility-to-catalog delay and human-review age. The proposed 24–48-hour objective begins when a record has sufficient validated evidence for publication; it is not a national inauguration SLA. Define eligibility/exclusions before measurement; report unresolved/quarantined records separately so the objective cannot hide backlog. A photo or membership fee never accelerates trust.

## BUC-D01–D08 and tests first

### BUC-D01 — Discover official station inventory

Actor: scheduled worker/operator. Fetch a complete versioned CSV or bounded API query; stage, validate identifiers/addresses/statuses, reconcile and publish through Directory. Missing page, layout change, oversize input or excessive unexplained delta quarantines the run and preserves the previous accepted catalog. Prove leading zeros/alphanumeric CNPJ, replay, duplicate rows, conflicting facts and concurrent first creation.

### BUC-D02 — Detect regulatory changes

Actor: DOU discovery worker. Read approved daily editions, retain edition/act reference and classify ANP grant, correction, revocation or unrelated act. Extract structured assertions, verify against the official act/API and reconcile. Unknown wording, amendments, conflicting chronology or ambiguous establishment needs review. No LLM-generated interpretation is sole publication authority. Test multiple establishments in one act, republication, operator succession and outage catch-up.

### BUC-D03 — Suggest or correct a station

Actor: active free account. Search for duplicates first; submit structured suggestion with optional evidence and location; receive a private request ID. Existing station detection may resolve to its canonical ID without publishing the submitter's identity. Failed signature, revoked session, replay, quota violation, another owner's evidence and conflicting idempotency payload are rejected. Submission never proves operation or grants editing ownership.

### BUC-D04 — Verify, decide and appeal

Actor: automated verifier for policy-approved exact official matches, otherwise an authorized reviewer. Review source/status/address/location conflicts, record reason and versioned decision, and publish eligible facts atomically. Expired evidence is inaccessible and cannot be relinked to extend retention. User cancellation, simultaneous reviewer actions, appeal and a new source correction have explicit tested transitions. Use existing moderation ports; no unaudited database editing.

### BUC-D05 — Read catalog and attach existing participation

Actor: anonymous reader or signed contributor. Read searchable stations even without official/community prices. Render no-price, authorization freshness and unknown coordinates explicitly. Resolve legacy full CNPJ to server UUID via a declared Directory read contract before contribution/rating/comment/report; never cast CNPJ to UUID or synthesize a price row. Account/social permissions and 280-scalar feedback rules remain unchanged. Test offline cache, pagination, unknown server state and denied/stale target.

### BUC-D06 — Reconcile lifecycle and recover

Actor: worker/operator. Apply authorized corrections or closure with a new revision, retaining referenced station IDs and source history. Lease fencing, concurrent importer/suggestion, duplicate job, database failure, restore replay and binary rollback cannot publish partial state or resurrect private evidence. User data erasure follows existing privacy workflows rather than an unlimited audit exception.

### BUC-D07 — Add a licensed complementary source

Actor: operator/verified partner adapter. Admit OSM or a contracted feed only after need, rights, schema, authentication, quota and freshness are documented. Supplement only the permitted attributes, flag conflicts and route candidates through the same verifier. Disable a provider without deleting unrelated facts. Owner representation, if introduced, needs independent proof and is separate from authorization and station editing privileges.

### BUC-D08 — Measure national throughput and timely publication

Actor: operator. Exercise a frozen synthetic workload, dependency outages, backlog/recovery and review queues; report valid denominators, hardware/configuration, exact revision and unmet budgets. Run anonymous read/contribution/social regressions while imports execute. A simulated 48-hour clock proves behavior, not 30 days of live service. Public freshness guarantees wait for measured source and staffing evidence.

## Contract implementation boundary

Verified representation is now explicitly planned in [STATION_PROFILE_REPRESENTATION](STATION_PROFILE_REPRESENTATION.md) / [P30–P33](../planning/STATION_PROFILE_PLAN.md), superseding P28-T04. Directory remains owner of Station/operator identities; a profile grant is scoped to an account/station/operator revision and never inferred from source/suggestion/partner access.

P25/P27 must update the owning versioned OpenAPI, fixtures, Data Model and privacy inventory before their new handlers/migrations. Names in this document are semantic targets, not endpoints or tables claimed to exist. Existing account writes, Directory UUIDs, anonymous reads, cache allowlists and media ownership remain in force. Unknown additive states have cautious client fallbacks. Native iOS work is deferred until an explicit request; Kotlin contracts preserve portable domain/application code.

## B-BR-COR-01..08 — station corrections (MC-B1)

Target: an active free account may propose a correction to an existing
directory station. Corrections are community assertions; they never overwrite
an official fact in place (D8). The fields available in this batch are
location, display name, brand, address and operating status. Fuel products,
services and EV connectors join the same contract after the dossier (A5) and
their own validators land.

- **COR-01 — Target and fields.** Every correction names one station UUID and
  exactly one field. Payloads are structured JSON validated by a per-field
  validator; unknown fields and unknown top-level members are rejected. A
  correction never creates a station, a price, a trust rating or ownership.
- **COR-02 — Official/community separation.** The official/ANP projection and
  the station's canonical identity remain unchanged. An accepted location
  becomes a new `reviewed` location revision through the existing ladder; an
  accepted name/brand/address/status becomes a community-layer assertion with
  its own label, source and observed date. Community consensus alone never
  changes an official field.
- **COR-03 — Proof, quota and idempotency.** Submission requires a live free
  account session and the existing signed-request proof. The account quota is
  20 submissions per rolling day. One open correction per account + station +
  field is allowed. `(account_id, client_submission_id)` is unique; the same key
  with the same body replays the record, the same key with another body is a
  conflict. Anonymous contributor proof alone is insufficient.
- **COR-04 — Field validation.** Location must be a finite WGS84 point inside
  Brazil and inside the station municipality polygon (500 m buffer), and may
  not exceed the allowed distance from the current point unless the payload
  carries accepted evidence. The request may carry only a coarse proximity
  bucket (for example `same_municipality`, `within_1km`); raw contributor GPS
  is never stored or published (P16). Name/brand/address are normalized,
  length-bounded, escaped on output and checked against the station context.
  An optional note is at most 280 scalar values and rejects URLs, e-mail
  addresses, phone numbers and CPF/CNPJ-like personal identifiers.
- **COR-05 — Consensus and review.** Consensus counts distinct accounts and
  distinct device keys; new accounts weigh less, and correlated submissions
  (same device, network or time window) count once. A correction matching
  official evidence (for example an ANP point within 50 m of the proposed pin,
  or an equal ANP name/brand) may become `auto_verified` with the evidence
  attached; everything else waits for an operator review. Review decisions are
  append-only and carry a mandatory reason. A community location accepted by
  review becomes `reviewed`; auto-verified official matches become the
  corresponding official tier, never `reviewed`.
- **COR-06 — Status lifecycle.** Frozen values are `pending`, `auto_verified`,
  `accepted`, `rejected`, `expired`, `cancelled`, `superseded` and `frozen`.
  Only `pending`/`auto_verified` are open; `cancelled` is owner-only;
  `expired` follows the evidence/policy window; a newer accepted correction
  supersedes an older open one for the same station+field; `frozen` is a review
  hold for flip-flopping. Every transition is audited. A decision after a
  cancellation, a duplicate reviewer decision or a foreign-owner action fails
  closed.
- **COR-07 — Privacy, retention and erasure.** Owner reads are private and
  never list who suggested what to the public. Evidence follows the existing
  24-hour private-media rules and EXIF stripping; expired evidence cannot be
  re-linked to extend retention. Account erasure deletes or anonymizes
  correction rows according to P13, while retaining the minimal audited
  decision metadata required by the moderation policy. No personal name, CPF,
  personal phone/e-mail or precise contributor GPS enters Git, logs or public
  APIs.
- **COR-08 — Abuse and audit.** Per-account/day and per-device/network controls
  bound flooding; a bounded per-station queue and a 30-day cooldown per
  station+field after a decision apply unless new evidence arrives.
  Closed/permanently-closed proposals never hide a station without ANP
  revocation evidence or staff review. A rejection ratio over a window lowers
  weight and may pause intake reversibly and with an audit record. Every staff
  decision records decider kind/id, reason and evidence; payment or subscription
  never affects review, intake or trust.

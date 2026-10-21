# Premium source policy v1

**2026-10-20 planned registration amendment:** [MEX contract](MOBILE_EXPANSION_CONTRACT.md)
selects one new free vehicle and verified Premium for additional registrations,
with preserved legacy data. MEX-01/07/08 align the affected capability/retained-data
contracts and atomic enforcement before activation. Existing three-vehicle source
ceiling and all other policy-v1 limits remain; this note is not an enabled quota
or an untested edit to machine-readable policy.

State: source contract frozen for PRM00-T02, 2026-10-13. [Machine-readable limits](../../contracts/premium/policy-v1.json) own exact source bounds; [Premium contract](PREMIUM_PRODUCT_CONTRACT.md) owns behavior. These are conservative implementation ceilings, not measured VPS capacity, product prices or an enabled paid service. User explicitly deferred payment-module/catalog setup and real provider evidence; deployment admission remains zero. Measure workload/cost/headroom before any later activation.

## Capability and cancellation matrix

ACTIVE and canceled-renewal-but-paid-through may grant only inside a fresh verified provider expiry. Provider GRACE preserves access through the provider's current expiry, with no locally invented extension. PENDING, pending-canceled, PAUSED, ON_HOLD, EXPIRED, REVOKED and unknown grant none. Every server operation rechecks active account, unrevoked session (for interactive commands), token-chain retirement, verified expiry and proof freshness. A provider outage cannot refresh proof. Freshness is 900s; boundary is exclusive. Future local offline lease is at most 24h and always clipped to verified paid-through time; server writes never use that lease. Clock rollback/future verification is refused.

Existing data read/export/correction/delete rights are independent of paid capability. No paid flag is read by community, moderation, account registration/recovery, local vehicles/history or basic alerts. Renew-cancel is distinct from immediate revocation. Financial/cloud metadata is owner-only, with no-store responses. Request account IDs never determine authority; derive owner from existing live session proof.

## Consent, retention and recovery

Cloud domains require explicit per-dataset opt-in. Credentials/keys/purchase tokens/permissions/precise GPS/photos/caches never enter user snapshots. Only acknowledged server revisions are protected. Snapshot cadence 24h, seven snapshots at most, 10 MiB each; three devices/vehicles, ten advanced rules, 50-operation batches/100-row pages, 20k ledger records and 12-month report query bounds are source ceilings. They are neither advertised entitlements nor capacity measurements. No extra vehicle slot is introduced.

Cloud expiry allows 30 days of owner-only exit download before scheduled cloud purge; retained local data is not purged. Live cloud/snapshot/report bytes become inaccessible immediately on erasure and are physically purged within the 24h active-storage objective; failed receipts remain tracked. Infrastructure backups have a separate maximum seven-day rotation and a deletion ledger outside the restored dataset; before serving restored data replay erasure/revocation and purge expired objects. This does not extend the existing operational photo cap. Actual provider/deletion deadline proofs are required before collection/paid activation. No new snapshot service is built by this batch.

Pending private operation receipts are pruned after 30 days only when no live job/lease references them. A terminated purchase's protected token and minimal anti-replay/chain receipt may remain for 90 days after verified access end; active/unknown chains cannot be pruned merely by age. Product/privacy policy must disclose the purpose; real collection is OFF until later privacy acceptance. Erasure cryptographically destroys recoverable token material immediately; a minimal nonreversible digest/retired-chain marker cannot regain rights or publicly identify an account. Later billing law/retention decisions must amend this policy before activation, not silently retain everything.

Use envelope encryption from standard-library AES-GCM for provider tokens with random nonce, dedicated external key and environment/package/account-bound AAD. Jobs/inbox carry digests and operation IDs, never plaintext tokens. Key management recovery is operator-owned and outside user backup; do not claim end-to-end encryption. No production key is introduced by this batch.

## Admission and provider behavior

At most two unfinished verification operations/account and 20 new operations/hour; identical completed replays do not spend another grant. Global paid admission is zero until future measured activation. Provider requests use five-second contexts, 64 KiB responses, strict expected HTTPS authority, no redirects and static redacted errors. No network call holds SQL row locks. Verification generations and 20s leases fence late replies; expired operations recover through a new attempt. A conflicting operation ID never changes its payload.

Acknowledgement intent is atomic with verified benefit persistence; five bounded attempts, stale-job fencing and replay retain recoverability. Provider notifications require independent OIDC signature/issuer/audience/service-account proof, bounded envelope/time, expected package and test/production routing. They request canonical refresh, never directly grant rights. Duplicate/out-of-order delivery cannot regress newer state.

## Decisions still outside this batch

Public product/base-plan IDs, prices, billing period, trial, Play credentials/OAuth wiring, merchant configuration, live notification subscription, billing Android UI, real sandbox lifecycle and all commercial release gates remain unset/PROOF_OWED by the user's clarification. A test fixture never supplies a deployable product or a manual grant path. Backend constructors accept explicit ports/configuration for the later module; production composition uses disabled provider ports.

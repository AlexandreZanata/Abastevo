# DRV-10 — Signed commands, capability and reconciliation contract

Date: 2026-10-26. B-BR-DRV-01/05/17; BUC-DRV-02/05/06/09. Freeze before behavior.

## Scope

This phase adds the private application command boundary above the DRV-07–09
stores. It does not add an HTTP router, OpenAPI document, mobile UI or cloud
opt-in; DRV-11/12 expose and certify the wire protocol. The driver store remains
private infrastructure.

## Signed owner proof and origin

- Every hosted write/read command passes through an `Authority` port that
  verifies the signed proof for the request. The verified principal carries
  owner, origin, contributor and key identity.
- The command service rejects a missing/failed proof, an owner mismatch (IDOR),
  a different canonical origin or a principal without contributor/key identity
  before touching storage. The proof must bind the request body hash; the port
  implementation owns that verification.
- The application service never trusts an owner/origin supplied by the client
  separately from the verified principal.

## Capability and retained data

- Hosted driver mutations recheck the live `professional.write` capability in
  the same transaction, before commit and therefore after the transaction's
  writes. The composition installs the subscription adapter through
  `Store.SetPaid`; an absent port is allowed only for private/local storage
  tests and is not valid hosted composition.
- A lapse/denial rolls back the complete command. Exact replay also rechecks
  capability.
- Reads, export and erasure are authenticated by owner proof but are not gated
  by Premium capability. They remain available after a lapse. Reusing the
  owner's retained records is not a new paid report.

## Reconciliation decisions

- Summary/detail and payout/earnings overlaps are never resolved by silently
  summing or deleting records. An explicit owner decision is append-only,
  versioned per decision ID and bound to an operation receipt.
- A decision names one primary earning, zero or more counterpart earning
  records, their exact revisions, platform and containing UTC window. SUMMARY_
  DETAIL requires at least one counterpart and both coverage kinds. PAYOUT_
  EARNINGS requires an opaque lowercase payout source hash and must not create
  another earning. Payout remains a cash fact, not earnings.
- Referenced records must belong to the owner, match platform/revision and fit
  inside the decision window. Foreign, stale, summary-only or missing records
  fail without a decision. Identical retries converge; changed bodies, stale
  versions and records changed after the decision are refused.
- Decisions share the earnings owner epoch. Record/source deletion and owner
  erasure delete decisions whose target is gone; an old epoch cannot recreate a
  decision. Report/import consumers must apply the decision before claiming
  non-duplicated totals; that consumption is DRV-11/19, not claimed here.

## Atomicity and recovery

- Duplicate operations converge to one receipt; a changed body or expected
  revision conflicts. Rejected corrections, cancellations, owner changes and
  stale epochs leave no partial earning, shift, distance, cost, maintenance or
  decision row.
- The owner advisory lock serializes driver mutations; capability and ownership
  are rechecked inside the same transaction. Cancellation after writes rolls
  back. No destructive migration is used.

## Evidence boundary

Real disposable PostGIS must cover signed-proof/owner/origin negatives, capa-
bility lapse with retained reads, vehicle/account denial, concurrent duplicate
commands, stale revisions, cancellation after writes, stale epochs and
reconciliation version/retry/rollback. HTTP status mapping, real identity-key
verification, OpenAPI and report consumption remain DRV-11/12.

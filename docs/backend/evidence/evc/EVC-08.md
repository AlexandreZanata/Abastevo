# EVC-08 refresh and generation differences

## EVC-08A source-based comparison

B-BR-EVC-08A. Match catalog generations through root source identities.
Record new and removed sites, source gains and losses, movement strictly
above 50 m and per-source connector changes. A place with a surviving
root is retained even if its canonical key changes. A prior root still
present in the next source inventory cannot be reported as removed.
A split or unexplained missing root is flagged for review.

BUC-EVC-08A. Compare synthetic generations with a two-to-one-source
transition, a true removal, a preserved upstream record missing from the
projection, and a movement around 50 m. Unchanged group values with
fresh pointers and source revisions do not produce connector changes.

The initial regression failed because the comparison was missing.
The comparator verifies both rights/geography bundles, checks that
the candidate inventory includes every published source assertion,
and reports deterministic changes sorted by source or site key.
An omitted root still present in the candidate inventory becomes a
review item. A confirmed site with a retained root is never counted
as removed. Connector values are compared by assertion identity.

Status: LOCAL_DONE / INTEGRATION_PENDING. Synthetic bundle, movement,
connector, split, source-loss and deterministic-order tests passed with
race detection, vet, staticcheck, whitespace and secret review.
Binding the complete inventory and invoking the comparator from the
scheduled operations entry point remain separate steps.

## EVC-08B verified source inventory

B-BR-EVC-08B. Bind the complete normalization decision inventory to
the reviewed candidate through its rights-pinned conflation manifest.
Accepted assertions may be projected. Quarantined and ignored records
still prove source presence and cannot be mistaken for removals.

BUC-EVC-08B. Verify the exact conflation, normalization and decision
hash chain. Reject tampered, duplicate, truncated and inconsistent
inputs. Replay the already approved national files without a database.

RED failed at the missing inventory verifier. The verifier reconciles
counts by source and state and binds the admitted catalog's own bytes,
hash and record count to the same reviewed manifest. A private or
quarantined source is present for diff purposes, but cannot appear in
the public projection. The comparator accepts only the verified
inventory value associated with the candidate manifest.

File-only replay of the already approved national input verified 1,775
decisions, 1,658 sites and 2,094 source-owned connector groups. Comparing
the same pinned generation produced an empty change set. No database
configuration was present for the replay.

Status: LOCAL_DONE / INTEGRATION_PENDING. Corruption, duplicate,
framing and reconciliation tests, race detection, vet, staticcheck,
whitespace and secret review passed.

## EVC-08C scheduled file-only review

B-BR-EVC-08C. Scheduled reviews read only owner-pinned candidate
and optional baseline directories. A weekly OCM timer and monthly OSM
timer invoke the same offline verifier. Each successful verification
writes a private, revision-bound report. Unapproved source acquisition
and catalog activation remain separate operator actions.

BUC-EVC-08C. Run the actual review command against a synthetic approved
candidate and against the already approved local national replay.
Verify private report creation, invalid configuration refusal and
the timer calendars without installing services on staging.

RED reproduced the missing review subcommand. The command now reads
an owner-local configuration, resolves a fixed bundle layout inside
opened roots and writes a mode-0600 JSON report atomically. A synthetic
test exercises the real entry point with an environment callback that
fails on any read. Invalid config fields are refused.

The real pinned replay through the built binary verified 1,775 decisions
and wrote a private empty repeat diff. The OCM Monday and first-of-month
OSM calendars parsed successfully. Both timer definitions and the service
template passed systemd verification against a temporary copy of the built
binary. No timer was installed or started.

Status: LOCAL_DONE / INTEGRATION_PENDING. Unit/race tests, vet,
staticcheck, whitespace and secret review passed. Binding the reviewed
baseline to the activation transaction remains open.

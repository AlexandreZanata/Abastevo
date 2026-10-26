# DRV-03–07 — Exact individual-driver model and private earnings persistence

Date: 2026-10-26. Selected by the owner after DRV-02. B-BR-DRV-02/04/05/06/07/08/10/12/13/14/15/17/18; BUC-DRV-01/02/05/06/09.

## Scope and isolation

Use the existing driver PR #152 (head `driver-suite/drv-00-02`, base `dev`).
Local work is isolated on `codex/driver-suite-drv-03-07`; do not push maintained
dev/main or alter PR #151. A normal merge of the published dev base is allowed
only to reconcile that base; review the PR diff against dev for driver-only scope.
Main's separate landing batch is not imported into this dev-targeted PR.
ADR-020's ordinary dev/main sequence resumes at cumulative integration; this
owner-requested separated PR is an exception, not a new repository-wide policy.

This batch implements pure rules and private backend storage, not the driver
hub, live GPS collection, a public HTTP route or a new paid entitlement. Those
remain in DRV-08 onward. No provider/bank/passenger integration is introduced.
Existing financial-v1 remains immutable. No new dependency is needed.

## DRV-03 money and coverage

BRL centavos, metres and seconds are integer inputs. Reuse the existing per-value
money/distance ceiling 10^12; quantities and previous ledger methods are unchanged.
Detailed earnings require all gross/fees/tips/bonus/refund components, each known
and nonnegative; earned = gross - fees + tips + bonus - refund. An inclusive net
summary has only its net amount; gross/fees stay unknown and included tips are
not added again. Negative results remain negative; use checked bounded sums and
arbitrary-precision rational intermediates. Typed REVERSAL (negative) or
CORRECTION (nonzero signed) adjustments link a record and expected revision;
never represent them as negative purchases. Payout/cash receipt is separate from
work earned. Complete matched owner/vehicle/window/currency coverage is required
for a complete operating result. Modeled estimates form a separately labeled
numerator; unknown/zero time or distance does not become a zero ratio. Weighted
ratios divide compatible sums. Input totals outside the ceiling fail visibly.

## DRV-04 session clock and replay

Pure local state machine: READY, RUNNING, PAUSED, CLOSED, RECOVERY_REQUIRED.
A conflict is an explicit rejected CAS/transition, not a second silently active
shift. Start/pause/resume/end/checkpoint use operation IDs and expected versions;
exact retry converges, conflicting/stale commands refuse. Retain at most the
last 50 receipts (the existing bounded command batch), so an older command must
still pass its version fence. One active shift per scoped authority is enforced
by an atomic application repository port, never claimed globally while offline.
Active milliseconds accumulate before conversion to seconds; paused time is not
worked time. UTC/IANA dates and monotonic boot/time are distinct. Caller-supplied
reviewed maximum checkpoint gap and wall-clock tolerance have no universal
magic defaults. Reboot, rollback, excessive gap or restore of an active shift
requires explicit gap review without adding missing work or restarting GPS.
Online-time intervals are half-open and use union, not a sum across platforms.

## DRV-05 distance

Observations retain source, owner/window/vehicle/segment and known coverage.
ODOMETER, GPS_ESTIMATE, PAID_REPORTED, MANUAL_TRIP and ROUTE_PREDICTION are distinct;
route prediction is not a measured denominator. Select one compatible source;
never add GPS and odometer. Reversed/cross-segment readings fail closed. Paid,
empty and personal partitions require explicit complete classification; otherwise
keep known raw values and unknown ratios. Synthetic GPS filtering uses a
versioned caller-supplied policy (accuracy, age, max gap, max speed, drift band),
not a fabricated device calibration. Reordered/stale/invalid/jumping fixes are
rejected. Tunnels/reboot/excessive gaps establish a new anchor without inventing
travel. The output is an estimate with quality/gap reasons. Raw traces are never
persisted to the server and no location permission/service is added here.

## DRV-06 cost meaning and allocation

Owned, leased and financed profiles are explicit, dated, and owner supplied.
Lease and owned depreciation are incompatible; financing principal is a cash/
capital movement, not an operating expense. Interest and fees may be separately
recorded. Actual money and estimates keep distinct source IDs/provenance.
No default depreciation, tax, commission or maintenance rate.

Allocation uses nonnegative explicit day/time/km weights, at most 1000 targets
(the existing report reference bound). Exact largest remainder followed by
stable ID breaks centavo ties. The sum equals the source amount across all
explicit buckets, including unallocated/private use; no allocation invents extra
money. Time/day proration uses a declared half-open source period with disjoint
subwindows and an explicit unallocated remainder. Missing/zero denominator is
not silently treated as complete coverage. A realized service replaces a linked
reserve only by an explicit replacement link; it does not create another ledger
expense. Corrections recompute from original sources; duplicated source IDs,
missing assumptions and incompatible classifications fail visibly.

## DRV-07 private storage and boundaries

Append migration 000133 after actual 000132; never rewrite old migrations. Store
BRL earnings with owner/vehicle, UTC half-open coverage, IANA timezone, platform
label, DETAIL/SUMMARY level, basis/components, immutable revisions, opaque SHA256
source identity and typed linked adjustments. No free-form note, passenger,
provider credential or raw route field. Owner/platform source identity is unique;
exact concurrent retries converge, a changed body/revision conflicts. Coverage
of one owner's same-platform records cannot overlap (summary/detail/import
reconciliation is explicit). Distinct owners remain isolated. Serializable
owner-locked mutations validate an owned, nondeleted vehicle through a
transaction-bound port implemented by its cloudsync owner, never driver SQL
against another module's private tables. Missing authorization port denies.

No HTTP route, sales grant or sync opt-in is invented. The adapter is private
infrastructure for the later authorized application/HTTP boundary (DRV-10–12).
Account soft/physical deletion erases new private records/receipts/revisions;
owned reads and explicit deletion/export ports retain scoped data exit. Register
the dataset as cloud-disabled until the actual signed backup/restore protocol is
implemented; do not claim DRV-16 from a table. Real disposable PostGIS proves
empty/132→133 migration, atomic rollback/recovery, duplicates/overlap races,
foreign ownership, linked adjustment revision and restart. No persistent/VPS DB.

### DRV-06 representation details before behavior

Stable allocation bucket IDs use bounded ASCII so Go byte ordering and Kotlin
ordering agree. Source amount can be explicitly unknown. DAY proration counts
ISO civil dates in a declared IANA timezone (not 86400-second assumptions across
DST); TIME proration uses explicit seconds. Both keep an unallocated remainder
bucket and refuse overlapping subwindows. Actual cost-profile entries must link
a ledger source/revision; principal is retained in the capital/cash bucket.
Reserve replacement links are explicit one-to-one, same-window source links;
replacing a reserve removes its entire modeled addition while keeping the
realized ledger cost once. Unknown amounts remain unknown, never default zero.

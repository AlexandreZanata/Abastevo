# F0 discovery protocol — Fleets and stations

Status: **PROTOCOL_DRAFTED**, 2026-11-10. Task: EMP-GOV-010.
This protocol prepares interviews and a synthetic walkthrough. Recruitment,
interviews and pilot operation require a separate authorized session.

## Participants and questions

Recruit by workflow and decision authority. For fleets, speak separately to
the person who fuels and records a trip, the reviewer who closes expenses,
and the buyer who controls the budget. Include fleets within the bands in the
[product vision](../02-product-vision.md#8-commercial-hypotheses), with at least
one workflow that regularly loses connectivity. For stations, include a
single-site operator and a multi-site operator. Ask who controls station
representation and whether the same person can approve fleet agreements.

1. Ask the participant to describe the last completed trip, fueling and
   review, or the last fleet transaction at a station. Record steps, handoffs,
   delays and failure recovery with generalized examples.
2. Ask which record was missing or disputed, who could correct it and how
   the correction became visible. For a fleet, cover offline use and review.
   For a station, cover proof of representation, negotiated conditions and
   reconciliation.
3. Walk the participant through a synthetic [cut C1](../14-roadmap.md#4-first-operational-cut-c1)
   example: assigned vehicle, offline trip and fueling, fictitious receipt,
   sync and return for correction. Ask which step would prevent adoption.
   Keep the station example separate and describe its representation and
   agreement capabilities as future work.
4. Ask what the current workflow costs in time or tools before showing
   the product's price hypotheses. Record which charging unit fits the
   workflow, who could approve the spend, and what change in scope or price
   would affect that approval.

## Packaging hypotheses

- **Fleets:** compare the proposed vehicle bands against charging per active
  vehicle and charging for document storage separately. Ask how seasonal
  fleets would count an active vehicle. Use the existing Starter and Pro
  figures only as interview prompts after the workflow discussion.
- **Stations:** test whether per-managed-station pricing fits single-site and
  multi-site workflows. Ask which capability would justify paying and what
  decisions belong at the station or network level.

Distinguish participant preference, willingness to seek approval and actual
budget authority. Keep stated willingness separate from observed purchase
behavior. Aggregate operational problems and price objections by segment.
Record counterexamples and unanswered questions alongside recurring patterns.
Return proposed scope changes to product review and pricing evidence to the
organization billing decision. These findings do not set prices or activate a
sales channel.

## Evidence and pilot gate

Use a participant code, role and size band in the synthesis. Keep contact
information outside the repository. Use fabricated examples and collect no
receipt image, identifier, location trace, employee record or payment data.
Ask permission before taking notes. Record no raw audio or screenshots.

The interview output is a synthesis with participant counts by segment,
observed workflow steps, adoption blockers, and disputed or unsupported
hypotheses. Later real-data pilots require the existing legal and release
gates. For an authorized cut C1 pilot, use the
[product measures](../02-product-vision.md#9-measures-of-success): receipt
legibility, time to review, offline acceptance without manual repair and
two-organization isolation evidence. Record a denominator and observation
window for each quantitative result.

# Mobile driver hub and Premium states contract

Date: 2026-10-26. B-BR-DRV-01/20. Freeze before behavior.

## Navigation entry

- A real `driver-hub` route is reachable from Perfil and from Veículos. It is a
  child of the Perfil tab, so the four-tab shell (Explorar, Comunidade, Mapa,
  Perfil), existing map deep links and back behavior stay unchanged.
- The hub exposes only existing real destinations: vehicle form/list, ledger
  entries and monthly indicators/export. No driver function is fabricated.

## Access states and retained data

- The hub derives state from the signed session and server-derived Premium
  status: VERIFIED, OFFLINE, LAPSED, UNAVAILABLE or SIGNED_OUT. It never infers
  entitlement from a local toggle.
- OFFLINE/UNAVAILABLE retain local vehicle/ledger/monthly actions and a retry.
  LAPSED still offers read, correction and monthly export of held data; the
  Premium action opens the existing Premium screen. No checkout, price, trial
  or trust/reputation change is created.
- SIGNED_OUT offers the existing sign-in flow and never shows another owner's
  records.

## First use and failures

- With no vehicle, the hub explains the vehicle-first path and opens the real
  vehicle form/list. With a vehicle, it opens real ledger entries and monthly
  indicators/export.
- Loading uses the shared loading state; vehicle/status failures show actionable
  retry/error copy. No placeholder success is shown.
- Shift control is not implemented in this phase (DRV-18); the hub does not
  render a fake shift action. This is an explicit scope boundary, not an
  acceptance claim for shift forms.

## Evidence boundary

JVM route/state tests and owned-emulator Compose tests must cover the hub,
states and four-tab/back behavior. The driver mobile/day gates remain later.

## iOS Driver Hub

The personal hub offers signed-out manual shifts and account-scoped hosted accounting. Manual shift recovery works with the local vehicle and exact window. GPS is an explicit option for a running shift. See the [iOS GPS lifecycle](gps-lifecycle.md#ios-session-lifecycle) for stop conditions and evidence.

Hosted earnings require an explicitly selected platform, interval, IANA time zone and exact centavos. The person declares either an inclusive net or every item in the breakdown. An absent value stays absent. The hub can enroll the selected vehicle after vehicle-dataset consent and device verification. It submits the earnings with the original operation ID through the protected signed queue. A conflict parks the immutable entry and prompts review.

Retained earnings and signed report reads continue when paid access lapses. Building a new report requires the server's current `professional.report` capability. See the [revisioned reports contract](reports.md) for coverage and signed route requirements.

# MAP-27 implementation contract

Scope: a bounded charging-stop planner that combines a real road route from the pinned native
engine with the MAP-26 pure energy model. Written 2026-10-26 before behavior. No new map source,
provider call, live availability claim, backend table, payment or "guaranteed arrival" promise.

B-BR-MAP27-01 (units and provenance): distances are metres, duration seconds, energy Wh, power W,
state of charge and reserve basis points of the owner-declared useful capacity, consumption
milli-Wh/km, money integer milli-BRL. Every candidate keeps its source profile, generation/source
revision, connector/current/raw code, optional declared power, optional dated report, access and
hours; a missing field stays typed as unknown. Static reference, dated community report and live
availability are distinct. No live status is admitted.

B-BR-MAP27-02 (road evidence): a candidate is only a stop when the pinned router returns a real
road leg from the current point. Straight-line distance may prefilter a corridor but never becomes
a drivable leg. Route warnings stay warnings; no route, missing maps or a refused leg never turns
into a reachable stop. Geographic near-duplicates are grouped by site and connector IDs, and a
connector at the same site/equipment is not called an independent fallback without evidence.

B-BR-MAP27-03 (energy): each leg uses `EvEnergyModel` with the explicit consumption margin. A leg
is reachable only when the arrival state of charge is at or above the declared reserve. A stop
charges only to the declared target SOC; charging energy is rounded up and cost/time are separate
results. The energy received at a charger is assumed only after the charger is selected; a failed
primary provides no energy. PHEV results keep `ELECTRIC_PORTION_ONLY`; liquid fuel is outside the
model and is never counted as electric reach.

B-BR-MAP27-04 (failure and alternative): a fallback starts from the actual state of charge on
arrival at the failed primary, after its detour, and uses the real road leg from that point to the
alternative. It must still respect the reserve. Failures carry scope SITE, EQUIPMENT or CONNECTOR;
a site failure excludes the whole site, an equipment failure its equipment, a connector failure that
connector. Any fallback candidate from the failed site is excluded because shared-site independence
is unproven. An energy-reachable alternative with unknown access/hours/tariff/power is reported as
unknown operation, never as confirmed availability.

B-BR-MAP27-05 (access and hours): private or explicitly closed candidates are not automatic stops.
Access UNKNOWN remains UNKNOWN and is surfaced. Opening hours need a real zone; an unknown zone,
unknown hours or unknown arrival time stays UNKNOWN and is not evaluated with a single Brazil zone.
Known hours are checked at the computed arrival instant; an overnight schedule is supported.

B-BR-MAP27-06 (prices and power): tariff is either a > 0 exact milli-BRL/kWh value with condition,
source and observation instant, or UNKNOWN. UNKNOWN is never zero and never wins a comparison by
cost. Declared charger power is only an upper bound; charge time is the MAP-26 lower bound or
UNKNOWN when power is missing. No charge curve, live occupancy, reservation or availability is
invented.

B-BR-MAP27-07 (bounds): the planner has declared limits for candidates, stops, route calls,
alternative checks and deadline. Each route call consumes the shared budget. Exhaustion returns an
explicit incomplete status, never a truncated result presented as the global optimum. Cancellation
and a changed generation prevent a late callback from publishing a result.

B-BR-MAP27-08 (refusals): invalid/duplicate points, missing battery/consumption/connector, bad
target SOC, unknown snapshot coverage, no route, missing map region, no compatible candidate,
unreachable candidate and insufficient energy remain distinct outcomes/reasons. No default zero,
no ANP->community fallback and no claim that the car will not run out.

BUC-MAP27: route input + vehicle connector/battery/reserve/consumption -> main road route ->
bounded corridor candidates -> real road legs -> energy/reserve check -> primary stop with charge
target -> alternative from failed-primary energy state -> explicit unknowns/gaps. A fallback is
only claimed when its road leg and arrival reserve are proven under the declared assumptions.

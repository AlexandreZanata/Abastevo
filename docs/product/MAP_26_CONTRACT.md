# MAP-26 implementation contract

Scope: a pure, conservative electric-energy model in the Kotlin domain. No backend
change, no API, no storage, no UI, no charging-stop planning (MAP-27), no provider or
elevation/climate source. Written 2026-10-26 before behavior.

B-BR-MAP26-01 (units, exact): useful battery capacity in Wh, state of charge and reserve in
basis points of that useful capacity (0..10,000; 1 bp = 0.01 %), consumption in
milli-Wh per km (1 Wh/km = 1,000), distance in metres, charger power in W, time in seconds. All
arithmetic is integer/rational with explicit rounding directions chosen to be conservative:
usable energy and reachable distance round DOWN; consumed energy and consumption calibration
round UP; arrival state of charge rounds DOWN. Every result is `Known(value)` or `Unknown(reason)`;
there is no zero default for a missing input.

B-BR-MAP26-02 (usable energy): usable = capacity x (soc - reserve) / 10,000 (floor); soc at or below
reserve is a known 0, not negative. Capacity is the USEFUL capacity the owner declares, never a
nominal/gross label assumed to be useful. A PHEV additionally declares the permille of that useful
capacity available to electric driving; its results carry scope ELECTRIC_PORTION_ONLY and never claim
a whole-trip feasibility because liquid fuel is outside this model.

B-BR-MAP26-03 (segments): segment energy = distance x consumption / 10^6 (ceil) with an optional
non-negative sensitivity margin in permille (+0..+1000) applied to consumption; reachable distance
and arrival state of charge are the inverse/consequence of the same arithmetic. Laboratory or
nominal range is never an input. Elevation and climate are not modelled; the margin is the only
sensitivity input and is explicit in the result.

B-BR-MAP26-04 (calibration): consumption may be calibrated only from private completed sessions
(energy Wh and distance metres each > 0), sum-first (total energy / total distance, never an average
of ratios), requiring at least 20,000 m of total distance; otherwise `calibration-insufficient`.

B-BR-MAP26-05 (charging time): declared charger power is only an upper bound on delivered power, so
the model returns a LOWER BOUND on time (energy x 3,600 / power, floor). It never returns a point
estimate; vehicle acceptance, charge curve, state of charge and temperature are unknown components.

B-BR-MAP26-06 (refusals): negative or out-of-range inputs, zero consumption/power, missing inputs and
results above the existing energy/distance ceilings produce an `Unknown` with a stable reason; nothing
throws for data the caller may legitimately lack. No claim of guaranteed trip feasibility is made.

BUC-MAP26: manual battery/soc/reserve/consumption (or calibrated consumption) -> usable energy ->
reach/arrival with a margin -> unknowns for any missing input; vectors hand-calculated independently
and monotonic behaviour checked.

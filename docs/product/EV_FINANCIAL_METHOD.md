# EV energy and cost method — proposed exact cross-language vectors

Date: 2026-10-20. State: CONTRACT_PROPOSED / IMPLEMENTATION_NOT_STARTED.
Owner: [EV rules](EV_SUPPORT_CONTRACT.md), EE-02/11/15/16/23. Extends, never
reinterprets, [financial-v1](PREMIUM_FINANCIAL_METHOD.md). Freeze machine vectors
before code; numeric input ceilings/rounding precision follow a recorded need
and existing bounds, not arbitrary guesses or provider float examples.

**EE-02 checkpoint, 2026-10-21:** [exact policy/data contract](EV_TYPED_DATA_CONTRACT.md)
and24 shared CSV arithmetic vectors are frozen and validated in Go/Kotlin/Rust.
This is contract arithmetic, not acceptance of the EE-11 runtime tariff engine,
measurement/window method, provider billing or SQL/report implementation.

**EE-11 checkpoint, 2026-10-21:** the pure tariff scenario engine is implemented
in Go/Kotlin with a Rust parity oracle over [42 shared synthetic rows](../../contracts/testdata/ev/tariff-v1.csv)
([evidence](../backend/evidence/ev-support/EE-11.md)). Only INCLUDED tax with
complete mandatory components, no undeclared minimum/maximum stage and the
supported final half-up rounding produce KNOWN centavos; absent energy,
incomplete/unknown fees, eligibility mismatch, partial validity windows and
money overflow are explicit refusals/UNKNOWN. This remains a local pure engine:
provider billing terms, SQL/report persistence, monthly timezone allocation and
public ranking are not accepted by this checkpoint.

## Basis and valid windows

Persist actual paid BRL in centavos; energy in an exact declared Wh-compatible
scale, power W, time seconds and odometer metres. Raw rate decimal, unit, billing
step, eligibility, minimum/maximum, tax basis, validity and timezone are preserved
and converted using checked integer/rational operations. Freeze final display
rounding and provider billing rounding separately; no rounding each intermediate
unless the actual tariff specifies it. Cross-language Rust/Go/Kotlin vectors must
agree. Unknown fees, amount, energy or basis are nullable, never zero defaults.

Keep four methods distinguishable: reference/modelled vehicle efficiency;
meter/charger-basis purchased-energy interval; actually measured vehicle energy;
and monetary observed-window cost. Compare only the same method/basis/unit and
matching eligibility. No phone GPS distance or nominal battery inferred energy
silently completes an odometer/energy window.

An electric matched interval requires a declared source/measurement basis,
positive complete distance and included energy with accepted start/end charge
state/anchor/gap policy. Different battery state at endpoints affects bought
energy versus consumption; disclose that limitation or return UNKNOWN under the
frozen method. Never call arbitrary sum of purchases divided by driving distance
exact battery efficiency. For a PHEV do not infer distance split between motors.

Monthly money totals use event occurrence in the selected IANA timezone. A
session can preserve start, end and payment timestamps; the frozen event-time
rule selects the monthly expense basis. Crossing midnight/month does not permit
inventing per-minute energy/cost without interval evidence. A monthly total does
not establish a complete monthly distance denominator. Retain separately labelled
valid observed-window metrics. A backdated correction invalidates dependent output.

## Worked synthetic acceptance vectors

1. **Unit conversion:** 150 Wh/km = 0.15 kWh/km = 15 kWh/100 km = 0.54 MJ/km.
   A PBE field labelled equivalent km/L is retained under its actual method; no
   implicit conversion to physical litres or guessed kWh based on that label.
2. **Actual energy charge:** 30 kWh at BRL 1.80/kWh = BRL 54.00; plus explicit
   BRL 2.00 session fee = BRL 56.00. At 160 km over a valid same-basis matched
   interval, bought energy is 18.75 kWh/100 km and cost is BRL 0.35/km, labelled
   purchased-energy/session-cost basis, not battery laboratory efficiency.
3. **Separate parking:** add BRL 8.00 parking to the previous same-window case:
   fuel/energy-session cost stays BRL 56.00, all included cost becomes BRL 64.00
   and BRL 0.40/km. Do not include parking twice or label BRL 0.40 energy-only.
4. **Multi-component tariff:** 12 kWh at BRL 2.00/kWh plus 600 charging seconds
   at BRL 0.50/minute plus BRL 2.00 fixed = BRL 31.00 before idle charge. Six
   idle minutes with five-minute grace, one-minute billing steps and BRL 1.00
   per charged minute add BRL 1.00: total BRL 32.00. Freeze inclusive boundary
   and step semantics with additional zero/exact-boundary/partial-step cases.
5. **Free is explicit:** known zero energy/session price plus BRL 10.00 parking
   is not a free whole visit. Missing energy price cannot be treated as zero;
   unknown mandatory fees prevent ranking by complete estimated total.
6. **PHEV monetary aggregation:** 25 L at BRL 6.00 = BRL 150.00 plus 20 kWh at
   BRL 2.00 = BRL 40.00; total BRL 190.00 over a valid same 500 km window is
   BRL 0.38/km. Do not derive liquid efficiency as 500/25 or electric efficiency
   as 500/20 without proven propulsion-specific distance/energy method.
7. **Measurement distinction:** 18 kWh measured at supply and 16 kWh measured
   at battery over verified corresponding boundaries differ by 2 kWh. Preserve
   both measurement bases and provenance; do not assert a universal loss rate,
   degradation or every future session's efficiency from this observation.
8. **SOC estimate:** nominal 60 kWh and 50 percentage-point SOC change does not
   establish 30 metered kWh. Usable/gross capacity, display estimation and losses
   are unknown unless supplied. Retain a labelled model estimate separately;
   do not overwrite the reported meter energy or actual paid total.
9. **Home tariff:** separate TE/TUSD references are incomplete as a final home
   rate without the applicable other components and basis. A user-supplied known
   effective BRL/kWh can drive a labelled estimate; actual paid session allocation
   remains a separate asserted amount. Solar/free/employer-paid is explicit data,
   not inferred from a location label or a vehicle model.
10. **Wallet is not a second session:** adding BRL 100.00 prepaid credit and later
    paying BRL 40.00 for one session does not create BRL 140.00 of consumed-energy
    expense. Wallet/payment processing is outside this scope; do not fabricate
    ledger energy or double count imported session totals and tariff components.
11. **Unknown or incompatible windows:** missing distance, mismatched SOC/gaps,
    meter-basis change, partial-month coverage and reset segments produce the
    relevant UNKNOWN reason; raw monetary/energy totals remain readable. A cost
    correction/deletion recomputes affected revisions, not an unrelated vehicle.
12. **Comparability and bounds:** unequal-distance fleet/vehicle summaries use
    eligible summed energy/cost/distance, never average individual ratios. Keep
    L, m3 and kWh totals separate. Refuse negative/overflow/noncanonical decimal,
    unknown currency conversion, unsupported precision and zero denominators.

These are test targets, not observed customer results, active billing terms or
implemented measurements. Add timezone/daylight-change, member/nonmember,
effective-tariff switch, provider rounding, capped/minimum charge and incomplete
components to the frozen test matrix before ranking a real offer.

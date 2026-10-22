# Electric vehicle data contract v1 — EE-02

Date2026-10-21. State: CONTRACT_FROZEN / RUNTIME_PENDING.
Owns B-BR-EV-01–07/10–12/16–22 and BUC-EV-01–12 at the contract level.
[Shapes](../../contracts/openapi/ev-support-planned.json),
[numeric policy](../../contracts/ev/numeric-policy-v1.json),
[shape examples](../../contracts/testdata/ev/support-v1.json) and
[shared exact examples](../../contracts/testdata/ev/exact-v1.csv) are staged;
there are no live routes, SQL/Room migrations or newly granted capabilities.

## Identity, vehicle capability and legacy evolution

EnergyVehicle holds one stable vehicle ID, propulsion, explicitly supported/
unsupported/unknown external charging, optional tank/fuel/battery/source specs,
connector support and independent AC/DC limits. ICE, BEV, PHEV, HEV, MHEV,
range extender, other and unknown are distinct. BEV needs no liquid tank/fuel;
PHEV may carry both energy systems and occupies one registration slot. HEV/MHEV
cannot be presented as externally chargeable from their names. Unknown source
propulsion retains its source label without coercion to ICE/BEV.

Gross and usable battery Wh stay optional and separate; no guessed default or
zero for unknown. Contradictory assertions remain source-qualified; the chosen
effective private specification requires correction rather than silently choosing
one. Do not infer actual charging speed, safe adapters, session duration, battery
degradation or vehicle-compatible connectors solely from an advertised station W.

Exact model/market/year/trim/edition/reference basis is optional. PBE_LAB,
manufacturer assertion, manual estimate and measured values are different methods.
Equivalent km/L is not real liquid use. No inferred manufacturer connector/battery
spec is supplied by an unrelated PBE energy-efficiency row.

Legacy vehicle UUIDs/fields keep their historical meanings. GASOLINE_PREMIUM is
the persisted Android additive legacy, normalized to GASOLINE_ADDITIVED for the
existing Go wire; GASOLINE_PREMIUM_GRADE remains separate. A legacy provenance
field in the staged example does not authorize changing old history or inventing
electricity as a FuelProduct. Old Room tank Double stays legacy-only; new energy/
money values use the exact contract. Actual migrations belong to EE-10/19 and
MEX-07/08, including existing-data upgrade/rollback/reopen acceptance.

Wire version1 is explicit. Unknown source connector codes can normalize to
UNKNOWN with their bounded source code; unknown top-level schema/unsupported
methods produce an explained unsupported state and retain accepted local data.
Never map an unfamiliar token to a known connector/fuel or fabricate compatibility.

## Public place, equipment and evidence boundaries

PublicLocation, EVSE and Connector have separate stable internal IDs and parent
references, each with source-scoped external ID/revision and rights/attribution
reference. No CNPJ/name/coordinate-only identity merge; co-located Directory
station links are optional and separately proven. Child equipment/connector
counts and simultaneous capacity are independent nullable facts. Missing child
identity is not permission to create a fictitious counted EVSE.

PublicLocation requires PUBLIC_VERIFIED publication eligibility. Access/hours,
price and availability may still be unknown/restricted. Coordinates refer only
to eligible public infrastructure; missing coordinates remain null. Private/home
addresses, contacts/comments/contributor positions and raw photos are absent.
Source qualification, an open-data flag or a community suggestion cannot itself
supply that public eligibility or a license over private metadata.

SourceAssertion binds source/external ID/revision, rights record, license,
attribution/change notice and observation clock. Only admitted field/provider
rights permit later API/offline packs. OCM export/API share lineage; repeated
snapshots/votes do not establish independent evidence. EE-01 samples are not
source admission; PBE rights are still pending. EE-03–09/18 own those gates.

Operational status has a separate source and expiration. Static catalog or dated
community report can only support UNKNOWN current operational status. An actual
AVAILABLE/OCCUPIED/FAULT/OUT_OF_SERVICE claim needs licensed operator basis and
real observation/expiry; the adapter must also verify observed<=now<expiry and
the source's accepted freshness. Future, expired, unavailable or unauthorized
evidence becomes UNKNOWN, never a renewed status from fetch/save time. The
schema checks structure, not proof of a real feed; EE-17 owns live acceptance.

## Exact numeric limits and arithmetic

Actual paid amounts: integer BRL centavos0..1e12. Odometer: integer metres0..1e12.
These retain the existing PRM ledger bounds, not new advertised storage rights.
Normalized whole Wh and W use0..1e9 (capacity/known max power strictly positive):
a conservative transport safety bound reusing the existing quantity scale limit,
not a physical EV maximum or proof of source correctness. An unrepresentable
source fraction must retain raw/source evidence and get UNSUPPORTED_PRECISION;
never round it silently into a measured session. References/rates use rationals.

SOC is0..10,000 basis points with explicit dashboard/BMS/estimate/unknown basis;
one unit is0.01 percentage point. It is not energy metering. Tariff scenarios
bound time to31,622,400 seconds (366 days, covering the existing at-most12-month
report envelope); this is a technical calculation/input budget, not a meaningful
charging duration or retention promise. Source exceptions require a reviewed
version amendment, never truncation or wraparound.

Rates are canonical reduced nonnegative numerator/positive denominator strings,
numerator<=1e12 and denominator<=1e9. These permit exact fractional centavo rates
and decimal source prices without binary floats. Numerators/denominators have no
sign, exponent or leading zero (except numerator0). Domain validation must also
check reduction and semantic ceilings. JSON integer transport stays within2^53-1;
intermediates may exceed64 bits and require exact checked/rational arithmetic.
Rust's test oracle uses i128 for the reviewed small vectors only; it is not
evidence that every future product of inputs fits i128. No new bigint dependency
or numerical runtime engine is introduced by EE-02.

Wh/kWh/W/seconds/metres/MJ/km have distinct meaning. Preserve raw numeric lexemes
before exact decimal conversion; source JSON must not pass through f64 and then
claim to preserve original precision. Unsupported precision, overflow, units or
currency fail explicitly without losing raw facts. Never sum L/M3 and Wh or
average ratios unweighted. Go/Kotlin/Rust share24 independently reviewed
arithmetic examples: conversions, actual fees, purchased-energy basis, parking,
time, idle grace/ceil steps, zero, fractional rates, PHEV money, SOC model and
rounding. Known result schemas bind method to unit, not just any number.

## Tariffs, fees and comparison

A tariff binds target location/EVSE/connector, revision, BRL, condition/qualifier,
timezone, effective interval, tax basis, explicit known components and rounding.
Components are energy Wh, charging seconds, idle seconds, session count and
parking seconds with rates in centavos per that base unit. For each accepted
component, quantity is rounded UP to its explicit billing_step after applicable
grace, then multiplied by the exact rate. SESSION quantity/step must be1;
grace applies only to sourced time components. Do not interpret a per-step price
as a per-second rate. Unrecognized source semantics remain unsupported.

The reference example charges301 idle seconds with300s grace and60s steps as one
charged minute; exactly300 is zero. This is a declared synthetic tariff, never a
universal operator billing rule. Minimum/maximum applies only at a source-declared
stage/basis; validate min<=max. A supported final-half-up-centavo tariff sums
exact components before its declared final rounding boundary. Display rounding
is separate from provider billing rules. Unknown tax inclusion/mandatory fees,
steps, min/max basis or unsupported rounding prevents total-cost ranking.

STANDARD has no membership qualifier; APP/MEMBER/EMPLOYER must identify known
eligibility. Unknown eligibility cannot become a universal standard offer.
Explicit zero energy is different from absent energy price and may coexist with
paid parking. An empty component list is not a free tariff. A source free-text
UsageCost stays a dated note; no regex/default-zero complete tariff is created.
Currency conversion, wallet processing, payment/start-stop and reservations remain
outside scope. Compare compatible complete energy rates with excluded fees
labelled, or exact same energy/time/parking scenario and eligible conditions.
Unknown mandatory components produce UNKNOWN_FEES, not the cheapest result.

## Private sessions, reports and frozen ports

PrivateSession has stable ID/vehicle, user-confirmed occurred_at and IANA entry
timezone, optional start/end/payment-derived context, actual total/energy when
known, measurement basis, optional odometer/SOC and tariff revision. An explicit
zero can be valid; missing measured energy or total is null. No GPS, home address,
photo, raw bill, wallet top-up or public post is required or automatically created.
Owner is derived from existing authenticated/signed account context outside the
payload; a request-supplied account ID cannot select somebody else's history.

Occurred_at is the user-confirmed expense event instant. Preserve session times;
crossing midnight/month does not allocate energy/cost into arbitrary minutes.
Reports use the chosen IANA timezone/half-open month, raw totals plus separate
matched windows, revisions and correction invalidation. Bought energy/distance
is labelled PURCHASED_ENERGY_INTERVAL with complete acquisitions and declared
distance/window/basis; it is not measured vehicle energy. Unknown SOC cannot
make that a vehicle-efficiency measurement. VEHICLE_MEASURED_ENERGY needs its
own complete matching vehicle measurement evidence; no fixed loss factor or
nominal battery inventory substitutes for it.

MATCHED_MONEY_WINDOW combines actual fuel/charging/selected expenses once over
the same valid distance window. No PHEV propulsion split from total kilometres,
no refuel+all-expenses double counting, no wallet credit plus session double
count. SOC*capacity is MODEL_ONLY, never metered Wh. Missing distance/gaps,
unsupported method/precision, mismatched bases or mandatory unknown fees produces
an explicit UNKNOWN result with null value; raw records/totals remain accessible.

Existing module ports are extended rather than replaced:

- Charging catalog reads: typed filters+source vector+bounded cursor → page or
  unavailable, anonymous; no ingestion/provider fan-out inside the request.
- Rust preparation: local file+pinned admitted profile/rights → verified typed
  artifacts/census; Go jobs own fetch/lease/atomic publication (EE-03–08).
- Community target resolution: stable place/EVSE/connector+source scope → eligible
  target/unavailable, through a charging-owned port; signed report lifecycle stays
  in existing community with no GPS requirement (EE-12–14/22).
- Private energy commands: existing owner/device/operation/revision/epoch/consent
  envelope → durable receipt/conflict; existing ledger/metrics/cloud modules own
  this boundary, not a new sync/ledger service (EE-15/16/24, MEX-10–15).
- Pure tariff calculation: exact tariff+declared scenario+known basis → method/
  units/value or explained unknown; no network, SQL, entitlement or provider call
  in the numerical domain (EE-11).
- Android public packs: permitted typed source snapshots → validated atomic Room
  generation; preserve private data and old accepted generation on failure
  (EE-18). Cache scope/expiry follows MEX/OPT; no national PDF/PBF scan on UI.

One-free/additional-Premium and held-data rights follow registration v2. Private
professional generation/writes/cloud keep the existing PRM capability matrix;
public discovery/community do not gain a paid gate. Sync excludes keys, tokens,
home/route coordinates, raw bills/photos and public caches. No future D document/
business portal or iOS scope is reopened. Source rights, real operator access,
actual SQL/HTTP/Room/window/crypto/native acceptance remain downstream obligations.

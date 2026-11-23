import Foundation

/// Shared premium vectors copied verbatim from `contracts/testdata/premium` (Go and Kotlin execute the
/// same files). `scripts/tests/test_ios_app_integrity.py` checks that every line still matches the repository.
enum LedgerFixtures {
    static let financialExecution = """
        id,operation,anchor,fills,distance,fuel_cost,expenses,window_start_odo,window_end_odo,window_start_ms,window_end_ms,has_gap,crosses_segment,odometer_missing,expected_quantity,expected_cost,expected_distance,expected_efficiency,expected_cost_per_km,expected_change,expected_unknown
        full-partial-full,INTERVAL,50000:30000:1:ETHANOL:L,20000:12000:0:ETHANOL:L;20000:12400:1:ETHANOL:L,400000,,,100000,500000,1000,2000,0,0,0,40000,24400,400000,10,61,,
        discount-actual-total,INTERVAL,1:1:1:ETHANOL:L,20000:11500:0:ETHANOL:L;20000:11500:1:ETHANOL:L,400000,,,100000,500000,1000,2000,0,0,0,40000,23000,400000,10,115/2,,
        weighted-vehicles,SUM,1:1:1:ETHANOL:L,100000:10000:6000:_;900000:60000:36000:_,400000,,,100000,500000,1000,2000,0,0,0,70000,42000,1000000,100/7,42,,
        noninteger-distance,INTERVAL,1:1:1:ETHANOL:L,12345:7406:1:ETHANOL:L,123450,,,100000,500000,1000,2000,0,0,0,12345,7406,123450,10,148120/2469,,
        partial-only,INTERVAL,1:1:0:ETHANOL:L,1:1:0:ETHANOL:L,400000,,,100000,500000,1000,2000,0,0,0,,,,,,,full-anchors-required
        missing-start,INTERVAL,1:1:0:ETHANOL:L,1:1:1:ETHANOL:L;1:1:1:ETHANOL:L,400000,,,100000,500000,1000,2000,0,0,0,,,,,,,full-anchors-required
        trailing-partial,INTERVAL,1:1:1:ETHANOL:L,1:1:1:ETHANOL:L;1:1:0:ETHANOL:L,400000,,,100000,500000,1000,2000,0,0,0,,,,,,,full-anchors-required
        missing-odometer,INTERVAL,1:1:1:ETHANOL:L,1:1:1:ETHANOL:L,400000,,,100000,500000,1000,2000,0,0,1,,,,,,,odometer-missing
        known-fill-gap,INTERVAL,1:1:1:ETHANOL:L,1:1:1:ETHANOL:L,400000,,,100000,500000,1000,2000,1,0,0,,,,,,,declared-fill-gap
        reset-segment,INTERVAL,1:1:1:ETHANOL:L,1:1:1:ETHANOL:L,400000,,,100000,500000,1000,2000,0,1,0,,,,,,,odometer-segment-boundary
        zero-distance,INTERVAL,1:1:1:ETHANOL:L,1:1:1:ETHANOL:L,0,,,100000,500000,1000,2000,0,0,0,,,,,,,distance-nonpositive
        mixed-fuel,INTERVAL,1:1:1:ETHANOL:L,1:1:1:ETHANOL:L;1:1:1:GASOLINE_REGULAR:L,400000,,,100000,500000,1000,2000,0,0,0,,,,,,,unsupported-mixed-method
        anchor-fuel-mismatch,INTERVAL,1:1:1:GASOLINE_REGULAR:L,1:1:1:ETHANOL:L,400000,,,100000,500000,1000,2000,0,0,0,,,,,,,unsupported-mixed-method
        gnv-pressure,INTERVAL,1:1:1:CNG:M3,1:1:1:CNG:M3,400000,,,100000,500000,1000,2000,0,0,0,,,,,,,unsupported-gnv-method
        mixed-unit,INTERVAL,1:1:1:ETHANOL:L,1:1:1:ETHANOL:L;1:1:1:CNG:M3,400000,,,100000,500000,1000,2000,0,0,0,,,,,,,incompatible-unit
        invalid-sole-unit,INTERVAL,1:1:1:ETHANOL:M3,1:1:1:ETHANOL:M3,400000,,,100000,500000,1000,2000,0,0,0,,,,,,,incompatible-unit
        overflow-input,INTERVAL,1:1:1:ETHANOL:L,1000000001:1:1:ETHANOL:L,400000,,,100000,500000,1000,2000,0,0,0,,,,,,,input-out-of-bounds
        large-valid-cost-scaling,INTERVAL,1:1:1:ETHANOL:L,10000*1:1000000000000:1:ETHANOL:L,1,,,100000,500000,1000,2000,0,0,0,10000,10000000000000000,1,1/10000,10000000000000000000,,
        large-weighted-scaling,SUM,1:1:1:ETHANOL:L,10000*1:1:1000000000000:_,400000,,,100000,500000,1000,2000,0,0,0,10000,10000000000000000,10000,1,1000000000000000,,
        sum-overflow,SUM,1:1:1:ETHANOL:L,1:1:9223372036854775807:_;1:1:1:_,400000,,,100000,500000,1000,2000,0,0,0,,,,,,,input-out-of-bounds
        unknown-interval-excluded,SUM,1:1:1:ETHANOL:L,100000:10000:6000:_;0:0:0:odometer-missing;900000:60000:36000:_,400000,,,100000,500000,1000,2000,0,0,0,70000,42000,1000000,100/7,42,,
        month-partial-distance,MATCHED,1:1:1:ETHANOL:L,,300000,24400,,100000,500000,1000,2000,0,0,0,,,,,,,unmatched-period-window
        matched-negative-cost,MATCHED,1:1:1:ETHANOL:L,,400000,-1,,100000,500000,1000,2000,0,0,0,,,,,,,input-out-of-bounds
        matched-invalid-odometer,MATCHED,1:1:1:ETHANOL:L,,400000,24400,,-9223372036854775808,9223372036854775807,1000,2000,0,0,0,,,,,,,input-out-of-bounds
        matched-window-and-dedup,MATCHED,1:1:1:ETHANOL:L,,400000,23000,e:5000:1000:200000;e:5000:1500:200000;late:9000:2000:200000,100000,500000,1000,2000,0,0,0,,28000,400000,,70,,
        matched-large-scaling,MATCHED,1:1:1:ETHANOL:L,,1,0,10000*e#:1000000000000:1500:_,1,2,1000,2000,0,0,0,,10000000000000000,1,,10000000000000000000,,
        single-cost-max,COST,1:1:1:ETHANOL:L,,1,1000000000000,,100000,500000,1000,2000,0,0,0,,1000000000000,1,,1000000000000000,,
        zero-baseline,COMPARE,1:1:1:ETHANOL:L,100000:10000:0:_;900000:60000:36000:_,400000,,,100000,500000,1000,2000,0,0,0,,,,,,,comparison-baseline-zero
        comparison-positive,COMPARE,1:1:1:ETHANOL:L,100000:10000:6000:_;900000:60000:36000:_,400000,,,100000,500000,1000,2000,0,0,0,,,,,,50,
        comparison-negative,COMPARE,1:1:1:ETHANOL:L,900000:60000:36000:_;100000:10000:6000:_,400000,,,100000,500000,1000,2000,0,0,0,,,,,,-100/3,
        comparison-incomplete-base,COMPARE,1:1:1:ETHANOL:L,0:0:0:odometer-missing;1:1:1:_,400000,,,100000,500000,1000,2000,0,0,0,,,,,,,baseline-incomplete: odometer-missing
        comparison-incomplete-current,COMPARE,1:1:1:ETHANOL:L,1:1:1:_;0:0:0:declared-fill-gap,400000,,,100000,500000,1000,2000,0,0,0,,,,,,,current-incomplete: declared-fill-gap
        comparison-invalid-numerator,COMPARE,1:1:1:ETHANOL:L,1:1:1:_;0:1:1:_,400000,,,100000,500000,1000,2000,0,0,0,,,,,,,input-out-of-bounds
        sum-distance-overflow,SUM,1:1:1:ETHANOL:L,9223372036854775807:1:1:_;1:1:1:_,400000,,,100000,500000,1000,2000,0,0,0,,,,,,,input-out-of-bounds
        sum-quantity-overflow,SUM,1:1:1:ETHANOL:L,1:9223372036854775807:1:_;1:1:1:_,400000,,,100000,500000,1000,2000,0,0,0,,,,,,,input-out-of-bounds
        sum-negative-input,SUM,1:1:1:ETHANOL:L,1:1:-1:_,400000,,,100000,500000,1000,2000,0,0,0,,,,,,,input-out-of-bounds
        """

    static let energyExecution = """
        id,purchased_wh,cost_centavos,distance_metres,has_energy,complete,has_gap,crosses_segment,odometer_missing,soc_inconsistent,mixed_carrier,expected_wh_per_km,expected_centavos_per_km,expected_unknown,fuel_cost_centavos,charge_cost_centavos,expected_combined_centavos_per_km,expected_combined_unknown
        weighted-sum-first,40000,4000,150000,true,true,false,false,false,false,false,800/3,80/3,,0,0,,
        exact-small,15000,1275,100000,true,true,false,false,false,false,false,150,51/4,,0,0,,
        phev-shared-distance,15000,1275,100000,true,true,false,false,false,false,true,,,phev-shared-distance,0,0,,
        soc-inconsistent,10000,1000,50000,true,true,false,false,false,true,false,,,soc-inconsistent,0,0,,
        partial-measurement,10000,1000,50000,true,false,false,false,false,false,false,,,partial-measurement,0,0,,
        energy-unmeasured,0,1000,50000,false,false,false,false,false,false,false,,,energy-unmeasured,0,0,,
        declared-fill-gap,10000,1000,50000,true,true,true,false,false,false,false,,,declared-fill-gap,0,0,,
        odometer-missing,10000,1000,0,true,true,false,false,true,false,false,,,odometer-missing,0,0,,
        distance-nonpositive,10000,1000,0,true,true,false,false,false,false,false,,,distance-nonpositive,0,0,,
        combined-exact,0,0,100000,false,false,false,false,false,false,false,,,,24800,1275,1043/4,
        combined-zero-distance,0,0,0,false,false,false,false,false,false,false,,,,1000,500,,distance-nonpositive
        """

    static let financialWire = """
        id,fuel,unit,quantity_milli,total_centavos,accepted
        ethanol,ETHANOL,L,20000,11500,1
        gasoline-regular,GASOLINE_REGULAR,L,20000,11500,1
        gasoline-additived,GASOLINE_ADDITIVED,L,20000,11500,1
        gasoline-premium,GASOLINE_PREMIUM_GRADE,L,20000,11500,1
        diesel-s500,DIESEL_S500,L,20000,11500,1
        diesel-s10,DIESEL_S10,L,20000,11500,1
        gnv-raw-valid,CNG,M3,20000,11500,1
        maximum,ETHANOL,L,1000000000,1000000000000,1
        zero-paid,ETHANOL,L,1,0,1
        flex-is-vehicle-not-fill,FLEX,L,20000,11500,0
        ev-not-liquid,ELECTRICITY,kWh,20000,11500,0
        unknown-fuel,BATTERY,L,20000,11500,0
        noncanonical-fuel,ethanol,L,20000,11500,0
        noncanonical-unit,ETHANOL,l,20000,11500,0
        unknown-unit,ETHANOL,GAL,20000,11500,0
        gnv-wrong-unit,CNG,L,20000,11500,0
        liquid-wrong-unit,ETHANOL,M3,20000,11500,0
        negative-quantity,ETHANOL,L,-1,11500,0
        zero-quantity,ETHANOL,L,0,11500,0
        quantity-bound,ETHANOL,L,1000000001,11500,0
        negative-total,ETHANOL,L,20000,-1,0
        total-bound,ETHANOL,L,20000,1000000000001,0
        """

    struct RegistrationCase {
        let id: String
        let registeredBefore: Int
        let newDistinctIds: Int
        let scope: String
        let authority: String
        let outcome: String
        let registeredAfter: Int
    }

    /// `vehicle-registration-v2.json`, version 2.
    static let vehicleRegistration: [RegistrationCase] = [
        RegistrationCase(id: "free-first", registeredBefore: 0, newDistinctIds: 1, scope: "LOCAL", authority: "NONE", outcome: "ACCEPT", registeredAfter: 1),
        RegistrationCase(id: "free-second", registeredBefore: 1, newDistinctIds: 1, scope: "LOCAL", authority: "NONE", outcome: "PREMIUM_REQUIRED", registeredAfter: 1),
        RegistrationCase(id: "legacy-two-correct", registeredBefore: 2, newDistinctIds: 0, scope: "LOCAL", authority: "NONE", outcome: "ACCEPT", registeredAfter: 2),
        RegistrationCase(id: "legacy-three-read", registeredBefore: 3, newDistinctIds: 0, scope: "LOCAL", authority: "NONE", outcome: "ACCEPT", registeredAfter: 3),
        RegistrationCase(id: "expired-three-new", registeredBefore: 3, newDistinctIds: 1, scope: "LOCAL", authority: "EXPIRED", outcome: "CAPACITY_EXCEEDED", registeredAfter: 3),
        RegistrationCase(id: "expired-two-correct", registeredBefore: 2, newDistinctIds: 0, scope: "LOCAL", authority: "EXPIRED", outcome: "ACCEPT", registeredAfter: 2),
        RegistrationCase(id: "expired-first-new-second", registeredBefore: 1, newDistinctIds: 1, scope: "LOCAL", authority: "EXPIRED", outcome: "PREMIUM_REQUIRED", registeredAfter: 1),
        RegistrationCase(id: "premium-third", registeredBefore: 2, newDistinctIds: 1, scope: "LOCAL", authority: "SERVER_VERIFIED", outcome: "ACCEPT", registeredAfter: 3),
        RegistrationCase(id: "premium-fourth", registeredBefore: 3, newDistinctIds: 1, scope: "LOCAL", authority: "SERVER_VERIFIED", outcome: "CAPACITY_EXCEEDED", registeredAfter: 3),
        RegistrationCase(id: "concurrent-free-first", registeredBefore: 0, newDistinctIds: 1, scope: "LOCAL", authority: "NONE", outcome: "ACCEPT", registeredAfter: 1),
        RegistrationCase(id: "concurrent-free-second", registeredBefore: 1, newDistinctIds: 1, scope: "LOCAL", authority: "NONE", outcome: "PREMIUM_REQUIRED", registeredAfter: 1),
        RegistrationCase(id: "import-two-atomic-refusal", registeredBefore: 0, newDistinctIds: 2, scope: "LOCAL", authority: "NONE", outcome: "PREMIUM_REQUIRED", registeredAfter: 0),
        RegistrationCase(id: "restore-same-ids", registeredBefore: 3, newDistinctIds: 0, scope: "LOCAL", authority: "EXPIRED", outcome: "ACCEPT", registeredAfter: 3),
        RegistrationCase(id: "replay-same-id", registeredBefore: 1, newDistinctIds: 0, scope: "LOCAL", authority: "NONE", outcome: "ACCEPT", registeredAfter: 1),
        RegistrationCase(id: "offline-valid", registeredBefore: 1, newDistinctIds: 1, scope: "LOCAL", authority: "LOCAL_VALID_LEASE", outcome: "ACCEPT", registeredAfter: 2),
        RegistrationCase(id: "offline-expired", registeredBefore: 1, newDistinctIds: 1, scope: "LOCAL", authority: "EXPIRED", outcome: "PREMIUM_REQUIRED", registeredAfter: 1),
        RegistrationCase(id: "clock-rollback", registeredBefore: 1, newDistinctIds: 1, scope: "LOCAL", authority: "CLOCK_ROLLBACK", outcome: "PREMIUM_REQUIRED", registeredAfter: 1),
        RegistrationCase(id: "offline-hosted-refused", registeredBefore: 1, newDistinctIds: 1, scope: "HOSTED", authority: "LOCAL_VALID_LEASE", outcome: "HOSTED_AUTHORITY_REQUIRED", registeredAfter: 1),
        RegistrationCase(id: "free-hosted-first", registeredBefore: 0, newDistinctIds: 1, scope: "HOSTED", authority: "NONE", outcome: "HOSTED_AUTHORITY_REQUIRED", registeredAfter: 0),
        RegistrationCase(id: "premium-hosted-first", registeredBefore: 0, newDistinctIds: 1, scope: "HOSTED", authority: "SERVER_VERIFIED", outcome: "ACCEPT", registeredAfter: 1),
        RegistrationCase(id: "stale-proof-second", registeredBefore: 1, newDistinctIds: 1, scope: "LOCAL", authority: "PROOF_STALE", outcome: "PREMIUM_REQUIRED", registeredAfter: 1),
        RegistrationCase(id: "unknown-policy-second", registeredBefore: 1, newDistinctIds: 1, scope: "LOCAL", authority: "UNKNOWN_POLICY", outcome: "PREMIUM_REQUIRED", registeredAfter: 1),
        RegistrationCase(id: "legacy-over-ceiling-correct", registeredBefore: 4, newDistinctIds: 0, scope: "LOCAL", authority: "NONE", outcome: "ACCEPT", registeredAfter: 4),
    ]
}

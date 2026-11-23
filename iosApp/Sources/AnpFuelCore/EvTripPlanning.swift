import Foundation

/// MAP-26/MAP-27 EV trip planning over admitted chargers.
/// Swift port of Android `EvEnergyModel`, `EvChargingPlanRules`, `EvRouteGeometry`,
/// `EvCandidateMapper` and `PlanEvTripUseCase`. Integer arithmetic only:
/// usable energy, reach and arrival state of charge round down; consumed energy
/// rounds up. Road legs always come from an `EvRoadRouter`; a straight line is
/// never substituted for a road. The plan estimates; it never guarantees a trip.
/// Zero postfix unwrap operators anywhere in this file.

public enum EnergyEstimate: Equatable, Sendable {
    case known(Int64)
    case unknown(String)

    public var value: Int64? {
        if case .known(let value) = self { return value }
        return nil
    }
}

public enum EnergyScope: String, Equatable, Sendable {
    case battery
    case electricPortionOnly
}

/// Owner-declared battery facts. Capacity is useful Wh; soc/reserve are basis points of it;
/// a PHEV declares the permille of capacity available to electric driving.
public struct EvBatteryInput: Equatable, Sendable {
    public var capacityWh: Int64?
    public var socBp: Int64?
    public var reserveBp: Int64?
    public var usablePermille: Int64?

    public init(capacityWh: Int64?, socBp: Int64?, reserveBp: Int64?, usablePermille: Int64? = nil) {
        self.capacityWh = capacityWh
        self.socBp = socBp
        self.reserveBp = reserveBp
        self.usablePermille = usablePermille
    }
}

/// Consumption in milli-Wh per km (150 Wh/km = 150,000) plus an optional margin in permille.
public struct EvConsumptionInput: Equatable, Sendable {
    public var consMilliWhPerKm: Int64?
    public var marginPermille: Int64?

    public init(consMilliWhPerKm: Int64?, marginPermille: Int64? = 0) {
        self.consMilliWhPerKm = consMilliWhPerKm
        self.marginPermille = marginPermille
    }
}

public struct EvCalibrationSession: Equatable, Sendable {
    public let energyWh: Int64
    public let distanceMetres: Int64

    public init(energyWh: Int64, distanceMetres: Int64) {
        self.energyWh = energyWh
        self.distanceMetres = distanceMetres
    }
}

/// Exact non-negative 64 x 64 -> 128-bit products divided by a 64-bit divisor.
enum EvExact {
    /// floor(a x b / d), or nil when the quotient does not fit 64 bits.
    static func mulDivFloor(_ a: UInt64, _ b: UInt64, _ d: UInt64) -> UInt64? {
        guard d > 0 else { return nil }
        let product = a.multipliedFullWidth(by: b)
        guard product.high < d else { return nil }
        return d.dividingFullWidth((high: product.high, low: product.low)).quotient
    }

    /// ceil(a x b / d), or nil when the quotient does not fit 64 bits.
    static func mulDivCeil(_ a: UInt64, _ b: UInt64, _ d: UInt64) -> UInt64? {
        guard d > 0 else { return nil }
        let product = a.multipliedFullWidth(by: b)
        guard product.high < d else { return nil }
        let division = d.dividingFullWidth((high: product.high, low: product.low))
        if division.remainder == 0 { return division.quotient }
        let (rounded, overflow) = division.quotient.addingReportingOverflow(1)
        return overflow ? nil : rounded
    }

    /// a x b when the product fits 64 bits.
    static func mul(_ a: UInt64, _ b: UInt64) -> UInt64? {
        let (value, overflow) = a.multipliedReportingOverflow(by: b)
        return overflow ? nil : value
    }
}

public enum EvEnergyModel {
    public static let maxEnergyWh: Int64 = 1_000_000_000_000
    public static let maxDistanceMetres: Int64 = 1_000_000_000_000
    public static let maxConsumptionMilli: Int64 = 1_000_000_000
    public static let maxMarginPermille: Int64 = 1_000
    public static let minCalibrationMetres: Int64 = 20_000

    public static func scope(of battery: EvBatteryInput) -> EnergyScope {
        battery.usablePermille != nil ? .electricPortionOnly : .battery
    }

    private struct Checked {
        let capacity: Int64
        let soc: Int64
        let reserve: Int64
        let portion: Int64?
    }

    private static func check(_ battery: EvBatteryInput) -> Result<Checked, EvEstimateFailure> {
        guard let capacity = battery.capacityWh else { return .failure(EvEstimateFailure("capacity-missing")) }
        guard let soc = battery.socBp else { return .failure(EvEstimateFailure("soc-missing")) }
        guard let reserve = battery.reserveBp else { return .failure(EvEstimateFailure("reserve-missing")) }
        guard (1...maxEnergyWh).contains(capacity), (0...10_000).contains(soc), (0...10_000).contains(reserve) else {
            return .failure(EvEstimateFailure("input-out-of-range"))
        }
        if let portion = battery.usablePermille, !(1...1_000).contains(portion) {
            return .failure(EvEstimateFailure("input-out-of-range"))
        }
        return .success(Checked(capacity: capacity, soc: soc, reserve: reserve, portion: battery.usablePermille))
    }

    /// capacity x (soc - reserve) / 10,000 [x portion / 1,000], floor; at or below reserve is a known 0.
    public static func usableEnergyWh(_ battery: EvBatteryInput) -> EnergyEstimate {
        switch check(battery) {
        case .failure(let failure):
            return .unknown(failure.reason)
        case .success(let checked):
            let above = UInt64(max(checked.soc - checked.reserve, 0))
            let capacity = UInt64(checked.capacity)
            let computed: UInt64?
            if let portion = checked.portion {
                computed = EvExact.mul(capacity, above).flatMap { EvExact.mulDivFloor($0, UInt64(portion), 10_000_000) }
            } else {
                computed = EvExact.mulDivFloor(capacity, above, 10_000)
            }
            guard let result = computed, result <= UInt64(Int64.max) else { return .unknown("out-of-bounds") }
            return .known(Int64(result))
        }
    }

    /// Effective consumption numerator over 10^6 (milli x permille), or the refusal.
    private static func effective(_ consumption: EvConsumptionInput) -> Result<UInt64, EvEstimateFailure> {
        guard let cons = consumption.consMilliWhPerKm else { return .failure(EvEstimateFailure("consumption-missing")) }
        if cons <= 0 { return .failure(EvEstimateFailure("consumption-nonpositive")) }
        if cons > maxConsumptionMilli { return .failure(EvEstimateFailure("input-out-of-range")) }
        let margin = consumption.marginPermille ?? 0
        guard (0...maxMarginPermille).contains(margin) else { return .failure(EvEstimateFailure("margin-invalid")) }
        return .success(UInt64(cons) * UInt64(1_000 + margin))
    }

    private static func checkDistance(_ distance: Int64?) -> EvEstimateFailure? {
        guard let distance else { return EvEstimateFailure("distance-missing") }
        if distance < 0 { return EvEstimateFailure("negative-input") }
        if distance > maxDistanceMetres { return EvEstimateFailure("input-out-of-range") }
        return nil
    }

    /// Energy for a segment: distance x consumption x (1 + margin) / 10^9, rounded up.
    public static func segmentEnergyWh(distanceMetres: Int64?, consumption: EvConsumptionInput) -> EnergyEstimate {
        let eff: UInt64
        switch effective(consumption) {
        case .failure(let failure): return .unknown(failure.reason)
        case .success(let value): eff = value
        }
        if let failure = checkDistance(distanceMetres) { return .unknown(failure.reason) }
        guard let distance = distanceMetres,
              let wh = EvExact.mulDivCeil(UInt64(distance), eff, 1_000_000_000),
              wh <= UInt64(maxEnergyWh) else {
            return .unknown("out-of-bounds")
        }
        return .known(Int64(wh))
    }

    /// Distance the usable energy covers at the effective consumption, rounded down; not a guarantee.
    public static func reachableDistanceMetres(battery: EvBatteryInput, consumption: EvConsumptionInput) -> EnergyEstimate {
        let usable: Int64
        switch usableEnergyWh(battery) {
        case .unknown(let reason): return .unknown(reason)
        case .known(let value): usable = value
        }
        let eff: UInt64
        switch effective(consumption) {
        case .failure(let failure): return .unknown(failure.reason)
        case .success(let value): eff = value
        }
        guard let metres = EvExact.mulDivFloor(UInt64(usable), 1_000_000_000, eff), metres <= UInt64(maxDistanceMetres) else {
            return .unknown("out-of-bounds")
        }
        return .known(Int64(metres))
    }

    /// State of charge (bp of useful capacity) after a segment, floor; may fall below any reserve.
    public static func arrivalSocBp(capacityWh: Int64?, socBp: Int64?, distanceMetres: Int64?, consumption: EvConsumptionInput) -> EnergyEstimate {
        guard let capacity = capacityWh else { return .unknown("capacity-missing") }
        guard let soc = socBp else { return .unknown("soc-missing") }
        let eff: UInt64
        switch effective(consumption) {
        case .failure(let failure): return .unknown(failure.reason)
        case .success(let value): eff = value
        }
        guard let distance = distanceMetres else { return .unknown("distance-missing") }
        guard (1...maxEnergyWh).contains(capacity), (0...10_000).contains(soc) else { return .unknown("input-out-of-range") }
        if let failure = checkDistance(distance) { return .unknown(failure.reason) }
        guard let needed = EvExact.mulDivCeil(UInt64(distance), eff, 1_000_000_000) else { return .unknown("energy-exceeds-battery") }
        let storedScaled = UInt64(capacity) * UInt64(soc)
        let neededScaled = needed.multipliedFullWidth(by: 10_000)
        if neededScaled.high > 0 || neededScaled.low > storedScaled { return .unknown("energy-exceeds-battery") }
        return .known(Int64((storedScaled - neededScaled.low) / UInt64(capacity)))
    }

    /// LOWER BOUND on charging time: energy x 3,600 / declared power, floor.
    public static func minimumChargeSeconds(energyWh: Int64?, declaredPowerW: Int64?) -> EnergyEstimate {
        guard let energy = energyWh else { return .unknown("energy-missing") }
        guard let power = declaredPowerW else { return .unknown("power-missing") }
        if energy < 0 { return .unknown("negative-input") }
        if power <= 0 { return .unknown("power-nonpositive") }
        if energy > maxEnergyWh { return .unknown("input-out-of-range") }
        guard let seconds = EvExact.mulDivFloor(UInt64(energy), 3_600, UInt64(power)) else { return .unknown("out-of-bounds") }
        return .known(Int64(seconds))
    }

    /// Sum-first calibration: total Wh / total km, rounded up, over at least 20 km.
    public static func calibrateConsumptionMilli(_ sessions: [EvCalibrationSession]) -> EnergyEstimate {
        if sessions.isEmpty { return .unknown("calibration-insufficient") }
        if sessions.contains(where: { $0.energyWh <= 0 || $0.distanceMetres <= 0 || $0.energyWh > maxEnergyWh || $0.distanceMetres > maxDistanceMetres }) {
            return .unknown("session-invalid")
        }
        let energy = sessions.reduce(UInt64(0)) { $0 + UInt64($1.energyWh) }
        let distance = sessions.reduce(UInt64(0)) { $0 + UInt64($1.distanceMetres) }
        if distance < UInt64(minCalibrationMetres) { return .unknown("calibration-insufficient") }
        guard let milli = EvExact.mulDivCeil(energy, 1_000_000, distance), milli <= UInt64(maxConsumptionMilli) else {
            return .unknown("out-of-bounds")
        }
        return .known(Int64(milli))
    }
}

struct EvEstimateFailure: Error, Equatable {
    let reason: String

    init(_ reason: String) {
        self.reason = reason
    }
}

public struct EvRoutePoint: Equatable, Hashable, Sendable {
    public let latitude: Double
    public let longitude: Double
    public let label: String

    public init(latitude: Double, longitude: Double, label: String = "") {
        self.latitude = latitude
        self.longitude = longitude
        self.label = label
    }

    var key: String { "\(latitude),\(longitude)" }
}

public enum EvRoadUnavailable: String, Equatable, Sendable {
    case noRoute
    case missingMaps
    case cancelled
    case timeout
    case budgetExhausted
    case notReady
    case unknown
}

public enum EvRoadLeg: Equatable, Sendable {
    case ready(distanceMeters: Int64, durationSeconds: Int64?, warnings: Bool, geometry: [EvRoutePoint])
    case unavailable(EvRoadUnavailable)

    public var distanceMeters: Int64? {
        if case .ready(let distance, _, _, _) = self { return distance }
        return nil
    }

    public var durationSeconds: Int64? {
        if case .ready(_, let duration, _, _) = self { return duration }
        return nil
    }

    var hasWarnings: Bool {
        if case .ready(_, _, let warnings, _) = self { return warnings }
        return false
    }

    var geometry: [EvRoutePoint] {
        if case .ready(_, _, _, let geometry) = self { return geometry }
        return []
    }
}

/// Real road distance and geometry for one ordered leg.
public protocol EvRoadRouter: Sendable {
    func leg(points: [EvRoutePoint]) async -> EvRoadLeg
}

public enum EvAccess: String, Equatable, Sendable {
    case publicAccess
    case unknown
    case privateAccess
}

public enum EvHoursStatus: String, Equatable, Sendable {
    case open
    case closed
    case unknown
}

/// One connector-level candidate. The catalog has no hours or tariff: both stay unknown.
public struct EvChargingCandidate: Equatable, Sendable {
    public let candidateId: String
    public let siteId: String
    public let siteName: String
    public let equipmentId: String
    public let connectorId: String
    public let connector: ChargingConnector
    public let current: ChargingCurrent
    public let powerW: Int64?
    public let access: EvAccess
    public let hours: EvHoursStatus
    public let tariffMilliBrlPerKwh: Int64?
    public let sourceRevision: String
    public let observedAt: Date?
    public let point: EvCoordinate

    public init(
        candidateId: String, siteId: String, siteName: String, equipmentId: String, connectorId: String,
        connector: ChargingConnector, current: ChargingCurrent, powerW: Int64?, access: EvAccess,
        hours: EvHoursStatus = .unknown, tariffMilliBrlPerKwh: Int64? = nil, sourceRevision: String,
        observedAt: Date? = nil, point: EvCoordinate
    ) {
        self.candidateId = candidateId
        self.siteId = siteId
        self.siteName = siteName
        self.equipmentId = equipmentId
        self.connectorId = connectorId
        self.connector = connector
        self.current = current
        self.powerW = powerW
        self.access = access
        self.hours = hours
        self.tariffMilliBrlPerKwh = tariffMilliBrlPerKwh
        self.sourceRevision = sourceRevision
        self.observedAt = observedAt
        self.point = point
    }

    var routePoint: EvRoutePoint {
        EvRoutePoint(latitude: point.latitude, longitude: point.longitude, label: siteName)
    }
}

public enum EvFailureScope: String, Equatable, Sendable {
    case site
    case equipment
    case connector
}

public struct EvFailure: Equatable, Sendable {
    public let scope: EvFailureScope
    public let siteId: String
    public let equipmentId: String?
    public let connectorId: String?

    public init(scope: EvFailureScope, siteId: String, equipmentId: String? = nil, connectorId: String? = nil) {
        self.scope = scope
        self.siteId = siteId
        self.equipmentId = equipmentId
        self.connectorId = connectorId
    }
}

public enum EvCoverage: String, Equatable, Sendable {
    case present
    case missing
    case unknown
}

public struct EvCandidateResult: Equatable, Sendable {
    public let candidates: [EvChargingCandidate]
    public let coverage: EvCoverage
    public let sourceRevision: String?

    public init(candidates: [EvChargingCandidate], coverage: EvCoverage, sourceRevision: String? = nil) {
        self.candidates = candidates
        self.coverage = coverage
        self.sourceRevision = sourceRevision
    }
}

public enum EvIssueCode: String, Equatable, Sendable {
    case invalidRoute
    case invalidBattery
    case invalidConsumption
    case invalidTargetSoc
    case invalidLimits
    case invalidFailure
    case batteryMissing
    case consumptionMissing
    case vehicleConnectorMissing
    case snapshotMissing
    case coverageUnknown
    case noCompatibleCandidate
    case incompatibleConnector
    case duplicateCandidate
    case noRoadRoute
    case missingMaps
    case routeCancelled
    case routeTimeout
    case routeNotReady
    case routeBudgetExhausted
    case roadWarning
    case accessPrivate
    case accessUnknown
    case hoursClosed
    case hoursUnknown
    case powerUnknown
    case tariffUnknown
    case reportDateUnknown
    case stopUnreachable
    case destinationUnreachable
    case stopLimitReached
    case chargeTargetAlreadyMet
    case alternativeSameSite
    case alternativeUnreachable
    case alternativeUnknownRoad
    case noEvidencedAlternative
    case energyUnknown
    case incomplete
}

public struct EvIssue: Equatable, Hashable, Sendable {
    public let code: EvIssueCode
    public let candidateId: String?

    public init(_ code: EvIssueCode, candidateId: String? = nil) {
        self.code = code
        self.candidateId = candidateId
    }
}

public enum EvPlanStatus: String, Equatable, Sendable {
    case planned
    case invalidInput
    case noRoute
    case insufficientData
    case noCandidates
    case insufficientEnergy
    case budgetExhausted
    case cancelled
}

public struct EvTripInput: Equatable, Sendable {
    public var origin: EvRoutePoint
    public var destination: EvRoutePoint
    public var departureAt: Date?
    public var battery: EvBatteryInput
    public var consumption: EvConsumptionInput
    public var vehicleConnector: ChargingConnector?
    public var chargeTargetSocBp: Int64
    public var maxStops: Int = 3
    public var corridorRadiusMeters: Int = 25_000
    public var maxCandidates: Int = 200
    public var maxRouteCalls: Int = 48
    public var maxCandidateProbes: Int = 6
    public var maxAlternativeChecks: Int = 6
    public var failures: [EvFailure] = []
    public var initialPoint: EvRoutePoint?
    public var initialSocBp: Int64?

    public init(
        origin: EvRoutePoint, destination: EvRoutePoint, departureAt: Date?, battery: EvBatteryInput,
        consumption: EvConsumptionInput, vehicleConnector: ChargingConnector?, chargeTargetSocBp: Int64
    ) {
        self.origin = origin
        self.destination = destination
        self.departureAt = departureAt
        self.battery = battery
        self.consumption = consumption
        self.vehicleConnector = vehicleConnector
        self.chargeTargetSocBp = chargeTargetSocBp
    }
}

public struct EvPlannedLeg: Equatable, Sendable {
    public let from: EvRoutePoint
    public let to: EvRoutePoint
    public let road: EvRoadLeg
    public let energyWh: EnergyEstimate
    public let arrivalSocBp: EnergyEstimate
}

public struct EvAlternative: Equatable, Sendable {
    public let candidate: EvChargingCandidate
    public let leg: EvPlannedLeg
    public let arrivalSocBp: EnergyEstimate
    public let issues: [EvIssueCode]
}

public struct EvPlannedStop: Equatable, Sendable {
    public let candidate: EvChargingCandidate
    public let arrival: EvPlannedLeg
    public let arrivalSocBp: Int64
    public let targetSocBp: Int64
    public let chargeEnergyWh: Int64
    public let minimumChargeSeconds: EnergyEstimate
    public let estimatedCostMilliBrl: EnergyEstimate
    public let issues: [EvIssueCode]
    public let alternative: EvAlternative?
}

public struct EvTripPlan: Equatable, Sendable {
    public let status: EvPlanStatus
    public let scope: EnergyScope
    public let mainRoute: EvPlannedLeg?
    public let stops: [EvPlannedStop]
    public let finalLeg: EvPlannedLeg?
    public let arrivalSocBp: EnergyEstimate?
    public let coverage: EvCoverage
    public let candidateCount: Int
    public let routeCalls: Int
    public let issues: [EvIssue]
    public let sourceRevision: String?
}

/// Prefilter geometry only; drivable legs still come from the router.
public enum EvRouteGeometry {
    static let earthRadiusMetres = 6_371_000.0

    public struct Projection: Equatable, Sendable {
        public let alongMetres: Int64
        public let offRouteMetres: Int64
    }

    public struct Box: Equatable, Sendable {
        public let minLatitude: Double
        public let maxLatitude: Double
        public let minLongitude: Double
        public let maxLongitude: Double

        public func contains(_ point: EvCoordinate) -> Bool {
            point.latitude >= minLatitude && point.latitude <= maxLatitude
                && point.longitude >= minLongitude && point.longitude <= maxLongitude
        }
    }

    public static func boundingBox(_ route: [EvRoutePoint], paddingMeters: Int) -> Box? {
        guard let minLat = route.map(\.latitude).min(), let maxLat = route.map(\.latitude).max(),
              let minLon = route.map(\.longitude).min(), let maxLon = route.map(\.longitude).max(),
              paddingMeters >= 0 else { return nil }
        let meanLat = min(80.0, max(-80.0, (minLat + maxLat) / 2.0))
        let latPad = Double(paddingMeters) / 111_320.0
        let lonPad = Double(paddingMeters) / max(1.0, 111_320.0 * cos(meanLat * .pi / 180.0))
        return Box(
            minLatitude: max(-90.0, minLat - latPad),
            maxLatitude: min(90.0, maxLat + latPad),
            minLongitude: max(-180.0, minLon - lonPad),
            maxLongitude: min(180.0, maxLon + lonPad)
        )
    }

    public static func distanceMeters(_ aLat: Double, _ aLon: Double, _ bLat: Double, _ bLon: Double) -> Double {
        let dLat = (bLat - aLat) * .pi / 180.0
        let dLon = (bLon - aLon) * .pi / 180.0
        let sLat = sin(dLat / 2)
        let sLon = sin(dLon / 2)
        let h = sLat * sLat + cos(aLat * .pi / 180.0) * cos(bLat * .pi / 180.0) * sLon * sLon
        return 2 * earthRadiusMetres * asin(min(1.0, sqrt(h)))
    }

    /// Nearest projection of `point` on the route with cumulative along-route metres.
    public static func project(_ point: EvCoordinate, on route: [EvRoutePoint]) -> Projection? {
        guard route.count >= 2 else { return nil }
        let cosLat = cos(point.latitude * .pi / 180.0)
        func x(_ lon: Double) -> Double { (lon - point.longitude) * .pi / 180.0 * earthRadiusMetres * cosLat }
        func y(_ lat: Double) -> Double { (lat - point.latitude) * .pi / 180.0 * earthRadiusMetres }
        var cumulative = 0.0
        var bestAlong = 0.0
        var bestOff = Double.greatestFiniteMagnitude
        for index in 0..<(route.count - 1) {
            let a = route[index]
            let b = route[index + 1]
            let ax = x(a.longitude)
            let ay = y(a.latitude)
            let dx = x(b.longitude) - ax
            let dy = y(b.latitude) - ay
            let lengthSquared = dx * dx + dy * dy
            let t = lengthSquared <= 0 ? 0.0 : min(1.0, max(0.0, (-ax * dx - ay * dy) / lengthSquared))
            let px = ax + t * dx
            let py = ay + t * dy
            let off = sqrt(px * px + py * py)
            let segment = distanceMeters(a.latitude, a.longitude, b.latitude, b.longitude)
            if off < bestOff {
                bestOff = off
                bestAlong = cumulative + t * segment
            }
            cumulative += segment
        }
        return Projection(alongMetres: Int64(bestAlong), offRouteMetres: Int64(bestOff))
    }
}

public enum EvChargingPlanRules {
    public static let maxStops = 5
    public static let maxCandidates = 500
    public static let maxRouteCalls = 200
    public static let maxProbes = 20
    public static let maxAlternativeChecks = 20
    /// Eligibility codes that remove a candidate; the others are shown as caveats.
    public static let blockingCodes: [EvIssueCode] = [.incompatibleConnector, .accessPrivate, .hoursClosed]

    public static func validate(_ input: EvTripInput) -> [EvIssue] {
        var issues: [EvIssue] = []
        let points = [input.origin, input.destination]
        if points.contains(where: { !validPoint($0) }) || Set(points.map(\.key)).count != points.count {
            issues.append(EvIssue(.invalidRoute))
        }
        switch EvEnergyModel.usableEnergyWh(input.battery) {
        case .unknown(let reason):
            issues.append(EvIssue(reason == "capacity-missing" ? .batteryMissing : .invalidBattery))
        case .known:
            if let initial = input.initialSocBp, !(0...10_000).contains(initial) { issues.append(EvIssue(.invalidBattery)) }
        }
        if case .unknown(let reason) = EvEnergyModel.segmentEnergyWh(distanceMetres: 0, consumption: input.consumption) {
            issues.append(EvIssue(reason == "consumption-missing" ? .consumptionMissing : .invalidConsumption))
        }
        if input.vehicleConnector == nil || input.vehicleConnector == .unknown {
            issues.append(EvIssue(.vehicleConnectorMissing))
        }
        if !(0...10_000).contains(input.chargeTargetSocBp) || (input.battery.reserveBp.map { input.chargeTargetSocBp <= $0 } ?? false) {
            issues.append(EvIssue(.invalidTargetSoc))
        }
        if !(0...maxStops).contains(input.maxStops) || !(1_000...100_000).contains(input.corridorRadiusMeters)
            || !(1...maxCandidates).contains(input.maxCandidates) || !(1...maxRouteCalls).contains(input.maxRouteCalls)
            || !(1...maxProbes).contains(input.maxCandidateProbes) || !(1...maxAlternativeChecks).contains(input.maxAlternativeChecks) {
            issues.append(EvIssue(.invalidLimits))
        }
        if input.failures.contains(where: { failure in
            failure.siteId.isEmpty
                || (failure.scope == .equipment && (failure.equipmentId ?? "").isEmpty)
                || (failure.scope == .connector && (failure.connectorId ?? "").isEmpty)
        }) {
            issues.append(EvIssue(.invalidFailure))
        }
        return issues
    }

    public static func eligibility(_ candidate: EvChargingCandidate, input: EvTripInput) -> [EvIssueCode] {
        var issues: [EvIssueCode] = []
        if candidate.connector != input.vehicleConnector { issues.append(.incompatibleConnector) }
        switch candidate.access {
        case .privateAccess: issues.append(.accessPrivate)
        case .unknown: issues.append(.accessUnknown)
        case .publicAccess: break
        }
        switch candidate.hours {
        case .closed: issues.append(.hoursClosed)
        case .unknown: issues.append(.hoursUnknown)
        case .open: break
        }
        if candidate.powerW == nil { issues.append(.powerUnknown) }
        if candidate.tariffMilliBrlPerKwh == nil { issues.append(.tariffUnknown) }
        if candidate.observedAt == nil { issues.append(.reportDateUnknown) }
        return issues
    }

    public static func excluded(_ candidate: EvChargingCandidate, failures: [EvFailure], alternative: Bool) -> Bool {
        if alternative && failures.contains(where: { $0.siteId == candidate.siteId }) { return true }
        return failures.contains { failure in
            switch failure.scope {
            case .site: return failure.siteId == candidate.siteId
            case .equipment: return failure.equipmentId == candidate.equipmentId
            case .connector: return failure.connectorId == candidate.connectorId
            }
        }
    }

    /// Deterministic one-per-site choice: highest power, then DC, then id.
    public static func bestPerSite(_ candidates: [EvChargingCandidate]) -> [EvChargingCandidate] {
        var bestBySite: [String: EvChargingCandidate] = [:]
        var order: [String] = []
        for candidate in candidates {
            guard let current = bestBySite[candidate.siteId] else {
                bestBySite[candidate.siteId] = candidate
                order.append(candidate.siteId)
                continue
            }
            if preferred(candidate, over: current) { bestBySite[candidate.siteId] = candidate }
        }
        return order.compactMap { bestBySite[$0] }
    }

    private static func preferred(_ lhs: EvChargingCandidate, over rhs: EvChargingCandidate) -> Bool {
        let lp = lhs.powerW ?? -1
        let rp = rhs.powerW ?? -1
        if lp != rp { return lp > rp }
        let ldc = lhs.current == .dc
        let rdc = rhs.current == .dc
        if ldc != rdc { return ldc }
        return lhs.candidateId < rhs.candidateId
    }

    public static func energyToTargetWh(battery: EvBatteryInput, currentSocBp: Int64, targetSocBp: Int64) -> EnergyEstimate {
        guard let capacity = battery.capacityWh else { return .unknown("capacity-missing") }
        guard (1...EvEnergyModel.maxEnergyWh).contains(capacity), (0...10_000).contains(currentSocBp),
              (0...10_000).contains(targetSocBp) else { return .unknown("input-out-of-range") }
        if let portion = battery.usablePermille, !(1...1_000).contains(portion) { return .unknown("input-out-of-range") }
        if targetSocBp <= currentSocBp { return .known(0) }
        let portion = UInt64(battery.usablePermille ?? 1_000)
        guard let scaled = EvExact.mul(UInt64(capacity), UInt64(targetSocBp - currentSocBp)),
              let value = EvExact.mulDivCeil(scaled, portion, 10_000_000),
              value <= UInt64(EvEnergyModel.maxEnergyWh) else {
            return .unknown("out-of-bounds")
        }
        return .known(Int64(value))
    }

    public static func chargeCostMilliBrl(energyWh: Int64, tariffMilliBrlPerKwh: Int64?) -> EnergyEstimate {
        if energyWh < 0 { return .unknown("negative-input") }
        guard let tariff = tariffMilliBrlPerKwh else { return .unknown("tariff-unknown") }
        guard tariff > 0, let value = EvExact.mulDivCeil(UInt64(energyWh), UInt64(tariff), 1_000),
              value <= UInt64(Int64.max) else {
            return .unknown("out-of-bounds")
        }
        return .known(Int64(value))
    }

    /// State at the failed primary with no energy from it.
    public static func failureReachability(battery: EvBatteryInput, arrivalSocBpAtPrimary: Int64, consumption: EvConsumptionInput, alternativeDistanceMeters: Int64) -> EnergyEstimate {
        guard let capacity = battery.capacityWh else { return .unknown("capacity-missing") }
        let effective = battery.usablePermille.map { max(1, capacity * $0 / 1_000) } ?? capacity
        return EvEnergyModel.arrivalSocBp(capacityWh: effective, socBp: arrivalSocBpAtPrimary, distanceMetres: alternativeDistanceMeters, consumption: consumption)
    }

    public static func isReachable(arrivalSocBp: Int64?, reserveBp: Int64?) -> Bool {
        guard let arrivalSocBp, let reserveBp else { return false }
        return arrivalSocBp >= reserveBp
    }

    static func validPoint(_ point: EvRoutePoint) -> Bool {
        point.latitude.isFinite && point.longitude.isFinite
            && (-90.0...90.0).contains(point.latitude) && (-180.0...180.0).contains(point.longitude)
            && point.label.count <= 512
    }
}

/// Snapshot -> candidates for the vehicle connector. Malformed metadata is skipped.
public enum EvCandidateMapper {
    public static func candidates(
        places: [ChargingPlace],
        connector: ChargingConnector,
        sourceRevision: String,
        route: [EvRoutePoint],
        corridorRadiusMeters: Int,
        limit: Int
    ) -> [EvChargingCandidate] {
        guard let box = EvRouteGeometry.boundingBox(route, paddingMeters: corridorRadiusMeters) else { return [] }
        var result: [EvChargingCandidate] = []
        for place in places {
            guard result.count < limit, let point = place.coordinate, box.contains(point) else { continue }
            let access: EvAccess = place.publicAccess == .publicAccess ? .publicAccess : .unknown
            for group in place.groups where group.connector == connector {
                result.append(EvChargingCandidate(
                    candidateId: group.id,
                    siteId: place.id,
                    siteName: place.displayName,
                    equipmentId: group.entityKey.isEmpty ? group.id : group.entityKey,
                    connectorId: group.id,
                    connector: group.connector,
                    current: group.current,
                    powerW: group.powerW,
                    access: access,
                    sourceRevision: sourceRevision.isEmpty ? "unknown" : sourceRevision,
                    point: point
                ))
            }
        }
        return Array(result.prefix(limit))
    }
}

/// MAP-27 orchestration with one bounded budget of router calls and a deadline.
public struct EvTripPlanner: Sendable {
    private let router: EvRoadRouter
    private let deadlineSeconds: TimeInterval
    private let now: @Sendable () -> Date

    public init(router: EvRoadRouter, deadlineSeconds: TimeInterval = 30, now: @escaping @Sendable () -> Date = { Date() }) {
        self.router = router
        self.deadlineSeconds = deadlineSeconds
        self.now = now
    }

    private final class Budget: @unchecked Sendable {
        let maxCalls: Int
        let deadline: Date
        let now: @Sendable () -> Date
        var calls = 0

        init(maxCalls: Int, deadline: Date, now: @escaping @Sendable () -> Date) {
            self.maxCalls = maxCalls
            self.deadline = deadline
            self.now = now
        }

        func consume() -> Bool {
            if calls >= maxCalls || now() >= deadline { return false }
            calls += 1
            return true
        }

        var exhausted: Bool { calls >= maxCalls || now() >= deadline }
    }

    /// `candidates` receives the main route geometry and returns admitted chargers near it.
    public func plan(_ input: EvTripInput, candidates: @Sendable ([EvRoutePoint]) -> EvCandidateResult) async -> EvTripPlan {
        var issues = EvChargingPlanRules.validate(input)
        let reserve = input.battery.reserveBp
        let start = input.initialPoint ?? input.origin
        var battery = input.battery
        battery.socBp = input.initialSocBp ?? input.battery.socBp
        if !issues.isEmpty {
            return emptyPlan(.invalidInput, battery: battery, coverage: .unknown, issues: issues, calls: 0, main: nil, revision: nil)
        }
        let budget = Budget(maxCalls: input.maxRouteCalls, deadline: now().addingTimeInterval(deadlineSeconds), now: now)
        let ordered = distinct([start, input.destination])
        let mainRoad = await call(ordered, budget)
        let mainLeg = plannedLeg(start, input.destination, mainRoad, battery, input.consumption)
        if case .unavailable(let reason) = mainRoad {
            issues.append(EvIssue(failureCode(reason)))
            return emptyPlan(planStatus(for: reason), battery: battery, coverage: .unknown, issues: issues, calls: budget.calls, main: mainLeg, revision: nil)
        }
        if mainLeg.arrivalSocBp.value == nil { issues.append(EvIssue(.energyUnknown)) }
        if mainRoad.hasWarnings { issues.append(EvIssue(.roadWarning)) }
        let geometry = mainRoad.geometry.count >= 2 ? mainRoad.geometry : ordered
        let found = candidates(geometry)
        if found.coverage != .present {
            issues.append(EvIssue(found.coverage == .missing ? .snapshotMissing : .coverageUnknown))
        }
        if let arrival = mainLeg.arrivalSocBp.value, let reserve, arrival >= reserve {
            return EvTripPlan(
                status: .planned, scope: EvEnergyModel.scope(of: battery), mainRoute: mainLeg, stops: [], finalLeg: nil,
                arrivalSocBp: mainLeg.arrivalSocBp, coverage: found.coverage, candidateCount: 0, routeCalls: budget.calls,
                issues: unique(issues), sourceRevision: found.sourceRevision
            )
        }
        let pool = found.candidates.filter { candidate in
            if candidate.connector != input.vehicleConnector { issues.append(EvIssue(.incompatibleConnector, candidateId: candidate.candidateId)) }
            if candidate.access == .privateAccess { issues.append(EvIssue(.accessPrivate, candidateId: candidate.candidateId)) }
            let failed = EvChargingPlanRules.excluded(candidate, failures: input.failures, alternative: false)
            if failed { issues.append(EvIssue(.duplicateCandidate, candidateId: candidate.candidateId)) }
            return candidate.connector == input.vehicleConnector && candidate.access != .privateAccess && !failed
        }
        let projected: [(candidate: EvChargingCandidate, projection: EvRouteGeometry.Projection)] =
            EvChargingPlanRules.bestPerSite(pool).compactMap { candidate in
                guard let projection = EvRouteGeometry.project(candidate.point, on: geometry),
                      projection.offRouteMetres <= Int64(input.corridorRadiusMeters) else { return nil }
                return (candidate: candidate, projection: projection)
            }
        guard var currentSoc = battery.socBp else {
            issues.append(EvIssue(.batteryMissing))
            return emptyPlan(.invalidInput, battery: battery, coverage: found.coverage, issues: issues, calls: budget.calls, main: mainLeg, revision: found.sourceRevision)
        }
        var currentPoint = start
        var currentAlong = EvRouteGeometry.project(EvCoordinate(latitude: start.latitude, longitude: start.longitude), on: geometry)?.alongMetres ?? 0
        var stops: [EvPlannedStop] = []
        var finalLeg: EvPlannedLeg?
        var status = EvPlanStatus.planned
        var selectedSites = Set<String>()

        planning: while true {
            let destinationRoad: EvRoadLeg
            if stops.isEmpty {
                destinationRoad = mainRoad
            } else {
                destinationRoad = await call(distinct([currentPoint, input.destination]), budget)
            }
            if case .unavailable(let reason) = destinationRoad {
                issues.append(EvIssue(failureCode(reason)))
                status = planStatus(for: reason)
                break planning
            }
            if destinationRoad.hasWarnings { issues.append(EvIssue(.roadWarning)) }
            var legBattery = battery
            legBattery.socBp = currentSoc
            let destinationLeg = plannedLeg(currentPoint, input.destination, destinationRoad, legBattery, input.consumption)
            if let arrival = destinationLeg.arrivalSocBp.value, let reserve, arrival >= reserve {
                finalLeg = destinationLeg
                break planning
            }
            if stops.count >= input.maxStops {
                issues.append(EvIssue(.stopLimitReached))
                status = .insufficientEnergy
                break planning
            }
            let allowBehind = stops.isEmpty && (input.initialPoint != nil || !input.failures.isEmpty)
            let ahead = projected
                .filter { allowBehind || $0.projection.alongMetres > currentAlong + 1 }
                .filter { !selectedSites.contains($0.candidate.siteId) }
                .filter { !EvChargingPlanRules.excluded($0.candidate, failures: input.failures, alternative: false) }
                .sorted { lhs, rhs in
                    if lhs.projection.alongMetres != rhs.projection.alongMetres {
                        return lhs.projection.alongMetres > rhs.projection.alongMetres
                    }
                    return lhs.candidate.siteId < rhs.candidate.siteId
                }
            if ahead.isEmpty {
                issues.append(EvIssue(.noCompatibleCandidate))
                status = found.coverage == .present ? .noCandidates : .insufficientData
                break planning
            }
            var chosen: (candidate: EvChargingCandidate, leg: EvPlannedLeg, along: Int64, soc: Int64, issues: [EvIssueCode], alternative: EvAlternative?)?
            var fallback: (candidate: EvChargingCandidate, leg: EvPlannedLeg, along: Int64, soc: Int64, issues: [EvIssueCode])?
            var probes = 0
            for entry in ahead {
                let candidate = entry.candidate
                if probes >= input.maxCandidateProbes {
                    issues.append(EvIssue(.incomplete, candidateId: candidate.candidateId))
                    if budget.exhausted { status = .budgetExhausted }
                    break
                }
                probes += 1
                let road = await call(distinct([currentPoint, candidate.routePoint]), budget)
                if case .unavailable(let reason) = road {
                    issues.append(EvIssue(failureCode(reason), candidateId: candidate.candidateId))
                    if reason == .budgetExhausted {
                        status = .budgetExhausted
                        break
                    }
                    continue
                }
                let planned = plannedLeg(currentPoint, candidate.routePoint, road, legBattery, input.consumption)
                if road.hasWarnings { issues.append(EvIssue(.roadWarning, candidateId: candidate.candidateId)) }
                let eligibility = EvChargingPlanRules.eligibility(candidate, input: input)
                let blocking = eligibility.filter { EvChargingPlanRules.blockingCodes.contains($0) }
                if !blocking.isEmpty {
                    issues.append(contentsOf: blocking.map { EvIssue($0, candidateId: candidate.candidateId) })
                    continue
                }
                guard let arrivalSoc = planned.arrivalSocBp.value else {
                    issues.append(EvIssue(.energyUnknown, candidateId: candidate.candidateId))
                    continue
                }
                if !EvChargingPlanRules.isReachable(arrivalSocBp: arrivalSoc, reserveBp: input.battery.reserveBp) {
                    issues.append(EvIssue(.stopUnreachable, candidateId: candidate.candidateId))
                    continue
                }
                if arrivalSoc >= input.chargeTargetSocBp {
                    issues.append(EvIssue(.chargeTargetAlreadyMet, candidateId: candidate.candidateId))
                    continue
                }
                let alternative = await evaluateAlternative(
                    primary: candidate, primaryArrivalSoc: arrivalSoc, pool: pool, selectedSites: selectedSites,
                    input: input, battery: battery, budget: budget, issues: &issues
                )
                if budget.exhausted {
                    issues.append(EvIssue(.routeBudgetExhausted, candidateId: candidate.candidateId))
                    status = .budgetExhausted
                    break
                }
                if let alternative {
                    chosen = (candidate, planned, entry.projection.alongMetres, arrivalSoc, eligibility, alternative)
                    break
                }
                if fallback == nil {
                    fallback = (candidate, planned, entry.projection.alongMetres, arrivalSoc, eligibility)
                }
            }
            if status == .budgetExhausted { break planning }
            if chosen == nil, let fallback {
                chosen = (fallback.candidate, fallback.leg, fallback.along, fallback.soc, fallback.issues, nil)
            }
            guard let stop = chosen else {
                issues.append(EvIssue(.destinationUnreachable))
                status = found.coverage == .present ? .insufficientEnergy : .insufficientData
                break planning
            }
            guard let chargeEnergy = EvChargingPlanRules.energyToTargetWh(
                battery: battery, currentSocBp: stop.soc, targetSocBp: input.chargeTargetSocBp
            ).value else {
                issues.append(EvIssue(.energyUnknown, candidateId: stop.candidate.candidateId))
                status = .insufficientData
                break planning
            }
            if stop.alternative == nil {
                issues.append(EvIssue(.noEvidencedAlternative, candidateId: stop.candidate.candidateId))
            }
            stops.append(EvPlannedStop(
                candidate: stop.candidate,
                arrival: stop.leg,
                arrivalSocBp: stop.soc,
                targetSocBp: input.chargeTargetSocBp,
                chargeEnergyWh: chargeEnergy,
                minimumChargeSeconds: EvEnergyModel.minimumChargeSeconds(energyWh: chargeEnergy, declaredPowerW: stop.candidate.powerW),
                estimatedCostMilliBrl: EvChargingPlanRules.chargeCostMilliBrl(energyWh: chargeEnergy, tariffMilliBrlPerKwh: stop.candidate.tariffMilliBrlPerKwh),
                issues: stop.issues,
                alternative: stop.alternative
            ))
            selectedSites.insert(stop.candidate.siteId)
            currentPoint = stop.candidate.routePoint
            currentSoc = input.chargeTargetSocBp
            currentAlong = stop.along
        }

        return EvTripPlan(
            status: status, scope: EvEnergyModel.scope(of: battery), mainRoute: mainLeg, stops: stops, finalLeg: finalLeg,
            arrivalSocBp: finalLeg?.arrivalSocBp, coverage: found.coverage, candidateCount: projected.count,
            routeCalls: budget.calls, issues: unique(issues), sourceRevision: found.sourceRevision
        )
    }

    private func evaluateAlternative(
        primary: EvChargingCandidate,
        primaryArrivalSoc: Int64,
        pool: [EvChargingCandidate],
        selectedSites: Set<String>,
        input: EvTripInput,
        battery: EvBatteryInput,
        budget: Budget,
        issues: inout [EvIssue]
    ) async -> EvAlternative? {
        let sameSite = pool.contains { $0.siteId == primary.siteId && $0.candidateId != primary.candidateId }
        let others = EvChargingPlanRules.bestPerSite(pool)
            .filter { $0.siteId != primary.siteId && !selectedSites.contains($0.siteId) }
            .filter { !EvChargingPlanRules.excluded($0, failures: input.failures, alternative: true) }
            .sorted { lhs, rhs in
                let ld = EvRouteGeometry.distanceMeters(primary.point.latitude, primary.point.longitude, lhs.point.latitude, lhs.point.longitude)
                let rd = EvRouteGeometry.distanceMeters(primary.point.latitude, primary.point.longitude, rhs.point.latitude, rhs.point.longitude)
                if ld != rd { return ld < rd }
                return lhs.candidateId < rhs.candidateId
            }
        var checks = 0
        for candidate in others {
            if checks >= input.maxAlternativeChecks {
                issues.append(EvIssue(.noEvidencedAlternative, candidateId: primary.candidateId))
                break
            }
            checks += 1
            let road = await call(distinct([primary.routePoint, candidate.routePoint]), budget)
            if case .unavailable(let reason) = road {
                issues.append(EvIssue(.alternativeUnknownRoad, candidateId: candidate.candidateId))
                if reason == .budgetExhausted { return nil }
                continue
            }
            if road.hasWarnings { issues.append(EvIssue(.roadWarning, candidateId: candidate.candidateId)) }
            var atPrimary = battery
            atPrimary.socBp = primaryArrivalSoc
            let planned = plannedLeg(primary.routePoint, candidate.routePoint, road, atPrimary, input.consumption)
            let eligibility = EvChargingPlanRules.eligibility(candidate, input: input)
            let blocking = eligibility.filter { EvChargingPlanRules.blockingCodes.contains($0) }
            if !blocking.isEmpty {
                issues.append(contentsOf: blocking.map { EvIssue($0, candidateId: candidate.candidateId) })
                continue
            }
            guard let distance = road.distanceMeters else { continue }
            let arrival = EvChargingPlanRules.failureReachability(
                battery: battery, arrivalSocBpAtPrimary: primaryArrivalSoc, consumption: input.consumption,
                alternativeDistanceMeters: distance
            )
            if EvChargingPlanRules.isReachable(arrivalSocBp: arrival.value, reserveBp: input.battery.reserveBp) {
                var alternativeIssues = eligibility
                if road.hasWarnings && !alternativeIssues.contains(.roadWarning) { alternativeIssues.append(.roadWarning) }
                return EvAlternative(candidate: candidate, leg: planned, arrivalSocBp: arrival, issues: alternativeIssues)
            }
            issues.append(EvIssue(.alternativeUnreachable, candidateId: candidate.candidateId))
        }
        if sameSite { issues.append(EvIssue(.alternativeSameSite, candidateId: primary.candidateId)) }
        return nil
    }

    private func call(_ points: [EvRoutePoint], _ budget: Budget) async -> EvRoadLeg {
        if Task.isCancelled { return .unavailable(.cancelled) }
        guard budget.consume() else { return .unavailable(.budgetExhausted) }
        return await router.leg(points: points)
    }

    private func distinct(_ points: [EvRoutePoint]) -> [EvRoutePoint] {
        var seen = Set<String>()
        return points.filter { seen.insert($0.key).inserted }
    }

    private func plannedLeg(_ from: EvRoutePoint, _ to: EvRoutePoint, _ road: EvRoadLeg, _ battery: EvBatteryInput, _ consumption: EvConsumptionInput) -> EvPlannedLeg {
        guard let distance = road.distanceMeters else {
            return EvPlannedLeg(from: from, to: to, road: road, energyWh: .unknown("road-unavailable"), arrivalSocBp: .unknown("road-unavailable"))
        }
        let energy = EvEnergyModel.segmentEnergyWh(distanceMetres: distance, consumption: consumption)
        let arrival: EnergyEstimate
        if let capacity = battery.capacityWh, let soc = battery.socBp {
            let effective = battery.usablePermille.map { max(1, capacity * $0 / 1_000) } ?? capacity
            arrival = EvEnergyModel.arrivalSocBp(capacityWh: effective, socBp: soc, distanceMetres: distance, consumption: consumption)
        } else {
            arrival = .unknown(battery.capacityWh == nil ? "capacity-missing" : "soc-missing")
        }
        return EvPlannedLeg(from: from, to: to, road: road, energyWh: energy, arrivalSocBp: arrival)
    }

    private func emptyPlan(
        _ status: EvPlanStatus, battery: EvBatteryInput, coverage: EvCoverage, issues: [EvIssue], calls: Int,
        main: EvPlannedLeg?, revision: String?
    ) -> EvTripPlan {
        EvTripPlan(
            status: status, scope: EvEnergyModel.scope(of: battery), mainRoute: main, stops: [], finalLeg: nil,
            arrivalSocBp: nil, coverage: coverage, candidateCount: 0, routeCalls: calls, issues: unique(issues),
            sourceRevision: revision
        )
    }

    private func unique(_ issues: [EvIssue]) -> [EvIssue] {
        var seen = Set<EvIssue>()
        return issues.filter { seen.insert($0).inserted }
    }

    private func planStatus(for reason: EvRoadUnavailable) -> EvPlanStatus {
        switch reason {
        case .cancelled: return .cancelled
        case .budgetExhausted: return .budgetExhausted
        default: return .noRoute
        }
    }

    private func failureCode(_ reason: EvRoadUnavailable) -> EvIssueCode {
        switch reason {
        case .cancelled: return .routeCancelled
        case .timeout: return .routeTimeout
        case .missingMaps: return .missingMaps
        case .budgetExhausted: return .routeBudgetExhausted
        case .notReady: return .routeNotReady
        case .noRoute, .unknown: return .noRoadRoute
        }
    }
}

/// Android `EvTripViewModel` input parsing: percents with at most two decimals as basis
/// points, consumption in Wh/km with at most three decimals as milli-Wh per km.
public enum EvPlannerInputParser {
    public static func percentBp(_ raw: String, min: Int64, max: Int64) -> Int64? {
        guard let scaled = scaledDecimal(raw, decimals: 2) else { return nil }
        return (min * 100...max * 100).contains(scaled) ? scaled : nil
    }

    public static func consumptionMilli(_ raw: String) -> Int64? {
        guard let scaled = scaledDecimal(raw, decimals: 3) else { return nil }
        return (1...EvEnergyModel.maxConsumptionMilli).contains(scaled) ? scaled : nil
    }

    /// Exact decimal text (comma or period) times 10^decimals; a longer scale is refused
    /// like Android `BigDecimal.scale()`, never rounded.
    static func scaledDecimal(_ raw: String, decimals: Int) -> Int64? {
        let text = raw.trimmingCharacters(in: .whitespacesAndNewlines).replacingOccurrences(of: ",", with: ".")
        var body = Substring(text)
        var negative = false
        if body.hasPrefix("-") {
            negative = true
            body = body.dropFirst()
        } else if body.hasPrefix("+") {
            body = body.dropFirst()
        }
        let parts = body.split(separator: ".", omittingEmptySubsequences: false)
        guard parts.count <= 2, !parts[0].isEmpty || parts.count == 2 else { return nil }
        let whole = parts[0]
        let fraction = parts.count == 2 ? parts[1] : Substring("")
        guard whole.isEmpty || AnpCalendar.isAsciiDigits(whole), fraction.isEmpty || AnpCalendar.isAsciiDigits(fraction),
              !(whole.isEmpty && fraction.isEmpty) else { return nil }
        guard fraction.count <= decimals, whole.count <= 15 else { return nil }
        let padded = String(fraction) + String(repeating: "0", count: decimals - fraction.count)
        guard let value = Int64((whole.isEmpty ? "0" : String(whole)) + padded) else { return nil }
        return negative ? -value : value
    }
}

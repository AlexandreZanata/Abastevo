import Foundation

/// The coordinates of the provider request. Display labels do not affect route identity.
public struct NavigationGuidanceInput: Equatable, Sendable {
    public let origin: EvRoutePoint
    public let destination: EvRoutePoint

    public init(origin: EvRoutePoint, destination: EvRoutePoint) {
        self.origin = origin
        self.destination = destination
    }

    public static func == (lhs: Self, rhs: Self) -> Bool {
        NavigationGuidancePolicy.samePoint(lhs.origin, rhs.origin)
            && NavigationGuidancePolicy.samePoint(lhs.destination, rhs.destination)
    }
}

public struct NavigationGuidanceStep: Equatable, Sendable {
    public let instruction: String
    public let distanceMeters: Double

    public init(instruction: String, distanceMeters: Double) {
        self.instruction = instruction
        self.distanceMeters = distanceMeters
    }
}

public struct NavigationGuidanceManeuver: Equatable, Identifiable, Sendable {
    public let index: Int
    public let instruction: String
    public var id: Int { index }
}

/// A provider route. Travel distance and maneuvers come from MapKit, with the polyline used for progress.
public struct NavigationGuidanceRoute: Equatable, Sendable {
    public let input: NavigationGuidanceInput
    public let geometry: [EvRoutePoint]
    public let distanceMeters: Double
    public let expectedTravelTime: TimeInterval
    public let steps: [NavigationGuidanceStep]
    fileprivate let cumulativeMeters: [Double]
    fileprivate let instructions: [InstructionPosition]

    fileprivate struct InstructionPosition: Equatable, Sendable {
        let maneuver: NavigationGuidanceManeuver
        let meters: Double
    }

    public init?(
        input: NavigationGuidanceInput, geometry: [EvRoutePoint], distanceMeters: Double,
        expectedTravelTime: TimeInterval, steps: [NavigationGuidanceStep]
    ) {
        guard NavigationGuidancePolicy.valid(input.origin), NavigationGuidancePolicy.valid(input.destination),
              !NavigationGuidancePolicy.samePoint(input.origin, input.destination),
              geometry.count >= 2, geometry.allSatisfy(NavigationGuidancePolicy.valid),
              distanceMeters.isFinite, distanceMeters > 0,
              expectedTravelTime.isFinite, expectedTravelTime > 0,
              steps.allSatisfy({ $0.distanceMeters.isFinite && $0.distanceMeters >= 0 }) else { return nil }
        var cumulative = [0.0]
        for (from, to) in zip(geometry, geometry.dropFirst()) {
            let travelled = NavigationGuidancePolicy.distance(from, to)
            guard let previous = cumulative.last, travelled.isFinite else { return nil }
            cumulative.append(previous + travelled)
        }
        guard let length = cumulative.last, length.isFinite, length >= 1 else { return nil }
        let stepTotal = steps.reduce(0.0) { $0 + $1.distanceMeters }
        guard stepTotal.isFinite else { return nil }
        var position = 0.0
        var instructions: [InstructionPosition] = []
        for (index, step) in steps.enumerated() {
            if !step.instruction.trimmingCharacters(in: .whitespacesAndNewlines).isEmpty {
                let offset = stepTotal > 0 ? position / stepTotal * distanceMeters : 0
                instructions.append(InstructionPosition(
                    maneuver: NavigationGuidanceManeuver(index: index, instruction: step.instruction), meters: offset
                ))
            }
            position += step.distanceMeters
        }
        self.input = input
        self.geometry = geometry
        self.distanceMeters = distanceMeters
        self.expectedTravelTime = expectedTravelTime
        self.steps = steps
        self.cumulativeMeters = cumulative
        self.instructions = instructions
    }

    fileprivate var geometryLength: Double { cumulativeMeters.last ?? 0 }

    fileprivate func projection(
        of point: EvRoutePoint, near progress: Double?, forwardAllowance: Double, backwardAllowance: Double
    ) -> (along: Double, off: Double)? {
        let coordinate = EvCoordinate(latitude: point.latitude, longitude: point.longitude)
        guard let progress else {
            return EvRouteGeometry.project(coordinate, on: geometry).map { (Double($0.alongMetres), Double($0.offRouteMetres)) }
        }
        let lower = max(0, progress - backwardAllowance)
        let upper = min(geometryLength, progress + forwardAllowance)
        let first = max(0, lowerBound(lower) - 1)
        let last = max(first + 1, lowerBound(upper))
        let localGeometry = Array(geometry[first...last])
        guard let projected = EvRouteGeometry.project(coordinate, on: localGeometry) else { return nil }
        let along = cumulativeMeters[first] + Double(projected.alongMetres)
        guard along >= lower - 1, along <= upper + 1 else { return (along, .infinity) }
        return (along, Double(projected.offRouteMetres))
    }

    private func lowerBound(_ distance: Double) -> Int {
        var lower = 0
        var upper = cumulativeMeters.count - 1
        while lower < upper {
            let midpoint = lower + (upper - lower) / 2
            if cumulativeMeters[midpoint] < distance { lower = midpoint + 1 } else { upper = midpoint }
        }
        return lower
    }
}

/// One admitted foreground or active-navigation fix. The sampled stream stays in memory only.
public struct NavigationGuidanceFix: Equatable, Sendable {
    public let point: EvRoutePoint
    public let accuracyMeters: Double
    public let timestamp: Date
    public let sampledAtNanos: UInt64

    /// CLLocation supplies a wall timestamp. Convert age at ingestion, then use uptime for all deadlines.
    public init?(
        point: EvRoutePoint, accuracyMeters: Double, timestamp: Date, receivedAt: Date, receivedUptimeNanos: UInt64
    ) {
        guard NavigationGuidancePolicy.valid(point), accuracyMeters.isFinite,
              accuracyMeters > 0, accuracyMeters <= NavigationGuidancePolicy.maxAccuracyMeters,
              timestamp.timeIntervalSince1970 > 0 else { return nil }
        let age = receivedAt.timeIntervalSince(timestamp)
        guard age.isFinite, age >= 0,
              age <= Double(NavigationGuidancePolicy.maxFixAgeNanos) / 1_000_000_000 else { return nil }
        let ageNanos = UInt64((age * 1_000_000_000).rounded(.down))
        guard receivedUptimeNanos > ageNanos else { return nil }
        self.point = point
        self.accuracyMeters = accuracyMeters
        self.timestamp = timestamp
        self.sampledAtNanos = receivedUptimeNanos - ageNanos
    }
}

public enum NavigationGuidancePolicy {
    public static let maxAccuracyMeters = 50.0
    public static let maxFixAgeNanos: UInt64 = 15_000_000_000
    public static let rerouteTimeoutNanos: UInt64 = 60_000_000_000
    /// Automatic reroutes back off: 15 s after the first, doubling up to 2 minutes while they keep
    /// coming within the 3-minute window. A person's explicit retry is never delayed.
    public static let rerouteCooldownBaseNanos: UInt64 = 15_000_000_000
    public static let rerouteCooldownMaxNanos: UInt64 = 120_000_000_000
    public static let rerouteWindowNanos: UInt64 = 180_000_000_000

    public static func rerouteCooldownNanos(afterConsecutive count: Int) -> UInt64 {
        guard count > 0 else { return 0 }
        let doubled = rerouteCooldownBaseNanos << UInt64(min(count - 1, 4))
        return min(doubled, rerouteCooldownMaxNanos)
    }

    public static func accepts(_ fix: NavigationGuidanceFix, nowNanos: UInt64) -> Bool {
        nowNanos >= fix.sampledAtNanos && nowNanos - fix.sampledAtNanos <= maxFixAgeNanos
    }

    public static func isRerouteExpired(startedAtNanos: UInt64, nowNanos: UInt64) -> Bool {
        nowNanos >= startedAtNanos && nowNanos - startedAtNanos > rerouteTimeoutNanos
    }

    public static func nativeLocaleFor(_ language: String) -> String? {
        let parts = language.trimmingCharacters(in: .whitespacesAndNewlines)
            .replacingOccurrences(of: "_", with: "-").lowercased().split(separator: "-")
        guard let base = parts.first else { return nil }
        switch base {
        case "pt": return "pt-BR"
        case "zh":
            if parts.dropFirst().contains(where: { ["tw", "hk", "mo", "hant"].contains($0) }) { return nil }
            return "zh-Hans"
        case "en", "de", "es", "fr", "ja", "ru": return String(base)
        default: return nil
        }
    }

    fileprivate static func valid(_ point: EvRoutePoint) -> Bool {
        point.latitude.isFinite && point.longitude.isFinite
            && (-90...90).contains(point.latitude) && (-180...180).contains(point.longitude)
    }

    fileprivate static func samePoint(_ lhs: EvRoutePoint, _ rhs: EvRoutePoint) -> Bool {
        lhs.latitude == rhs.latitude && lhs.longitude == rhs.longitude
    }

    fileprivate static func distance(_ lhs: EvRoutePoint, _ rhs: EvRoutePoint) -> Double {
        EvRouteGeometry.distanceMeters(lhs.latitude, lhs.longitude, rhs.latitude, rhs.longitude)
    }
}

public enum NavigationGuidanceState: Equatable, Sendable {
    case waitingForFix
    case following
    case weakSignal
    case offRoute
    case rerouting
    case rerouteFailed
    case arrived
}

public struct NavigationGuidanceProgress: Equatable, Sendable {
    public let remainingMeters: Double
    public let remainingSeconds: TimeInterval
    public let nextManeuver: NavigationGuidanceManeuver?
    public let distanceToManeuverMeters: Double?
}

public struct NavigationGuidanceReroute: Equatable, Sendable {
    public let id: UUID
    public let input: NavigationGuidanceInput
    public let startedAtNanos: UInt64
}

public struct NavigationGuidanceUpdate: Equatable, Sendable {
    public let state: NavigationGuidanceState
    public let progress: NavigationGuidanceProgress?
    public let speech: NavigationGuidanceManeuver?
    public let reroute: NavigationGuidanceReroute?
}

/// One explicitly started run. The platform host owns permission, audio, resource cleanup and owner fencing.
public struct NavigationGuidance: Sendable {
    public private(set) var route: NavigationGuidanceRoute
    public private(set) var state = NavigationGuidanceState.waitingForFix
    public private(set) var progress: NavigationGuidanceProgress?
    public private(set) var pendingReroute: NavigationGuidanceReroute?
    private var latestFix: NavigationGuidanceFix?
    private var latestProgressNanos: UInt64?
    private var alongMeters: Double?
    private var offRouteFixes = 0
    private var arrivalFixes = 0
    private var spokenManeuvers = Set<Int>()
    private var lastAutomaticRerouteNanos: UInt64?
    private var consecutiveAutomaticReroutes = 0

    public init?(route: NavigationGuidanceRoute, input: NavigationGuidanceInput) {
        guard route.input == input else { return nil }
        self.route = route
    }

    public mutating func ingest(_ fix: NavigationGuidanceFix?, nowNanos: UInt64) -> NavigationGuidanceUpdate {
        _ = advance(nowNanos: nowNanos)
        guard state != .arrived else { return update() }
        guard let fix, NavigationGuidancePolicy.accepts(fix, nowNanos: nowNanos) else { return loseFix() }
        if let previous = latestFix {
            guard fix.sampledAtNanos > previous.sampledAtNanos, fix.timestamp > previous.timestamp else { return update() }
            if fix.sampledAtNanos - previous.sampledAtNanos > NavigationGuidancePolicy.maxFixAgeNanos {
                offRouteFixes = 0
                arrivalFixes = 0
            }
        }
        latestFix = fix

        // Match near previous progress to avoid jumping ahead across loops or nearby parallel legs.
        let movementTime = latestProgressNanos.map { Double(fix.sampledAtNanos - $0) / 1_000_000_000 } ?? 0
        let tolerance = max(30, 2 * fix.accuracyMeters)
        guard let projection = route.projection(of: fix.point, near: alongMeters,
                                                forwardAllowance: max(75, 80 * movementTime + tolerance),
                                                backwardAllowance: tolerance) else { return loseFix() }
        if projection.off > tolerance {
            arrivalFixes = 0
            if pendingReroute != nil || state == .rerouteFailed { return update() }
            state = .offRoute
            offRouteFixes += 1
            guard offRouteFixes >= 3 else { return update() }
            return automaticRerouteAllowed(nowNanos: nowNanos) ? startAutomaticReroute(from: fix, nowNanos: nowNanos) : update()
        }

        pendingReroute = nil
        offRouteFixes = 0
        state = .following
        let along = max(alongMeters ?? 0, projection.along)
        alongMeters = along
        latestProgressNanos = fix.sampledAtNanos
        let fraction = min(1, along / route.geometryLength)
        let position = route.distanceMeters * fraction
        let remaining = route.distanceMeters * (1 - fraction)
        let next = route.instructions.first { $0.meters + 8 >= position }
        let turnDistance = next.map { max(0, $0.meters - position) }
        progress = NavigationGuidanceProgress(
            remainingMeters: remaining, remainingSeconds: route.expectedTravelTime * (1 - fraction),
            nextManeuver: next?.maneuver, distanceToManeuverMeters: turnDistance
        )

        // A GPS projection by itself cannot complete a route. Require a second accurate endpoint fix.
        if canArrive(fix: fix, remainingMeters: remaining) {
            arrivalFixes += 1
            if arrivalFixes >= 2 {
                state = .arrived
                progress = NavigationGuidanceProgress(remainingMeters: 0, remainingSeconds: 0,
                                                      nextManeuver: nil, distanceToManeuverMeters: nil)
                return update()
            }
        } else {
            arrivalFixes = 0
        }
        if let next, let turnDistance, turnDistance <= 250, spokenManeuvers.insert(next.maneuver.index).inserted {
            return update(speech: next.maneuver)
        }
        return update()
    }

    /// The host can expire a reroute or stale location even when the providers send no callbacks.
    public mutating func advance(nowNanos: UInt64) -> NavigationGuidanceUpdate {
        if let request = pendingReroute,
           NavigationGuidancePolicy.isRerouteExpired(startedAtNanos: request.startedAtNanos, nowNanos: nowNanos) {
            pendingReroute = nil
            state = .rerouteFailed
        } else if (state == .following || state == .offRoute), let latestFix,
                  !NavigationGuidancePolicy.accepts(latestFix, nowNanos: nowNanos) {
            return loseFix()
        }
        return update()
    }

    public mutating func retryReroute(nowNanos: UInt64) -> NavigationGuidanceUpdate {
        _ = advance(nowNanos: nowNanos)
        guard state == .rerouteFailed, let latestFix,
              NavigationGuidancePolicy.accepts(latestFix, nowNanos: nowNanos) else { return update() }
        return startReroute(from: latestFix, nowNanos: nowNanos)
    }

    public mutating func completeReroute(
        _ request: NavigationGuidanceReroute, route replacement: NavigationGuidanceRoute?, nowNanos: UInt64
    ) -> NavigationGuidanceUpdate {
        _ = advance(nowNanos: nowNanos)
        guard pendingReroute == request, nowNanos >= request.startedAtNanos else { return update() }
        pendingReroute = nil
        guard let replacement, replacement.input == request.input else {
            state = .rerouteFailed
            return update()
        }
        self.route = replacement
        let fix = latestFix
        latestFix = nil
        latestProgressNanos = nil
        alongMeters = nil
        progress = nil
        offRouteFixes = 0
        arrivalFixes = 0
        spokenManeuvers = []
        state = .waitingForFix
        guard let fix, NavigationGuidancePolicy.accepts(fix, nowNanos: nowNanos) else { return update() }
        return ingest(fix, nowNanos: nowNanos)
    }

    private func automaticRerouteAllowed(nowNanos: UInt64) -> Bool {
        guard let last = lastAutomaticRerouteNanos, nowNanos >= last else { return true }
        let elapsed = nowNanos - last
        if elapsed > NavigationGuidancePolicy.rerouteWindowNanos { return true }
        return elapsed >= NavigationGuidancePolicy.rerouteCooldownNanos(afterConsecutive: consecutiveAutomaticReroutes)
    }

    private mutating func startAutomaticReroute(from fix: NavigationGuidanceFix, nowNanos: UInt64) -> NavigationGuidanceUpdate {
        if let last = lastAutomaticRerouteNanos, nowNanos >= last,
           nowNanos - last <= NavigationGuidancePolicy.rerouteWindowNanos {
            consecutiveAutomaticReroutes = min(consecutiveAutomaticReroutes + 1, 16)
        } else {
            consecutiveAutomaticReroutes = 1
        }
        lastAutomaticRerouteNanos = nowNanos
        return startReroute(from: fix, nowNanos: nowNanos)
    }

    private mutating func startReroute(from fix: NavigationGuidanceFix, nowNanos: UInt64) -> NavigationGuidanceUpdate {
        let input = NavigationGuidanceInput(origin: fix.point, destination: route.input.destination)
        let request = NavigationGuidanceReroute(id: UUID(), input: input, startedAtNanos: nowNanos)
        pendingReroute = request
        state = .rerouting
        return update(reroute: request)
    }

    private mutating func loseFix() -> NavigationGuidanceUpdate {
        offRouteFixes = 0
        arrivalFixes = 0
        if state != .rerouting && state != .rerouteFailed { state = .weakSignal }
        return update()
    }

    private func canArrive(fix: NavigationGuidanceFix, remainingMeters: Double) -> Bool {
        guard let endpoint = route.geometry.last else { return false }
        return fix.accuracyMeters <= 25 && remainingMeters <= 20
            && NavigationGuidancePolicy.distance(fix.point, endpoint) <= 20
    }

    private func update(
        speech: NavigationGuidanceManeuver? = nil, reroute: NavigationGuidanceReroute? = nil
    ) -> NavigationGuidanceUpdate {
        NavigationGuidanceUpdate(state: state, progress: progress, speech: speech, reroute: reroute)
    }
}

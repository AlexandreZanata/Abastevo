import AnpFuelCore
import CoreLocation
import MapKit
import SwiftUI

public struct NavigationDestination: Identifiable {
    public let id = UUID()
    public let title: String
    public let latitude: Double
    public let longitude: Double
    let allowsWaze: Bool

    public init(title: String, latitude: Double, longitude: Double) {
        self.init(title: title, latitude: latitude, longitude: longitude, allowsWaze: false)
    }

    init(title: String, latitude: Double, longitude: Double, allowsWaze: Bool) {
        self.title = title
        self.latitude = latitude
        self.longitude = longitude
        self.allowsWaze = allowsWaze
    }

    var coordinate: CLLocationCoordinate2D { CLLocationCoordinate2D(latitude: latitude, longitude: longitude) }
    var isRoutable: Bool { !title.isEmpty && CLLocationCoordinate2DIsValid(coordinate) && (latitude != 0 || longitude != 0) }
    var point: EvRoutePoint { EvRoutePoint(latitude: latitude, longitude: longitude, label: title) }
}

@MainActor
final class NavigationCoordinator: ObservableObject {
    enum Phase: Equatable {
        case idle, planning, preview, originUnavailable, permissionRequested, permissionDenied, providerFailed
        case active(NavigationGuidanceState)
    }

    @Published private(set) var phase: Phase = .idle
    @Published private(set) var destination: NavigationDestination?
    @Published private(set) var mapRoute: MKRoute?
    @Published private(set) var routeRevision = UUID()
    @Published private(set) var progress: NavigationGuidanceProgress?
    @Published private(set) var currentPosition: CLLocationCoordinate2D?
    @Published private(set) var hasSession = false
    @Published private(set) var voiceFailed = false

    private let location: ActiveNavigationLocation
    private let voice = IosVoiceGuidance()
    private var guidance: NavigationGuidance?
    private var routed: NavigationGuidanceRoute?
    private var previewTask: Task<Void, Never>?
    private var rerouteTask: Task<Void, Never>?
    private var rerouteID: UUID?
    private var timer: Task<Void, Never>?
    private var ownerFence = UUID()

    init(location: ActiveNavigationLocation) {
        self.location = location
    }

    var voiceEnabled = true {
        didSet {
            if !voiceEnabled {
                voice.stop()
                voiceFailed = false
            }
        }
    }

    func prepare(destination: NavigationDestination) {
        if hasSession { return }
        guard self.destination?.id != destination.id else { return }
        stop()
        self.destination = destination
    }

    func plan(
        destination: NavigationDestination, from manual: EvRoutePoint? = nil,
        requestOrigin: @escaping () async -> CLLocationCoordinate2D?
    ) {
        guard !hasSession, destination.isRoutable else {
            if !hasSession { phase = .providerFailed }
            return
        }
        previewTask?.cancel()
        ownerFence = UUID()
        let token = ownerFence
        self.destination = destination
        phase = .planning
        routed = nil
        mapRoute = nil
        previewTask = Task { [weak self] in
            let origin: EvRoutePoint?
            if let manual {
                origin = manual
            } else if let fix = await requestOrigin() {
                origin = EvRoutePoint(latitude: fix.latitude, longitude: fix.longitude)
            } else {
                origin = nil
            }
            guard let self, ownerFence == token, !Task.isCancelled else { return }
            guard let origin else {
                phase = .originUnavailable
                return
            }
            let input = NavigationGuidanceInput(origin: origin, destination: destination.point)
            do {
                let (route, native) = try await Self.plan(input)
                guard ownerFence == token, !Task.isCancelled else { return }
                install(route, native: native)
                phase = .preview
            } catch {
                guard ownerFence == token, !Task.isCancelled else { return }
                phase = .providerFailed
            }
        }
    }

    func start() {
        guard !hasSession, let routed, let destination,
              routed.input.destination.latitude == destination.latitude,
              routed.input.destination.longitude == destination.longitude,
              let run = NavigationGuidance(route: routed, input: routed.input) else { return }
        guidance = run
        hasSession = true
        let token = ownerFence
        location.start { [weak self] event in
            guard let self, ownerFence == token, hasSession else { return }
            switch event {
            case .requestingPermission: phase = .permissionRequested
            case .active:
                phase = .active(.waitingForFix)
                startTimer(token)
            case .permissionDenied:
                end()
                phase = .permissionDenied
            case .serviceUnavailable:
                end()
                phase = .providerFailed
            case .weakSignal:
                phase = .active(.weakSignal)
            case .fix(let fix):
                let uptime = DispatchTime.now().uptimeNanoseconds
                let input = NavigationGuidanceFix(
                    point: EvRoutePoint(latitude: fix.coordinate.latitude, longitude: fix.coordinate.longitude),
                    accuracyMeters: fix.horizontalAccuracy, timestamp: fix.timestamp, receivedAt: Date(),
                    receivedUptimeNanos: uptime
                )
                if let input, NavigationGuidancePolicy.accepts(input, nowNanos: uptime) {
                    currentPosition = fix.coordinate
                }
                if let update = guidance?.ingest(input, nowNanos: uptime) { consume(update) }
            }
        }
    }

    func foregrounded() {
        location.foregrounded()
    }

    func retry() {
        let uptime = DispatchTime.now().uptimeNanoseconds
        if let update = guidance?.retryReroute(nowNanos: uptime) { consume(update) }
    }

    func end() {
        ownerFence = UUID()
        guidance = nil
        hasSession = false
        progress = nil
        currentPosition = nil
        voiceFailed = false
        previewTask?.cancel()
        previewTask = nil
        cancelReroute()
        timer?.cancel()
        timer = nil
        location.stop()
        voice.stop()
        phase = routed == nil ? .idle : .preview
    }

    func stop() {
        end()
        destination = nil
        routed = nil
        mapRoute = nil
        routeRevision = UUID()
        phase = .idle
    }

    private func startTimer(_ token: UUID) {
        timer?.cancel()
        timer = Task { [weak self] in
            while !Task.isCancelled {
                do { try await Task.sleep(for: .seconds(1)) } catch { return }
                guard let self, ownerFence == token, hasSession else { return }
                if let update = guidance?.advance(nowNanos: DispatchTime.now().uptimeNanoseconds) {
                    consume(update)
                }
            }
        }
    }

    private func consume(_ update: NavigationGuidanceUpdate) {
        phase = .active(update.state)
        progress = update.progress
        if update.state == .arrived {
            end()
            phase = .active(.arrived)
            return
        }

        if let command = update.reroute {
            reroute(command)
        } else if let rerouteID, guidance?.pendingReroute?.id != rerouteID {
            cancelReroute()
        }
        if let speech = update.speech, voiceEnabled {
            let appLanguage = Bundle.main.preferredLocalizations.first ?? "en"
            if let language = NavigationGuidancePolicy.nativeLocaleFor(appLanguage) {
                voiceFailed = !voice.speak(instruction: speech.instruction, localeCode: language)
            }
        }
    }

    private func reroute(_ command: NavigationGuidanceReroute) {
        cancelReroute()
        rerouteID = command.id
        let token = ownerFence
        rerouteTask = Task { [weak self] in
            let result = try? await Self.plan(command.input)
            guard let self, !Task.isCancelled, ownerFence == token, rerouteID == command.id,
                  guidance?.pendingReroute?.id == command.id else { return }
            rerouteTask = nil
            rerouteID = nil
            let update = guidance?.completeReroute(command, route: result?.0, nowNanos: DispatchTime.now().uptimeNanoseconds)
            if let result, guidance?.route == result.0 {
                install(result.0, native: result.1)
            }
            if let update { consume(update) }
        }
    }

    private func cancelReroute() {
        rerouteTask?.cancel()
        rerouteTask = nil
        rerouteID = nil
    }

    private func install(_ route: NavigationGuidanceRoute, native: MKRoute) {
        routed = route
        mapRoute = native
        routeRevision = UUID()
    }

    private enum ProviderError: Error { case noRoute }

    private static func plan(_ input: NavigationGuidanceInput) async throws -> (NavigationGuidanceRoute, MKRoute) {
        let request = MKDirections.Request()
        request.source = mapItem(input.origin)
        request.destination = mapItem(input.destination)
        request.transportType = .automobile
        let directions = MKDirections(request: request)
        // The result is checked against the session again when the provider returns.
        let response = try await directions.calculate()
        try Task.checkCancellation()
        guard let native = response.routes.first, (2...200_000).contains(native.polyline.pointCount) else {
            throw ProviderError.noRoute
        }
        var coordinates = [CLLocationCoordinate2D](repeating: .init(latitude: 0, longitude: 0), count: native.polyline.pointCount)
        native.polyline.getCoordinates(&coordinates, range: NSRange(location: 0, length: coordinates.count))
        let points = coordinates.map { EvRoutePoint(latitude: $0.latitude, longitude: $0.longitude) }
        let steps = native.steps.map { NavigationGuidanceStep(instruction: $0.instructions, distanceMeters: $0.distance) }
        guard let route = NavigationGuidanceRoute(input: input, geometry: points, distanceMeters: native.distance,
                                                  expectedTravelTime: native.expectedTravelTime, steps: steps) else {
            throw ProviderError.noRoute
        }
        return (route, native)
    }

    static func mapItem(_ point: EvRoutePoint) -> MKMapItem {
        let coordinate = CLLocationCoordinate2D(latitude: point.latitude, longitude: point.longitude)
        let item = MKMapItem(placemark: MKPlacemark(coordinate: coordinate))
        if !point.label.isEmpty { item.name = point.label }
        return item
    }
}

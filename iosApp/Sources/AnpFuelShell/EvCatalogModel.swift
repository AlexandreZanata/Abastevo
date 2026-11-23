import Foundation
import Combine
import MapKit
import AnpFuelCore

public enum EvCatalogScope: String, CaseIterable, Identifiable {
    case city
    case state
    case country

    public var id: String { rawValue }

    public var label: String {
        switch self {
        case .city: return String(localized: "Cidade")
        case .state: return String(localized: "Estado")
        case .country: return String(localized: "Brasil")
        }
    }
}

public struct EvRefreshProgress: Equatable {
    public let pages: Int
    public let expectedPages: Int?
}

/// Admitted charging catalog snapshot for Explore, map pins and the trip planner.
/// Android references: `ChargingExploreViewModel`, `ChargingDiscoveryRepository`,
/// `ChargingMarkersSource` and `RoomEvCandidateSource`.
@MainActor
public final class EvCatalogModel: ObservableObject {
    @Published public private(set) var snapshot: EvCatalogSnapshot?
    @Published public private(set) var isLoading = false
    @Published public private(set) var refreshProgress: EvRefreshProgress?
    @Published public private(set) var errorMessage: String?
    @Published public var filter = ChargingFilter()
    @Published public var scope: EvCatalogScope = .city

    let service: EvCatalogService
    private weak var host: AppStateViewModel?
    private var refreshTask: Task<Void, Never>?
    private var loaded = false

    init(service: EvCatalogService = EvCatalogService()) {
        self.service = service
    }

    func attach(_ host: AppStateViewModel) {
        self.host = host
        loadIfNeeded()
    }

    public var isRefreshing: Bool { refreshProgress != nil }

    public var places: [ChargingPlace] { snapshot?.places ?? [] }

    public var age: EvSnapshotAge {
        ChargingRules.age(nowEpochSeconds: Int64(Date().timeIntervalSince1970), savedAtEpochSeconds: snapshot?.savedAtEpochSeconds)
    }

    public var savedAt: Date? {
        snapshot.map { Date(timeIntervalSince1970: TimeInterval($0.savedAtEpochSeconds)) }
    }

    public var storedBytes: Int64 { service.store.storedBytes() }

    func loadIfNeeded() {
        guard !loaded, !isLoading else { return }
        isLoading = true
        let store = service.store
        Task { [weak self] in
            let stored = await Task.detached(priority: .utility) { store.load() }.value
            guard let self else { return }
            self.loaded = true
            self.isLoading = false
            self.apply(stored)
        }
    }

    /// A snapshot pulled from another API origin is never shown as this origin's catalog.
    func originChanged() {
        refreshTask?.cancel()
        loaded = false
        snapshot = nil
        host?.evMapPoints = []
        loadIfNeeded()
    }

    private func apply(_ stored: EvCatalogSnapshot?) {
        guard let host else { return }
        if let stored, stored.origin == host.snapshot.apiEnvironment.origin {
            snapshot = stored
        } else {
            snapshot = nil
        }
        host.evMapPoints = ChargingRules.mapPoints(snapshot?.places ?? [])
    }

    /// Single-flight manual refresh; the saved catalog stays until a new one is complete.
    public func refresh() {
        guard let host, refreshTask == nil else { return }
        guard !host.isOfflineSimulation else {
            errorMessage = String(localized: "Sem conexão. O catálogo salvo continua disponível.")
            return
        }
        errorMessage = nil
        refreshProgress = EvRefreshProgress(pages: 0, expectedPages: nil)
        let client = AbastevoHttpClient(environment: host.snapshot.apiEnvironment)
        let service = self.service
        refreshTask = Task { [weak self] in
            do {
                let fresh = try await service.refresh(client: client) { pages, expected in
                    Task { @MainActor [weak self] in
                        guard let self, self.refreshTask != nil else { return }
                        self.refreshProgress = EvRefreshProgress(pages: pages, expectedPages: expected)
                    }
                }
                guard let self else { return }
                self.refreshTask = nil
                self.refreshProgress = nil
                self.loaded = true
                self.apply(fresh)
            } catch {
                guard let self else { return }
                self.refreshTask = nil
                self.refreshProgress = nil
                if !(error is CancellationError) {
                    self.errorMessage = String(localized: "Não foi possível atualizar o catálogo de recarga. O catálogo salvo continua disponível.")
                }
            }
        }
    }

    public func cancelRefresh() {
        refreshTask?.cancel()
    }

    public func filteredPlaces(city: Municipality?, near: EvCoordinate?) -> [ChargingPlace] {
        let currentScope = scope
        let currentFilter = filter
        let matching = places.filter { place in
            guard ChargingRules.matches(place, filter: currentFilter) else { return false }
            switch currentScope {
            case .city:
                guard let city else { return true }
                return ChargingRules.isInCity(place, city: city)
            case .state:
                guard let city else { return true }
                return place.uf?.uppercased() == city.state.rawValue
            case .country:
                return true
            }
        }
        guard let near else {
            return matching.sorted { $0.displayName.localizedCaseInsensitiveCompare($1.displayName) == .orderedAscending }
        }
        return matching.sorted { lhs, rhs in
            let left = lhs.coordinate.map { ChargingRules.distanceMeters($0, near) } ?? Double.greatestFiniteMagnitude
            let right = rhs.coordinate.map { ChargingRules.distanceMeters($0, near) } ?? Double.greatestFiniteMagnitude
            return left < right
        }
    }

    /// Planner candidates around the road geometry (Android `RoomEvCandidateSource`).
    /// The returned source captures the snapshot by value and never touches this model.
    func candidateSource(connector: ChargingConnector) -> @Sendable ([EvRoutePoint]) -> EvCandidateResult {
        guard let snapshot else {
            return { _ in EvCandidateResult(candidates: [], coverage: .missing) }
        }
        let places = snapshot.places
        let revision = snapshot.sourceRevision
        return { route in
            let candidates = EvCandidateMapper.candidates(
                places: places, connector: connector, sourceRevision: revision,
                route: route, corridorRadiusMeters: 25_000, limit: 200
            )
            return EvCandidateResult(candidates: candidates, coverage: .present, sourceRevision: revision)
        }
    }
}

/// MapKit road legs for the EV planner; a failed request is "no route", never a straight line.
struct MapKitEvRouter: EvRoadRouter {
    static let maxGeometryPoints = 1_500

    func leg(points: [EvRoutePoint]) async -> EvRoadLeg {
        guard points.count >= 2, let first = points.first, let last = points.last else { return .unavailable(.noRoute) }
        let request = Task { @MainActor () -> EvRoadLeg in
            let directions = MKDirections.Request()
            directions.source = MKMapItem(placemark: MKPlacemark(coordinate: CLLocationCoordinate2D(latitude: first.latitude, longitude: first.longitude)))
            directions.destination = MKMapItem(placemark: MKPlacemark(coordinate: CLLocationCoordinate2D(latitude: last.latitude, longitude: last.longitude)))
            directions.transportType = .automobile
            do {
                let response = try await MKDirections(request: directions).calculate()
                guard let route = response.routes.first, route.distance >= 1 else { return .unavailable(.noRoute) }
                let duration: Int64? = route.expectedTravelTime >= 1 ? Int64(route.expectedTravelTime.rounded()) : nil
                return .ready(
                    distanceMeters: Int64(route.distance.rounded()),
                    durationSeconds: duration,
                    warnings: !route.advisoryNotices.isEmpty,
                    geometry: MapKitEvRouter.geometry(route.polyline)
                )
            } catch {
                if Task.isCancelled { return .unavailable(.cancelled) }
                return .unavailable(.noRoute)
            }
        }
        return await withTaskCancellationHandler {
            await request.value
        } onCancel: {
            request.cancel()
        }
    }

    private static func geometry(_ polyline: MKPolyline) -> [EvRoutePoint] {
        let count = polyline.pointCount
        guard count >= 2 else { return [] }
        let points = polyline.points()
        let step = max(1, count / maxGeometryPoints)
        var result: [EvRoutePoint] = []
        var index = 0
        while index < count {
            let coordinate = points[index].coordinate
            result.append(EvRoutePoint(latitude: coordinate.latitude, longitude: coordinate.longitude))
            index += step
        }
        let lastCoordinate = points[count - 1].coordinate
        if result.last.map({ $0.latitude != lastCoordinate.latitude || $0.longitude != lastCoordinate.longitude }) ?? true {
            result.append(EvRoutePoint(latitude: lastCoordinate.latitude, longitude: lastCoordinate.longitude))
        }
        return result
    }
}

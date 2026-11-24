import AnpFuelCore
import MapKit
import SwiftUI

enum MapSearchSelection {
    case station(StationRecord)
    case charging(ChargingPlace)
    case apple(title: String, coordinate: EvCoordinate)

    var title: String {
        switch self {
        case .station(let station): return station.name
        case .charging(let place): return place.displayName
        case .apple(let title, _): return title
        }
    }

    var icon: String {
        switch self {
        case .station: return "fuelpump"
        case .charging: return "bolt.car"
        case .apple: return "mappin"
        }
    }

    var coordinate: EvCoordinate? {
        switch self {
        case .station(let station): return MapSearchPolicy.pin(for: station)
        case .charging(let place): return place.coordinate
        case .apple(_, let coordinate): return coordinate
        }
    }
}

private struct ApplePlaceMatch: Identifiable {
    let id = UUID()
    let title: String
    let address: String?
    let coordinate: EvCoordinate
}

@MainActor
private final class MapPlaceSearchModel: ObservableObject {
    enum Phase {
        case idle
        case loading
        case loaded([ApplePlaceMatch])
        case failed
    }

    @Published private(set) var phase = Phase.idle
    private var scope: StationCatalogSearchModel.Scope?
    private var search: MKLocalSearch?
    private var generation = UUID()

    func phase(for scope: StationCatalogSearchModel.Scope) -> Phase {
        self.scope == scope ? phase : .idle
    }

    func cancel() {
        generation = UUID()
        search?.cancel()
        search = nil
        scope = nil
        phase = .idle
    }

    func load(scope: StationCatalogSearchModel.Scope, city: Municipality) async {
        cancel()
        self.scope = scope
        guard !scope.offline, let query = MapSearchPolicy.appleQuery(scope.query, in: city) else {
            phase = .failed
            return
        }
        let token = generation
        phase = .loading
        do {
            try await Task.sleep(for: .milliseconds(300))
            guard active(token) else { return }
            let request = MKLocalSearch.Request()
            request.naturalLanguageQuery = query
            request.resultTypes = [.address, .pointOfInterest]
            let search = MKLocalSearch(request: request)
            self.search = search
            let response = try await search.start()
            guard active(token) else { return }
            let matches = response.mapItems.prefix(MapSearchPolicy.directoryPageSize).compactMap { item -> ApplePlaceMatch? in
                let point = item.placemark.coordinate
                let address = item.placemark.title
                guard let title = item.name ?? address, !title.isEmpty,
                      let coordinate = MapSearchPolicy.coordinate(latitude: point.latitude, longitude: point.longitude) else {
                    return nil
                }
                return ApplePlaceMatch(title: String(title.prefix(512)), address: address, coordinate: coordinate)
            }
            phase = .loaded(matches)
        } catch let error as MKError where error.code == .placemarkNotFound {
            if active(token) { phase = .loaded([]) }
        } catch {
            if active(token) { phase = .failed }
        }
        if generation == token { search = nil }
    }

    private func active(_ token: UUID) -> Bool { token == generation && !Task.isCancelled }
}

@MainActor
struct MapDiscoverySearchSheet: View {
    @Environment(\.dismiss) private var dismiss
    @ObservedObject var viewModel: AppStateViewModel
    let showStations: Bool
    let showCharging: Bool
    let onFocus: (MapSearchSelection?) -> Void
    let onSelect: (MapSearchSelection) -> Void

    @StateObject private var stations = StationCatalogSearchModel(pageSize: MapSearchPolicy.directoryPageSize)
    @StateObject private var apple = MapPlaceSearchModel()
    @State private var text = ""
    @State private var submittedQuery: String?
    @State private var refreshID = UUID()

    private struct Request: Hashable {
        let scope: StationCatalogSearchModel.Scope?
        let refreshID: UUID
    }

    private var activeScope: StationCatalogSearchModel.Scope? {
        submittedQuery.map { stations.activeScope(for: $0, on: viewModel) }
    }

    var body: some View {
        NavigationStack {
            List {
                if let scope = activeScope, let city = viewModel.selectedMunicipality, scope.municipalityCode == city.ibgeCode {
                    if showStations { stationSection(scope: scope) }
                    if showCharging { chargingSection(scope: scope, city: city) }
                    appleSection(scope: scope)
                } else {
                    Text(String(localized: "android.search_min_chars_hint"))
                        .foregroundStyle(.secondary)
                }
            }
            .navigationTitle(String(localized: "android.map_search"))
            .navigationBarTitleDisplayMode(.inline)
            .searchable(text: $text, placement: .navigationBarDrawer(displayMode: .always),
                        prompt: String(localized: "map.search_prompt"))
            .onSubmit(of: .search, submit)
            .onChange(of: text) { _, new in
                if StationDossierCodec.normalizedSearch(new) != submittedQuery {
                    submittedQuery = nil
                    stations.cancel()
                    apple.cancel()
                    onFocus(nil)
                }
            }
            .toolbar {
                ToolbarItem(placement: .cancellationAction) {
                    Button(String(localized: "android.feed_close")) { dismiss() }
                }
                ToolbarItem(placement: .primaryAction) {
                    Button(String(localized: "android.map_search"), action: submit)
                        .disabled(StationDossierCodec.normalizedSearch(text).isEmpty)
                }
            }
            .task(id: Request(scope: activeScope, refreshID: refreshID)) {
                guard let scope = activeScope, let city = viewModel.selectedMunicipality,
                      scope.municipalityCode == city.ibgeCode else {
                    apple.cancel()
                    return
                }
                async let local: Void = apple.load(scope: scope, city: city)
                if showStations {
                    await stations.load(scope, host: viewModel, reset: true, debounce: true)
                }
                if !Task.isCancelled, scope == activeScope {
                    focusSingleMatch(scope: scope, city: city)
                }
                await local
            }
            .onDisappear {
                stations.cancel()
                apple.cancel()
            }
        }
    }

    private func submit() {
        let clean = StationDossierCodec.normalizedSearch(text)
        guard !clean.isEmpty else { return }
        submittedQuery = clean
        refreshID = UUID()
    }

    private func chargingMatches(scope: StationCatalogSearchModel.Scope, city: Municipality) -> [ChargingPlace] {
        guard showCharging else { return [] }
        return MapSearchPolicy.chargingMatches(
            viewModel.evCatalog.places, query: scope.query, municipality: city, filter: viewModel.evCatalog.filter
        )
    }

    private func focusSingleMatch(scope: StationCatalogSearchModel.Scope, city: Municipality) {
        if showStations {
            guard stations.isCurrent(scope), stations.failure == nil || stations.showsLoadedFallback, stations.cursor == nil else { return }
        }
        let fuelMatches = showStations ? stations.rows : []
        let chargers = chargingMatches(scope: scope, city: city)
        guard fuelMatches.count + chargers.count == 1 else { return }
        let selection = fuelMatches.first.map(MapSearchSelection.station) ?? chargers.first.map(MapSearchSelection.charging)
        if let selection, selection.coordinate != nil { onFocus(selection) }
    }

    private func stationSection(scope: StationCatalogSearchModel.Scope) -> some View {
        Section(String(localized: "map.search_abastevo_stations")) {
            if stations.isCurrent(scope) {
                if stations.showsLoadedFallback {
                    Text(String(localized: "android.server_station_cached"))
                        .font(.footnote)
                        .foregroundStyle(.secondary)
                } else if stations.failure != nil {
                    Button(String(localized: "android.action_retry"), action: submit)
                    Text(String(localized: "android.map_station_search_failed"))
                        .foregroundStyle(.secondary)
                }
                ForEach(stations.rows) { station in
                    Button {
                        onSelect(.station(station))
                    } label: {
                        searchRow(
                            station.name, address: station.address,
                            warning: stationLocationWarning(station),
                            symbol: "fuelpump"
                        )
                    }
                }
                if stations.cursor != nil, !stations.isLoading {
                    Button(String(localized: "android.feed_load_more")) {
                        Task { await stations.load(scope, host: viewModel, reset: false) }
                    }
                }
                if stations.isLoading {
                    ProgressView().accessibilityLabel(String(localized: "android.map_search"))
                } else if stations.rows.isEmpty, stations.failure == nil {
                    emptyMessage
                }
            } else {
                ProgressView().accessibilityLabel(String(localized: "android.map_search"))
            }
        }
    }

    private func stationLocationWarning(_ station: StationRecord) -> String? {
        guard MapSearchPolicy.pin(for: station) != nil else { return String(localized: "android.map_location_unknown") }
        return station.pinTier == .reviewed ? nil : station.pinTier.localizedLabel
    }

    private func chargingSection(scope: StationCatalogSearchModel.Scope, city: Municipality) -> some View {
        Section(String(localized: "android.charging_explore_title")) {
            let matches = chargingMatches(scope: scope, city: city)
            if viewModel.evCatalog.snapshot == nil {
                Text(String(localized: "android.charging_catalog_unavailable"))
                    .foregroundStyle(.secondary)
            } else if matches.isEmpty {
                emptyMessage
            } else {
                ForEach(matches) { place in
                    Button {
                        onSelect(.charging(place))
                    } label: {
                        searchRow(
                            place.displayName, address: place.addressLine,
                            warning: place.coordinate == nil ? String(localized: "android.map_location_unknown") : nil,
                            symbol: "bolt.car"
                        )
                    }
                }
            }
        }
    }

    private func appleSection(scope: StationCatalogSearchModel.Scope) -> some View {
        Section(String(localized: "map.search_apple_maps")) {
            switch apple.phase(for: scope) {
            case .idle, .loading:
                ProgressView().accessibilityLabel(String(localized: "android.map_search"))
            case .failed:
                Text(String(localized: "map.search_apple_unavailable"))
                    .foregroundStyle(.secondary)
                if !scope.offline { Button(String(localized: "android.action_retry"), action: submit) }
            case .loaded(let matches):
                if matches.isEmpty {
                    emptyMessage
                } else {
                    ForEach(matches) { place in
                        Button {
                            onSelect(.apple(title: place.title, coordinate: place.coordinate))
                        } label: {
                            searchRow(place.title, address: place.address, warning: nil, symbol: "mappin")
                        }
                    }
                }
            }
        }
    }

    private var emptyMessage: some View {
        Text(String(localized: "android.map_no_results")).foregroundStyle(.secondary)
    }

    private func searchRow(_ title: String, address: String?, warning: String?, symbol: String) -> some View {
        Label {
            VStack(alignment: .leading, spacing: 4) {
                Text(title)
                if let address, !address.isEmpty, address != title {
                    Text(address).foregroundStyle(.secondary).font(.subheadline)
                }
                if let warning {
                    Text(warning).foregroundStyle(.secondary).font(.footnote)
                }
            }
        } icon: {
            Image(systemName: symbol)
        }
        .accessibilityElement(children: .combine)
    }
}

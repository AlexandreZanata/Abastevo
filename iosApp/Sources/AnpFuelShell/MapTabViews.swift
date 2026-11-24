import SwiftUI
import MapKit
import AnpFuelCore
#if os(iOS) && canImport(UIKit)
import UIKit
#endif

/// Map tab with station pins, route preview, EV trip planner and saved places.
public struct MapRootView: View {
    @ObservedObject var viewModel: AppStateViewModel
    @State private var cameraPosition: MapCameraPosition = .automatic
    @State private var showEvSites = true
    @State private var showCombustionStations = true
    @State private var sourceFilter = "all"
    @State private var visibleRegion: MKCoordinateRegion?
    @State private var selectedEvPlace: ChargingPlace?
    @State private var showSearch = false
    @State private var searchSelection: MapSearchSelection?
    @State private var searchFocus: MapSearchSelection?

    public var body: some View {
        let pinStations = viewModel.sortedStations.filter { $0.pinTier.allowsMapPin }
        let evPins = showEvSites ? evPinsForMap(pinStations) : []
        let listOnlyStations = viewModel.sortedStations.filter { !$0.pinTier.allowsMapPin }
        NavigationStack(path: $viewModel.mapPath) {
            VStack(spacing: 0) {
                mapToolbar
                ZStack(alignment: .bottomTrailing) {
                    Map(position: $cameraPosition) {
                        if showCombustionStations {
                            ForEach(pinStations) { st in
                                if sourceFilter != "community" || viewModel.communityObservation(stationId: st.id, product: viewModel.snapshot.selectedFuel) != nil {
                                    Annotation(
                                        st.name,
                                        coordinate: CLLocationCoordinate2D(latitude: st.latitude, longitude: st.longitude)
                                    ) {
                                        Button {
                                            viewModel.selectedStation = st
                                        } label: {
                                            stationMapPin(st)
                                        }
                                    }
                                }
                            }
                        }
                        ForEach(evPins) { point in
                            Annotation(
                                point.place.displayName,
                                coordinate: CLLocationCoordinate2D(latitude: point.coordinate.latitude, longitude: point.coordinate.longitude)
                            ) {
                                Button {
                                    selectedEvPlace = point.place
                                } label: {
                                    VStack(spacing: 2) {
                                        Image(systemName: "bolt.car.fill")
                                            .font(.caption.bold())
                                            .foregroundStyle(.white)
                                            .padding(6)
                                            .background(AbastevoTheme.accentLight)
                                            .clipShape(Circle())
                                        Text(point.place.maxPowerW.map(EvText.power) ?? String(localized: "map.electric"))
                                            .font(.system(size: 10, weight: .bold))
                                            .padding(.horizontal, 5)
                                            .padding(.vertical, 2)
                                            .background(.ultraThinMaterial)
                                            .clipShape(Capsule())
                                    }
                                }
                                .accessibilityLabel(point.place.displayName)
                            }
                        }
                        if let focus = searchFocus, let coordinate = focus.coordinate,
                           !isVisible(focus, stations: pinStations, chargers: evPins) {
                            let point = CLLocationCoordinate2D(latitude: coordinate.latitude, longitude: coordinate.longitude)
                            Annotation(focus.title, coordinate: point) {
                                Button { showMapSearchSelection(focus) } label: {
                                    Image(systemName: focus.icon)
                                        .padding(9)
                                        .background(.regularMaterial, in: Circle())
                                }
                                .accessibilityLabel(focus.title)
                            }
                        }
                    }
                    .frame(maxWidth: .infinity, maxHeight: .infinity)
                    .onMapCameraChange(frequency: .onEnd) { context in
                        visibleRegion = context.region
                    }
                    .onChange(of: viewModel.selectedMunicipality?.ibgeCode) { _, _ in
                        cameraPosition = .automatic
                        searchFocus = nil
                        searchSelection = nil
                    }
                    .onChange(of: viewModel.snapshot.apiEnvironment) { _, _ in
                        searchFocus = nil
                        searchSelection = nil
                    }

                    VStack(spacing: 8) {
                        Button {
                            Task {
                                await viewModel.requestOneShotDeviceLocation()
                                if let coord = viewModel.lastDeviceCoordinate {
                                    cameraPosition = .region(MKCoordinateRegion(
                                        center: coord,
                                        span: MKCoordinateSpan(latitudeDelta: 0.04, longitudeDelta: 0.04)
                                    ))
                                }
                            }
                        } label: {
                            Image(systemName: "location.fill")
                                .padding(12)
                                .background(.regularMaterial)
                                .clipShape(Circle())
                        }
                        .accessibilityLabel(String(localized: "android.map_guidance_recenter"))
                    }
                    .padding(16)
                }

                if !listOnlyStations.isEmpty {
                    HStack {
                        Image(systemName: "info.circle")
                            .font(.caption)
                        Text(String(localized: "map.stations_without_pin", defaultValue: "Stations without a map pin: \(listOnlyStations.count.formatted()). Find them in the list."))
                            .font(.caption2)
                        Spacer()
                        Button(String(localized: "map.show_list")) {
                            viewModel.selectedTab = .explore
                        }
                        .font(.caption2.bold())
                    }
                    .padding(.horizontal, 12)
                    .padding(.vertical, 6)
                    .background(Color.secondary.opacity(0.12))
                }
            }
            .sheet(item: $selectedEvPlace) { place in
                EvPlaceDetailSheet(place: place, snapshot: viewModel.evCatalog.snapshot)
            }
            .sheet(isPresented: $showSearch, onDismiss: completeMapSearchSelection) {
                MapDiscoverySearchSheet(
                    viewModel: viewModel, showStations: showCombustionStations || !showEvSites,
                    showCharging: showEvSites || !showCombustionStations,
                    onFocus: focusMapSearchResult, onSelect: { result in
                        searchSelection = result
                        showSearch = false
                    }
                )
            }
        }
    }

    private func isVisible(_ match: MapSearchSelection, stations: [StationRecord], chargers: [ChargingMapPoint]) -> Bool {
        switch match {
        case .station(let station):
            return showCombustionStations && stations.contains { $0.id == station.id } &&
                (sourceFilter != "community" || viewModel.communityObservation(stationId: station.id, product: viewModel.snapshot.selectedFuel) != nil)
        case .charging(let place):
            return chargers.contains { $0.place.id == place.id }
        case .apple:
            return false
        }
    }

    private func focusMapSearchResult(_ result: MapSearchSelection?) {
        searchFocus = result
        guard let point = result?.coordinate else { return }
        cameraPosition = .region(MKCoordinateRegion(
            center: CLLocationCoordinate2D(latitude: point.latitude, longitude: point.longitude),
            latitudinalMeters: 1000, longitudinalMeters: 1000
        ))
    }

    private func completeMapSearchSelection() {
        guard let result = searchSelection else { return }
        searchSelection = nil
        showMapSearchSelection(result)
    }

    private func showMapSearchSelection(_ result: MapSearchSelection) {
        focusMapSearchResult(result)
        switch result {
        case .station(let station):
            viewModel.selectedStation = station
        case .charging(let place):
            selectedEvPlace = place
        case .apple(let title, let point):
            viewModel.navigationDestination = NavigationDestination(
                title: title, latitude: point.latitude, longitude: point.longitude
            )
        }
    }

    /// Admitted charger coordinates inside the visible map region (Android `ChargingMarkersSource`).
    private func evPinsForMap(_ pinStations: [StationRecord]) -> [ChargingMapPoint] {
        let center: EvCoordinate
        let latitudeDelta: Double
        let longitudeDelta: Double
        if let region = visibleRegion {
            center = EvCoordinate(latitude: region.center.latitude, longitude: region.center.longitude)
            latitudeDelta = region.span.latitudeDelta
            longitudeDelta = region.span.longitudeDelta
        } else if let device = viewModel.lastDeviceCoordinate {
            center = EvCoordinate(latitude: device.latitude, longitude: device.longitude)
            latitudeDelta = 0.3
            longitudeDelta = 0.3
        } else if let station = pinStations.first {
            center = EvCoordinate(latitude: station.latitude, longitude: station.longitude)
            latitudeDelta = 0.3
            longitudeDelta = 0.3
        } else {
            return []
        }
        return ChargingRules.pins(
            viewModel.evMapPoints, center: center, latitudeDelta: latitudeDelta, longitudeDelta: longitudeDelta, limit: 150
        )
    }

    private var mapToolbar: some View {
        ScrollView(.horizontal, showsIndicators: false) {
            HStack(spacing: 8) {
                Button {
                    showSearch = true
                } label: {
                    Label(String(localized: "android.map_search"), systemImage: "magnifyingglass")
                }
                .buttonStyle(.bordered)
                Picker(String(localized: "android.map_filter_source"), selection: $sourceFilter) {
                    Text(String(localized: "android.map_filter_all")).tag("all")
                    Text(String(localized: "android.map_source_community")).tag("community")
                }
                .pickerStyle(.segmented)
                .frame(width: 170)

                Toggle(String(localized: "android.nav_stations"), isOn: $showCombustionStations)
                    .toggleStyle(.button)
                Toggle(String(localized: "map.electric"), isOn: $showEvSites)
                    .toggleStyle(.button)

                Button {
                    viewModel.activeSheet = .evTripPlanner
                } label: {
                    Label(String(localized: "map.ev_route"), systemImage: "bolt.horizontal.fill")
                }
                .buttonStyle(.bordered)

                Button {
                    viewModel.activeSheet = .savedPlaces
                } label: {
                    Label(String(localized: "map.saved"), systemImage: "bookmark.fill")
                }
                .buttonStyle(.bordered)

                ActiveNavigationResumeButton(navigation: viewModel.navigation) {
                    viewModel.navigationDestination = $0
                }

                Button {
                    viewModel.activeSheet = .mapDownloads
                } label: {
                    Label(String(localized: "map.offline.action"), systemImage: "arrow.down.circle")
                }
                .buttonStyle(.bordered)
            }
            .padding(.horizontal, 12)
            .padding(.vertical, 8)
        }
        .background(.bar)
    }

    private func stationMapPin(_ st: StationRecord) -> some View {
        let fuel = viewModel.snapshot.selectedFuel
        let price = viewModel.communityObservation(stationId: st.id, product: fuel)?.amountMilli
        return VStack(spacing: 2) {
            Text(price.map { AbastevoTheme.formatBrlMilli($0) } ?? "—")
                .font(.caption2.bold())
                .foregroundStyle(.white)
                .padding(.horizontal, 7)
                .padding(.vertical, 4)
                .background(AbastevoTheme.ink)
                .clipShape(Capsule())
            Image(systemName: "arrowtriangle.down.fill")
                .font(.system(size: 8))
                .foregroundStyle(AbastevoTheme.ink)
        }
    }

}

public struct NativeRouteSheetView: View {
    @Environment(\.dismiss) private var dismiss
    @ObservedObject var viewModel: AppStateViewModel
    let station: StationRecord
    @State private var destination: NavigationDestination

    init(viewModel: AppStateViewModel, station: StationRecord) {
        self.viewModel = viewModel
        self.station = station
        _destination = State(initialValue: NavigationDestination(
            title: station.name, latitude: station.latitude, longitude: station.longitude, allowsWaze: true
        ))
    }

    public var body: some View {
        if station.pinTier.allowsMapPin {
            ActiveNavigationSheetView(viewModel: viewModel, destination: destination)
        } else {
            NavigationStack {
                ContentUnavailableView(
                    String(localized: "map.route.destination_unverified"), systemImage: "location.slash"
                )
                .toolbar {
                    ToolbarItem(placement: .cancellationAction) {
                        Button(String(localized: "android.feed_close")) { dismiss() }
                    }
                }
            }
        }
    }
}

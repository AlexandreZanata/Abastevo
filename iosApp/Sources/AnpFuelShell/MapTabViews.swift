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
    @State private var selectedEvPoint: ChargingMapPoint?

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
                                    selectedEvPoint = point
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
                    }
                    .frame(maxWidth: .infinity, maxHeight: .infinity)
                    .onMapCameraChange(frequency: .onEnd) { context in
                        visibleRegion = context.region
                    }
                    .onChange(of: viewModel.selectedMunicipality?.ibgeCode) { _, _ in
                        cameraPosition = .automatic
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
            .sheet(item: $selectedEvPoint) { point in
                EvPlaceDetailSheet(place: point.place, snapshot: viewModel.evCatalog.snapshot)
            }
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
    @State private var requestCount = 0
    @State private var isCalculating = false
    @State private var route: MKRoute?
    @State private var routeMessage: String?

    public var body: some View {
        NavigationStack {
            List {
                if station.pinTier.allowsMapPin {
                    Section(String(localized: "map.route.destination")) {
                        Text(station.name).font(.headline)
                        Text(station.address)
                            .font(.subheadline)
                            .foregroundStyle(.secondary)
                        Button(String(localized: "map.route.calculate")) { requestCount += 1 }
                            .disabled(isCalculating)
                        if isCalculating {
                            ProgressView(String(localized: "android.map_route_calculating"))
                        } else if let routeMessage {
                            Text(routeMessage).foregroundStyle(.secondary)
                        }
                    }

                    if let route {
                        Section(String(localized: "map.route.preview")) {
                            Text(Measurement(value: route.distance / 1_000, unit: UnitLength.kilometers).formatted(.measurement(width: .abbreviated, usage: .asProvided, numberFormatStyle: .number.precision(.fractionLength(1)))))
                            Toggle(String(localized: "map.route.voice"), isOn: $viewModel.isVoiceGuidanceEnabled)
                            ForEach(route.steps.indices, id: \.self) { index in
                                let instruction = route.steps[index].instructions
                                if !instruction.isEmpty {
                                    Button {
                                        viewModel.speakGuidanceStep(instruction)
                                    } label: {
                                        Label(instruction, systemImage: "speaker.wave.2")
                                    }
                                    .disabled(!viewModel.isVoiceGuidanceEnabled)
                                }
                            }
                        }
                    }

                    Section(String(localized: "map.route.navigation")) {
                        Button(String(localized: "map.route.open_apple_maps")) {
                            let item = destination(for: station)
                            item.openInMaps(launchOptions: [MKLaunchOptionsDirectionsModeKey: MKLaunchOptionsDirectionsModeDriving])
                        }
                        if let wazeUrl = URL(string: "https://waze.com/ul?ll=\(station.latitude),\(station.longitude)&navigate=yes") {
                            Link(String(localized: "map.route.open_waze"), destination: wazeUrl)
                        }
                    }
                } else {
                    ContentUnavailableView(String(localized: "map.route.destination_unverified"), systemImage: "location.slash")
                }
            }
            .navigationTitle(String(localized: "map.route.station"))
            .toolbar {
                ToolbarItem(placement: .cancellationAction) {
                    Button(String(localized: "android.feed_close")) { dismiss() }
                }
            }
        }
        .task(id: requestCount) {
            if requestCount > 0, station.pinTier.allowsMapPin {
                await calculateRoute(to: station)
            }
        }
        .onDisappear { viewModel.stopGuidance() }
    }

    private func destination(for station: StationRecord) -> MKMapItem {
        let coordinate = CLLocationCoordinate2D(latitude: station.latitude, longitude: station.longitude)
        let item = MKMapItem(placemark: MKPlacemark(coordinate: coordinate))
        item.name = station.name
        return item
    }

    private func calculateRoute(to station: StationRecord) async {
        isCalculating = true
        route = nil
        routeMessage = nil
        defer { isCalculating = false }

        guard let origin = await viewModel.requestRouteOrigin(), !Task.isCancelled else {
            routeMessage = String(localized: "map.route.location_unavailable")
            return
        }
        let request = MKDirections.Request()
        request.source = MKMapItem(placemark: MKPlacemark(coordinate: origin))
        request.destination = destination(for: station)
        request.transportType = .automobile
        do {
            let response = try await MKDirections(request: request).calculate()
            guard !Task.isCancelled else { return }
            route = response.routes.first
            if route == nil {
                routeMessage = String(localized: "map.route.not_found")
            }
        } catch {
            guard !Task.isCancelled else { return }
            routeMessage = String(localized: "map.route.failed")
        }
    }
}

public struct SavedPlacesSheetView: View {
    @Environment(\.dismiss) private var dismiss
    @ObservedObject var viewModel: AppStateViewModel

    public var body: some View {
        NavigationStack {
            List {
                Section(String(localized: "map.saved.places")) {
                    ForEach(viewModel.snapshot.savedPlaces) { place in
                        VStack(alignment: .leading) {
                            Text(place.title).font(.headline)
                            Text(place.subtitle).font(.caption).foregroundStyle(.secondary)
                        }
                    }
                }
                Section(String(localized: "map.saved.trips")) {
                    ForEach(viewModel.snapshot.savedTrips) { trip in
                        VStack(alignment: .leading) {
                            Text(trip.title).font(.headline)
                            let distance = Measurement(value: trip.distanceKm, unit: UnitLength.kilometers)
                                .formatted(.measurement(width: .abbreviated, usage: .asProvided, numberFormatStyle: .number.precision(.fractionLength(0))))
                            Text(String(localized: "map.saved.trip_summary", defaultValue: "\(trip.originLabel) → \(trip.destinationLabel) (\(distance))"))
                                .font(.caption)
                                .foregroundStyle(.secondary)
                        }
                    }
                }
            }
            .navigationTitle(String(localized: "map.saved.title"))
            .toolbar {
                ToolbarItem(placement: .cancellationAction) {
                    Button(String(localized: "android.feed_close")) { dismiss() }
                }
            }
        }
    }
}

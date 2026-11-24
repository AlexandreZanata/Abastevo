import AnpFuelCore
import CoreLocation
import MapKit
import SwiftUI
import UIKit

@MainActor
struct ActiveNavigationSheetView: View {
    @Environment(\.dismiss) private var dismiss
    @ObservedObject var viewModel: AppStateViewModel
    @ObservedObject private var coordinator: NavigationCoordinator
    let destination: NavigationDestination
    @State private var camera: MapCameraPosition = .automatic
    @State private var showOriginPicker = false
    @State private var manualOrigin: EvRoutePoint?

    init(viewModel: AppStateViewModel, destination: NavigationDestination) {
        self.viewModel = viewModel
        self.destination = destination
        _coordinator = ObservedObject(wrappedValue: viewModel.navigation)
    }

    private var shownDestination: NavigationDestination { coordinator.destination ?? destination }

    var body: some View {
        NavigationStack {
            List {
                if shownDestination.isRoutable {
                    Section(String(localized: "map.route.destination")) {
                        Text(shownDestination.title).font(.headline)
                        if coordinator.hasSession && shownDestination.id != destination.id {
                            Text(String(localized: "android.map_route_stop_first"))
                                .foregroundStyle(.secondary)
                        }
                        routeMap
                    }
                    Section {
                        status
                        if let route = coordinator.mapRoute {
                            if let progress = coordinator.progress {
                                Text(estimate(metres: progress.remainingMeters, seconds: progress.remainingSeconds))
                                    .font(.headline.monospacedDigit())
                                if let next = progress.nextManeuver {
                                    Text(next.instruction).font(.headline)
                                    if let distance = progress.distanceToManeuverMeters {
                                        Text(distanceText(distance)).foregroundStyle(.secondary)
                                    }
                                }
                            } else {
                                Text(estimate(metres: route.distance, seconds: route.expectedTravelTime))
                                    .font(.headline.monospacedDigit())
                            }
                        }
                        if coordinator.hasSession {
                            guidanceControls
                        } else {
                            previewControls
                        }
                    }
                    if !coordinator.hasSession, let route = coordinator.mapRoute {
                        Section(String(localized: "android.map_route_steps")) {
                            ForEach(route.steps.indices, id: \.self) { index in
                                let step = route.steps[index]
                                if !step.instructions.isEmpty {
                                    Text(step.instructions)
                                }
                            }
                        }
                    }
                    externalNavigation
                } else {
                    ContentUnavailableView(String(localized: "map.route.destination_unverified"), systemImage: "location.slash")
                }
            }
            .navigationTitle(String(localized: "map.route.navigation"))
            .toolbar {
                ToolbarItem(placement: .cancellationAction) {
                    Button(String(localized: "android.feed_close")) { dismiss() }
                }
            }
        }
        .task(id: destination.id) {
            let otherSession = coordinator.hasSession && coordinator.destination?.id != destination.id
            coordinator.prepare(destination: destination)
            coordinator.voiceEnabled = viewModel.isVoiceGuidanceEnabled
            if !otherSession && coordinator.phase == .idle && destination.isRoutable {
                plan()
            }
            showWholeRoute()
        }
        .onChange(of: coordinator.routeRevision) { _, _ in showWholeRoute() }
        .onChange(of: viewModel.isVoiceGuidanceEnabled) { _, enabled in coordinator.voiceEnabled = enabled }
        .sheet(isPresented: $showOriginPicker) {
            NavigationOriginPicker { selected in
                manualOrigin = selected
                plan()
            }
        }
    }

    private var routeMap: some View {
        Map(position: $camera, interactionModes: [.pan, .zoom, .rotate]) {
            Marker(shownDestination.title, coordinate: shownDestination.coordinate)
            if let route = coordinator.mapRoute {
                MapPolyline(route.polyline).stroke(AbastevoTheme.accentLight, lineWidth: 6)
            }
            if let position = coordinator.currentPosition {
                Annotation(String(localized: "map.active.position"), coordinate: position) {
                    Image(systemName: "location.circle.fill")
                        .font(.title2)
                        .foregroundStyle(AbastevoTheme.ink, .white)
                        .accessibilityLabel(String(localized: "map.active.position"))
                }
            }
        }
        .frame(minHeight: 220)
        .mapControls {
            MapCompass()
            MapScaleView()
        }
        .accessibilityLabel(String(localized: "map.route.preview"))
    }

    @ViewBuilder private var status: some View {
        switch coordinator.phase {
        case .idle, .preview: EmptyView()
        case .planning: ProgressView(String(localized: "android.map_route_calculating"))
        case .originUnavailable: Text(String(localized: "map.route.location_unavailable"))
        case .permissionRequested: Text(String(localized: "android.map_route_locating"))
        case .permissionDenied:
            Text(String(localized: "map.active.permission"))
            Button(String(localized: "map.active.open_settings")) {
                if let url = URL(string: UIApplication.openSettingsURLString) { UIApplication.shared.open(url) }
            }
        case .providerFailed: Text(String(localized: "map.route.failed"))
        case .active(let state):
            switch state {
            case .waitingForFix: ProgressView(String(localized: "android.map_route_locating"))
            case .following: EmptyView()
            case .weakSignal: Text(String(localized: "map.active.weak"))
            case .offRoute: Text(String(localized: "map.active.off_route"))
            case .rerouting: ProgressView(String(localized: "map.active.rerouting"))
            case .rerouteFailed:
                Text(String(localized: "map.active.reroute_failed"))
                Button(String(localized: "map.route.calculate")) { coordinator.retry() }
            case .arrived: Text(String(localized: "android.map_route_arrived")).font(.headline)
            }
        }
        if coordinator.voiceFailed {
            Text(String(localized: "map.active.voice_unavailable"))
                .foregroundStyle(.secondary)
        }
    }

    private var guidanceControls: some View {
        Group {
            Text(String(localized: "map.active.background_notice"))
                .font(.footnote)
                .foregroundStyle(.secondary)
            Toggle(String(localized: "map.route.voice"), isOn: $viewModel.isVoiceGuidanceEnabled)
            Button(String(localized: "android.map_guidance_overview"), action: showWholeRoute)
            if let position = coordinator.currentPosition {
                Button(String(localized: "android.map_guidance_recenter")) {
                    camera = .region(MKCoordinateRegion(
                        center: position,
                        span: MKCoordinateSpan(latitudeDelta: 0.008, longitudeDelta: 0.008)
                    ))
                }
            }
            Button(String(localized: "map.active.stop"), role: .destructive) { coordinator.end() }
        }
    }

    private var previewControls: some View {
        Group {
            if let manualOrigin, !manualOrigin.label.isEmpty {
                Label(manualOrigin.label, systemImage: "point.topleft.down.to.point.bottomright.curvepath")
            }
            Button(String(localized: "map.active.choose_origin")) { showOriginPicker = true }
                .disabled(coordinator.phase == .planning)
            Button(String(localized: "map.route.calculate")) {
                manualOrigin = nil
                plan()
            }
            .disabled(coordinator.phase == .planning)
            if coordinator.mapRoute != nil {
                Text(String(localized: "map.active.background_notice"))
                    .font(.footnote)
                    .foregroundStyle(.secondary)
                Button(String(localized: "map.active.start")) { coordinator.start() }
                    .buttonStyle(.borderedProminent)
            }
        }
    }

    private var externalNavigation: some View {
        Section {
            Button(String(localized: "map.route.open_apple_maps")) {
                NavigationCoordinator.mapItem(shownDestination.point).openInMaps(
                    launchOptions: [MKLaunchOptionsDirectionsModeKey: MKLaunchOptionsDirectionsModeDriving]
                )
            }
            if shownDestination.allowsWaze,
               let url = URL(string: "https://waze.com/ul?ll=\(shownDestination.latitude),\(shownDestination.longitude)&navigate=yes") {
                Link(String(localized: "map.route.open_waze"), destination: url)
            }
        }
    }

    private func plan() {
        coordinator.plan(destination: destination, from: manualOrigin) {
            await viewModel.requestRouteOrigin()
        }
    }

    private func showWholeRoute() {
        if let route = coordinator.mapRoute {
            camera = .rect(route.polyline.boundingMapRect)
        } else if shownDestination.isRoutable {
            camera = .region(MKCoordinateRegion(
                center: shownDestination.coordinate,
                span: MKCoordinateSpan(latitudeDelta: 0.03, longitudeDelta: 0.03)
            ))
        }
    }

    private func estimate(metres: Double, seconds: TimeInterval) -> String {
        let distance = distanceText(metres)
        let time = Measurement(value: seconds / 60, unit: UnitDuration.minutes)
            .formatted(.measurement(width: .abbreviated, usage: .asProvided, numberFormatStyle: .number.precision(.fractionLength(0))))
        return String(localized: "android.map_route_estimate", defaultValue: "\(distance) · \(time)")
    }

    private func distanceText(_ metres: Double) -> String {
        Measurement(value: metres >= 1000 ? metres / 1000 : metres,
                    unit: metres >= 1000 ? UnitLength.kilometers : UnitLength.meters)
            .formatted(.measurement(width: .abbreviated, usage: .asProvided, numberFormatStyle: .number.precision(.fractionLength(0...1))))
    }
}

@MainActor
struct ActiveNavigationResumeButton: View {
    @ObservedObject var navigation: NavigationCoordinator
    let resume: (NavigationDestination) -> Void

    var body: some View {
        if navigation.hasSession, let destination = navigation.destination {
            Button {
                resume(destination)
            } label: {
                Label(String(localized: "map.active.open"), systemImage: "location.north.line.fill")
            }
            .buttonStyle(.bordered)
        }
    }
}

@MainActor
private struct NavigationOriginPicker: View {
    @Environment(\.dismiss) private var dismiss
    let chose: (EvRoutePoint) -> Void
    @State private var query = ""
    @State private var results: [EvRoutePoint] = []
    @State private var loading = false
    @State private var unavailable = false

    private var term: String { query.trimmingCharacters(in: .whitespacesAndNewlines) }

    var body: some View {
        NavigationStack {
            List {
                if loading { ProgressView() }
                if unavailable {
                    Text(String(localized: "map.active.origin_failed"))
                }
                ForEach(Array(results.enumerated()), id: \.offset) { _, point in
                    Button(point.label) {
                        chose(point)
                        dismiss()
                    }
                }
            }
            .navigationTitle(String(localized: "map.active.choose_origin"))
            .searchable(text: $query, prompt: String(localized: "map.active.search_origin"))
            .toolbar {
                ToolbarItem(placement: .cancellationAction) {
                    Button(String(localized: "android.feed_close")) { dismiss() }
                }
            }
            .task(id: term) {
                await search(term)
            }
        }
    }

    private func search(_ submitted: String) async {
        results = []
        unavailable = false
        loading = submitted.count >= 2
        guard loading else { return }
        defer { if submitted == term { loading = false } }
        do {
            try await Task.sleep(for: .milliseconds(300))
            let request = MKLocalSearch.Request()
            request.naturalLanguageQuery = String(submitted.prefix(160))
            request.resultTypes = .address
            let response = try await MKLocalSearch(request: request).start()
            guard !Task.isCancelled, submitted == term else { return }
            results = response.mapItems.prefix(20).compactMap { item in
                let coordinate = item.placemark.coordinate
                guard CLLocationCoordinate2DIsValid(coordinate),
                      let label = item.placemark.title ?? item.name, !label.isEmpty else { return nil }
                return EvRoutePoint(latitude: coordinate.latitude, longitude: coordinate.longitude, label: label)
            }
            unavailable = results.isEmpty
        } catch {
            if !Task.isCancelled && submitted == term { unavailable = true }
        }
    }
}

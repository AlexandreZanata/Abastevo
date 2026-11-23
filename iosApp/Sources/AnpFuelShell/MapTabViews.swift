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

    public var body: some View {
        let pinStations = viewModel.sortedStations.filter { $0.pinTier.allowsMapPin }
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
                        if showEvSites {
                            ForEach(viewModel.evSites) { ev in
                                Annotation(
                                    ev.name,
                                    coordinate: CLLocationCoordinate2D(latitude: ev.latitude, longitude: ev.longitude)
                                ) {
                                    VStack(spacing: 2) {
                                        Image(systemName: "bolt.car.fill")
                                            .font(.caption.bold())
                                            .foregroundStyle(.white)
                                            .padding(6)
                                            .background(AbastevoTheme.accentLight)
                                            .clipShape(Circle())
                                        Text("\(ev.maxPowerKw) kW")
                                            .font(.system(size: 10, weight: .bold))
                                            .padding(.horizontal, 5)
                                            .padding(.vertical, 2)
                                            .background(.ultraThinMaterial)
                                            .clipShape(Capsule())
                                    }
                                }
                            }
                        }
                    }
                    .frame(maxWidth: .infinity, maxHeight: .infinity)
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
                        .accessibilityLabel("Centralizar na minha localização")
                    }
                    .padding(16)
                }

                if !listOnlyStations.isEmpty {
                    HStack {
                        Image(systemName: "info.circle")
                            .font(.caption)
                        Text("\(listOnlyStations.count) posto(s) sem pino exato permanecem acessíveis na lista.")
                            .font(.caption2)
                        Spacer()
                        Button("Ver lista") {
                            viewModel.selectedTab = .explore
                        }
                        .font(.caption2.bold())
                    }
                    .padding(.horizontal, 12)
                    .padding(.vertical, 6)
                    .background(Color.secondary.opacity(0.12))
                }
            }
        }
    }

    private var mapToolbar: some View {
        ScrollView(.horizontal, showsIndicators: false) {
            HStack(spacing: 8) {
                Picker("Fonte", selection: $sourceFilter) {
                    Text("Todos").tag("all")
                    Text("Comunidade").tag("community")
                }
                .pickerStyle(.segmented)
                .frame(width: 170)

                Toggle("Postos", isOn: $showCombustionStations)
                    .toggleStyle(.button)
                Toggle("EV", isOn: $showEvSites)
                    .toggleStyle(.button)

                Button {
                    viewModel.activeSheet = .evTripPlanner
                } label: {
                    Label("Rota EV", systemImage: "bolt.horizontal.fill")
                }
                .buttonStyle(.bordered)

                Button {
                    viewModel.activeSheet = .savedPlaces
                } label: {
                    Label("Salvos", systemImage: "bookmark.fill")
                }
                .buttonStyle(.bordered)

                Button {
                    viewModel.activeSheet = .mapDownloads
                } label: {
                    Label("Mapas Offline", systemImage: "arrow.down.circle")
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
                    Section("Destino") {
                        Text(station.name).font(.headline)
                        Text(station.address)
                            .font(.subheadline)
                            .foregroundStyle(.secondary)
                        Button("Calcular rota de carro") { requestCount += 1 }
                            .disabled(isCalculating)
                        if isCalculating {
                            ProgressView("Calculando rota…")
                        } else if let routeMessage {
                            Text(routeMessage).foregroundStyle(.secondary)
                        }
                    }

                    if let route {
                        Section("Prévia da rota") {
                            Text("\((route.distance / 1_000).formatted(.number.precision(.fractionLength(1)))) km")
                            Toggle("Ler instruções em voz alta", isOn: $viewModel.isVoiceGuidanceEnabled)
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

                    Section("Navegação em tempo real") {
                        Button("Abrir no Apple Maps") {
                            let item = destination(for: station)
                            item.openInMaps(launchOptions: [MKLaunchOptionsDirectionsModeKey: MKLaunchOptionsDirectionsModeDriving])
                        }
                        if let wazeUrl = URL(string: "https://waze.com/ul?ll=\(station.latitude),\(station.longitude)&navigate=yes") {
                            Link("Abrir no Waze", destination: wazeUrl)
                        }
                    }
                } else {
                    ContentUnavailableView("Destino sem coordenadas verificadas", systemImage: "location.slash")
                }
            }
            .navigationTitle("Rota para o posto")
            .toolbar {
                ToolbarItem(placement: .cancellationAction) {
                    Button("Fechar") { dismiss() }
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
            routeMessage = "Localização atual indisponível. Você pode abrir o destino em um aplicativo de navegação."
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
                routeMessage = "O Apple Maps não encontrou uma rota para este destino."
            }
        } catch {
            guard !Task.isCancelled else { return }
            routeMessage = "Não foi possível calcular a rota. Sem internet, toque em Abrir no Apple Maps para navegar com a região baixada nos Mapas Offline."
        }
    }
}

public struct EvTripPlannerSheetView: View {
    @Environment(\.dismiss) private var dismiss
    @ObservedObject var viewModel: AppStateViewModel

    @State private var originName = "São Paulo, SP"
    @State private var destinationName = "Curitiba, PR"
    @State private var distanceKm = 408.0
    @State private var batteryKwh = 52.0
    @State private var startSoc = 85.0
    @State private var reserveSoc = 15.0
    @State private var whPerKm = 165.0

    public var body: some View {
        let req = EvTripPlanRequest(
            originName: originName,
            destinationName: destinationName,
            distanceKm: distanceKm,
            batteryKwh: batteryKwh,
            startSocPercent: startSoc,
            reserveMinSocPercent: reserveSoc,
            whPerKm: whPerKm
        )
        let outcome = EvTripPlanner.plan(request: req, sites: viewModel.evSites)
        NavigationStack {
            List {
                Section("Parâmetros da Viagem EV") {
                    TextField("Origem", text: $originName)
                    TextField("Destino", text: $destinationName)
                    HStack {
                        Text("Distância: \(Int(distanceKm)) km")
                        Slider(value: $distanceKm, in: 50...1200, step: 10)
                    }
                    HStack {
                        Text("Bateria: \(Int(batteryKwh)) kWh")
                        Slider(value: $batteryKwh, in: 20...120, step: 2)
                    }
                    HStack {
                        Text("SoC inicial: \(Int(startSoc))%")
                        Slider(value: $startSoc, in: 20...100, step: 5)
                    }
                    HStack {
                        Text("Reserva mín.: \(Int(reserveSoc))%")
                        Slider(value: $reserveSoc, in: 5...30, step: 5)
                    }
                }
                Section("Estimativa de consumo para a distância informada") {
                    Text("Energia total estimada: \(String(format: "%.1f kWh", outcome.totalEnergyKwh))")
                    if !viewModel.evSites.isEmpty {
                        Text("Paradas de recarga: \(outcome.stops.count) · Tempo: \(outcome.totalChargingMinutes) min")
                        Text("Custo estimado de recargas: \(AbastevoTheme.formatCentavos(outcome.totalCostCentavos))")
                            .font(.headline)
                            .foregroundStyle(AbastevoTheme.accentLight)
                        Text("SoC estimado na chegada: \(Int(outcome.arrivalSocPercent))%")
                    } else {
                        Text("Catálogo de recarga indisponível. Paradas, custo e autonomia de chegada ainda não podem ser verificados.")
                            .foregroundStyle(.secondary)
                    }
                }
                if !outcome.stops.isEmpty {
                    Section("Paradas Sugeridas") {
                        ForEach(outcome.stops) { stop in
                            VStack(alignment: .leading, spacing: 4) {
                                Text("\(stop.site.name) (\(stop.site.maxPowerKw) kW)").font(.headline)
                                Text("Km \(Int(stop.arrivalKm)) · +\(String(format: "%.1f kWh", stop.chargeKwh)) · ~\(stop.estimatedMinutes) min · \(AbastevoTheme.formatCentavos(stop.estimatedCostCentavos))")
                                    .font(.caption)
                                    .foregroundStyle(.secondary)
                            }
                        }
                    }
                }
            }
            .navigationTitle("Planejador de Viagem EV")
            .toolbar {
                ToolbarItem(placement: .cancellationAction) {
                    Button("Fechar") { dismiss() }
                }
            }
        }
    }
}

public struct SavedPlacesSheetView: View {
    @Environment(\.dismiss) private var dismiss
    @ObservedObject var viewModel: AppStateViewModel

    public var body: some View {
        NavigationStack {
            List {
                Section("Locais e Postos Salvos") {
                    ForEach(viewModel.snapshot.savedPlaces) { place in
                        VStack(alignment: .leading) {
                            Text(place.title).font(.headline)
                            Text(place.subtitle).font(.caption).foregroundStyle(.secondary)
                        }
                    }
                }
                Section("Viagens Planejadas") {
                    ForEach(viewModel.snapshot.savedTrips) { trip in
                        VStack(alignment: .leading) {
                            Text(trip.title).font(.headline)
                            Text("\(trip.originLabel) → \(trip.destinationLabel) (\(Int(trip.distanceKm)) km)")
                                .font(.caption)
                                .foregroundStyle(.secondary)
                        }
                    }
                }
            }
            .navigationTitle("Salvos & Viagens")
            .toolbar {
                ToolbarItem(placement: .cancellationAction) {
                    Button("Fechar") { dismiss() }
                }
            }
        }
    }
}

import SwiftUI
import MapKit
import AnpFuelCore
#if os(iOS) && canImport(UIKit)
import UIKit
#endif

/// Map Tab + Native Route + EV Trip Planner + Saved Places & Regional Map Downloads.
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
                                if sourceFilter != "community" || st.latestCommunityPrice(for: viewModel.snapshot.selectedFuel) != nil {
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
                    .onAppear { recenterOnCity() }
                    .onChange(of: viewModel.selectedMunicipality.ibgeCode) { _, _ in
                        recenterOnCity()
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
        let price = st.effectiveDisplayPrice(for: fuel)
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

    private func recenterOnCity() {
        let city = viewModel.selectedMunicipality
        cameraPosition = .region(MKCoordinateRegion(
            center: CLLocationCoordinate2D(latitude: city.latitude, longitude: city.longitude),
            span: MKCoordinateSpan(latitudeDelta: 0.05, longitudeDelta: 0.05)
        ))
    }
}

public struct NativeRouteSheetView: View {
    @Environment(\.dismiss) private var dismiss
    @ObservedObject var viewModel: AppStateViewModel
    @State private var avoidTolls = false
    @State private var avoidUnpaved = true
    @State private var currentStepIndex = 0

    public var body: some View {
        let station = viewModel.activeRouteStation ?? viewModel.sortedStations.first
        let city = viewModel.selectedMunicipality
        let steps = [
            "Siga pela via principal em direção a \(station?.neighborhood ?? "Centro").",
            "Vire à direita na \(station?.address ?? "avenida indicada") em 600 metros.",
            "Seu destino \(station?.name ?? "Posto") estará à direita."
        ]
        NavigationStack {
            List {
                if let st = station {
                    let distKm = st.distanceKm(fromLat: city.latitude, fromLon: city.longitude)
                    Section("Destino Selecionado") {
                        Text(st.name).font(.headline)
                        Text("\(st.address) · \(String(format: "%.1f km", max(0.3, distKm)))")
                            .font(.subheadline)
                            .foregroundStyle(.secondary)
                        Toggle("Evitar pedágios", isOn: $avoidTolls)
                        Toggle("Evitar vias sem asfalto", isOn: $avoidUnpaved)
                        Toggle("Guia por voz (AVSpeech)", isOn: $viewModel.isVoiceGuidanceEnabled)
                    }

                    Section("Navegação Passo a Passo") {
                        ForEach(Array(steps.enumerated()), id: \.offset) { idx, text in
                            HStack {
                                Image(systemName: idx == currentStepIndex ? "speaker.wave.2.fill" : "arrow.turn.up.right")
                                    .foregroundStyle(idx == currentStepIndex ? AbastevoTheme.accentLight : .secondary)
                                Text(text).font(.subheadline)
                            }
                            .onTapGesture {
                                currentStepIndex = idx
                                viewModel.speakGuidanceStep(text)
                            }
                        }
                        Button {
                            let msg = steps[currentStepIndex % steps.count]
                            viewModel.speakGuidanceStep(msg)
                            currentStepIndex = (currentStepIndex + 1) % steps.count
                        } label: {
                            Label("Reproduzir instrução de voz", systemImage: "play.circle.fill")
                        }
                    }

                    Section("Abrir em Navegador Externo") {
                        Button("Abrir no Apple Maps") {
                            let item = MKMapItem(placemark: MKPlacemark(coordinate: CLLocationCoordinate2D(latitude: st.latitude, longitude: st.longitude)))
                            item.name = st.name
                            item.openInMaps(launchOptions: [MKLaunchOptionsDirectionsModeKey: MKLaunchOptionsDirectionsModeDriving])
                        }
                        if let wazeUrl = URL(string: "https://waze.com/ul?ll=\(st.latitude),\(st.longitude)&navigate=yes") {
                            Link("Abrir no Waze", destination: wazeUrl)
                        }
                    }
                }
            }
            .navigationTitle("Navegação Nativa")
            .toolbar {
                ToolbarItem(placement: .cancellationAction) {
                    Button("Fechar") { dismiss() }
                }
            }
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
                Section("Resumo do Planejamento") {
                    Text("Energia total estimada: \(String(format: "%.1f kWh", outcome.totalEnergyKwh))")
                    Text("Paradas de recarga: \(outcome.stops.count) · Tempo: \(outcome.totalChargingMinutes) min")
                    Text("Custo estimado de recargas: \(AbastevoTheme.formatCentavos(outcome.totalCostCentavos))")
                        .font(.headline)
                        .foregroundStyle(AbastevoTheme.accentLight)
                    Text("SoC estimado na chegada: \(Int(outcome.arrivalSocPercent))%")
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

public struct MapDownloadsSheetView: View {
    @Environment(\.dismiss) private var dismiss
    @ObservedObject var viewModel: AppStateViewModel

    public var body: some View {
        NavigationStack {
            List(SeedCatalog.mapRegions) { pkg in
                let isDownloaded = viewModel.snapshot.downloadedMapRegions.contains(pkg.id)
                HStack {
                    VStack(alignment: .leading, spacing: 4) {
                        Text(pkg.regionName).font(.headline)
                        Text("UFs: \(pkg.statesCovered) · \(pkg.sizeMegabytes) MB · SHA-256 \(pkg.sha256Prefix)…")
                            .font(.caption)
                            .foregroundStyle(.secondary)
                    }
                    Spacer()
                    Button(isDownloaded ? "Remover" : "Baixar") {
                        viewModel.toggleDownloadMapRegion(pkg)
                    }
                    .buttonStyle(.bordered)
                    .tint(isDownloaded ? .red : AbastevoTheme.accentLight)
                }
                .padding(.vertical, 4)
            }
            .navigationTitle("Mapas Offline por Região")
            .toolbar {
                ToolbarItem(placement: .cancellationAction) {
                    Button("Fechar") { dismiss() }
                }
            }
        }
    }
}

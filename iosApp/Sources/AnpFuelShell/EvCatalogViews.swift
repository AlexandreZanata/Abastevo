import SwiftUI
import MapKit
import AnpFuelCore

/// Admitted EV charging catalog: Explore list, filters, place detail and the trip planner.
public struct EvChargingExploreSheetView: View {
    @Environment(\.dismiss) private var dismiss
    @ObservedObject var viewModel: AppStateViewModel

    public var body: some View {
        NavigationStack {
            EvExploreContent(app: viewModel, ev: viewModel.evCatalog)
                .navigationTitle("Recarga Elétrica (EV)")
                .toolbar {
                    ToolbarItem(placement: .cancellationAction) {
                        Button("Fechar") { dismiss() }
                    }
                }
        }
    }
}

struct EvExploreContent: View {
    @ObservedObject var app: AppStateViewModel
    @ObservedObject var ev: EvCatalogModel
    @State private var visibleLimit = 100

    private let powerOptions = [0, 7, 22, 50, 150]

    var body: some View {
        let near = app.lastDeviceCoordinate.map { EvCoordinate(latitude: $0.latitude, longitude: $0.longitude) }
        let places = ev.filteredPlaces(city: app.selectedMunicipality, near: near)
        List {
            Section("Catálogo") {
                EvCatalogStatusView(ev: ev)
            }
            Section("Filtros") {
                Picker("Abrangência", selection: $ev.scope) {
                    ForEach(EvCatalogScope.allCases) { scope in
                        Text(scope.label).tag(scope)
                    }
                }
                .pickerStyle(.segmented)
                Picker("Conector", selection: $ev.filter.connector) {
                    Text("Todos").tag(ChargingConnector?.none)
                    ForEach(ChargingConnector.allCases) { connector in
                        Text(connector.label).tag(ChargingConnector?.some(connector))
                    }
                }
                Picker("Corrente", selection: $ev.filter.current) {
                    Text("Todas").tag(ChargingCurrent?.none)
                    Text("AC").tag(ChargingCurrent?.some(.ac))
                    Text("DC").tag(ChargingCurrent?.some(.dc))
                }
                Picker("Potência mínima", selection: Binding(
                    get: { ev.filter.minPowerKw ?? 0 },
                    set: { ev.filter.minPowerKw = $0 > 0 ? $0 : nil }
                )) {
                    ForEach(powerOptions, id: \.self) { kw in
                        Text(kw == 0 ? String(localized: "Qualquer") : "\(kw) kW").tag(kw)
                    }
                }
                Toggle("Somente acesso público", isOn: $ev.filter.publicOnly)
                TextField("Buscar por nome, operador ou endereço", text: $ev.filter.query)
            }
            Section(String(localized: "Pontos de recarga (\(places.count))")) {
                if ev.isLoading {
                    ProgressView()
                } else if ev.snapshot == nil {
                    Text("Catálogo de recarga ainda não baixado neste aparelho. Toque em Atualizar com conexão.")
                        .foregroundStyle(.secondary)
                } else if places.isEmpty {
                    Text("Nenhum ponto de recarga para estes filtros.")
                        .foregroundStyle(.secondary)
                }
                ForEach(places.prefix(visibleLimit)) { place in
                    NavigationLink {
                        EvPlaceDetailView(place: place, snapshot: ev.snapshot, vehicleConnector: ev.filter.connector)
                    } label: {
                        EvPlaceRow(place: place, near: near, connector: ev.filter.connector)
                    }
                }
                if places.count > visibleLimit {
                    Button("Mostrar mais") { visibleLimit += 100 }
                }
            }
        }
        .onAppear { ev.loadIfNeeded() }
    }
}

struct EvCatalogStatusView: View {
    @ObservedObject var ev: EvCatalogModel

    var body: some View {
        VStack(alignment: .leading, spacing: 6) {
            if let snapshot = ev.snapshot {
                Text(snapshot.profile == EvCatalogService.nationalProfile ? "Catálogo nacional de recarga" : "Catálogo Open Charge Map")
                    .font(.subheadline.bold())
                Text("\(snapshot.places.count) locais · revisão \(String(snapshot.sourceRevision.prefix(12)))")
                    .font(.caption)
                    .foregroundStyle(.secondary)
                if let savedAt = ev.savedAt {
                    Text("Salvo \(AnpWeekText.age(since: savedAt)) · \(ev.age == .stale ? String(localized: "desatualizado") : String(localized: "recente"))")
                        .font(.caption)
                        .foregroundStyle(ev.age == .stale ? Color.orange : Color.secondary)
                }
                Text("Fonte: \(snapshot.license). Dados de referência; disponibilidade em tempo real não é informada.")
                    .font(.caption2)
                    .foregroundStyle(.secondary)
            } else {
                Text("Nenhum catálogo salvo.")
                    .font(.subheadline)
            }
            if let progress = ev.refreshProgress {
                HStack {
                    if let expected = progress.expectedPages, expected > 0 {
                        ProgressView(value: Double(min(progress.pages, expected)), total: Double(expected))
                    } else {
                        ProgressView()
                    }
                    Text("Página \(progress.pages)")
                        .font(.caption)
                    Button("Cancelar") { ev.cancelRefresh() }
                        .font(.caption)
                }
            } else {
                Button {
                    ev.refresh()
                } label: {
                    Label("Atualizar catálogo", systemImage: "arrow.clockwise")
                }
            }
            if let message = ev.errorMessage {
                Text(message)
                    .font(.caption)
                    .foregroundStyle(.orange)
            }
        }
    }
}

struct EvPlaceRow: View {
    let place: ChargingPlace
    let near: EvCoordinate?
    let connector: ChargingConnector?

    var body: some View {
        VStack(alignment: .leading, spacing: 4) {
            HStack {
                Text(place.displayName)
                    .font(.headline)
                Spacer()
                if let power = place.maxPowerW {
                    Text(EvText.power(power))
                        .font(.caption.bold())
                        .padding(.horizontal, 8)
                        .padding(.vertical, 3)
                        .background(AbastevoTheme.accentLight.opacity(0.16))
                        .clipShape(Capsule())
                }
            }
            if let operatorName = place.operatorName {
                Text(operatorName)
                    .font(.caption)
                    .foregroundStyle(.secondary)
            }
            if let address = place.addressLine {
                Text(address)
                    .font(.caption2)
                    .foregroundStyle(.secondary)
            }
            HStack(spacing: 8) {
                Text(place.connectors.isEmpty ? String(localized: "Conectores não informados") : place.connectors.map(\.label).joined(separator: ", "))
                    .font(.caption)
                if let near, let coordinate = place.coordinate {
                    Text(EvText.distance(ChargingRules.distanceMeters(near, coordinate)))
                        .font(.caption)
                        .foregroundStyle(.secondary)
                }
                if place.coordinate == nil {
                    Text("Sem coordenada admitida")
                        .font(.caption2)
                        .foregroundStyle(.secondary)
                }
            }
            if let connector {
                EvCompatibilityBadge(verdict: ChargingRules.assess(vehicleConnector: connector, groups: place.groups))
            }
        }
        .padding(.vertical, 2)
    }
}

struct EvCompatibilityBadge: View {
    let verdict: ChargingCompatibilityVerdict

    var body: some View {
        switch verdict {
        case .compatible:
            Label("Compatível com o conector escolhido", systemImage: "checkmark.circle")
                .font(.caption2)
                .foregroundStyle(AbastevoTheme.accentLight)
        case .unknown:
            Label("Compatibilidade desconhecida", systemImage: "questionmark.circle")
                .font(.caption2)
                .foregroundStyle(.secondary)
        case .incompatible:
            Label("Conector incompatível", systemImage: "xmark.circle")
                .font(.caption2)
                .foregroundStyle(.orange)
        }
    }
}

struct EvPlaceDetailView: View {
    let place: ChargingPlace
    let snapshot: EvCatalogSnapshot?
    let vehicleConnector: ChargingConnector?

    var body: some View {
        List {
            Section {
                Text(place.displayName).font(.title3.bold())
                if let operatorName = place.operatorName {
                    Text("Operador: \(operatorName)")
                }
                if let address = place.addressLine {
                    Text(address).foregroundStyle(.secondary)
                }
                Text(place.publicAccess == .publicAccess ? "Acesso público informado" : "Acesso não confirmado")
                    .font(.caption)
                if place.confirmedByTwoSources {
                    Label("Confirmado por duas fontes (OSM e OCM)", systemImage: "checkmark.seal")
                        .font(.caption)
                }
                if let coordinate = place.coordinate {
                    Text("Coordenada admitida: \(EvText.coordinate(coordinate))")
                        .font(.caption)
                        .foregroundStyle(.secondary)
                    Button("Abrir no Apple Maps") {
                        let item = MKMapItem(placemark: MKPlacemark(coordinate: CLLocationCoordinate2D(latitude: coordinate.latitude, longitude: coordinate.longitude)))
                        item.name = place.displayName
                        item.openInMaps(launchOptions: [MKLaunchOptionsDirectionsModeKey: MKLaunchOptionsDirectionsModeDriving])
                    }
                } else {
                    Text("Sem coordenada admitida: este local não aparece no mapa nem em rotas.")
                        .font(.caption)
                        .foregroundStyle(.secondary)
                }
            }
            Section("Conectores (\(place.groups.count) grupos · \(place.reportedPoints) pontos informados)") {
                if place.groups.isEmpty {
                    Text("Conectores não informados pela fonte.").foregroundStyle(.secondary)
                }
                ForEach(place.groups) { group in
                    VStack(alignment: .leading, spacing: 2) {
                        Text("\(group.connector.label) · \(group.current.rawValue)")
                            .font(.subheadline.bold())
                        Text(group.powerW.map(EvText.power) ?? String(localized: "Potência não informada"))
                            .font(.caption)
                        if let quantity = group.reportedQuantity {
                            Text("Quantidade informada: \(quantity)").font(.caption2)
                        }
                        if group.connector == .unknown {
                            Text("Código da fonte: \(group.rawConnectorCode)")
                                .font(.caption2)
                                .foregroundStyle(.secondary)
                        }
                    }
                }
                if let vehicleConnector {
                    EvCompatibilityBadge(verdict: ChargingRules.assess(vehicleConnector: vehicleConnector, groups: place.groups))
                }
            }
            Section("Fontes e licenças") {
                ForEach(Array(place.sources.enumerated()), id: \.offset) { _, source in
                    VStack(alignment: .leading, spacing: 2) {
                        Text("\(source.source) · \(source.externalId)").font(.caption.bold())
                        Text(source.attribution).font(.caption2)
                        Text(source.license).font(.caption2).foregroundStyle(.secondary)
                    }
                }
                if let snapshot {
                    Text("Catálogo \(snapshot.profile), revisão \(String(snapshot.sourceRevision.prefix(12))), salvo \(AnpWeekText.age(since: Date(timeIntervalSince1970: TimeInterval(snapshot.savedAtEpochSeconds)))).")
                        .font(.caption2)
                        .foregroundStyle(.secondary)
                }
                Text("Disponibilidade, horário e tarifa não fazem parte do catálogo.")
                    .font(.caption2)
                    .foregroundStyle(.secondary)
            }
        }
        .navigationTitle("Ponto de recarga")
    }
}

/// Map pin detail sheet.
struct EvPlaceDetailSheet: View {
    @Environment(\.dismiss) private var dismiss
    let place: ChargingPlace
    let snapshot: EvCatalogSnapshot?

    var body: some View {
        NavigationStack {
            EvPlaceDetailView(place: place, snapshot: snapshot, vehicleConnector: nil)
                .toolbar {
                    ToolbarItem(placement: .cancellationAction) {
                        Button("Fechar") { dismiss() }
                    }
                }
        }
    }
}

enum EvText {
    static func power(_ watts: Int64) -> String {
        let kw = Double(watts) / 1_000.0
        return kw >= 10 ? "\(Int(kw.rounded())) kW" : "\(kw.formatted(.number.precision(.fractionLength(1)))) kW"
    }

    static func distance(_ meters: Double) -> String {
        meters >= 1_000 ? "\((meters / 1_000).formatted(.number.precision(.fractionLength(1)))) km" : "\(Int(meters.rounded())) m"
    }

    static func coordinate(_ value: EvCoordinate) -> String {
        "\(value.latitude.formatted(.number.precision(.fractionLength(5)))), \(value.longitude.formatted(.number.precision(.fractionLength(5))))"
    }

    static func percent(_ basisPoints: Int64) -> String {
        "\((Double(basisPoints) / 100.0).formatted(.number.precision(.fractionLength(0...1))))%"
    }

    static func energy(_ wattHours: Int64) -> String {
        "\((Double(wattHours) / 1_000.0).formatted(.number.precision(.fractionLength(1)))) kWh"
    }

    static func duration(_ seconds: Int64) -> String {
        let minutes = max(1, (seconds + 59) / 60)
        return minutes >= 60 ? "\(minutes / 60) h \(minutes % 60) min" : "\(minutes) min"
    }

    static func issue(_ code: EvIssueCode) -> String {
        switch code {
        case .snapshotMissing: return String(localized: "Catálogo de recarga não baixado.")
        case .coverageUnknown: return String(localized: "Cobertura do catálogo desconhecida.")
        case .noCompatibleCandidate: return String(localized: "Nenhum carregador compatível ao longo da rota.")
        case .noRoadRoute: return String(localized: "Rota rodoviária indisponível.")
        case .routeBudgetExhausted: return String(localized: "Limite de consultas de rota atingido.")
        case .roadWarning: return String(localized: "A rota tem avisos do Apple Maps.")
        case .accessUnknown: return String(localized: "Acesso público não confirmado.")
        case .hoursUnknown: return String(localized: "Horário de funcionamento desconhecido.")
        case .powerUnknown: return String(localized: "Potência não informada.")
        case .tariffUnknown: return String(localized: "Tarifa desconhecida: custo não estimado.")
        case .stopUnreachable: return String(localized: "Algum carregador ficou fora do alcance com a reserva.")
        case .destinationUnreachable: return String(localized: "Destino fora do alcance com os carregadores disponíveis.")
        case .stopLimitReached: return String(localized: "Limite de paradas atingido.")
        case .noEvidencedAlternative: return String(localized: "Sem alternativa comprovada se a parada falhar.")
        case .vehicleConnectorMissing: return String(localized: "Escolha o conector do veículo.")
        case .batteryMissing, .invalidBattery: return String(localized: "Informe a bateria útil do veículo.")
        case .consumptionMissing, .invalidConsumption: return String(localized: "Informe um consumo válido.")
        case .invalidTargetSoc: return String(localized: "A meta de carga deve ser maior que a reserva.")
        case .invalidRoute: return String(localized: "Origem e destino inválidos.")
        case .energyUnknown: return String(localized: "Energia do trecho desconhecida.")
        default: return code.rawValue
        }
    }

    static func status(_ status: EvPlanStatus) -> String {
        switch status {
        case .planned: return String(localized: "Plano calculado")
        case .invalidInput: return String(localized: "Dados inválidos")
        case .noRoute: return String(localized: "Sem rota rodoviária")
        case .insufficientData: return String(localized: "Dados insuficientes")
        case .noCandidates: return String(localized: "Sem carregadores compatíveis")
        case .insufficientEnergy: return String(localized: "Energia insuficiente para o destino")
        case .budgetExhausted: return String(localized: "Limite de consultas atingido")
        case .cancelled: return String(localized: "Cancelado")
        }
    }
}

/// MAP-28 owner-declared inputs over the MAP-27 planner. Routes come from MapKit
/// (online); chargers come only from the admitted catalog snapshot.
public struct EvTripPlannerSheetView: View {
    @Environment(\.dismiss) private var dismiss
    @ObservedObject var viewModel: AppStateViewModel

    public var body: some View {
        NavigationStack {
            EvPlannerContent(app: viewModel, ev: viewModel.evCatalog)
                .navigationTitle("Planejador de Viagem EV")
                .toolbar {
                    ToolbarItem(placement: .cancellationAction) {
                        Button("Fechar") { dismiss() }
                    }
                }
        }
    }
}

struct EvPlannerContent: View {
    @ObservedObject var app: AppStateViewModel
    @ObservedObject var ev: EvCatalogModel

    @AppStorage("ev-planner-connector") private var connectorRaw = ChargingConnector.ccs2.rawValue
    @AppStorage("ev-planner-soc") private var socText = "30"
    @AppStorage("ev-planner-reserve") private var reserveText = "10"
    @AppStorage("ev-planner-target") private var targetText = "80"
    @AppStorage("ev-planner-consumption") private var consumptionText = "180"
    @AppStorage("ev-planner-phev-share") private var phevShareText = "100"
    @State private var vehicleId = ""
    @State private var manualBatteryText = ""
    @State private var destinationText = ""
    @State private var originText = ""
    @State private var useCurrentLocation = true
    @State private var isPlanning = false
    @State private var plan: EvTripPlan?
    @State private var lastInput: EvTripInput?
    @State private var message: String?
    @State private var planTask: Task<Void, Never>?

    private var electricVehicles: [VehicleRecord] {
        app.snapshot.vehicles.filter { $0.propulsion.supportsElectric && $0.batteryCapacityWh > 0 }
    }

    private var selectedVehicle: VehicleRecord? {
        electricVehicles.first { $0.id == vehicleId } ?? electricVehicles.first
    }

    var body: some View {
        List {
            Section("Veículo") {
                if electricVehicles.isEmpty {
                    TextField("Bateria útil (kWh)", text: $manualBatteryText)
                        .decimalKeyboard()
                    Text("Cadastre um veículo elétrico no Perfil para usar a bateria dele.")
                        .font(.caption)
                        .foregroundStyle(.secondary)
                } else {
                    Picker("Veículo", selection: $vehicleId) {
                        ForEach(electricVehicles) { vehicle in
                            Text(vehicle.name).tag(vehicle.id)
                        }
                    }
                    if let vehicle = selectedVehicle {
                        Text("Bateria útil: \(EvText.energy(vehicle.batteryCapacityWh))")
                            .font(.caption)
                        if vehicle.propulsion == .plugInHybrid {
                            TextField("Parcela elétrica da bateria (%)", text: $phevShareText)
                                .decimalKeyboard()
                        }
                    }
                }
                Picker("Conector do veículo", selection: $connectorRaw) {
                    ForEach(ChargingConnector.vehicleChoices) { connector in
                        Text(connector.label).tag(connector.rawValue)
                    }
                }
            }
            Section("Energia") {
                LabeledContent("Carga atual (%)") {
                    TextField("30", text: $socText).decimalKeyboard().multilineTextAlignment(.trailing)
                }
                LabeledContent("Reserva mínima (%)") {
                    TextField("10", text: $reserveText).decimalKeyboard().multilineTextAlignment(.trailing)
                }
                LabeledContent("Meta de carga (%)") {
                    TextField("80", text: $targetText).decimalKeyboard().multilineTextAlignment(.trailing)
                }
                LabeledContent("Consumo (Wh/km)") {
                    TextField("180", text: $consumptionText).decimalKeyboard().multilineTextAlignment(.trailing)
                }
            }
            Section("Rota") {
                Toggle("Partir da minha localização", isOn: $useCurrentLocation)
                if !useCurrentLocation {
                    TextField("Origem (cidade ou endereço)", text: $originText)
                }
                TextField("Destino (cidade ou endereço)", text: $destinationText)
                if isPlanning {
                    HStack {
                        ProgressView()
                        Text("Calculando com o Apple Maps…")
                        Spacer()
                        Button("Cancelar") { planTask?.cancel() }
                    }
                } else {
                    Button("Planejar viagem") { startPlan(failures: []) }
                        .disabled(destinationText.trimmingCharacters(in: .whitespacesAndNewlines).isEmpty)
                }
                if ev.snapshot == nil {
                    Text("Baixe o catálogo de recarga em Explorar › Recarga EV para considerar paradas.")
                        .font(.caption)
                        .foregroundStyle(.secondary)
                }
                if let message {
                    Text(message).font(.caption).foregroundStyle(.orange)
                }
            }
            if let plan {
                planSections(plan)
            }
            Section {
                Text("Estimativa conservadora sem trânsito, relevo ou disponibilidade em tempo real. O plano nunca garante a viagem.")
                    .font(.caption2)
                    .foregroundStyle(.secondary)
            }
        }
        .onAppear {
            ev.loadIfNeeded()
            if vehicleId.isEmpty { vehicleId = electricVehicles.first?.id ?? "" }
        }
        .onDisappear { planTask?.cancel() }
    }

    @ViewBuilder
    private func planSections(_ plan: EvTripPlan) -> some View {
        Section("Resultado: \(EvText.status(plan.status))") {
            if let distance = plan.mainRoute?.road.distanceMeters {
                Text("Distância rodoviária: \(EvText.distance(Double(distance)))")
            }
            if let energy = plan.mainRoute?.energyWh.value {
                Text("Energia estimada do trajeto: \(EvText.energy(energy))")
            }
            if let arrival = plan.arrivalSocBp?.value {
                Text("Carga estimada na chegada: \(EvText.percent(arrival))")
                    .font(.headline)
            }
            Text("Paradas: \(plan.stops.count) · Carregadores avaliados: \(plan.candidateCount) · Consultas de rota: \(plan.routeCalls)")
                .font(.caption)
                .foregroundStyle(.secondary)
            if plan.scope == .electricPortionOnly {
                Text("Considera apenas a parcela elétrica da bateria do híbrido plug-in.")
                    .font(.caption2)
            }
        }
        if !plan.stops.isEmpty {
            Section("Paradas sugeridas") {
                ForEach(Array(plan.stops.enumerated()), id: \.offset) { index, stop in
                    VStack(alignment: .leading, spacing: 4) {
                        Text("\(index + 1). \(stop.candidate.siteName)").font(.headline)
                        Text("\(stop.candidate.connector.label) · \(stop.candidate.powerW.map(EvText.power) ?? String(localized: "potência não informada"))")
                            .font(.caption)
                        Text("Chegada com \(EvText.percent(stop.arrivalSocBp)) · carregar \(EvText.energy(stop.chargeEnergyWh)) até \(EvText.percent(stop.targetSocBp))")
                            .font(.caption)
                        if let seconds = stop.minimumChargeSeconds.value {
                            Text("Tempo mínimo de recarga: \(EvText.duration(seconds))").font(.caption)
                        }
                        if let alternative = stop.alternative {
                            Text("Alternativa próxima: \(alternative.candidate.siteName)")
                                .font(.caption2)
                                .foregroundStyle(.secondary)
                        }
                    }
                }
                Button("A primeira parada falhou: replanejar sem ela") { replanWithoutFirstStop(plan) }
            }
        }
        let codes = uniqueCodes(plan.issues)
        if !codes.isEmpty {
            Section("Avisos") {
                ForEach(codes, id: \.self) { code in
                    Text(EvText.issue(code)).font(.caption)
                }
            }
        }
    }

    private func uniqueCodes(_ issues: [EvIssue]) -> [EvIssueCode] {
        var seen: [EvIssueCode] = []
        for issue in issues where !seen.contains(issue.code) { seen.append(issue.code) }
        return seen
    }

    private func batteryInput() -> EvBatteryInput? {
        let capacity: Int64?
        var portion: Int64?
        if let vehicle = selectedVehicle {
            capacity = vehicle.batteryCapacityWh
            if vehicle.propulsion == .plugInHybrid {
                guard let share = EvPlannerInputParser.percentBp(phevShareText, min: 1, max: 100) else { return nil }
                portion = max(1, share / 10)
            }
        } else {
            capacity = try? PortableMoney.parseBatteryCapacityWh(manualBatteryText)
        }
        guard let soc = EvPlannerInputParser.percentBp(socText, min: 0, max: 100),
              let reserve = EvPlannerInputParser.percentBp(reserveText, min: 0, max: 100) else { return nil }
        return EvBatteryInput(capacityWh: capacity, socBp: soc, reserveBp: reserve, usablePermille: portion)
    }

    private func startPlan(failures: [EvFailure]) {
        guard let battery = batteryInput() else {
            message = String(localized: "Revise bateria, carga atual e reserva (0 a 100%).")
            return
        }
        guard let target = EvPlannerInputParser.percentBp(targetText, min: 0, max: 100) else {
            message = String(localized: "Meta de carga inválida (0 a 100%).")
            return
        }
        guard let consumption = EvPlannerInputParser.consumptionMilli(consumptionText) else {
            message = String(localized: "Consumo inválido. Use Wh/km, por exemplo 180.")
            return
        }
        let connector = ChargingConnector(rawValue: connectorRaw)
        let destinationQuery = destinationText
        let originQuery = useCurrentLocation ? nil : originText
        message = nil
        plan = nil
        isPlanning = true
        planTask?.cancel()
        planTask = Task {
            defer { isPlanning = false }
            guard let origin = await resolveOrigin(originQuery) else {
                message = String(localized: "Origem indisponível. Permita a localização ou digite uma origem.")
                return
            }
            guard let destination = await search(destinationQuery) else {
                message = String(localized: "Destino não encontrado pelo Apple Maps.")
                return
            }
            var input = EvTripInput(
                origin: origin, destination: destination, departureAt: Date(), battery: battery,
                consumption: EvConsumptionInput(consMilliWhPerKm: consumption), vehicleConnector: connector,
                chargeTargetSocBp: target
            )
            input.failures = failures
            lastInput = input
            let planner = EvTripPlanner(router: MapKitEvRouter())
            let source = ev.candidateSource(connector: connector ?? .unknown)
            let result = await planner.plan(input, candidates: source)
            guard !Task.isCancelled else { return }
            plan = result
        }
    }

    /// The failed charger gives no energy: replan from its arrival charge (Android `markPrimaryFailed`).
    private func replanWithoutFirstStop(_ current: EvTripPlan) {
        guard let stop = current.stops.first, var input = lastInput else { return }
        let failurePoint = EvRoutePoint(latitude: stop.candidate.point.latitude, longitude: stop.candidate.point.longitude, label: stop.candidate.siteName)
        input.initialPoint = failurePoint
        input.initialSocBp = stop.arrivalSocBp
        input.failures.append(EvFailure(scope: .site, siteId: stop.candidate.siteId))
        input.departureAt = Date()
        let source = ev.candidateSource(connector: input.vehicleConnector ?? .unknown)
        let next = input
        lastInput = next
        plan = nil
        isPlanning = true
        planTask?.cancel()
        planTask = Task {
            defer { isPlanning = false }
            let result = await EvTripPlanner(router: MapKitEvRouter()).plan(next, candidates: source)
            guard !Task.isCancelled else { return }
            plan = result
        }
    }

    private func resolveOrigin(_ query: String?) async -> EvRoutePoint? {
        if let query {
            return await search(query)
        }
        guard let coordinate = await app.requestRouteOrigin() else { return nil }
        return EvRoutePoint(latitude: coordinate.latitude, longitude: coordinate.longitude, label: String(localized: "Minha localização"))
    }

    private func search(_ text: String) async -> EvRoutePoint? {
        let query = text.trimmingCharacters(in: .whitespacesAndNewlines)
        guard !query.isEmpty, !app.isOfflineSimulation else { return nil }
        let request = MKLocalSearch.Request()
        request.naturalLanguageQuery = query
        guard let response = try? await MKLocalSearch(request: request).start(),
              let item = response.mapItems.first else { return nil }
        let coordinate = item.placemark.coordinate
        guard CLLocationCoordinate2DIsValid(coordinate) else { return nil }
        return EvRoutePoint(latitude: coordinate.latitude, longitude: coordinate.longitude, label: item.name ?? query)
    }
}

extension View {
    /// Decimal keypad on iPhone and iPad; a no-op where the modifier does not exist.
    @ViewBuilder
    func decimalKeyboard() -> some View {
        #if os(iOS)
        self.keyboardType(.decimalPad)
        #else
        self
        #endif
    }
}

import SwiftUI
import AnpFuelCore

/// Explore Tab + Station Detail + Price History + EV Explore + Location/Week Pickers + Quality/Suggest/FAQ.
public struct ExploreRootView: View {
    @ObservedObject var viewModel: AppStateViewModel
    @State private var tankLitersDouble: Double = 50.0
    @State private var searchStationText: String = ""

    public var body: some View {
        NavigationStack(path: $viewModel.explorePath) {
            ScrollView {
                VStack(alignment: .leading, spacing: 14) {
                    FuelProductChipsBar(
                        selectedFuel: viewModel.snapshot.selectedFuel,
                        locale: viewModel.currentLocale,
                        onSelect: { viewModel.selectFuel($0) }
                    )

                    if let avg = viewModel.officialAverage {
                        officialAnpCard(avg: avg)
                            .padding(.horizontal, 16)
                    }

                    communitySummaryCard
                        .padding(.horizontal, 16)

                    tankFillCalculatorCard
                        .padding(.horizontal, 16)

                    discoveryShortcutsGrid
                        .padding(.horizontal, 16)

                    stationListSection
                        .padding(.horizontal, 16)
                }
                .padding(.vertical, 12)
            }
            .refreshable {
                viewModel.reloadCityData()
            }
        }
    }

    private func officialAnpCard(avg: OfficialAnpAverage) -> some View {
        EnergiaCard {
            HStack {
                SourceTrustBadge(isOfficial: true, label: "ANP Oficial")
                Spacer()
                Text(avg.surveyDateLabel)
                    .font(.caption)
                    .foregroundStyle(.secondary)
            }
            Text("Média oficial em \(viewModel.selectedMunicipality.displayTitle)")
                .font(.subheadline.bold())
            HStack(alignment: .firstTextBaseline, spacing: 16) {
                VStack(alignment: .leading) {
                    Text("Média")
                        .font(.caption)
                        .foregroundStyle(.secondary)
                    Text(AbastevoTheme.formatBrlMilli(avg.meanMilli))
                        .font(.title.bold())
                }
                VStack(alignment: .leading) {
                    Text("Mínimo")
                        .font(.caption)
                        .foregroundStyle(.secondary)
                    Text(AbastevoTheme.formatBrlMilli(avg.minMilli))
                        .font(.headline)
                        .foregroundStyle(AbastevoTheme.accentLight)
                }
                VStack(alignment: .leading) {
                    Text("Máximo")
                        .font(.caption)
                        .foregroundStyle(.secondary)
                    Text(AbastevoTheme.formatBrlMilli(avg.maxMilli))
                        .font(.headline)
                }
                Spacer()
            }
            HStack {
                Text("\(avg.stationCount) postos na pesquisa semanal")
                    .font(.caption)
                    .foregroundStyle(.secondary)
                Spacer()
                if let url = URL(string: OfficialAnpAverage.anpOfficialSourceUrl) {
                    Link("Fonte gov.br/anp", destination: url)
                        .font(.caption.bold())
                }
            }
        }
    }

    private var communitySummaryCard: some View {
        let feed = viewModel.visibleCommunityFeed
        let latest = feed.first
        return EnergiaCard {
            HStack {
                SourceTrustBadge(isOfficial: false, label: "Comunidade")
                Spacer()
                Button {
                    viewModel.activeSheet = .capturePrice
                } label: {
                    Label("Enviar foto", systemImage: "camera.fill")
                        .font(.caption.bold())
                }
                .buttonStyle(.borderedProminent)
                .tint(AbastevoTheme.accentLight)
            }
            if let latest = latest {
                Text("Menor/último relato recente: \(AbastevoTheme.formatBrlMilli(latest.amountMilli))")
                    .font(.headline)
                Text("\(latest.stationName) · \(latest.condition.labelPtBr) · \(latest.confirmations) confirmações")
                    .font(.caption)
                    .foregroundStyle(.secondary)
            } else {
                Text("Sem preços comunitários para este combustível nesta semana")
                    .font(.subheadline)
                    .foregroundStyle(.secondary)
            }
        }
    }

    private var tankFillCalculatorCard: some View {
        let fuel = viewModel.snapshot.selectedFuel
        let stationPrices: [StationPrice] = viewModel.sortedStations.compactMap { st in
            guard let p = st.effectiveDisplayPrice(for: fuel) else { return nil }
            return StationPrice(station: st.name, unitMilli: p)
        }
        let ml = Int64(tankLitersDouble * 1000.0)
        let cheapestFill = try? TankFillUseCase.fillCheapest(prices: stationPrices, capacityMilliLiters: ml)
        let gasPrice = viewModel.sortedStations.compactMap { $0.effectiveDisplayPrice(for: .gasolineRegular) }.min()
        let ethPrice = viewModel.sortedStations.compactMap { $0.effectiveDisplayPrice(for: .ethanol) }.min()

        return EnergiaCard {
            HStack {
                Text("Simulador de Tanque (\(Int(tankLitersDouble)) \(fuel.shortUnitLabel))")
                    .font(.subheadline.bold())
                Spacer()
                Stepper("", value: $tankLitersDouble, in: 10...120, step: 5)
                    .labelsHidden()
            }
            if let fill = cheapestFill {
                HStack {
                    VStack(alignment: .leading, spacing: 2) {
                        Text(fill.station)
                            .font(.subheadline.weight(.semibold))
                        Text("Total exato no menor preço (\(AbastevoTheme.formatBrlMilli(fill.unitMilli))/\(fuel.shortUnitLabel))")
                            .font(.caption)
                            .foregroundStyle(.secondary)
                    }
                    Spacer()
                    Text(AbastevoTheme.formatBrlMilli(fill.totalMilli))
                        .font(.title3.bold())
                }
            }
            if let gp = gasPrice, let ep = ethPrice, gp > 0 {
                let ratio = Double(ep) / Double(gp)
                let percent = Int((ratio * 100).rounded())
                let ethanolAdvantage = ratio <= 0.70
                HStack(spacing: 6) {
                    Image(systemName: ethanolAdvantage ? "leaf.fill" : "fuelpump.fill")
                        .foregroundStyle(ethanolAdvantage ? AbastevoTheme.accentLight : .orange)
                    Text("Etanol/Gasolina: \(percent)% — \(ethanolAdvantage ? "Etanol compensa pela regra dos 70%" : "Gasolina tende a compensar pela regra dos 70%")")
                        .font(.caption)
                        .foregroundStyle(.secondary)
                }
            }
        }
    }

    private var discoveryShortcutsGrid: some View {
        LazyVGrid(columns: [GridItem(.flexible()), GridItem(.flexible()), GridItem(.flexible())], spacing: 10) {
            shortcutButton(title: "Histórico", icon: "chart.line.uptrend.xyaxis") {
                viewModel.activeSheet = .priceHistory
            }
            shortcutButton(title: "Recarga EV", icon: "bolt.car.fill") {
                viewModel.activeSheet = .evExplore
            }
            shortcutButton(title: "Qualidade", icon: "flask.fill") {
                viewModel.activeSheet = .qualityReports
            }
            shortcutButton(title: "Sugerir Posto", icon: "plus.circle.fill") {
                viewModel.activeSheet = .suggestStation
            }
            shortcutButton(title: "Perguntas", icon: "questionmark.circle.fill") {
                viewModel.activeSheet = .masterFaq
            }
            shortcutButton(title: "Rotas EV", icon: "map.fill") {
                viewModel.activeSheet = .evTripPlanner
            }
        }
    }

    private func shortcutButton(title: String, icon: String, action: @escaping () -> Void) -> some View {
        Button {
            IosHaptics.lightTap()
            action()
        } label: {
            VStack(spacing: 6) {
                Image(systemName: icon)
                    .font(.title3)
                    .foregroundStyle(AbastevoTheme.accentLight)
                Text(title)
                    .font(.caption.weight(.semibold))
                    .foregroundStyle(.primary)
                    .lineLimit(1)
            }
            .frame(maxWidth: .infinity)
            .padding(.vertical, 12)
            .background(Color.secondary.opacity(0.08))
            .clipShape(RoundedRectangle(cornerRadius: 12, style: .continuous))
        }
        .buttonStyle(.plain)
    }

    private var stationListSection: some View {
        let filtered = viewModel.sortedStations.filter { st in
            searchStationText.trimmingCharacters(in: .whitespacesAndNewlines).isEmpty
                || st.name.localizedCaseInsensitiveContains(searchStationText)
                || st.brand.localizedCaseInsensitiveContains(searchStationText)
                || st.neighborhood.localizedCaseInsensitiveContains(searchStationText)
        }
        return VStack(alignment: .leading, spacing: 10) {
            HStack {
                Text("Postos em \(viewModel.selectedMunicipality.name)")
                    .font(.headline)
                Spacer()
                Picker("Ordenar", selection: $viewModel.sortOrder) {
                    ForEach(StationSortOrder.allCases) { order in
                        Text(order.labelPtBr).tag(order)
                    }
                }
                .pickerStyle(.menu)
            }

            TextField("Filtrar por nome, bandeira ou bairro...", text: $searchStationText)
                .textFieldStyle(.roundedBorder)

            ForEach(filtered) { station in
                Button {
                    viewModel.selectedStation = station
                } label: {
                    stationRowCard(station)
                }
                .buttonStyle(.plain)
            }
        }
    }

    private func stationRowCard(_ station: StationRecord) -> some View {
        let fuel = viewModel.snapshot.selectedFuel
        let community = station.latestCommunityPrice(for: fuel)
        let official = station.officialPrice(for: fuel)
        return EnergiaCard {
            HStack(alignment: .top) {
                VStack(alignment: .leading, spacing: 4) {
                    Text(station.name)
                        .font(.headline)
                    Text("\(station.brand) · \(station.neighborhood)")
                        .font(.caption)
                        .foregroundStyle(.secondary)
                    Text(station.address)
                        .font(.caption2)
                        .foregroundStyle(.secondary)
                    if let promo = station.ownerPromotionText {
                        Text(promo)
                            .font(.caption2.bold())
                            .foregroundStyle(AbastevoTheme.accentLight)
                    }
                }
                Spacer()
                VStack(alignment: .trailing, spacing: 4) {
                    if let comm = community {
                        Text(AbastevoTheme.formatBrlMilli(comm.amountMilli))
                            .font(.title3.bold())
                        SourceTrustBadge(isOfficial: false, label: "\(comm.confirmations) conf.")
                    }
                    if let off = official {
                        Text("ANP: \(AbastevoTheme.formatBrlMilli(off))")
                            .font(.caption.weight(.semibold))
                            .foregroundStyle(.blue)
                    }
                    if community == nil && official == nil {
                        Text("Sem preço")
                            .font(.caption)
                            .foregroundStyle(.secondary)
                    }
                }
            }
        }
    }
}

public struct StationDetailSheetView: View {
    @Environment(\.dismiss) private var dismiss
    @ObservedObject var viewModel: AppStateViewModel
    let station: StationRecord

    @State private var commentText = ""
    @State private var commentNotice: String?

    public var body: some View {
        NavigationStack {
            ScrollView {
                VStack(alignment: .leading, spacing: 14) {
                    EnergiaCard {
                        HStack {
                            Text(station.brand)
                                .font(.caption.bold())
                                .padding(.horizontal, 8)
                                .padding(.vertical, 4)
                                .background(Color.secondary.opacity(0.14))
                                .clipShape(Capsule())
                            Text(station.pinTier.labelPtBr)
                                .font(.caption2)
                                .foregroundStyle(.secondary)
                            Spacer()
                            Button {
                                viewModel.toggleSavedStation(station)
                            } label: {
                                let saved = viewModel.snapshot.savedPlaces.contains(where: { $0.stationId == station.id })
                                Image(systemName: saved ? "bookmark.fill" : "bookmark")
                            }
                        }
                        Text(station.name).font(.title3.bold())
                        Text("\(station.address) — \(station.neighborhood)")
                            .font(.subheadline)
                            .foregroundStyle(.secondary)
                        Text("CNPJ: \(station.cnpj) · Horário: \(station.openingHours)")
                            .font(.caption)
                            .foregroundStyle(.secondary)
                        if let promo = station.ownerPromotionText {
                            Text("Promoção informada: \(promo)")
                                .font(.caption.bold())
                                .foregroundStyle(AbastevoTheme.accentLight)
                        }
                        HStack(spacing: 10) {
                            Button {
                                viewModel.activeSheet = .capturePrice
                            } label: {
                                Label("Enviar foto", systemImage: "camera.fill")
                                    .frame(maxWidth: .infinity)
                            }
                            .buttonStyle(.borderedProminent)
                            .tint(AbastevoTheme.accentLight)

                            Button {
                                viewModel.activeRouteStation = station
                                viewModel.selectedTab = .map
                                viewModel.activeSheet = .nativeRoute
                                dismiss()
                            } label: {
                                Label("Traçar rota", systemImage: "arrow.triangle.turn.up.right.diamond.fill")
                                    .frame(maxWidth: .infinity)
                            }
                            .buttonStyle(.bordered)
                        }
                    }

                    EnergiaCard {
                        Text("Preços por Combustível (ANP vs Comunidade)")
                            .font(.headline)
                        ForEach(FuelProduct.allCases) { product in
                            let comm = station.latestCommunityPrice(for: product)
                            let off = station.officialPrice(for: product)
                            if comm != nil || off != nil {
                                VStack(alignment: .leading, spacing: 4) {
                                    HStack {
                                        Text(product.displayNamePtBr)
                                            .font(.subheadline.bold())
                                        Spacer()
                                        if let comm = comm {
                                            Text("Comunidade: \(AbastevoTheme.formatBrlMilli(comm.amountMilli))")
                                                .font(.subheadline.bold())
                                                .foregroundStyle(AbastevoTheme.accentLight)
                                        }
                                        if let off = off {
                                            Text("ANP: \(AbastevoTheme.formatBrlMilli(off))")
                                                .font(.caption.weight(.semibold))
                                                .foregroundStyle(.blue)
                                        }
                                    }
                                    if let comm = comm {
                                        HStack(spacing: 12) {
                                            Button("Confirmar (\(comm.confirmations))") {
                                                viewModel.confirmObservation(comm)
                                            }
                                            .font(.caption.bold())
                                            Button("Útil (\(comm.helpfulCount))") {
                                                viewModel.markObservationHelpful(comm, isHelpful: true)
                                            }
                                            .font(.caption)
                                            Text(comm.condition.labelPtBr)
                                                .font(.caption2)
                                                .foregroundStyle(.secondary)
                                        }
                                    }
                                    Divider()
                                }
                            }
                        }
                    }

                    EnergiaCard {
                        Text("Qualidade e Fiscalização (PMQC / ANP)")
                            .font(.headline)
                        Text("Ensaios registrados: \(station.qualitySummary.samplesTested) · Não conformidades: \(station.qualitySummary.nonConformingCount) · Autuações: \(station.qualitySummary.enforcementNotices)")
                            .font(.subheadline)
                        Text("Última referência: \(station.qualitySummary.lastAuditLabel)")
                            .font(.caption)
                            .foregroundStyle(.secondary)
                    }

                    EnergiaCard {
                        Text("Comentários da Comunidade")
                            .font(.headline)
                        let remaining = FeedbackDisplay.charsRemaining(commentText)
                        FeedbackView(
                            commentsRemaining: remaining,
                            agreementText: "Avalie relatos deste posto",
                            state: viewModel.currentAccountIdOrNil == nil ? .signInRequired : .idle
                        )
                        HStack {
                            TextField("Escreva sua experiência (até 280 caracteres)...", text: $commentText)
                                .textFieldStyle(.roundedBorder)
                            Button("Enviar") {
                                switch viewModel.addStationComment(stationId: station.id, text: commentText) {
                                case .success:
                                    commentText = ""
                                    commentNotice = "Comentário publicado."
                                case .failure(let msg):
                                    commentNotice = msg
                                }
                            }
                            .buttonStyle(.borderedProminent)
                            .tint(AbastevoTheme.ink)
                        }
                        if let notice = commentNotice {
                            Text(notice)
                                .font(.caption)
                                .foregroundStyle(.secondary)
                        }

                        let comments = viewModel.snapshot.stationComments.filter { $0.stationId == station.id }
                        ForEach(comments) { c in
                            VStack(alignment: .leading, spacing: 4) {
                                HStack {
                                    Text(c.authorAlias).font(.caption.bold())
                                    Spacer()
                                    Text(c.createdAtLabel).font(.caption2).foregroundStyle(.secondary)
                                }
                                Text(c.text).font(.subheadline)
                                HStack(spacing: 12) {
                                    Text(FeedbackDisplay.agreementLine(valid: c.validVotes, invalid: c.invalidVotes))
                                        .font(.caption2)
                                        .foregroundStyle(.secondary)
                                    Button("Válido") { viewModel.voteStationComment(commentId: c.id, isValid: true) }
                                        .font(.caption2.bold())
                                    Button("Contestar") { viewModel.voteStationComment(commentId: c.id, isValid: false) }
                                        .font(.caption2)
                                }
                                Divider()
                            }
                        }
                    }
                }
                .padding(16)
            }
            .navigationTitle("Detalhes do Posto")
            .toolbar {
                ToolbarItem(placement: .cancellationAction) {
                    Button("Fechar") { dismiss() }
                }
            }
        }
    }
}

public struct LocationPickerSheetView: View {
    @Environment(\.dismiss) private var dismiss
    @ObservedObject var viewModel: AppStateViewModel
    @State private var query = ""
    @State private var selectedState: BrazilianState?

    public var body: some View {
        NavigationStack {
            VStack(spacing: 12) {
                Button {
                    Task { await viewModel.requestOneShotDeviceLocation() }
                } label: {
                    Label("Usar localização atual (leitura única)", systemImage: "location.fill")
                        .frame(maxWidth: .infinity)
                }
                .buttonStyle(.borderedProminent)
                .tint(AbastevoTheme.accentLight)
                .padding(.horizontal, 16)

                if let recovery = viewModel.lastLocationRecovery {
                    HStack {
                        Image(systemName: recovery.allowsClaim ? "checkmark.shield.fill" : "info.circle")
                        Text("Veredito GPS: \(recovery.verdict) (\(recovery.recoveryCode))")
                            .font(.caption)
                    }
                    .foregroundStyle(.secondary)
                    .padding(.horizontal, 16)
                }

                ScrollView(.horizontal, showsIndicators: false) {
                    HStack(spacing: 6) {
                        Button("Todos") { selectedState = nil }
                            .buttonStyle(.bordered)
                        ForEach(BrazilianState.allCases) { uf in
                            Button(uf.rawValue) { selectedState = uf }
                                .buttonStyle(.bordered)
                                .tint(selectedState == uf ? AbastevoTheme.accentLight : .secondary)
                        }
                    }
                    .padding(.horizontal, 16)
                }

                TextField("Pesquisar cidade...", text: $query)
                    .textFieldStyle(.roundedBorder)
                    .padding(.horizontal, 16)

                List(Municipality.rankMatches(query: query, stateFilter: selectedState, catalog: SeedCatalog.municipalities)) { city in
                    Button {
                        viewModel.selectMunicipality(city)
                        dismiss()
                    } label: {
                        HStack {
                            VStack(alignment: .leading) {
                                Text(city.name).font(.headline)
                                Text("\(city.state.fullName) · IBGE \(city.ibgeCode)")
                                    .font(.caption)
                                    .foregroundStyle(.secondary)
                            }
                            Spacer()
                            if city.ibgeCode == viewModel.selectedMunicipality.ibgeCode {
                                Image(systemName: "checkmark.circle.fill")
                                    .foregroundStyle(AbastevoTheme.accentLight)
                            }
                        }
                    }
                }
            }
            .navigationTitle("Escolher Cidade")
            .toolbar {
                ToolbarItem(placement: .cancellationAction) {
                    Button("Fechar") { dismiss() }
                }
            }
        }
    }
}

public struct WeekPickerSheetView: View {
    @Environment(\.dismiss) private var dismiss
    @ObservedObject var viewModel: AppStateViewModel

    public var body: some View {
        NavigationStack {
            List(SeedCatalog.surveyWeeks) { week in
                Button {
                    viewModel.selectWeek(week)
                    dismiss()
                } label: {
                    VStack(alignment: .leading, spacing: 4) {
                        HStack {
                            Text(week.label).font(.headline)
                            Spacer()
                            if viewModel.selectedWeek.id == week.id {
                                Image(systemName: "checkmark.circle.fill")
                                    .foregroundStyle(AbastevoTheme.accentLight)
                            }
                        }
                        Text("\(week.stationCount) postos · \(viewModel.snapshot.downloadedWeekIds.contains(week.id) ? "Disponível offline" : "Toque para baixar")")
                            .font(.caption)
                            .foregroundStyle(.secondary)
                        if let note = week.operationalNote {
                            Text(note).font(.caption2).foregroundStyle(.blue)
                        }
                    }
                }
            }
            .navigationTitle("Semanas da Pesquisa ANP")
            .toolbar {
                ToolbarItem(placement: .cancellationAction) {
                    Button("Fechar") { dismiss() }
                }
            }
        }
    }
}

public struct PriceHistorySheetView: View {
    @Environment(\.dismiss) private var dismiss
    @ObservedObject var viewModel: AppStateViewModel

    public var body: some View {
        let fuel = viewModel.snapshot.selectedFuel
        let points = SeedCatalog.priceHistory(for: fuel)
        let alertEnabled = viewModel.snapshot.priceDropAlerts.contains(fuel.rawValue)
        NavigationStack {
            List {
                Section("Alerta de Queda de Preço") {
                    Toggle("Avisar quando \(fuel.displayNamePtBr) cair", isOn: Binding(
                        get: { alertEnabled },
                        set: { _ in viewModel.togglePriceDropAlert(product: fuel) }
                    ))
                }
                Section("Série Histórica Semanal ANP — \(fuel.displayNamePtBr)") {
                    ForEach(points) { pt in
                        HStack {
                            VStack(alignment: .leading) {
                                Text(pt.weekLabel).font(.headline)
                                Text("Semana \(pt.weekId)").font(.caption).foregroundStyle(.secondary)
                            }
                            Spacer()
                            VStack(alignment: .trailing) {
                                Text("Média: \(AbastevoTheme.formatBrlMilli(pt.meanMilli))")
                                    .font(.subheadline.bold())
                                Text("Mín \(AbastevoTheme.formatBrlMilli(pt.minMilli)) · Máx \(AbastevoTheme.formatBrlMilli(pt.maxMilli))")
                                    .font(.caption2)
                                    .foregroundStyle(.secondary)
                            }
                        }
                    }
                }
            }
            .navigationTitle("Histórico de Preços")
            .toolbar {
                ToolbarItem(placement: .cancellationAction) {
                    Button("Fechar") { dismiss() }
                }
            }
        }
    }
}

public struct EvChargingExploreSheetView: View {
    @Environment(\.dismiss) private var dismiss
    @ObservedObject var viewModel: AppStateViewModel
    @State private var connectorFilter = "Todos"
    @State private var kwhChargeAmount = 35.0

    public var body: some View {
        let connectors = ["Todos", "CCS2", "Type 2", "CHAdeMO"]
        let filtered = viewModel.evSites.filter { site in
            connectorFilter == "Todos" || site.connectorTypes.contains(connectorFilter)
        }
        NavigationStack {
            List {
                Section("Simular Recarga (\(Int(kwhChargeAmount)) kWh)") {
                    Slider(value: $kwhChargeAmount, in: 10...90, step: 5)
                    Picker("Conector", selection: $connectorFilter) {
                        ForEach(connectors, id: \.self) { Text($0) }
                    }
                    .pickerStyle(.segmented)
                }
                Section("Eletropostos Verificados") {
                    ForEach(filtered) { site in
                        VStack(alignment: .leading, spacing: 6) {
                            HStack {
                                Text(site.name).font(.headline)
                                Spacer()
                                Text("\(site.maxPowerKw) kW")
                                    .font(.caption.bold())
                                    .padding(.horizontal, 8)
                                    .padding(.vertical, 3)
                                    .background(AbastevoTheme.accentLight.opacity(0.16))
                                    .clipShape(Capsule())
                            }
                            Text("\(site.operatorName) · \(site.cityLabel)")
                                .font(.caption)
                                .foregroundStyle(.secondary)
                            Text("Conectores: \(site.connectorTypes.joined(separator: ", ")) · \(site.availablePorts)/\(site.totalPorts) vagas livres")
                                .font(.caption)
                            let estCentavos = site.sessionFeeCentavos + Int64(kwhChargeAmount * Double(site.pricePerKwhCentavos))
                            Text("Tarifa: \(AbastevoTheme.formatCentavos(site.pricePerKwhCentavos))/kWh · Estimativa \(Int(kwhChargeAmount)) kWh: \(AbastevoTheme.formatCentavos(estCentavos))")
                                .font(.subheadline.bold())
                                .foregroundStyle(AbastevoTheme.accentLight)
                        }
                        .padding(.vertical, 4)
                    }
                }
            }
            .navigationTitle("Recarga Elétrica (EV)")
            .toolbar {
                ToolbarItem(placement: .cancellationAction) {
                    Button("Fechar") { dismiss() }
                }
            }
        }
    }
}

public struct QualityReportsSheetView: View {
    @Environment(\.dismiss) private var dismiss
    @ObservedObject var viewModel: AppStateViewModel

    public var body: some View {
        NavigationStack {
            List(viewModel.snapshot.qualityOccurrences) { occ in
                VStack(alignment: .leading, spacing: 4) {
                    HStack {
                        Text(occ.kind).font(.caption.bold()).foregroundStyle(.orange)
                        Spacer()
                        Text(occ.dateLabel).font(.caption2).foregroundStyle(.secondary)
                    }
                    Text(occ.stationName).font(.headline)
                    Text("\(occ.cityLabel) · \(occ.fuelLabel)").font(.caption).foregroundStyle(.secondary)
                    Text(occ.summary).font(.subheadline)
                    Text("Fonte: \(occ.sourceLabel)").font(.caption2).foregroundStyle(.blue)
                }
                .padding(.vertical, 4)
            }
            .navigationTitle("Qualidade & Ocorrências")
            .toolbar {
                ToolbarItem(placement: .cancellationAction) {
                    Button("Fechar") { dismiss() }
                }
            }
        }
    }
}

public struct SuggestStationSheetView: View {
    @Environment(\.dismiss) private var dismiss
    @ObservedObject var viewModel: AppStateViewModel
    @State private var name = ""
    @State private var brand = ""
    @State private var address = ""
    @State private var cnpj = ""
    @State private var isEv = false
    @State private var savedBanner: String?

    public var body: some View {
        NavigationStack {
            Form {
                Section("Dados do Posto ou Eletroposto") {
                    TextField("Nome fantasia", text: $name)
                    TextField("Bandeira ou rede", text: $brand)
                    TextField("Endereço completo", text: $address)
                    TextField("CNPJ (opcional)", text: $cnpj)
                    Toggle("É ponto de recarga elétrica (EV)", isOn: $isEv)
                }
                if let banner = savedBanner {
                    Section {
                        Text(banner).foregroundStyle(AbastevoTheme.accentLight)
                    }
                }
                Section {
                    Button("Enviar sugestão") {
                        let draft = StationSuggestionDraft(
                            name: name,
                            brand: brand,
                            address: address,
                            municipalityCode: viewModel.selectedMunicipality.ibgeCode,
                            cnpj: cnpj,
                            isEvSite: isEv,
                            createdAtLabel: "Agora"
                        )
                        viewModel.snapshot.stationSuggestions.insert(draft, at: 0)
                        viewModel.persist()
                        savedBanner = "Sugestão registrada para revisão em \(viewModel.selectedMunicipality.displayTitle)."
                        name = ""; brand = ""; address = ""; cnpj = ""
                    }
                    .disabled(name.trimmingCharacters(in: .whitespacesAndNewlines).isEmpty)
                }
            }
            .navigationTitle("Sugerir Posto")
            .toolbar {
                ToolbarItem(placement: .cancellationAction) {
                    Button("Fechar") { dismiss() }
                }
            }
        }
    }
}

public struct MasterFaqSheetView: View {
    @Environment(\.dismiss) private var dismiss

    public var body: some View {
        NavigationStack {
            List {
                faqRow(q: "Qual a diferença entre preço ANP e Comunidade?", a: "Os preços da ANP vêm da pesquisa semanal oficial (com atraso de publicação). Os preços da Comunidade são enviados por motoristas no posto, acompanhados de foto e confirmações.")
                faqRow(q: "O abastevo aceita pagamento para destacar postos?", a: "Não. Pagamentos nunca alteram a ordem de menor preço nem aumentam a confiança de um posto ou usuário.")
                faqRow(q: "Como o aplicativo funciona sem internet?", a: "As semanas da ANP, lista de cidades e mapas regionais ficam em cache local. Suas contribuições e lançamentos entram na fila offline.")
                faqRow(q: "As fotos que envio ficam armazenadas para sempre?", a: "Não. No celular, as fotos comprimidas em JPEG expiram em 24 horas e ficam fora do backup automático.")
            }
            .navigationTitle("Perguntas Frequentes")
            .toolbar {
                ToolbarItem(placement: .cancellationAction) {
                    Button("Fechar") { dismiss() }
                }
            }
        }
    }

    private func faqRow(q: String, a: String) -> some View {
        VStack(alignment: .leading, spacing: 6) {
            Text(q).font(.headline)
            Text(a).font(.subheadline).foregroundStyle(.secondary)
        }
        .padding(.vertical, 4)
    }
}

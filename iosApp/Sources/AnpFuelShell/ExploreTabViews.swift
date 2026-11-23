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

                    officialAnpCard
                        .padding(.horizontal, 16)

                    communitySummaryCard
                        .padding(.horizontal, 16)

                    if viewModel.snapshot.selectedFuel.unitWire == "L" {
                        tankFillCalculatorCard
                            .padding(.horizontal, 16)
                    }

                    discoveryShortcutsGrid
                        .padding(.horizontal, 16)

                    stationListSection
                        .padding(.horizontal, 16)
                }
                .padding(.vertical, 12)
            }
            .refreshable {
                await viewModel.refreshCityData()
            }
        }
    }

    private var officialAnpCard: some View {
        EnergiaCard {
            Text("Pesquisa oficial da ANP")
                .font(.subheadline.bold())
            Text("Média municipal indisponível. Consulte as coletas oficiais na ficha de cada posto.")
                .font(.subheadline)
                .foregroundStyle(.secondary)
            if let url = URL(string: OfficialAnpAverage.anpOfficialSourceUrl) {
                Link("Consulte a ANP", destination: url)
                    .font(.caption.bold())
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
            } else if viewModel.communityReadState == .loading {
                ProgressView()
            } else if viewModel.communityReadState == .unavailable {
                Text("Preços da comunidade indisponíveis.")
                    .font(.subheadline)
                    .foregroundStyle(.secondary)
            } else {
                Text("Sem preços comunitários recentes para este combustível.")
                    .font(.subheadline)
                    .foregroundStyle(.secondary)
            }
        }
    }

    private var tankFillCalculatorCard: some View {
        let fuel = viewModel.snapshot.selectedFuel
        let stationPrices: [StationPrice] = viewModel.sortedStations.compactMap { st in
            guard let price = viewModel.communityObservation(stationId: st.id, product: fuel) else { return nil }
            return StationPrice(station: st.name, unitMilli: price.amountMilli)
        }
        let ml = Int64(tankLitersDouble * 1000.0)
        let cheapestFill = try? TankFillUseCase.fillCheapest(prices: stationPrices, capacityMilliLiters: ml)

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
                        Text("Preço comunitário disponível (\(AbastevoTheme.formatBrlMilli(fill.unitMilli))/\(fuel.shortUnitLabel))")
                            .font(.caption)
                            .foregroundStyle(.secondary)
                    }
                    Spacer()
                    Text(AbastevoTheme.formatBrlMilli(fill.totalMilli))
                        .font(.title3.bold())
                }
            } else {
                Text("Aguardando preços de postos para estimar o total.")
                    .font(.caption)
                    .foregroundStyle(.secondary)
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
                Text(viewModel.selectedMunicipality.map { String(localized: "Postos em \($0.name)") } ?? String(localized: "Postos"))
                    .font(.headline)
                Spacer()
                Picker("Ordenar", selection: $viewModel.sortOrder) {
                    ForEach(StationSortOrder.allCases) { order in
                        Text(order.labelPtBr).tag(order)
                    }
                }
                .pickerStyle(.menu)
            }

            TextField("Filtrar postos carregados por nome ou bairro", text: $searchStationText)
                .textFieldStyle(.roundedBorder)

            if viewModel.selectedMunicipality == nil {
                Button("Escolher cidade") { viewModel.activeSheet = .locationPicker }
            } else if viewModel.stationReadState == .loading {
                ProgressView()
            } else if viewModel.stationReadState == .unavailable {
                Text("Catálogo de postos indisponível.")
                Button("Tentar novamente") { viewModel.reloadCityData() }
            } else if filtered.isEmpty {
                Text("Nenhum posto encontrado nesta lista.")
            }
            if viewModel.sortOrder == .distance && viewModel.lastDeviceCoordinate == nil {
                Text("Use sua localização no mapa para ordenar por distância.")
                    .font(.caption)
            }

            ForEach(filtered) { station in
                Button {
                    viewModel.selectedStation = station
                } label: {
                    stationRowCard(station)
                }
                .buttonStyle(.plain)
            }
            if viewModel.stationPageUnavailable {
                Text("Não foi possível carregar a próxima página.")
                    .font(.caption)
            }
            if viewModel.isLoadingMoreStations {
                ProgressView()
            } else if viewModel.nextStationCursor != nil {
                Button("Carregar mais postos") {
                    Task { await viewModel.loadMoreStations() }
                }
            }
        }
    }

    private func stationRowCard(_ station: StationRecord) -> some View {
        let fuel = viewModel.snapshot.selectedFuel
        let community = viewModel.communityObservation(stationId: station.id, product: fuel)
        return EnergiaCard {
            HStack(alignment: .top) {
                VStack(alignment: .leading, spacing: 4) {
                    Text(station.name)
                        .font(.headline)
                    Text(station.neighborhood)
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
                    if community == nil {
                        Text("Ver preços")
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
    @State private var publicComments: [PublicStationComment] = []
    @State private var commentsReadState: PublicReadState = .loading
    @State private var officialPrices: [OfficialStationPrice] = []
    @State private var officialReadState: PublicReadState = .loading
    @State private var officialReadAttempt = 0
    @State private var showRoute = false

    public var body: some View {
        NavigationStack {
            ScrollView {
                VStack(alignment: .leading, spacing: 14) {
                    EnergiaCard {
                        HStack {
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
                        if !station.cnpj.isEmpty {
                            Text("CNPJ: \(station.cnpj)")
                                .font(.caption)
                                .foregroundStyle(.secondary)
                        }
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
                                showRoute = true
                            } label: {
                                Label("Traçar rota", systemImage: "arrow.triangle.turn.up.right.diamond.fill")
                                    .frame(maxWidth: .infinity)
                            }
                            .buttonStyle(.bordered)
                            .disabled(!station.pinTier.allowsMapPin)
                        }
                    }

                    EnergiaCard {
                        Text("Preços por Combustível (ANP vs Comunidade)")
                            .font(.headline)
                        switch officialReadState {
                        case .loading:
                            ProgressView()
                        case .unavailable:
                            Text("Preços oficiais deste posto indisponíveis.")
                            Button("Tentar novamente") { officialReadAttempt += 1 }
                        case .loaded:
                            if officialPrices.isEmpty {
                                Text("Nenhuma coleta oficial registrada para este posto.")
                            }
                        }
                        ForEach(FuelProduct.allCases) { product in
                            let comm = viewModel.communityObservation(stationId: station.id, product: product)
                            let off = officialPrices.first { $0.product == product }
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
                                            Text("ANP: \(AbastevoTheme.formatBrlMilli(off.amountMilli))")
                                                .font(.caption.weight(.semibold))
                                                .foregroundStyle(.blue)
                                        }
                                    }
                                    if let off {
                                        Text("Coleta ANP: \(off.collectedOn) · Pesquisa: \(off.surveyStart) a \(off.surveyEnd)")
                                            .font(.caption)
                                            .foregroundStyle(.secondary)
                                    }
                                    if let comm = comm {
                                        HStack(spacing: 12) {
                                            Label("Confirmações: \(comm.confirmations)", systemImage: "hand.thumbsup")
                                                .font(.caption)
                                                .foregroundStyle(.secondary)
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
                        Text("Dados de qualidade deste posto indisponíveis.")
                            .font(.subheadline)
                            .foregroundStyle(.secondary)
                    }

                    EnergiaCard {
                        Text("Comentários da comunidade · \(viewModel.snapshot.selectedFuel.displayNamePtBr)")
                            .font(.headline)
                        switch commentsReadState {
                        case .loading:
                            ProgressView()
                        case .unavailable:
                            Text("Não foi possível carregar os comentários.").foregroundStyle(.secondary)
                        case .loaded:
                            if publicComments.isEmpty {
                                Text("Nenhum comentário publicado para este combustível.").foregroundStyle(.secondary)
                            }
                            ForEach(publicComments) { comment in
                                VStack(alignment: .leading, spacing: 4) {
                                    Text(comment.publicAlias).font(.caption.bold())
                                    Text(comment.text)
                                }
                            }
                        }
                    }

                    EnergiaCard {
                        Text("Rascunhos neste dispositivo")
                            .font(.headline)
                        HStack {
                            TextField("Escreva sua experiência (até 280 caracteres)...", text: $commentText)
                                .textFieldStyle(.roundedBorder)
                            Button("Salvar rascunho") {
                                switch viewModel.saveStationCommentDraft(stationId: station.id, text: commentText) {
                                case .success:
                                    commentText = ""
                                    commentNotice = "Comentário salvo neste dispositivo."
                                case .failure(let msg):
                                    commentNotice = msg.localizedDescription
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

                        let comments = viewModel.snapshot.stationComments.filter { $0.isOwn && $0.stationId == station.id }
                        ForEach(comments) { c in
                            VStack(alignment: .leading, spacing: 4) {
                                Text(c.text).font(.subheadline)
                                Divider()
                            }
                        }
                    }
                }
                .padding(16)
            }
            .task(id: viewModel.snapshot.selectedFuel) {
                publicComments = []
                commentsReadState = .loading
                do {
                    let fuel = viewModel.snapshot.selectedFuel
                    let loaded = try await viewModel.socialReadClient().fetchStationComments(stationId: station.id, fuel: fuel)
                    guard !Task.isCancelled else { return }
                    publicComments = loaded
                    commentsReadState = .loaded
                } catch {
                    guard !Task.isCancelled else { return }
                    commentsReadState = .unavailable
                }
            }
            .task(id: officialReadAttempt) {
                officialPrices = []
                officialReadState = .loading
                do {
                    let prices = try await viewModel.officialPrices(for: station)
                    guard !Task.isCancelled else { return }
                    officialPrices = prices
                    officialReadState = .loaded
                } catch {
                    guard !Task.isCancelled else { return }
                    officialReadState = .unavailable
                }
            }
            .sheet(isPresented: $showRoute) {
                NativeRouteSheetView(viewModel: viewModel, station: station)
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

                TextField("Pesquisar município, UF ou código IBGE", text: $query)
                    .textFieldStyle(.roundedBorder)
                    .padding(.horizontal, 16)

                let cities = viewModel.municipalityCatalog.search(query: query, state: selectedState)
                if viewModel.municipalityCatalog.count == 0 {
                    ContentUnavailableView("Catálogo de municípios indisponível", systemImage: "building.2")
                } else if cities.isEmpty {
                    ContentUnavailableView("Nenhum município encontrado", systemImage: "magnifyingglass")
                } else {
                    List(cities) { city in
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
                                if city.ibgeCode == viewModel.selectedMunicipality?.ibgeCode {
                                    Image(systemName: "checkmark.circle.fill")
                                        .foregroundStyle(AbastevoTheme.accentLight)
                                }
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

    public var body: some View {
        NavigationStack {
            ContentUnavailableView(
                "Semanas indisponíveis",
                systemImage: "calendar",
                description: Text("As datas das coletas disponíveis aparecem na ficha do posto.")
            )
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

    public var body: some View {
        NavigationStack {
            ContentUnavailableView(
                "Histórico municipal indisponível",
                systemImage: "chart.line.uptrend.xyaxis",
                description: Text("Consulte o preço oficial e a data da coleta na ficha de cada posto.")
            )
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
                    if viewModel.evSites.isEmpty {
                        Text("Catálogo de recarga indisponível.")
                            .foregroundStyle(.secondary)
                    } else if filtered.isEmpty {
                        Text("Nenhum local encontrado para este conector.")
                            .foregroundStyle(.secondary)
                    }
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

    public var body: some View {
        NavigationStack {
            ContentUnavailableView("Catálogo de qualidade indisponível", systemImage: "flask")
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
                    Button("Salvar rascunho") {
                        guard let municipality = viewModel.selectedMunicipality else { return }
                        let draft = StationSuggestionDraft(
                            name: name,
                            brand: brand,
                            address: address,
                            municipalityCode: municipality.ibgeCode,
                            cnpj: cnpj,
                            isEvSite: isEv,
                            createdAtLabel: "Agora"
                        )
                        viewModel.snapshot.stationSuggestions.insert(draft, at: 0)
                        viewModel.persist()
                        savedBanner = "Rascunho salvo neste dispositivo. Envio indisponível."
                        name = ""; brand = ""; address = ""; cnpj = ""
                    }
                    .disabled(name.trimmingCharacters(in: .whitespacesAndNewlines).isEmpty || viewModel.selectedMunicipality == nil)
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
                faqRow(q: "Como o aplicativo funciona sem internet?", a: "A lista de cidades vem com o app. Seus rascunhos de preço e lançamentos do diário ficam neste aparelho. Consultas de preços e rotas precisam de conexão.")
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

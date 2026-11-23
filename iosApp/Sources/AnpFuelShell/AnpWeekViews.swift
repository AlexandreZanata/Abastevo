import SwiftUI
import AnpFuelCore

/// Explore card with the selected city's official ANP averages for the active week.
struct AnpCityAverageCard: View {
    @ObservedObject var app: AppStateViewModel
    @ObservedObject var anp: AnpSurveyModel
    @State private var alertMessage: String?

    var body: some View {
        EnergiaCard {
            HStack(alignment: .firstTextBaseline) {
                SourceTrustBadge(isOfficial: true, label: "ANP")
                Text("Pesquisa oficial da ANP")
                    .font(.subheadline.bold())
                Spacer()
                Button {
                    app.activeSheet = .weekPicker
                } label: {
                    Label(anp.displayWeek.map(AnpWeekText.compact) ?? String(localized: "Semanas"), systemImage: "calendar")
                        .font(.caption.bold())
                }
                .buttonStyle(.bordered)
            }
            AnpSyncStatusView(anp: anp)
            content
            HStack {
                Button {
                    app.activeSheet = .priceHistory
                } label: {
                    Label("Histórico", systemImage: "chart.bar.xaxis")
                        .font(.caption.bold())
                }
                .disabled(anp.manifests.isEmpty)
                Spacer()
                if let url = URL(string: OfficialAnpAverage.anpOfficialSourceUrl) {
                    Link("Fonte: ANP", destination: url)
                        .font(.caption.bold())
                }
            }
        }
    }

    @ViewBuilder
    private var content: some View {
        if anp.manifests.isEmpty {
            Text("Nenhuma semana da pesquisa ANP baixada neste aparelho.")
                .font(.subheadline)
                .foregroundStyle(.secondary)
            Button {
                Task { await anp.manualSync() }
            } label: {
                Label("Baixar a semana mais recente", systemImage: "arrow.down.circle")
            }
            .disabled(anp.isSyncing || app.isOfflineSimulation)
        } else if app.selectedMunicipality == nil {
            Button("Escolher cidade") { app.activeSheet = .locationPicker }
        } else if anp.cityAverages.isEmpty {
            Text(availabilityText)
                .font(.subheadline)
                .foregroundStyle(.secondary)
        } else {
            ForEach(anp.cityAverages, id: \.product) { average in
                AnpAverageRow(average: average, highlighted: average.product == app.snapshot.selectedFuel)
            }
            if let note = anp.displayManifest?.operationalNote {
                Text("Nota da ANP: \(note)")
                    .font(.caption2)
                    .foregroundStyle(.secondary)
            }
            alertToggle
        }
    }

    private var availabilityText: String {
        switch anp.cityAvailability {
        case .noDataThisWeek:
            return String(localized: "A ANP não pesquisou esta cidade na semana exibida. Escolha outra semana.")
        case .neverInAnp:
            return String(localized: "Esta cidade não aparece nas semanas baixadas da pesquisa ANP.")
        default:
            return String(localized: "Média municipal indisponível para esta cidade.")
        }
    }

    private var alertToggle: some View {
        let fuel = app.snapshot.selectedFuel
        return VStack(alignment: .leading, spacing: 4) {
            Toggle(isOn: Binding(
                get: { anp.isAlertEnabled(fuel) },
                set: { enabled in
                    Task {
                        let allowed = await anp.setAlert(fuel, enabled: enabled)
                        alertMessage = enabled && !allowed
                            ? String(localized: "Permita notificações nos Ajustes do iPhone para receber o alerta.")
                            : nil
                    }
                }
            )) {
                Text("Avisar quando \(fuel.displayNamePtBr) cair na pesquisa ANP")
                    .font(.caption)
            }
            if let alertMessage {
                Text(alertMessage)
                    .font(.caption2)
                    .foregroundStyle(.orange)
            }
        }
    }
}

struct AnpAverageRow: View {
    let average: AnpMunicipalityAverage
    let highlighted: Bool

    var body: some View {
        HStack(alignment: .firstTextBaseline) {
            VStack(alignment: .leading, spacing: 2) {
                Text(average.product.displayNamePtBr)
                    .font(highlighted ? .subheadline.bold() : .subheadline)
                if let minimum = average.minimumMilli, let maximum = average.maximumMilli {
                    Text("Mín. \(AnpPriceRounding.formatBrl(milli: minimum)) · Máx. \(AnpPriceRounding.formatBrl(milli: maximum))")
                        .font(.caption2)
                        .foregroundStyle(.secondary)
                }
            }
            Spacer()
            VStack(alignment: .trailing, spacing: 2) {
                Text(average.averageMilli.map { AnpPriceRounding.formatBrl(milli: $0) } ?? "—")
                    .font(highlighted ? .headline : .subheadline)
                if let count = average.stationCount {
                    Text("\(count) postos")
                        .font(.caption2)
                        .foregroundStyle(.secondary)
                }
            }
        }
        .accessibilityElement(children: .combine)
    }
}

/// Download progress, readiness, verification and messages shared by the card and the picker.
struct AnpSyncStatusView: View {
    @ObservedObject var anp: AnpSurveyModel

    var body: some View {
        VStack(alignment: .leading, spacing: 4) {
            if let progress = anp.progress {
                HStack {
                    if let fraction = progress.fraction {
                        ProgressView(value: fraction)
                    } else {
                        ProgressView()
                    }
                    Button("Cancelar") { anp.cancelSync() }
                        .font(.caption)
                }
                Text(progressText(progress))
                    .font(.caption2)
                    .foregroundStyle(.secondary)
            } else if anp.isCheckingLatest {
                HStack {
                    ProgressView()
                    Text("Consultando a página da ANP…").font(.caption)
                }
            }
            if let week = anp.displayWeek {
                if anp.isStaleVerified, let verification = anp.verification {
                    Text("Verificado \(AnpWeekText.age(since: Date(timeIntervalSince1970: TimeInterval(verification.verifiedAtEpochSeconds)))): a ANP ainda não publicou semana mais nova.")
                        .font(.caption2)
                        .foregroundStyle(.secondary)
                } else if anp.isStale {
                    Text("Dados de \(AnpWeekText.range(week)) estão desatualizados. Puxe para atualizar.")
                        .font(.caption2)
                        .foregroundStyle(.orange)
                } else if anp.readiness == .partial {
                    Text("Semana baixada sem os preços por posto deste estado.")
                        .font(.caption2)
                        .foregroundStyle(.secondary)
                }
                if !anp.isDisplayingLatest {
                    Text("Exibindo semana escolhida; há uma semana mais recente baixada.")
                        .font(.caption2)
                        .foregroundStyle(.secondary)
                }
            }
            if let notice = anp.notice {
                Text(noticeText(notice))
                    .font(.caption)
                    .foregroundStyle(AbastevoTheme.accentLight)
            }
            if let error = anp.syncError {
                Text(error)
                    .font(.caption)
                    .foregroundStyle(.orange)
            }
        }
    }

    private func progressText(_ progress: AnpDownloadProgress) -> String {
        let received = AnpWeekText.bytes(progress.receivedBytes)
        let total = progress.expectedBytes.map { " de \(AnpWeekText.bytes($0))" } ?? ""
        switch progress.stage {
        case .summary: return String(localized: "Baixando médias municipais: \(received)\(total)")
        case .stations: return String(localized: "Baixando preços por posto: \(received)\(total)")
        case .importing: return String(localized: "Verificando e importando a planilha…")
        case .saving: return String(localized: "Salvando neste aparelho…")
        }
    }

    private func noticeText(_ notice: AnpSurveyModel.Notice) -> String {
        switch notice {
        case .updated(let week): return String(localized: "Semana \(week) baixada e verificada.")
        case .partial(let week): return String(localized: "Semana \(week) baixada sem os preços por posto. Tente novamente mais tarde.")
        case .verifiedNoNewWeek(let week): return String(localized: "Verificado: \(week) é a semana mais recente da ANP.")
        case .alreadyDownloaded(let week): return String(localized: "Semana \(week) já está neste aparelho.")
        }
    }
}

public struct WeekPickerSheetView: View {
    @Environment(\.dismiss) private var dismiss
    @ObservedObject var viewModel: AppStateViewModel

    public var body: some View {
        NavigationStack {
            AnpWeekPickerContent(app: viewModel, anp: viewModel.anpSurvey)
                .navigationTitle("Semanas da Pesquisa ANP")
                .toolbar {
                    ToolbarItem(placement: .cancellationAction) {
                        Button("Fechar") { dismiss() }
                    }
                    ToolbarItem(placement: .primaryAction) {
                        Button {
                            viewModel.anpSurvey.loadCatalog()
                        } label: {
                            Image(systemName: "arrow.clockwise")
                        }
                        .accessibilityLabel("Atualizar lista de semanas")
                    }
                }
        }
    }
}

struct AnpWeekPickerContent: View {
    @ObservedObject var app: AppStateViewModel
    @ObservedObject var anp: AnpSurveyModel

    var body: some View {
        List {
            Section("Semana em uso") {
                if let week = anp.displayWeek {
                    Text(AnpWeekText.range(week)).font(.headline)
                    Text(anp.isDisplayingLatest ? "Mais recente baixada" : "Semana escolhida")
                        .font(.caption)
                        .foregroundStyle(.secondary)
                } else {
                    Text("Nenhuma semana baixada.").foregroundStyle(.secondary)
                }
                AnpSyncStatusView(anp: anp)
                if let latest = anp.catalog.first {
                    Button("Usar a semana mais recente (\(AnpWeekText.compact(latest.week)))") { anp.select(latest) }
                        .disabled(anp.isSyncing)
                }
                if let check = anp.lastCatalogCheck {
                    Text("Lista da ANP consultada \(AnpWeekText.age(since: check)).")
                        .font(.caption2)
                        .foregroundStyle(.secondary)
                }
            }

            if !anp.manifests.isEmpty {
                Section("Baixadas neste aparelho (\(AnpWeekText.bytes(anp.storedBytes)))") {
                    ForEach(anp.manifests) { manifest in
                        downloadedRow(manifest)
                    }
                    .onDelete { offsets in
                        let ids = offsets.map { anp.manifests[$0].week.id }
                        ids.forEach { anp.delete(weekId: $0) }
                    }
                }
            }

            if anp.isLoadingCatalog {
                Section { ProgressView("Carregando semanas da ANP…") }
            } else if let error = anp.catalogError {
                Section {
                    Text(error).foregroundStyle(.secondary)
                    Button("Tentar novamente") { anp.loadCatalog() }
                }
            }

            ForEach(groupedCatalog, id: \.year) { group in
                Section(String(group.year)) {
                    ForEach(group.entries) { entry in
                        catalogRow(entry)
                    }
                }
            }

            Section {
                Text("Arquivos oficiais baixados de gov.br/anp, verificados (tamanho, SHA-256 e conteúdo da planilha) e guardados fora do backup. Preços por posto ficam restritos ao estado da cidade escolhida.")
                    .font(.caption2)
                    .foregroundStyle(.secondary)
            }
        }
        .onAppear {
            if anp.catalog.isEmpty { anp.loadCatalog() }
        }
    }

    private var groupedCatalog: [(year: Int, entries: [AnpSurveyCatalogEntry])] {
        var groups: [Int: [AnpSurveyCatalogEntry]] = [:]
        for entry in anp.catalog { groups[entry.week.endYear, default: []].append(entry) }
        return groups.keys.sorted(by: >).map { year in (year: year, entries: groups[year] ?? []) }
    }

    private func downloadedRow(_ manifest: AnpWeekManifest) -> some View {
        let active = manifest.week == anp.displayWeek
        let stationText: String
        if let scope = manifest.stationScope {
            stationText = String(localized: "\(manifest.stationRowCount) preços de postos (\(scope.rawValue))")
        } else {
            stationText = String(localized: "sem preços por posto")
        }
        let downloadedAt = Date(timeIntervalSince1970: TimeInterval(manifest.downloadedAtEpochSeconds))
        return Button {
            if let entry = anp.catalog.first(where: { $0.week == manifest.week }) {
                anp.select(entry)
            } else {
                app.snapshot.selectedWeekId = manifest.week.id
                app.persist()
                anp.reloadLocal()
            }
        } label: {
            HStack {
                VStack(alignment: .leading, spacing: 2) {
                    Text(AnpWeekText.range(manifest.week)).font(.subheadline.bold())
                    Text("\(manifest.averageRowCount) médias · \(stationText)")
                        .font(.caption)
                    Text("Baixado \(AnpWeekText.age(since: downloadedAt)) · \(AnpWeekText.bytes(manifest.downloadedBytes)) transferidos")
                        .font(.caption2)
                        .foregroundStyle(.secondary)
                    if manifest.quarantinedRowCount > 0 {
                        Text("\(manifest.quarantinedRowCount) linhas ignoradas por rótulo desconhecido")
                            .font(.caption2)
                            .foregroundStyle(.secondary)
                    }
                }
                Spacer()
                if active {
                    Image(systemName: "checkmark.circle.fill").foregroundStyle(AbastevoTheme.accentLight)
                }
            }
        }
        .buttonStyle(.plain)
        .disabled(anp.isSyncing)
    }

    private func catalogRow(_ entry: AnpSurveyCatalogEntry) -> some View {
        let downloaded = anp.manifest(for: entry)
        let syncing = anp.syncingWeekId == entry.week.id
        return VStack(alignment: .leading, spacing: 4) {
            HStack {
                VStack(alignment: .leading, spacing: 2) {
                    Text(AnpWeekText.range(entry.week)).font(.subheadline)
                    if let published = entry.publishedOnIso {
                        Text("Atualizado pela ANP em \(AnpWeekText.brazilian(published))")
                            .font(.caption2)
                            .foregroundStyle(.secondary)
                    }
                }
                Spacer()
                if syncing {
                    ProgressView()
                } else if downloaded != nil {
                    Label("Baixada", systemImage: "checkmark.circle")
                        .font(.caption)
                        .foregroundStyle(AbastevoTheme.accentLight)
                }
            }
            if let note = entry.operationalNote {
                Text("Nota: \(note)")
                    .font(.caption2)
                    .foregroundStyle(.orange)
            }
            HStack {
                Button("Usar esta semana") { anp.select(entry) }
                    .buttonStyle(.borderedProminent)
                    .tint(AbastevoTheme.accentLight)
                if downloaded == nil {
                    Button("Só baixar") { anp.download(entry) }
                        .buttonStyle(.bordered)
                }
            }
            .font(.caption)
            .disabled(anp.isSyncing || app.isOfflineSimulation)
        }
        .padding(.vertical, 2)
    }
}

public struct PriceHistorySheetView: View {
    @Environment(\.dismiss) private var dismiss
    @ObservedObject var viewModel: AppStateViewModel

    public var body: some View {
        NavigationStack {
            AnpHistoryContent(app: viewModel, anp: viewModel.anpSurvey)
                .navigationTitle("Histórico de Preços")
                .toolbar {
                    ToolbarItem(placement: .cancellationAction) {
                        Button("Fechar") { dismiss() }
                    }
                }
        }
    }
}

struct AnpHistoryContent: View {
    @ObservedObject var app: AppStateViewModel
    @ObservedObject var anp: AnpSurveyModel
    @State private var fuel: FuelProduct = .gasolineRegular
    @State private var history: AnpCityHistory?
    @State private var isLoading = true
    @State private var selectedWeekId: String?

    var body: some View {
        List {
            Section {
                if app.isOfflineSimulation {
                    Label("Sem conexão: histórico das semanas baixadas neste aparelho.", systemImage: "wifi.slash")
                        .font(.caption)
                }
                Picker("Combustível", selection: $fuel) {
                    ForEach(AnpOfficialRules.productDisplayOrder) { product in
                        Text(product.displayNamePtBr).tag(product)
                    }
                }
                Text(app.selectedMunicipality?.displayTitle ?? String(localized: "Escolha uma cidade"))
                    .font(.headline)
            }
            if isLoading {
                Section { ProgressView() }
            } else if app.selectedMunicipality == nil {
                Section {
                    Button("Escolher cidade") { app.activeSheet = .locationPicker }
                }
            } else {
                switch history {
                case .available(let entries):
                    Section("Média semanal (ANP)") {
                        AnpTrendBars(entries: entries, selectedWeekId: $selectedWeekId)
                            .frame(height: 140)
                        if let selected = entries.first(where: { $0.week.id == selectedWeekId }) {
                            Text("\(AnpWeekText.compact(selected.week)): \(selected.averageMilli.map { AnpPriceRounding.formatBrl(milli: $0) } ?? "—")")
                                .font(.caption.bold())
                        }
                    }
                    Section("Semanas") {
                        ForEach(entries) { entry in
                            HStack {
                                Text(AnpWeekText.compact(entry.week))
                                Spacer()
                                Text(entry.averageMilli.map { AnpPriceRounding.formatBrl(milli: $0) } ?? String(localized: "Preço indisponível"))
                                    .font(.headline)
                            }
                        }
                    }
                case .insufficient(let weeks):
                    Section {
                        Text("Histórico insuficiente: são necessárias pelo menos duas semanas baixadas com este combustível nesta cidade (há \(weeks)).")
                            .foregroundStyle(.secondary)
                        Button("Baixar mais semanas") { app.activeSheet = .weekPicker }
                    }
                case nil:
                    Section { Text("Histórico indisponível.").foregroundStyle(.secondary) }
                }
            }
            Section {
                if let url = URL(string: OfficialAnpAverage.anpOfficialSourceUrl) {
                    Link("Fonte: ANP — Levantamento de Preços de Combustíveis", destination: url)
                        .font(.caption)
                }
            }
        }
        .onAppear { fuel = app.snapshot.selectedFuel }
        .task(id: "\(fuel.rawValue)|\(app.snapshot.selectedMunicipalityCode)|\(anp.manifests.count)") {
            isLoading = true
            selectedWeekId = nil
            let loaded = await anp.cityHistory(product: fuel)
            guard !Task.isCancelled else { return }
            history = loaded
            isLoading = false
        }
    }
}

/// Android `PriceHistoryTrendChart`: oldest to newest bars, tap to read a week.
struct AnpTrendBars: View {
    let entries: [AnpCityHistoryEntry]
    @Binding var selectedWeekId: String?

    var body: some View {
        let ordered = entries.filter { $0.averageMilli != nil }.sorted { $0.week.startIso < $1.week.startIso }
        let fractions = AnpOfficialRules.trendBarFractions(ordered.compactMap(\.averageMilli))
        GeometryReader { proxy in
            HStack(alignment: .bottom, spacing: 6) {
                ForEach(Array(ordered.enumerated()), id: \.element.id) { index, entry in
                    let fraction = index < fractions.count ? fractions[index] : 0.15
                    RoundedRectangle(cornerRadius: 4)
                        .fill(AbastevoTheme.accentLight.opacity(selectedWeekId == nil || selectedWeekId == entry.week.id ? 1.0 : 0.45))
                        .frame(height: max(4, proxy.size.height * fraction))
                        .frame(maxWidth: .infinity)
                        .onTapGesture {
                            selectedWeekId = selectedWeekId == entry.week.id ? nil : entry.week.id
                        }
                        .accessibilityLabel(AnpWeekText.compact(entry.week))
                        .accessibilityValue(entry.averageMilli.map { AnpPriceRounding.formatBrl(milli: $0) } ?? "")
                        .accessibilityAddTraits(.isButton)
                }
            }
            .frame(maxHeight: .infinity, alignment: .bottom)
        }
    }
}

/// Weekly official prices of one directory station (backend ANP history), with offline fallback.
struct StationOfficialHistoryCard: View {
    @ObservedObject var viewModel: AppStateViewModel
    let station: StationRecord
    @State private var entries: [OfficialPriceHistoryEntry] = []
    @State private var state: PublicReadState = .loading

    var body: some View {
        let fuel = viewModel.snapshot.selectedFuel
        EnergiaCard {
            Text("Histórico ANP deste posto · \(fuel.displayNamePtBr)")
                .font(.headline)
            switch state {
            case .loading:
                ProgressView()
            case .unavailable:
                Text(viewModel.isOfflineSimulation
                     ? String(localized: "Sem conexão: histórico por posto indisponível offline.")
                     : String(localized: "Histórico oficial deste posto indisponível."))
                    .font(.subheadline)
                    .foregroundStyle(.secondary)
            case .loaded:
                if entries.isEmpty {
                    Text("Nenhuma coleta ANP registrada para este combustível.")
                        .font(.subheadline)
                        .foregroundStyle(.secondary)
                }
                ForEach(entries) { entry in
                    HStack {
                        VStack(alignment: .leading, spacing: 2) {
                            Text(AnpWeekText.compact(entry.surveyWeek)).font(.subheadline)
                            Text("Coleta em \(AnpWeekText.brazilian(entry.collectedOn))")
                                .font(.caption2)
                                .foregroundStyle(.secondary)
                        }
                        Spacer()
                        Text(AbastevoTheme.formatBrlMilli(entry.amountMilli))
                            .font(.subheadline.bold())
                    }
                }
            }
        }
        .task(id: fuel) {
            entries = []
            state = .loading
            do {
                let client = try viewModel.socialReadClient()
                let loaded = try await client.fetchOfficialPriceHistory(stationId: station.id, fuel: fuel)
                guard !Task.isCancelled else { return }
                entries = loaded
                state = .loaded
            } catch {
                guard !Task.isCancelled else { return }
                state = .unavailable
            }
        }
    }
}

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
                Text(String(localized: "anp.week.official_title"))
                    .font(.subheadline.bold())
                Spacer()
                Button {
                    app.activeSheet = .weekPicker
                } label: {
                    Label(anp.displayWeek.map(AnpWeekText.compact) ?? String(localized: "anp.week.weeks"), systemImage: "calendar")
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
                    Label(String(localized: "android.nav_history"), systemImage: "chart.bar.xaxis")
                        .font(.caption.bold())
                }
                .disabled(anp.manifests.isEmpty)
                Spacer()
                if let url = URL(string: OfficialAnpAverage.anpOfficialSourceUrl) {
                    Link(String(localized: "anp.week.source"), destination: url)
                        .font(.caption.bold())
                }
            }
        }
    }

    @ViewBuilder
    private var content: some View {
        if anp.manifests.isEmpty {
            Text(String(localized: "anp.week.no_downloads_full"))
                .font(.subheadline)
                .foregroundStyle(.secondary)
            Button {
                Task { await anp.manualSync() }
            } label: {
                Label(String(localized: "anp.week.download_latest"), systemImage: "arrow.down.circle")
            }
            .disabled(anp.isSyncing || app.isOfflineSimulation)
        } else if app.selectedMunicipality == nil {
            Button(String(localized: "android.feed_choose_city")) { app.activeSheet = .locationPicker }
        } else if anp.cityAverages.isEmpty {
            Text(availabilityText)
                .font(.subheadline)
                .foregroundStyle(.secondary)
        } else {
            ForEach(anp.cityAverages, id: \.product) { average in
                AnpAverageRow(average: average, highlighted: average.product == app.snapshot.selectedFuel)
            }
            if let note = anp.displayManifest?.operationalNote {
                Text(String(localized: "android.week_picker_operational_note", defaultValue: "Note: \(note)"))
                    .font(.caption2)
                    .foregroundStyle(.secondary)
            }
            alertToggle
        }
    }

    private var availabilityText: String {
        switch anp.cityAvailability {
        case .noDataThisWeek:
            return String(localized: "android.prices_empty_no_data_this_week")
        case .neverInAnp:
            return String(localized: "anp.week.city_missing")
        default:
            return String(localized: "anp.week.average_unavailable")
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
                            ? String(localized: "anp.alert.settings_permission")
                            : nil
                    }
                }
            )) {
                Text(String(localized: "anp.alert.toggle", defaultValue: "Notify me when the ANP price for \(fuel.localizedName) falls"))
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
                Text(average.product.localizedName)
                    .font(highlighted ? .subheadline.bold() : .subheadline)
                if let minimum = average.minimumMilli, let maximum = average.maximumMilli {
                    Text(String(localized: "anp.week.price_range", defaultValue: "Min. \(AnpPriceRounding.formatBrl(milli: minimum, locale: .current)) · Max. \(AnpPriceRounding.formatBrl(milli: maximum, locale: .current))"))
                        .font(.caption2)
                        .foregroundStyle(.secondary)
                }
            }
            Spacer()
            VStack(alignment: .trailing, spacing: 2) {
                Text(average.averageMilli.map { AnpPriceRounding.formatBrl(milli: $0, locale: .current) } ?? "—")
                    .font(highlighted ? .headline : .subheadline)
                if let count = average.stationCount {
                    Text(String(localized: "anp.week.station_count", defaultValue: "Stations: \(count.formatted())"))
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
                    Button(String(localized: "android.map_cancel")) { anp.cancelSync() }
                        .font(.caption)
                }
                Text(progressText(progress))
                    .font(.caption2)
                    .foregroundStyle(.secondary)
            } else if anp.isCheckingLatest {
                HStack {
                    ProgressView()
                    Text(String(localized: "anp.week.checking")).font(.caption)
                }
            }
            if let week = anp.displayWeek {
                if anp.isStaleVerified, let verification = anp.verification {
                    Text(String(localized: "anp.week.verified_no_update", defaultValue: "Checked \(AnpWeekText.age(since: Date(timeIntervalSince1970: TimeInterval(verification.verifiedAtEpochSeconds)))). ANP has not published a newer week."))
                        .font(.caption2)
                        .foregroundStyle(.secondary)
                } else if anp.isStale {
                    Text(String(localized: "anp.week.stale", defaultValue: "Data for \(AnpWeekText.range(week)) is out of date. Pull to refresh."))
                        .font(.caption2)
                        .foregroundStyle(.orange)
                } else if anp.readiness == .partial {
                    Text(String(localized: "anp.week.partial_state"))
                        .font(.caption2)
                        .foregroundStyle(.secondary)
                }
                if !anp.isDisplayingLatest {
                    Text(String(localized: "anp.week.older_selected"))
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
        let transfer = progress.expectedBytes.map { total in
            String(localized: "android.map_downloads_progress", defaultValue: "\(received) of \(AnpWeekText.bytes(total))")
        } ?? received
        switch progress.stage {
        case .summary: return String(localized: "anp.week.downloading_averages", defaultValue: "Downloading city averages: \(transfer)")
        case .stations: return String(localized: "anp.week.downloading_prices", defaultValue: "Downloading station prices: \(transfer)")
        case .importing: return String(localized: "android.sync_progress_importing")
        case .saving: return String(localized: "anp.week.saving")
        }
    }

    private func noticeText(_ notice: AnpSurveyModel.Notice) -> String {
        switch notice {
        case .updated(let week): return String(localized: "anp.week.downloaded", defaultValue: "Week \(week) downloaded and verified.")
        case .partial(let week): return String(localized: "anp.week.partial_retry", defaultValue: "Week \(week) downloaded without station prices. Try again later.")
        case .verifiedNoNewWeek(let week): return String(localized: "anp.week.verified_latest", defaultValue: "Confirmed: \(week) is the latest ANP week.")
        case .alreadyDownloaded(let week): return String(localized: "anp.week.already_local", defaultValue: "Week \(week) is already on this device.")
        }
    }
}

public struct WeekPickerSheetView: View {
    @Environment(\.dismiss) private var dismiss
    @ObservedObject var viewModel: AppStateViewModel

    public var body: some View {
        NavigationStack {
            AnpWeekPickerContent(app: viewModel, anp: viewModel.anpSurvey)
                .navigationTitle(String(localized: "anp.week.picker"))
                .toolbar {
                    ToolbarItem(placement: .cancellationAction) {
                        Button(String(localized: "android.feed_close")) { dismiss() }
                    }
                    ToolbarItem(placement: .primaryAction) {
                        Button {
                            viewModel.anpSurvey.loadCatalog()
                        } label: {
                            Image(systemName: "arrow.clockwise")
                        }
                        .accessibilityLabel(String(localized: "anp.week.refresh_list"))
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
            Section(String(localized: "anp.week.active")) {
                if let week = anp.displayWeek {
                    Text(AnpWeekText.range(week)).font(.headline)
                    Text(anp.isDisplayingLatest ? String(localized: "anp.week.latest_downloaded") : String(localized: "anp.week.selected"))
                        .font(.caption)
                        .foregroundStyle(.secondary)
                } else {
                    Text(String(localized: "anp.week.no_downloads")).foregroundStyle(.secondary)
                }
                AnpSyncStatusView(anp: anp)
                if let latest = anp.catalog.first {
                    Button(String(localized: "anp.week.use_latest", defaultValue: "Use the latest week (\(AnpWeekText.compact(latest.week)))")) { anp.select(latest) }
                        .disabled(anp.isSyncing)
                }
                if let check = anp.lastCatalogCheck {
                    Text(String(localized: "anp.week.catalog_checked", defaultValue: "ANP week list checked \(AnpWeekText.age(since: check))."))
                        .font(.caption2)
                        .foregroundStyle(.secondary)
                }
            }

            if !anp.manifests.isEmpty {
                Section(String(localized: "anp.week.downloads_size", defaultValue: "Downloaded on this device (\(AnpWeekText.bytes(anp.storedBytes)))")) {
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
                Section { ProgressView(String(localized: "anp.week.loading")) }
            } else if let error = anp.catalogError {
                Section {
                    Text(error).foregroundStyle(.secondary)
                    Button(String(localized: "android.feed_retry")) { anp.loadCatalog() }
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
                Text(String(localized: "anp.week.verification"))
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
            stationText = String(localized: "anp.week.station_price_count", defaultValue: "Station prices: \(manifest.stationRowCount.formatted()) (\(scope.rawValue))")
        } else {
            stationText = String(localized: "anp.week.no_station_prices")
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
                    Text(String(localized: "anp.week.row_summary", defaultValue: "Averages: \(manifest.averageRowCount.formatted()) · \(stationText)"))
                        .font(.caption)
                    Text(String(localized: "anp.week.transfer_summary", defaultValue: "Downloaded \(AnpWeekText.age(since: downloadedAt)) · \(AnpWeekText.bytes(manifest.downloadedBytes)) transferred"))
                        .font(.caption2)
                        .foregroundStyle(.secondary)
                    if manifest.quarantinedRowCount > 0 {
                        Text(String(localized: "anp.week.quarantined_rows", defaultValue: "Rows skipped due to unknown labels: \(manifest.quarantinedRowCount.formatted())"))
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
                        Text(String(localized: "android.week_picker_updated_at", defaultValue: "Updated \(AnpWeekText.day(published))"))
                            .font(.caption2)
                            .foregroundStyle(.secondary)
                    }
                }
                Spacer()
                if syncing {
                    ProgressView()
                } else if downloaded != nil {
                    Label(String(localized: "android.week_picker_downloaded_tag"), systemImage: "checkmark.circle")
                        .font(.caption)
                        .foregroundStyle(AbastevoTheme.accentLight)
                }
            }
            if let note = entry.operationalNote {
                Text(String(localized: "android.week_picker_operational_note", defaultValue: "Note: \(note)"))
                    .font(.caption2)
                    .foregroundStyle(.orange)
            }
            HStack {
                Button(String(localized: "anp.week.use")) { anp.select(entry) }
                    .buttonStyle(.borderedProminent)
                    .tint(AbastevoTheme.accentLight)
                if downloaded == nil {
                    Button(String(localized: "anp.week.download_only")) { anp.download(entry) }
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
                .navigationTitle(String(localized: "android.history_title"))
                .toolbar {
                    ToolbarItem(placement: .cancellationAction) {
                        Button(String(localized: "android.feed_close")) { dismiss() }
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
                    Label(String(localized: "anp.history.offline"), systemImage: "wifi.slash")
                        .font(.caption)
                }
                Picker(String(localized: "android.history_fuel_product_label"), selection: $fuel) {
                    ForEach(AnpOfficialRules.productDisplayOrder) { product in
                        Text(product.localizedName).tag(product)
                    }
                }
                Text(app.selectedMunicipality?.displayTitle ?? String(localized: "android.feed_choose_city"))
                    .font(.headline)
            }
            if isLoading {
                Section { ProgressView() }
            } else if app.selectedMunicipality == nil {
                Section {
                    Button(String(localized: "android.feed_choose_city")) { app.activeSheet = .locationPicker }
                }
            } else {
                switch history {
                case .available(let entries):
                    Section(String(localized: "anp.history.weekly_average")) {
                        AnpTrendBars(entries: entries, selectedWeekId: $selectedWeekId)
                            .frame(height: 140)
                        if let selected = entries.first(where: { $0.week.id == selectedWeekId }) {
                            Text("\(AnpWeekText.compact(selected.week)): \(selected.averageMilli.map { AnpPriceRounding.formatBrl(milli: $0, locale: .current) } ?? "—")")
                                .font(.caption.bold())
                        }
                    }
                    Section(String(localized: "anp.week.weeks")) {
                        ForEach(entries) { entry in
                            HStack {
                                Text(AnpWeekText.compact(entry.week))
                                Spacer()
                                Text(entry.averageMilli.map { AnpPriceRounding.formatBrl(milli: $0, locale: .current) } ?? String(localized: "anp.history.price_unavailable"))
                                    .font(.headline)
                            }
                        }
                    }
                case .insufficient(let weeks):
                    Section {
                        Text(String(localized: "anp.history.insufficient", defaultValue: "History requires at least two downloaded weeks for this fuel and city. Available: \(weeks.formatted())."))
                            .foregroundStyle(.secondary)
                        Button(String(localized: "anp.history.more_weeks")) { app.activeSheet = .weekPicker }
                    }
                case nil:
                    Section { Text(String(localized: "anp.history.unavailable")).foregroundStyle(.secondary) }
                }
            }
            Section {
                if let url = URL(string: OfficialAnpAverage.anpOfficialSourceUrl) {
                    Link(String(localized: "anp.history.source"), destination: url)
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
                        .accessibilityValue(entry.averageMilli.map { AnpPriceRounding.formatBrl(milli: $0, locale: .current) } ?? "")
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
            Text(String(localized: "anp.station_history.title", defaultValue: "Station ANP history · \(fuel.localizedName)"))
                .font(.headline)
            switch state {
            case .loading:
                ProgressView()
            case .unavailable:
                Text(viewModel.isOfflineSimulation
                     ? String(localized: "anp.station_history.offline")
                     : String(localized: "anp.station_history.unavailable"))
                    .font(.subheadline)
                    .foregroundStyle(.secondary)
            case .loaded:
                if entries.isEmpty {
                    Text(String(localized: "anp.station_history.empty"))
                        .font(.subheadline)
                        .foregroundStyle(.secondary)
                }
                ForEach(entries) { entry in
                    HStack {
                        VStack(alignment: .leading, spacing: 2) {
                            Text(AnpWeekText.compact(entry.surveyWeek)).font(.subheadline)
                            Text(String(localized: "anp.station_history.collected", defaultValue: "Collected on \(AnpWeekText.day(entry.collectedOn))"))
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

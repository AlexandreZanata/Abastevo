import Foundation
import Combine
import UserNotifications
import AnpFuelCore

/// ANP survey weeks, downloads, active week, city averages, history and local
/// price-drop alerts. Android references: `WeekPickerViewModel`,
/// `ActiveSurveyWeekViewModel`, `HomeViewModel.refresh`, `HistoryViewModel`,
/// `SyncWorker` / `AutoDownloadLatestWeekUseCase` and `EvaluatePriceDropAlertsUseCase`.
@MainActor
public final class AnpSurveyModel: ObservableObject {
    public enum Notice: Equatable {
        case updated(String)
        case partial(String)
        case verifiedNoNewWeek(String)
        case alreadyDownloaded(String)
    }

    @Published public private(set) var catalog: [AnpSurveyCatalogEntry] = []
    @Published public private(set) var isLoadingCatalog = false
    @Published public private(set) var catalogError: String?
    @Published public private(set) var manifests: [AnpWeekManifest] = []
    @Published public private(set) var syncingWeekId: String?
    @Published public private(set) var isCheckingLatest = false
    @Published public private(set) var progress: AnpDownloadProgress?
    @Published public private(set) var syncError: String?
    @Published public private(set) var notice: Notice?
    @Published public private(set) var displayWeek: AnpSurveyWeekRef?
    @Published public private(set) var cityAverages: [AnpMunicipalityAverage] = []
    @Published public private(set) var cityAvailability: AnpCityAvailability?
    @Published public private(set) var displayStations: [AnpStationWeekPrice] = []
    @Published public private(set) var weekLabel = String(localized: "anp.survey")
    @Published public private(set) var lastCatalogCheck: Date?
    @Published public private(set) var verification: AnpManualVerification?

    let service: AnpSurveyService
    private weak var host: AppStateViewModel?
    private var syncTask: Task<Void, Never>?
    private var stationsLoad: Task<Void, Never>?

    init(service: AnpSurveyService = AnpSurveyService()) {
        self.service = service
    }

    func attach(_ host: AppStateViewModel) {
        self.host = host
        reloadLocal()
    }

    // MARK: - Derived state

    public var isSyncing: Bool { syncingWeekId != nil || isCheckingLatest }

    public var latestDownloadedWeek: AnpSurveyWeekRef? { AnpOfficialRules.latestWeek(manifests) }

    public var readiness: AnpDataReadiness {
        AnpOfficialRules.readiness(manifests: manifests, scope: host?.selectedMunicipality?.state, todayIso: AnpCalendar.localTodayIso())
    }

    public var isStale: Bool { readiness == .stale }

    public var isDisplayingLatest: Bool { displayWeek != nil && displayWeek == latestDownloadedWeek }

    /// Android `HomeUiState.isStaleVerified`: a manual check confirmed ANP has no newer week.
    public var isStaleVerified: Bool {
        guard isStale, let verification, let displayWeek, displayWeek == latestDownloadedWeek else { return false }
        return verification.weekId == displayWeek.id
    }

    public var displayManifest: AnpWeekManifest? {
        guard let displayWeek else { return nil }
        return manifests.first { $0.week == displayWeek }
    }

    public var activeWeekId: String { host?.snapshot.selectedWeekId ?? "" }

    public func manifest(for entry: AnpSurveyCatalogEntry) -> AnpWeekManifest? {
        manifests.first { $0.week == entry.week }
    }

    public var storedBytes: Int64 { manifests.reduce(0) { $0 + $1.storedBytes } }

    /// Official station prices of the displayed downloaded week, matched by CNPJ.
    public func stationWeekPrices(cnpj: String) -> [AnpStationWeekPrice] {
        guard let displayWeek else { return [] }
        return AnpOfficialRules.stationPrices(rows: AnpWeekRows(week: displayWeek, averages: [], stations: displayStations), cnpj: cnpj)
    }

    // MARK: - Local data

    func reloadLocal() {
        manifests = service.store.manifests()
        let state = service.store.loadState()
        lastCatalogCheck = state.lastCatalogCheckEpochSeconds.map { Date(timeIntervalSince1970: TimeInterval($0)) }
        verification = state.verification
        displayWeek = AnpOfficialRules.displayWeek(activeWeekId: activeWeekId, manifests: manifests)
        weekLabel = displayWeek.map(AnpWeekText.compact) ?? String(localized: "anp.survey")
        cityContextChanged()
    }

    /// Recomputes the selected city's averages and reloads the week's station rows off the main thread.
    func cityContextChanged() {
        guard let city = host?.selectedMunicipality, let week = displayWeek else {
            cityAverages = []
            cityAvailability = nil
            displayStations = []
            return
        }
        let averages = service.store.averages(weekId: week.id) ?? []
        cityAverages = AnpOfficialRules.cityAverages(averages, city: city)
        if cityAverages.isEmpty {
            let others = manifests.filter { $0.week != week }.prefix(8).compactMap { manifest -> AnpWeekRows? in
                guard let rows = service.store.averages(weekId: manifest.week.id) else { return nil }
                return AnpWeekRows(week: manifest.week, averages: rows)
            }
            cityAvailability = AnpOfficialRules.availability(
                displayWeek: AnpWeekRows(week: week, averages: averages), otherWeeks: Array(others), city: city
            )
        } else {
            cityAvailability = .hasData
        }
        stationsLoad?.cancel()
        let store = service.store
        let weekId = week.id
        stationsLoad = Task { [weak self] in
            let rows = await Task.detached(priority: .utility) { store.stations(weekId: weekId) ?? [] }.value
            guard !Task.isCancelled, let self, self.displayWeek?.id == weekId else { return }
            self.displayStations = rows
        }
        if manifests.count != service.store.manifests().count { manifests = service.store.manifests() }
    }

    /// UC-006 city history for one fuel from every downloaded week.
    public func cityHistory(product: FuelProduct) async -> AnpCityHistory? {
        guard let city = host?.selectedMunicipality else { return nil }
        let store = service.store
        let weeks = manifests.map(\.week)
        return await Task.detached(priority: .userInitiated) { () -> AnpCityHistory in
            let rows = weeks.compactMap { week -> AnpWeekRows? in
                guard let averages = store.averages(weekId: week.id) else { return nil }
                return AnpWeekRows(week: week, averages: averages)
            }
            return AnpOfficialRules.history(weeks: rows, city: city, product: product)
        }.value
    }

    // MARK: - Catalog and downloads

    public func loadCatalog() {
        guard let host, !isLoadingCatalog else { return }
        guard !host.isOfflineSimulation else {
            catalogError = String(localized: "anp.catalog.offline")
            return
        }
        isLoadingCatalog = true
        catalogError = nil
        let service = self.service
        Task { [weak self] in
            do {
                let entries = try await service.discoverCatalog(automatic: false)
                guard let self else { return }
                self.catalog = entries
                self.isLoadingCatalog = false
                self.reloadState()
            } catch {
                guard let self else { return }
                self.isLoadingCatalog = false
                self.catalogError = Self.message(for: error)
            }
        }
    }

    /// Selects the week as active and downloads it when needed (Android `selectWeek`).
    public func select(_ entry: AnpSurveyCatalogEntry) {
        guard !isSyncing else { return }
        setActiveWeek(entry.week.id)
        if manifest(for: entry) != nil {
            notice = .alreadyDownloaded(AnpWeekText.compact(entry.week))
        }
        runSync(entry, automatic: false, activateOnSuccess: true)
    }

    /// Downloads in place without changing the active week (Android `downloadWeek`).
    public func download(_ entry: AnpSurveyCatalogEntry) {
        guard !isSyncing else { return }
        runSync(entry, automatic: false, activateOnSuccess: false)
    }

    public func delete(weekId: String) {
        guard syncingWeekId != weekId else { return }
        service.store.delete(weekId: weekId)
        if activeWeekId == weekId { setActiveWeek("") }
        reloadLocal()
    }

    public func cancelSync() {
        syncTask?.cancel()
    }

    public func clearMessages() {
        notice = nil
        syncError = nil
    }

    /// Manual sync (pull to refresh): the newest listing week, or a verified "no new week".
    public func manualSync() async {
        guard let host, !isSyncing else { return }
        guard !host.isOfflineSimulation else {
            syncError = String(localized: "anp.sync.offline")
            return
        }
        isCheckingLatest = true
        syncError = nil
        notice = nil
        do {
            let entries = try await service.discoverCatalog(automatic: false)
            catalog = entries
            isCheckingLatest = false
            guard let latest = entries.first else { return }
            if service.isCurrent(latest, scope: host.selectedMunicipality?.state) != nil {
                service.store.mutateState {
                    $0.verification = AnpManualVerification(weekId: latest.week.id, verifiedAtEpochSeconds: Int64(Date().timeIntervalSince1970))
                }
                notice = .verifiedNoNewWeek(AnpWeekText.compact(latest.week))
                reloadState()
                return
            }
            runSync(latest, automatic: false, activateOnSuccess: activeWeekId.isEmpty || activeWeekId == latestDownloadedWeek?.id)
            await syncTask?.value
        } catch {
            isCheckingLatest = false
            syncError = Self.message(for: error)
        }
    }

    /// Weekly automatic refresh on foreground, Wi-Fi only (Android periodic `SyncWorker`).
    public func automaticSyncIfDue(now: Date = Date()) {
        guard let host, !host.isOfflineSimulation, !isSyncing, host.snapshot.termsAccepted else { return }
        let state = service.store.loadState()
        guard AnpOfficialRules.shouldRunAutomaticCheck(
            lastCheckEpochSeconds: state.lastCatalogCheckEpochSeconds, nowEpochSeconds: Int64(now.timeIntervalSince1970)
        ) else { return }
        isCheckingLatest = true
        let service = self.service
        let scope = host.selectedMunicipality?.state
        Task { [weak self] in
            let discovered = try? await service.discoverCatalog(automatic: true)
            guard let self else { return }
            self.isCheckingLatest = false
            guard let entries = discovered, let latest = entries.first else { return }
            self.catalog = entries
            if self.activeWeekId == latest.week.id || self.activeWeekId.isEmpty, service.isCurrent(latest, scope: scope) != nil {
                self.reloadState()
                return
            }
            self.runSync(latest, automatic: true, activateOnSuccess: true)
        }
    }

    private func runSync(_ entry: AnpSurveyCatalogEntry, automatic: Bool, activateOnSuccess: Bool) {
        guard syncingWeekId == nil else { return }
        let scope = host?.selectedMunicipality?.state
        syncingWeekId = entry.week.id
        syncError = nil
        progress = AnpDownloadProgress(stage: .summary)
        let service = self.service
        syncTask = Task { [weak self] in
            do {
                let outcome = try await service.sync(entry: entry, scope: scope, automatic: automatic) { value in
                    Task { @MainActor [weak self] in
                        guard let self, self.syncingWeekId == entry.week.id else { return }
                        self.progress = value
                    }
                }
                guard let self else { return }
                self.finishSync()
                if activateOnSuccess { self.setActiveWeek(entry.week.id) }
                switch outcome {
                case .downloaded(_, let stationsFailed):
                    let label = AnpWeekText.compact(entry.week)
                    self.notice = stationsFailed ? .partial(label) : .updated(label)
                    self.reloadLocal()
                    await self.evaluatePriceDrops()
                case .alreadyDownloaded:
                    self.reloadLocal()
                }
            } catch {
                guard let self else { return }
                self.finishSync()
                if !(error is CancellationError) && !automatic {
                    self.syncError = Self.message(for: error)
                }
                self.reloadLocal()
            }
        }
    }

    private func finishSync() {
        syncingWeekId = nil
        progress = nil
    }

    private func reloadState() {
        let state = service.store.loadState()
        lastCatalogCheck = state.lastCatalogCheckEpochSeconds.map { Date(timeIntervalSince1970: TimeInterval($0)) }
        verification = state.verification
    }

    private func setActiveWeek(_ id: String) {
        guard let host, host.snapshot.selectedWeekId != id else { return }
        host.snapshot.selectedWeekId = id
        host.persist()
        reloadLocal()
    }

    // MARK: - Price-drop alerts (UC-014 / BR-025)

    public func isAlertEnabled(_ product: FuelProduct) -> Bool {
        host?.snapshot.priceDropAlerts.contains(product.rawValue) ?? false
    }

    /// Enabling asks for notification permission at this user action only.
    public func setAlert(_ product: FuelProduct, enabled: Bool) async -> Bool {
        guard let host else { return false }
        if enabled {
            let center = UNUserNotificationCenter.current()
            let settings = await center.notificationSettings()
            var allowed = settings.authorizationStatus == .authorized || settings.authorizationStatus == .provisional
            if settings.authorizationStatus == .notDetermined {
                allowed = (try? await center.requestAuthorization(options: [.alert, .sound])) ?? false
            }
            if !host.snapshot.priceDropAlerts.contains(product.rawValue) {
                host.snapshot.priceDropAlerts.append(product.rawValue)
                host.persist()
            }
            return allowed
        }
        host.snapshot.priceDropAlerts.removeAll { $0 == product.rawValue }
        host.persist()
        return true
    }

    func evaluatePriceDrops() async {
        guard let host, let city = host.selectedMunicipality, let current = displayWeek else { return }
        let products = host.snapshot.priceDropAlerts.compactMap { FuelProduct(rawValue: $0) }
        guard !products.isEmpty else { return }
        let center = UNUserNotificationCenter.current()
        let settings = await center.notificationSettings()
        guard settings.authorizationStatus == .authorized || settings.authorizationStatus == .provisional else { return }
        guard let previous = AnpOfficialRules.previousWeek(current: current, weeks: manifests.map(\.week)) else { return }
        let store = service.store
        let rows = await Task.detached(priority: .utility) { () -> (AnpWeekRows, AnpWeekRows) in
            (AnpWeekRows(week: current, averages: store.averages(weekId: current.id) ?? [], stations: store.stations(weekId: current.id) ?? []),
             AnpWeekRows(week: previous, averages: store.averages(weekId: previous.id) ?? [], stations: store.stations(weekId: previous.id) ?? []))
        }.value
        let drops = AnpOfficialRules.priceDrops(
            products: products, city: city, current: rows.0, previous: rows.1,
            notifiedWeekByAlert: store.loadState().notifiedWeekByAlert
        )
        for drop in drops {
            let content = UNMutableNotificationContent()
            content.title = String(localized: "anp.alert.drop", defaultValue: "Price drop: \(drop.product.localizedName)")
            content.body = String(localized: "anp.alert.survey_price", defaultValue: "\(city.displayTitle): \(AnpPriceRounding.formatBrl(milli: drop.currentMilli, locale: .current)) in the \(AnpWeekText.compact(current)) ANP survey (previously \(AnpPriceRounding.formatBrl(milli: drop.previousMilli, locale: .current))).")
            let request = UNNotificationRequest(identifier: "\(drop.alertKey):\(drop.weekId)", content: content, trigger: nil)
            do {
                try await center.add(request)
                store.mutateState { $0.notifiedWeekByAlert[drop.alertKey] = drop.weekId }
            } catch {
                continue
            }
        }
    }

    // MARK: - Messages

    static func message(for error: Error) -> String {
        if let client = error as? AnpSurveyClientError {
            switch client {
            case .refusedUrl: return String(localized: "anp.error.url")
            case .httpStatus(let code): return String(localized: "anp.error.http", defaultValue: "The ANP website returned an error (\(String(code))). Try again later.")
            case .tooLarge: return String(localized: "anp.error.size")
            case .empty, .notSpreadsheet, .unsupportedContentType: return String(localized: "anp.error.file")
            case .transport: return String(localized: "anp.error.connect")
            case .emptyCatalog: return String(localized: "anp.error.empty")
            }
        }
        if let xlsx = error as? AnpXlsxError {
            switch xlsx {
            case .weekMismatch: return String(localized: "anp.error.week")
            case .limitExceeded, .entryTooLarge: return String(localized: "anp.error.read_limit")
            default: return String(localized: "anp.error.read")
            }
        }
        if error is AnpStoreError { return String(localized: "anp.error.save") }
        return String(localized: "anp.error.download")
    }
}

/// Display official survey dates in UTC with the user's regional format.
enum AnpWeekText {
    static func compact(_ week: AnpSurveyWeekRef) -> String {
        format(startIso: week.startIso, endIso: week.endIso, dateStyle: .abbreviated)
    }

    static func range(_ week: AnpSurveyWeekRef) -> String {
        range(startIso: week.startIso, endIso: week.endIso)
    }

    static func range(startIso: String, endIso: String) -> String {
        format(startIso: startIso, endIso: endIso, dateStyle: .numeric)
    }

    static func day(_ iso: String) -> String {
        guard let date = AnpCalendar.parseIsoDay(iso) else { return "—" }
        return date.formatted(Date.FormatStyle(date: .numeric, time: .omitted, timeZone: .gmt))
    }

    private static func format(startIso: String, endIso: String, dateStyle: Date.IntervalFormatStyle.DateStyle) -> String {
        guard let start = AnpCalendar.parseIsoDay(startIso),
              let end = AnpCalendar.parseIsoDay(endIso), start <= end else { return "—" }
        if start == end { return day(startIso) }
        return (start..<end).formatted(Date.IntervalFormatStyle(date: dateStyle, time: .omitted, timeZone: .gmt))
    }

    static func bytes(_ count: Int64) -> String {
        count.formatted(.byteCount(style: .file))
    }

    static func age(since date: Date) -> String {
        date.formatted(.relative(presentation: .numeric, unitsStyle: .wide))
    }
}

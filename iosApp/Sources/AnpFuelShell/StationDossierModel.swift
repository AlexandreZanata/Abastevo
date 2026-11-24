import SwiftUI
import AnpFuelCore

@MainActor
final class StationDossierModel: ObservableObject {
    enum IdentityState: Equatable {
        case loading, resolved, unavailable, notFound
    }

    @Published private(set) var identity: StationDossier?
    @Published private(set) var identityState: IdentityState = .loading
    @Published private(set) var officialRead: OfficialPriceRead?
    @Published private(set) var community: [FuelProduct: StationDossierCommunityPrice] = [:]
    @Published private(set) var priceState: PublicReadState = .loading
    @Published private(set) var loadedOrigin: String?
    @Published private(set) var loadedSeedId: String?

    private var generation = UUID()

    var canonicalStation: StationRecord? {
        guard identityState == .resolved else { return nil }
        return identity?.station
    }

    func matches(seed: StationRecord, origin: String) -> Bool {
        loadedSeedId == seed.id && loadedOrigin == origin
    }

    func load(_ seed: StationRecord, on host: AppStateViewModel) async {
        let token = UUID()
        generation = token
        let origin = host.snapshot.apiEnvironment.origin
        loadedOrigin = origin
        loadedSeedId = seed.id
        identityState = .loading
        identity = nil
        officialRead = nil
        community = [:]
        priceState = .loading
        guard let key = StationDossierIdentity.from(seed) else {
            identityState = .unavailable
            priceState = .unavailable
            return
        }
        do {
            let client = try host.socialReadClient()
            let resolved = try await client.fetchStationDossier(key)
            guard active(host, origin: origin, token: token) else { return }
            identity = resolved
            identityState = .resolved

            let prices = try await client.fetchStationDossierPrices(stationId: resolved.station.id)
            guard active(host, origin: origin, token: token) else { return }
            officialRead = OfficialPriceRead(prices: prices.official, source: .live)
            community = prices.community
            priceState = .loaded
            host.publicReadCache.saveStationPrices(
                origin: origin, stationId: resolved.station.id,
                municipalityCode: resolved.station.municipalityCode, prices: prices.official
            )
        } catch {
            guard active(host, origin: origin, token: token) else { return }
            if identity == nil, let error = error as? HttpClientError, case .httpStatus(404, _) = error {
                identityState = .notFound
                priceState = .unavailable
                return
            }
            if identity == nil { identityState = .unavailable }
            priceState = .unavailable
            let knownStation = identity?.station ?? seed
            if case .canonical = key,
               let cached = host.publicReadCache.stationPrices(origin: origin, stationId: knownStation.id) {
                officialRead = OfficialPriceRead(
                    prices: cached.prices.map(\.officialPrice),
                    source: .cached(Date(timeIntervalSince1970: TimeInterval(cached.savedAtEpochSeconds)))
                )
                return
            }
            if let week = host.anpSurvey.displayWeek {
                let rows = host.anpSurvey.stationWeekPrices(cnpj: knownStation.cnpj)
                if !rows.isEmpty {
                    officialRead = OfficialPriceRead(prices: AnpOfficialRules.officialPrices(rows, week: week), source: .anpDownload(week))
                }
            }
        }
    }

    /// The district feed is a fallback only for the same verified station and fuel, never for an explicit projection refusal.
    func feedFallback(for fuel: FuelProduct, on host: AppStateViewModel) -> CommunityPriceObservation? {
        guard community[fuel] == nil, let station = canonicalStation,
              host.selectedMunicipality?.ibgeCode == station.municipalityCode else { return nil }
        return host.communityObservation(stationId: station.id, product: fuel)
    }

    private func active(_ host: AppStateViewModel, origin: String, token: UUID) -> Bool {
        !Task.isCancelled && host.snapshot.apiEnvironment.origin == origin && generation == token
    }
}

@MainActor
final class StationCatalogSearchModel: ObservableObject {
    struct Scope: Hashable {
        let origin: String
        let municipalityCode: String?
        let query: String
        let offline: Bool
    }

    @Published private(set) var rows: [StationRecord] = []
    @Published private(set) var cursor: String?
    @Published private(set) var isLoading = false
    @Published private(set) var failure: HttpClientError?
    @Published private(set) var showsLoadedFallback = false
    private var scope: Scope?
    private var generation = UUID()
    private var usedCursors: Set<String> = []
    private let pageSize: Int

    init(pageSize: Int = 20) { self.pageSize = pageSize }

    func cancel() {
        scope = nil
        generation = UUID()
        usedCursors = []
        isLoading = false
        failure = nil
        showsLoadedFallback = false
        cursor = nil
        rows = []
    }

    func isCurrent(_ request: Scope) -> Bool { scope == request }

    func activeScope(for raw: String, on host: AppStateViewModel) -> Scope {
        Scope(
            origin: host.snapshot.apiEnvironment.origin, municipalityCode: host.selectedMunicipality?.ibgeCode,
            query: StationDossierCodec.normalizedSearch(raw), offline: host.isOfflineSimulation
        )
    }

    func load(_ requested: Scope, host: AppStateViewModel, reset: Bool, debounce: Bool = false) async {
        guard !requested.query.isEmpty, let city = host.selectedMunicipality,
              city.ibgeCode == requested.municipalityCode else { return }
        if !reset, scope != requested || isLoading || cursor == nil { return }
        if reset {
            if scope != requested { rows = [] }
            scope = requested
            generation = UUID()
            cursor = nil
            usedCursors = []
        }
        let token = generation
        let continuation = reset ? nil : cursor
        isLoading = true
        failure = nil
        showsLoadedFallback = false
        defer { if generation == token { isLoading = false } }

        do {
            if debounce { try await Task.sleep(for: .milliseconds(300)) }
            guard active(requested, on: host, token: token) else { return }
            let page = try await host.socialReadClient().searchStationDirectory(
                in: city, query: requested.query, cursor: continuation, pageSize: pageSize
            )
            guard active(requested, on: host, token: token) else { return }
            if let next = page.nextCursor, next == continuation || usedCursors.contains(next) {
                throw HttpClientError.httpStatus(400, "station.bad-cursor")
            }
            if let continuation { usedCursors.insert(continuation) }
            var seen = Set(reset ? [] : rows.map(\.id))
            let gained = page.stations.filter { seen.insert($0.id).inserted }
            rows = reset ? gained : rows + gained
            cursor = page.nextCursor
        } catch {
            guard active(requested, on: host, token: token) else { return }
            let http = error as? HttpClientError ?? .unavailable
            failure = http
            if case .httpStatus(400, "station.bad-cursor") = http { cursor = nil }
            if reset, rows.isEmpty, canUseFallback(http) {
                let cached = host.publicReadCache.directory(origin: requested.origin, municipalityCode: city.ibgeCode)
                let available = host.stations.isEmpty ? (cached?.stations ?? []) : host.stations
                rows = Array(available.lazy.filter {
                    $0.municipalityCode == city.ibgeCode && $0.name.localizedStandardContains(requested.query)
                }.prefix(pageSize))
                showsLoadedFallback = !rows.isEmpty
            }
        }
    }

    private func canUseFallback(_ error: HttpClientError) -> Bool {
        switch error {
        case .unavailable: return true
        case .httpStatus(let status, _): return status == 408 || status == 429 || (500...599).contains(status)
        default: return false
        }
    }

    private func active(_ request: Scope, on host: AppStateViewModel, token: UUID) -> Bool {
        !Task.isCancelled && scope == request && generation == token
            && activeScope(for: request.query, on: host) == request
    }
}

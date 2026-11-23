import Foundation
import Combine
import SwiftUI
import CoreLocation
import AnpFuelCore

public struct AppValidationError: LocalizedError {
    public let message: String

    public init(_ message: String) {
        self.message = message
    }

    public var errorDescription: String? { message }
}

/// Central MainActor view-model coordinating all Abastevo iOS flows.
@MainActor
public final class AppStateViewModel: ObservableObject {
    @Published public var snapshot: PersistedAppSnapshot
    public let municipalityCatalog: MunicipalityCatalog
    @Published public var selectedTab: AppTab = .explore
    @Published public var activeSheet: AppModalSheet?
    @Published public var selectedStation: StationRecord?
    @Published public var sortOrder: StationSortOrder = .lowestPrice
    @Published public var feedSortOrder: FeedSortOrder = .recent {
        didSet { if oldValue != feedSortOrder { reloadCityData() } }
    }
    @Published public var isOfflineSimulation: Bool = false {
        didSet { if oldValue != isOfflineSimulation { reloadCityData() } }
    }
    @Published public var isSyncing: Bool = false
    @Published public var bannerMessage: String?

    // Auth & Identity
    @Published public var authFlowState: AuthFlowState = .loggedOut
    @Published public var signedInAlias: String?
    @Published public var authStatusMessage: String?

    // Location & Map
    @Published public var lastDeviceCoordinate: CLLocationCoordinate2D?
    @Published public var lastLocationRisk: PortableLocation.FixRisk?
    @Published public var isVoiceGuidanceEnabled: Bool = true

    // Community & Outbox
    @Published public var communityFeed: [CommunityPriceObservation] = []
    @Published public var stations: [StationRecord] = []
    @Published public var evSites: [EvChargingSite] = []
    @Published private(set) var stationReadState: PublicReadState = .loading
    @Published private(set) var communityReadState: PublicReadState = .loading
    @Published private(set) var nextStationCursor: String?
    @Published private(set) var isLoadingMoreStations = false
    @Published private(set) var stationPageUnavailable = false

    // Navigation paths per tab (reset on tab re-tap per B-BR-NAV-01/02)
    @Published public var explorePath = NavigationPath()
    @Published public var communityPath = NavigationPath()
    @Published public var mapPath = NavigationPath()
    @Published public var profilePath = NavigationPath()

    private let store: IosLocalStore
    private let keychain: IosKeychainSecretStore
    private let proofSigner: IosDeviceProofSigner
    private let httpClient: AbastevoHttpClient
    private let authFlow: AuthFlow
    private let photoCache: IosTransientPhotoCache
    private let photoFlow: PhotoFlow
    private let locationManager = IosLocationManager()
    private let voiceGuidance = IosVoiceGuidance()
    private var directoryRead: Task<Void, Never>?
    private var communityRead: Task<Void, Never>?
    private var cityReadGeneration = UUID()

    /// StoreKit 2 Premium; the server verifies every App Store transaction.
    public lazy var premiumStore: PremiumStoreModel = PremiumStoreModel(
        client: httpClient,
        session: { [weak self] in self?.authFlow.currentSession() },
        signer: { [weak self] in self?.premiumRequestSigner() }
    )

    public init(store: IosLocalStore = IosLocalStore()) {
        self.store = store
        self.municipalityCatalog = (try? MunicipalityCatalog(bundle: .main)) ?? .empty
        let loaded = store.load()
        self.snapshot = loaded
        self.keychain = IosKeychainSecretStore()
        self.proofSigner = IosDeviceProofSigner()
        self.httpClient = AbastevoHttpClient(environment: loaded.apiEnvironment)
        let ports = AuthPorts(
            store: keychain,
            keys: keychain,
            clock: SystemWallClock(),
            nonces: RandomNonceSource(),
            api: httpClient
        )
        self.authFlow = AuthFlow(ports: ports)
        let cache = IosTransientPhotoCache(
            directory: IosTransientPhotoCache.defaultDirectory(),
            clock: { Int64(Date().timeIntervalSince1970 * 1000) }
        )
        self.photoCache = cache
        self.photoFlow = PhotoFlow(
            ports: PhotoPorts(
                decoder: IosImageIoPhotoCodec(),
                encoder: IosImageIoPhotoCodec(),
                cache: cache,
                clock: SystemPhotoClock(),
                ids: RandomPhotoIdSource()
            )
        )
        _ = self.photoFlow.sweepExpired()
        rehydrateAuth()
        reloadCityData()
    }

    public var currentLocale: SupportedAppLocale {
        return SupportedAppLocale(rawValue: snapshot.localeCode) ?? .ptBR
    }

    public var selectedMunicipality: Municipality? { municipalityCatalog.municipality(code: snapshot.selectedMunicipalityCode) }

    public var municipalityLabel: String { selectedMunicipality?.displayTitle ?? String(localized: "Escolher cidade") }

    public var surveyWeekLabel: String { String(localized: "Pesquisa ANP") }

    public var sortedStations: [StationRecord] {
        let fuel = snapshot.selectedFuel
        let referenceLocation = lastDeviceCoordinate
        return stations.sorted { lhs, rhs in
            switch sortOrder {
            case .lowestPrice:
                let lp = communityObservation(stationId: lhs.id, product: fuel)?.amountMilli ?? Int64.max
                let rp = communityObservation(stationId: rhs.id, product: fuel)?.amountMilli ?? Int64.max
                if lp != rp { return lp < rp }
                return lhs.name < rhs.name
            case .mostRecent:
                let lt = communityObservation(stationId: lhs.id, product: fuel)?.observedEpochSeconds ?? 0
                let rt = communityObservation(stationId: rhs.id, product: fuel)?.observedEpochSeconds ?? 0
                if lt != rt { return lt > rt }
                return lhs.name < rhs.name
            case .distance:
                guard let referenceLocation else { return lhs.name < rhs.name }
                if lhs.pinTier.allowsMapPin != rhs.pinTier.allowsMapPin { return lhs.pinTier.allowsMapPin }
                guard lhs.pinTier.allowsMapPin else { return lhs.name < rhs.name }
                return lhs.distanceKm(fromLat: referenceLocation.latitude, fromLon: referenceLocation.longitude)
                    < rhs.distanceKm(fromLat: referenceLocation.latitude, fromLon: referenceLocation.longitude)
            case .name:
                return lhs.name.localizedCaseInsensitiveCompare(rhs.name) == .orderedAscending
            }
        }
    }

    public var visibleCommunityFeed: [CommunityPriceObservation] {
        let fuel = snapshot.selectedFuel
        guard let city = selectedMunicipality else { return [] }
        let values = communityFeed.filter { $0.product == fuel && $0.municipalityCode == city.ibgeCode }
        switch feedSortOrder {
        case .recent:
            return values.sorted { $0.observedEpochSeconds > $1.observedEpochSeconds }
        case .cheapest:
            return values.sorted { $0.amountMilli < $1.amountMilli }
        }
    }

    public func communityObservation(stationId: String, product: FuelProduct) -> CommunityPriceObservation? {
        communityFeed.first { $0.stationId == stationId && $0.product == product }
    }

    public var preferredColorScheme: ColorScheme? {
        guard let dark = snapshot.darkThemeEnabled else { return nil }
        return dark ? .dark : .light
    }

    // MARK: - Navigation (B-BR-NAV-01/02)

    public func selectTab(_ tab: AppTab) {
        if selectedTab == tab {
            resetPath(for: tab)
        } else {
            selectedTab = tab
            resetPath(for: tab)
        }
    }

    private func resetPath(for tab: AppTab) {
        switch tab {
        case .explore: explorePath = NavigationPath()
        case .community: communityPath = NavigationPath()
        case .map: mapPath = NavigationPath()
        case .profile: profilePath = NavigationPath()
        }
    }

    // MARK: - Terms & Onboarding

    public func acceptTerms() {
        snapshot.termsAccepted = true
        persist()
    }

    public func completeOnboarding() {
        snapshot.onboardingCompleted = true
        persist()
    }

    // MARK: - Preferences & Catalog

    public func selectMunicipality(_ city: Municipality) {
        snapshot.selectedMunicipalityCode = city.ibgeCode
        persist()
        reloadCityData()
    }

    public func selectFuel(_ fuel: FuelProduct) {
        guard snapshot.selectedFuel != fuel else { return }
        snapshot.selectedFuel = fuel
        persist()
        reloadCityData()
    }

    public func setLocale(_ locale: SupportedAppLocale) {
        snapshot.localeCode = locale.rawValue
        persist()
    }

    public func setTheme(dark: Bool?) {
        snapshot.darkThemeEnabled = dark
        persist()
    }

    public var canSwitchApiEnvironment: Bool {
        keychain.load() == nil && keychain.loadKey() == nil
    }

    public func setApiEnvironment(_ env: ApiEnvironment) {
        guard env != snapshot.apiEnvironment, canSwitchApiEnvironment else { return }
        snapshot.apiEnvironment = env
        httpClient.environment = env
        persist()
        reloadCityData()
    }

    public func togglePriceDropAlert(product: FuelProduct) {
        if let idx = snapshot.priceDropAlerts.firstIndex(of: product.rawValue) {
            snapshot.priceDropAlerts.remove(at: idx)
        } else {
            snapshot.priceDropAlerts.append(product.rawValue)
        }
        persist()
    }

    public func reloadCityData() {
        directoryRead?.cancel()
        communityRead?.cancel()
        let generation = UUID()
        cityReadGeneration = generation
        let municipality = selectedMunicipality
        let fuel = snapshot.selectedFuel
        let order = feedSortOrder
        let client = httpClient

        stations = []
        communityFeed = []
        nextStationCursor = nil
        isLoadingMoreStations = false
        stationPageUnavailable = false
        guard !isOfflineSimulation, let municipality else {
            stationReadState = .unavailable
            communityReadState = .unavailable
            return
        }
        stationReadState = .loading
        communityReadState = .loading

        directoryRead = Task { [weak self] in
            let page = try? await client.fetchStations(in: municipality)
            guard !Task.isCancelled, let self, self.cityReadGeneration == generation else { return }
            self.stations = page?.stations ?? []
            self.nextStationCursor = page?.nextCursor
            self.stationReadState = page == nil ? .unavailable : .loaded
        }
        communityRead = Task { [weak self] in
            let feed = await client.fetchCommunityFeed(
                state: municipality.state, municipalityCode: municipality.ibgeCode, fuel: fuel, sort: order
            )
            guard !Task.isCancelled, let self, self.cityReadGeneration == generation else { return }
            self.communityFeed = feed ?? []
            self.communityReadState = feed == nil ? .unavailable : .loaded
        }
    }

    public func refreshCityData() async {
        reloadCityData()
        let directory = directoryRead
        let community = communityRead
        await directory?.value
        await community?.value
    }

    public func loadMoreStations() async {
        guard !isLoadingMoreStations, let cursor = nextStationCursor, let municipality = selectedMunicipality else { return }
        let generation = cityReadGeneration
        isLoadingMoreStations = true
        stationPageUnavailable = false
        let page = try? await httpClient.fetchStations(in: municipality, after: cursor)
        guard cityReadGeneration == generation else { return }
        isLoadingMoreStations = false
        guard !Task.isCancelled else { return }
        guard let page else {
            stationPageUnavailable = true
            return
        }
        let currentIds = Set(stations.map(\.id))
        stations.append(contentsOf: page.stations.filter { !currentIds.contains($0.id) })
        nextStationCursor = cursor == page.nextCursor ? nil : page.nextCursor
    }

    public func officialPrices(for station: StationRecord) async throws -> [OfficialStationPrice] {
        guard !isOfflineSimulation else { throw HttpClientError.unavailable }
        let generation = cityReadGeneration
        let prices = try await httpClient.fetchOfficialStationPrices(stationId: station.id)
        guard cityReadGeneration == generation else { throw CancellationError() }
        return prices
    }

    // MARK: - One-shot Core Location (no polling / no background)

    public func requestOneShotDeviceLocation() async {
        let (reading, coord) = await locationManager.requestOneShotFix()
        let risk = PortableLocation.classify(
            PortableLocation.FixInput(
                permissionGranted: reading.permissionGranted,
                hasFix: coord != nil,
                sourceInfoPresent: reading.sourceInfoAvailable,
                simulated: reading.simulated,
                accuracyMeters: reading.hasAccuracy ? reading.accuracyMeters : nil,
                fixAgeSeconds: reading.fixAgeSeconds,
                clockSkewSeconds: 0,
                manual: false
            )
        )
        self.lastLocationRisk = risk
        self.lastDeviceCoordinate = coord
    }

    // MARK: - Auth & Account Management

    public func rehydrateAuth() {
        let state = authFlow.rehydrate()
        self.authFlowState = state
        switch state {
        case .active(let session):
            self.signedInAlias = authFlow.currentKey()?.username ?? "Conta \(session.accountId.prefix(6))"
        case .needsRefresh(let session):
            self.signedInAlias = authFlow.currentKey()?.username ?? "Conta \(session.accountId.prefix(6))"
        case .pendingKeyAccount, .loggedOut:
            self.signedInAlias = nil
        }
    }

    public var currentAccountIdOrNil: String? {
        switch authFlowState {
        case .active(let session), .needsRefresh(let session):
            return session.accountId
        case .pendingKeyAccount, .loggedOut:
            return nil
        }
    }

    public func requestEmailOtp(email: String) {
        if isOfflineSimulation {
            authStatusMessage = "Modo offline: conecte-se para solicitar código por e-mail."
            return
        }
        switch authFlow.requestEmailCode(email: email) {
        case .ok:
            authStatusMessage = "Se o e-mail for válido, enviamos um código de 6 dígitos."
        case .err(let code):
            authStatusMessage = "Não foi possível enviar o código (\(code))."
        }
    }

    public func verifyEmailOtp(email: String, code: String) {
        switch authFlow.consumeEmailCode(email: email, code: code) {
        case .ok(let outcome):
            authStatusMessage = outcome.created ? "Conta gratuita criada e conectada." : "Sessão conectada com sucesso."
            rehydrateAuth()
        case .err(let verdict):
            authStatusMessage = "Falha ao entrar (\(verdict))."
        }
    }

    public func createAnonymousKeyAccount(username: String) -> AuthKeyIssued? {
        guard !isOfflineSimulation else {
            authStatusMessage = "Conecte-se para criar sua conta e receber a chave."
            return nil
        }
        switch authFlow.createKeyAccount(username: username) {
        case .ok(let issued):
            authStatusMessage = "Chave emitida. Guarde-a antes de prosseguir."
            rehydrateAuth()
            return issued
        case .err(let verdict):
            authStatusMessage = "Erro ao criar chave (\(verdict))."
            return nil
        }
    }

    public func loginWithAccountKey(_ rawKey: String) {
        switch authFlow.loginWithKey(rawKey: rawKey) {
        case .ok:
            authStatusMessage = "Acesso via chave restaurado."
            rehydrateAuth()
        case .err(let verdict):
            authStatusMessage = "Chave inválida ou indisponível (\(verdict))."
        }
    }

    public func logout() {
        let ownerId = currentAccountIdOrNil
        _ = authFlow.logout()
        photoCache.purgeAll()
        store.purgeOwnedData(snapshot: &snapshot, ownerAccountId: ownerId)
        rehydrateAuth()
        authStatusMessage = "Sessão encerrada e dados privados limpos."
    }

    public func deleteAccountAndPurge() {
        guard !isOfflineSimulation, let ownerId = currentAccountIdOrNil else {
            authStatusMessage = "Entre na sua conta com conexão à internet para solicitar a exclusão."
            return
        }
        switch authFlow.deleteAccount() {
        case .ok:
            proofSigner.resetKey()
            photoCache.purgeAll()
            store.purgeOwnedData(snapshot: &snapshot, ownerAccountId: ownerId)
            rehydrateAuth()
            authStatusMessage = "O servidor registrou seu pedido de exclusão."
        case .err:
            rehydrateAuth()
            authStatusMessage = "Não foi possível confirmar a exclusão. Verifique sua conexão e tente novamente."
        }
    }

    public func exportMyDataJson() -> String {
        let encoder = JSONEncoder()
        encoder.outputFormatting = [.prettyPrinted, .sortedKeys]
        if let data = try? encoder.encode(snapshot),
           let str = String(data: data, encoding: .utf8) {
            return str
        }
        return "{}"
    }

    // MARK: - Contributions and Photo Pipeline

    public func savePriceContributionDraft(
        station: StationRecord,
        product: FuelProduct,
        priceText: String,
        condition: PriceConditionKind,
        rawPhotoData: Data?,
        photoMime: String
    ) -> Result<MyContributionEntry, AppValidationError> {
        let milli: Int64
        do {
            milli = try PortableMoney.parse(priceText)
        } catch let err as PortableMoneyError {
            return .failure(AppValidationError("Preço inválido (\(err.code)). Use vírgula, ex: 5,999."))
        } catch {
            return .failure(AppValidationError("Preço inválido."))
        }

        let nowMs = Int64(Date().timeIntervalSince1970 * 1000)
        var photoId: String?
        var photoExpiresAt: Int64?

        if let bytes = rawPhotoData, !bytes.isEmpty {
            switch photoFlow.prepare(intentBytes: bytes, mime: photoMime) {
            case .ready(let id, _, _):
                photoId = id
                photoExpiresAt = nowMs + PortablePhoto.transientTTLMillis
            case .refused(let code):
                return .failure(AppValidationError("Foto recusada (\(code)). Use JPEG/HEIC/PNG até 256 KiB comprimidos."))
            }
        }

        let riskVerdict = lastLocationRisk?.verdict ?? PortableLocation.manual
        let entry = MyContributionEntry(
            commandId: UUID().uuidString.lowercased(),
            stationId: station.id,
            stationName: station.name,
            product: product,
            amountMilli: milli,
            condition: condition,
            statusLabel: "Rascunho neste dispositivo",
            locationVerdict: riskVerdict,
            photoCacheId: photoId,
            photoExpiresAtMillis: photoExpiresAt,
            createdAtLabel: "Agora"
        )
        snapshot.myContributions.insert(entry, at: 0)

        persist()
        return .success(entry)
    }

    // MARK: - Station Comments & Forum

    public func saveStationCommentDraft(stationId: String, text: String) -> Result<StationCommentItem, AppValidationError> {
        let normalized = PortableText.normalize(text)
        guard PortableText.isValidComment(normalized) else {
            return .failure(AppValidationError("Comentário deve ter entre 1 e 280 caracteres."))
        }
        let comment = StationCommentItem(
            stationId: stationId,
            authorAlias: signedInAlias ?? "Você",
            text: normalized,
            createdAtLabel: "Agora",
            validVotes: 0,
            invalidVotes: 0,
            revision: 1,
            isOwn: true
        )
        snapshot.stationComments.insert(comment, at: 0)
        persist()
        return .success(comment)
    }

    func socialReadClient() throws -> AbastevoHttpClient {
        guard !isOfflineSimulation else { throw HttpClientError.unavailable }
        return httpClient
    }

    public func saveForumTopicDraft(categorySlug: String, title: String, body: String, referenceUrl: String) -> Result<ForumTopicItem, AppValidationError> {
        let cleanTitle = title.trimmingCharacters(in: .whitespacesAndNewlines)
        let cleanBody = body.trimmingCharacters(in: .whitespacesAndNewlines)
        guard cleanTitle.count >= 4 && !cleanBody.isEmpty else {
            return .failure(AppValidationError("Informe título e descrição."))
        }
        let trimmedUrl = referenceUrl.trimmingCharacters(in: .whitespacesAndNewlines)
        if !trimmedUrl.isEmpty && !ForumSafeLinks.isAllowedHttpsLink(trimmedUrl) {
            return .failure(AppValidationError("Link de referência deve usar HTTPS válido sem credenciais."))
        }
        let topic = ForumTopicItem(
            id: UUID().uuidString.lowercased(),
            categorySlug: categorySlug,
            kind: .community,
            title: cleanTitle,
            summary: String(cleanBody.prefix(120)),
            body: cleanBody,
            authorId: currentAccountIdOrNil ?? "local-user",
            authorName: signedInAlias ?? "Colaborador",
            regionLabel: municipalityLabel,
            createdAtLabel: "Agora",
            externalReferenceUrl: trimmedUrl.isEmpty ? nil : trimmedUrl
        )
        snapshot.forumTopics.insert(topic, at: 0)
        persist()
        return .success(topic)
    }

    public func unblockUser(userId: String) {
        snapshot.blockedUsers.removeAll { $0.id == userId }
        persist()
    }

    // MARK: - Vehicles, Ledger & Driver Hub

    public func addVehicle(name: String, propulsion: Propulsion, primaryFuel: FuelProduct, tankLitersText: String, batteryKwhText: String) -> Result<VehicleRecord, AppValidationError> {
        guard snapshot.vehicles.count < VehicleRecord.maxFreeVehicles else {
            return .failure(AppValidationError("O plano gratuito permite cadastrar um veículo."))
        }
        let cleanName = name.trimmingCharacters(in: .whitespacesAndNewlines)
        guard !cleanName.isEmpty else { return .failure(AppValidationError("Informe o nome do veículo.")) }
        let tankMl: Int64
        if propulsion.supportsFuel {
            guard let parsed = try? PortableMoney.parseCapacityMilliLiters(tankLitersText) else {
                return .failure(AppValidationError("Capacidade do tanque inválida (1 a 200 L)."))
            }
            tankMl = parsed
        } else {
            tankMl = 0
        }
        let batteryWh: Int64
        if propulsion.supportsElectric {
            guard let parsed = try? PortableMoney.parseBatteryCapacityWh(batteryKwhText) else {
                return .failure(AppValidationError("Informe uma capacidade de bateria válida em kWh."))
            }
            batteryWh = parsed
        } else {
            batteryWh = 0
        }
        let vehicle = VehicleRecord(
            name: cleanName,
            propulsion: propulsion,
            primaryFuel: primaryFuel,
            tankCapacityMilliLiters: tankMl,
            batteryCapacityWh: batteryWh
        )
        snapshot.vehicles.append(vehicle)
        persist()
        return .success(vehicle)
    }

    public func removeVehicles(_ vehicleIds: Set<String>) {
        snapshot.vehicles.removeAll { vehicleIds.contains($0.id) }
        persist()
    }

    public func addLedgerEntry(vehicleId: String, kind: LedgerEntryKind, odometerKm: Int64, quantityMilli: Int64, totalCentavos: Int64, note: String) -> Result<Void, AppValidationError> {
        guard let vehicle = snapshot.vehicles.first(where: { $0.id == vehicleId }) else {
            return .failure(AppValidationError("Selecione um veículo cadastrado."))
        }
        if kind == .refuel && (!vehicle.propulsion.supportsFuel || vehicle.primaryFuel == .lpgP13) {
            return .failure(AppValidationError("Abastecimento indisponível para este veículo."))
        }
        if kind == .charge && !vehicle.propulsion.supportsElectric {
            return .failure(AppValidationError("Recarga elétrica indisponível para este veículo."))
        }
        let maxOdometerKm: Int64 = kind == .charge ? 1_000_000 : PremiumEnergyCalculator.maxDistanceMetres / 1000
        guard (0...maxOdometerKm).contains(odometerKm) else {
            return .failure(AppValidationError("Informe um hodômetro válido em quilômetros."))
        }
        let validQuantity: ClosedRange<Int64> = kind == .refuel ? 1...1_000_000_000 : 0...1_000_000_000
        guard validQuantity.contains(quantityMilli), (0...PremiumEnergyCalculator.maxTotalCentavos).contains(totalCentavos) else {
            return .failure(AppValidationError("Informe uma quantidade e um total válidos."))
        }
        guard note.unicodeScalars.count <= 280, note.rangeOfCharacter(from: .controlCharacters) == nil else {
            return .failure(AppValidationError("Use até 280 caracteres de texto na observação."))
        }
        let formatter = DateFormatter()
        let date = Date()
        formatter.dateFormat = "dd/MM/yyyy"
        let dateStr = formatter.string(from: date)
        formatter.dateFormat = "yyyy-MM"
        let monthStr = formatter.string(from: date)
        let entry = LedgerEntryRecord(
            vehicleId: vehicleId,
            kind: kind,
            dateLabel: dateStr,
            monthKey: monthStr,
            odometerKm: odometerKm,
            quantityMilli: quantityMilli,
            totalCentavos: totalCentavos,
            note: note
        )
        snapshot.ledgerEntries.insert(entry, at: 0)
        persist()
        return .success(())
    }

    public func startDriverShift(vehicleId: String, startOdometerKm: Int64) -> Result<Void, AppValidationError> {
        guard snapshot.vehicles.contains(where: { $0.id == vehicleId }) else {
            return .failure(AppValidationError("Selecione um veículo cadastrado."))
        }
        guard !snapshot.driverShifts.contains(where: \.isActive) else {
            return .failure(AppValidationError("Encerre o turno em andamento antes de iniciar outro."))
        }
        guard (0...(PremiumEnergyCalculator.maxDistanceMetres / 1000)).contains(startOdometerKm) else {
            return .failure(AppValidationError("Informe o hodômetro inicial em quilômetros."))
        }
        let shift = DriverShiftRecord(vehicleId: vehicleId, startedAtLabel: Date().ISO8601Format(), startOdometerKm: startOdometerKm)
        snapshot.driverShifts.insert(shift, at: 0)
        persist()
        return .success(())
    }

    public func finishActiveDriverShift(shiftId: String, endOdometerKm: Int64, grossCentavos: Int64, fuelCentavos: Int64, minutes: Int) -> Result<Void, AppValidationError> {
        guard let idx = snapshot.driverShifts.firstIndex(where: { $0.id == shiftId && $0.isActive }) else {
            return .failure(AppValidationError("O turno selecionado já foi encerrado."))
        }
        let current = snapshot.driverShifts[idx]
        guard endOdometerKm >= current.startOdometerKm,
              (0...(PremiumEnergyCalculator.maxDistanceMetres / 1000)).contains(endOdometerKm) else {
            return .failure(AppValidationError("O hodômetro final deve ser igual ou maior que o inicial."))
        }
        guard [grossCentavos, fuelCentavos].allSatisfy({ (0...PortableMoney.maxRecordCentavos).contains($0) }), minutes > 0 else {
            return .failure(AppValidationError("Informe valores e duração válidos para o turno."))
        }
        snapshot.driverShifts[idx] = DriverShiftRecord(
            id: current.id,
            vehicleId: current.vehicleId,
            startedAtLabel: current.startedAtLabel,
            endedAtLabel: Date().ISO8601Format(),
            startOdometerKm: current.startOdometerKm,
            endOdometerKm: endOdometerKm,
            grossEarningsCentavos: grossCentavos,
            fuelCostCentavos: fuelCentavos,
            otherExpenseCentavos: current.otherExpenseCentavos,
            activeMinutes: minutes,
            isActive: false
        )
        persist()
        return .success(())
    }

    // MARK: - Map and route preview

    public func requestRouteOrigin() async -> CLLocationCoordinate2D? {
        let (reading, coordinate) = await locationManager.requestOneShotFix()
        guard reading.permissionGranted, reading.hasAccuracy, reading.accuracyMeters >= 0,
              let age = reading.fixAgeSeconds, age >= 0, age <= PortableLocation.maxFixAgeSeconds,
              let coordinate, CLLocationCoordinate2DIsValid(coordinate) else {
            return nil
        }
        return coordinate
    }

    public func speakGuidanceStep(_ text: String) {
        guard isVoiceGuidanceEnabled else { return }
        voiceGuidance.speak(instruction: text, localeCode: currentLocale.rawValue)
    }

    public func stopGuidance() {
        voiceGuidance.stop()
    }

    public func toggleSavedStation(_ station: StationRecord) {
        if let idx = snapshot.savedPlaces.firstIndex(where: { $0.stationId == station.id }) {
            snapshot.savedPlaces.remove(at: idx)
        } else {
            snapshot.savedPlaces.insert(
                SavedPlaceRecord(
                    title: station.name,
                    subtitle: "\(station.brand) · \(station.neighborhood)",
                    latitude: station.latitude,
                    longitude: station.longitude,
                    stationId: station.id
                ),
                at: 0
            )
        }
        persist()
    }

    public func clearOfflineCache() {
        photoCache.purgeAll()
        store.clearCacheKeepingAccount(&snapshot)
        reloadCityData()
    }

    public func estimatedStorageKb() -> Int64 {
        return max(1, store.estimatedStorageBytes() / 1024)
    }

    public func persist() {
        store.save(snapshot)
    }

    /// The device-key signer bound to the signed-in account, or nil while the
    /// device has no bound contributor key.
    func premiumRequestSigner() -> PremiumRequestSigner? {
        return nil
    }
}

public enum PublicReadState: Equatable {
    case loading
    case loaded
    case unavailable
}

public enum AppModalSheet: String, Identifiable {
    case locationPicker
    case weekPicker
    case priceHistory
    case evExplore
    case qualityReports
    case suggestStation
    case masterFaq
    case capturePrice
    case forumFeed
    case contributors
    case myContributions
    case evTripPlanner
    case savedPlaces
    case mapDownloads
    case auth
    case accountManagement
    case premium
    case vehiclesLedger
    case driverHub
    case enterpriseHub
    case arcadeGames
    case trainingConsent
    case settings
    case help
    case privacy

    public var id: String { rawValue }
}

private struct SystemWallClock: AuthWallClock {
    func nowEpochSeconds() -> Int64 {
        return Int64(Date().timeIntervalSince1970)
    }
}

private struct RandomNonceSource: PortableNonceSource {
    func nextNonce() -> String {
        return UUID().uuidString.lowercased()
    }
}

private struct SystemPhotoClock: PhotoClock {
    func nowMillis() -> Int64 {
        return Int64(Date().timeIntervalSince1970 * 1000)
    }
}

private struct RandomPhotoIdSource: PhotoIdSource {
    func nextId() -> String {
        return UUID().uuidString.lowercased()
    }
}

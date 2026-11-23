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
    @Published public private(set) var isAuthWorking = false

    // Location & Map
    @Published public var lastDeviceCoordinate: CLLocationCoordinate2D?
    @Published public var lastLocationRisk: PortableLocation.FixRisk?
    @Published public var isVoiceGuidanceEnabled: Bool = true

    // Community & Outbox
    @Published public var communityFeed: [CommunityPriceObservation] = []
    @Published public var stations: [StationRecord] = []
    @Published public var evMapPoints: [ChargingMapPoint] = []
    @Published private(set) var stationReadState: PublicReadState = .loading
    @Published private(set) var communityReadState: PublicReadState = .loading
    @Published private(set) var nextStationCursor: String?
    @Published private(set) var isLoadingMoreStations = false
    @Published private(set) var stationPageUnavailable = false

    // Official ANP weeks, EV catalog and offline public reads
    public let anpSurvey: AnpSurveyModel
    public let evCatalog: EvCatalogModel
    let publicReadCache: PublicReadCache
    @Published public private(set) var directoryCachedAt: Date?
    @Published public private(set) var communityCachedAt: Date?
    @Published public private(set) var communityExpiredCount = 0
    @Published private var surveyWeekTitle = String(localized: "Pesquisa ANP")
    private var communityExpiries: [String: Int64] = [:]

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
    /// Device contributor identity and signed community writes (outbox, receipts, owner history).
    public let identity: IdentityCoordinator
    /// Abastevo Empresas: company access, the protected per-organization queue and signed company commands.
    public let enterprise: EnterpriseCoordinator
    private var identityRelay: AnyCancellable?
    private var pendingAppleNonce: String?

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
        self.identity = IdentityCoordinator(
            authFlow: authFlow,
            signer: proofSigner,
            photoCache: cache,
            environment: loaded.apiEnvironment
        )
        self.anpSurvey = AnpSurveyModel()
        self.evCatalog = EvCatalogModel()
        self.publicReadCache = PublicReadCache()
        self.enterprise = EnterpriseCoordinator(sessions: AuthFlowSessionSource(flow: authFlow), identity: identity)
        _ = self.photoFlow.sweepExpired()
        rehydrateAuth()
        reloadCityData()
        identityRelay = identity.objectWillChange.sink { [weak self] _ in
            self?.objectWillChange.send()
        }
        identity.isOfflineProbe = { [weak self] in
            self?.isOfflineSimulation ?? false
        }
        enterprise.isOfflineProbe = { [weak self] in
            self?.isOfflineSimulation ?? false
        }
        identity.start()
        anpSurvey.attach(self)
        evCatalog.attach(self)
        anpSurvey.$weekLabel.assign(to: &$surveyWeekTitle)
    }

    public var currentLocale: SupportedAppLocale {
        return SupportedAppLocale(rawValue: snapshot.localeCode) ?? .ptBR
    }

    public var selectedMunicipality: Municipality? { municipalityCatalog.municipality(code: snapshot.selectedMunicipalityCode) }

    public var municipalityLabel: String { selectedMunicipality?.displayTitle ?? String(localized: "Escolher cidade") }

    public var surveyWeekLabel: String { surveyWeekTitle }

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
        let now = Int64(Date().timeIntervalSince1970)
        let values = communityFeed.filter {
            $0.product == fuel && $0.municipalityCode == city.ibgeCode && (communityExpiries[$0.id].map { $0 > now } ?? true)
        }
        switch feedSortOrder {
        case .recent:
            return values.sorted { $0.observedEpochSeconds > $1.observedEpochSeconds }
        case .cheapest:
            return values.sorted { $0.amountMilli < $1.amountMilli }
        }
    }

    public func communityObservation(stationId: String, product: FuelProduct) -> CommunityPriceObservation? {
        let now = Int64(Date().timeIntervalSince1970)
        return communityFeed.first {
            $0.stationId == stationId && $0.product == product && (communityExpiries[$0.id].map { $0 > now } ?? true)
        }
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
        identity.updateEnvironment(env)
        enterprise.purgeOwnedData()
        persist()
        evCatalog.originChanged()
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
        let cache = publicReadCache
        let origin = snapshot.apiEnvironment.origin

        stations = []
        communityFeed = []
        communityExpiries = [:]
        nextStationCursor = nil
        isLoadingMoreStations = false
        stationPageUnavailable = false
        directoryCachedAt = nil
        communityCachedAt = nil
        communityExpiredCount = 0
        anpSurvey.cityContextChanged()
        guard let municipality else {
            stationReadState = .unavailable
            communityReadState = .unavailable
            return
        }
        guard !isOfflineSimulation else {
            applyCachedDirectory(origin: origin, municipality: municipality)
            applyCachedCommunity(origin: origin, municipality: municipality, fuel: fuel)
            return
        }
        stationReadState = .loading
        communityReadState = .loading

        directoryRead = Task { [weak self] in
            let loaded = try? await client.fetchStations(in: municipality)
            guard !Task.isCancelled, let self, self.cityReadGeneration == generation else { return }
            guard let page = loaded else {
                self.applyCachedDirectory(origin: origin, municipality: municipality)
                return
            }
            self.stations = page.stations
            self.nextStationCursor = page.nextCursor
            self.stationReadState = .loaded
            cache.saveDirectory(origin: origin, municipalityCode: municipality.ibgeCode, stations: page.stations)
        }
        communityRead = Task { [weak self] in
            let fetched = try? await client.fetchCommunityFeedItems(
                state: municipality.state, municipalityCode: municipality.ibgeCode, fuel: fuel, sort: order
            )
            guard !Task.isCancelled, let self, self.cityReadGeneration == generation else { return }
            guard let items = fetched else {
                self.applyCachedCommunity(origin: origin, municipality: municipality, fuel: fuel)
                return
            }
            let now = Int64(Date().timeIntervalSince1970)
            let live = items.filter { $0.isLive(nowEpochSeconds: now) }
            self.communityFeed = live.map(\.observation)
            self.communityExpiries = Dictionary(live.map { ($0.observation.id, $0.expiresAtEpochSeconds) }, uniquingKeysWith: { first, _ in first })
            self.communityReadState = .loaded
            cache.saveCommunityFeed(origin: origin, municipalityCode: municipality.ibgeCode, fuel: fuel, items: items)
        }
    }

    /// Last directory page saved for this origin and city, labelled with its save time.
    private func applyCachedDirectory(origin: String, municipality: Municipality) {
        guard let cached = publicReadCache.directory(origin: origin, municipalityCode: municipality.ibgeCode) else {
            stationReadState = .unavailable
            return
        }
        stations = cached.stations
        nextStationCursor = nil
        directoryCachedAt = Date(timeIntervalSince1970: TimeInterval(cached.savedAtEpochSeconds))
        stationReadState = .loaded
    }

    /// Saved community prices still before their server expiry; expired ones are only counted.
    private func applyCachedCommunity(origin: String, municipality: Municipality, fuel: FuelProduct) {
        guard let cached = publicReadCache.communityFeed(origin: origin, municipalityCode: municipality.ibgeCode, fuel: fuel) else {
            communityReadState = .unavailable
            return
        }
        let now = Int64(Date().timeIntervalSince1970)
        let live = cached.items.filter { $0.isLive(nowEpochSeconds: now) }
        communityFeed = live.map(\.observation)
        communityExpiries = Dictionary(live.map { ($0.observation.id, $0.expiresAtEpochSeconds) }, uniquingKeysWith: { first, _ in first })
        communityExpiredCount = cached.expiredCount(nowEpochSeconds: now)
        communityCachedAt = Date(timeIntervalSince1970: TimeInterval(cached.savedAtEpochSeconds))
        communityReadState = .loaded
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
        let flow = authFlow
        runAuth({ flow.requestEmailCode(email: email) }) { viewModel, result in
            switch result {
            case .ok:
                viewModel.authStatusMessage = "Se o e-mail for válido, enviamos um código de 6 dígitos."
            case .err(let code):
                viewModel.authStatusMessage = "Não foi possível enviar o código (\(code))."
            }
        }
    }

    public func verifyEmailOtp(email: String, code: String) {
        let flow = authFlow
        runAuth({ flow.consumeEmailCode(email: email, code: code) }) { viewModel, result in
            switch result {
            case .ok(let outcome):
                viewModel.authStatusMessage = outcome.created ? "Conta gratuita criada e conectada." : "Sessão conectada com sucesso."
                viewModel.completeSignIn()
            case .err(let verdict):
                viewModel.authStatusMessage = "Falha ao entrar (\(verdict))."
            }
        }
    }

    public func createAnonymousKeyAccount(username: String) async -> AuthKeyIssued? {
        guard !isOfflineSimulation else {
            authStatusMessage = "Conecte-se para criar sua conta e receber a chave."
            return nil
        }
        let flow = authFlow
        isAuthWorking = true
        let result = await AuthBlockingWork.run { flow.createKeyAccount(username: username) }
        isAuthWorking = false
        switch result {
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
        let flow = authFlow
        runAuth({ flow.loginWithKey(rawKey: rawKey) }) { viewModel, result in
            switch result {
            case .ok:
                viewModel.authStatusMessage = "Acesso via chave restaurado."
                viewModel.completeSignIn()
            case .err(let verdict):
                viewModel.authStatusMessage = "Chave inválida ou indisponível (\(verdict))."
            }
        }
    }

    /// Value for `ASAuthorizationAppleIDRequest.nonce`; the raw nonce stays here until the reply arrives.
    public func prepareAppleSignInNonce() -> String? {
        guard let raw = AppleSignInNonce.makeRawNonce() else {
            pendingAppleNonce = nil
            return nil
        }
        pendingAppleNonce = raw
        return AppleSignInNonce.requestNonce(forRaw: raw)
    }

    public func completeAppleSignIn(identityToken: Data?, authorizationCode: Data?) {
        guard let rawNonce = pendingAppleNonce else {
            authStatusMessage = "Não foi possível iniciar o login com a Apple. Tente novamente."
            return
        }
        pendingAppleNonce = nil
        guard let tokenData = identityToken,
              let idToken = String(data: tokenData, encoding: .utf8),
              !idToken.isEmpty else {
            authStatusMessage = "A Apple não retornou um token de identidade."
            return
        }
        guard !isOfflineSimulation else {
            authStatusMessage = "Conecte-se para entrar com a Apple."
            return
        }
        let code = authorizationCode.flatMap { String(data: $0, encoding: .utf8) }
        let hasKeyAccount = authFlow.currentKey() != nil
        let flow = authFlow
        runAuth({
            flow.loginWithProvider(
                provider: PortableAuth.providerApple,
                idToken: idToken,
                rawNonce: rawNonce,
                authorizationCode: code
            )
        }) { viewModel, result in
            switch result {
            case .ok(let outcome):
                viewModel.authStatusMessage = outcome.created ? "Conta gratuita criada com a Apple." : "Conectado com a Apple."
                viewModel.completeSignIn()
            case .err(let verdict):
                if verdict == AccountProviderLogin.unavailableVerdict {
                    viewModel.authStatusMessage = "Login com a Apple ainda não está disponível neste servidor."
                } else if verdict == AuthFlow.clientInvalid && hasKeyAccount {
                    viewModel.authStatusMessage = "Saia da conta com chave antes de entrar com a Apple."
                } else {
                    viewModel.authStatusMessage = "Falha no login com a Apple (\(verdict))."
                }
            }
        }
    }

    public func failAppleSignIn(cancelled: Bool) {
        pendingAppleNonce = nil
        authStatusMessage = cancelled ? "Login com a Apple cancelado." : "Login com a Apple indisponível neste aparelho."
    }

    /// After any login: refresh the visible state, then register and bind the device key for owner-signed writes.
    private func completeSignIn() {
        rehydrateAuth()
        identity.afterSignIn()
    }

    /// Runs one blocking `AuthFlow` call off the main actor and applies its result on the main actor.
    private func runAuth<T>(
        _ work: @escaping @Sendable () -> T,
        apply: @escaping (AppStateViewModel, T) -> Void
    ) {
        isAuthWorking = true
        Task { [weak self] in
            let result = await AuthBlockingWork.run(work)
            guard let self else { return }
            self.isAuthWorking = false
            apply(self, result)
        }
    }

    public func logout() {
        let ownerId = currentAccountIdOrNil
        photoCache.purgeAll()
        identity.purgeOwnedData()
        enterprise.purgeOwnedData()
        store.purgeOwnedData(snapshot: &snapshot, ownerAccountId: ownerId)
        let flow = authFlow
        runAuth({ flow.logout() }) { viewModel, _ in
            viewModel.rehydrateAuth()
            viewModel.authStatusMessage = "Sessão encerrada e dados privados limpos."
        }
    }

    public func deleteAccountAndPurge() {
        guard !isOfflineSimulation, let ownerId = currentAccountIdOrNil else {
            authStatusMessage = "Entre na sua conta com conexão à internet para solicitar a exclusão."
            return
        }
        let flow = authFlow
        runAuth({ flow.deleteAccount() }) { viewModel, result in
            switch result {
            case .ok:
                viewModel.finishAccountDeletion(ownerId: ownerId)
            case .err:
                viewModel.rehydrateAuth()
                viewModel.authStatusMessage = "Não foi possível confirmar a exclusão. Verifique sua conexão e tente novamente."
            }
        }
    }

    private func finishAccountDeletion(ownerId: String) {
        proofSigner.resetKey()
        identity.forgetDeviceIdentity()
        enterprise.purgeOwnedData()
        photoCache.purgeAll()
        store.purgeOwnedData(snapshot: &snapshot, ownerAccountId: ownerId)
        rehydrateAuth()
        authStatusMessage = "O servidor registrou seu pedido de exclusão."
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

    /// Queues a signed price observation (`POST /v1/observations`). An evidence photo needs a capture grant
    /// for the same station and a capture inside its camera window; other photos never leave the device.
    public func submitPriceContribution(
        station: StationRecord,
        product: FuelProduct,
        priceText: String,
        condition: PriceConditionKind,
        evidencePhoto: Data?,
        photoMime: String,
        photoCapturedAtMillis: Int64?,
        photoGrant: PhotoCaptureGrant?
    ) -> Result<CommunityOutboxItem, AppValidationError> {
        let milli: Int64
        do {
            milli = try PortableMoney.parse(priceText)
        } catch let err as PortableMoneyError {
            return .failure(AppValidationError("Preço inválido (\(err.code)). Use vírgula, ex: 5,999."))
        } catch {
            return .failure(AppValidationError("Preço inválido."))
        }
        guard milli <= CommunityWire.maxAmountMilli else {
            return .failure(AppValidationError("Preço acima do limite aceito pelo servidor."))
        }
        guard CommunityWire.conditionWire(condition) != nil else {
            return .failure(AppValidationError("O servidor ainda não aceita a condição Pix. Escolha outra condição."))
        }

        let nowMs = Int64(Date().timeIntervalSince1970 * 1000)
        var photo: CommunityPhotoEvidence?
        if let bytes = evidencePhoto, !bytes.isEmpty {
            guard let grant = photoGrant,
                  let capturedAt = photoCapturedAtMillis,
                  grant.admits(stationId: station.id, capturedAtMillis: capturedAt) else {
                return .failure(AppValidationError("A autorização da foto não vale para este posto ou expirou. Tire a foto novamente."))
            }
            guard condition == .standard else {
                return .failure(AppValidationError("Foto como evidência exige o preço padrão na bomba."))
            }
            switch photoFlow.prepareAt(intentBytes: bytes, mime: photoMime, capturedAtMillis: min(capturedAt, nowMs)) {
            case .ready(let id, _, _):
                photo = CommunityPhotoEvidence(
                    photoId: id,
                    captureId: grant.captureId,
                    capturedAtMillis: capturedAt,
                    captureExpiresAtMillis: grant.expiresAtMillis
                )
            case .refused(let code):
                return .failure(AppValidationError("Foto recusada (\(code)). Use JPEG/HEIC/PNG até 256 KiB comprimidos."))
            }
        }

        let draft = CommunityPriceDraft(
            stationId: station.id,
            stationName: station.name,
            product: product,
            amountMilli: milli,
            condition: condition,
            capturedAtMillis: photo?.capturedAtMillis ?? nowMs,
            photo: photo
        )
        guard let item = identity.enqueue(kind: .priceObservation, price: draft, text: nil) else {
            if let photo {
                photoFlow.discard(id: photo.photoId)
            }
            return .failure(AppValidationError("A chave deste aparelho está indisponível."))
        }
        return .success(item)
    }

    // MARK: - Station Comments & Forum

    /// Queues a station comment (`POST /v1/feedback/comments`, account session). Up to 280 Unicode scalars.
    public func publishStationComment(stationId: String, product: FuelProduct, text: String) -> Result<CommunityOutboxItem, AppValidationError> {
        let normalized = PortableText.normalize(text)
        guard PortableText.isValidComment(normalized) else {
            return .failure(AppValidationError("Comentário deve ter entre 1 e 280 caracteres."))
        }
        let draft = CommunityTextDraft(text: normalized, stationId: stationId, product: product)
        guard let item = identity.enqueue(kind: .stationComment, price: nil, text: draft) else {
            return .failure(AppValidationError("A chave deste aparelho está indisponível."))
        }
        return .success(item)
    }

    func socialReadClient() throws -> AbastevoHttpClient {
        guard !isOfflineSimulation else { throw HttpClientError.unavailable }
        return httpClient
    }

    /// Queues a forum topic (`POST /v1/forum/topics`, account session plus device proof). It enters moderation.
    public func publishForumTopic(categoryCode: String, title: String, body: String) -> Result<CommunityOutboxItem, AppValidationError> {
        let cleanTitle = title.trimmingCharacters(in: .whitespacesAndNewlines)
        let cleanBody = body.trimmingCharacters(in: .whitespacesAndNewlines)
        guard !categoryCode.isEmpty else {
            return .failure(AppValidationError("Escolha uma categoria."))
        }
        guard CommunityWire.isValidTopic(title: cleanTitle, body: cleanBody) else {
            return .failure(AppValidationError("Informe título (até 120 caracteres) e descrição (até 4000)."))
        }
        let city = selectedMunicipality
        let draft = CommunityTextDraft(
            text: cleanBody,
            categoryCode: categoryCode,
            title: cleanTitle,
            regionUf: city?.state.rawValue,
            regionMunicipalityIbge: city.flatMap { Int($0.ibgeCode) }
        )
        guard let item = identity.enqueue(kind: .forumTopic, price: nil, text: draft) else {
            return .failure(AppValidationError("A chave deste aparelho está indisponível."))
        }
        return .success(item)
    }

    /// Queues a forum comment (`POST /v1/forum/posts/{id}/comments`). Up to 280 Unicode scalars.
    public func publishForumComment(postId: String, text: String) -> Result<CommunityOutboxItem, AppValidationError> {
        let normalized = PortableText.normalize(text)
        guard CommunityWire.isUuid(postId), PortableText.isValidComment(normalized) else {
            return .failure(AppValidationError("Comentário deve ter entre 1 e 280 caracteres."))
        }
        let draft = CommunityTextDraft(text: normalized, postId: postId)
        guard let item = identity.enqueue(kind: .forumComment, price: nil, text: draft) else {
            return .failure(AppValidationError("A chave deste aparelho está indisponível."))
        }
        return .success(item)
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
        publicReadCache.clear()
        store.clearCacheKeepingAccount(&snapshot)
        reloadCityData()
    }

    public func estimatedStorageKb() -> Int64 {
        let bytes = store.estimatedStorageBytes() + anpSurvey.storedBytes + evCatalog.storedBytes + publicReadCache.storedBytes()
        return max(1, bytes / 1024)
    }

    public func persist() {
        store.save(snapshot)
    }

    /// The signed transport for the signed-in account; it registers and binds
    /// the device key on first use.
    func premiumRequestSigner() -> PremiumRequestSigner? {
        guard currentAccountIdOrNil != nil else { return nil }
        return identity.signedTransport
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

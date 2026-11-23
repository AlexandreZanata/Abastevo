import Foundation
import Combine
import SwiftUI
import CoreLocation
import AnpFuelCore

/// Central MainActor view-model coordinating all Abastevo iOS flows.
@MainActor
public final class AppStateViewModel: ObservableObject {
    @Published public var snapshot: PersistedAppSnapshot
    @Published public var selectedTab: AppTab = .explore
    @Published public var activeSheet: AppModalSheet?
    @Published public var selectedStation: StationRecord?
    @Published public var selectedForumTopic: ForumTopicItem?
    @Published public var sortOrder: StationSortOrder = .lowestPrice
    @Published public var feedSortOrder: FeedSortOrder = .recent
    @Published public var isOfflineSimulation: Bool = false
    @Published public var isSyncing: Bool = false
    @Published public var bannerMessage: String?

    // Auth & Identity
    @Published public var authFlowState: AuthFlowState = .loggedOut
    @Published public var signedInAlias: String?
    @Published public var authStatusMessage: String?

    // Location & Map
    @Published public var lastDeviceCoordinate: CLLocationCoordinate2D?
    @Published public var lastLocationRisk: PortableLocation.FixRisk?
    @Published public var lastLocationRecovery: PortableLocationRecovery.Recovery?
    @Published public var activeRouteStation: StationRecord?
    @Published public var isVoiceGuidanceEnabled: Bool = true

    // Community & Outbox
    @Published public var communityFeed: [CommunityPriceObservation] = []
    @Published public var stations: [StationRecord] = []
    @Published public var evSites: [EvChargingSite] = SeedCatalog.evChargingSites

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

    public init(store: IosLocalStore = IosLocalStore()) {
        self.store = store
        let loaded = store.load()
        self.snapshot = loaded
        self.keychain = IosKeychainSecretStore()
        self.proofSigner = IosDeviceProofSigner()
        self.httpClient = AbastevoHttpClient(environment: loaded.apiEnvironment, proofSigner: proofSigner)
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

    public var selectedMunicipality: Municipality {
        return SeedCatalog.municipalities.first(where: { $0.ibgeCode == snapshot.selectedMunicipalityCode })
            ?? SeedCatalog.municipalities[0]
    }

    public var selectedWeek: SurveyWeek {
        return SeedCatalog.surveyWeeks.first(where: { $0.id == snapshot.selectedWeekId })
            ?? SeedCatalog.surveyWeeks[0]
    }

    public var officialAverage: OfficialAnpAverage? {
        return SeedCatalog.officialAverage(
            for: selectedMunicipality,
            weekId: selectedWeek.id,
            product: snapshot.selectedFuel,
            stations: stations
        )
    }

    public var sortedStations: [StationRecord] {
        let fuel = snapshot.selectedFuel
        let refLat = lastDeviceCoordinate?.latitude ?? selectedMunicipality.latitude
        let refLon = lastDeviceCoordinate?.longitude ?? selectedMunicipality.longitude
        return stations.sorted { lhs, rhs in
            switch sortOrder {
            case .lowestPrice:
                let lp = lhs.effectiveDisplayPrice(for: fuel) ?? Int64.max
                let rp = rhs.effectiveDisplayPrice(for: fuel) ?? Int64.max
                if lp != rp { return lp < rp }
                return lhs.name < rhs.name
            case .mostRecent:
                let lt = lhs.latestCommunityPrice(for: fuel)?.observedEpochSeconds ?? 0
                let rt = rhs.latestCommunityPrice(for: fuel)?.observedEpochSeconds ?? 0
                if lt != rt { return lt > rt }
                return (lhs.effectiveDisplayPrice(for: fuel) ?? Int64.max) < (rhs.effectiveDisplayPrice(for: fuel) ?? Int64.max)
            case .distance:
                return lhs.distanceKm(fromLat: refLat, fromLon: refLon) < rhs.distanceKm(fromLat: refLat, fromLon: refLon)
            case .name:
                return lhs.name.localizedCaseInsensitiveCompare(rhs.name) == .orderedAscending
            }
        }
    }

    public var visibleCommunityFeed: [CommunityPriceObservation] {
        let fuel = snapshot.selectedFuel
        let combined = stations.flatMap { $0.communityObservations } + communityFeed
        var deduped: [String: CommunityPriceObservation] = [:]
        for item in combined where item.product == fuel {
            deduped[item.id] = item
        }
        let values = Array(deduped.values)
        switch feedSortOrder {
        case .recent:
            return values.sorted { $0.observedEpochSeconds > $1.observedEpochSeconds }
        case .cheapest:
            return values.sorted { $0.amountMilli < $1.amountMilli }
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

    public func selectWeek(_ week: SurveyWeek) {
        snapshot.selectedWeekId = week.id
        if !snapshot.downloadedWeekIds.contains(week.id) {
            snapshot.downloadedWeekIds.append(week.id)
        }
        persist()
        reloadCityData()
    }

    public func selectFuel(_ fuel: FuelProduct) {
        snapshot.selectedFuel = fuel
        persist()
    }

    public func setLocale(_ locale: SupportedAppLocale) {
        snapshot.localeCode = locale.rawValue
        persist()
    }

    public func setTheme(dark: Bool?) {
        snapshot.darkThemeEnabled = dark
        persist()
    }

    public func setApiEnvironment(_ env: ApiEnvironment) {
        snapshot.apiEnvironment = env
        httpClient.environment = env
        persist()
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
        let baseStations = SeedCatalog.stationsFor(municipality: selectedMunicipality)
        self.stations = baseStations
        self.communityFeed = baseStations.flatMap { $0.communityObservations }
        guard !isOfflineSimulation else { return }
        Task {
            if let remoteFeed = await httpClient.fetchCommunityFeed(
                state: selectedMunicipality.state,
                municipalityCode: selectedMunicipality.ibgeCode,
                fuel: snapshot.selectedFuel,
                sort: feedSortOrder
            ), !remoteFeed.isEmpty {
                self.communityFeed = remoteFeed
            }
        }
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
        self.lastLocationRecovery = PortableLocationRecovery.recover(risk)
        if let coord = coord {
            self.lastDeviceCoordinate = coord
            if let nearest = SeedCatalog.municipalities.min(by: {
                let d1 = hypot($0.latitude - coord.latitude, $0.longitude - coord.longitude)
                let d2 = hypot($1.latitude - coord.latitude, $1.longitude - coord.longitude)
                return d1 < d2
            }) {
                selectMunicipality(nearest)
            }
        }
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
        case .pendingKeyAccount(let backup):
            self.signedInAlias = backup.username
        case .loggedOut:
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
        if isOfflineSimulation {
            let localKey = "abv-\(UUID().uuidString.prefix(12).lowercased())"
            let backup = AuthKeyBackup(username: username.lowercased(), accountKey: localKey)
            try? keychain.saveKey(backup)
            rehydrateAuth()
            authStatusMessage = "Chave criada offline; salve em local seguro."
            return AuthKeyIssued(username: backup.username, accountKey: localKey)
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

    public func completeSignInWithApple(userIdentifier: String, identityToken: String) {
        guard !userIdentifier.isEmpty else {
            authStatusMessage = "Sign in with Apple cancelado."
            return
        }
        let now = Int64(Date().timeIntervalSince1970)
        let session = PortableAuth.Session(
            familyId: "apple-\(UUID().uuidString.lowercased())",
            accountId: "apple-\(userIdentifier.prefix(16))",
            accessToken: identityToken.isEmpty ? "apple-local-proof" : identityToken,
            refreshToken: "apple-refresh-\(UUID().uuidString.lowercased())",
            accessExpiresAt: now + PortableAuth.accessTTLSeconds,
            absoluteExpiresAt: now + PortableAuth.refreshAbsoluteSeconds
        )
        try? keychain.save(session)
        signedInAlias = "Apple ID"
        authFlowState = .active(session)
        authStatusMessage = "Conectado com Sign in with Apple."
    }

    public func logout() {
        let ownerId = currentAccountIdOrNil
        _ = authFlow.logout()
        photoCache.purgeAll()
        store.purgeOwnedData(snapshot: &snapshot, ownerPrefix: ownerId)
        rehydrateAuth()
        authStatusMessage = "Sessão encerrada e dados privados limpos."
    }

    public func deleteAccountAndPurge() {
        let ownerId = currentAccountIdOrNil
        _ = authFlow.deleteAccount()
        try? keychain.clear()
        try? keychain.clearKey()
        proofSigner.resetKey()
        photoCache.purgeAll()
        store.purgeOwnedData(snapshot: &snapshot, ownerPrefix: ownerId)
        rehydrateAuth()
        authStatusMessage = "Conta encerrada e dados locais apagados."
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

    // MARK: - Contributions, Photo Pipeline & Votes

    public func submitPriceContribution(
        station: StationRecord,
        product: FuelProduct,
        priceText: String,
        condition: PriceConditionKind,
        rawPhotoData: Data?,
        photoMime: String
    ) -> Result<MyContributionEntry, String> {
        let milli: Int64
        do {
            milli = try PortableMoney.parse(priceText)
        } catch let err as PortableMoneyError {
            return .failure("Preço inválido (\(err.code)). Use vírgula, ex: 5,999.")
        } catch {
            return .failure("Preço inválido.")
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
                return .failure("Foto recusada (\(code)). Use JPEG/HEIC/PNG até 256 KiB comprimidos.")
            }
        }

        let commandId = UUID().uuidString.lowercased()
        let payload = "{\"station_id\":\"\(station.id)\",\"fuel\":\"\(product.wireCode)\",\"amount_milli_brl\":\(milli),\"condition\":\"\(condition.wireKind)\",\"photo_id\":\"\(photoId ?? "")\"}"
        if let updated = try? PortableOutbox.enqueue(
            commands: snapshot.outboxCommands,
            commandId: commandId,
            kind: "price-observation",
            payload: payload
        ) {
            snapshot.outboxCommands = updated
        }

        let riskVerdict = lastLocationRisk?.verdict ?? PortableLocation.manual
        let entry = MyContributionEntry(
            commandId: commandId,
            stationId: station.id,
            stationName: station.name,
            product: product,
            amountMilli: milli,
            condition: condition,
            statusLabel: isOfflineSimulation ? "Na fila offline" : "Enviado para validação",
            locationVerdict: riskVerdict,
            photoExpiresAtMillis: photoExpiresAt,
            createdAtLabel: "Agora"
        )
        snapshot.myContributions.insert(entry, at: 0)

        let obs = CommunityPriceObservation(
            id: "local-\(commandId)",
            stationId: station.id,
            stationName: station.name,
            municipalityCode: station.municipalityCode,
            product: product,
            amountMilli: milli,
            condition: condition,
            confirmations: 1,
            disputes: 0,
            helpfulCount: 0,
            notHelpfulCount: 0,
            observedAtLabel: "Agora (você)",
            observedEpochSeconds: Int64(Date().timeIntervalSince1970),
            hasPhotoEvidence: photoId != nil,
            isOwnSubmission: true
        )
        communityFeed.insert(obs, at: 0)
        if let stIdx = stations.firstIndex(where: { $0.id == station.id }) {
            var updatedStation = stations[stIdx]
            var obsList = updatedStation.communityObservations
            obsList.insert(obs, at: 0)
            updatedStation = StationRecord(
                id: updatedStation.id,
                cnpj: updatedStation.cnpj,
                name: updatedStation.name,
                brand: updatedStation.brand,
                address: updatedStation.address,
                neighborhood: updatedStation.neighborhood,
                municipalityCode: updatedStation.municipalityCode,
                latitude: updatedStation.latitude,
                longitude: updatedStation.longitude,
                pinTier: updatedStation.pinTier,
                officialPricesMilli: updatedStation.officialPricesMilli,
                officialDateLabel: updatedStation.officialDateLabel,
                communityObservations: obsList,
                openingHours: updatedStation.openingHours,
                paymentMethods: updatedStation.paymentMethods,
                qualitySummary: updatedStation.qualitySummary,
                isClaimedOwner: updatedStation.isClaimedOwner,
                ownerPromotionText: updatedStation.ownerPromotionText
            )
            stations[stIdx] = updatedStation
            if selectedStation?.id == updatedStation.id {
                selectedStation = updatedStation
            }
        }
        persist()
        return .success(entry)
    }

    public func confirmObservation(_ obs: CommunityPriceObservation) {
        updateObservation(obs.id) { current in
            CommunityPriceObservation(
                id: current.id,
                stationId: current.stationId,
                stationName: current.stationName,
                municipalityCode: current.municipalityCode,
                product: current.product,
                amountMilli: current.amountMilli,
                condition: current.condition,
                confirmations: current.confirmations + 1,
                disputes: current.disputes,
                helpfulCount: current.helpfulCount,
                notHelpfulCount: current.notHelpfulCount,
                observedAtLabel: current.observedAtLabel,
                observedEpochSeconds: current.observedEpochSeconds,
                hasPhotoEvidence: current.hasPhotoEvidence,
                isOwnSubmission: current.isOwnSubmission
            )
        }
    }

    public func markObservationHelpful(_ obs: CommunityPriceObservation, isHelpful: Bool) {
        updateObservation(obs.id) { current in
            CommunityPriceObservation(
                id: current.id,
                stationId: current.stationId,
                stationName: current.stationName,
                municipalityCode: current.municipalityCode,
                product: current.product,
                amountMilli: current.amountMilli,
                condition: current.condition,
                confirmations: current.confirmations,
                disputes: current.disputes,
                helpfulCount: current.helpfulCount + (isHelpful ? 1 : 0),
                notHelpfulCount: current.notHelpfulCount + (isHelpful ? 0 : 1),
                observedAtLabel: current.observedAtLabel,
                observedEpochSeconds: current.observedEpochSeconds,
                hasPhotoEvidence: current.hasPhotoEvidence,
                isOwnSubmission: current.isOwnSubmission
            )
        }
    }

    private func updateObservation(_ obsId: String, transform: (CommunityPriceObservation) -> CommunityPriceObservation) {
        communityFeed = communityFeed.map { $0.id == obsId ? transform($0) : $0 }
        stations = stations.map { st in
            let updatedObs = st.communityObservations.map { $0.id == obsId ? transform($0) : $0 }
            return StationRecord(
                id: st.id,
                cnpj: st.cnpj,
                name: st.name,
                brand: st.brand,
                address: st.address,
                neighborhood: st.neighborhood,
                municipalityCode: st.municipalityCode,
                latitude: st.latitude,
                longitude: st.longitude,
                pinTier: st.pinTier,
                officialPricesMilli: st.officialPricesMilli,
                officialDateLabel: st.officialDateLabel,
                communityObservations: updatedObs,
                openingHours: st.openingHours,
                paymentMethods: st.paymentMethods,
                qualitySummary: st.qualitySummary,
                isClaimedOwner: st.isClaimedOwner,
                ownerPromotionText: st.ownerPromotionText
            )
        }
        if let sel = selectedStation, let updated = stations.first(where: { $0.id == sel.id }) {
            selectedStation = updated
        }
    }

    // MARK: - Station Comments & Forum

    public func addStationComment(stationId: String, text: String) -> Result<StationCommentItem, String> {
        let normalized = PortableText.normalize(text)
        guard PortableText.isValidComment(normalized) else {
            return .failure("Comentário deve ter entre 1 e 280 caracteres.")
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

    public func voteStationComment(commentId: String, isValid: Bool) {
        snapshot.stationComments = snapshot.stationComments.map { c in
            guard c.id == commentId else { return c }
            return StationCommentItem(
                id: c.id,
                stationId: c.stationId,
                authorAlias: c.authorAlias,
                text: c.text,
                createdAtLabel: c.createdAtLabel,
                validVotes: c.validVotes + (isValid ? 1 : 0),
                invalidVotes: c.invalidVotes + (isValid ? 0 : 1),
                revision: c.revision,
                isOwn: c.isOwn,
                isReported: c.isReported
            )
        }
        persist()
    }

    public func createForumTopic(categorySlug: String, title: String, body: String, referenceUrl: String) -> Result<ForumTopicItem, String> {
        let cleanTitle = title.trimmingCharacters(in: .whitespacesAndNewlines)
        let cleanBody = body.trimmingCharacters(in: .whitespacesAndNewlines)
        guard cleanTitle.count >= 4 && !cleanBody.isEmpty else {
            return .failure("Informe título e descrição.")
        }
        let trimmedUrl = referenceUrl.trimmingCharacters(in: .whitespacesAndNewlines)
        if !trimmedUrl.isEmpty && !ForumSafeLinks.isAllowedHttpsLink(trimmedUrl) {
            return .failure("Link de referência deve usar HTTPS válido sem credenciais.")
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
            regionLabel: selectedMunicipality.displayTitle,
            createdAtLabel: "Agora",
            externalReferenceUrl: trimmedUrl.isEmpty ? nil : trimmedUrl
        )
        snapshot.forumTopics.insert(topic, at: 0)
        persist()
        return .success(topic)
    }

    public func addForumComment(topicId: String, text: String) {
        let normalized = PortableText.normalize(text)
        guard PortableText.isValidComment(normalized) else { return }
        let comment = ForumCommentItem(
            topicId: topicId,
            authorId: currentAccountIdOrNil ?? "local-user",
            authorName: signedInAlias ?? "Você",
            body: normalized,
            createdAtLabel: "Agora",
            isOwn: true
        )
        snapshot.forumTopics = snapshot.forumTopics.map { topic in
            guard topic.id == topicId else { return topic }
            var comments = topic.comments
            comments.append(comment)
            return ForumTopicItem(
                id: topic.id,
                categorySlug: topic.categorySlug,
                kind: topic.kind,
                title: topic.title,
                summary: topic.summary,
                body: topic.body,
                authorId: topic.authorId,
                authorName: topic.authorName,
                regionLabel: topic.regionLabel,
                createdAtLabel: topic.createdAtLabel,
                externalReferenceUrl: topic.externalReferenceUrl,
                reactionCount: topic.reactionCount,
                myReacted: topic.myReacted,
                comments: comments
            )
        }
        if let current = selectedForumTopic, current.id == topicId {
            selectedForumTopic = snapshot.forumTopics.first { $0.id == topicId }
        }
        persist()
    }

    public func toggleForumReaction(topicId: String) {
        snapshot.forumTopics = snapshot.forumTopics.map { topic in
            guard topic.id == topicId else { return topic }
            let nextReacted = !topic.myReacted
            return ForumTopicItem(
                id: topic.id,
                categorySlug: topic.categorySlug,
                kind: topic.kind,
                title: topic.title,
                summary: topic.summary,
                body: topic.body,
                authorId: topic.authorId,
                authorName: topic.authorName,
                regionLabel: topic.regionLabel,
                createdAtLabel: topic.createdAtLabel,
                externalReferenceUrl: topic.externalReferenceUrl,
                reactionCount: max(0, topic.reactionCount + (nextReacted ? 1 : -1)),
                myReacted: nextReacted,
                comments: topic.comments
            )
        }
        if let current = selectedForumTopic, current.id == topicId {
            selectedForumTopic = snapshot.forumTopics.first { $0.id == topicId }
        }
        persist()
    }

    public func blockUser(userId: String, alias: String) {
        guard !snapshot.blockedUsers.contains(where: { $0.id == userId }) else { return }
        snapshot.blockedUsers.append(BlockedUserEntry(id: userId, displayAlias: alias, blockedAtLabel: "Agora"))
        persist()
    }

    public func unblockUser(userId: String) {
        snapshot.blockedUsers.removeAll { $0.id == userId }
        persist()
    }

    // MARK: - Vehicles, Ledger & Driver Hub

    public func addVehicle(name: String, propulsion: Propulsion, primaryFuel: FuelProduct, tankLitersText: String, batteryKwhText: String) -> Result<VehicleRecord, String> {
        guard snapshot.vehicles.count < VehicleRecord.maxFreeVehicles else {
            return .failure("O plano gratuito comporta até 3 veículos.")
        }
        let cleanName = name.trimmingCharacters(in: .whitespacesAndNewlines)
        guard !cleanName.isEmpty else { return .failure("Informe o nome do veículo.") }
        let tankMl: Int64
        if propulsion.supportsFuel {
            guard let parsed = try? PortableMoney.parseCapacityMilliLiters(tankLitersText) else {
                return .failure("Capacidade do tanque inválida (1 a 200 L).")
            }
            tankMl = parsed
        } else {
            tankMl = 0
        }
        let batteryWh = Int64((Double(batteryKwhText.replacingOccurrences(of: ",", with: ".")) ?? 50.0) * 1000.0)
        let vehicle = VehicleRecord(
            name: cleanName,
            propulsion: propulsion,
            primaryFuel: primaryFuel,
            tankCapacityMilliLiters: tankMl,
            batteryCapacityWh: propulsion.supportsElectric ? max(1000, batteryWh) : 0
        )
        snapshot.vehicles.append(vehicle)
        persist()
        return .success(vehicle)
    }

    public func removeVehicle(_ vehicleId: String) {
        snapshot.vehicles.removeAll { $0.id == vehicleId }
        persist()
    }

    public func addLedgerEntry(vehicleId: String, kind: LedgerEntryKind, odometerKm: Int64, quantityMilli: Int64, totalCentavos: Int64, note: String) {
        let formatter = DateFormatter()
        formatter.dateFormat = "dd/MM/yyyy"
        let dateStr = formatter.string(from: Date())
        formatter.dateFormat = "yyyy-MM"
        let monthStr = formatter.string(from: Date())
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
    }

    public func startDriverShift(vehicleId: String, startOdometerKm: Int64) {
        let shift = DriverShiftRecord(vehicleId: vehicleId, startedAtLabel: "Agora", startOdometerKm: startOdometerKm, isActive: true)
        snapshot.driverShifts.insert(shift, at: 0)
        persist()
    }

    public func finishActiveDriverShift(endOdometerKm: Int64, grossCentavos: Int64, fuelCentavos: Int64, expenseCentavos: Int64, minutes: Int) {
        guard let idx = snapshot.driverShifts.firstIndex(where: { $0.isActive }) else { return }
        let current = snapshot.driverShifts[idx]
        snapshot.driverShifts[idx] = DriverShiftRecord(
            id: current.id,
            vehicleId: current.vehicleId,
            startedAtLabel: current.startedAtLabel,
            endedAtLabel: "Encerrado",
            startOdometerKm: current.startOdometerKm,
            endOdometerKm: max(current.startOdometerKm, endOdometerKm),
            grossEarningsCentavos: grossCentavos,
            fuelCostCentavos: fuelCentavos,
            otherExpenseCentavos: expenseCentavos,
            activeMinutes: max(1, minutes),
            isActive: false
        )
        persist()
    }

    // MARK: - Map, Voice Guidance & Downloads

    public func speakGuidanceStep(_ text: String) {
        guard isVoiceGuidanceEnabled else { return }
        voiceGuidance.speak(instruction: text, localeCode: currentLocale.rawValue)
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

    public func toggleDownloadMapRegion(_ package: MapRegionPackage) {
        if let idx = snapshot.downloadedMapRegions.firstIndex(of: package.id) {
            snapshot.downloadedMapRegions.remove(at: idx)
        } else {
            snapshot.downloadedMapRegions.append(package.id)
        }
        persist()
    }

    // MARK: - Training Consent (RVA08 owner-scoped)

    public var currentConsentOwnerScope: String? {
        guard let accountId = currentAccountIdOrNil,
              let fp = proofSigner.ensurePublicJwk()?.fingerprint else {
            return nil
        }
        return "account:\(accountId):\(fp)|\(snapshot.apiEnvironment.origin)"
    }

    public var currentTrainingConsent: TrainingConsentRecord {
        guard let scope = currentConsentOwnerScope else {
            return TrainingConsentRecord(ownerKey: "", origin: snapshot.apiEnvironment.origin, granted: false)
        }
        return snapshot.trainingConsentByOwner[scope]
            ?? TrainingConsentRecord(ownerKey: scope, origin: snapshot.apiEnvironment.origin, granted: false)
    }

    public func grantTrainingConsent(explicitlyChecked: Bool) -> Bool {
        guard explicitlyChecked,
              let scope = currentConsentOwnerScope,
              TrainingTermsContract.verifiedDigestMatches() else {
            return false
        }
        let nextRev = currentTrainingConsent.revision + 1
        let record = TrainingConsentRecord(
            ownerKey: scope,
            origin: snapshot.apiEnvironment.origin,
            granted: true,
            pendingSync: isOfflineSimulation,
            termsVersion: TrainingTermsContract.version,
            termsDigest: TrainingTermsContract.digestSha256,
            revision: nextRev,
            updatedAtLabel: "Agora"
        )
        snapshot.trainingConsentByOwner[scope] = record
        persist()
        if !isOfflineSimulation {
            let cmdId = UUID().uuidString.lowercased()
            Task {
                let synced = await httpClient.syncTrainingConsentGrant(record: record, commandId: cmdId)
                if !synced {
                    let copy = TrainingConsentRecord(
                        ownerKey: record.ownerKey,
                        origin: record.origin,
                        granted: record.granted,
                        pendingSync: true,
                        termsVersion: record.termsVersion,
                        termsDigest: record.termsDigest,
                        revision: record.revision,
                        updatedAtLabel: record.updatedAtLabel
                    )
                    self.snapshot.trainingConsentByOwner[scope] = copy
                    self.persist()
                }
            }
        }
        return true
    }

    public func revokeTrainingConsentImmediately() {
        guard let scope = currentConsentOwnerScope else { return }
        let nextRev = currentTrainingConsent.revision + 1
        let record = TrainingConsentRecord(
            ownerKey: scope,
            origin: snapshot.apiEnvironment.origin,
            granted: false,
            pendingSync: true,
            termsVersion: TrainingTermsContract.version,
            termsDigest: TrainingTermsContract.digestSha256,
            revision: nextRev,
            updatedAtLabel: "Revogado agora"
        )
        snapshot.trainingConsentByOwner[scope] = record
        persist()
        if !isOfflineSimulation {
            let cmdId = UUID().uuidString.lowercased()
            Task {
                let ok = await httpClient.syncTrainingConsentRevoke(revision: nextRev, commandId: cmdId)
                if ok {
                    let synced = TrainingConsentRecord(
                        ownerKey: scope,
                        origin: snapshot.apiEnvironment.origin,
                        granted: false,
                        pendingSync: false,
                        termsVersion: TrainingTermsContract.version,
                        termsDigest: TrainingTermsContract.digestSha256,
                        revision: nextRev,
                        updatedAtLabel: "Revogado e confirmado"
                    )
                    self.snapshot.trainingConsentByOwner[scope] = synced
                    self.persist()
                }
            }
        }
    }

    // MARK: - Enterprise Suite

    public func joinSampleEnterprise(inviteCode: String) {
        let clean = inviteCode.trimmingCharacters(in: .whitespacesAndNewlines).uppercased()
        let org = EnterpriseOrganization(
            id: "org-\(clean.isEmpty ? "DEMO" : clean)",
            name: "Transportadora Horizonte (\(clean.isEmpty ? "DEMO" : clean))",
            cnpj: "67.852.667/0001-93",
            roleTitle: "Motorista",
            status: "ATIVO"
        )
        if !snapshot.enterpriseOrganizations.contains(where: { $0.id == org.id }) {
            snapshot.enterpriseOrganizations.append(org)
        }
        snapshot.activeOrganizationId = org.id
        if snapshot.enterpriseTrips.isEmpty {
            snapshot.enterpriseTrips = [
                EnterpriseTripMission(
                    id: "emp-trip-1",
                    code: "MIS-402",
                    originLabel: selectedMunicipality.displayTitle,
                    destinationLabel: "Terminal Logístico Sul",
                    vehiclePlate: "ABV-3A21",
                    status: .assigned
                )
            ]
        }
        persist()
    }

    public func startEnterpriseTrip(_ tripId: String, odometerKm: Int64, explicitTrackingGrant: Bool) {
        snapshot.enterpriseTrips = snapshot.enterpriseTrips.map { trip in
            guard trip.id == tripId else { return trip }
            return EnterpriseTripMission(
                id: trip.id,
                code: trip.code,
                originLabel: trip.originLabel,
                destinationLabel: trip.destinationLabel,
                vehiclePlate: trip.vehiclePlate,
                status: .active,
                startOdometerKm: odometerKm,
                endOdometerKm: nil,
                trackingGrantedFromScreen: explicitTrackingGrant,
                fuelingAuthCode: "ABV-\(Int.random(in: 1000...9999))"
            )
        }
        persist()
    }

    public func finishEnterpriseTrip(_ tripId: String, endOdometerKm: Int64) {
        snapshot.enterpriseTrips = snapshot.enterpriseTrips.map { trip in
            guard trip.id == tripId else { return trip }
            return EnterpriseTripMission(
                id: trip.id,
                code: trip.code,
                originLabel: trip.originLabel,
                destinationLabel: trip.destinationLabel,
                vehiclePlate: trip.vehiclePlate,
                status: .finished,
                startOdometerKm: trip.startOdometerKm,
                endOdometerKm: max(trip.startOdometerKm ?? 0, endOdometerKm),
                trackingGrantedFromScreen: false,
                fuelingAuthCode: nil
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
    case nativeRoute
    case evTripPlanner
    case savedPlaces
    case mapDownloads
    case auth
    case accountManagement
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

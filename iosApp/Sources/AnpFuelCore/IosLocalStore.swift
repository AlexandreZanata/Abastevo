import Foundation

/// Offline-first local persistence (backup-excluded per SEC05-D) and complete
/// Brazilian seed catalog synchronized with `main`.
/// Zero postfix unwrap operators anywhere in this file.
public struct PersistedAppSnapshot: Codable, Equatable, Sendable {
    public var termsAccepted: Bool
    public var onboardingCompleted: Bool
    public var selectedMunicipalityCode: String
    public var selectedWeekId: String
    public var selectedFuel: FuelProduct
    public var localeCode: String
    public var darkThemeEnabled: Bool?
    public var apiEnvironment: ApiEnvironment
    public var vehicles: [VehicleRecord]
    public var ledgerEntries: [LedgerEntryRecord]
    public var driverShifts: [DriverShiftRecord]
    public var priceDropAlerts: [String]
    public var outboxCommands: [OutboxCommand]
    public var myContributions: [MyContributionEntry]
    public var stationComments: [StationCommentItem]
    public var forumTopics: [ForumTopicItem]
    public var blockedUsers: [BlockedUserEntry]
    public var savedPlaces: [SavedPlaceRecord]
    public var savedTrips: [SavedTripRecord]
    public var downloadedMapRegions: [String]
    public var downloadedWeekIds: [String]
    public var trainingConsentByOwner: [String: TrainingConsentRecord]
    public var enterpriseOrganizations: [EnterpriseOrganization]
    public var activeOrganizationId: String?
    public var enterpriseTrips: [EnterpriseTripMission]
    public var enterpriseExpenses: [EnterpriseExpenseItem]
    public var enterpriseChecklists: [EnterpriseChecklistRun]
    public var stationSuggestions: [StationSuggestionDraft]
    public var qualityOccurrences: [QualityOccurrenceRecord]
    public var bestFuelRunScore: Int
    public var bestFuelFlightScore: Int

    public init(
        termsAccepted: Bool = false,
        onboardingCompleted: Bool = false,
        selectedMunicipalityCode: String = "3550308",
        selectedWeekId: String = "2026-W46",
        selectedFuel: FuelProduct = .gasolineRegular,
        localeCode: String = "pt-BR",
        darkThemeEnabled: Bool? = nil,
        apiEnvironment: ApiEnvironment = .staging,
        vehicles: [VehicleRecord] = [],
        ledgerEntries: [LedgerEntryRecord] = [],
        driverShifts: [DriverShiftRecord] = [],
        priceDropAlerts: [String] = [],
        outboxCommands: [OutboxCommand] = [],
        myContributions: [MyContributionEntry] = [],
        stationComments: [StationCommentItem] = SeedCatalog.defaultComments,
        forumTopics: [ForumTopicItem] = SeedCatalog.defaultForumTopics,
        blockedUsers: [BlockedUserEntry] = [],
        savedPlaces: [SavedPlaceRecord] = SeedCatalog.defaultSavedPlaces,
        savedTrips: [SavedTripRecord] = SeedCatalog.defaultSavedTrips,
        downloadedMapRegions: [String] = ["sudeste-sp"],
        downloadedWeekIds: [String] = ["2026-W46", "2026-W45"],
        trainingConsentByOwner: [String: TrainingConsentRecord] = [:],
        enterpriseOrganizations: [EnterpriseOrganization] = [],
        activeOrganizationId: String? = nil,
        enterpriseTrips: [EnterpriseTripMission] = [],
        enterpriseExpenses: [EnterpriseExpenseItem] = [],
        enterpriseChecklists: [EnterpriseChecklistRun] = [],
        stationSuggestions: [StationSuggestionDraft] = [],
        qualityOccurrences: [QualityOccurrenceRecord] = SeedCatalog.defaultOccurrences,
        bestFuelRunScore: Int = 0,
        bestFuelFlightScore: Int = 0
    ) {
        self.termsAccepted = termsAccepted
        self.onboardingCompleted = onboardingCompleted
        self.selectedMunicipalityCode = selectedMunicipalityCode
        self.selectedWeekId = selectedWeekId
        self.selectedFuel = selectedFuel
        self.localeCode = localeCode
        self.darkThemeEnabled = darkThemeEnabled
        self.apiEnvironment = apiEnvironment
        self.vehicles = vehicles
        self.ledgerEntries = ledgerEntries
        self.driverShifts = driverShifts
        self.priceDropAlerts = priceDropAlerts
        self.outboxCommands = outboxCommands
        self.myContributions = myContributions
        self.stationComments = stationComments
        self.forumTopics = forumTopics
        self.blockedUsers = blockedUsers
        self.savedPlaces = savedPlaces
        self.savedTrips = savedTrips
        self.downloadedMapRegions = downloadedMapRegions
        self.downloadedWeekIds = downloadedWeekIds
        self.trainingConsentByOwner = trainingConsentByOwner
        self.enterpriseOrganizations = enterpriseOrganizations
        self.activeOrganizationId = activeOrganizationId
        self.enterpriseTrips = enterpriseTrips
        self.enterpriseExpenses = enterpriseExpenses
        self.enterpriseChecklists = enterpriseChecklists
        self.stationSuggestions = stationSuggestions
        self.qualityOccurrences = qualityOccurrences
        self.bestFuelRunScore = bestFuelRunScore
        self.bestFuelFlightScore = bestFuelFlightScore
    }
}

public final class IosLocalStore {
    private let fileUrl: URL
    private let lock = NSLock()

    public init(directory: URL? = nil) {
        let baseDir = directory ?? Self.defaultDirectory()
        try? FileManager.default.createDirectory(at: baseDir, withIntermediateDirectories: true)
        Self.excludeFromBackup(baseDir)
        self.fileUrl = baseDir.appendingPathComponent("abastevo-state-v3.json")
    }

    public static func defaultDirectory() -> URL {
        let appSupport = FileManager.default.urls(for: .applicationSupportDirectory, in: .userDomainMask).first
            ?? FileManager.default.temporaryDirectory
        return appSupport.appendingPathComponent("anpfuel-store", isDirectory: true)
    }

    public func load() -> PersistedAppSnapshot {
        lock.lock()
        defer { lock.unlock() }
        guard let data = try? Data(contentsOf: fileUrl),
              let decoded = try? JSONDecoder().decode(PersistedAppSnapshot.self, from: data) else {
            return PersistedAppSnapshot()
        }
        return decoded
    }

    public func save(_ snapshot: PersistedAppSnapshot) {
        lock.lock()
        defer { lock.unlock() }
        let tempUrl = fileUrl.appendingPathExtension("tmp")
        guard let data = try? JSONEncoder().encode(snapshot) else { return }
        try? data.write(to: tempUrl, options: .atomic)
        try? FileManager.default.removeItem(at: fileUrl)
        try? FileManager.default.moveItem(at: tempUrl, to: fileUrl)
        Self.excludeFromBackup(fileUrl)
    }

    /// SEC04-D: wipes owner-scoped private data on logout/deletion while preserving public catalog selections.
    public func purgeOwnedData(snapshot: inout PersistedAppSnapshot, ownerPrefix: String?) {
        snapshot.outboxCommands.removeAll()
        snapshot.myContributions.removeAll()
        snapshot.driverShifts.removeAll()
        snapshot.ledgerEntries.removeAll()
        snapshot.enterpriseOrganizations.removeAll()
        snapshot.activeOrganizationId = nil
        snapshot.enterpriseTrips.removeAll()
        snapshot.enterpriseExpenses.removeAll()
        snapshot.enterpriseChecklists.removeAll()
        if let prefix = ownerPrefix {
            snapshot.trainingConsentByOwner = snapshot.trainingConsentByOwner.filter { key, _ in
                !key.hasPrefix(prefix)
            }
        } else {
            snapshot.trainingConsentByOwner.removeAll()
        }
        save(snapshot)
    }

    public func estimatedStorageBytes() -> Int64 {
        let attrs = try? FileManager.default.attributesOfItem(atPath: fileUrl.path)
        return (attrs?[.size] as? NSNumber)?.int64Value ?? 0
    }

    public func clearCacheKeepingAccount(_ snapshot: inout PersistedAppSnapshot) {
        snapshot.downloadedWeekIds = ["2026-W46"]
        snapshot.downloadedMapRegions = []
        snapshot.stationComments = SeedCatalog.defaultComments
        snapshot.forumTopics = SeedCatalog.defaultForumTopics
        save(snapshot)
    }

    private static func excludeFromBackup(_ url: URL) {
        var mutableUrl = url
        var values = URLResourceValues()
        values.isExcludedFromBackup = true
        try? mutableUrl.setResourceValues(values)
    }
}

public enum SeedCatalog {
    public static let municipalities: [Municipality] = [
        Municipality(ibgeCode: "3550308", name: "São Paulo", state: .SP, latitude: -23.5505, longitude: -46.6333),
        Municipality(ibgeCode: "3509502", name: "Campinas", state: .SP, latitude: -22.9056, longitude: -47.0608),
        Municipality(ibgeCode: "3548500", name: "Santos", state: .SP, latitude: -23.9608, longitude: -46.3336),
        Municipality(ibgeCode: "3304557", name: "Rio de Janeiro", state: .RJ, latitude: -22.9068, longitude: -43.1729),
        Municipality(ibgeCode: "3303302", name: "Niterói", state: .RJ, latitude: -22.8832, longitude: -43.1034),
        Municipality(ibgeCode: "3106200", name: "Belo Horizonte", state: .MG, latitude: -19.9167, longitude: -43.9345),
        Municipality(ibgeCode: "3170206", name: "Uberlândia", state: .MG, latitude: -18.9186, longitude: -48.2772),
        Municipality(ibgeCode: "4106902", name: "Curitiba", state: .PR, latitude: -25.4284, longitude: -49.2733),
        Municipality(ibgeCode: "4113700", name: "Londrina", state: .PR, latitude: -23.3103, longitude: -51.1628),
        Municipality(ibgeCode: "4314902", name: "Porto Alegre", state: .RS, latitude: -30.0346, longitude: -51.2177),
        Municipality(ibgeCode: "4205407", name: "Florianópolis", state: .SC, latitude: -27.5954, longitude: -48.5480),
        Municipality(ibgeCode: "5300108", name: "Brasília", state: .DF, latitude: -15.7975, longitude: -47.8919),
        Municipality(ibgeCode: "5208707", name: "Goiânia", state: .GO, latitude: -16.6869, longitude: -49.2648),
        Municipality(ibgeCode: "5107925", name: "Sorriso", state: .MT, latitude: -12.5425, longitude: -55.7211),
        Municipality(ibgeCode: "5103403", name: "Cuiabá", state: .MT, latitude: -15.6014, longitude: -56.0979),
        Municipality(ibgeCode: "5002704", name: "Campo Grande", state: .MS, latitude: -20.4697, longitude: -54.6201),
        Municipality(ibgeCode: "2927408", name: "Salvador", state: .BA, latitude: -12.9714, longitude: -38.5014),
        Municipality(ibgeCode: "2611606", name: "Recife", state: .PE, latitude: -8.0476, longitude: -34.8770),
        Municipality(ibgeCode: "2304400", name: "Fortaleza", state: .CE, latitude: -3.7319, longitude: -38.5267),
        Municipality(ibgeCode: "2408102", name: "Natal", state: .RN, latitude: -5.7945, longitude: -35.2110),
        Municipality(ibgeCode: "2507507", name: "João Pessoa", state: .PB, latitude: -7.1195, longitude: -34.8450),
        Municipality(ibgeCode: "2704302", name: "Maceió", state: .AL, latitude: -9.6658, longitude: -35.7350),
        Municipality(ibgeCode: "2800308", name: "Aracaju", state: .SE, latitude: -10.9472, longitude: -37.0731),
        Municipality(ibgeCode: "2211001", name: "Teresina", state: .PI, latitude: -5.0919, longitude: -42.8034),
        Municipality(ibgeCode: "2111300", name: "São Luís", state: .MA, latitude: -2.5307, longitude: -44.3068),
        Municipality(ibgeCode: "1302603", name: "Manaus", state: .AM, latitude: -3.1190, longitude: -60.0217),
        Municipality(ibgeCode: "1501402", name: "Belém", state: .PA, latitude: -1.4558, longitude: -48.4902),
        Municipality(ibgeCode: "1721000", name: "Palmas", state: .TO, latitude: -10.1840, longitude: -48.3336),
        Municipality(ibgeCode: "1100205", name: "Porto Velho", state: .RO, latitude: -8.7612, longitude: -63.9004),
        Municipality(ibgeCode: "1200401", name: "Rio Branco", state: .AC, latitude: -9.9754, longitude: -67.8249),
        Municipality(ibgeCode: "1600303", name: "Macapá", state: .AP, latitude: 0.0349, longitude: -51.0694),
        Municipality(ibgeCode: "1400100", name: "Boa Vista", state: .RR, latitude: 2.8235, longitude: -60.6758),
        Municipality(ibgeCode: "3205309", name: "Vitória", state: .ES, latitude: -20.3155, longitude: -40.3128),
    ]

    public static let surveyWeeks: [SurveyWeek] = [
        SurveyWeek(
            id: "2026-W46",
            label: "15/11/2026 a 21/11/2026 (Mais recente)",
            startDate: "2026-11-15",
            endDate: "2026-11-21",
            isLatest: true,
            isDownloaded: true,
            stationCount: 4820,
            operationalNote: "Pesquisa semanal oficial ANP consolidada com revisão de bandeiras e CNPJ."
        ),
        SurveyWeek(
            id: "2026-W45",
            label: "08/11/2026 a 14/11/2026",
            startDate: "2026-11-08",
            endDate: "2026-11-14",
            isLatest: false,
            isDownloaded: true,
            stationCount: 4795,
            operationalNote: nil
        ),
        SurveyWeek(
            id: "2026-W44",
            label: "01/11/2026 a 07/11/2026",
            startDate: "2026-11-01",
            endDate: "2026-11-07",
            isLatest: false,
            isDownloaded: false,
            stationCount: 4812,
            operationalNote: nil
        ),
        SurveyWeek(
            id: "2026-W43",
            label: "25/10/2026 a 31/10/2026",
            startDate: "2026-10-25",
            endDate: "2026-10-31",
            isLatest: false,
            isDownloaded: false,
            stationCount: 4760,
            operationalNote: nil
        ),
    ]

    public static func stationsFor(municipality: Municipality) -> [StationRecord] {
        let baseLat = municipality.latitude
        let baseLon = municipality.longitude
        let code = municipality.ibgeCode
        let city = municipality.name
        let nowSec = Int64(Date().timeIntervalSince1970)
        return [
            StationRecord(
                id: "st-\(code)-001",
                cnpj: "11222333000181",
                name: "Auto Posto Paulista Centro",
                brand: "Ipiranga",
                address: "Av. Principal, 1240",
                neighborhood: "Centro",
                municipalityCode: code,
                latitude: baseLat + 0.0042,
                longitude: baseLon - 0.0038,
                pinTier: .reviewed,
                officialPricesMilli: [
                    .gasolineRegular: 5990,
                    .gasolinePremium: 6290,
                    .gasolinePremiumGrade: 7490,
                    .ethanol: 3980,
                    .dieselS10: 6150,
                    .dieselS500: 5980,
                    .cng: 4690,
                    .lpgP13: 108000,
                ],
                officialDateLabel: "ANP 19/11/2026",
                communityObservations: [
                    CommunityPriceObservation(
                        id: "obs-\(code)-1",
                        stationId: "st-\(code)-001",
                        stationName: "Auto Posto Paulista Centro",
                        municipalityCode: code,
                        product: .gasolineRegular,
                        amountMilli: 5890,
                        condition: .pix,
                        confirmations: 6,
                        disputes: 0,
                        helpfulCount: 9,
                        notHelpfulCount: 1,
                        observedAtLabel: "Hoje, 08:40",
                        observedEpochSeconds: nowSec - 1800,
                        hasPhotoEvidence: true
                    ),
                    CommunityPriceObservation(
                        id: "obs-\(code)-2",
                        stationId: "st-\(code)-001",
                        stationName: "Auto Posto Paulista Centro",
                        municipalityCode: code,
                        product: .ethanol,
                        amountMilli: 3890,
                        condition: .standard,
                        confirmations: 4,
                        disputes: 0,
                        helpfulCount: 5,
                        notHelpfulCount: 0,
                        observedAtLabel: "Hoje, 07:15",
                        observedEpochSeconds: nowSec - 7200,
                        hasPhotoEvidence: true
                    ),
                ],
                openingHours: "24 horas",
                paymentMethods: ["Pix", "Débito", "Crédito", "App Ipiranga"],
                qualitySummary: StationQualitySummary(samplesTested: 8, nonConformingCount: 0, enforcementNotices: 0, lastAuditLabel: "2026-10"),
                isClaimedOwner: true,
                ownerPromotionText: "Desconto de R$ 0,10/L no Pix às terças e quintas"
            ),
            StationRecord(
                id: "st-\(code)-002",
                cnpj: "22333444000192",
                name: "Posto Bandeira Branca Economia (\(city))",
                brand: "Bandeira Branca",
                address: "Rua das Flores, 455",
                neighborhood: "Jardim América",
                municipalityCode: code,
                latitude: baseLat - 0.0055,
                longitude: baseLon + 0.0041,
                pinTier: .osmMapped,
                officialPricesMilli: [
                    .gasolineRegular: 5850,
                    .gasolinePremium: 6090,
                    .ethanol: 3820,
                    .dieselS10: 5990,
                    .dieselS500: 5870,
                ],
                officialDateLabel: "ANP 18/11/2026",
                communityObservations: [
                    CommunityPriceObservation(
                        id: "obs-\(code)-3",
                        stationId: "st-\(code)-002",
                        stationName: "Posto Bandeira Branca Economia (\(city))",
                        municipalityCode: code,
                        product: .gasolineRegular,
                        amountMilli: 5790,
                        condition: .standard,
                        confirmations: 8,
                        disputes: 1,
                        helpfulCount: 12,
                        notHelpfulCount: 1,
                        observedAtLabel: "Ontem, 18:20",
                        observedEpochSeconds: nowSec - 36000,
                        hasPhotoEvidence: true
                    )
                ],
                openingHours: "06:00 - 23:00",
                paymentMethods: ["Dinheiro", "Pix", "Débito"],
                qualitySummary: StationQualitySummary(samplesTested: 5, nonConformingCount: 0, enforcementNotices: 0, lastAuditLabel: "2026-09")
            ),
            StationRecord(
                id: "st-\(code)-003",
                cnpj: "33444555000103",
                name: "Posto Petrobras Rodovia",
                brand: "Vibra / Petrobras",
                address: "Av. Marginal Sul, km 14",
                neighborhood: "Distrito Industrial",
                municipalityCode: code,
                latitude: baseLat + 0.0085,
                longitude: baseLon + 0.0072,
                pinTier: .anpReported,
                officialPricesMilli: [
                    .gasolineRegular: 6120,
                    .gasolinePremium: 6390,
                    .gasolinePremiumGrade: 7790,
                    .ethanol: 4140,
                    .dieselS10: 6040,
                    .dieselS500: 5910,
                    .cng: 4550,
                ],
                officialDateLabel: "ANP 19/11/2026",
                communityObservations: [],
                openingHours: "24 horas",
                paymentMethods: ["Pix", "Débito", "Crédito", "Premmia"],
                qualitySummary: StationQualitySummary(samplesTested: 11, nonConformingCount: 1, enforcementNotices: 1, lastAuditLabel: "2026-10")
            ),
            StationRecord(
                id: "st-\(code)-004",
                cnpj: "44555666000114",
                name: "Auto Posto Shell Avenida",
                brand: "Raízen / Shell",
                address: "Av. Brasil, 890",
                neighborhood: "Vila Nova",
                municipalityCode: code,
                latitude: baseLat - 0.0021,
                longitude: baseLon - 0.0064,
                pinTier: .reviewed,
                officialPricesMilli: [
                    .gasolineRegular: 6040,
                    .gasolinePremium: 6340,
                    .gasolinePremiumGrade: 7850,
                    .ethanol: 4050,
                    .dieselS10: 6180,
                ],
                officialDateLabel: "ANP 18/11/2026",
                communityObservations: [
                    CommunityPriceObservation(
                        id: "obs-\(code)-4",
                        stationId: "st-\(code)-004",
                        stationName: "Auto Posto Shell Avenida",
                        municipalityCode: code,
                        product: .gasolinePremiumGrade,
                        amountMilli: 7690,
                        condition: .app,
                        confirmations: 3,
                        disputes: 0,
                        helpfulCount: 4,
                        notHelpfulCount: 0,
                        observedAtLabel: "Hoje, 06:50",
                        observedEpochSeconds: nowSec - 9400,
                        hasPhotoEvidence: true
                    )
                ],
                openingHours: "05:30 - 23:30",
                paymentMethods: ["Shell Box", "Pix", "Crédito", "Débito"],
                qualitySummary: StationQualitySummary(samplesTested: 7, nonConformingCount: 0, enforcementNotices: 0, lastAuditLabel: "2026-11")
            ),
            StationRecord(
                id: "st-\(code)-005",
                cnpj: "55666777000125",
                name: "Posto Bairro Esperança (Sem pino exato)",
                brand: "Alesat",
                address: "Estrada Municipal s/n",
                neighborhood: "Zona Rural",
                municipalityCode: code,
                latitude: baseLat,
                longitude: baseLon,
                pinTier: .cityCentroid,
                officialPricesMilli: [
                    .gasolineRegular: 5940,
                    .ethanol: 3940,
                    .dieselS500: 5950,
                    .lpgP13: 105000,
                ],
                officialDateLabel: "ANP 17/11/2026",
                communityObservations: [],
                openingHours: "07:00 - 20:00",
                paymentMethods: ["Dinheiro", "Pix"],
                qualitySummary: StationQualitySummary(samplesTested: 2, nonConformingCount: 0, enforcementNotices: 0, lastAuditLabel: "2026-08")
            ),
        ]
    }

    public static func officialAverage(for municipality: Municipality, weekId: String, product: FuelProduct, stations: [StationRecord]) -> OfficialAnpAverage? {
        let prices = stations.compactMap { $0.officialPrice(for: product) }
        guard !prices.isEmpty else { return nil }
        let sum = prices.reduce(0, +)
        let mean = sum / Int64(prices.count)
        let minP = prices.min() ?? mean
        let maxP = prices.max() ?? mean
        return OfficialAnpAverage(
            municipalityCode: municipality.ibgeCode,
            surveyWeekId: weekId,
            product: product,
            meanMilli: mean,
            minMilli: minP,
            maxMilli: maxP,
            stationCount: prices.count,
            surveyDateLabel: "Semana \(weekId)"
        )
    }

    public static func priceHistory(for product: FuelProduct) -> [PriceHistoryPoint] {
        let base: Int64
        switch product {
        case .gasolineRegular: base = 5990
        case .gasolinePremium: base = 6260
        case .gasolinePremiumGrade: base = 7620
        case .ethanol: base = 3980
        case .dieselS10: base = 6090
        case .dieselS500: base = 5920
        case .cng: base = 4620
        case .lpgP13: base = 107000
        }
        return [
            PriceHistoryPoint(weekId: "2026-W43", weekLabel: "25/10", product: product, meanMilli: base + 90, minMilli: base - 120, maxMilli: base + 290),
            PriceHistoryPoint(weekId: "2026-W44", weekLabel: "01/11", product: product, meanMilli: base + 50, minMilli: base - 140, maxMilli: base + 240),
            PriceHistoryPoint(weekId: "2026-W45", weekLabel: "08/11", product: product, meanMilli: base + 20, minMilli: base - 150, maxMilli: base + 210),
            PriceHistoryPoint(weekId: "2026-W46", weekLabel: "15/11", product: product, meanMilli: base, minMilli: base - 170, maxMilli: base + 190),
        ]
    }

    public static let evChargingSites: [EvChargingSite] = [
        EvChargingSite(
            id: "ev-sp-01",
            name: "Eletroposto Marginal Pinheiros Hub",
            operatorName: "Abastevo EV Rede",
            cityLabel: "São Paulo, SP",
            municipalityCode: "3550308",
            latitude: -23.5672,
            longitude: -46.7019,
            connectorTypes: ["CCS2", "Type 2"],
            maxPowerKw: 150,
            pricePerKwhCentavos: 215,
            sessionFeeCentavos: 350,
            availablePorts: 3,
            totalPorts: 4,
            lastVerifiedLabel: "Verificado hoje"
        ),
        EvChargingSite(
            id: "ev-registro-02",
            name: "Parada Registro BR-116 Ultra",
            operatorName: "Corredor Sul EV",
            cityLabel: "Registro, SP",
            municipalityCode: "3542602",
            latitude: -24.4961,
            longitude: -47.8447,
            connectorTypes: ["CCS2", "CHAdeMO"],
            maxPowerKw: 120,
            pricePerKwhCentavos: 229,
            sessionFeeCentavos: 0,
            availablePorts: 2,
            totalPorts: 2,
            lastVerifiedLabel: "Verificado ontem"
        ),
        EvChargingSite(
            id: "ev-curitiba-03",
            name: "Estação Batel EcoCharge",
            operatorName: "Paraná Energia",
            cityLabel: "Curitiba, PR",
            municipalityCode: "4106902",
            latitude: -25.4412,
            longitude: -49.2881,
            connectorTypes: ["CCS2", "Type 2"],
            maxPowerKw: 80,
            pricePerKwhCentavos: 199,
            sessionFeeCentavos: 200,
            availablePorts: 2,
            totalPorts: 3,
            lastVerifiedLabel: "Verificado há 2 dias"
        ),
        EvChargingSite(
            id: "ev-bsb-04",
            name: "Eixo Monumental Eletroposto",
            operatorName: "Planalto Charge",
            cityLabel: "Brasília, DF",
            municipalityCode: "5300108",
            latitude: -15.7895,
            longitude: -47.8825,
            connectorTypes: ["CCS2", "Type 2"],
            maxPowerKw: 60,
            pricePerKwhCentavos: 195,
            sessionFeeCentavos: 0,
            availablePorts: 4,
            totalPorts: 4,
            lastVerifiedLabel: "Verificado hoje"
        ),
    ]

    public static let forumCategories: [ForumCategoryItem] = [
        ForumCategoryItem(id: "cat-guias", slug: "guias", title: "Guias e Direitos", postCount: 4),
        ForumCategoryItem(id: "cat-qualidade", slug: "qualidade", title: "Qualidade e PMQC", postCount: 3),
        ForumCategoryItem(id: "cat-ev", slug: "mobilidade-eletrica", title: "Mobilidade Elétrica", postCount: 2),
        ForumCategoryItem(id: "cat-regioes", slug: "dicas-regionais", title: "Dicas Regionais", postCount: 5),
    ]

    public static let defaultForumTopics: [ForumTopicItem] = [
        ForumTopicItem(
            id: "11111111-1111-4111-8111-111111111111",
            categorySlug: "guias",
            kind: .editorial,
            title: "Etanol ou Gasolina: quando os 70% valem e como medir seu carro",
            summary: "Entenda o critério de paridade energética e como registrar consumo real no seu veículo.",
            body: "A regra dos 70% é uma média prática baseada na densidade energética do etanol hidratado frente à gasolina comum C. Veículos flex modernos podem alcançar entre 68% e 74% de rendimento relativo dependendo do trajeto urbano ou rodoviário. No Abastevo, você pode cadastrar até 3 veículos gratuitamente e ajustar a eficiência real do seu modelo.",
            authorId: "editorial-team",
            authorName: "Equipe abastevo",
            regionLabel: "Brasil",
            createdAtLabel: "18/11/2026",
            externalReferenceUrl: "https://abastevo.com.br/guias/etanol-ou-gasolina/",
            reactionCount: 24,
            comments: [
                ForumCommentItem(
                    id: "c-101",
                    topicId: "11111111-1111-4111-8111-111111111111",
                    authorId: "user-marcos",
                    authorName: "Marcos S.",
                    body: "No meu carro 1.0 turbo na estrada, o etanol rende 72% da gasolina.",
                    createdAtLabel: "19/11/2026",
                    helpfulCount: 6
                )
            ]
        ),
        ForumTopicItem(
            id: "22222222-2222-4222-8222-222222222222",
            categorySlug: "qualidade",
            kind: .editorial,
            title: "Como funciona o PMQC da ANP e o que observar na bomba",
            summary: "Saiba diferenciar dados oficiais do Programa de Monitoramento da Qualidade dos Combustíveis de relatos da comunidade.",
            body: "O PMQC reúne ensaios laboratoriais oficiais sobre teor de etanol anidro na gasolina, massa específica e ponto de fulgor. O Abastevo mantém resultados oficiais da ANP separados dos relatos comunitários para garantir transparência.",
            authorId: "editorial-team",
            authorName: "Equipe abastevo",
            regionLabel: "Brasil",
            createdAtLabel: "16/11/2026",
            externalReferenceUrl: "https://abastevo.com.br/guias/identificar-combustivel-adulterado/",
            reactionCount: 19
        ),
        ForumTopicItem(
            id: "33333333-3333-4333-8333-333333333333",
            categorySlug: "mobilidade-eletrica",
            kind: .community,
            title: "Corredor SP – Curitiba com carregadores CCS2 funcionando bem",
            summary: "Relato de viagem com recarga rápida em Registro e chegada com 28% de bateria.",
            body: "Fiz o trajeto saindo de São Paulo com 92% de bateria em um hatch elétrico de 50 kWh. A parada em Registro no carregador de 120 kW foi rápida e sem fila.",
            authorId: "user-clara",
            authorName: "Clara R.",
            regionLabel: "Sudeste / Sul",
            createdAtLabel: "20/11/2026",
            reactionCount: 11
        ),
    ]

    public static let contributors: [ContributorEntry] = [
        ContributorEntry(id: "ctb-1", alias: "ana_sp", cityLabel: "São Paulo, SP", acceptedPrices: 142, confirmationsGiven: 318, badgeTitle: "Guardiã Regional"),
        ContributorEntry(id: "ctb-2", alias: "rodrigo_pr", cityLabel: "Curitiba, PR", acceptedPrices: 98, confirmationsGiven: 204, badgeTitle: "Verificador Ativo"),
        ContributorEntry(id: "ctb-3", alias: "beatriz_mt", cityLabel: "Sorriso, MT", acceptedPrices: 76, confirmationsGiven: 155, badgeTitle: "Colaboradora Verificada"),
        ContributorEntry(id: "ctb-4", alias: "carlos_mg", cityLabel: "Belo Horizonte, MG", acceptedPrices: 64, confirmationsGiven: 119, badgeTitle: "Colaborador Ativo"),
    ]

    public static let mapRegions: [MapRegionPackage] = [
        MapRegionPackage(id: "sudeste-sp", regionName: "Sudeste — São Paulo e Litoral", statesCovered: "SP", sizeMegabytes: 184, sha256Prefix: "7e9a34c1", isDownloaded: true, downloadedAtLabel: "20/11/2026"),
        MapRegionPackage(id: "sudeste-rj-mg", regionName: "Sudeste — RJ, MG e ES", statesCovered: "RJ, MG, ES", sizeMegabytes: 242, sha256Prefix: "31bc8820", isDownloaded: false),
        MapRegionPackage(id: "sul-pr-sc-rs", regionName: "Sul — Paraná, Santa Catarina e RS", statesCovered: "PR, SC, RS", sizeMegabytes: 196, sha256Prefix: "98df02aa", isDownloaded: false),
        MapRegionPackage(id: "centro-oeste", regionName: "Centro-Oeste — DF, GO, MT e MS", statesCovered: "DF, GO, MT, MS", sizeMegabytes: 138, sha256Prefix: "55ac190f", isDownloaded: false),
        MapRegionPackage(id: "nordeste", regionName: "Nordeste — Capitais e Rodovias", statesCovered: "BA, PE, CE, RN, PB, AL, SE, PI, MA", sizeMegabytes: 215, sha256Prefix: "c204d9e8", isDownloaded: false),
        MapRegionPackage(id: "norte", regionName: "Norte — Capitais e Eixos", statesCovered: "AM, PA, TO, RO, AC, AP, RR", sizeMegabytes: 118, sha256Prefix: "44ea7712", isDownloaded: false),
    ]

    public static let defaultComments: [StationCommentItem] = [
        StationCommentItem(
            id: "sc-1",
            stationId: "st-3550308-001",
            authorAlias: "ana_sp",
            text: "Preço no Pix confirmado na bomba 3 hoje pela manhã, atendimento rápido e nota fiscal emitida na hora.",
            createdAtLabel: "Hoje, 08:45",
            validVotes: 5,
            invalidVotes: 0
        )
    ]

    public static let defaultSavedPlaces: [SavedPlaceRecord] = [
        SavedPlaceRecord(id: "sp-1", title: "Auto Posto Paulista Centro", subtitle: "São Paulo, SP · Favorito", latitude: -23.5463, longitude: -46.6371, stationId: "st-3550308-001")
    ]

    public static let defaultSavedTrips: [SavedTripRecord] = [
        SavedTripRecord(id: "trip-1", title: "São Paulo → Curitiba (BR-116)", originLabel: "São Paulo, SP", destinationLabel: "Curitiba, PR", distanceKm: 408.0)
    ]

    public static let defaultOccurrences: [QualityOccurrenceRecord] = [
        QualityOccurrenceRecord(
            id: "occ-1",
            stationId: "st-3550308-003",
            stationName: "Posto Petrobras Rodovia",
            cityLabel: "São Paulo, SP",
            kind: "Autuação ANP / PMQC",
            fuelLabel: "Gasolina Comum",
            summary: "Registro histórico de inconformidade no teste de teor de etanol anidro (regularizado após nova coleta).",
            dateLabel: "12/10/2026",
            sourceLabel: "ANP / PMQC Oficial"
        )
    ]
}

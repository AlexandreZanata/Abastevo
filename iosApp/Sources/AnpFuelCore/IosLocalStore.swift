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
    public var downloadedWeekIds: [String]
    public var trainingConsentByOwner: [String: TrainingConsentRecord]
    public var stationSuggestions: [StationSuggestionDraft]
    public var qualityOccurrences: [QualityOccurrenceRecord]
    public var bestFuelRunScore: Int
    public var bestFuelFlightScore: Int

    public init(
        termsAccepted: Bool = false,
        onboardingCompleted: Bool = false,
        selectedMunicipalityCode: String = "3550308",
        selectedWeekId: String = "",
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
        stationComments: [StationCommentItem] = [],
        forumTopics: [ForumTopicItem] = [],
        blockedUsers: [BlockedUserEntry] = [],
        savedPlaces: [SavedPlaceRecord] = [],
        savedTrips: [SavedTripRecord] = [],
        downloadedWeekIds: [String] = [],
        trainingConsentByOwner: [String: TrainingConsentRecord] = [:],
        stationSuggestions: [StationSuggestionDraft] = [],
        qualityOccurrences: [QualityOccurrenceRecord] = [],
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
        self.downloadedWeekIds = downloadedWeekIds
        self.trainingConsentByOwner = trainingConsentByOwner
        self.stationSuggestions = stationSuggestions
        self.qualityOccurrences = qualityOccurrences
        self.bestFuelRunScore = bestFuelRunScore
        self.bestFuelFlightScore = bestFuelFlightScore
    }
}

public final class IosLocalStore {
    private let fileUrl: URL
    private let lock = NSLock()
    private static let oldForumSampleIDs: Set<String> = [
        "11111111-1111-4111-8111-111111111111",
        "22222222-2222-4222-8222-222222222222",
        "33333333-3333-4333-8333-333333333333",
    ]

    public init(directory: URL? = nil) {
        let baseDir = directory ?? Self.defaultDirectory()
        try? FileManager.default.createDirectory(at: baseDir, withIntermediateDirectories: true)
        Self.excludeFromBackup(baseDir)
        self.fileUrl = baseDir.appendingPathComponent("abastevo-state-v3.json")
    }

    public var savedMapDirectory: URL {
        fileUrl.deletingLastPathComponent().appendingPathComponent("saved-map", isDirectory: true)
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
              var decoded = try? JSONDecoder().decode(PersistedAppSnapshot.self, from: data) else {
            return PersistedAppSnapshot()
        }
        decoded.savedPlaces.removeAll { $0.id == "sp-1" && $0.stationId == "st-3550308-001" }
        decoded.savedTrips.removeAll { $0.id == "trip-1" }
        decoded.stationComments.removeAll { !$0.isOwn }
        decoded.forumTopics = decoded.forumTopics.compactMap { topic in
            guard Self.oldForumSampleIDs.contains(topic.id) else { return topic }
            let authoredDrafts = topic.comments.filter(\.isOwn)
            guard !authoredDrafts.isEmpty else { return nil }
            return ForumTopicItem(
                id: topic.id,
                categorySlug: "",
                kind: .community,
                title: "",
                summary: "",
                body: "",
                authorId: "",
                authorName: "",
                regionLabel: "",
                createdAtLabel: "",
                comments: authoredDrafts
            )
        }
        return decoded
    }

    public func save(_ snapshot: PersistedAppSnapshot) {
        try? saveChecked(snapshot)
    }

    public func saveChecked(_ snapshot: PersistedAppSnapshot) throws {
        lock.lock()
        defer { lock.unlock() }
        let data = try JSONEncoder().encode(snapshot)
        try data.write(to: fileUrl, options: .atomic)
        Self.excludeFromBackup(fileUrl)
    }

    /// SEC04-D: wipes owner-scoped private data on logout/deletion while preserving public catalog selections.
    public func purgeOwnedData(snapshot: inout PersistedAppSnapshot, ownerAccountId: String?) {
        snapshot.outboxCommands.removeAll()
        snapshot.myContributions.removeAll()
        snapshot.driverShifts.removeAll()
        snapshot.ledgerEntries.removeAll()
        if let accountId = ownerAccountId {
            snapshot.trainingConsentByOwner = snapshot.trainingConsentByOwner.filter { key, _ in
                !key.hasPrefix("account:\(accountId):")
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
        snapshot.downloadedWeekIds.removeAll()
        save(snapshot)
    }

    private static func excludeFromBackup(_ url: URL) {
        var mutableUrl = url
        var values = URLResourceValues()
        values.isExcludedFromBackup = true
        try? mutableUrl.setResourceValues(values)
    }
}

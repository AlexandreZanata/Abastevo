import Foundation

public struct SavedMapInventory: Codable, Equatable, Sendable {
    public var places: [SavedMapPlace]
    public var trips: [SavedMapTrip]
    public var legacyPlaces: [SavedPlaceRecord]
    public var legacyTrips: [SavedTripRecord]

    public init(
        places: [SavedMapPlace] = [], trips: [SavedMapTrip] = [],
        legacyPlaces: [SavedPlaceRecord] = [], legacyTrips: [SavedTripRecord] = []
    ) {
        self.places = places
        self.trips = trips
        self.legacyPlaces = legacyPlaces
        self.legacyTrips = legacyTrips
    }

    public var hasExportableRecords: Bool { !places.isEmpty || !trips.isEmpty || !legacyPlaces.isEmpty }

    public func hasStation(_ id: String) -> Bool {
        places.contains { $0.stationId == id.lowercased() } || legacyPlaces.contains { $0.stationId == id }
    }
}

public struct SavedMapImportResult: Equatable, Sendable {
    public let importedPlaces: Int
    public let importedTrips: Int
    public let alreadyImported: Bool
    public let skippedUnsupported: Int
    public let truncatedFields: Int
}

/// One protected private sidecar per account/guest and API origin. Imports and
/// deletes save a single atomic document containing only that scope's data.
public final class SavedMapStore: @unchecked Sendable {
    private struct Vault: Codable, Equatable {
        let scope: SavedMapScope
        var inventory = SavedMapInventory()
        var tombstones: Set<String> = []
        var importedFiles: Set<String> = []
        var migratedLegacy = false
    }

    private let directory: URL
    private let lock = NSLock()

    public init(directory: URL) { self.directory = directory }

    public func inventory(in scope: SavedMapScope) throws -> SavedMapInventory {
        lock.lock()
        defer { lock.unlock() }
        return try load(scope).inventory
    }

    @discardableResult
    public func migrateLegacy(
        _ places: [SavedPlaceRecord], trips: [SavedTripRecord], inGuestScope scope: SavedMapScope
    ) throws -> SavedMapInventory {
        guard scope.owner == SavedMapScope.guest else { throw SavedMapError.scopeChanged }
        return try update(scope) { vault in
            guard !vault.migratedLegacy else { return }
            vault.inventory.legacyPlaces = places
            vault.inventory.legacyTrips = trips
            vault.migratedLegacy = true
        }
    }

    public func toggleStation(
        id: String, name: String, point: SavedGeoPoint, source: SavedMapSource,
        note: String?, nowMillis: Int64, in scope: SavedMapScope
    ) throws -> SavedMapInventory {
        let stationId = id.lowercased()
        return try update(scope) { vault in
            if let index = vault.inventory.places.firstIndex(where: { $0.stationId == stationId }) {
                let old = vault.inventory.places.remove(at: index)
                vault.tombstones.insert(old.id)
            } else if let index = vault.inventory.legacyPlaces.firstIndex(where: { $0.stationId == id }) {
                let old = vault.inventory.legacyPlaces.remove(at: index)
                vault.tombstones.insert(Self.legacyExportID(old))
            } else {
                guard Self.placeCount(vault) < SavedMapBounds.places else { throw SavedMapError.placeQuota }
                guard SavedMapText.validID(stationId) else { throw SavedMapError.invalidRecord }
                let title = SavedMapText.limited(SavedMapText.cleaned(name), limit: SavedMapBounds.name)
                let caption = note.map { SavedMapText.limited(SavedMapText.cleaned($0), limit: SavedMapBounds.note) }
                let item = try SavedMapPlace(
                    id: UUID().uuidString.lowercased(), name: title, point: point, stationId: stationId,
                    source: source, sourceId: stationId, note: caption, createdAtMillis: nowMillis
                )
                vault.inventory.places.insert(item, at: 0)
            }
        }
    }

    public enum Selection: Equatable, Sendable {
        case everything
        case place(String)
        case trip(String)
        case legacyPlace(String)
        case legacyTrip(String)
    }

    public func delete(_ selection: Selection, in scope: SavedMapScope) throws -> SavedMapInventory {
        try update(scope) { vault in
            switch selection {
            case .place(let id):
                guard let index = vault.inventory.places.firstIndex(where: { $0.id == id }) else { return }
                vault.inventory.places.remove(at: index)
                vault.tombstones.insert(id)
            case .trip(let id):
                guard let index = vault.inventory.trips.firstIndex(where: { $0.id == id }) else { return }
                vault.inventory.trips.remove(at: index)
                vault.tombstones.insert(id)
            case .legacyPlace(let id):
                guard let index = vault.inventory.legacyPlaces.firstIndex(where: { $0.id == id }) else { return }
                let old = vault.inventory.legacyPlaces.remove(at: index)
                vault.tombstones.insert(Self.legacyExportID(old))
            case .legacyTrip(let id):
                guard let index = vault.inventory.legacyTrips.firstIndex(where: { $0.id == id }) else { return }
                vault.inventory.legacyTrips.remove(at: index)
                if SavedMapText.validID(id) { vault.tombstones.insert(id) }
            case .everything:
                return
            }
        }
    }

    public func importGPX(
        _ content: SavedGPXContent, restoringDeleted: Bool, in scope: SavedMapScope
    ) throws -> (inventory: SavedMapInventory, result: SavedMapImportResult) {
        var addedPlaces = 0
        var addedTrips = 0
        var alreadyImported = false
        let inventory = try update(scope) { vault in
            let incomingIDs = Set(content.places.map(\.id) + content.trips.map(\.id))
            let deleted = incomingIDs.intersection(vault.tombstones)
            if !deleted.isEmpty, !restoringDeleted { throw SavedMapError.restoreConfirmationRequired }
            if deleted.isEmpty, vault.importedFiles.contains(content.contentSha256) {
                alreadyImported = true
                return
            }
            var placeIDs = Set(vault.inventory.places.map(\.id))
            placeIDs.formUnion(vault.inventory.legacyPlaces.map(Self.legacyExportID))
            var stationIDs = Set(vault.inventory.places.compactMap(\.stationId))
            stationIDs.formUnion(vault.inventory.legacyPlaces.compactMap { $0.stationId?.lowercased() })
            var newPlaces: [SavedMapPlace] = []
            for place in content.places {
                guard !placeIDs.contains(place.id) else { continue }
                if let station = place.stationId, !stationIDs.insert(station).inserted { continue }
                placeIDs.insert(place.id)
                newPlaces.append(place)
            }
            let tripIDs = Set(vault.inventory.trips.map(\.id) + vault.inventory.legacyTrips.map(\.id))
            let newTrips = content.trips.filter { !tripIDs.contains($0.id) }
            guard Self.placeCount(vault) + newPlaces.count <= SavedMapBounds.places else { throw SavedMapError.placeQuota }
            guard vault.inventory.trips.count + vault.inventory.legacyTrips.count + newTrips.count <= SavedMapBounds.trips else {
                throw SavedMapError.tripQuota
            }
            vault.inventory.places.append(contentsOf: newPlaces)
            vault.inventory.trips.append(contentsOf: newTrips)
            vault.tombstones.subtract(newPlaces.map(\.id))
            vault.tombstones.subtract(newTrips.map(\.id))
            vault.importedFiles.insert(content.contentSha256)
            addedPlaces = newPlaces.count
            addedTrips = newTrips.count
        }
        return (inventory, SavedMapImportResult(
            importedPlaces: addedPlaces, importedTrips: addedTrips, alreadyImported: alreadyImported,
            skippedUnsupported: content.skippedUnsupported, truncatedFields: content.truncatedFields
        ))
    }

    public func exportGPX(
        _ selection: Selection, unnamedPlace: String, in scope: SavedMapScope
    ) throws -> Data {
        let inventory = try inventory(in: scope)
        switch selection {
        case .everything:
            let legacy = try inventory.legacyPlaces.map { try Self.exportableLegacyPlace($0, unnamed: unnamedPlace) }
            return try SavedGPX.write(places: inventory.places + legacy, trips: inventory.trips)
        case .place(let id):
            return try SavedGPX.write(places: inventory.places.filter { $0.id == id }, trips: [])
        case .trip(let id):
            return try SavedGPX.write(places: [], trips: inventory.trips.filter { $0.id == id })
        case .legacyPlace(let id):
            let legacy = try inventory.legacyPlaces.filter { $0.id == id }.map { try Self.exportableLegacyPlace($0, unnamed: unnamedPlace) }
            return try SavedGPX.write(places: legacy, trips: [])
        case .legacyTrip:
            throw SavedMapError.nothingToExport
        }
    }

    public func erase(_ scope: SavedMapScope) throws {
        lock.lock()
        defer { lock.unlock() }
        let file = url(scope)
        guard FileManager.default.fileExists(atPath: file.path) else { return }
        do {
            try FileManager.default.removeItem(at: file)
        } catch {
            throw SavedMapError.localStorage
        }
    }

    private func update(_ scope: SavedMapScope, change: (inout Vault) throws -> Void) throws -> SavedMapInventory {
        lock.lock()
        defer { lock.unlock() }
        var vault = try load(scope)
        let previous = vault
        try change(&vault)
        if vault != previous { try save(vault) }
        return vault.inventory
    }

    private func url(_ scope: SavedMapScope) -> URL {
        directory.appendingPathComponent(scope.fileKey).appendingPathExtension("json")
    }

    private func load(_ scope: SavedMapScope) throws -> Vault {
        let file = url(scope)
        guard FileManager.default.fileExists(atPath: file.path) else { return Vault(scope: scope) }
        do {
            let data = try Data(contentsOf: file)
            let vault = try JSONDecoder().decode(Vault.self, from: data)
            guard vault.scope == scope else { throw SavedMapError.localStorage }
            return vault
        } catch {
            throw SavedMapError.localStorage
        }
    }

    private func save(_ vault: Vault) throws {
        do {
            var folder = directory
            try FileManager.default.createDirectory(at: folder, withIntermediateDirectories: true)
            var backup = URLResourceValues()
            backup.isExcludedFromBackup = true
            try folder.setResourceValues(backup)
            let bytes = try JSONEncoder().encode(vault)
            #if os(iOS)
            try bytes.write(to: url(vault.scope), options: [.atomic, .completeFileProtection])
            #else
            try bytes.write(to: url(vault.scope), options: .atomic)
            #endif
        } catch {
            throw SavedMapError.localStorage
        }
    }

    private static func placeCount(_ vault: Vault) -> Int {
        vault.inventory.places.count + vault.inventory.legacyPlaces.count
    }

    private static func legacyExportID(_ place: SavedPlaceRecord) -> String {
        let old = place.id.lowercased()
        return SavedMapText.validID(old) ? old : SavedMapText.deterministicID("legacy-place:" + place.id)
    }

    private static func exportableLegacyPlace(_ place: SavedPlaceRecord, unnamed: String) throws -> SavedMapPlace {
        let safeName = SavedMapText.limited(SavedMapText.cleaned(place.title), limit: SavedMapBounds.name)
        let note = SavedMapText.limited(SavedMapText.cleaned(place.subtitle), limit: SavedMapBounds.note)
        let oldStation = place.stationId?.lowercased()
        let verifiedShape = oldStation.flatMap { SavedMapText.validID($0) ? $0 : nil }
        let source = place.stationId.map { SavedMapText.limited(SavedMapText.cleaned($0), limit: SavedMapBounds.sourceID) }
        return try SavedMapPlace(
            id: legacyExportID(place), name: safeName.isEmpty ? unnamed : safeName,
            point: SavedGeoPoint(latitude: place.latitude, longitude: place.longitude),
            stationId: verifiedShape, source: place.stationId == nil ? .manual : .anp,
            sourceId: source.flatMap { $0.isEmpty ? nil : $0 }, note: note.isEmpty ? nil : note, createdAtMillis: 0
        )
    }
}

import Foundation

public enum MapSearchPolicy {
    public static let directoryPageSize = 50

    public static func appleQuery(_ query: String, in municipality: Municipality) -> String? {
        let clean = StationDossierCodec.normalizedSearch(query)
        guard !clean.isEmpty else { return nil }
        // Only the manually selected public municipality is sent with the search text.
        return "\(clean), \(municipality.displayTitle), Brasil"
    }

    public static func pin(for station: StationRecord) -> EvCoordinate? {
        guard station.pinTier.allowsMapPin else { return nil }
        return coordinate(latitude: station.latitude, longitude: station.longitude)
    }

    public static func coordinate(latitude: Double, longitude: Double) -> EvCoordinate? {
        guard (-90...90).contains(latitude), (-180...180).contains(longitude) else { return nil }
        return EvCoordinate(latitude: latitude, longitude: longitude)
    }

    public static func chargingMatches(
        _ places: [ChargingPlace], query: String, municipality: Municipality, filter: ChargingFilter
    ) -> [ChargingPlace] {
        let clean = StationDossierCodec.normalizedSearch(query)
        guard !clean.isEmpty else { return [] }
        var activeFilters = filter
        activeFilters.query = clean
        return Array(places.lazy.filter {
            ChargingRules.isInCity($0, city: municipality) && ChargingRules.matches($0, filter: activeFilters)
        }.prefix(directoryPageSize))
    }
}

import Foundation

public struct SavedGPXContent: Sendable {
    public let places: [SavedMapPlace]
    public let trips: [SavedMapTrip]
    public let contentSha256: String
    public let skippedUnsupported: Int
    public let truncatedFields: Int
}

public enum SavedGPX {
    public static let namespace = "https://abastevo.com.br/ns/saved/1"
    public static let licenseNotice = "Contains public ANP open data, Abastevo community contributions and "
        + "OpenStreetMap-derived route plans. Check the original source terms before redistributing. "
        + "This file may reveal personal places and routes."

    public static func parse(
        _ data: Data, nowMillis: Int64, unnamedPlace: String, unnamedTrip: String
    ) throws -> SavedGPXContent {
        guard !data.isEmpty else { throw SavedMapError.fileEmpty }
        let root = try StrictSavedXML.parse(data)
        guard root.localName.lowercased() == "gpx" else { throw SavedMapError.unsupportedFormat }
        let hash = SavedMapText.fingerprint(data)
        var context = ImportContext(hash: hash, nowMillis: nowMillis, unnamedPlace: unnamedPlace, unnamedTrip: unnamedTrip)
        let waypointNodes = root.all("wpt")
        let trackNodes = root.all("trk")
        let routeNodes = root.all("rte")
        guard waypointNodes.count <= SavedMapBounds.places,
              trackNodes.count + routeNodes.count <= SavedMapBounds.trips else { throw SavedMapError.tooManyRecords }
        let places = try waypointNodes.map { try context.place($0) }
        let tracks = try trackNodes.compactMap { try context.trip($0, isRoute: false) }
        let routes = try routeNodes.compactMap { try context.trip($0, isRoute: true) }
        let trips = tracks + routes
        guard !places.isEmpty || !trips.isEmpty else { throw SavedMapError.noRecords }
        guard Set(places.map(\.id)).count == places.count, Set(trips.map(\.id)).count == trips.count else {
            throw SavedMapError.duplicateRecordID
        }
        let known: Set<String> = ["wpt", "trk", "rte", "metadata", "extensions"]
        let skipped = context.skipped + root.children.filter { !known.contains($0.localName) }.count
        return SavedGPXContent(places: places, trips: trips, contentSha256: hash,
                               skippedUnsupported: skipped, truncatedFields: context.truncated)
    }

    private struct ImportContext {
        let hash: String
        let nowMillis: Int64
        let unnamedPlace: String
        let unnamedTrip: String
        var recordOrdinal = 0
        var skipped = 0
        var truncated = 0

        mutating func place(_ node: StrictSavedXML.Node) throws -> SavedMapPlace {
            let ext = extensions(node)
            let stationId = ext["stationId"]?.lowercased()
            let station = ext["kind"] == "STATION" && (stationId.map(SavedMapText.validID) ?? false)
            let source = ext["sourceKind"].flatMap(SavedMapSource.init(rawValue:)) ?? .imported
            return try SavedMapPlace(
                id: id(ext), name: fit(node.text("name"), max: SavedMapBounds.name) ?? unnamedPlace,
                point: coordinate(node), stationId: station ? stationId : nil, source: source,
                sourceId: fit(ext["sourceId"], max: SavedMapBounds.sourceID),
                note: fit(node.text("desc") ?? node.text("cmt"), max: SavedMapBounds.note), createdAtMillis: nowMillis
            )
        }

        mutating func trip(_ node: StrictSavedXML.Node, isRoute: Bool) throws -> SavedMapTrip? {
            let nodes = isRoute ? node.all("rtept") : node.all("trkseg").flatMap { $0.all("trkpt") }
            if nodes.isEmpty {
                skipped += 1
                return nil
            }
            guard nodes.count <= SavedMapBounds.trackPoints else { throw SavedMapError.tooManyPoints }
            let ext = extensions(node)
            let points = try nodes.map { point in
                try SavedMapTripPoint(point: coordinate(point),
                                     elevationCm: isRoute ? nil : elevation(point.text("ele")),
                                     timeMillis: isRoute ? nil : instant(point.text("time")))
            }
            let plan = isRoute && ext["kind"] == SavedMapTrip.Kind.routePlan.rawValue
                && (2...SavedMapBounds.routePoints).contains(points.count) && Set(points.map(\.point)).count == points.count
            let kind: SavedMapTrip.Kind
            if plan {
                kind = .routePlan
            } else if ext["kind"] == SavedMapTrip.Kind.recordedTrack.rawValue {
                kind = .recordedTrack
            } else {
                kind = .importedTrack
            }
            let requestedAvoidance = Set((ext["avoid"] ?? "").split(separator: ",").map {
                $0.trimmingCharacters(in: .whitespacesAndNewlines)
            })
            return try SavedMapTrip(
                id: id(ext), kind: kind, name: fit(node.text("name"), max: SavedMapBounds.name) ?? unnamedTrip,
                points: points, avoid: plan ? requestedAvoidance.intersection(SavedMapTrip.avoidance) : [],
                sourceId: fit(ext["sourceId"], max: SavedMapBounds.sourceID),
                note: fit(node.text("desc"), max: SavedMapBounds.note), createdAtMillis: nowMillis
            )
        }

        private mutating func id(_ ext: [String: String]) -> String {
            recordOrdinal += 1
            if let given = ext["id"]?.lowercased(), SavedMapText.validID(given) { return given }
            return SavedMapText.deterministicID("\(hash):\(recordOrdinal)")
        }

        private mutating func fit(_ raw: String?, max: Int) -> String? {
            guard let raw else { return nil }
            let text = SavedMapText.cleaned(raw)
            guard !text.isEmpty else { return nil }
            if text.utf16.count <= max { return text }
            truncated += 1
            return SavedMapText.limited(text, limit: max)
        }

        private func coordinate(_ node: StrictSavedXML.Node) throws -> SavedGeoPoint {
            guard let lat = node.attributes["lat"], let lon = node.attributes["lon"] else {
                throw SavedMapError.coordinateMalformed
            }
            return try SavedGeoPoint.parse(latitude: lat, longitude: lon)
        }

        private func extensions(_ node: StrictSavedXML.Node) -> [String: String] {
            var result: [String: String] = [:]
            for child in node.first("extensions")?.children ?? [] where child.name.hasPrefix("abv:") {
                result[child.localName] = child.text.trimmingCharacters(in: .whitespacesAndNewlines)
            }
            return result
        }

        private func elevation(_ text: String?) -> Int? {
            guard let text, var raw = Decimal(string: text, locale: Locale(identifier: "en_US_POSIX")),
                  raw >= -10_001, raw <= 100_001 else { return nil }
            // Reject Decimal's permissive partial parses before rounding.
            guard text.range(of: #"^[+-]?(?:[0-9]+(?:\.[0-9]*)?|\.[0-9]+)(?:[eE][+-]?[0-9]+)?$"#,
                             options: .regularExpression) != nil else { return nil }
            var rounded = Decimal()
            NSDecimalRound(&rounded, &raw, 2, .plain)
            let cm = NSDecimalNumber(decimal: rounded * 100).intValue
            return (-1_000_000...10_000_000).contains(cm) ? cm : nil
        }

        private func instant(_ text: String?) -> Int64? {
            guard let text else { return nil }
            let formatter = ISO8601DateFormatter()
            formatter.formatOptions = [.withInternetDateTime, .withFractionalSeconds]
            var date = formatter.date(from: text)
            if date == nil {
                formatter.formatOptions = [.withInternetDateTime]
                date = formatter.date(from: text)
            }
            guard let millis = date.map({ ($0.timeIntervalSince1970 * 1_000).rounded() }),
                  millis >= 0, millis < Double(Int64.max) else { return nil }
            return Int64(millis)
        }
    }

    public static func write(places: [SavedMapPlace], trips: [SavedMapTrip]) throws -> Data {
        guard !places.isEmpty || !trips.isEmpty else { throw SavedMapError.nothingToExport }
        guard places.count <= SavedMapBounds.places, trips.count <= SavedMapBounds.trips else {
            throw SavedMapError.tooManyRecords
        }
        // Export has no byte cap, like Android `GeoExchange.writeGpx`: the record counts are bounded,
        // and "export everything" must not fail because two large imported tracks add up past 5 MiB.
        var document = Data()
        func append(_ text: String) throws {
            document.append(contentsOf: text.utf8)
        }

        let schema = "http://www.topografix.com/GPX/1/1"
        try append("<?xml version=\"1.0\" encoding=\"UTF-8\"?>\n")
        try append("<gpx version=\"1.1\" creator=\"Abastevo\" xmlns=\"\(schema)\" xmlns:abv=\"\(namespace)\">\n")
        try append("<metadata><desc>\(escape(licenseNotice))</desc></metadata>\n")
        for place in places.sorted(by: { $0.id < $1.id }) {
            try append(coordinates(place.point, tag: "wpt") + text("name", place.name) + text("desc", place.note))
            try append(extensions([
                ("id", place.id), ("kind", place.stationId != nil ? "STATION" : "PLACE"),
                ("stationId", place.stationId), ("sourceKind", place.source.rawValue), ("sourceId", place.sourceId),
            ]) + "</wpt>\n")
        }
        let formatter = ISO8601DateFormatter()
        formatter.formatOptions = [.withInternetDateTime, .withFractionalSeconds]
        for trip in trips.sorted(by: { $0.id < $1.id }) {
            let route = trip.kind == .routePlan
            let tag = route ? "rte" : "trk"
            try append("<\(tag)>" + text("name", trip.name) + text("desc", trip.note))
            try append(extensions([
                ("id", trip.id), ("kind", trip.kind.rawValue), ("sourceId", trip.sourceId),
                ("avoid", trip.avoid.isEmpty ? nil : trip.avoid.sorted().joined(separator: ",")),
            ]))
            if !route { try append("<trkseg>") }
            let pointTag = route ? "rtept" : "trkpt"
            for entry in trip.points {
                let height = entry.elevationCm.map { decimal(Int64($0), scale: 100, digits: 2) }
                let time = entry.timeMillis.map { formatter.string(from: Date(timeIntervalSince1970: Double($0) / 1_000)) }
                try append(coordinates(entry.point, tag: pointTag) + text("ele", height) + text("time", time) + "</\(pointTag)>")
            }
            if !route { try append("</trkseg>") }
            try append("</\(tag)>\n")
        }
        try append("</gpx>\n")
        return document
    }

    private static func coordinates(_ point: SavedGeoPoint, tag: String) -> String {
        "<\(tag) lat=\"\(decimal(point.latitudeMicro, scale: 1_000_000, digits: 6))\" "
            + "lon=\"\(decimal(point.longitudeMicro, scale: 1_000_000, digits: 6))\">"
    }

    private static func decimal(_ value: Int64, scale: Int64, digits: Int) -> String {
        let sign = value < 0 ? "-" : ""
        let magnitude = abs(value)
        let remainder = String(magnitude % scale)
        return sign + String(magnitude / scale) + "." + String(repeating: "0", count: digits - remainder.count) + remainder
    }

    private static func extensions(_ fields: [(String, String?)]) -> String {
        let content = fields.map { text("abv:" + $0.0, $0.1) }.joined()
        return content.isEmpty ? "" : "<extensions>\(content)</extensions>"
    }

    private static func text(_ label: String, _ value: String?) -> String {
        guard let value else { return "" }
        return "<\(label)>\(escape(value))</\(label)>"
    }

    private static func escape(_ value: String) -> String {
        value.replacingOccurrences(of: "&", with: "&amp;")
            .replacingOccurrences(of: "<", with: "&lt;")
            .replacingOccurrences(of: ">", with: "&gt;")
            .replacingOccurrences(of: "\"", with: "&quot;")
            .replacingOccurrences(of: "'", with: "&apos;")
    }
}

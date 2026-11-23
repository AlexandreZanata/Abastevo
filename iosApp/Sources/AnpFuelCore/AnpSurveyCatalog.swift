import Foundation

/// ANP weekly survey catalog read from the public gov.br listing page.
/// Swift port of Android `AnpEndpoints`, `AnpPriceTableUrlParser`,
/// `AnpListingWeekCatalogParser`, `AnpListingWeekHeaderParser`,
/// `AnpListingUpdatedAtParser`, `AnpListingOperationalNoteParser`,
/// `AnpBrazilianDateParser`, `AnpDateMapper`, `AnpStateMapper`,
/// `FuelProductNormalizationRule` and `MunicipalitySearchTextNormalizer`.
/// Zero postfix unwrap operators anywhere in this file.

/// Compiled regular expression that degrades to "no match" instead of trapping.
final class AnpPattern: @unchecked Sendable {
    let regex: NSRegularExpression?

    init(_ pattern: String, options: NSRegularExpression.Options = [.caseInsensitive]) {
        regex = try? NSRegularExpression(pattern: pattern, options: options)
    }

    func matches(in text: String) -> [NSTextCheckingResult] {
        guard let regex else { return [] }
        let length = (text as NSString).length
        return regex.matches(in: text, options: [], range: NSRange(location: 0, length: length))
    }

    /// Capture groups of the first match (index 0 is the whole match).
    func firstGroups(in text: String) -> [String?]? {
        guard let regex else { return nil }
        let ns = text as NSString
        guard let match = regex.firstMatch(in: text, options: [], range: NSRange(location: 0, length: ns.length)) else {
            return nil
        }
        return (0..<match.numberOfRanges).map { index -> String? in
            let range = match.range(at: index)
            return range.location == NSNotFound ? nil : ns.substring(with: range)
        }
    }

    func contains(_ text: String) -> Bool {
        firstGroups(in: text) != nil
    }

    func replacing(in text: String, with template: String) -> String {
        guard let regex else { return text }
        let length = (text as NSString).length
        return regex.stringByReplacingMatches(
            in: text, options: [], range: NSRange(location: 0, length: length), withTemplate: template
        )
    }
}

/// Calendar-day arithmetic on ISO `yyyy-MM-dd` strings in a fixed Gregorian UTC calendar.
public enum AnpCalendar {
    static let utc: Calendar = {
        var calendar = Calendar(identifier: .gregorian)
        calendar.timeZone = TimeZone(secondsFromGMT: 0) ?? TimeZone.current
        return calendar
    }()

    public static func day(year: Int, month: Int, day: Int) -> Date? {
        guard (1...9999).contains(year), (1...12).contains(month), (1...31).contains(day) else { return nil }
        var components = DateComponents()
        components.year = year
        components.month = month
        components.day = day
        guard let date = utc.date(from: components) else { return nil }
        let check = utc.dateComponents([.year, .month, .day], from: date)
        guard check.year == year, check.month == month, check.day == day else { return nil }
        return date
    }

    public static func parseIsoDay(_ text: String) -> Date? {
        let parts = text.split(separator: "-", omittingEmptySubsequences: false)
        guard parts.count == 3, parts[0].count == 4, parts[1].count == 2, parts[2].count == 2,
              parts.allSatisfy({ isAsciiDigits($0) }),
              let year = Int(parts[0]), let month = Int(parts[1]), let dayOfMonth = Int(parts[2]) else {
            return nil
        }
        return day(year: year, month: month, day: dayOfMonth)
    }

    public static func isoDay(_ date: Date) -> String {
        let components = utc.dateComponents([.year, .month, .day], from: date)
        return "\(pad(components.year ?? 0, 4))-\(pad(components.month ?? 0, 2))-\(pad(components.day ?? 0, 2))"
    }

    /// The device's local calendar date as ISO text (Android `LocalDate.now()`).
    public static func localTodayIso(now: Date = Date(), calendar: Calendar = Calendar.current) -> String {
        let components = calendar.dateComponents([.year, .month, .day], from: now)
        return "\(pad(components.year ?? 0, 4))-\(pad(components.month ?? 0, 2))-\(pad(components.day ?? 0, 2))"
    }

    public static func daysBetween(_ startIso: String, _ endIso: String) -> Int? {
        guard let start = parseIsoDay(startIso), let end = parseIsoDay(endIso) else { return nil }
        return utc.dateComponents([.day], from: start, to: end).day
    }

    /// Strict `d/M/yyyy` (one or two digit day and month, four digit year) to ISO.
    public static func parseBrazilianDay(_ text: String) -> String? {
        let parts = text.trimmingCharacters(in: .whitespacesAndNewlines)
            .split(separator: "/", omittingEmptySubsequences: false)
        guard parts.count == 3, (1...2).contains(parts[0].count), (1...2).contains(parts[1].count),
              parts[2].count == 4, parts.allSatisfy({ isAsciiDigits($0) }),
              let dayOfMonth = Int(parts[0]), let month = Int(parts[1]), let year = Int(parts[2]),
              let date = day(year: year, month: month, day: dayOfMonth) else {
            return nil
        }
        return isoDay(date)
    }

    /// Excel 1900-system serial day to ISO (Android `AnpDateMapper`: epoch 1899-12-30, fraction truncated).
    public static func excelSerialToIso(_ text: String?) -> String? {
        guard let raw = text?.trimmingCharacters(in: .whitespacesAndNewlines), !raw.isEmpty,
              let value = Double(raw), value.isFinite, value > -1_000_000, value < 3_000_000,
              let epoch = day(year: 1899, month: 12, day: 30),
              let date = utc.date(byAdding: .day, value: Int(value), to: epoch) else {
            return nil
        }
        return isoDay(date)
    }

    static func pad(_ value: Int, _ width: Int) -> String {
        let text = String(value)
        return text.count >= width ? text : String(repeating: "0", count: width - text.count) + text
    }

    static func isAsciiDigits<S: StringProtocol>(_ text: S) -> Bool {
        !text.isEmpty && text.utf8.allSatisfy { $0 >= 48 && $0 <= 57 }
    }
}

/// One ANP survey week: at most seven inclusive days (Android `SurveyWeek`).
public struct AnpSurveyWeekRef: Codable, Hashable, Comparable, Identifiable, Sendable {
    public let startIso: String
    public let endIso: String

    public var id: String { "\(startIso)_\(endIso)" }

    public var endYear: Int { Int(endIso.prefix(4)) ?? 0 }

    public init?(startIso: String, endIso: String) {
        guard AnpCalendar.parseIsoDay(startIso) != nil, AnpCalendar.parseIsoDay(endIso) != nil,
              let span = AnpCalendar.daysBetween(startIso, endIso), span >= 0, span <= 6 else {
            return nil
        }
        self.startIso = startIso
        self.endIso = endIso
    }

    public init?(id: String) {
        let parts = id.split(separator: "_", omittingEmptySubsequences: false)
        guard parts.count == 2 else { return nil }
        self.init(startIso: String(parts[0]), endIso: String(parts[1]))
    }

    private enum CodingKeys: String, CodingKey {
        case startIso
        case endIso
    }

    public init(from decoder: Decoder) throws {
        let values = try decoder.container(keyedBy: CodingKeys.self)
        let start = try values.decode(String.self, forKey: .startIso)
        let end = try values.decode(String.self, forKey: .endIso)
        guard let week = AnpSurveyWeekRef(startIso: start, endIso: end) else {
            throw DecodingError.dataCorruptedError(forKey: .endIso, in: values, debugDescription: "invalid survey week")
        }
        self = week
    }

    public static func < (lhs: AnpSurveyWeekRef, rhs: AnpSurveyWeekRef) -> Bool {
        if lhs.endIso != rhs.endIso { return lhs.endIso < rhs.endIso }
        return lhs.startIso < rhs.startIso
    }
}

/// SEC06-A approved ANP origins: HTTPS, exact gov.br host, the ANP price hierarchy only.
public enum AnpEndpoints {
    public static let listingPageUrl =
        "https://www.gov.br/anp/pt-br/assuntos/precos-e-defesa-da-concorrencia/precos/"
        + "levantamento-de-precos-de-combustiveis-ultimas-semanas-pesquisadas"

    public static let approvedHosts: Set<String> = ["www.gov.br"]
    public static let approvedPathPrefixes = ["/anp/pt-br/assuntos/precos-e-defesa-da-concorrencia/precos/"]

    public static func isApprovedUrl(_ raw: String) -> Bool {
        let trimmed = raw.trimmingCharacters(in: .whitespacesAndNewlines)
        guard let components = URLComponents(string: trimmed),
              components.scheme?.lowercased() == "https",
              components.user == nil, components.password == nil,
              let host = components.host?.lowercased(), approvedHosts.contains(host) else {
            return false
        }
        if let port = components.port, port != 443 { return false }
        let path = components.percentEncodedPath
        if path.split(separator: "/").contains(where: { $0 == ".." || $0 == "." }) { return false }
        return approvedPathPrefixes.contains { path.hasPrefix($0) }
    }
}

public enum AnpPriceTableKind: String, Codable, Sendable {
    case weeklySummary = "WEEKLY_SUMMARY"
    case stationDetail = "STATION_DETAIL"
}

/// Canonical and legacy gov.br file names (Android `AnpPriceTableUrlParser`).
public enum AnpPriceTableUrl {
    private static let canonical = AnpPattern(
        "arquivos-lpc/\\d{4}/(resumo[_-]semanal|revendas)[_-]lpc[_-](\\d{4}-\\d{2}-\\d{2})[-_](\\d{4}-\\d{2}-\\d{2})(?:-\\d+)?\\.xlsx"
    )
    private static let legacyHyphen = AnpPattern(
        "arquivos-lpc/\\d{4}/(resumo[_-]semanal|revendas)[_-]lpc[_-](\\d{4}-\\d{2}-\\d{2})[-_](\\d{4}-\\d{2}-\\d{2})\\.xlsx"
    )
    private static let dotDate = AnpPattern(
        "arquivos-lpc/\\d{4}/(resumo[_-]semanal|revendas)[_-]lpc[_-](\\d{4})\\.(\\d{2})\\.(\\d{2})[_-](\\d{4})\\.(\\d{2})\\.(\\d{2})\\.xlsx"
    )
    private static let compactDate = AnpPattern(
        "arquivos-lpc/\\d{4}/(resumo[_-]semanal|revendas)[_-]lpc[_-](\\d{2})(\\d{2})(\\d{4})-a-(\\d{2})(\\d{2})(\\d{4})\\.xlsx"
    )
    private static let lpcFile = AnpPattern("arquivos-lpc/\\d{4}/.*\\.xlsx")

    static var patternsCompile: Bool {
        [canonical, legacyHyphen, dotDate, compactDate, lpcFile].allSatisfy { $0.regex != nil }
    }

    public static func isPriceTableUrl(_ url: String) -> Bool {
        canonical.contains(url) || legacyHyphen.contains(url) || dotDate.contains(url)
            || compactDate.contains(url) || lpcFile.contains(url)
    }

    public static func parse(_ url: String) -> (kind: AnpPriceTableKind, week: AnpSurveyWeekRef)? {
        let trimmed = url.trimmingCharacters(in: .whitespacesAndNewlines)
        if let groups = canonical.firstGroups(in: trimmed) {
            return build(groups, start: groups[2], end: groups[3])
        }
        if let groups = legacyHyphen.firstGroups(in: trimmed) {
            return build(groups, start: groups[2], end: groups[3])
        }
        if let groups = dotDate.firstGroups(in: trimmed), groups.count == 8 {
            let start = join(groups[2], groups[3], groups[4])
            let end = join(groups[5], groups[6], groups[7])
            return build(groups, start: start, end: end)
        }
        if let groups = compactDate.firstGroups(in: trimmed), groups.count == 8 {
            let start = join(groups[4], groups[3], groups[2])
            let end = join(groups[7], groups[6], groups[5])
            return build(groups, start: start, end: end)
        }
        return nil
    }

    public static func inferKind(linkText: String) -> AnpPriceTableKind? {
        let normalized = linkText.lowercased()
        if normalized.contains("médios semanais") || normalized.contains("medios semanais") {
            return .weeklySummary
        }
        if normalized.contains("posto revendedor") { return .stationDetail }
        return nil
    }

    /// The final path segment, refused when blank, traversal-like or non-printable ASCII.
    public static func safeFileName(_ url: String) -> String? {
        let withoutQuery = url.trimmingCharacters(in: .whitespacesAndNewlines)
            .split(separator: "?", maxSplits: 1, omittingEmptySubsequences: false).first.map(String.init) ?? ""
        let path = withoutQuery.split(separator: "#", maxSplits: 1, omittingEmptySubsequences: false)
            .first.map(String.init) ?? ""
        guard let segment = path.split(separator: "/", omittingEmptySubsequences: false).last.map(String.init),
              !segment.trimmingCharacters(in: .whitespaces).isEmpty,
              !segment.contains("\\"), !segment.contains(".."),
              segment.unicodeScalars.allSatisfy({ $0.value >= 32 && $0.value <= 126 }) else {
            return nil
        }
        return segment
    }

    private static func build(_ groups: [String?], start: String?, end: String?) -> (kind: AnpPriceTableKind, week: AnpSurveyWeekRef)? {
        guard groups.count > 1, let token = groups[1], let start, let end,
              let kind = tableKind(token), let week = AnpSurveyWeekRef(startIso: start, endIso: end) else {
            return nil
        }
        return (kind, week)
    }

    private static func join(_ year: String?, _ month: String?, _ day: String?) -> String? {
        guard let year, let month, let day else { return nil }
        return "\(year)-\(month)-\(day)"
    }

    private static func tableKind(_ token: String) -> AnpPriceTableKind? {
        switch token.lowercased().replacingOccurrences(of: "-", with: "_") {
        case "resumo_semanal": return .weeklySummary
        case "revendas": return .stationDetail
        default: return nil
        }
    }
}

/// One listing week block with both downloads (Android `SurveyWeekCatalogEntry`).
public struct AnpSurveyCatalogEntry: Codable, Equatable, Identifiable, Sendable {
    public let week: AnpSurveyWeekRef
    public let summaryUrl: String
    public let stationUrl: String
    public let publishedOnIso: String?
    public let operationalNote: String?

    public var id: String { week.id }

    public init(week: AnpSurveyWeekRef, summaryUrl: String, stationUrl: String, publishedOnIso: String? = nil, operationalNote: String? = nil) {
        self.week = week
        self.summaryUrl = summaryUrl
        self.stationUrl = stationUrl
        self.publishedOnIso = publishedOnIso
        self.operationalNote = operationalNote
    }
}

/// Groups listing links under each "d/M/yyyy a d/M/yyyy" header until the next header.
public enum AnpListingParser {
    private static let weekHeader = AnpPattern("(\\d{1,2}/\\d{1,2}/\\d{4})\\s+a\\s+(\\d{1,2}/\\d{1,2}/\\d{4})")
    private static let anchor = AnpPattern(
        "<a\\b[^>]*?\\bhref\\s*=\\s*(?:\"([^\"]*)\"|'([^']*)')[^>]*>(.*?)</a\\s*>",
        options: [.caseInsensitive, .dotMatchesLineSeparators]
    )
    private static let tag = AnpPattern("<[^>]*>", options: [])
    private static let noteContainer = AnpPattern(
        "<(p|div|em|small|span)\\b[^>]*>(.*?)</\\1\\s*>",
        options: [.caseInsensitive, .dotMatchesLineSeparators]
    )
    private static let note = AnpPattern("^\\s*(NOTA|Aviso)\\s*:?\\s*(.+)$", options: [.caseInsensitive, .dotMatchesLineSeparators])
    private static let updatedAt = AnpPattern("\\(Atualizado em\\s+(\\d{1,2}/\\d{1,2}/\\d{4})\\)")

    static var patternsCompile: Bool {
        [weekHeader, anchor, tag, noteContainer, note, updatedAt].allSatisfy { $0.regex != nil }
    }

    public static func parseCatalog(html: String, baseUrl: String = AnpEndpoints.listingPageUrl) -> [AnpSurveyCatalogEntry] {
        let normalized = html
            .replacingOccurrences(of: "&nbsp;", with: " ")
            .replacingOccurrences(of: "\u{00A0}", with: " ")
        let ns = normalized as NSString
        let headers = weekHeader.matches(in: normalized)
        var entries: [AnpSurveyCatalogEntry] = []
        var seen = Set<String>()
        for (index, header) in headers.enumerated() {
            guard let week = parseWeekHeader(ns.substring(with: header.range)) else { continue }
            let start = header.range.location + header.range.length
            let end = index + 1 < headers.count ? headers[index + 1].range.location : ns.length
            guard end >= start else { continue }
            let section = ns.substring(with: NSRange(location: start, length: end - start))
            guard let entry = parseSection(week: week, section: section, baseUrl: baseUrl),
                  !seen.contains(entry.week.id) else { continue }
            seen.insert(entry.week.id)
            entries.append(entry)
        }
        return entries.sorted { $0.week > $1.week }
    }

    public static func parseWeekHeader(_ text: String) -> AnpSurveyWeekRef? {
        guard let groups = weekHeader.firstGroups(in: text.trimmingCharacters(in: .whitespacesAndNewlines)),
              groups.count == 3, let startText = groups[1], let endText = groups[2],
              let start = AnpCalendar.parseBrazilianDay(startText),
              let end = AnpCalendar.parseBrazilianDay(endText) else {
            return nil
        }
        return AnpSurveyWeekRef(startIso: start, endIso: end)
    }

    public static func parseUpdatedAt(_ text: String) -> String? {
        guard let groups = updatedAt.firstGroups(in: text), groups.count == 2, let date = groups[1] else { return nil }
        return AnpCalendar.parseBrazilianDay(date)
    }

    public static func parseOperationalNote(_ text: String) -> String? {
        let trimmed = text.trimmingCharacters(in: .whitespacesAndNewlines)
        guard !trimmed.isEmpty, let groups = note.firstGroups(in: trimmed), groups.count == 3,
              groups[0] == trimmed, let body = groups[2] else {
            return nil
        }
        let cleaned = body.trimmingCharacters(in: .whitespacesAndNewlines)
        return cleaned.isEmpty ? nil : cleaned
    }

    static func plainText(_ html: String) -> String {
        decodeEntities(tag.replacing(in: html, with: " "))
            .split(whereSeparator: { $0.isWhitespace })
            .joined(separator: " ")
    }

    static func decodeEntities(_ text: String) -> String {
        var result = ""
        var index = text.startIndex
        while let ampersand = text[index...].firstIndex(of: "&") {
            result.append(contentsOf: text[index..<ampersand])
            if let semicolon = text[ampersand...].firstIndex(of: ";"),
               text.distance(from: ampersand, to: semicolon) <= 10,
               let decoded = decodeEntity(String(text[ampersand...semicolon])) {
                result.append(contentsOf: decoded)
                index = text.index(after: semicolon)
                continue
            }
            result.append("&")
            index = text.index(after: ampersand)
        }
        result.append(contentsOf: text[index...])
        return result
    }

    private static let namedEntities: [String: String] = [
        "&amp;": "&", "&quot;": "\"", "&apos;": "'", "&lt;": "<", "&gt;": ">", "&nbsp;": " ",
        "&aacute;": "á", "&agrave;": "à", "&acirc;": "â", "&atilde;": "ã", "&eacute;": "é",
        "&ecirc;": "ê", "&iacute;": "í", "&oacute;": "ó", "&ocirc;": "ô", "&otilde;": "õ",
        "&uacute;": "ú", "&ccedil;": "ç", "&Aacute;": "Á", "&Eacute;": "É", "&Iacute;": "Í",
        "&Oacute;": "Ó", "&Uacute;": "Ú", "&Ccedil;": "Ç", "&Atilde;": "Ã", "&Otilde;": "Õ",
        "&ordm;": "º", "&ordf;": "ª",
    ]

    private static func decodeEntity(_ entity: String) -> String? {
        if let named = namedEntities[entity] { return named }
        guard entity.hasPrefix("&#"), entity.hasSuffix(";") else { return nil }
        let body = entity.dropFirst(2).dropLast()
        let scalarValue: UInt32?
        if body.hasPrefix("x") || body.hasPrefix("X") {
            scalarValue = UInt32(body.dropFirst(), radix: 16)
        } else {
            scalarValue = UInt32(body, radix: 10)
        }
        guard let value = scalarValue, let scalar = Unicode.Scalar(value) else { return nil }
        return String(Character(scalar))
    }

    static func absoluteUrl(_ href: String, baseUrl: String) -> String? {
        let cleaned = decodeEntities(href).trimmingCharacters(in: .whitespacesAndNewlines)
        guard !cleaned.isEmpty, let base = URL(string: baseUrl) else { return nil }
        let encoded = cleaned.replacingOccurrences(of: " ", with: "%20")
        guard let resolved = URL(string: encoded, relativeTo: base) else { return nil }
        return resolved.absoluteString
    }

    private static func parseSection(week: AnpSurveyWeekRef, section: String, baseUrl: String) -> AnpSurveyCatalogEntry? {
        let ns = section as NSString
        var summaryUrl: String?
        var stationUrl: String?
        var summaryPublished: String?
        var stationPublished: String?
        for match in anchor.matches(in: section) {
            guard match.numberOfRanges == 4 else { continue }
            let quoted = match.range(at: 1)
            let hrefRange = quoted.location != NSNotFound ? quoted : match.range(at: 2)
            guard hrefRange.location != NSNotFound,
                  let href = absoluteUrl(ns.substring(with: hrefRange), baseUrl: baseUrl),
                  AnpPriceTableUrl.isPriceTableUrl(href) else { continue }
            let textRange = match.range(at: 3)
            let linkText = textRange.location == NSNotFound ? "" : plainText(ns.substring(with: textRange))
            guard let kind = AnpPriceTableUrl.parse(href)?.kind ?? AnpPriceTableUrl.inferKind(linkText: linkText) else {
                continue
            }
            let published = parseUpdatedAt(contextText(ns, match: match, fallback: linkText))
            switch kind {
            case .weeklySummary:
                if summaryUrl == nil {
                    summaryUrl = href
                    summaryPublished = published
                }
            case .stationDetail:
                if stationUrl == nil {
                    stationUrl = href
                    stationPublished = published
                }
            }
        }
        guard let summary = summaryUrl, let station = stationUrl else { return nil }
        return AnpSurveyCatalogEntry(
            week: week,
            summaryUrl: summary,
            stationUrl: station,
            publishedOnIso: [summaryPublished, stationPublished].compactMap { $0 }.max(),
            operationalNote: operationalNote(section)
        )
    }

    /// The anchor and the text that follows it inside the same list item.
    private static func contextText(_ ns: NSString, match: NSTextCheckingResult, fallback: String) -> String {
        let anchorEnd = match.range.location + match.range.length
        guard anchorEnd <= ns.length else { return fallback }
        let tail = NSRange(location: anchorEnd, length: ns.length - anchorEnd)
        var stop = ns.length
        for marker in ["</li", "<li", "<a"] {
            let found = ns.range(of: marker, options: [.caseInsensitive], range: tail)
            if found.location != NSNotFound { stop = min(stop, found.location) }
        }
        guard stop > match.range.location else { return fallback }
        return plainText(ns.substring(with: NSRange(location: match.range.location, length: stop - match.range.location)))
    }

    private static func operationalNote(_ section: String) -> String? {
        let ns = section as NSString
        for match in noteContainer.matches(in: section) where match.numberOfRanges == 3 {
            let body = match.range(at: 2)
            guard body.location != NSNotFound else { continue }
            if let found = parseOperationalNote(plainText(ns.substring(with: body))) { return found }
        }
        return parseOperationalNote(plainText(section))
    }
}

/// Accent-insensitive labels shared by the ANP spreadsheets and the IBGE catalog.
public enum AnpTextNormalizer {
    private static let portuguese = Locale(identifier: "pt_BR")

    /// Android `MunicipalitySearchTextNormalizer.normalize`.
    public static func municipality(_ text: String) -> String {
        text.trimmingCharacters(in: .whitespacesAndNewlines)
            .folding(options: [.diacriticInsensitive], locale: portuguese)
            .replacingOccurrences(of: "'", with: "")
            .replacingOccurrences(of: "-", with: " ")
            .split(whereSeparator: { $0.isWhitespace })
            .joined(separator: " ")
            .uppercased()
    }

    /// BR-002 ANP product labels. Superset of Android `FuelProductNormalizationRule`
    /// that also accepts the label variants pinned in `contracts/testdata/anp`.
    public static func fuelProduct(anpLabel: String) -> FuelProduct? {
        let key = anpLabel
            .folding(options: [.diacriticInsensitive], locale: portuguese)
            .split(whereSeparator: { $0.isWhitespace })
            .joined(separator: " ")
            .uppercased()
        guard !key.isEmpty else { return nil }
        return productLabels[key]
    }

    private static let productLabels: [String: FuelProduct] = [
        "ETANOL HIDRATADO": .ethanol,
        "ETANOL": .ethanol,
        "GASOLINA COMUM": .gasolineRegular,
        "GASOLINA ADITIVADA": .gasolinePremium,
        "GASOLINA PREMIUM": .gasolinePremiumGrade,
        "GASOLINA PREMIUM GRADE": .gasolinePremiumGrade,
        "OLEO DIESEL": .dieselS500,
        "OLEO DIESEL S500": .dieselS500,
        "DIESEL S500": .dieselS500,
        "OLEO DIESEL S10": .dieselS10,
        "DIESEL S10": .dieselS10,
        "GNV": .cng,
        "GLP": .lpgP13,
        "GLP P13": .lpgP13,
    ]

    /// Android `AnpStateMapper`: UF abbreviation or Portuguese state name.
    public static func state(anpLabel: String) -> BrazilianState? {
        stateLabels[stateKey(anpLabel)]
    }

    private static let stateLabels: [String: BrazilianState] = {
        var labels: [String: BrazilianState] = [:]
        for state in BrazilianState.allCases {
            labels[AnpTextNormalizer.stateKey(state.rawValue)] = state
            labels[AnpTextNormalizer.stateKey(state.fullName)] = state
        }
        return labels
    }()

    private static func stateKey(_ text: String) -> String {
        text.trimmingCharacters(in: .whitespacesAndNewlines)
            .folding(options: [.diacriticInsensitive], locale: portuguese)
            .uppercased()
    }

    /// Android `AnpCnpjMapper.normalizeDigits`: numeric cells regain leading zeros.
    public static func cnpjDigits(_ raw: String) -> String {
        let trimmed = raw.trimmingCharacters(in: .whitespacesAndNewlines)
        var source = trimmed
        if trimmed.lowercased().contains("e"), let decimal = Decimal(string: trimmed, locale: Locale(identifier: "en_US_POSIX")) {
            source = "\(decimal)"
        }
        let digits = String(source.unicodeScalars.filter { $0.value >= 48 && $0.value <= 57 }.map(Character.init))
        if digits.count == 14 { return digits }
        if digits.count < 14 { return String(repeating: "0", count: 14 - digits.count) + digits }
        return String(digits.suffix(14))
    }
}

/// Android `PriceAmount.of(BigDecimal(cell).toDouble())`: half-up to centavos,
/// stored as integer milli-BRL (6.385 -> 6390).
public enum AnpPriceRounding {
    public static func milli(fromCell text: String?) -> Int64? {
        guard var raw = text?.trimmingCharacters(in: .whitespacesAndNewlines), !raw.isEmpty else { return nil }
        if !raw.contains("."), raw.contains(",") {
            raw = raw.replacingOccurrences(of: ",", with: ".")
        }
        guard let value = Double(raw), value.isFinite, value > 0, value < 1_000_000 else { return nil }
        let canonical = "\(value)"
        guard !canonical.lowercased().contains("e") else { return nil }
        let parts = canonical.split(separator: ".", omittingEmptySubsequences: false)
        guard parts.count <= 2, let whole = Int64(parts[0]) else { return nil }
        let fraction: [UInt8] = parts.count == 2 ? Array(parts[1].utf8) : []
        func digit(_ index: Int) -> Int64 {
            guard index < fraction.count else { return 0 }
            return Int64(fraction[index]) - 48
        }
        var centavos = whole * 100 + digit(0) * 10 + digit(1)
        if digit(2) >= 5 { centavos += 1 }
        guard centavos > 0 else { return nil }
        return centavos * 10
    }

    /// "R$ 6,39": ANP amounts are presented with two decimals, like Android `PriceFormatter`.
    public static func formatBrl(milli: Int64) -> String {
        let centavos = (milli + 5) / 10
        let reais = centavos / 100
        let cents = centavos % 100
        let centsText = cents < 10 ? "0\(cents)" : "\(cents)"
        return "R$ \(groupThousands(reais)),\(centsText)"
    }

    private static func groupThousands(_ value: Int64) -> String {
        let digits = String(value)
        var groups: [String] = []
        var end = digits.endIndex
        while end > digits.startIndex {
            let start = digits.index(end, offsetBy: -3, limitedBy: digits.startIndex) ?? digits.startIndex
            groups.insert(String(digits[start..<end]), at: 0)
            end = start
        }
        return groups.joined(separator: ".")
    }
}

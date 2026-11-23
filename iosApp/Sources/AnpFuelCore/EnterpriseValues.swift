import Foundation

/// Exact enterprise values shared with Android `EnterprisePartition`, `EnterpriseTime`,
/// `MeterInput` and the Go intake: identifiers, canonical origins and instants,
/// time declarations, meter readings and bounded free text.
/// Zero postfix unwrap operators anywhere in this file.
public enum EnterpriseValues {
    public static let nilId = "00000000-0000-0000-0000-000000000000"
    public static let maxOdometerMetres: Int64 = 1_000_000_000_000
    public static let maxHourMeterSeconds: Int64 = 3_600_000_000
    public static let maxCentavos: Int64 = 1_000_000_000_000
    public static let maxReasonScalars = 280

    // MARK: - Identifiers and origins

    /// Lowercase 8-4-4-4-12 hexadecimal, never the nil UUID.
    public static func isId(_ value: String) -> Bool {
        let bytes = Array(value.utf8)
        guard bytes.count == 36, value != nilId else { return false }
        for (index, byte) in bytes.enumerated() {
            if index == 8 || index == 13 || index == 18 || index == 23 {
                if byte != 0x2D { return false }
            } else if !((byte >= 0x30 && byte <= 0x39) || (byte >= 0x61 && byte <= 0x66)) {
                return false
            }
        }
        return true
    }

    public static func newId() -> String {
        return UUID().uuidString.lowercased()
    }

    /// `https://host[:port]` written exactly as canonical: lowercase host, no path, query, credentials or port 443.
    public static func isCanonicalOrigin(_ value: String) -> Bool {
        guard value.hasPrefix("https://"), let url = URL(string: value), url.scheme == "https",
              let host = url.host, !host.isEmpty, url.user == nil, url.password == nil,
              url.query == nil, url.fragment == nil, url.path.isEmpty else {
            return false
        }
        if host != host.lowercased() { return false }
        if let port = url.port {
            if port == 443 || port < 1 || port > 65_535 { return false }
            return value == "https://\(host):\(port)"
        }
        return value == "https://\(host)"
    }

    // MARK: - Instants

    /// `EnterpriseTime.formatInstant`: UTC, `Z`, a fraction without trailing zeros (milliseconds here).
    public static func formatInstant(epochMillis: Int64) -> String {
        let clamped = max(0, epochMillis)
        let seconds = clamped / 1000
        let millis = clamped % 1000
        let days = seconds / 86_400
        let secondOfDay = seconds % 86_400
        let civil = civilDate(days: days)
        var text = String(format: "%04lld-%02lld-%02lldT%02lld:%02lld:%02lld",
                          civil.year, civil.month, civil.day,
                          secondOfDay / 3600, (secondOfDay % 3600) / 60, secondOfDay % 60)
        if millis != 0 {
            var fraction = String(format: "%03lld", millis)
            while fraction.hasSuffix("0") {
                fraction.removeLast()
            }
            text += "." + fraction
        }
        return text + "Z"
    }

    /// A canonical UTC instant as the server writes it: `YYYY-MM-DDTHH:MM:SS[.f{1,9}]Z`, no trailing zero.
    public static func isCanonicalInstant(_ raw: String) -> Bool {
        return parseInstantMillis(raw) != nil
    }

    /// Milliseconds since the epoch for a canonical UTC instant; the sub-millisecond part is truncated.
    public static func parseInstantMillis(_ raw: String) -> Int64? {
        let bytes = Array(raw.utf8)
        guard bytes.count >= 20, bytes.count <= 30, bytes.last == UInt8(ascii: "Z") else { return nil }
        func number(_ range: Range<Int>) -> Int64? {
            var value: Int64 = 0
            for index in range {
                let byte = bytes[index]
                guard byte >= 0x30, byte <= 0x39 else { return nil }
                value = value * 10 + Int64(byte - 0x30)
            }
            return value
        }
        guard bytes[4] == 0x2D, bytes[7] == 0x2D, bytes[10] == UInt8(ascii: "T"), bytes[13] == 0x3A, bytes[16] == 0x3A,
              let year = number(0..<4), let month = number(5..<7), let day = number(8..<10),
              let hour = number(11..<13), let minute = number(14..<16), let second = number(17..<19),
              year >= 1, month >= 1, month <= 12, day >= 1, day <= daysInMonth(year: year, month: month),
              hour <= 23, minute <= 59, second <= 59 else {
            return nil
        }
        var millis: Int64 = 0
        if bytes.count > 20 {
            guard bytes[19] == 0x2E else { return nil }
            let digits = bytes.count - 21
            guard digits >= 1, digits <= 9, bytes[bytes.count - 2] != 0x30, let fraction = number(20..<(bytes.count - 1)) else {
                return nil
            }
            var scaled = fraction
            var power = digits
            while power > 3 {
                scaled /= 10
                power -= 1
            }
            while power < 3 {
                scaled *= 10
                power += 1
            }
            millis = scaled
        } else if bytes.count != 20 {
            return nil
        }
        let days = daysFromCivil(year: year, month: month, day: day)
        return ((days * 86_400) + hour * 3600 + minute * 60 + second) * 1000 + millis
    }

    static func daysInMonth(year: Int64, month: Int64) -> Int64 {
        switch month {
        case 1, 3, 5, 7, 8, 10, 12: return 31
        case 4, 6, 9, 11: return 30
        default:
            let leap = (year % 4 == 0 && year % 100 != 0) || year % 400 == 0
            return leap ? 29 : 28
        }
    }

    static func daysFromCivil(year: Int64, month: Int64, day: Int64) -> Int64 {
        let y = month <= 2 ? year - 1 : year
        let era = (y >= 0 ? y : y - 399) / 400
        let yoe = y - era * 400
        let shifted = month > 2 ? month - 3 : month + 9
        let doy = (153 * shifted + 2) / 5 + day - 1
        let doe = yoe * 365 + yoe / 4 - yoe / 100 + doy
        return era * 146_097 + doe - 719_468
    }

    static func civilDate(days: Int64) -> (year: Int64, month: Int64, day: Int64) {
        let z = days + 719_468
        let era = (z >= 0 ? z : z - 146_096) / 146_097
        let doe = z - era * 146_097
        let yoe = (doe - doe / 1460 + doe / 36_524 - doe / 146_096) / 365
        let doy = doe - (365 * yoe + yoe / 4 - yoe / 100)
        let mp = (5 * doy + 2) / 153
        let day = doy - (153 * mp + 2) / 5 + 1
        let month = mp < 10 ? mp + 3 : mp - 9
        let year = yoe + era * 400 + (month <= 2 ? 1 : 0)
        return (year, month, day)
    }

    // MARK: - Free text

    /// Nonblank, no C0/C1 control, at most `maxScalars` Unicode scalars (Kotlin `validText` with `required`).
    public static func isValidText(_ text: String, maxScalars: Int, required: Bool = true) -> Bool {
        var count = 0
        var says = false
        for scalar in text.unicodeScalars {
            let value = scalar.value
            if value < 0x20 || (value >= 0x7F && value <= 0x9F) { return false }
            if !scalar.properties.isWhitespace { says = true }
            count += 1
            if count > maxScalars { return false }
        }
        return says || (!required && text.isEmpty)
    }

    // MARK: - Meter input

    /// Decimal kilometres (comma or point, up to three decimals) as exact metres within the odometer bound.
    public static func parseKilometres(_ raw: String) -> Int64? {
        guard let metres = parseScaledDecimal(raw, fractionDigits: 3, unit: 1000) else { return nil }
        return metres <= maxOdometerMetres ? metres : nil
    }

    /// Decimal hours (comma or point, up to two decimals) as exact seconds within the hour meter bound.
    public static func parseHours(_ raw: String) -> Int64? {
        guard let hundredths = parseScaledDecimal(raw, fractionDigits: 2, unit: 100) else { return nil }
        let (seconds, overflow) = hundredths.multipliedReportingOverflow(by: 36)
        if overflow || seconds > maxHourMeterSeconds { return nil }
        return seconds
    }

    /// Exact fixed point: digits, one optional separator and at most `fractionDigits` decimals; no sign or grouping.
    static func parseScaledDecimal(_ raw: String, fractionDigits: Int, unit: Int64) -> Int64? {
        let text = raw.trimmingCharacters(in: .whitespaces)
        guard !text.isEmpty, text.count <= 20 else { return nil }
        let normalized = text.replacingOccurrences(of: ",", with: ".")
        let parts = normalized.split(separator: ".", omittingEmptySubsequences: false)
        guard parts.count <= 2, let whole = parts.first, !whole.isEmpty,
              whole.allSatisfy({ $0.isASCII && $0.isNumber }) else {
            return nil
        }
        var fraction = ""
        if parts.count == 2 {
            fraction = String(parts[1])
            guard !fraction.isEmpty, fraction.count <= fractionDigits,
                  fraction.allSatisfy({ $0.isASCII && $0.isNumber }) else {
                return nil
            }
        }
        while fraction.count < fractionDigits {
            fraction += "0"
        }
        guard let wholeValue = Int64(whole), let fractionValue = Int64(fraction.isEmpty ? "0" : fraction) else { return nil }
        let (scaled, overflow) = wholeValue.multipliedReportingOverflow(by: unit)
        if overflow { return nil }
        let (total, overflowSum) = scaled.addingReportingOverflow(fractionValue)
        return overflowSum ? nil : total
    }

    /// Display of exact metres as kilometres with up to three decimals (pt-BR comma).
    public static func kilometresLabel(metres: Int64) -> String {
        let whole = metres / 1000
        let rest = metres % 1000
        if rest == 0 { return "\(whole) km" }
        var fraction = String(format: "%03lld", rest)
        while fraction.hasSuffix("0") { fraction.removeLast() }
        return "\(whole),\(fraction) km"
    }

    /// Display of exact seconds as hours with up to two decimals (pt-BR comma).
    public static func hoursLabel(seconds: Int64) -> String {
        let hundredths = seconds / 36
        let whole = hundredths / 100
        let rest = hundredths % 100
        if rest == 0 { return "\(whole) h" }
        return String(format: "%lld,%02lld h", whole, rest)
    }
}

/// The OrgTime declaration every operation carries: instant, IANA entry zone and device offset.
public struct EnterpriseTimeDeclaration: Codable, Equatable, Sendable {
    public let occurredAt: String
    public let entryZone: String
    public let deviceOffsetSeconds: Int64

    public init(occurredAt: String, entryZone: String, deviceOffsetSeconds: Int64) {
        self.occurredAt = occurredAt
        self.entryZone = entryZone
        self.deviceOffsetSeconds = deviceOffsetSeconds
    }

    /// The device clock and zone at entry. A zone the server cannot resolve is declared as UTC.
    public static func at(epochMillis: Int64, zone: TimeZone) -> EnterpriseTimeDeclaration {
        let date = Date(timeIntervalSince1970: TimeInterval(epochMillis) / 1000)
        let identifier = isDeclarableZone(zone.identifier) ? zone.identifier : "UTC"
        let offset = Int64(max(-64_800, min(64_800, zone.secondsFromGMT(for: date))))
        return EnterpriseTimeDeclaration(
            occurredAt: EnterpriseValues.formatInstant(epochMillis: epochMillis),
            entryZone: identifier,
            deviceOffsetSeconds: identifier == "UTC" && zone.identifier != "UTC" ? 0 : offset
        )
    }

    /// `UTC` or a region zone of `[A-Za-z0-9_+/-]{1,128}` outside the posix, right and SystemV trees.
    public static func isDeclarableZone(_ zone: String) -> Bool {
        guard !zone.isEmpty, zone.utf8.count <= 128 else { return false }
        let allowed = zone.unicodeScalars.allSatisfy { scalar in
            let value = scalar.value
            return (value >= 0x41 && value <= 0x5A) || (value >= 0x61 && value <= 0x7A) || (value >= 0x30 && value <= 0x39) ||
                scalar == "_" || scalar == "+" || scalar == "/" || scalar == "-"
        }
        guard allowed else { return false }
        if zone == "UTC" { return true }
        return zone.contains("/") && !zone.contains("//") && !zone.hasPrefix("posix/") &&
            !zone.hasPrefix("right/") && !zone.hasPrefix("SystemV/")
    }

    var isValid: Bool {
        return EnterpriseValues.isCanonicalInstant(occurredAt) && Self.isDeclarableZone(entryZone) &&
            deviceOffsetSeconds >= -64_800 && deviceOffsetSeconds <= 64_800
    }
}

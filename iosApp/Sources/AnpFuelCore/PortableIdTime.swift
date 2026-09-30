/// Portable identity and time representations (P12-T04).
///
/// Swift port of the Kotlin `PortableIdTime`: UUIDs are lowercase `8-4-4-4-12`
/// hex strings and instants are UTC `YYYY-MM-DDTHH:MM:SSZ`. No `UUID` or
/// `ISO8601DateFormatter` anywhere in portable code; native adapters keep
/// Apple APIs behind ports.
///
/// NOT COMPILED: no Swift/Xcode toolchain on this Linux host. First real
/// build/test runs on macOS (Xcode 26.4); see iosApp/README.md.
public enum PortableIdTime {
    /// Lowercase canonical UUID only; uppercase is rejected, not coerced.
    public static func isUuid(_ text: String) -> Bool {
        let chars = Array(text)
        if chars.count != 36 { return false }
        for (i, c) in chars.enumerated() {
            if i == 8 || i == 13 || i == 18 || i == 23 {
                if c != "-" { return false }
            } else if !isLowerHex(c) {
                return false
            }
        }
        return true
    }

    /// Strict UTC instant `YYYY-MM-DDTHH:MM:SSZ` with calendar range checks.
    public static func isUtcInstant(_ text: String) -> Bool {
        let chars = Array(text)
        if chars.count != 20 { return false }
        if chars[4] != "-" || chars[7] != "-" || chars[10] != "T" ||
            chars[13] != ":" || chars[16] != ":" || chars[19] != "Z" {
            return false
        }
        for i in [0, 1, 2, 3, 5, 6, 8, 9, 11, 12, 14, 15, 17, 18] {
            if !isDigit(chars[i]) { return false }
        }
        let year = part(chars, 0, 4)
        let month = part(chars, 5, 7)
        let day = part(chars, 8, 10)
        let hour = part(chars, 11, 13)
        let minute = part(chars, 14, 16)
        let second = part(chars, 17, 19)
        if month < 1 || month > 12 { return false }
        if day < 1 || day > daysInMonth(year: year, month: month) { return false }
        if hour > 23 || minute > 59 || second > 59 { return false }
        return true
    }

    private static func isLowerHex(_ c: Character) -> Bool {
        return (c >= "0" && c <= "9") || (c >= "a" && c <= "f")
    }

    private static func isDigit(_ c: Character) -> Bool {
        return c >= "0" && c <= "9"
    }

    private static func part(_ chars: [Character], _ from: Int, _ to: Int) -> Int {
        var value = 0
        for i in from..<to {
            guard let ascii = chars[i].asciiValue, ascii >= 48 && ascii <= 57 else { return -1 }
            value = value * 10 + Int(ascii - 48)
        }
        return value
    }

    private static func daysInMonth(year: Int, month: Int) -> Int {
        switch month {
        case 1, 3, 5, 7, 8, 10, 12: return 31
        case 4, 6, 9, 11: return 30
        default: return isLeapYear(year) ? 29 : 28
        }
    }

    private static func isLeapYear(_ year: Int) -> Bool {
        return year % 4 == 0 && (year % 100 != 0 || year % 400 == 0)
    }
}

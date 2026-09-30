/// Portable exact money in integer milli-BRL (P12-T04, Swift port of P12-T02).
///
/// Pure Swift with no platform imports in logic paths, mirroring the Kotlin
/// `PortableMoney` and the backend Go kernel case-for-case (B-BR-002): no
/// floats, ANP comma decimals, nonzero precision beyond 3 decimals refused
/// (never rounded), range exactly 1...1_000_000 milli-BRL.
///
/// NOT COMPILED: no Swift/Xcode toolchain on this Linux host. First real
/// build/test runs on macOS (Xcode 26.4); see iosApp/README.md.
public struct PortableMoneyError: Error, Equatable {
    public let code: String
    public init(_ code: String) { self.code = code }
}

public enum PortableMoney {
    public static let minMilli: Int64 = 1
    public static let maxMilli: Int64 = 1_000_000

    /// Tank capacity bound in milli-liters (200 L, mirrors TankCapacity).
    public static let maxCapacityMilliLiters: Int64 = 200_000

    /// Parses ANP price text into integer milli-BRL. Throws with a stable
    /// quarantine-style code (`missing-price`, `negative-price`,
    /// `zero-price`, `over-precision`, `invalid-price`, `over-range`).
    public static func parse(_ text: String) throws -> Int64 {
        var trimmed = text.trimmingCharacters(in: .whitespaces)
        if trimmed.isEmpty { throw PortableMoneyError("missing-price") }
        var negative = false
        if trimmed.hasPrefix("-") {
            negative = true
            trimmed = String(trimmed.dropFirst()).trimmingCharacters(in: .whitespaces)
        }
        if trimmed.hasPrefix("+") { throw PortableMoneyError("invalid-price") }
        let intPart: String
        let fracPart: String
        if trimmed.contains(",") {
            let parts = trimmed.split(separator: ",", omittingEmptySubsequences: false)
            guard parts.count == 2 else { throw PortableMoneyError("invalid-price") }
            intPart = String(parts[0])
            fracPart = String(parts[1])
            if intPart.isEmpty || fracPart.isEmpty { throw PortableMoneyError("invalid-price") }
        } else {
            intPart = trimmed
            fracPart = ""
        }
        guard allDigits(intPart) && (fracPart.isEmpty || allDigits(fracPart)) else {
            throw PortableMoneyError("invalid-price")
        }
        var frac = fracPart
        if frac.count > 3 {
            for ch in frac.dropFirst(3) {
                if ch != "0" { throw PortableMoneyError("over-precision") }
            }
            frac = String(frac.prefix(3))
        }
        while frac.count < 3 { frac += "0" }
        var milli: Int64 = 0
        for ch in intPart {
            milli = milli * 10 + (try digit(ch))
            if milli > maxMilli { throw PortableMoneyError("over-range") }
        }
        var fracValue: Int64 = 0
        for ch in frac {
            fracValue = fracValue * 10 + (try digit(ch))
        }
        milli = milli * 1000 + fracValue
        if negative { throw PortableMoneyError("negative-price") }
        if milli < minMilli {
            if (intPart + frac).allSatisfy({ $0 == "0" }) {
                throw PortableMoneyError("zero-price")
            }
            throw PortableMoneyError("invalid-price")
        }
        if milli > maxMilli { throw PortableMoneyError("over-range") }
        return milli
    }

    /// Returns "ok" or the stable rejection code for `text`; never throws.
    public static func code(of text: String) -> String {
        do {
            _ = try parse(text)
            return "ok"
        } catch let error as PortableMoneyError {
            return error.code
        } catch {
            return "invalid-price"
        }
    }

    /// Formats milli-BRL canonically as "<int>,<3dp>" (5999 -> "5,999").
    public static func format(_ milli: Int64) throws -> String {
        if milli < minMilli {
            throw PortableMoneyError(milli == 0 ? "zero-price" : "invalid-price")
        }
        if milli > maxMilli { throw PortableMoneyError("over-range") }
        let intPart = milli / 1000
        var frac = String(milli % 1000)
        while frac.count < 3 { frac = "0" + frac }
        return "\(intPart),\(frac)"
    }

    /// Parses tank capacity text with the same comma grammar into
    /// milli-liters. Range is 1...200_000 (0 < liters <= 200).
    public static func parseCapacityMilliLiters(_ text: String) throws -> Int64 {
        let milliLiters = try parseCapacityRaw(text)
        if milliLiters < 1 {
            throw PortableMoneyError(milliLiters == 0 ? "zero-price" : "invalid-price")
        }
        if milliLiters > maxCapacityMilliLiters { throw PortableMoneyError("over-range") }
        return milliLiters
    }

    /// Exact tank-fill total in milli-BRL: round-half-up of
    /// `unitMilli * capacityMilliLiters / 1000`, with overflow refusal.
    public static func multiplyTankFill(unitMilli: Int64, capacityMilliLiters: Int64) throws -> Int64 {
        guard unitMilli >= minMilli && unitMilli <= maxMilli else {
            throw PortableMoneyError("over-range")
        }
        guard capacityMilliLiters >= 1 && capacityMilliLiters <= maxCapacityMilliLiters else {
            throw PortableMoneyError("over-range")
        }
        let (product, overflow) = unitMilli.multipliedReportingOverflow(by: capacityMilliLiters)
        if overflow { throw PortableMoneyError("over-range") }
        return (product + 500) / 1000
    }

    private static func parseCapacityRaw(_ text: String) throws -> Int64 {
        var trimmed = text.trimmingCharacters(in: .whitespaces)
        if trimmed.isEmpty { throw PortableMoneyError("missing-price") }
        if trimmed.hasPrefix("-") { throw PortableMoneyError("negative-price") }
        if trimmed.hasPrefix("+") { throw PortableMoneyError("invalid-price") }
        let intPart: String
        let fracPart: String
        if trimmed.contains(",") {
            let parts = trimmed.split(separator: ",", omittingEmptySubsequences: false)
            guard parts.count == 2 else { throw PortableMoneyError("invalid-price") }
            intPart = String(parts[0])
            fracPart = String(parts[1])
            if intPart.isEmpty || fracPart.isEmpty { throw PortableMoneyError("invalid-price") }
        } else {
            intPart = trimmed
            fracPart = ""
        }
        guard allDigits(intPart) && (fracPart.isEmpty || allDigits(fracPart)) else {
            throw PortableMoneyError("invalid-price")
        }
        var frac = fracPart
        if frac.count > 3 {
            for ch in frac.dropFirst(3) {
                if ch != "0" { throw PortableMoneyError("over-precision") }
            }
            frac = String(frac.prefix(3))
        }
        while frac.count < 3 { frac += "0" }
        var liters: Int64 = 0
        for ch in intPart {
            liters = liters * 10 + (try digit(ch))
            if liters > maxCapacityMilliLiters { throw PortableMoneyError("over-range") }
        }
        var fracValue: Int64 = 0
        for ch in frac {
            fracValue = fracValue * 10 + (try digit(ch))
        }
        return liters * 1000 + fracValue
    }

    private static func digit(_ c: Character) throws -> Int64 {
        guard let ascii = c.asciiValue, ascii >= 48 && ascii <= 57 else {
            throw PortableMoneyError("invalid-price")
        }
        return Int64(ascii - 48)
    }

    private static func allDigits(_ s: String) -> Bool {
        if s.isEmpty { return false }
        return s.allSatisfy({ $0 >= "0" && $0 <= "9" })
    }
}

/// Portable station/fuel feedback values (P14-T01, B-BR-F02…F06).
///
/// Swift port of the Kotlin `PortableFeedback` (itself mirroring the frozen
/// backend rules): integer 1–5 stars and floor basis-point agreement with
/// nil for zero votes. Text counting stays in `PortableText` (280 scalars,
/// shared vectors).
///
/// NOT COMPILED: no Swift/Xcode toolchain on this Linux host. First real
/// build/test runs on macOS (Xcode 26.4); see iosApp/README.md.
public enum PortableFeedback {
    public static let ratingMin = 1
    public static let ratingMax = 5

    public static let agreementScale: Int64 = 10000

    public static func isValidRating(_ stars: Int) -> Bool {
        return stars >= ratingMin && stars <= ratingMax
    }

    public static func agreementBasisPoints(valid: Int64, invalid: Int64) -> Int64? {
        // Mirrors Kotlin require{}: negative denominators are a
        // programmer error, never a quiet null (null means "no votes").
        precondition(valid >= 0 && invalid >= 0, "negative vote count")
        let total = valid + invalid
        guard total > 0 else { return nil }
        return agreementScale * valid / total
    }
}

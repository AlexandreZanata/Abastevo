/// Portable Unicode text rules for community comments (P12-T04, F03).
///
/// Swift port of the Kotlin `PortableText`: length is counted in Unicode
/// scalar values after trimming and CRLF normalization, via
/// `unicodeScalars` (an astral-plane emoji counts one, a combining sequence
/// counts base plus mark — identical to Go rune count and Kotlin scalars).
/// Never use `String.count` here: it counts grapheme clusters.
///
/// NOT COMPILED: no Swift/Xcode toolchain on this Linux host. First real
/// build/test runs on macOS (Xcode 26.4); see iosApp/README.md.
public enum PortableText {
    /// Comments and replies hold at most 280 Unicode scalar values (F03).
    public static let maxCommentScalars = 280

    /// Trims surrounding whitespace and normalizes CRLF/CR breaks to LF.
    public static func normalize(_ text: String) -> String {
        let trimmed = text.trimmingCharacters(in: .whitespacesAndNewlines)
        return trimmed
            .replacingOccurrences(of: "\r\n", with: "\n")
            .replacingOccurrences(of: "\r", with: "\n")
    }

    /// Counts Unicode scalar values.
    public static func countScalars(_ text: String) -> Int {
        return text.unicodeScalars.count
    }

    /// Nonempty after normalization and at most `maxCommentScalars` scalars.
    public static func isValidComment(_ text: String) -> Bool {
        let normalized = normalize(text)
        if normalized.isEmpty { return false }
        return countScalars(normalized) <= maxCommentScalars
    }
}

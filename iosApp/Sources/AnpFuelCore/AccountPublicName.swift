import Foundation

/// Client copy of the account API public-name rule (Android `com.anpfuel.domain.account.PublicName`).
///
/// The app refuses a name before it requests a sign-up code or saves a name.
/// The server stays authoritative; the API, Android and this port all run
/// `contracts/testdata/account/public-names-v1.tsv`. The rule works on Unicode
/// scalars, which match Go runes and Java code points.
public enum PublicName {
    public static let minLength = 2
    public static let maxLength = 40
    private static let minLetters = 2
    private static let maxDigits = 4

    private static let reservedWords: Set<String> = [
        "abastevo", "admin", "administrador", "moderador", "moderacao", "moderação", "suporte", "anp", "oficial",
    ]

    private static let allowedPunctuation: Set<Unicode.Scalar> = [" ", "'", "\u{2019}", "-", "."]

    /// NFC with surrounding whitespace removed and inner runs collapsed to one space.
    public static func normalize(_ name: String) -> String {
        var words: [String] = []
        var word = String.UnicodeScalarView()
        for scalar in name.unicodeScalars {
            if scalar.properties.isWhitespace {
                if !word.isEmpty {
                    words.append(String(word))
                }
                word = String.UnicodeScalarView()
            } else {
                word.append(scalar)
            }
        }
        if !word.isEmpty {
            words.append(String(word))
        }
        return words.joined(separator: " ").precomposedStringWithCanonicalMapping
    }

    public static func isValid(_ name: String) -> Bool {
        if name.unicodeScalars.contains(where: { isControlOrFormat($0) }) {
            return false
        }
        let normalized = normalize(name)
        let size = normalized.unicodeScalars.count
        if size < minLength || size > maxLength {
            return false
        }
        if containsReservedWord(normalized) {
            return false
        }
        var letters = 0
        var digits = 0
        for scalar in normalized.unicodeScalars {
            if isLetter(scalar) {
                letters += 1
            } else if scalar.properties.generalCategory == .decimalNumber {
                digits += 1
            } else if !isMark(scalar) && !allowedPunctuation.contains(scalar) {
                return false
            }
        }
        return letters >= minLetters && digits <= maxDigits
    }

    /// A word is a run of letters and marks; digits and punctuation separate words.
    private static func containsReservedWord(_ name: String) -> Bool {
        var word = String.UnicodeScalarView()
        for scalar in name.lowercased().unicodeScalars {
            if isLetter(scalar) || isMark(scalar) {
                word.append(scalar)
                continue
            }
            if reservedWords.contains(String(word)) {
                return true
            }
            word = String.UnicodeScalarView()
        }
        return reservedWords.contains(String(word))
    }

    private static func isControlOrFormat(_ scalar: Unicode.Scalar) -> Bool {
        switch scalar.properties.generalCategory {
        case .control, .format:
            return true
        default:
            return false
        }
    }

    private static func isLetter(_ scalar: Unicode.Scalar) -> Bool {
        switch scalar.properties.generalCategory {
        case .uppercaseLetter, .lowercaseLetter, .titlecaseLetter, .modifierLetter, .otherLetter:
            return true
        default:
            return false
        }
    }

    private static func isMark(_ scalar: Unicode.Scalar) -> Bool {
        switch scalar.properties.generalCategory {
        case .nonspacingMark, .spacingMark, .enclosingMark:
            return true
        default:
            return false
        }
    }
}

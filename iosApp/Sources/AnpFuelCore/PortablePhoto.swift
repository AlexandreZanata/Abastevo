/// Portable lightweight-photo values (P15-T01/T02, B-BR-M01…M03).
///
/// Swift port of the Kotlin `PortablePhoto` (itself mirroring the frozen
/// backend forward budgets): JPEG wire, 150 KiB target, 256 KiB hard cap,
/// 1600-pixel edge, 2 MP integer bound, at most 3 encoding attempts,
/// 32 MiB per-image working-memory hypothesis and one 24 h transient
/// deadline. The server always enforces; the client only plans
/// sample-decode, bounds attempts and expires its transient cache.
///
/// NOT COMPILED: no Swift/Xcode toolchain on this Linux host. First real
/// build/test runs on macOS (Xcode 26.4); see iosApp/README.md.
public enum PortablePhoto {
    public static let wireMime = "image/jpeg"

    public static let targetBytes: Int64 = 153600
    public static let capBytes: Int64 = 262144

    public static let maxEdgePixels = 1600
    public static let maxPixels: Int64 = 2000000

    public static let maxAttempts = 3

    public static let workingMemoryHypothesisBytes: Int64 = 33554432

    public static let transientTTLMillis: Int64 = 24 * 3600 * 1000

    public static let unsupportedFormat = "unsupported-format"
    public static let undecodableInput = "undecodable-input"
    public static let overBudget = "over-budget"
    public static let encodeFailed = "encode-failed"
    public static let ok = "ok"

    public static func isSupportedIntentMime(_ mime: String) -> Bool {
        switch mime.trimmingCharacters(in: .whitespaces).lowercased() {
        case "image/jpeg", "image/png", "image/heic", "image/heif":
            return true
        default:
            return false
        }
    }

    public static func sampleSizeForBounds(width: Int, height: Int) -> Int {
        precondition(width > 0 && height > 0, "non-positive frame")
        var size = 1
        while width / size > maxEdgePixels
            || height / size > maxEdgePixels
            || Int64(width / size) * Int64(height / size) > maxPixels {
            size *= 2
        }
        return size
    }

    public static func fitsWireCap(_ bytes: Int64) -> Bool {
        return bytes >= 1 && bytes <= capBytes
    }

    public static func isTransientExpired(capturedAtMillis: Int64, nowMillis: Int64) -> Bool {
        return nowMillis - capturedAtMillis >= transientTTLMillis
    }
}

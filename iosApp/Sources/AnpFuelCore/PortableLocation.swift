/// Portable location risk contract (P16-T01, B-BR-L01…L04).
///
/// Swift port of the Kotlin `PortableLocation` (itself mirroring the frozen
/// backend classifier): OS source signals (`LocationCompat.isMock` on
/// Android / `CLLocationSourceInformation.isSimulatedBySoftware` on iOS)
/// and fix metadata reduce to one verdict before any proximity claim
/// exists. Detects platform-marked simulation only; never promises
/// universal spoof-proofing, and a forged client flag cannot produce
/// source info the OS did not provide. Golden vectors:
/// `contracts/testdata/location/risk-v1.json` (replayed by Go).
///
/// NOT COMPILED: no Swift/Xcode toolchain on this Linux host. First real
/// build/test runs on macOS (Xcode 26.4); see iosApp/README.md.
public enum PortableLocation {
    public static let policyVersion = "location-v1"

    public static let verified = "VERIFIED"
    public static let degraded = "DEGRADED"
    public static let manual = "MANUAL"
    public static let denied = "DENIED"
    public static let simulated = "SIMULATED"
    public static let unknown = "UNKNOWN"

    public static let reasonSimulatedSource = "simulated-source"
    public static let reasonPermissionDenied = "permission-denied"
    public static let reasonNoFix = "no-fix"
    public static let reasonSourceMissing = "source-info-missing"
    public static let reasonStaleFix = "stale-fix"
    public static let reasonClockAnomaly = "clock-anomaly"
    public static let reasonCoarseAccuracy = "coarse-accuracy"
    public static let reasonManualEntry = "manual-entry"

    public static let maxFixAgeSeconds: Int64 = 120
    public static let maxClockSkewSeconds: Int64 = 300
    public static let maxClaimAccuracyM: Double = 100.0

    public static let freshnessFresh = "FRESH"
    public static let freshnessStale = "STALE"
    public static let freshnessUnknown = "UNKNOWN"
    public static let accuracyAccurate = "ACCURATE"
    public static let accuracyCoarse = "COARSE"
    public static let accuracyUnknown = "UNKNOWN"

    public struct FixInput {
        public let permissionGranted: Bool
        public let hasFix: Bool
        public let sourceInfoPresent: Bool
        public let simulated: Bool
        public let accuracyMeters: Double?
        public let fixAgeSeconds: Int64?
        public let clockSkewSeconds: Int64?
        public let manual: Bool

        public init(
            permissionGranted: Bool,
            hasFix: Bool,
            sourceInfoPresent: Bool,
            simulated: Bool,
            accuracyMeters: Double?,
            fixAgeSeconds: Int64?,
            clockSkewSeconds: Int64?,
            manual: Bool = false
        ) {
            self.permissionGranted = permissionGranted
            self.hasFix = hasFix
            self.sourceInfoPresent = sourceInfoPresent
            self.simulated = simulated
            self.accuracyMeters = accuracyMeters
            self.fixAgeSeconds = fixAgeSeconds
            self.clockSkewSeconds = clockSkewSeconds
            self.manual = manual
        }
    }

    public struct FixRisk {
        public let verdict: String
        public let reason: String
        public let freshness: String
        public let accuracy: String
        public let policyVersion: String

        public var allowsClaim: Bool { verdict == PortableLocation.verified }
    }

    public static func classify(_ input: FixInput) -> FixRisk {
        let freshness = freshnessBand(input.fixAgeSeconds)
        let accuracy = accuracyBand(input.accuracyMeters)
        var verdict = unknown
        var reason = reasonNoFix

        if input.manual {
            verdict = manual
            reason = reasonManualEntry
        } else if !input.permissionGranted {
            verdict = denied
            reason = reasonPermissionDenied
        } else if !input.hasFix || input.accuracyMeters == nil || input.fixAgeSeconds == nil {
            verdict = unknown
            reason = reasonNoFix
        } else if !input.sourceInfoPresent {
            verdict = unknown
            reason = reasonSourceMissing
        } else if input.simulated {
            verdict = simulated
            reason = reasonSimulatedSource
        } else if let age = input.fixAgeSeconds, let accuracyM = input.accuracyMeters {
            if age < 0 {
                verdict = unknown
                reason = reasonClockAnomaly
            } else if let skew = input.clockSkewSeconds, abs(skew) > maxClockSkewSeconds {
                verdict = unknown
                reason = reasonClockAnomaly
            } else if age > maxFixAgeSeconds {
                verdict = unknown
                reason = reasonStaleFix
            } else if accuracyM > maxClaimAccuracyM {
                verdict = degraded
                reason = reasonCoarseAccuracy
            } else {
                verdict = verified
                reason = ""
            }
        }
        return FixRisk(
            verdict: verdict,
            reason: reason,
            freshness: freshness,
            accuracy: accuracy,
            policyVersion: policyVersion
        )
    }

    private static func freshnessBand(_ ageSeconds: Int64?) -> String {
        guard let age = ageSeconds, age >= 0 else { return freshnessUnknown }
        return age > maxFixAgeSeconds ? freshnessStale : freshnessFresh
    }

    private static func accuracyBand(_ accuracyMeters: Double?) -> String {
        guard let accuracy = accuracyMeters else { return accuracyUnknown }
        return accuracy > maxClaimAccuracyM ? accuracyCoarse : accuracyAccurate
    }
}

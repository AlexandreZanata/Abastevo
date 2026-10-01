/// Portable location denial/degraded recovery (P16-T04, BUC-L03).
///
/// Swift port of the Kotlin `PortableLocationRecovery` (itself over the
/// frozen `PortableLocation` contract): one classified risk maps to a
/// stable disclosure + recovery vocabulary without touching providers,
/// clocks or I/O. Recovery never polls, never caches fixes and never
/// enables background tracking. Every blocked verdict preserves free
/// use (browsing, manual lookup, offline); only the location-dependent
/// proximity claim stays gated.
///
/// NOT COMPILED: no Swift/Xcode toolchain on this Linux host. First real
/// build/test runs on macOS (Xcode 26.4); see iosApp/README.md.
public enum PortableLocationRecovery {
    public static let none = "none"
    public static let openSettings = "open-settings"
    public static let disableSimulation = "disable-simulation"
    public static let enablePrecise = "enable-precise"
    public static let retryFix = "retry-fix"
    public static let browseOnly = "browse-only"

    public static let disclosureFixAccepted = "fix-accepted"

    public struct Recovery {
        public let verdict: String
        public let reason: String
        public let disclosureCode: String
        public let recoveryCode: String
        public let allowsClaim: Bool
        public let allowsBrowse: Bool
        public let allowsManual: Bool
        public let allowsOffline: Bool

        public init(
            verdict: String,
            reason: String,
            disclosureCode: String,
            recoveryCode: String,
            allowsClaim: Bool,
            allowsBrowse: Bool = true,
            allowsManual: Bool = true,
            allowsOffline: Bool = true
        ) {
            self.verdict = verdict
            self.reason = reason
            self.disclosureCode = disclosureCode
            self.recoveryCode = recoveryCode
            self.allowsClaim = allowsClaim
            self.allowsBrowse = allowsBrowse
            self.allowsManual = allowsManual
            self.allowsOffline = allowsOffline
        }

        public var preservesFreeUse: Bool {
            return allowsBrowse && allowsManual && allowsOffline
        }
    }

    public static func recover(_ risk: PortableLocation.FixRisk) -> Recovery {
        let disclosure: String
        if risk.allowsClaim {
            disclosure = disclosureFixAccepted
        } else {
            disclosure = risk.reason
        }
        let recovery: String
        switch risk.verdict {
        case PortableLocation.verified:
            recovery = none
        case PortableLocation.denied:
            recovery = openSettings
        case PortableLocation.simulated:
            recovery = disableSimulation
        case PortableLocation.degraded:
            recovery = enablePrecise
        case PortableLocation.manual:
            recovery = browseOnly
        default:
            recovery = retryFix
        }
        return Recovery(
            verdict: risk.verdict,
            reason: risk.reason,
            disclosureCode: disclosure,
            recoveryCode: recovery,
            allowsClaim: risk.allowsClaim
        )
    }
}

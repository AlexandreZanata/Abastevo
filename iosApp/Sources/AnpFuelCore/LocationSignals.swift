import CoreLocation

/// Native iOS location signals (P16-T02, L01…L03).
///
/// Reads the one-shot current fix and reports the OS simulation flag
/// (`CLLocationSourceInformation.isSimulatedBySoftware`, iOS 15+).
/// Missing source information (older iOS, unknown provider) folds to
/// unavailable: a client claim can never upgrade it. Never requests
/// location updates, never polls, never enables background tracking;
/// staleness is enforced by the frozen contract. No blacklists, no
/// developer-option checks.
///
/// Coordinates never leave this file as values: callers reduce the
/// fix to accuracy and age first, and only bands persist downstream.
///
/// NOT COMPILED: no Swift/Xcode toolchain on this Linux host. First real
/// build/test runs on macOS (Xcode 26.4); see iosApp/README.md.
public struct LocationSignalReading {
    public let permissionGranted: Bool
    public let sourceInfoAvailable: Bool
    public let simulated: Bool
    public let hasAccuracy: Bool
    public let accuracyMeters: Double
    public let fixAgeSeconds: Int64?
}

public enum LocationReading {
    /// Permission missing: adapters return the denied shape without
    /// touching providers.
    public static func denied() -> LocationSignalReading {
        return LocationSignalReading(
            permissionGranted: false,
            sourceInfoAvailable: false,
            simulated: false,
            hasAccuracy: false,
            accuracyMeters: 0,
            fixAgeSeconds: nil
        )
    }

    /// Permission granted but no fix available right now.
    public static func absent() -> LocationSignalReading {
        return LocationSignalReading(
            permissionGranted: true,
            sourceInfoAvailable: false,
            simulated: false,
            hasAccuracy: false,
            accuracyMeters: 0,
            fixAgeSeconds: nil
        )
    }

    /// Reduces one Core Location fix. A nil `isSimulated` (source
    /// information missing) stays unavailable even when the caller
    /// claims otherwise. Negative accuracy is invalid data, not a
    /// precise fix. A missing timestamp leaves age unknown for the
    /// contract to refuse.
    public static func fromFix(
        permissionGranted: Bool,
        isSimulatedBySoftware: Bool?,
        horizontalAccuracy: Double?,
        fixAgeSeconds: Int64?
    ) -> LocationSignalReading {
        guard permissionGranted else { return denied() }
        guard let isSimulated = isSimulatedBySoftware else {
            return LocationSignalReading(
                permissionGranted: true,
                sourceInfoAvailable: false,
                simulated: false,
                hasAccuracy: false,
                accuracyMeters: 0,
                fixAgeSeconds: fixAgeSeconds
            )
        }
        if let accuracy = horizontalAccuracy, accuracy >= 0 {
            return LocationSignalReading(
                permissionGranted: true,
                sourceInfoAvailable: true,
                simulated: isSimulated,
                hasAccuracy: true,
                accuracyMeters: accuracy,
                fixAgeSeconds: fixAgeSeconds
            )
        }
        return LocationSignalReading(
            permissionGranted: true,
            sourceInfoAvailable: true,
            simulated: isSimulated,
            hasAccuracy: false,
            accuracyMeters: 0,
            fixAgeSeconds: fixAgeSeconds
        )
    }
}

import CryptoKit
import Foundation

/// The local vehicle estimate uses exact hundredths of km/L. It is independent
/// of ledger entries, measured efficiency, comparisons, sync and entitlements.
public struct LedgerEstimatedEfficiency: Equatable, Sendable {
    public let hundredths: Int64

    /// Android's bounded BigDecimal contract, including effective decimal scale.
    public init?(_ input: String) {
        let value = input.trimmingCharacters(in: .whitespacesAndNewlines)
            .replacingOccurrences(of: ",", with: ".")
        guard !value.isEmpty, value.utf8.count <= 64 else { return nil }
        let components = value.split(omittingEmptySubsequences: false) { $0 == "e" || $0 == "E" }
        guard components.count == 1 || components.count == 2, let significand = components.first else { return nil }
        let exponent: Int
        if components.count == 2 {
            guard let parsed = Int(components[1]), (-64...64).contains(parsed) else { return nil }
            exponent = parsed
        } else {
            exponent = 0
        }
        let unsigned = significand.hasPrefix("+") ? significand.dropFirst() : significand[...]
        let parts = unsigned.split(separator: ".", maxSplits: 1, omittingEmptySubsequences: false)
        guard let whole = parts.first else { return nil }
        let fraction = parts.count == 2 ? parts[1] : ""
        let scale = fraction.count - exponent
        guard scale <= 2 else { return nil }
        let digits = String(whole) + String(fraction)
        guard !digits.isEmpty, digits.utf8.allSatisfy({ (48...57).contains($0) }) else { return nil }
        let nonzero = digits.drop(while: { $0 == "0" })
        let padding = 2 - scale
        guard !nonzero.isEmpty, nonzero.count + padding <= 6,
              let scaled = Int64(String(nonzero) + String(repeating: "0", count: padding)),
              (1...100_000).contains(scaled) else { return nil }
        hundredths = scaled
    }

    public var canonical: String { LedgerUnits.formatScaled(hundredths, fractionDigits: 2, separator: ".") }

    public func formatted(locale: Locale) -> String {
        LedgerUnits.formatScaled(hundredths, fractionDigits: 2, locale: locale)
    }

    public func fallback(for indicators: LedgerMonthlyIndicators) -> LedgerEstimatedEfficiency? {
        guard !indicators.measured.unknown.isEmpty, !indicators.windowNote.hasUnverifiedOdometer else { return nil }
        return self
    }
}

public struct LedgerEstimateScope: Codable, Hashable, Sendable {
    public let accountId: String?
    public let origin: String

    public init?(accountId: String?, origin: String) {
        guard !origin.isEmpty, origin.utf8.count <= 256, !origin.contains("\n"),
              accountId.map({ !$0.isEmpty && $0.utf8.count <= 128 && !$0.contains("\n") }) ?? true else { return nil }
        self.accountId = accountId
        self.origin = origin
    }

    private var identity: String {
        if let accountId { return "account:\(accountId)" }
        return "guest"
    }

    var filename: String {
        let digest = SHA256.hash(data: Data("\(identity)\n\(origin)".utf8))
        return digest.map { String(format: "%02x", $0) }.joined() + ".json"
    }
}

/// Private local settings, one file per account or guest and server origin.
public final class LedgerEfficiencyPreferenceStore: @unchecked Sendable {
    public enum Failure: Error, Equatable, Sendable {
        case invalidVehicle
        case unavailable
    }

    private struct Stored: Codable {
        let scope: LedgerEstimateScope
        var estimates: [String: String] = [:]
    }

    private let directory: URL
    private let lock = NSLock()

    public init(directory: URL? = nil) {
        self.directory = directory ?? IosLocalStore.defaultDirectory().appendingPathComponent("ledger-efficiency", isDirectory: true)
    }

    public func estimate(for vehicleId: String, in scope: LedgerEstimateScope) -> LedgerEstimatedEfficiency? {
        lock.lock()
        defer { lock.unlock() }
        guard let raw = (try? read(scope))?.estimates[vehicleId] else { return nil }
        return LedgerEstimatedEfficiency(raw)
    }

    public func set(_ estimate: LedgerEstimatedEfficiency?, for vehicleId: String, in scope: LedgerEstimateScope) throws {
        guard !vehicleId.isEmpty, vehicleId.utf8.count <= 128 else { throw Failure.invalidVehicle }
        lock.lock()
        defer { lock.unlock() }
        var stored = try read(scope)
        guard stored.estimates[vehicleId] != estimate?.canonical else { return }
        stored.estimates[vehicleId] = estimate?.canonical
        try write(stored)
    }

    public func removeVehicles(_ vehicleIds: Set<String>, in scope: LedgerEstimateScope) throws {
        lock.lock()
        defer { lock.unlock() }
        var stored = try read(scope)
        let retained = stored.estimates.filter { !vehicleIds.contains($0.key) }
        guard retained != stored.estimates else { return }
        stored.estimates = retained
        try write(stored)
    }

    public func erase(_ scope: LedgerEstimateScope) throws {
        lock.lock()
        defer { lock.unlock() }
        try remove(file(scope))
    }

    private func file(_ scope: LedgerEstimateScope) -> URL { directory.appendingPathComponent(scope.filename) }

    private func read(_ scope: LedgerEstimateScope) throws -> Stored {
        let url = file(scope)
        guard FileManager.default.fileExists(atPath: url.path) else { return Stored(scope: scope) }
        do {
            let stored = try JSONDecoder().decode(Stored.self, from: Data(contentsOf: url))
            guard stored.scope == scope else { throw Failure.unavailable }
            return stored
        } catch {
            throw Failure.unavailable
        }
    }

    private func write(_ stored: Stored) throws {
        let url = file(stored.scope)
        guard !stored.estimates.isEmpty else {
            try remove(url)
            return
        }
        do {
            var folder = directory
            try FileManager.default.createDirectory(at: folder, withIntermediateDirectories: true)
            var resources = URLResourceValues()
            resources.isExcludedFromBackup = true
            try folder.setResourceValues(resources)
            let bytes = try JSONEncoder().encode(stored)
            #if os(iOS)
            try bytes.write(to: url, options: [.atomic, .completeFileProtection])
            #else
            try bytes.write(to: url, options: .atomic)
            #endif
        } catch {
            throw Failure.unavailable
        }
    }

    private func remove(_ url: URL) throws {
        if FileManager.default.fileExists(atPath: url.path) {
            do {
                try FileManager.default.removeItem(at: url)
            } catch {
                throw Failure.unavailable
            }
        }
    }
}

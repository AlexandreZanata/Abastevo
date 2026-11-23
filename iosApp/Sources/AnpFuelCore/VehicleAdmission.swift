import Foundation

/// MEX-08 local vehicle admission (VEHICLE_REGISTRATION_POLICY v2, B-BR-IOS-VEHICLE).
///
/// Swift port of Android `domain/premium/VehicleAdmission.kt`. One new local
/// vehicle is free, including guest and offline use. A second or third needs
/// the `vehicles.multiple` capability through fresh server proof (at most
/// 900 seconds old, never from the future) or a valid offline lease bounded by
/// min(verifiedAt + 24 h, paid-through). Held vehicles are always readable,
/// correctable and removable; the technical ceiling stays three. Raw values
/// match `contracts/testdata/premium/vehicle-registration-v2.json`.
/// Zero postfix unwrap operators anywhere in this file.

public enum VehicleAdmissionScope: String, Codable, Sendable {
    case local = "LOCAL"
    case hosted = "HOSTED"
}

public enum VehicleAdmissionAuthority: String, Codable, Sendable {
    /// No proof or lease (`NONE` on the wire); named to avoid clashing with `Optional.none`.
    case absent = "NONE"
    case expired = "EXPIRED"
    case serverVerified = "SERVER_VERIFIED"
    case localValidLease = "LOCAL_VALID_LEASE"
    case clockRollback = "CLOCK_ROLLBACK"
    case proofStale = "PROOF_STALE"
    case unknownPolicy = "UNKNOWN_POLICY"
}

public enum VehicleAdmissionOutcome: String, Codable, Sendable {
    case accept = "ACCEPT"
    case premiumRequired = "PREMIUM_REQUIRED"
    case capacityExceeded = "CAPACITY_EXCEEDED"
    case hostedAuthorityRequired = "HOSTED_AUTHORITY_REQUIRED"
}

/// Owner- and origin-bound capability lease saved after a verified server status.
public struct VehicleCapabilityLease: Codable, Equatable, Sendable {
    public static let policyVersion = 2
    public static let capability = "vehicles.multiple"
    public static let offlineLeaseMillis: Int64 = 24 * 60 * 60 * 1000

    public let version: Int
    public let accountId: String
    public let origin: String
    public let capability: String
    public let verifiedAtMillis: Int64
    public let paidThroughMillis: Int64
    public let revoked: Bool

    public init(
        version: Int = VehicleCapabilityLease.policyVersion,
        accountId: String,
        origin: String,
        capability: String = VehicleCapabilityLease.capability,
        verifiedAtMillis: Int64,
        paidThroughMillis: Int64,
        revoked: Bool = false
    ) {
        self.version = version
        self.accountId = accountId
        self.origin = origin
        self.capability = capability
        self.verifiedAtMillis = verifiedAtMillis
        self.paidThroughMillis = paidThroughMillis
        self.revoked = revoked
    }
}

public enum VehicleAdmissionRule {
    public static let freeLimit = 1
    public static let technicalCeiling = 3
    public static let proofFreshnessMillis: Int64 = 900_000

    /// Fresh means observed at most 900 s ago and not in the future; anything else is stale.
    public static func resolveServerAuthority(verifiedAtMillis: Int64?, nowMillis: Int64) -> VehicleAdmissionAuthority {
        guard let verifiedAtMillis else { return .proofStale }
        let age = nowMillis - verifiedAtMillis
        return age >= 0 && age <= proofFreshnessMillis ? .serverVerified : .proofStale
    }

    /// Returns nil for negative counts (Android throws).
    public static func decide(
        scope: VehicleAdmissionScope,
        authority: VehicleAdmissionAuthority,
        registeredBefore: Int,
        newDistinctIds: Int
    ) -> VehicleAdmissionOutcome? {
        guard registeredBefore >= 0, newDistinctIds >= 0 else { return nil }
        if scope == .hosted && authority != .serverVerified { return .hostedAuthorityRequired }
        if newDistinctIds == 0 { return .accept }
        let after = registeredBefore + newDistinctIds
        if after <= freeLimit { return .accept }
        if after > technicalCeiling { return .capacityExceeded }
        return authority == .serverVerified || authority == .localValidLease ? .accept : .premiumRequired
    }

    /// Wrong owner or origin is ignored; revoked, rolled-back, elapsed and malformed leases never authorize.
    public static func resolveLocalAuthority(
        lease: VehicleCapabilityLease?,
        accountId: String,
        origin: String,
        nowMillis: Int64
    ) -> VehicleAdmissionAuthority {
        guard let lease else { return .absent }
        if lease.version != VehicleCapabilityLease.policyVersion || lease.capability != VehicleCapabilityLease.capability {
            return .unknownPolicy
        }
        if lease.accountId != accountId || lease.origin != origin { return .absent }
        if lease.revoked { return .expired }
        if nowMillis < lease.verifiedAtMillis { return .clockRollback }
        if lease.paidThroughMillis <= lease.verifiedAtMillis { return .unknownPolicy }
        let offlineEnd = lease.verifiedAtMillis.addingReportingOverflow(VehicleCapabilityLease.offlineLeaseMillis)
        let end = offlineEnd.overflow ? lease.paidThroughMillis : min(offlineEnd.partialValue, lease.paidThroughMillis)
        if nowMillis >= end { return .expired }
        return .localValidLease
    }
}

/// One server Premium status as observed by this device, bound to its owner and origin.
public struct PremiumCapabilityObservation: Codable, Equatable, Sendable {
    public let accountId: String
    public let origin: String
    public let observedAtMillis: Int64
    public let active: Bool
    public let capabilities: [String]
    public let validUntilMillis: Int64?

    public init(
        accountId: String,
        origin: String,
        observedAtMillis: Int64,
        active: Bool,
        capabilities: [String],
        validUntilMillis: Int64?
    ) {
        self.accountId = accountId
        self.origin = origin
        self.observedAtMillis = observedAtMillis
        self.active = active
        self.capabilities = capabilities
        self.validUntilMillis = validUntilMillis
    }

    public func grants(_ capability: String) -> Bool {
        return active && capabilities.contains(capability)
    }

    /// A lease for `vehicles.multiple`, or a revoked one when the server no longer grants it.
    public func vehicleLease() -> VehicleCapabilityLease? {
        if grants(VehicleCapabilityLease.capability), let until = validUntilMillis {
            return VehicleCapabilityLease(
                accountId: accountId,
                origin: origin,
                verifiedAtMillis: observedAtMillis,
                paidThroughMillis: until
            )
        }
        return VehicleCapabilityLease(
            accountId: accountId,
            origin: origin,
            verifiedAtMillis: observedAtMillis,
            paidThroughMillis: observedAtMillis,
            revoked: true
        )
    }
}

public enum VehicleAdmission {
    /// Admission for one new local vehicle: fresh server proof first, then the stored lease.
    public static func outcomeForNewVehicle(
        registered: Int,
        accountId: String?,
        origin: String,
        observation: PremiumCapabilityObservation?,
        lease: VehicleCapabilityLease?,
        nowMillis: Int64
    ) -> VehicleAdmissionOutcome {
        let resolved = Self.authority(accountId: accountId, origin: origin, observation: observation, lease: lease, nowMillis: nowMillis)
        return VehicleAdmissionRule.decide(scope: .local, authority: resolved, registeredBefore: registered, newDistinctIds: 1)
            ?? .premiumRequired
    }

    public static func authority(
        accountId: String?,
        origin: String,
        observation: PremiumCapabilityObservation?,
        lease: VehicleCapabilityLease?,
        nowMillis: Int64
    ) -> VehicleAdmissionAuthority {
        guard let accountId else { return .absent }
        if let observation, observation.accountId == accountId, observation.origin == origin,
           observation.grants(VehicleCapabilityLease.capability) {
            let server = VehicleAdmissionRule.resolveServerAuthority(
                verifiedAtMillis: observation.observedAtMillis,
                nowMillis: nowMillis
            )
            if server == .serverVerified { return server }
        }
        return VehicleAdmissionRule.resolveLocalAuthority(lease: lease, accountId: accountId, origin: origin, nowMillis: nowMillis)
    }
}

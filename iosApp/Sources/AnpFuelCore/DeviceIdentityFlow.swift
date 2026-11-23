import Foundation
#if canImport(Security)
import Security
#endif

/// Device contributor identity: registration (P10-T03) and account binding (P13-T04C).
///
/// A REGISTER challenge proves possession of the device key for
/// `POST /v1/contributors`; the proof lines travel in the body and cover the
/// bodyless base of that route. A signed-in account then binds the
/// contributor with `anpfuel-bind.v1` signed over SHA-256, so owner-signed
/// writes resolve to the account. Mirrors Android `AnonymousProofHttpClient`
/// and `PhotoProofTransport`. The stored record holds public identifiers only.
/// Zero postfix unwrap operators anywhere in this file.
public struct DeviceIdentityRecord: Codable, Equatable, Sendable {
    public let origin: String
    public let fingerprint: String
    public let contributorId: String
    public let keyId: String

    public init(origin: String, fingerprint: String, contributorId: String, keyId: String) {
        self.origin = origin
        self.fingerprint = fingerprint
        self.contributorId = contributorId
        self.keyId = keyId
    }
}

public protocol DeviceIdentityStoring: AnyObject, Sendable {
    func load() -> DeviceIdentityRecord?
    func save(_ record: DeviceIdentityRecord) throws
    func clear()
}

/// Keychain record of the registered contributor. Never synchronized to iCloud and readable after first unlock.
public final class IosDeviceIdentityKeychainStore: DeviceIdentityStoring, @unchecked Sendable {
    private let service: String
    private let account: String
    private let useMemoryOnly: Bool
    private let lock = NSLock()
    private var cached: DeviceIdentityRecord?

    public init(
        service: String = "com.anpfuel.app.deviceidentity",
        account: String = "contributor-v1",
        useMemoryOnly: Bool = false
    ) {
        self.service = service
        self.account = account
        self.useMemoryOnly = useMemoryOnly
    }

    public func load() -> DeviceIdentityRecord? {
        lock.lock()
        defer { lock.unlock() }
        if let cached {
            return cached
        }
        #if canImport(Security)
        if !useMemoryOnly {
            let query: [String: Any] = [
                kSecClass as String: kSecClassGenericPassword,
                kSecAttrService as String: service,
                kSecAttrAccount as String: account,
                kSecReturnData as String: true,
                kSecMatchLimit as String: kSecMatchLimitOne,
            ]
            var result: AnyObject?
            if SecItemCopyMatching(query as CFDictionary, &result) == errSecSuccess,
               let data = result as? Data,
               let record = try? JSONDecoder().decode(DeviceIdentityRecord.self, from: data) {
                cached = record
                return record
            }
        }
        #endif
        return nil
    }

    public func save(_ record: DeviceIdentityRecord) throws {
        let data = try JSONEncoder().encode(record)
        lock.lock()
        defer { lock.unlock() }
        cached = record
        #if canImport(Security)
        if !useMemoryOnly {
            SecItemDelete([
                kSecClass as String: kSecClassGenericPassword,
                kSecAttrService as String: service,
                kSecAttrAccount as String: account,
            ] as CFDictionary)
            let attributes: [String: Any] = [
                kSecClass as String: kSecClassGenericPassword,
                kSecAttrService as String: service,
                kSecAttrAccount as String: account,
                kSecValueData as String: data,
                kSecAttrAccessible as String: kSecAttrAccessibleAfterFirstUnlockThisDeviceOnly,
                kSecAttrSynchronizable as String: false,
            ]
            let status = SecItemAdd(attributes as CFDictionary, nil)
            if status != errSecSuccess {
                throw NSError(domain: "AnpFuelKeychain", code: Int(status))
            }
        }
        #endif
    }

    public func clear() {
        lock.lock()
        defer { lock.unlock() }
        cached = nil
        #if canImport(Security)
        if !useMemoryOnly {
            SecItemDelete([
                kSecClass as String: kSecClassGenericPassword,
                kSecAttrService as String: service,
                kSecAttrAccount as String: account,
            ] as CFDictionary)
        }
        #endif
    }
}

public struct IdentityChallenge: Equatable, Sendable {
    public let challengeId: String
    public let nonce: String
    public let fingerprint: String
    public let purpose: String

    public init(challengeId: String, nonce: String, fingerprint: String, purpose: String) {
        self.challengeId = challengeId
        self.nonce = nonce
        self.fingerprint = fingerprint
        self.purpose = purpose
    }
}

public struct ContributorRegistration: Equatable, Sendable {
    public let contributorId: String
    public let keyId: String
    public let existed: Bool

    public init(contributorId: String, keyId: String, existed: Bool) {
        self.contributorId = contributorId
        self.keyId = keyId
        self.existed = existed
    }
}

public struct ContributorBindingReceipt: Equatable, Sendable {
    public let contributorId: String
    public let keyFingerprint: String
    public let boundAt: String

    public init(contributorId: String, keyFingerprint: String, boundAt: String) {
        self.contributorId = contributorId
        self.keyFingerprint = keyFingerprint
        self.boundAt = boundAt
    }
}

/// Wire shapes of `/v1/identity/challenges`, `/v1/contributors` and `/v1/accounts/bindings/bind`.
public enum DeviceIdentityWire {
    public static let challengePath = "/v1/identity/challenges"
    public static let registrationPath = "/v1/contributors"
    public static let bindingPath = "/v1/accounts/bindings/bind"

    public static func challengeBody(fingerprint: String, purpose: String) -> Data? {
        guard PortableAnonymousProof.isFingerprintShape(fingerprint),
              let parsed = PortableAnonymousProof.parsePurpose(purpose) else {
            return nil
        }
        return encode(["fingerprint": fingerprint, "purpose": parsed])
    }

    /// Accepts only a reply that echoes the fingerprint and purpose and binds the nonce to the challenge id.
    public static func decodeChallenge(_ data: Data, fingerprint: String, purpose: String) -> IdentityChallenge? {
        guard let json = object(data),
              let challengeId = json["challenge_id"] as? String,
              !challengeId.isEmpty,
              let nonce = json["nonce"] as? String,
              let echoedFingerprint = json["fingerprint"] as? String,
              echoedFingerprint == fingerprint,
              let echoedPurpose = json["purpose"] as? String,
              echoedPurpose == purpose,
              let parts = PortableAnonymousProof.splitNonce(nonce),
              parts.0 == challengeId else {
            return nil
        }
        return IdentityChallenge(challengeId: challengeId, nonce: nonce, fingerprint: echoedFingerprint, purpose: echoedPurpose)
    }

    public static func registrationBody(
        jwkX: String,
        jwkY: String,
        challengeId: String,
        lines: [String],
        signature: String
    ) -> Data? {
        guard !jwkX.isEmpty, !jwkY.isEmpty, !challengeId.isEmpty, !signature.isEmpty,
              PortableAnonymousProof.isLinesShapeValid(lines) else {
            return nil
        }
        let jwk: [String: Any] = ["kty": "EC", "crv": "P-256", "x": jwkX, "y": jwkY]
        let proof: [String: Any] = ["lines": lines, "signature": signature]
        return encode([
            "public_jwk": jwk,
            "challenge_id": challengeId,
            "proof": proof,
        ])
    }

    public static func decodeRegistration(_ data: Data) -> ContributorRegistration? {
        guard let json = object(data),
              let contributorId = json["contributor_id"] as? String,
              UUID(uuidString: contributorId) != nil,
              let keyId = json["key_id"] as? String,
              UUID(uuidString: keyId) != nil else {
            return nil
        }
        let existed = json["existed"] as? Bool ?? false
        return ContributorRegistration(contributorId: contributorId, keyId: keyId, existed: existed)
    }

    public static func bindingBody(familyId: String, accessToken: String, contributorId: String, proof: String) -> Data? {
        guard !familyId.isEmpty, !accessToken.isEmpty, !contributorId.isEmpty, !proof.isEmpty else {
            return nil
        }
        return encode([
            "family_id": familyId,
            "access_token": accessToken,
            "contributor_id": contributorId,
            "proof": proof,
        ])
    }

    public static func decodeBinding(_ data: Data) -> ContributorBindingReceipt? {
        guard let json = object(data),
              let binding = json["binding"] as? [String: Any],
              let contributorId = binding["contributor_id"] as? String,
              !contributorId.isEmpty,
              let fingerprint = binding["key_fingerprint"] as? String,
              PortableAnonymousProof.isFingerprintShape(fingerprint) else {
            return nil
        }
        let boundAt = binding["bound_at"] as? String ?? ""
        return ContributorBindingReceipt(contributorId: contributorId, keyFingerprint: fingerprint, boundAt: boundAt)
    }

    static func encode(_ value: [String: Any]) -> Data? {
        guard JSONSerialization.isValidJSONObject(value) else { return nil }
        return try? JSONSerialization.data(withJSONObject: value, options: [.sortedKeys, .withoutEscapingSlashes])
    }

    static func object(_ data: Data) -> [String: Any]? {
        guard !data.isEmpty, data.count <= SignedRequestTransport.maxResponseBytes else { return nil }
        return (try? JSONSerialization.jsonObject(with: data)) as? [String: Any]
    }
}

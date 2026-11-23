import Foundation
#if canImport(CryptoKit)
import CryptoKit
#endif
#if canImport(Security)
import Security
#endif

/// Portable anonymous proof-of-possession values (P10-T03) and account binding (P13-T04C).
///
/// Mirrors `PortableAnonymousProof.kt` and `PortableAccountBinding.kt` case-for-case.
/// Zero postfix unwrap operators anywhere in this file.
public enum PortableAnonymousProof {
    public static let purposeRegister = "REGISTER"
    public static let purposeSign = "SIGN"

    public static let challengeTTLSeconds: Int64 = 300
    public static let maxBodyBytes = 65_536

    public static let lostKeyNotice =
        "Reinstalling or clearing app data deletes this device key. " +
        "The anonymous identity cannot be recovered without the old key."

    public static func buildBaseLines(
        method: String,
        authority: String,
        path: String,
        query: String,
        contentType: String?,
        contentDigest: String?,
        created: Int64,
        expires: Int64,
        keyId: String,
        nonce: String
    ) -> [String] {
        var lines: [String] = [
            "\"@method\": \(method)",
            "\"@authority\": \(authority)",
            "\"@path\": \(path)",
            "\"@query\": \(query)",
        ]
        if let contentType = contentType, let contentDigest = contentDigest {
            lines.append("\"content-type\": \(contentType)")
            lines.append("\"content-digest\": \(contentDigest)")
        }
        lines.append("\"created\": \(created)")
        lines.append("\"expires\": \(expires)")
        lines.append("\"keyid\": \"\(keyId)\"")
        lines.append("\"nonce\": \"\(nonce)\"")
        return lines
    }

    public static func buildBase(_ lines: [String]) -> String {
        return lines.joined(separator: "\n")
    }

    public static func isLinesShapeValid(_ lines: [String]) -> Bool {
        if lines.count != 8 && lines.count != 10 { return false }
        var want: [String] = [
            "\"@method\":",
            "\"@authority\":",
            "\"@path\":",
            "\"@query\":",
        ]
        if lines.count == 10 {
            want.append("\"content-type\":")
            want.append("\"content-digest\":")
        }
        want.append("\"created\":")
        want.append("\"expires\":")
        want.append("\"keyid\":")
        want.append("\"nonce\":")
        for idx in 0..<lines.count {
            if !lines[idx].hasPrefix(want[idx]) { return false }
        }
        return true
    }

    public static func parsePurpose(_ value: String) -> String? {
        switch value.trimmingCharacters(in: .whitespacesAndNewlines).uppercased() {
        case purposeRegister: return purposeRegister
        case purposeSign: return purposeSign
        default: return nil
        }
    }

    public static func isFingerprintShape(_ value: String) -> Bool {
        let fp = value.trimmingCharacters(in: .whitespacesAndNewlines)
        if !fp.hasPrefix("fp:") || fp.count != 67 { return false }
        for ch in fp.dropFirst(3) {
            let isDigit = ch >= "0" && ch <= "9"
            let isLowerHex = ch >= "a" && ch <= "f"
            if !isDigit && !isLowerHex { return false }
        }
        return true
    }

    public static func splitNonce(_ nonce: String) -> (String, String)? {
        guard let dot = nonce.firstIndex(of: ".") else { return nil }
        let prefix = nonce[..<dot]
        let suffixStart = nonce.index(after: dot)
        let suffix = nonce[suffixStart...]
        if prefix.isEmpty || suffix.isEmpty { return nil }
        for scalar in nonce.unicodeScalars {
            if scalar.value <= 0x20 || scalar.value > 0x7E { return nil }
        }
        return (String(prefix), String(suffix))
    }

    public static func isWindowValid(created: Int64, expires: Int64, nowEpochSeconds: Int64) -> Bool {
        if created < 0 || expires <= created { return false }
        if expires - created > challengeTTLSeconds { return false }
        if nowEpochSeconds >= expires { return false }
        if created > nowEpochSeconds + challengeTTLSeconds { return false }
        return true
    }

    public static func rotationIntentJson(
        newJwkX: String,
        newJwkY: String,
        oldChallengeId: String,
        newChallengeId: String
    ) -> String {
        return "{\"new_jwk\":{\"kty\":\"EC\",\"crv\":\"P-256\",\"x\":\"\(newJwkX)\"," +
            "\"y\":\"\(newJwkY)\"},\"old_challenge_id\":\"\(oldChallengeId)\"," +
            "\"new_challenge_id\":\"\(newChallengeId)\"}"
    }

    public static func thumbprintInput(jwkX: String, jwkY: String) -> String {
        return "{\"crv\":\"P-256\",\"kty\":\"EC\",\"x\":\"\(jwkX)\",\"y\":\"\(jwkY)\"}"
    }
}

public enum PortableAccountBinding {
    public static let proofTTLSeconds: Int64 = 300
    public static let proofLifetimeSeconds: Int64 = 120

    public static func statement(accountId: String, contributorId: String, expiry: Int64) -> String {
        return "anpfuel-bind.v1\n\(accountId)\n\(contributorId)\n\(expiry)"
    }

    public static func proof(fingerprint: String, expiry: Int64, signatureB64Url: String) -> String? {
        if !PortableAnonymousProof.isFingerprintShape(fingerprint) { return nil }
        if expiry <= 0 || signatureB64Url.isEmpty { return nil }
        for ch in signatureB64Url {
            let ok = (ch >= "A" && ch <= "Z") || (ch >= "a" && ch <= "z") || (ch >= "0" && ch <= "9") || ch == "-" || ch == "_"
            if !ok { return nil }
        }
        return "\(fingerprint).\(expiry).\(signatureB64Url)"
    }
}

public enum PortableBase64Url {
    public static func encode(_ data: Data) -> String {
        return data.base64EncodedString()
            .replacingOccurrences(of: "+", with: "-")
            .replacingOccurrences(of: "/", with: "_")
            .replacingOccurrences(of: "=", with: "")
    }

    public static func decode(_ text: String) -> Data? {
        var b64 = text
            .replacingOccurrences(of: "-", with: "+")
            .replacingOccurrences(of: "_", with: "/")
        let remainder = b64.count % 4
        if remainder > 0 {
            b64 += String(repeating: "=", count: 4 - remainder)
        }
        return Data(base64Encoded: b64)
    }
}

/// Native iOS ECDSA P-256 device key signer backed by CryptoKit and Keychain.
public final class IosDeviceProofSigner {
    public struct PublicJwk: Equatable {
        public let x: String
        public let y: String
        public let fingerprint: String

        public init(x: String, y: String, fingerprint: String) {
            self.x = x
            self.y = y
            self.fingerprint = fingerprint
        }
    }

    private let service: String
    private let accountKey: String
    private let lock = NSLock()
    private var cachedPrivateKeyData: Data?

    public init(
        service: String = "com.anpfuel.app.deviceproof",
        accountKey: String = "p256-signing-key-v1"
    ) {
        self.service = service
        self.accountKey = accountKey
    }

    public static func sha256Hex(_ data: Data) -> String {
        #if canImport(CryptoKit)
        let digest = SHA256.hash(data: data)
        return digest.map { String(format: "%02x", $0) }.joined()
        #else
        return ""
        #endif
    }

    public static func sha512ContentDigest(_ body: Data) -> String {
        #if canImport(CryptoKit)
        let digest = SHA512.hash(data: body)
        let b64 = Data(digest).base64EncodedString()
        return "\"sha-512=:\(b64):\""
        #else
        return "\"sha-512=::\""
        #endif
    }

    public func ensurePublicJwk() -> PublicJwk? {
        #if canImport(CryptoKit)
        guard let key = loadOrCreateKey() else { return nil }
        let raw = key.publicKey.rawRepresentation
        guard raw.count == 64 else { return nil }
        let x = PortableBase64Url.encode(raw.prefix(32))
        let y = PortableBase64Url.encode(raw.suffix(32))
        let thumbprintSource = Data(PortableAnonymousProof.thumbprintInput(jwkX: x, jwkY: y).utf8)
        let fingerprint = "fp:\(Self.sha256Hex(thumbprintSource))"
        return PublicJwk(x: x, y: y, fingerprint: fingerprint)
        #else
        return nil
        #endif
    }

    public func signProofLines(_ lines: [String]) -> String? {
        #if canImport(CryptoKit)
        guard PortableAnonymousProof.isLinesShapeValid(lines),
              let key = loadOrCreateKey() else {
            return nil
        }
        let baseData = Data(PortableAnonymousProof.buildBase(lines).utf8)
        let digest = SHA512.hash(data: baseData)
        guard let signature = try? key.signature(for: digest) else {
            return nil
        }
        return PortableBase64Url.encode(signature.rawRepresentation)
        #else
        return nil
        #endif
    }

    public func signBindingStatement(_ statement: String) -> String? {
        #if canImport(CryptoKit)
        guard let key = loadOrCreateKey() else { return nil }
        let digest = SHA256.hash(data: Data(statement.utf8))
        guard let signature = try? key.signature(for: digest) else { return nil }
        return PortableBase64Url.encode(signature.rawRepresentation)
        #else
        return nil
        #endif
    }

    public func resetKey() {
        lock.lock()
        defer { lock.unlock() }
        cachedPrivateKeyData = nil
        #if canImport(Security)
        let query: [String: Any] = [
            kSecClass as String: kSecClassGenericPassword,
            kSecAttrService as String: service,
            kSecAttrAccount as String: accountKey,
        ]
        SecItemDelete(query as CFDictionary)
        #endif
    }

    #if canImport(CryptoKit)
    private func loadOrCreateKey() -> P256.Signing.PrivateKey? {
        lock.lock()
        defer { lock.unlock() }
        if let raw = cachedPrivateKeyData,
           let key = try? P256.Signing.PrivateKey(rawRepresentation: raw) {
            return key
        }
        #if canImport(Security)
        let query: [String: Any] = [
            kSecClass as String: kSecClassGenericPassword,
            kSecAttrService as String: service,
            kSecAttrAccount as String: accountKey,
            kSecReturnData as String: true,
            kSecMatchLimit as String: kSecMatchLimitOne,
        ]
        var result: AnyObject?
        if SecItemCopyMatching(query as CFDictionary, &result) == errSecSuccess,
           let data = result as? Data,
           let key = try? P256.Signing.PrivateKey(rawRepresentation: data) {
            cachedPrivateKeyData = data
            return key
        }
        #endif
        let generated = P256.Signing.PrivateKey()
        let raw = generated.rawRepresentation
        cachedPrivateKeyData = raw
        #if canImport(Security)
        let attributes: [String: Any] = [
            kSecClass as String: kSecClassGenericPassword,
            kSecAttrService as String: service,
            kSecAttrAccount as String: accountKey,
            kSecValueData as String: raw,
            kSecAttrAccessible as String: kSecAttrAccessibleAfterFirstUnlockThisDeviceOnly,
            kSecAttrSynchronizable as String: false,
        ]
        SecItemDelete([
            kSecClass as String: kSecClassGenericPassword,
            kSecAttrService as String: service,
            kSecAttrAccount as String: accountKey,
        ] as CFDictionary)
        SecItemAdd(attributes as CFDictionary, nil)
        #endif
        return generated
    }
    #endif
}

/// Portable FREE-account auth values shared by Android and iPhone (P13-T05A).
///
/// Swift port of the Kotlin `PortableAuth`: provider allowlist, frozen OIDC
/// constants, code shape, session liveness, nonce binding and strict
/// callback parsing. The client only hints while the server enforces; a
/// mismatched callback refuses locally and its proof is never sent.
///
/// NOT COMPILED: no Swift/Xcode toolchain on this Linux host. First real
/// build/test runs on macOS (Xcode 26.4); see iosApp/README.md.
public enum PortableAuth {
    public static let providerGoogle = "google"
    public static let providerApple = "apple"
    public static let channelEmail = "email"

    public static let issuerGoogle = "https://accounts.google.com"
    public static let issuerApple = "https://appleid.apple.com"

    public static let audience = "anpfuel-backend"

    public static let codeDigits = 6
    public static let accessTTLSeconds: Int64 = 900
    public static let refreshAbsoluteSeconds: Int64 = 30 * 24 * 3600

    public static let callbackScheme = "anpfuel"
    public static let callbackHost = "auth"
    public static let callbackPath = "/callback"

    public struct Session {
        public let familyId: String
        public let accountId: String
        public let accessToken: String
        public let refreshToken: String
        public let accessExpiresAt: Int64
        public let absoluteExpiresAt: Int64

        public init(
            familyId: String, accountId: String,
            accessToken: String, refreshToken: String,
            accessExpiresAt: Int64, absoluteExpiresAt: Int64
        ) {
            self.familyId = familyId
            self.accountId = accountId
            self.accessToken = accessToken
            self.refreshToken = refreshToken
            self.accessExpiresAt = accessExpiresAt
            self.absoluteExpiresAt = absoluteExpiresAt
        }
    }

    public struct LoginAttempt {
        public let provider: String
        public let nonce: String
        public let state: String

        public init(provider: String, nonce: String, state: String) {
            self.provider = provider
            self.nonce = nonce
            self.state = state
        }
    }

    public struct ProviderCallback {
        public let provider: String
        public let idToken: String
        public let nonce: String
        public let state: String

        public init(provider: String, idToken: String, nonce: String, state: String) {
            self.provider = provider
            self.idToken = idToken
            self.nonce = nonce
            self.state = state
        }
    }

    public static func isProvider(_ value: String) -> Bool {
        return value == providerGoogle || value == providerApple
    }

    public static func issuerOf(_ provider: String) -> String {
        switch provider {
        case providerGoogle: return issuerGoogle
        case providerApple: return issuerApple
        default: return ""
        }
    }

    public static func isCodeShape(_ code: String) -> Bool {
        guard code.count == codeDigits else { return false }
        return code.allSatisfy { $0.isASCII && $0.isNumber }
    }

    public static func isAccessLive(_ session: Session, nowEpochSeconds: Int64) -> Bool {
        return nowEpochSeconds < session.accessExpiresAt &&
            nowEpochSeconds < session.absoluteExpiresAt
    }

    public static func isRefreshLive(_ session: Session, nowEpochSeconds: Int64) -> Bool {
        return nowEpochSeconds < session.absoluteExpiresAt
    }

    public static func bind(
        attempt: LoginAttempt,
        callback: ProviderCallback
    ) -> ProviderCallback? {
        guard isProvider(callback.provider) else { return nil }
        guard !callback.idToken.isEmpty, !callback.nonce.isEmpty, !callback.state.isEmpty else { return nil }
        guard callback.provider == attempt.provider else { return nil }
        guard callback.nonce == attempt.nonce else { return nil }
        guard callback.state == attempt.state else { return nil }
        return callback
    }

    public static func parseCallback(_ url: String) -> ProviderCallback? {
        let schemeParts = url.split(separator: ":", maxSplits: 1, omittingEmptySubsequences: false)
        guard schemeParts.count == 2, schemeParts[0] == callbackScheme else { return nil }
        var rest = String(schemeParts[1])
        guard rest.hasPrefix("//") else { return nil }
        rest = String(rest.dropFirst(2))
        guard let queryStart = rest.firstIndex(of: "?") else { return nil }
        let hostPath = String(rest[..<queryStart])
        guard hostPath == callbackHost + callbackPath else { return nil }
        let query = String(rest[rest.index(after: queryStart)...])
        var params: [String: String] = [:]
        for pair in query.split(separator: "&") {
            let kv = pair.split(separator: "=", maxSplits: 1, omittingEmptySubsequences: false)
            guard kv.count == 2, !kv[0].isEmpty, !kv[1].isEmpty else { return nil }
            let key = String(kv[0])
            guard params[key] == nil else { return nil }
            params[key] = String(kv[1])
        }
        guard let provider = params["provider"],
              let idToken = params["id_token"],
              let nonce = params["nonce"],
              let state = params["state"] else { return nil }
        return ProviderCallback(provider: provider, idToken: idToken, nonce: nonce, state: state)
    }

    public static func callbackUrl() -> String {
        return "\(callbackScheme)://\(callbackHost)\(callbackPath)"
    }
}

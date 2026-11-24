import Foundation
#if canImport(Security)
import Security
#endif

/// Native provider login (`POST /v1/accounts/providers/login`) and off-main auth execution.
///
/// Sign in with Apple receives the SHA-256 lowercase hex of a random raw
/// nonce in `ASAuthorizationAppleIDRequest.nonce`; the app then sends the raw
/// nonce with the identity token and the optional authorization code. The
/// reply carries the account, one session and whether the login created the
/// account. No name or e-mail scope is requested. Zero postfix unwrap
/// operators anywhere in this file.
public enum AccountProviderLogin {
    public static let path = "/v1/accounts/providers/login"
    public static let unavailableVerdict = "provider-unavailable"
    public static let maxNonceChars = 256

    public static func body(
        provider: String,
        idToken: String,
        rawNonce: String,
        authorizationCode: String?
    ) -> [String: Any] {
        var body: [String: Any] = [
            "provider": provider,
            "id_token": idToken,
            "nonce": rawNonce,
        ]
        if let code = authorizationCode, !code.isEmpty {
            body["authorization_code"] = code
        }
        return body
    }
}

public enum AppleSignInNonce {
    /// 32 random bytes as unpadded base64url, or nil when the system generator fails.
    public static func makeRawNonce(byteCount: Int = 32) -> String? {
        guard byteCount >= 16 && byteCount <= 64 else { return nil }
        #if canImport(Security)
        var bytes = [UInt8](repeating: 0, count: byteCount)
        let status = SecRandomCopyBytes(kSecRandomDefault, byteCount, &bytes)
        guard status == errSecSuccess else { return nil }
        return PortableBase64Url.encode(Data(bytes))
        #else
        return nil
        #endif
    }

    /// Value for `ASAuthorizationAppleIDRequest.nonce`.
    public static func requestNonce(forRaw rawNonce: String) -> String {
        return IosDeviceProofSigner.sha256Hex(Data(rawNonce.utf8))
    }
}

/// Runs a blocking auth call (the synchronous `AuthFlow` bridge) on a global queue, off the main actor.
public enum AuthBlockingWork {
    public static func run<T>(_ work: @escaping @Sendable () -> T) async -> T {
        return await withCheckedContinuation { (continuation: CheckedContinuation<T, Never>) in
            DispatchQueue.global(qos: .userInitiated).async {
                continuation.resume(returning: work())
            }
        }
    }
}

/// `SignedSessionSource` backed by `AuthFlow`: local rehydration for owner scopes, refresh off the main actor.
public final class AuthFlowSessionSource: SignedSessionSource, @unchecked Sendable {
    private let flow: AuthFlow

    public init(flow: AuthFlow) {
        self.flow = flow
    }

    public func currentAccountId() -> String? {
        switch flow.rehydrate() {
        case .active(let session), .needsRefresh(let session):
            return session.accountId
        case .pendingKeyAccount, .loggedOut:
            return nil
        }
    }

    public func liveSession() async -> PortableAuth.Session? {
        let flow = self.flow
        let result = await AuthBlockingWork.run { flow.refreshSession() }
        switch result {
        case .ok(let session):
            return session
        case .err:
            return nil
        }
    }
}

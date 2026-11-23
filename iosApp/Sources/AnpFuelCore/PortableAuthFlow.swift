import Foundation
#if canImport(Security)
import Security
#endif

/// Result type for portable account flows (P13-T05A).
/// Zero postfix unwrap operators anywhere in this file.
public enum AuthApiResult<T: Equatable>: Equatable {
    case ok(T)
    case err(String)
}

public struct EmptyAuthValue: Equatable {
    public init() {}
}

public struct AuthConsumeOk: Equatable {
    public let session: PortableAuth.Session
    public let created: Bool

    public init(session: PortableAuth.Session, created: Bool) {
        self.session = session
        self.created = created
    }
}

public struct AuthKeyIssued: Equatable {
    public let username: String
    public let accountKey: String

    public init(username: String, accountKey: String) {
        self.username = username
        self.accountKey = accountKey
    }
}

public struct AuthKeyBackup: Equatable, Codable, Sendable {
    public let username: String
    public let accountKey: String

    public init(username: String, accountKey: String) {
        self.username = username
        self.accountKey = accountKey
    }
}

public struct AuthKeyLogin: Equatable {
    public let session: PortableAuth.Session
    public let username: String

    public init(session: PortableAuth.Session, username: String) {
        self.session = session
        self.username = username
    }
}

public struct AuthLoginOutcome: Equatable {
    public let session: PortableAuth.Session
    public let created: Bool

    public init(session: PortableAuth.Session, created: Bool) {
        self.session = session
        self.created = created
    }
}

public enum AuthFlowState: Equatable {
    case loggedOut
    case pendingKeyAccount(AuthKeyBackup)
    case active(PortableAuth.Session)
    case needsRefresh(PortableAuth.Session)
}

public protocol AuthSessionStore {
    func save(_ session: PortableAuth.Session) throws
    func load() -> PortableAuth.Session?
    func clear() throws
}

public protocol AuthKeyStore {
    func saveKey(_ backup: AuthKeyBackup) throws
    func loadKey() -> AuthKeyBackup?
    func clearKey() throws
}

public protocol AuthWallClock {
    func nowEpochSeconds() -> Int64
}

public protocol PortableNonceSource {
    func nextNonce() -> String
}

public protocol AuthAccountApi {
    func requestCode(email: String) -> AuthApiResult<EmptyAuthValue>
    func consumeCode(email: String, code: String) -> AuthApiResult<AuthConsumeOk>
    func createKeyAccount(username: String) -> AuthApiResult<AuthKeyIssued>
    func loginWithKey(accountKey: String) -> AuthApiResult<AuthKeyLogin>
    func refresh(familyId: String, refreshToken: String) -> AuthApiResult<PortableAuth.Session>
    func revokeAll(familyId: String, accessToken: String) -> AuthApiResult<EmptyAuthValue>
    func linkProvider(
        session: PortableAuth.Session,
        provider: String,
        idToken: String,
        nonce: String
    ) -> AuthApiResult<EmptyAuthValue>
    func deleteAccount(session: PortableAuth.Session) -> AuthApiResult<EmptyAuthValue>
}

public protocol AuthOperationLock {
    func withLock<T>(_ operation: () -> T) -> T
}

public final class MutexAuthOperationLock: AuthOperationLock {
    private let lock = NSRecursiveLock()
    public init() {}
    public func withLock<T>(_ operation: () -> T) -> T {
        lock.lock()
        defer { lock.unlock() }
        return operation()
    }
}

public struct AuthPorts {
    public let store: AuthSessionStore
    public let keys: AuthKeyStore
    public let clock: AuthWallClock
    public let nonces: PortableNonceSource
    public let api: AuthAccountApi
    public let lock: AuthOperationLock

    public init(
        store: AuthSessionStore,
        keys: AuthKeyStore,
        clock: AuthWallClock,
        nonces: PortableNonceSource,
        api: AuthAccountApi,
        lock: AuthOperationLock = MutexAuthOperationLock()
    ) {
        self.store = store
        self.keys = keys
        self.clock = clock
        self.nonces = nonces
        self.api = api
        self.lock = lock
    }
}

/// Swift port of `AuthFlow` (`application/portable/PortableAuthFlow.kt`).
public final class AuthFlow {
    public static let clientInvalid = "client-invalid"
    public static let secureStorageUnavailable = "secure-storage-unavailable"
    public static let maxProofChars = 8192

    private let ports: AuthPorts
    private var spentNonces: Set<String> = []

    public init(ports: AuthPorts) {
        self.ports = ports
    }

    public func requestEmailCode(email: String) -> AuthApiResult<EmptyAuthValue> {
        if !isEmailHint(email) { return .err(Self.clientInvalid) }
        return ports.api.requestCode(email: email.trimmingCharacters(in: .whitespacesAndNewlines))
    }

    public func consumeEmailCode(email: String, code: String) -> AuthApiResult<AuthLoginOutcome> {
        return ports.lock.withLock {
            if ports.keys.loadKey() != nil {
                return .err(Self.clientInvalid)
            }
            if !isEmailHint(email) || !PortableAuth.isCodeShape(code) {
                return .err(Self.clientInvalid)
            }
            switch ports.api.consumeCode(
                email: email.trimmingCharacters(in: .whitespacesAndNewlines),
                code: code
            ) {
            case .err(let verdict):
                return .err(verdict)
            case .ok(let res):
                if persistSession(res.session) {
                    return .ok(AuthLoginOutcome(session: res.session, created: res.created))
                }
                return .err(Self.secureStorageUnavailable)
            }
        }
    }

    public func createKeyAccount(username: String) -> AuthApiResult<AuthKeyIssued> {
        return ports.lock.withLock {
            let name = username.trimmingCharacters(in: .whitespacesAndNewlines).lowercased()
            if !isUsernameHint(name) {
                return .err(Self.clientInvalid)
            }
            switch ports.api.createKeyAccount(username: name) {
            case .err(let verdict):
                return .err(verdict)
            case .ok(let issued):
                try? ports.keys.saveKey(AuthKeyBackup(username: issued.username, accountKey: issued.accountKey))
                return .ok(issued)
            }
        }
    }

    public func loginWithKey(rawKey: String) -> AuthApiResult<AuthLoginOutcome> {
        return ports.lock.withLock {
            let trimmedKey = rawKey.trimmingCharacters(in: .whitespacesAndNewlines)
            if trimmedKey.isEmpty {
                return .err(Self.clientInvalid)
            }
            switch ports.api.loginWithKey(accountKey: trimmedKey) {
            case .err(let verdict):
                return .err(verdict)
            case .ok(let res):
                let backup = AuthKeyBackup(username: res.username, accountKey: trimmedKey)
                let previousBackup = ports.keys.loadKey()
                let previousSession = ports.store.load()
                var savedKey = false
                do {
                    try ports.keys.saveKey(backup)
                    savedKey = ports.keys.loadKey() == backup
                } catch {
                    savedKey = false
                }
                if !savedKey || !persistSession(res.session) {
                    if let prevBackup = previousBackup {
                        try? ports.keys.saveKey(prevBackup)
                    }
                    if let prevSession = previousSession {
                        try? ports.store.save(prevSession)
                    }
                    return .err(Self.secureStorageUnavailable)
                }
                return .ok(AuthLoginOutcome(session: res.session, created: false))
            }
        }
    }

    public func currentKey() -> AuthKeyBackup? {
        return ports.keys.loadKey()
    }

    public func currentSession() -> PortableAuth.Session? {
        guard let session = ports.store.load() else { return nil }
        if !PortableAuth.isAccessLive(session, nowEpochSeconds: ports.clock.nowEpochSeconds()) {
            return nil
        }
        return session
    }

    public func refreshSession() -> AuthApiResult<PortableAuth.Session?> {
        return ports.lock.withLock {
            guard let session = ports.store.load() else { return .ok(nil) }
            let now = ports.clock.nowEpochSeconds()
            if PortableAuth.isAccessLive(session, nowEpochSeconds: now) && session.accessExpiresAt - now > 60 {
                return .ok(session)
            }
            let backup = ports.keys.loadKey()
            if !PortableAuth.isRefreshLive(session, nowEpochSeconds: now) && backup == nil {
                return clearLocalAuth() ? .ok(nil) : .err(Self.secureStorageUnavailable)
            }
            switch ports.api.refresh(familyId: session.familyId, refreshToken: session.refreshToken) {
            case .err(let verdict):
                if verdict == PortableAuth.Verdict.sessionExpired, let keyBackup = backup {
                    switch ports.api.loginWithKey(accountKey: keyBackup.accountKey) {
                    case .err(let loginVerdict):
                        return isAuthDenial(loginVerdict) ? clearAndDeny(loginVerdict) : .err(loginVerdict)
                    case .ok(let loginRes):
                        if loginRes.session.accountId != session.accountId {
                            return clearAndDeny(Self.clientInvalid)
                        }
                        if persistSession(loginRes.session) {
                            return .ok(loginRes.session)
                        }
                        return .err(Self.secureStorageUnavailable)
                    }
                }
                return isAuthDenial(verdict) ? clearAndDeny(verdict) : .err(verdict)
            case .ok(let refreshed):
                if refreshed.accountId != session.accountId {
                    return clearAndDeny(Self.clientInvalid)
                }
                if persistSession(refreshed) {
                    return .ok(refreshed)
                }
                return .err(Self.secureStorageUnavailable)
            }
        }
    }

    public func logout() -> AuthApiResult<EmptyAuthValue> {
        return ports.lock.withLock {
            let session = ports.store.load()
            let cleared = clearLocalAuth()
            let result: AuthApiResult<EmptyAuthValue>
            if let existing = session {
                result = ports.api.revokeAll(familyId: existing.familyId, accessToken: existing.accessToken)
            } else {
                result = .ok(EmptyAuthValue())
            }
            return cleared ? result : .err(Self.secureStorageUnavailable)
        }
    }

    public func rehydrate() -> AuthFlowState {
        guard let session = ports.store.load() else {
            if let backup = ports.keys.loadKey() {
                return .pendingKeyAccount(backup)
            }
            return .loggedOut
        }
        if session.familyId.isEmpty || session.accessToken.isEmpty ||
            session.refreshToken.isEmpty || session.accountId.isEmpty {
            return .loggedOut
        }
        let now = ports.clock.nowEpochSeconds()
        if PortableAuth.isAccessLive(session, nowEpochSeconds: now) {
            return .active(session)
        }
        if PortableAuth.isRefreshLive(session, nowEpochSeconds: now) || ports.keys.loadKey() != nil {
            return .needsRefresh(session)
        }
        return .loggedOut
    }

    public func beginProviderLogin(provider: String) -> AuthApiResult<PortableAuth.LoginAttempt> {
        if !PortableAuth.isProvider(provider) { return .err(Self.clientInvalid) }
        let attempt = PortableAuth.LoginAttempt(
            provider: provider,
            nonce: ports.nonces.nextNonce(),
            state: ports.nonces.nextNonce()
        )
        if attempt.nonce.isEmpty || attempt.state.isEmpty {
            return .err(Self.clientInvalid)
        }
        return .ok(attempt)
    }

    public func completeProviderLogin(
        session: PortableAuth.Session,
        attempt: PortableAuth.LoginAttempt,
        callback: PortableAuth.ProviderCallback
    ) -> AuthApiResult<EmptyAuthValue> {
        return ports.lock.withLock {
            if spentNonces.contains(callback.nonce) {
                return .err(PortableAuth.Verdict.codeUnknown)
            }
            guard let bound = PortableAuth.bind(attempt: attempt, callback: callback) else {
                return .err(Self.clientInvalid)
            }
            if bound.idToken.count > Self.maxProofChars {
                return .err(Self.clientInvalid)
            }
            let result = ports.api.linkProvider(
                session: session,
                provider: bound.provider,
                idToken: bound.idToken,
                nonce: bound.nonce
            )
            switch result {
            case .err(let verdict):
                if verdict == PortableAuth.Verdict.codeUnknown || verdict == "oidc-nonce-reused" {
                    spentNonces.insert(callback.nonce)
                }
                return .err(verdict)
            case .ok:
                spentNonces.insert(callback.nonce)
                return .ok(EmptyAuthValue())
            }
        }
    }

    public func deleteAccount() -> AuthApiResult<EmptyAuthValue> {
        return ports.lock.withLock {
            let session: PortableAuth.Session
            switch refreshSession() {
            case .err(let verdict):
                return .err(verdict)
            case .ok(let maybeSession):
                guard let live = maybeSession else {
                    return .err(Self.clientInvalid)
                }
                session = live
            }
            let result = ports.api.deleteAccount(session: session)
            switch result {
            case .ok:
                if !clearLocalAuth() { return .err(Self.secureStorageUnavailable) }
                return result
            case .err(let verdict):
                if isAuthDenial(verdict) {
                    if !clearLocalAuth() { return .err(Self.secureStorageUnavailable) }
                }
                return result
            }
        }
    }

    private func persistSession(_ session: PortableAuth.Session) -> Bool {
        do {
            try ports.store.save(session)
            return ports.store.load() == session
        } catch {
            return false
        }
    }

    private func isAuthDenial(_ verdict: String) -> Bool {
        switch verdict {
        case PortableAuth.Verdict.sessionRevoked,
             PortableAuth.Verdict.sessionReuse,
             PortableAuth.Verdict.sessionInvalid,
             PortableAuth.Verdict.sessionExpired,
             PortableAuth.Verdict.accountSuspended,
             PortableAuth.Verdict.accountDeleted,
             PortableAuth.Verdict.codeUnknown,
             "key-invalid",
             "account-not-found",
             Self.clientInvalid:
            return true
        default:
            return false
        }
    }

    private func clearLocalAuth() -> Bool {
        var success = true
        do { try ports.store.clear() } catch { success = false }
        do { try ports.keys.clearKey() } catch { success = false }
        return success
    }

    private func clearAndDeny(_ verdict: String) -> AuthApiResult<PortableAuth.Session?> {
        return clearLocalAuth() ? .err(verdict) : .err(Self.secureStorageUnavailable)
    }

    private func isEmailHint(_ address: String) -> Bool {
        let trimmed = address.trimmingCharacters(in: .whitespacesAndNewlines)
        if trimmed.isEmpty || trimmed.count > 254 || trimmed.contains(" ") { return false }
        guard let atIdx = trimmed.firstIndex(of: "@") else { return false }
        return atIdx > trimmed.startIndex && atIdx < trimmed.index(before: trimmed.endIndex)
    }

    private func isUsernameHint(_ name: String) -> Bool {
        if name.count < 3 || name.count > 20 { return false }
        guard let first = name.first, first >= "a" && first <= "z" else { return false }
        return name.allSatisfy { ($0 >= "a" && $0 <= "z") || ($0 >= "0" && $0 <= "9") }
    }
}

/// Native Keychain-backed auth session and key backup stores for iOS.
public final class IosKeychainSecretStore: AuthSessionStore, AuthKeyStore {
    private let service: String
    private let sessionAccount = "session-v1"
    private let keyAccount = "account-key-v1"
    private let lock = NSRecursiveLock()
    private var memoryCache: [String: Data] = [:]
    private let useMemoryOnly: Bool

    public init(
        service: String = "com.anpfuel.app.auth",
        useMemoryOnly: Bool = false
    ) {
        self.service = service
        self.useMemoryOnly = useMemoryOnly
    }

    public func save(_ session: PortableAuth.Session) throws {
        let data = try JSONEncoder().encode(session)
        try write(data, account: sessionAccount)
    }

    public func load() -> PortableAuth.Session? {
        lock.lock()
        defer { lock.unlock() }
        guard let data = read(account: sessionAccount),
              let session = try? JSONDecoder().decode(PortableAuth.Session.self, from: data) else { return nil }
        if session.familyId.hasPrefix("apple-") && session.refreshToken.hasPrefix("apple-refresh-") {
            try? delete(account: sessionAccount)
            return nil
        }
        return session
    }

    public func clear() throws {
        try delete(account: sessionAccount)
    }

    public func saveKey(_ backup: AuthKeyBackup) throws {
        let data = try JSONEncoder().encode(backup)
        try write(data, account: keyAccount)
    }

    public func loadKey() -> AuthKeyBackup? {
        lock.lock()
        defer { lock.unlock() }
        guard let data = read(account: keyAccount),
              let backup = try? JSONDecoder().decode(AuthKeyBackup.self, from: data) else { return nil }
        if backup.accountKey.hasPrefix("abv-") && backup.accountKey.count == 16 {
            try? delete(account: keyAccount)
            return nil
        }
        return backup
    }

    public func clearKey() throws {
        try delete(account: keyAccount)
    }

    private func write(_ data: Data, account: String) throws {
        lock.lock()
        defer { lock.unlock() }
        memoryCache[account] = data
        #if canImport(Security)
        if !useMemoryOnly {
            let deleteQuery: [String: Any] = [
                kSecClass as String: kSecClassGenericPassword,
                kSecAttrService as String: service,
                kSecAttrAccount as String: account,
            ]
            SecItemDelete(deleteQuery as CFDictionary)
            let addQuery: [String: Any] = [
                kSecClass as String: kSecClassGenericPassword,
                kSecAttrService as String: service,
                kSecAttrAccount as String: account,
                kSecValueData as String: data,
                kSecAttrAccessible as String: kSecAttrAccessibleAfterFirstUnlockThisDeviceOnly,
                kSecAttrSynchronizable as String: false,
            ]
            let status = SecItemAdd(addQuery as CFDictionary, nil)
            if status != errSecSuccess {
                throw NSError(domain: "AnpFuelKeychain", code: Int(status))
            }
        }
        #endif
    }

    private func read(account: String) -> Data? {
        lock.lock()
        defer { lock.unlock() }
        if let cached = memoryCache[account] {
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
               let data = result as? Data {
                memoryCache[account] = data
                return data
            }
        }
        #endif
        return nil
    }

    private func delete(account: String) throws {
        lock.lock()
        defer { lock.unlock() }
        memoryCache.removeValue(forKey: account)
        #if canImport(Security)
        if !useMemoryOnly {
            let query: [String: Any] = [
                kSecClass as String: kSecClassGenericPassword,
                kSecAttrService as String: service,
                kSecAttrAccount as String: account,
            ]
            let status = SecItemDelete(query as CFDictionary)
            if status != errSecSuccess && status != errSecItemNotFound {
                throw NSError(domain: "AnpFuelKeychain", code: Int(status))
            }
        }
        #endif
    }
}

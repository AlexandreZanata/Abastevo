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

/// Session plus whether the login created the account (native provider sign-in).
public struct AuthConsumeOk: Equatable {
    public let session: PortableAuth.Session
    public let created: Bool

    public init(session: PortableAuth.Session, created: Bool) {
        self.session = session
        self.created = created
    }
}

/// One-display key issued with a session by verified sign-up or key recovery.
public struct AuthIssuedKey: Equatable {
    public let session: PortableAuth.Session
    public let publicName: String
    public let accountKey: String

    public init(session: PortableAuth.Session, publicName: String, accountKey: String) {
        self.session = session
        self.publicName = publicName
        self.accountKey = accountKey
    }
}

/// The only display of a new key, kept even when durable storage fails (Android `AuthFlow.NewKey`).
public struct AuthNewKey: Equatable {
    public let backup: AuthKeyBackup
    public let sessionSaved: Bool

    public init(backup: AuthKeyBackup, sessionSaved: Bool) {
        self.backup = backup
        self.sessionSaved = sessionSaved
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
    /// `POST /v1/accounts/email/codes`; every well-formed request gets the same accepted reply.
    func requestCode(email: String, purpose: PortableAuth.CodePurpose, locale: String) -> AuthApiResult<EmptyAuthValue>
    /// `POST /v1/accounts/keys/signup`: public name plus a verified recovery e-mail.
    func signUp(name: String, email: String, code: String) -> AuthApiResult<AuthIssuedKey>
    /// `POST /v1/accounts/recovery`: replaces the key and revokes the other session families.
    func recover(email: String, code: String) -> AuthApiResult<AuthIssuedKey>
    func profile(session: PortableAuth.Session) -> AuthApiResult<PortableAuth.Profile>
    func updateName(session: PortableAuth.Session, name: String) -> AuthApiResult<PortableAuth.Profile>
    /// Needs a login family at most 600 seconds old (`reauthentication-required` otherwise).
    func setRecoveryEmail(session: PortableAuth.Session, email: String, code: String) -> AuthApiResult<PortableAuth.Profile>
    /// Legacy username sign-up (`POST /v1/accounts/keys`), kept for compatibility; the app no longer offers it.
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
    /// `POST /v1/accounts/providers/login`: native provider token plus the raw nonce; replies with account and session.
    func loginWithProvider(
        provider: String,
        idToken: String,
        nonce: String,
        authorizationCode: String?
    ) -> AuthApiResult<AuthConsumeOk>
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
/// Every mutation runs under `ports.lock`; the shell calls it off the main actor through `AuthBlockingWork`.
public final class AuthFlow: @unchecked Sendable {
    public static let clientInvalid = "client-invalid"
    public static let secureStorageUnavailable = "secure-storage-unavailable"
    public static let maxProofChars = 8192

    private let ports: AuthPorts
    private var spentNonces: Set<String> = []

    public init(ports: AuthPorts) {
        self.ports = ports
    }

    /// Requests a six-digit e-mail code. The purpose and locale only select the message copy.
    public func requestCode(
        email: String,
        purpose: PortableAuth.CodePurpose,
        locale: String
    ) -> AuthApiResult<EmptyAuthValue> {
        if !isEmailHint(email) { return .err(Self.clientInvalid) }
        return ports.api.requestCode(
            email: email.trimmingCharacters(in: .whitespacesAndNewlines),
            purpose: purpose,
            locale: locale
        )
    }

    /// Sign-up checks the public name before any code is requested, so a refused name never spends a code.
    public func requestSignUpCode(name: String, email: String, locale: String) -> AuthApiResult<EmptyAuthValue> {
        if !PublicName.isValid(name) { return .err(PortableAuth.Verdict.nameInvalid) }
        return requestCode(email: email, purpose: .signup, locale: locale)
    }

    /// Creates a FREE key account with a public name and a verified recovery e-mail, signed in at once.
    public func signUp(name: String, email: String, code: String) -> AuthApiResult<AuthNewKey> {
        return issueKey(email: email, code: code) { address in
            if !PublicName.isValid(name) {
                return .err(PortableAuth.Verdict.nameInvalid)
            }
            return ports.api.signUp(name: PublicName.normalize(name), email: address, code: code)
        }
    }

    /// Replaces the key through the verified recovery e-mail. The previous key and other sessions stop working.
    public func recover(email: String, code: String) -> AuthApiResult<AuthNewKey> {
        return issueKey(email: email, code: code) { address in
            ports.api.recover(email: address, code: code)
        }
    }

    public func profile() -> AuthApiResult<PortableAuth.Profile> {
        return withSession { session in ports.api.profile(session: session) }
    }

    public func updateName(_ name: String) -> AuthApiResult<PortableAuth.Profile> {
        if !PublicName.isValid(name) { return .err(PortableAuth.Verdict.nameInvalid) }
        let normalized = PublicName.normalize(name)
        return withSession { session in ports.api.updateName(session: session, name: normalized) }
    }

    public func setRecoveryEmail(email: String, code: String) -> AuthApiResult<PortableAuth.Profile> {
        if !isEmailHint(email) || !PortableAuth.isCodeShape(code) { return .err(Self.clientInvalid) }
        let address = email.trimmingCharacters(in: .whitespacesAndNewlines)
        return withSession { session in
            ports.api.setRecoveryEmail(session: session, email: address, code: code)
        }
    }

    /// Native provider login (Sign in with Apple). The raw nonce spends once client-side; the server
    /// matches its SHA-256 against the token claim. A retained key account refuses unless the login
    /// re-authenticates the stored account (`expectedAccountId`), which can never switch accounts.
    public func loginWithProvider(
        provider: String,
        idToken: String,
        rawNonce: String,
        authorizationCode: String?,
        expectedAccountId: String? = nil
    ) -> AuthApiResult<AuthLoginOutcome> {
        return ports.lock.withLock {
            if let expected = expectedAccountId {
                if ports.store.load()?.accountId != expected {
                    return .err(Self.clientInvalid)
                }
            } else if ports.keys.loadKey() != nil {
                return .err(Self.clientInvalid)
            }
            if !PortableAuth.isProvider(provider) || idToken.isEmpty || idToken.count > Self.maxProofChars ||
                rawNonce.isEmpty || rawNonce.count > AccountProviderLogin.maxNonceChars {
                return .err(Self.clientInvalid)
            }
            if spentNonces.contains(rawNonce) {
                return .err(PortableAuth.Verdict.codeUnknown)
            }
            let code: String? = (authorizationCode?.isEmpty ?? true) ? nil : authorizationCode
            switch ports.api.loginWithProvider(provider: provider, idToken: idToken, nonce: rawNonce, authorizationCode: code) {
            case .err(let verdict):
                if verdict == PortableAuth.Verdict.codeUnknown || verdict == "oidc-nonce-reused" {
                    spentNonces.insert(rawNonce)
                }
                return .err(verdict)
            case .ok(let res):
                spentNonces.insert(rawNonce)
                if let expected = expectedAccountId, res.session.accountId != expected {
                    return .err(Self.clientInvalid)
                }
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

    /// Key-only login. With `expectedAccountId` it re-authenticates the stored account and refuses any other.
    public func loginWithKey(rawKey: String, expectedAccountId: String? = nil) -> AuthApiResult<AuthLoginOutcome> {
        return ports.lock.withLock {
            let trimmedKey = rawKey.trimmingCharacters(in: .whitespacesAndNewlines)
            if trimmedKey.isEmpty {
                return .err(Self.clientInvalid)
            }
            if let expected = expectedAccountId, ports.store.load()?.accountId != expected {
                return .err(Self.clientInvalid)
            }
            switch ports.api.loginWithKey(accountKey: trimmedKey) {
            case .err(let verdict):
                return .err(verdict)
            case .ok(let res):
                if let expected = expectedAccountId, res.session.accountId != expected {
                    return .err(Self.clientInvalid)
                }
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

    private func issueKey(
        email: String,
        code: String,
        request: (String) -> AuthApiResult<AuthIssuedKey>
    ) -> AuthApiResult<AuthNewKey> {
        return ports.lock.withLock {
            if !isEmailHint(email) || !PortableAuth.isCodeShape(code) {
                return .err(Self.clientInvalid)
            }
            switch request(email.trimmingCharacters(in: .whitespacesAndNewlines)) {
            case .err(let verdict):
                return .err(verdict)
            case .ok(let issued):
                let backup = AuthKeyBackup(username: issued.publicName, accountKey: issued.accountKey)
                let saved = saveKey(backup)
                if !saved {
                    // Recovery has invalidated the preceding key; keeping it would misrepresent the backup.
                    try? ports.keys.clearKey()
                }
                let sessionSaved = saved && persistSession(issued.session)
                if !sessionSaved {
                    try? ports.store.clear()
                }
                return .ok(AuthNewKey(backup: backup, sessionSaved: sessionSaved))
            }
        }
    }

    private func withSession<T: Equatable>(
        _ request: (PortableAuth.Session) -> AuthApiResult<T>
    ) -> AuthApiResult<T> {
        return ports.lock.withLock {
            switch refreshSession() {
            case .err(let verdict):
                return .err(verdict)
            case .ok(let maybeSession):
                guard let session = maybeSession else {
                    return .err(PortableAuth.Verdict.sessionExpired)
                }
                return request(session)
            }
        }
    }

    private func saveKey(_ backup: AuthKeyBackup) -> Bool {
        do {
            try ports.keys.saveKey(backup)
            return ports.keys.loadKey() == backup
        } catch {
            return false
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

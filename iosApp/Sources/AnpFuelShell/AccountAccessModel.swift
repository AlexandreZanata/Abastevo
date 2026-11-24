import Foundation
import Combine
import AnpFuelCore

/// Verified sign-up, key recovery and private profile changes (Android `AuthViewModel`).
///
/// `AuthFlow` owns proof, session and key custody. This model keeps one
/// journey's form, the key shown once after sign-up or recovery and the
/// owner profile. Every blocking call runs through the host's `runAuth`.
@MainActor
public final class AccountAccessModel: ObservableObject {
    public enum Journey: Equatable, Sendable {
        case signUp
        case recovery
        case recoveryEmail
    }

    public struct CodeForm: Equatable, Sendable {
        public var name = ""
        public var email = ""
        public var code = ""
        public var codeSentTo: String?
        public var resendSeconds = 0
    }

    /// A key shown exactly once. `recoverable` keys have a verified recovery e-mail.
    public struct IssuedKey: Equatable {
        public let backup: AuthKeyBackup
        public let recovered: Bool
        public let recoverable: Bool
        public let sessionSaved: Bool
    }

    /// The server's per-address cooldown between two codes.
    static let resendCooldownSeconds = 60

    @Published public var form = CodeForm()
    @Published public var keyEntry = ""
    @Published public private(set) var error: AccountAccessError?
    @Published public private(set) var issuedKey: IssuedKey?
    @Published public private(set) var profile: PortableAuth.Profile?
    @Published public private(set) var profileError: AccountAccessError?
    @Published public private(set) var isLoadingProfile = false
    @Published public private(set) var freshProofAccepted = false

    private weak var host: AppStateViewModel?
    private var resendTask: Task<Void, Never>?
    private var owner: String?

    init(host: AppStateViewModel) {
        self.host = host
    }

    /// True once a code was requested for the address now in the form.
    public var codeSent: Bool {
        guard let sentTo = form.codeSentTo else { return false }
        return sentTo == Self.trimmed(form.email)
    }

    public func canRequestCode(for journey: Journey) -> Bool {
        if Self.trimmed(form.email).isEmpty { return false }
        if journey == .signUp && Self.trimmed(form.name).isEmpty { return false }
        return !codeSent || form.resendSeconds == 0
    }

    public var canVerifyCode: Bool {
        codeSent && PortableAuth.isCodeShape(form.code)
    }

    static func sanitizeCode(_ value: String) -> String {
        String(value.filter { $0.isASCII && $0.isNumber }.prefix(PortableAuth.codeDigits))
    }

    public func begin(_ journey: Journey) {
        stopResend()
        form = CodeForm()
        error = nil
        if journey == .recoveryEmail {
            keyEntry = ""
            freshProofAccepted = false
        }
    }

    public func clearError() {
        error = nil
    }

    // MARK: - E-mail code journeys

    public func requestCode(for journey: Journey) {
        guard let host, !host.isAuthWorking, canRequestCode(for: journey) else { return }
        if journey == .signUp && !PublicName.isValid(form.name) {
            error = .invalidName
            return
        }
        if host.isOfflineSimulation {
            error = .offline
            return
        }
        let flow = host.authFlow
        let snapshot = form
        let locale = Self.codeLocale
        error = nil
        host.runAuth({ () -> AuthApiResult<EmptyAuthValue> in
            switch journey {
            case .signUp:
                return flow.requestSignUpCode(name: snapshot.name, email: snapshot.email, locale: locale)
            case .recovery:
                return flow.requestCode(email: snapshot.email, purpose: .recovery, locale: locale)
            case .recoveryEmail:
                return flow.requestCode(email: snapshot.email, purpose: .attach, locale: locale)
            }
        }) { [weak self] _, result in
            guard let self else { return }
            switch result {
            case .ok:
                self.form.codeSentTo = Self.trimmed(snapshot.email)
                self.form.code = ""
                self.startResend()
            case .err(let verdict):
                self.error = AccountAccessError(verdict: verdict)
            }
        }
    }

    /// Sign-up and recovery sign in at once and show the new key; a recovery e-mail change updates the profile.
    public func verifyCode(for journey: Journey, onSuccess: @escaping () -> Void) {
        guard let host, !host.isAuthWorking, canVerifyCode else { return }
        if host.isOfflineSimulation {
            error = .offline
            return
        }
        let flow = host.authFlow
        let snapshot = form
        error = nil
        switch journey {
        case .signUp, .recovery:
            host.runAuth({ () -> AuthApiResult<AuthNewKey> in
                journey == .signUp
                    ? flow.signUp(name: snapshot.name, email: snapshot.email, code: snapshot.code)
                    : flow.recover(email: snapshot.email, code: snapshot.code)
            }) { [weak self] host, result in
                guard let self else { return }
                switch result {
                case .ok(let newKey):
                    self.stopResend()
                    self.form = CodeForm()
                    self.issuedKey = IssuedKey(
                        backup: newKey.backup,
                        recovered: journey == .recovery,
                        recoverable: true,
                        sessionSaved: newKey.sessionSaved
                    )
                    if newKey.sessionSaved {
                        host.completeSignIn()
                    } else {
                        self.error = .secureStorage
                        host.rehydrateAuth()
                    }
                    onSuccess()
                case .err(let verdict):
                    self.error = AccountAccessError(verdict: verdict)
                }
            }
        case .recoveryEmail:
            host.runAuth({ () -> AuthApiResult<PortableAuth.Profile> in
                flow.setRecoveryEmail(email: snapshot.email, code: snapshot.code)
            }) { [weak self] host, result in
                guard let self else { return }
                switch result {
                case .ok(let profile):
                    self.stopResend()
                    self.form = CodeForm()
                    self.apply(profile, host: host)
                    onSuccess()
                case .err(let verdict):
                    let refusal = AccountAccessError(verdict: verdict)
                    if refusal == .reauthenticationRequired {
                        self.freshProofAccepted = false
                    }
                    self.error = refusal
                    host.rehydrateAuth()
                }
            }
        }
    }

    /// Leaves the key panel. Without a saved session the shown key signs in first, as on Android.
    public func continueAfterIssuedKey(onDone: @escaping () -> Void) {
        guard let host, !host.isAuthWorking, let shown = issuedKey ?? pendingKey(host: host) else { return }
        if shown.sessionSaved {
            issuedKey = nil
            error = nil
            onDone()
            return
        }
        let flow = host.authFlow
        let key = shown.backup.accountKey
        error = nil
        host.runAuth({ () -> AuthApiResult<AuthLoginOutcome> in
            flow.loginWithKey(rawKey: key)
        }) { [weak self] host, result in
            guard let self else { return }
            switch result {
            case .ok:
                self.issuedKey = nil
                host.completeSignIn()
                onDone()
            case .err(let verdict):
                self.error = AccountAccessError(verdict: verdict)
                host.rehydrateAuth()
            }
        }
    }

    /// The key shown in the sign-in sheet: the one just issued, or a retained backup without a session.
    public func shownKey(host: AppStateViewModel) -> IssuedKey? {
        issuedKey ?? pendingKey(host: host)
    }

    private func pendingKey(host: AppStateViewModel) -> IssuedKey? {
        guard case .pendingKeyAccount(let backup) = host.authFlowState else { return nil }
        return IssuedKey(backup: backup, recovered: false, recoverable: false, sessionSaved: false)
    }

    // MARK: - Fresh proof before a recovery e-mail change

    public func reauthenticateWithKey() {
        guard let host, !host.isAuthWorking else { return }
        guard let accountId = host.currentAccountIdOrNil else {
            error = .sessionExpired
            return
        }
        if host.isOfflineSimulation {
            error = .offline
            return
        }
        let flow = host.authFlow
        let key = keyEntry
        error = nil
        host.runAuth({ () -> AuthApiResult<AuthLoginOutcome> in
            flow.loginWithKey(rawKey: key, expectedAccountId: accountId)
        }) { [weak self] host, result in
            guard let self else { return }
            self.keyEntry = ""
            self.acceptFreshProof(result, host: host)
        }
    }

    public func reauthenticateWithApple(identityToken: Data?, authorizationCode: Data?) {
        guard let host, !host.isAuthWorking else { return }
        guard let rawNonce = host.takeAppleNonce(),
              let tokenData = identityToken,
              let idToken = String(data: tokenData, encoding: .utf8),
              !idToken.isEmpty else {
            error = .providerDenied
            return
        }
        guard let accountId = host.currentAccountIdOrNil else {
            error = .sessionExpired
            return
        }
        if host.isOfflineSimulation {
            error = .offline
            return
        }
        let flow = host.authFlow
        let code = authorizationCode.flatMap { String(data: $0, encoding: .utf8) }
        error = nil
        host.runAuth({ () -> AuthApiResult<AuthLoginOutcome> in
            flow.loginWithProvider(
                provider: PortableAuth.providerApple,
                idToken: idToken,
                rawNonce: rawNonce,
                authorizationCode: code,
                expectedAccountId: accountId
            )
        }) { [weak self] host, result in
            self?.acceptFreshProof(result, host: host)
        }
    }

    public func failAppleReauthentication(cancelled: Bool) {
        _ = host?.takeAppleNonce()
        error = cancelled ? nil : .providerDenied
    }

    private func acceptFreshProof(_ result: AuthApiResult<AuthLoginOutcome>, host: AppStateViewModel) {
        switch result {
        case .ok:
            freshProofAccepted = true
            error = nil
        case .err(let verdict):
            error = AccountAccessError(verdict: verdict)
        }
        host.rehydrateAuth()
    }

    // MARK: - Owner profile

    public func loadProfile() {
        guard let host, !host.isAuthWorking, host.currentAccountIdOrNil != nil else { return }
        if host.isOfflineSimulation {
            profileError = .offline
            return
        }
        let flow = host.authFlow
        isLoadingProfile = true
        profileError = nil
        host.runAuth({ () -> AuthApiResult<PortableAuth.Profile> in
            flow.profile()
        }) { [weak self] host, result in
            guard let self else { return }
            self.isLoadingProfile = false
            switch result {
            case .ok(let profile):
                self.apply(profile, host: host)
            case .err(let verdict):
                self.profileError = AccountAccessError(verdict: verdict)
                host.rehydrateAuth()
            }
        }
    }

    public func saveName(_ name: String, onSaved: @escaping () -> Void) {
        guard let host, !host.isAuthWorking else { return }
        if !PublicName.isValid(name) {
            error = .invalidName
            return
        }
        if host.isOfflineSimulation {
            error = .offline
            return
        }
        let flow = host.authFlow
        error = nil
        host.runAuth({ () -> AuthApiResult<PortableAuth.Profile> in
            flow.updateName(name)
        }) { [weak self] host, result in
            guard let self else { return }
            switch result {
            case .ok(let profile):
                self.apply(profile, host: host)
                onSaved()
            case .err(let verdict):
                self.error = AccountAccessError(verdict: verdict)
                host.rehydrateAuth()
            }
        }
    }

    /// Drops the profile and any proof state when the signed-in owner changes or signs out.
    /// A key shown once stays until its Continue button.
    func ownerChanged(to accountId: String?) {
        guard accountId != owner else { return }
        owner = accountId
        profile = nil
        profileError = nil
        freshProofAccepted = false
        keyEntry = ""
    }

    /// The confirmed public name for the header, when the loaded profile belongs to the account.
    func publicName(for accountId: String) -> String? {
        guard let profile, profile.accountId == accountId else { return nil }
        return profile.publicName
    }

    private func apply(_ profile: PortableAuth.Profile, host: AppStateViewModel) {
        guard profile.accountId == host.currentAccountIdOrNil else { return }
        self.profile = profile
        profileError = nil
        host.signedInAlias = profile.publicName
    }

    // MARK: - Resend cooldown

    private func startResend() {
        resendTask?.cancel()
        form.resendSeconds = Self.resendCooldownSeconds
        resendTask = Task { [weak self] in
            while !Task.isCancelled {
                try? await Task.sleep(nanoseconds: 1_000_000_000)
                guard let self, !Task.isCancelled else { return }
                if self.form.resendSeconds <= 1 {
                    self.form.resendSeconds = 0
                    return
                }
                self.form.resendSeconds -= 1
            }
        }
    }

    private func stopResend() {
        resendTask?.cancel()
        resendTask = nil
    }

    private static func trimmed(_ value: String) -> String {
        value.trimmingCharacters(in: .whitespacesAndNewlines)
    }

    /// The app's resolved language selects the e-mail copy (English or Brazilian Portuguese).
    private static var codeLocale: String {
        Bundle.main.preferredLocalizations.first ?? "pt-BR"
    }
}

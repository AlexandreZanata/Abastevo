import SwiftUI
import AnpFuelCore
#if canImport(AuthenticationServices)
import AuthenticationServices
#endif

/// Localized copy for account refusals; shared by every sign-in and profile screen.
enum AccountAccessCopy {
    static func message(for error: AccountAccessError) -> String {
        switch error {
        case .invalidInput: return String(localized: "android.auth_error_input")
        case .wrongCode: return String(localized: "android.auth_error_code")
        case .wrongKey: return String(localized: "android.auth_error_key")
        case .expiredCode: return String(localized: "android.auth_error_expired")
        case .lockedCode: return String(localized: "android.auth_error_locked")
        case .sessionExpired: return String(localized: "android.auth_error_session")
        case .suspended: return String(localized: "android.auth_error_suspended")
        case .deleted: return String(localized: "android.auth_error_deleted")
        case .offline: return String(localized: "android.auth_error_offline")
        case .providerDenied: return String(localized: "android.auth_error_provider")
        case .providerUnavailable: return String(localized: "auth.apple.serverUnavailable")
        case .secureStorage: return String(localized: "android.auth_error_secure_storage")
        case .invalidName: return String(localized: "android.auth_error_public_name")
        case .addressLinked: return String(localized: "android.auth_error_address_linked")
        case .recoveryNoAccount: return String(localized: "android.auth_error_recovery_no_account")
        case .reauthenticationRequired: return String(localized: "android.auth_error_fresh_login")
        case .emailLoginRetired: return String(localized: "auth.error.email_login_retired")
        case .throttled: return String(localized: "auth.error.throttled")
        case .unknown: return String(localized: "android.auth_error_unknown")
        }
    }
}

/// Shows a refusal below a form, in the error color.
struct AccountAccessErrorSection: View {
    let error: AccountAccessError?

    var body: some View {
        if let error {
            Section {
                Text(AccountAccessCopy.message(for: error))
                    .foregroundStyle(.red)
            }
        }
    }
}

/// A key shown once after sign-up or recovery, or a retained key that still needs its first sign-in.
struct IssuedKeySection: View {
    let issued: AccountAccessModel.IssuedKey
    let isWorking: Bool
    let onContinue: () -> Void

    var body: some View {
        Section {
            Text(warning)
                .font(.subheadline)
            if issued.recoverable {
                LabeledContent(String(localized: "android.auth_public_name"), value: issued.backup.username)
            } else {
                Text(String(localized: "android.profile_backup_username", defaultValue: "Username: \(issued.backup.username)"))
                    .font(.subheadline.bold())
            }
            Text(issued.backup.accountKey)
                .font(.body.monospaced())
                .textSelection(.enabled)
                .privacySensitive()
                .accessibilityLabel(String(localized: "android.auth_key_label"))
            Button {
                onContinue()
            } label: {
                Text(String(localized: "android.auth_saved_enter"))
                    .frame(maxWidth: .infinity)
            }
            .buttonStyle(.borderedProminent)
            .tint(AbastevoTheme.ink)
            .disabled(isWorking)
        } header: {
            Text(issued.recovered ? String(localized: "android.auth_new_key_title") : String(localized: "android.auth_key_issued_title"))
        }
    }

    private var warning: String {
        if issued.recovered {
            return String(localized: "android.auth_new_key_copy")
        }
        return issued.recoverable
            ? String(localized: "android.auth_key_issued_warning")
            : String(localized: "android.auth_legacy_key_issued_warning")
    }
}

/// Sign-up, key recovery or a recovery e-mail change: one address proven with a six-digit code.
struct AccountCodeJourneyView: View {
    @Environment(\.dismiss) private var dismiss
    @ObservedObject var viewModel: AppStateViewModel
    @ObservedObject var access: AccountAccessModel
    let journey: AccountAccessModel.Journey
    @State private var started = false

    var body: some View {
        Form {
            if journey == .recoveryEmail && !access.freshProofAccepted {
                AccountReauthenticationSection(viewModel: viewModel, access: access)
            } else {
                addressSection
                if access.codeSent {
                    codeSection
                }
            }
            AccountAccessErrorSection(error: access.error)
            if viewModel.isAuthWorking {
                Section {
                    ProgressView()
                }
            }
        }
        .navigationTitle(title)
        .onAppear {
            // Only the first appearance starts the journey; returning from the Apple sheet keeps it.
            if !started {
                started = true
                access.begin(journey)
            }
        }
    }

    private var title: String {
        switch journey {
        case .signUp: return String(localized: "android.auth_create_account")
        case .recovery: return String(localized: "android.auth_lost_key")
        case .recoveryEmail: return String(localized: "android.auth_recovery_email_title")
        }
    }

    private var addressSection: some View {
        Section {
            if journey == .signUp {
                TextField(String(localized: "android.auth_public_name"), text: $access.form.name)
                    .textContentType(.name)
                    .autocorrectionDisabled()
                Text(String(localized: "android.auth_name_helper"))
                    .font(.caption)
                    .foregroundStyle(.secondary)
            }
            TextField(String(localized: "android.auth_email_label"), text: $access.form.email)
                .keyboardType(.emailAddress)
                .textContentType(.emailAddress)
                .textInputAutocapitalization(.never)
                .autocorrectionDisabled()
            if journey != .recovery {
                Text(String(localized: "android.auth_email_helper"))
                    .font(.caption)
                    .foregroundStyle(.secondary)
            }
            if !access.codeSent {
                Button(String(localized: "auth.send_code")) {
                    access.requestCode(for: journey)
                }
                .disabled(viewModel.isAuthWorking || !access.canRequestCode(for: journey))
            }
        }
    }

    private var codeSection: some View {
        Section {
            Text(String(localized: "android.auth_email_check_inbox"))
                .font(.subheadline)
            TextField(String(localized: "android.auth_code_label"), text: codeBinding)
                .keyboardType(.numberPad)
                .textContentType(.oneTimeCode)
            Button(String(localized: "android.auth_verify_code")) {
                access.verifyCode(for: journey) { dismiss() }
            }
            .disabled(viewModel.isAuthWorking || !access.canVerifyCode)
            Button(resendLabel) {
                access.requestCode(for: journey)
            }
            .disabled(viewModel.isAuthWorking || access.form.resendSeconds > 0)
        } header: {
            Text(String(localized: "android.auth_email_verification_title"))
        }
    }

    private var resendLabel: String {
        let seconds = access.form.resendSeconds
        if seconds == 0 {
            return String(localized: "android.auth_resend_code")
        }
        return String(localized: "android.auth_resend_after", defaultValue: "Resend in \(seconds) s")
    }

    private var codeBinding: Binding<String> {
        Binding(
            get: { access.form.code },
            set: { access.form.code = AccountAccessModel.sanitizeCode($0) }
        )
    }
}

/// A login family at most 600 seconds old: the account key, or Sign in with Apple for a linked account.
struct AccountReauthenticationSection: View {
    @ObservedObject var viewModel: AppStateViewModel
    @ObservedObject var access: AccountAccessModel

    var body: some View {
        Section {
            if access.profile?.hasAccountKey != false {
                SecureField(String(localized: "android.auth_key_label"), text: $access.keyEntry)
                    .textInputAutocapitalization(.never)
                    .autocorrectionDisabled()
                Button(String(localized: "android.auth_key_login")) {
                    access.reauthenticateWithKey()
                }
                .disabled(viewModel.isAuthWorking || access.keyEntry.trimmingCharacters(in: .whitespacesAndNewlines).isEmpty)
            }
            if access.profile?.providers.contains(PortableAuth.providerApple) == true {
                AppleReauthenticationButton(viewModel: viewModel, access: access)
            }
        } header: {
            Text(String(localized: "android.auth_reauthenticate_title"))
        } footer: {
            Text(String(localized: "android.auth_error_fresh_login"))
        }
    }
}

/// Sign in with Apple that only renews the session of the account already on the device.
struct AppleReauthenticationButton: View {
    @ObservedObject var viewModel: AppStateViewModel
    @ObservedObject var access: AccountAccessModel
    @Environment(\.colorScheme) private var colorScheme

    var body: some View {
        #if canImport(AuthenticationServices)
        SignInWithAppleButton(.continue) { request in
            request.requestedScopes = []
            request.nonce = viewModel.prepareAppleSignInNonce()
        } onCompletion: { result in
            switch result {
            case .success(let authorization):
                if let credential = authorization.credential as? ASAuthorizationAppleIDCredential {
                    access.reauthenticateWithApple(
                        identityToken: credential.identityToken,
                        authorizationCode: credential.authorizationCode
                    )
                } else {
                    access.failAppleReauthentication(cancelled: false)
                }
            case .failure(let error):
                access.failAppleReauthentication(cancelled: (error as? ASAuthorizationError)?.code == .canceled)
            }
        }
        .signInWithAppleButtonStyle(colorScheme == .dark ? .white : .black)
        .frame(height: 44)
        .disabled(viewModel.isAuthWorking || viewModel.isOfflineSimulation)
        #else
        Text(String(localized: "identity.platform_unavailable"))
            .foregroundStyle(.secondary)
        #endif
    }
}

/// Public name and recovery e-mail of the signed-in account.
struct AccountProfileSection: View {
    @ObservedObject var viewModel: AppStateViewModel
    @ObservedObject var access: AccountAccessModel

    var body: some View {
        Section {
            if let profile = access.profile {
                LabeledContent(String(localized: "android.auth_public_name"), value: profile.publicName)
                NavigationLink {
                    PublicNameEditView(viewModel: viewModel, access: access, initialName: profile.name ?? "")
                } label: {
                    Text(String(localized: "android.auth_edit_name"))
                }
                if let hint = profile.recoveryEmailHint {
                    LabeledContent(String(localized: "android.auth_recovery_email_title"), value: hint)
                }
                NavigationLink {
                    AccountCodeJourneyView(viewModel: viewModel, access: access, journey: .recoveryEmail)
                } label: {
                    Text(profile.recoveryEmailHint == nil
                         ? String(localized: "android.auth_add_recovery_email")
                         : String(localized: "android.auth_change_email"))
                }
            } else if access.isLoadingProfile {
                ProgressView()
            } else if let error = access.profileError {
                Text(AccountAccessCopy.message(for: error))
                    .foregroundStyle(.secondary)
                Button(String(localized: "android.action_retry")) {
                    access.loadProfile()
                }
                .disabled(viewModel.isAuthWorking)
            }
        } header: {
            Text(String(localized: "android.profile_account_section"))
        } footer: {
            if let profile = access.profile, profile.hasAccountKey,
               profile.name == nil || profile.recoveryEmailHint == nil {
                Text(String(localized: "android.auth_protect_copy"))
            }
        }
    }
}

/// Edits the public name shown on comments and as the default forum author name.
struct PublicNameEditView: View {
    @Environment(\.dismiss) private var dismiss
    @ObservedObject var viewModel: AppStateViewModel
    @ObservedObject var access: AccountAccessModel
    @State private var name: String

    init(viewModel: AppStateViewModel, access: AccountAccessModel, initialName: String) {
        self.viewModel = viewModel
        self.access = access
        _name = State(initialValue: initialName)
    }

    var body: some View {
        Form {
            Section {
                TextField(String(localized: "android.auth_public_name"), text: $name)
                    .textContentType(.name)
                    .autocorrectionDisabled()
            } footer: {
                Text(String(localized: "android.auth_name_helper"))
            }
            Section {
                Button(String(localized: "android.auth_save_name")) {
                    access.saveName(name) { dismiss() }
                }
                .disabled(viewModel.isAuthWorking || name.trimmingCharacters(in: .whitespacesAndNewlines).isEmpty)
            }
            AccountAccessErrorSection(error: access.error)
        }
        .navigationTitle(String(localized: "android.auth_edit_name"))
        .onAppear { access.clearError() }
    }
}

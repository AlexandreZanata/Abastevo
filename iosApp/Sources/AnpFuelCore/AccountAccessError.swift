import Foundation

/// Refusal categories the account screens explain (Android `AuthUiError`).
///
/// Wrong and already used codes share one message, as on Android. The
/// retired passwordless e-mail login and the account API's 429 budgets have
/// their own categories so a stale path or a throttled request never reads
/// as a generic failure.
public enum AccountAccessError: Equatable, Sendable {
    case invalidInput
    case wrongCode
    case wrongKey
    case expiredCode
    case lockedCode
    case sessionExpired
    case suspended
    case deleted
    case offline
    case providerDenied
    case providerUnavailable
    case secureStorage
    case invalidName
    case addressLinked
    case recoveryNoAccount
    case reauthenticationRequired
    case emailLoginRetired
    case throttled
    case unknown

    public init(verdict: String) {
        switch verdict {
        case AuthFlow.clientInvalid, "username-invalid", "bad-body", PortableAuth.Verdict.emailInvalid:
            self = .invalidInput
        case AuthFlow.secureStorageUnavailable:
            self = .secureStorage
        case PortableAuth.Verdict.codeUnknown, PortableAuth.Verdict.codeConsumed:
            self = .wrongCode
        case "key-invalid":
            self = .wrongKey
        case PortableAuth.Verdict.codeExpired:
            self = .expiredCode
        case PortableAuth.Verdict.codeLocked:
            self = .lockedCode
        case PortableAuth.Verdict.sessionReuse, PortableAuth.Verdict.sessionRevoked,
             PortableAuth.Verdict.sessionInvalid, PortableAuth.Verdict.sessionExpired:
            self = .sessionExpired
        case PortableAuth.Verdict.accountSuspended:
            self = .suspended
        case PortableAuth.Verdict.accountDeleted:
            self = .deleted
        case PortableAuth.Verdict.nameInvalid:
            self = .invalidName
        case PortableAuth.Verdict.addressLinked:
            self = .addressLinked
        case PortableAuth.Verdict.recoveryNoAccount:
            self = .recoveryNoAccount
        case PortableAuth.Verdict.reauthenticationRequired:
            self = .reauthenticationRequired
        case PortableAuth.Verdict.emailLoginRetired:
            self = .emailLoginRetired
        case PortableAuth.Verdict.admissionExhausted, PortableAuth.Verdict.sessionBudgetExhausted:
            self = .throttled
        case PortableAuth.Verdict.providerUnavailable, "oidc-unavailable":
            self = .providerUnavailable
        case PortableAuth.unavailable:
            self = .offline
        case let other where other.hasPrefix("oidc-") || other.hasPrefix("link-"):
            self = .providerDenied
        default:
            self = .unknown
        }
    }
}

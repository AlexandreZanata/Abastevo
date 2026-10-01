/// Portable feedback presentation (P17-T03, B-BR-F03/F06).
///
/// Swift port of the Kotlin `FeedbackDisplay` (itself rendering the frozen
/// backend tallies): agreement renders from exact counts with floor
/// truncation and one decimal (2 valid / 1 invalid → 66.6%); zero votes
/// render "No votes", never 0% certainty. Text travels verbatim and
/// callers must use plain `Text` — HTML is never rendered. Rejection
/// labels are fixed short codes; private report reasons and dispute
/// detail never enter copy.
///
/// NOT COMPILED: no Swift/Xcode toolchain on this Linux host. First real
/// build/test runs on macOS (Xcode 26.4); see iosApp/README.md.
public enum FeedbackRejectKind: String, CaseIterable {
    case targetInvalid
    case ratingOutOfRange
    case textEmpty
    case textTooLong
    case notAuthor
    case staleRevision
    case commentNotFound
    case ratingNotFound
    case selfVote
    case voteChoiceInvalid
    case reportInvalid
    case quotaExceeded
    case transport
    case gateRequired
}

public enum FeedbackDisplay {
    /// Scalar budget left for the 280 rule; negative means over.
    public static func charsRemaining(_ text: String) -> Int {
        let normalized = PortableText.normalize(text)
        return PortableText.maxCommentScalars - PortableText.countScalars(normalized)
    }

    /// One-line agreement with valid/invalid/total counts.
    public static func agreementLine(valid: Int64, invalid: Int64) -> String {
        let total = valid + invalid
        if total <= 0 { return "No votes" }
        guard let basisPoints = PortableFeedback.agreementBasisPoints(valid: valid, invalid: invalid) else {
            return "No votes"
        }
        let whole = basisPoints / 100
        let tenth = (basisPoints % 100) / 10
        return "\(whole).\(tenth)% · \(valid) valid / \(invalid) invalid"
    }

    public static func rejectKindLabel(_ kind: FeedbackRejectKind) -> String {
        switch kind {
        case .targetInvalid: return "TARGET"
        case .ratingOutOfRange: return "STARS"
        case .textEmpty: return "EMPTY"
        case .textTooLong: return "TOO_LONG"
        case .notAuthor: return "NOT_AUTHOR"
        case .staleRevision: return "STALE"
        case .commentNotFound: return "NOT_FOUND"
        case .ratingNotFound: return "NOT_FOUND"
        case .selfVote: return "SELF_VOTE"
        case .voteChoiceInvalid: return "CHOICE"
        case .reportInvalid: return "REPORT"
        case .quotaExceeded: return "QUOTA"
        case .transport: return "TRANSPORT"
        case .gateRequired: return "SIGN_IN"
        }
    }
}

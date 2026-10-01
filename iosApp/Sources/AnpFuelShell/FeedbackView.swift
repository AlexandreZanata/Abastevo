import SwiftUI
import AnpFuelCore

/// Thin iPhone feedback view (P17-T03).
///
/// Renders the same fixed copy as Android `FeedbackDisplay`: 280-scalar
/// counter, floor-truncated agreement line ("No votes", never 0%
/// certainty) and fixed rejection labels. Text is plain `Text` only —
/// HTML is never rendered. No network IO happens here; writes flow
/// through `FeedbackFlow` guards and the transport binding verified on
/// macOS against a synthetic server.
///
/// NOT COMPILED: no Swift/Xcode toolchain on this Linux host. First real
/// simulator/device run happens on macOS (Xcode 26.4); see iosApp/README.md.
struct FeedbackView: View {
    let commentsRemaining: Int
    let agreementText: String
    let state: FeedbackUiState

    var body: some View {
        VStack(alignment: .leading, spacing: 8) {
            Text("\(commentsRemaining) characters left")
                .font(.caption)
                .foregroundStyle(.secondary)
            Text(agreementText)
                .font(.subheadline)
            switch state {
            case .disabled:
                Text("Feedback unavailable").foregroundStyle(.secondary)
            case .signInRequired:
                Text("Sign in to participate").font(.headline)
            case .submitting:
                ProgressView()
            case .rejected(let kindLabel, let message):
                Text("\(kindLabel): \(message)").foregroundStyle(.red)
            case .queued(let opId):
                Text("Queued \(opId)").foregroundStyle(.secondary)
            case .saved(let commentId, let revision):
                Text("Saved \(commentId) rev \(revision)")
            case .voted(let valid, let invalid):
                Text(FeedbackDisplay.agreementLine(valid: valid, invalid: invalid))
            case .voteRemoved:
                Text("Vote removed")
            case .reported:
                Text("Reported")
            case .commentDeleted:
                Text("Deleted")
            case .idle:
                Text("Share station feedback").foregroundStyle(.secondary)
            }
        }
        .padding()
    }
}

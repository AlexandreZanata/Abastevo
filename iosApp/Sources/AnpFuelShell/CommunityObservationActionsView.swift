import SwiftUI
import AnpFuelCore

/// Price confirmations affect consensus. Helpfulness and reports are displayed separately.
@MainActor
struct CommunityObservationActionsView: View {
    @ObservedObject var viewModel: AppStateViewModel
    let observation: CommunityPriceObservation
    let reference: CommunityContributionReference
    let origin: String

    @State private var tally: CommunityHelpfulnessTally?
    @State private var lastScope: Scope?
    @State private var loadError: String?
    @State private var notice: String?
    @State private var retryIntent: CommunityObservationIntent?
    @State private var notBefore: Date?
    @State private var generation = UUID()
    @State private var isLoading = false
    @State private var isWriting = false
    @State private var confirmed = false
    @State private var reported = false
    @State private var showingConfirmation = false
    @State private var showingReasons = false

    private struct Scope: Equatable {
        let accountId: String?
        let origin: String
        let offline: Bool
    }

    private var scope: Scope {
        Scope(accountId: viewModel.currentAccountIdOrNil, origin: viewModel.snapshot.apiEnvironment.origin,
              offline: viewModel.isOfflineSimulation)
    }

    var body: some View {
        List {
            Section {
                Text(observation.stationName).font(.headline)
                Text(AbastevoTheme.formatBrlMilli(observation.amountMilli)).font(.title3)
                Text(observation.condition.localizedLabel).foregroundStyle(.secondary)
            }
            if lastScope == scope {
                tallySection
                if let notice {
                    Text(notice).foregroundStyle(.secondary)
                }
                retrySection
            }
        }
        .navigationTitle(String(localized: "observation.actions.open"))
        .task(id: scope) { await load() }
        .refreshable { await load() }
        .confirmationDialog(
            String(localized: "android.community_confirm_price"),
            isPresented: $showingConfirmation, titleVisibility: .visible
        ) {
            Button(String(localized: "android.community_vote_confirm")) {
                Task { await perform(.confirm) }
            }
        } message: {
            Text(String(localized: "android.community_vote_dispute_hint"))
        }
        .confirmationDialog(
            String(localized: "android.community_helpful_report"), isPresented: $showingReasons, titleVisibility: .visible
        ) {
            ForEach(CommunityDisputeReason.allCases) { reason in
                Button(reason.label) { Task { await perform(.dispute(reason)) } }
            }
        }
    }

    @ViewBuilder private var tallySection: some View {
        Section(String(localized: "android.community_helpful_title")) {
            if isLoading {
                ProgressView(String(localized: "observation.actions.loading"))
            }
            if let loadError {
                Text(loadError).foregroundStyle(.secondary)
                Button(String(localized: "android.action_retry")) { Task { await load() } }
                    .disabled(isLoading)
            }
            if let tally {
                Text(String(localized: "android.community_helpful_counts",
                            defaultValue: "\(tally.helpful) helpful · \(tally.notHelpful) not helpful"))
                if let choice = tally.myChoice {
                    Text(String(localized: "android.community_helpful_choice", defaultValue: "Your choice: \(choice.label)"))
                }
                if tally.underReview {
                    Text(String(localized: "android.community_helpful_under_review"))
                }
            }
            if tally?.ownContribution == true {
                Text(String(localized: "android.community_own_contribution"))
                    .foregroundStyle(.secondary)
            } else if scope.accountId == nil {
                Text(String(localized: "android.community_helpful_guest_body")).foregroundStyle(.secondary)
                Button(String(localized: "android.community_helpful_guest")) { viewModel.activeSheet = .auth }
            } else if tally != nil {
                voteButtons
            }
        }
    }

    private var voteButtons: some View {
        Group {
            Button(String(localized: "android.community_helpful_helpful")) {
                Task { await perform(.helpfulness(.helpful)) }
            }
            .disabled(tally?.myChoice == .helpful)
            Button(String(localized: "android.community_helpful_not_helpful")) {
                Task { await perform(.helpfulness(.notHelpful)) }
            }
            .disabled(tally?.myChoice == .notHelpful)
            if tally?.myChoice != nil {
                Button(String(localized: "android.community_helpful_remove")) {
                    Task { await perform(.removeHelpfulness) }
                }
            }
            Button(String(localized: "android.community_confirm_price")) { showingConfirmation = true }
                .disabled(confirmed)
            Button(String(localized: "android.community_helpful_report")) { showingReasons = true }
                .disabled(reported)
        }
        .disabled(isLoading || isWriting || retryIntent != nil || scope.offline)
    }

    @ViewBuilder private var retrySection: some View {
        if isWriting {
            ProgressView(String(localized: "observation.actions.sending"))
        }
        if let intent = retryIntent, let notBefore {
            TimelineView(.periodic(from: .now, by: 1)) { timeline in
                Button(String(localized: "android.action_retry")) { Task { await perform(intent.action) } }
                    .disabled(isWriting || isLoading || scope.offline || timeline.date < notBefore)
                if timeline.date < notBefore {
                    Text(String(localized: "observation.actions.retry_at",
                                defaultValue: "Retry at \(notBefore.formatted(date: .omitted, time: .shortened))."))
                        .foregroundStyle(.secondary)
                }
            }
        }
    }

    private func load() async {
        let requestScope = scope
        let changed = lastScope != requestScope
        guard changed || (!isLoading && !isWriting) else { return }
        generation = UUID()
        let ticket = generation
        if changed {
            lastScope = requestScope
            tally = nil
            notice = nil
            retryIntent = nil
            notBefore = nil
            confirmed = false
            reported = false
            isWriting = false
        }
        loadError = nil
        isLoading = true
        defer { if generation == ticket { isLoading = false } }
        guard requestScope.origin == origin else {
            loadError = String(localized: "android.community_action_unavailable")
            return
        }
        do {
            let value: CommunityHelpfulnessTally
            if requestScope.accountId != nil {
                let client = viewModel.communityGateway
                guard !requestScope.offline else { throw SignedTransportError.offline }
                guard client.transport.origin == origin, let owner = client.transport.ownerScope(),
                      owner.hasPrefix("account:") else { throw CommunityWriteError.signInRequired }
                value = try await client.ownerHelpfulness(for: reference, ownerScope: owner)
            } else {
                value = try await viewModel.publicObservationHelpfulness(reference)
            }
            guard !Task.isCancelled, generation == ticket, scope == requestScope else { return }
            tally = value
        } catch {
            guard !Task.isCancelled, generation == ticket, scope == requestScope else { return }
            tally = nil
            loadError = Self.message(for: error)
        }
    }

    private func perform(_ action: CommunityObservationAction) async {
        let requestScope = scope
        guard requestScope == lastScope, requestScope.origin == origin, !requestScope.offline,
              !isLoading, !isWriting, requestScope.accountId != nil,
              let tally, tally.ownContribution != true,
              notBefore.map({ $0 <= Date() }) ?? true else { return }
        let client = viewModel.communityGateway
        guard client.transport.origin == origin, let owner = client.transport.ownerScope(),
              owner.hasPrefix("account:") else { return }
        let intent = CommunityObservationIntent.prepare(
            reference: reference, ownerScope: owner, origin: origin, action: action, retrying: retryIntent
        )
        let ticket = generation
        isWriting = true
        notice = nil
        defer { if generation == ticket { isWriting = false } }
        do {
            let receipt = try await client.submitObservationAction(intent)
            guard !Task.isCancelled, generation == ticket, scope == requestScope else { return }
            retryIntent = nil
            notBefore = nil
            switch receipt {
            case .confirmation:
                confirmed = true
                notice = String(localized: "android.community_vote_confirmed")
            case .dispute:
                reported = true
                notice = String(localized: "android.station_page_report_received")
            case .helpfulness(let updated):
                self.tally = updated
                notice = String(localized: "android.community_helpful_voted")
            case .removedHelpfulness(let updated):
                self.tally = updated
                notice = String(localized: "android.community_helpful_removed")
            }
        } catch {
            guard !Task.isCancelled, generation == ticket, scope == requestScope else { return }
            let deadline = CommunityObservationRetry.notBefore(error)
            retryIntent = deadline.map { _ in intent }
            notBefore = deadline
            notice = Self.message(for: error)
            if let refusal = error as? SignedTransportError, case .refused(_, let code?, _) = refusal {
                switch code {
                case "community.stale-revision", "community.ineligible-target", "community.not-found", "community.self-vote", "community.self-confirmation":
                    self.tally = nil
                case "community.already-confirmed":
                    confirmed = true
                default: break
                }
            }
        }
    }

    private static func message(for error: Error) -> String {
        if let refusal = error as? SignedTransportError, case .refused(_, let code?, _) = refusal {
            switch code {
            case "community.self-vote", "community.self-confirmation":
                return String(localized: "android.community_own_contribution")
            case "community.already-confirmed":
                return String(localized: "android.community_vote_replayed")
            case "community.stale-revision", "community.ineligible-target", "community.not-found":
                return String(localized: "android.community_action_unavailable")
            case "community.auth-required":
                return String(localized: "identity.sign_in_required")
            case "community.bad-body", "community.invalid", "community.invalid-vote", "community.bad-tally-filter":
                return String(localized: "observation.actions.invalid")
            case "community.conflict":
                return String(localized: "observation.actions.conflict")
            case "community.review-unavailable":
                return String(localized: "observation.actions.review_unavailable")
            default: break
            }
        }
        return IdentityCoordinator.describe(error)
    }
}

private extension CommunityHelpfulnessChoice {
    var label: String {
        switch self {
        case .helpful: return String(localized: "android.community_helpful_helpful")
        case .notHelpful: return String(localized: "android.community_helpful_not_helpful")
        }
    }
}

private extension CommunityDisputeReason {
    var label: String {
        switch self {
        case .priceChanged: return String(localized: "android.community_report_price_changed")
        case .wrongStation: return String(localized: "android.community_report_wrong_station")
        case .wrongProduct: return String(localized: "android.community_report_wrong_product")
        case .wrongCondition: return String(localized: "android.community_report_wrong_condition")
        case .evidenceMismatch: return String(localized: "android.community_report_evidence")
        case .other: return String(localized: "android.community_report_other")
        }
    }
}

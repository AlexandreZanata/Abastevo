import SwiftUI
import AnpFuelCore

public struct CommunitySafetyActions: View {
    @ObservedObject var model: CommunitySafetyViewModel
    let reference: CommunitySafetyReference
    let onSignIn: () -> Void

    @State private var report: CommunityReportTarget?
    @State private var confirmBlock = false
    @State private var blocking = false
    @State private var blockOperationId = UUID().uuidString.lowercased()
    @State private var failure: String?

    public init(model: CommunitySafetyViewModel, reference: CommunitySafetyReference, onSignIn: @escaping () -> Void) {
        self.model = model
        self.reference = reference
        self.onSignIn = onSignIn
    }

    public var body: some View {
        Menu {
            Button(contentReportLabel) {
                guard model.signedIn else { onSignIn(); return }
                report = reference.kind == .stationComment ? .station : .forum
            }
            Button(String(localized: "android.safety_report_user")) {
                guard model.signedIn else { onSignIn(); return }
                report = .user
            }
            Button(String(localized: "android.safety_block_user"), role: .destructive) {
                guard model.signedIn else { onSignIn(); return }
                confirmBlock = true
            }
        } label: {
            Label(String(localized: "android.safety_more_actions"), systemImage: "ellipsis")
                .labelStyle(.iconOnly)
                .frame(minWidth: 44, minHeight: 44)
        }
        .disabled(blocking || !reference.isWellFormed)
        .sheet(item: $report) { target in
            CommunityReportSheet(model: model, reference: reference, target: target, onSignIn: onSignIn)
        }
        .confirmationDialog(String(localized: "android.safety_block_title"), isPresented: $confirmBlock, titleVisibility: .visible) {
            Button(String(localized: "android.safety_block_confirm"), role: .destructive, action: block)
            Button(String(localized: "android.action_cancel"), role: .cancel) {}
        } message: {
            Text(String(localized: "android.safety_block_message"))
        }
        .alert(String(localized: "android.safety_block_user"), isPresented: hasFailure) {
            Button(String(localized: "android.feed_close"), role: .cancel) {}
        } message: {
            Text(failure ?? "")
        }
        .onChange(of: model.ownerGeneration) { _, _ in
            report = nil
            confirmBlock = false
            failure = nil
            blockOperationId = UUID().uuidString.lowercased()
        }
    }

    private var contentReportLabel: String {
        reference.kind == .forumPost
            ? String(localized: "android.safety_report_topic")
            : String(localized: "android.safety_report_comment")
    }

    private var hasFailure: Binding<Bool> {
        Binding(get: { failure != nil }, set: { if !$0 { failure = nil } })
    }

    private func block() {
        guard !blocking else { return }
        blocking = true
        let operationId = blockOperationId
        let owner = model.ownerGeneration
        Task {
            defer { blocking = false }
            do {
                try await model.block(reference, operationId: operationId)
                blockOperationId = UUID().uuidString.lowercased()
            } catch {
                guard model.ownerGeneration == owner else { return }
                let refusal = CommunitySafetyError.from(error)
                if refusal.needsSignIn { onSignIn() }
                else { failure = refusal.localizedMessage }
            }
        }
    }
}

private enum CommunityReportTarget: String, Identifiable {
    case user
    case forum
    case station

    var id: String { rawValue }

    var title: String {
        switch self {
        case .user: return String(localized: "android.safety_report_user_title")
        case .forum: return String(localized: "android.forum_report_dialog_title")
        case .station: return String(localized: "android.station_page_report_title")
        }
    }
}

private struct CommunityReportSheet: View {
    @Environment(\.dismiss) private var dismiss
    @ObservedObject var model: CommunitySafetyViewModel
    let reference: CommunitySafetyReference
    let target: CommunityReportTarget
    let onSignIn: () -> Void

    @State private var userReason = CommunityUserReportReason.spam
    @State private var forumReason = CommunityForumReportReason.spam
    @State private var stationReason = CommunityStationReportReason.spam
    @State private var detail = ""
    @State private var failure: String?
    @State private var submitting = false
    @State private var operationId = UUID().uuidString.lowercased()
    @State private var attempted: Draft?

    private enum Draft: Equatable {
        case user(CommunityUserReportReason, String?)
        case forum(CommunityForumReportReason, String?)
        case station(CommunityStationReportReason)
    }

    var body: some View {
        NavigationStack {
            Form {
                switch target {
                case .user:
                    Text(String(localized: "android.safety_report_user_prompt"))
                    Picker(String(localized: "android.forum_report_dialog_reason_prompt"), selection: $userReason) {
                        ForEach(CommunityUserReportReason.allCases, id: \.self) { reason in
                            Text(reason.label).tag(reason)
                        }
                    }
                    TextField(String(localized: "android.safety_report_detail_hint"), text: $detail, axis: .vertical)
                        .lineLimit(2...5)
                    let used = CommunitySafetyWire.normalizedUserDetail(detail)?.unicodeScalars.count ?? 0
                    let maximum = CommunitySafetyWire.maxUserDetailScalars
                    Text(String(localized: "android.safety_report_detail_counter", defaultValue: "\(used) / \(maximum)"))
                        .font(.caption)
                        .foregroundStyle(detailFits ? Color.secondary : AbastevoTheme.errorRed)
                case .forum:
                    Picker(String(localized: "android.forum_report_dialog_reason_prompt"), selection: $forumReason) {
                        ForEach(CommunityForumReportReason.allCases, id: \.self) { reason in
                            Text(reason.label).tag(reason)
                        }
                    }
                    TextField(String(localized: "android.forum_report_detail_hint"), text: $detail, axis: .vertical)
                        .lineLimit(2...5)
                case .station:
                    Text(String(localized: "android.station_page_report_copy"))
                    Picker(String(localized: "android.forum_report_dialog_reason_prompt"), selection: $stationReason) {
                        ForEach(CommunityStationReportReason.allCases, id: \.self) { reason in
                            Text(reason.label).tag(reason)
                        }
                    }
                }
                if let failure {
                    Text(failure).foregroundStyle(AbastevoTheme.errorRed)
                }
                if submitting { ProgressView() }
            }
            .disabled(submitting)
            .navigationTitle(target.title)
            .toolbar {
                ToolbarItem(placement: .cancellationAction) {
                    Button(String(localized: "android.action_cancel")) { dismiss() }
                        .disabled(submitting)
                }
                ToolbarItem(placement: .confirmationAction) {
                    Button(String(localized: "android.safety_report_send"), action: submit)
                        .disabled(submitting || !detailFits)
                }
            }
        }
        .interactiveDismissDisabled(submitting)
    }

    private var detailFits: Bool {
        switch target {
        case .user: return CommunitySafetyWire.userDetailFits(detail)
        case .forum: return CommunitySafetyWire.forumDetailFits(detail)
        case .station: return true
        }
    }

    private var draft: Draft {
        switch target {
        case .user: return .user(userReason, CommunitySafetyWire.normalizedUserDetail(detail))
        case .forum: return .forum(forumReason, CommunitySafetyWire.normalizedForumDetail(detail))
        case .station: return .station(stationReason)
        }
    }

    private func submit() {
        guard !submitting, detailFits else { return }
        let draft = draft
        if let attempted, attempted != draft { operationId = UUID().uuidString.lowercased() }
        attempted = draft
        submitting = true
        failure = nil
        let operationId = operationId
        let owner = model.ownerGeneration
        Task {
            defer { submitting = false }
            do {
                switch draft {
                case .user(let reason, let detail):
                    try await model.reportUser(reference, reason: reason, detail: detail, operationId: operationId)
                case .forum(let reason, let detail):
                    try await model.reportForum(reference, reason: reason, detail: detail, operationId: operationId)
                case .station(let reason):
                    try await model.reportStation(reference, reason: reason, operationId: operationId)
                }
                dismiss()
            } catch {
                guard model.ownerGeneration == owner else { return }
                let refusal = CommunitySafetyError.from(error)
                if refusal.needsSignIn {
                    dismiss()
                    onSignIn()
                } else {
                    failure = refusal.localizedMessage
                }
            }
        }
    }
}

public struct CommunitySafetyNoticeLine: View {
    @ObservedObject var model: CommunitySafetyViewModel

    public init(model: CommunitySafetyViewModel) {
        self.model = model
    }

    public var body: some View {
        if let notice = model.notice {
            Text(notice.label)
                .foregroundStyle(AbastevoTheme.accentLight)
                .font(.footnote)
        }
    }
}

struct CommunityBlockedUsersSection: View {
    @ObservedObject var model: CommunitySafetyViewModel
    let onSignIn: () -> Void

    @State private var pending: CommunityBlockedUser?
    @State private var error: String?
    @State private var operationId = UUID().uuidString.lowercased()
    @State private var attemptedBlockId: String?

    var body: some View {
        Section(String(localized: "android.blocked_users_title")) {
            switch model.blockList {
            case .signedOut:
                Button(String(localized: "android.blocked_users_sign_in"), action: onSignIn)
            case .idle, .loading:
                ProgressView()
            case .failed(let failure):
                Text(String(localized: "android.blocked_users_load_error"))
                    .foregroundStyle(.secondary)
                Button(String(localized: "android.action_retry")) {
                    if failure.needsSignIn { onSignIn() }
                    else { Task { await model.refreshBlockedUsers() } }
                }
            case .loaded(let users):
                if users.isEmpty {
                    Text(String(localized: "android.blocked_users_empty_title"))
                        .foregroundStyle(.secondary)
                }
                ForEach(users) { user in
                    VStack(alignment: .leading, spacing: 6) {
                        Text(user.displayLabel).font(.headline)
                        if let date = ISO8601DateFormatter().date(from: user.createdAt) {
                            let day = date.formatted(date: .abbreviated, time: .omitted)
                            Text(String(localized: "android.blocked_users_since", defaultValue: "Blocked on \(day)"))
                                .font(.caption).foregroundStyle(.secondary)
                        }
                        Button(String(localized: "android.blocked_users_unblock")) {
                            error = nil
                            pending = user
                        }
                        .disabled(model.removingBlock)
                    }
                }
            }
            if let error { Text(error).foregroundStyle(AbastevoTheme.errorRed) }
            if case .unblocked? = model.notice {
                Text(String(localized: "android.blocked_users_unblocked"))
                    .foregroundStyle(AbastevoTheme.accentLight)
            }
        }
        .task(id: model.ownerGeneration) { await model.refreshBlockedUsers() }
        .confirmationDialog(unblockTitle, isPresented: showsUnblock, titleVisibility: .visible, presenting: pending) { user in
            Button(String(localized: "android.blocked_users_unblock"), action: { unblock(user) })
            Button(String(localized: "android.action_cancel"), role: .cancel) {}
        } message: { _ in
            Text(String(localized: "android.blocked_users_unblock_message"))
        }
        .onChange(of: model.ownerGeneration) { _, _ in
            pending = nil
            error = nil
            attemptedBlockId = nil
        }
    }

    private var unblockTitle: String {
        guard let pending else { return String(localized: "android.blocked_users_unblock") }
        return String(localized: "android.blocked_users_unblock_title", defaultValue: "Unblock \(pending.displayLabel)?")
    }

    private var showsUnblock: Binding<Bool> {
        Binding(get: { pending != nil }, set: { if !$0 { pending = nil } })
    }

    private func unblock(_ user: CommunityBlockedUser) {
        if user.id != attemptedBlockId { operationId = UUID().uuidString.lowercased() }
        attemptedBlockId = user.id
        let operationId = operationId
        let owner = model.ownerGeneration
        Task {
            do {
                try await model.unblock(id: user.id, operationId: operationId)
                attemptedBlockId = nil
            } catch {
                guard model.ownerGeneration == owner else { return }
                let refusal = CommunitySafetyError.from(error)
                if refusal.needsSignIn { onSignIn() }
                else { self.error = refusal.localizedMessage }
            }
        }
    }
}

private extension CommunityBlockedUser {
    var displayLabel: String {
        if !label.isEmpty { return label }
        switch sourceKind {
        case .forumPost?, .forumComment?: return String(localized: "android.blocked_users_fallback_forum")
        case .stationComment?: return String(localized: "android.blocked_users_fallback_station")
        case nil: return String(localized: "android.blocked_users_fallback_generic")
        }
    }
}

private extension CommunitySafetyViewModel.Notice {
    var label: String {
        switch self {
        case .forumReport: return String(localized: "android.forum_report_success")
        case .stationReport: return String(localized: "android.station_page_report_received")
        case .userReport(true): return String(localized: "android.safety_report_user_replayed")
        case .userReport(false): return String(localized: "android.safety_report_user_received")
        case .blocked: return String(localized: "android.safety_block_done")
        case .unblocked: return String(localized: "android.blocked_users_unblocked")
        }
    }
}

private extension CommunitySafetyError {
    var needsSignIn: Bool {
        switch self {
        case .signInRequired, .refused(status: 401, code: _, retryAfterSeconds: _): return true
        default: return false
        }
    }

    var localizedMessage: String {
        switch self {
        case .offline:
            return String(localized: "android.safety_error_offline")
        case .refused(status: 400, code: "safety.self-target", retryAfterSeconds: _):
            return String(localized: "android.safety_error_self_target")
        case .refused(status: 404, code: _, retryAfterSeconds: _):
            return String(localized: "android.safety_error_not_found")
        case .refused(status: 409, code: "safety.block-limit", retryAfterSeconds: _):
            return String(localized: "android.safety_error_block_limit")
        case .refused(status: 429, code: _, retryAfterSeconds: let retryAfter):
            if let retryAfter {
                return String(localized: "android.safety_error_rate_limited", defaultValue: "Too many attempts. Try again in \(retryAfter) seconds.")
            }
            return String(localized: "safety.rate_limited")
        default:
            return String(localized: "android.safety_error_generic")
        }
    }
}

private extension CommunityUserReportReason {
    var label: String {
        switch self {
        case .spam: return String(localized: "android.safety_reason_spam")
        case .harassment: return String(localized: "android.safety_reason_harassment")
        case .hate: return String(localized: "android.safety_reason_hate")
        case .impersonation: return String(localized: "android.safety_reason_impersonation")
        case .illegal: return String(localized: "android.safety_reason_illegal")
        case .other: return String(localized: "android.safety_reason_other")
        }
    }
}

private extension CommunityForumReportReason {
    var label: String {
        switch self {
        case .spam: return String(localized: "android.forum_report_reason_spam")
        case .harassment: return String(localized: "android.forum_report_reason_harassment")
        case .defamation: return String(localized: "android.forum_report_reason_defamation")
        case .offTopic: return String(localized: "android.forum_report_reason_off_topic")
        case .illegal: return String(localized: "android.forum_report_reason_illegal")
        case .other: return String(localized: "android.forum_report_reason_other")
        }
    }
}

private extension CommunityStationReportReason {
    var label: String {
        switch self {
        case .spam: return String(localized: "android.station_page_report_spam")
        case .offensive: return String(localized: "android.station_page_report_abuse")
        case .misleading: return String(localized: "android.station_page_report_misleading")
        case .inappropriate: return String(localized: "android.station_page_report_other")
        }
    }
}

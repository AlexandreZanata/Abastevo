import SwiftUI
import AnpFuelCore

struct EnterpriseReturnChecklistBinding: Equatable {
    let partition: EnterprisePartition
    let membershipId: String
    let enrollmentId: String
    let tripId: String
    let assetId: String
    let generation: UUID
}

@MainActor
struct EnterpriseReturnChecklistView: View {
    @Environment(\.dismiss) private var dismiss
    @ObservedObject var enterprise: EnterpriseCoordinator
    let trip: EnterpriseTripRecord
    let onDone: (EnterpriseReturnChecklistCompletion?) -> Void
    @State private var loadedBinding: EnterpriseReturnChecklistBinding?
    @State private var loading = false
    @State private var readingRun = false
    @State private var message: String?
    @State private var runMessage: String?
    @State private var noteEdits: [Int: String] = [:]

    private var binding: EnterpriseReturnChecklistBinding? { enterprise.returnChecklistBinding(tripId: trip.id) }
    private var book: EnterpriseReturnChecklistBook? {
        guard let binding, loadedBinding == binding else { return nil }
        return enterprise.returnChecklistBook(binding)
    }
    private var draft: EnterpriseReturnChecklistDraft? {
        (book?.draft).flatMap { $0.matches(trip) ? $0 : nil }
    }
    private var queueItem: EnterpriseQueueItem? {
        guard let completion = draft?.completion else { return nil }
        return enterprise.snapshot?.queue.first { $0.id == completion.operationId }
    }
    private var acceptedRunID: String? { queueItem?.status == .accepted ? draft?.completion?.runId : nil }

    var body: some View {
        Form {
            if let binding, loadedBinding == binding {
                Section {
                    Text(trip.assetLabel).font(.headline)
                    if let template = draft?.applicable ?? book?.cached(assetId: trip.assetId) {
                        if let name = template.name { Text(name) }
                        let evaluated = Date(timeIntervalSince1970: Double(template.evaluatedAtMillis) / 1_000)
                            .formatted(.dateTime.year().month().day().hour().minute())
                        Text(String(localized: "enterprise.checklist.cached_on", defaultValue: "Company policy checked on \(evaluated)"))
                            .font(.caption).foregroundStyle(.secondary)
                    }
                    if loading { ProgressView(String(localized: "enterprise.checklist.loading")) }
                    if let message { Text(message).foregroundStyle(.secondary) }
                }
                if let draft, let completed = draft.completion {
                    completionSection(completed, binding: binding)
                } else if let draft {
                    TimelineView(.periodic(from: .now, by: 10)) { context in
                        if draft.isExpired(nowMillis: Int64(context.date.timeIntervalSince1970 * 1_000)) {
                            Text(String(localized: "android.enterprise_checklists_expired"))
                            Button(String(localized: "android.enterprise_checklists_start_again")) { start(binding) }
                        } else {
                            answerSections(draft, binding: binding)
                        }
                    }
                } else {
                    availability(binding: binding)
                }
            } else {
                Text(String(localized: "enterprise.context.choose_before_recording")).foregroundStyle(.secondary)
            }
        }
        .privacySensitive()
        .navigationTitle(String(localized: "enterprise.checklist.return_title"))
        .toolbar {
            ToolbarItem(placement: .cancellationAction) {
                Button(String(localized: "android.enterprise_trips_cancel")) { dismiss() }
            }
        }
        .task(id: binding) {
            loadedBinding = binding
            message = nil
            runMessage = nil
            loading = false
            readingRun = false
            noteEdits = [:]
            guard let binding else { return }
            await refresh(binding)
        }
        .task(id: acceptedRunID) {
            guard acceptedRunID != nil, let binding, let completion = draft?.completion else { return }
            await refreshRun(completion, binding: binding)
        }

    }

    private func availability(binding: EnterpriseReturnChecklistBinding) -> some View {
        Section {
            if let applicable = book?.cached(assetId: trip.assetId) {
                if applicable.required {
                    Button(String(localized: "android.enterprise_checklists_start")) { start(binding) }
                } else {
                    Text(String(localized: "android.enterprise_checklists_none_required"))
                    Button(String(localized: "enterprise.checklist.continue_finish")) {
                        guard self.binding == binding else { return }
                        onDone(nil)
                    }
                }
            }
            Button(String(localized: "android.action_retry")) { Task { await refresh(binding) } }
                .disabled(loading || enterprise.isOfflineProbe())
        }
    }

    @ViewBuilder
    private func answerSections(_ draft: EnterpriseReturnChecklistDraft, binding: EnterpriseReturnChecklistBinding) -> some View {
        ForEach(draft.applicable.items) { item in
            if draft.answers.indices.contains(item.position - 1), draft.notes.indices.contains(item.position - 1) {
                answerSection(item, draft: draft, binding: binding)
            }
        }
        Section {
            if draft.applicable.policyMode == "WARN" && hasCriticalIssue(draft) {
                Toggle(String(localized: "enterprise.checklist.acknowledged"), isOn: Binding(
                    get: { self.draft?.acknowledged == true }, set: { value in
                        present(enterprise.acknowledgeReturnChecklist(binding, draftId: draft.id, acknowledged: value))
                    }
                ))
            }
            Button(String(localized: "android.enterprise_checklists_complete")) {
                present(enterprise.completeReturnChecklist(binding, draftId: draft.id))
            }
            .buttonStyle(.borderedProminent)
            .tint(AbastevoTheme.ink)
            .disabled(!noteEdits.isEmpty)
        } footer: {
            Text(String(localized: "android.enterprise_checklists_company_rechecks"))
        }
    }

    private func answerSection(_ item: EnterpriseChecklistItem, draft: EnterpriseReturnChecklistDraft,
                               binding: EnterpriseReturnChecklistBinding) -> some View {
        let offset = item.position - 1
        let answer = draft.answers[offset]
        let note = draft.notes[offset]
        return Section(item.label) {
            if item.critical {
                Text(String(localized: "android.enterprise_checklists_critical")).foregroundStyle(AbastevoTheme.errorRed)
            }
            Picker(String(localized: "enterprise.checklist.answer"), selection: Binding(
                get: { answer }, set: { value in
                    let result = enterprise.updateReturnChecklist(binding, draftId: draft.id, position: item.position,
                                                                  answer: value, note: value == nil ? "" : note)
                    if case .success = result, value == nil { noteEdits[item.position] = nil }
                    present(result)
                }
            )) {
                Text(String(localized: "android.enterprise_checklists_unanswered")).tag(nil as EnterpriseChecklistAnswer?)
                Text(String(localized: "android.enterprise_checklists_answer_ok")).tag(Optional(EnterpriseChecklistAnswer.ok))
                Text(String(localized: "android.enterprise_checklists_answer_not_ok")).tag(Optional(EnterpriseChecklistAnswer.notOk))
                if item.optional {
                    Text(String(localized: "android.enterprise_checklists_answer_na")).tag(Optional(EnterpriseChecklistAnswer.notApplicable))
                }
            }
            .accessibilityLabel(item.label)
            if let answer {
                TextField(String(localized: "android.enterprise_checklists_note"), text: Binding(
                    get: { noteEdits[item.position] ?? note }, set: { value in
                        if value.unicodeScalars.count <= 140 { noteEdits[item.position] = value == note ? nil : value }
                    }
                ), axis: .vertical)
                .lineLimit(2...4)
                if let edited = noteEdits[item.position] {
                    Button(String(localized: "android.enterprise_checklists_save_note")) {
                        let normalized = edited.trimmingCharacters(in: .whitespacesAndNewlines)
                        let result = enterprise.updateReturnChecklist(binding, draftId: draft.id, position: item.position,
                                                                      answer: answer, note: normalized)
                        if case .success = result { noteEdits[item.position] = nil }
                        present(result)
                    }
                }
            }
            if item.photoOnNotOk && answer == .notOk {
                Text(String(localized: "enterprise.checklist.photo_unavailable"))
                    .font(.caption).foregroundStyle(AbastevoTheme.warningLight)
            }
        }
    }

    private func completionSection(_ completion: EnterpriseReturnChecklistCompletion, binding: EnterpriseReturnChecklistBinding) -> some View {
        Section {
            LabeledContent(String(localized: "android.enterprise_checklists_outcome_title"), value: outcome(completion.outcome))
            if let item = queueItem {
                Text(EnterpriseCoordinator.statusLabel(item)).foregroundStyle(.secondary)
            }
            if let result = book?.serverRun {
                LabeledContent(String(localized: "android.enterprise_checklists_server_title"), value: outcome(result.outcome))
                if result.outcomeDiffers { Text(String(localized: "enterprise.checklist.server_changed")) }
            } else {
                Text(String(localized: "android.enterprise_checklists_server_waiting")).foregroundStyle(.secondary)
            }
            if readingRun { ProgressView() }
            if let runMessage { Text(runMessage).foregroundStyle(.secondary) }
            if queueItem?.status == .rejected {
                Button(String(localized: "android.enterprise_checklists_start_again")) { start(binding) }
            }
            if queueItem?.status == .accepted {
                Button(String(localized: "android.action_retry")) { Task { await refreshRun(completion, binding: binding) } }
                    .disabled(readingRun || enterprise.isOfflineProbe())
            }
            Button(String(localized: "enterprise.checklist.continue_finish")) {
                guard self.binding == binding,
                      enterprise.returnChecklistCompletion(tripId: trip.id, runId: completion.runId) == completion else { return }
                onDone(completion)
            }
            .disabled(queueItem == nil || queueItem?.status == .rejected)
        }
    }

    private func refresh(_ binding: EnterpriseReturnChecklistBinding) async {
        guard loadedBinding == binding, !loading else { return }
        loading = true
        let result = await enterprise.refreshReturnChecklist(binding)
        guard loadedBinding == binding, self.binding == binding, !Task.isCancelled else { return }
        present(result)
        loading = false
    }

    private func refreshRun(_ completion: EnterpriseReturnChecklistCompletion, binding: EnterpriseReturnChecklistBinding) async {
        guard loadedBinding == binding, !readingRun else { return }
        readingRun = true
        let result = await enterprise.refreshReturnChecklistRun(binding, completion: completion)
        guard loadedBinding == binding, self.binding == binding, !Task.isCancelled else { return }
        if case .failure(let failure) = result { runMessage = failure.message } else { runMessage = nil }
        readingRun = false
    }

    private func start(_ binding: EnterpriseReturnChecklistBinding) {
        let result = enterprise.startReturnChecklist(binding)
        if case .success = result { noteEdits = [:] }
        present(result)
    }

    private func present(_ result: Result<Void, AppValidationError>) {
        if case .failure(let failure) = result { message = failure.message } else { message = nil }
    }

    private func hasCriticalIssue(_ draft: EnterpriseReturnChecklistDraft) -> Bool {
        zip(draft.applicable.items, draft.answers).contains { $0.0.critical && $0.1 == .notOk }
    }

    private func outcome(_ value: EnterpriseChecklistOutcome) -> String {
        switch value {
        case .completedOk: return String(localized: "android.enterprise_checklists_outcome_ok")
        case .completedWithIssues: return String(localized: "android.enterprise_checklists_outcome_issues")
        case .blocking: return String(localized: "android.enterprise_checklists_outcome_blocking")
        }
    }
}

import SwiftUI
import PhotosUI
import AnpFuelCore
#if canImport(UIKit)
import UIKit
#endif

/// Abastevo Empresas: invitations, company selection and the driver's company operations.
public struct EnterpriseHubSheetView: View {
    @Environment(\.dismiss) private var dismiss
    @ObservedObject var viewModel: AppStateViewModel

    public var body: some View {
        NavigationStack {
            EnterpriseHubContent(
                enterprise: viewModel.enterprise,
                signedIn: viewModel.currentAccountIdOrNil != nil,
                openSignIn: { viewModel.activeSheet = .auth }
            )
            .navigationTitle(String(localized: "enterprise.title"))
            .toolbar {
                ToolbarItem(placement: .cancellationAction) {
                    Button(String(localized: "android.feed_close")) { dismiss() }
                }
            }
        }
    }
}

struct EnterpriseHubContent: View {
    @ObservedObject var enterprise: EnterpriseCoordinator
    let signedIn: Bool
    let openSignIn: () -> Void

    var body: some View {
        List {
            Section {
                Label(String(localized: "android.enterprise_test_environment"), systemImage: "testtube.2")
                    .font(.subheadline.weight(.semibold))
                Text(String(localized: "enterprise.context.partition_notice"))
                    .font(.footnote)
                    .foregroundStyle(.secondary)
            }
            if !signedIn {
                Section {
                    Text(String(localized: "enterprise.context.sign_in"))
                    Button(String(localized: "android.enterprise_invitation_sign_in"), action: openSignIn)
                        .buttonStyle(.borderedProminent)
                        .tint(AbastevoTheme.ink)
                }
            } else if !enterprise.isOriginSupported {
                Section {
                    Text(String(localized: "enterprise.context.origin_unavailable"))
                }
            } else if let active = enterprise.active {
                EnterpriseCompanySections(enterprise: enterprise, active: active)
            } else {
                EnterpriseContextSections(enterprise: enterprise)
            }
            if let notice = enterprise.notice {
                Section(String(localized: "enterprise.notice")) {
                    Text(notice).font(.footnote)
                }
            }
        }
        .onAppear {
            enterprise.revalidate()
            if signedIn && enterprise.active == nil && !enterprise.contextsLoaded {
                enterprise.refreshContexts()
            }
        }
    }
}

// MARK: - Context selection

struct EnterpriseContextSections: View {
    @ObservedObject var enterprise: EnterpriseCoordinator

    var body: some View {
        Section {
            Text(String(localized: "android.enterprise_context_intro"))
                .font(.footnote)
                .foregroundStyle(.secondary)
            if enterprise.isBusy {
                ProgressView(String(localized: "android.enterprise_context_busy"))
            }
            ForEach(enterprise.contexts) { context in
                Button {
                    enterprise.select(context)
                } label: {
                    VStack(alignment: .leading, spacing: 2) {
                        Text(String(localized: "android.enterprise_context_select", defaultValue: "Enter \(context.name)")).font(.headline)
                        if let state = context.state, state != "ACTIVE" {
                            Text(EnterpriseLabels.organizationState(state))
                                .font(.caption)
                                .foregroundStyle(.secondary)
                        }
                    }
                }
                .disabled(enterprise.isBusy)
            }
            if enterprise.contextsLoaded && enterprise.contexts.isEmpty && !enterprise.isBusy {
                Text(String(localized: "android.enterprise_context_empty")).foregroundStyle(.secondary)
            }
            Button(String(localized: "android.enterprise_context_refresh")) {
                enterprise.refreshContexts()
            }
            .disabled(enterprise.isBusy)
            if let error = enterprise.accessError {
                Text(EnterpriseCoordinator.accessMessage(error))
                    .font(.footnote)
                    .foregroundStyle(AbastevoTheme.errorRed)
            }
        } header: {
            Text(String(localized: "android.enterprise_context_title"))
        } footer: {
            Text(String(localized: "enterprise.context.selection_notice"))
        }
        Section {
            NavigationLink {
                EnterpriseInvitationView(enterprise: enterprise)
            } label: {
                Label(String(localized: "android.enterprise_context_invitation"), systemImage: "envelope.open")
            }
        }
    }
}

// MARK: - Active company

struct EnterpriseCompanySections: View {
    @ObservedObject var enterprise: EnterpriseCoordinator
    let active: EnterpriseActiveCompany

    var body: some View {
        let snapshot = enterprise.snapshot
        let openTrip = snapshot?.openTrip
        Section {
            Text(String(localized: "android.enterprise_context_current", defaultValue: "You are in \(active.summary.name)")).font(.headline)
            if active.locked {
                Text(String(localized: "enterprise.context.locked"))
                    .font(.footnote)
                    .foregroundStyle(AbastevoTheme.errorRed)
            } else {
                Text(String(localized: "android.enterprise_context_confirmed")).font(.footnote).foregroundStyle(.secondary)
            }
            Button(String(localized: "android.enterprise_context_personal")) {
                enterprise.leaveCompany()
            }
            Button(String(localized: "android.enterprise_context_switch")) {
                enterprise.leaveCompany()
                enterprise.refreshContexts()
            }
        } header: {
            Text(String(localized: "android.enterprise_context_home"))
        } footer: {
            Text(String(localized: "android.enterprise_context_ownership"))
        }

        if !active.locked {
            Section {
                if let trip = openTrip {
                    VStack(alignment: .leading, spacing: 4) {
                        Text(String(localized: "enterprise.trip.in_progress")).font(.headline)
                        Text(trip.assetLabel)
                        Text(String(localized: "enterprise.trip.start_time", defaultValue: "Started \(EnterpriseLabels.dateTime(millis: trip.startedAtMillis))"))
                            .font(.caption)
                            .foregroundStyle(.secondary)
                    }
                    NavigationLink(String(localized: "android.enterprise_trips_finish")) {
                        EnterpriseTripFinishView(enterprise: enterprise, trip: trip)
                    }
                } else {
                    NavigationLink(String(localized: "android.enterprise_trips_start")) {
                        EnterpriseTripStartView(enterprise: enterprise)
                    }
                }
                NavigationLink(String(localized: "android.enterprise_expenses_new")) {
                    EnterpriseExpenseView(enterprise: enterprise)
                }
            } header: {
                Text(String(localized: "enterprise.operations"))
            } footer: {
                Text(String(localized: "enterprise.tracking_off"))
            }

            Section {
                if enterprise.isRefreshingAssets {
                    ProgressView(String(localized: "enterprise.assets.refreshing"))
                }
                if enterprise.snapshot?.reference?.forbidden == true {
                    Text(String(localized: "enterprise.assets.forbidden")).foregroundStyle(.secondary)
                } else if enterprise.assets.isEmpty {
                    Text(String(localized: "android.enterprise_assets_empty")).foregroundStyle(.secondary)
                }
                if !enterprise.assets.isEmpty && !enterprise.assetsConfirmed {
                    Text(String(localized: "android.enterprise_assets_not_confirmed"))
                        .font(.footnote)
                        .foregroundStyle(AbastevoTheme.warningLight)
                }
                ForEach(enterprise.assets) { asset in
                    VStack(alignment: .leading, spacing: 2) {
                        Text(EnterpriseCoordinator.assetLabel(asset)).font(.subheadline.weight(.semibold))
                        Text(EnterpriseLabels.assetStatus(asset, nowMillis: EnterpriseLabels.nowMillis()))
                            .font(.caption)
                            .foregroundStyle(.secondary)
                    }
                }
                Button(String(localized: "android.enterprise_assets_refresh")) {
                    enterprise.refreshAssets()
                }
                .disabled(enterprise.isRefreshingAssets)
            } header: {
                Text(String(localized: "android.enterprise_assets_title"))
            }
        }

        Section {
            let unsent = snapshot?.unsentCount ?? 0
            if enterprise.isDelivering {
                ProgressView(String(localized: "enterprise.queue.sending"))
            }
            Text(unsent == 0
                 ? String(localized: "enterprise.queue.no_pending")
                 : String(localized: "enterprise.queue.pending_count", defaultValue: "Records waiting to be sent: \(unsent.formatted())."))
            NavigationLink(String(localized: "enterprise.queue.title")) {
                EnterpriseQueueView(enterprise: enterprise)
            }
            Button(String(localized: "android.enterprise_trips_send_now")) {
                enterprise.deliverNow()
            }
            .disabled(enterprise.isDelivering)
        } header: {
            Text(String(localized: "android.enterprise_sync_title"))
        } footer: {
            Text(String(localized: "enterprise.queue.offline_notice"))
        }
    }
}

// MARK: - Invitation

struct EnterpriseInvitationView: View {
    @ObservedObject var enterprise: EnterpriseCoordinator
    @Environment(\.dismiss) private var dismiss
    @State private var code = ""
    @State private var consented = false

    var body: some View {
        Form {
            if let result = enterprise.invitationResult {
                Section {
                    if result.isActive {
                        Text(String(localized: "android.enterprise_invitation_active"))
                    } else if result.awaitsConfirmation {
                        Text(String(localized: "android.enterprise_invitation_pending"))
                    } else {
                        Text(String(localized: "android.enterprise_invitation_declined"))
                    }
                    Button(String(localized: "android.enterprise_context_my_companies")) {
                        enterprise.restartInvitation()
                        enterprise.refreshContexts()
                        dismiss()
                    }
                    Button(String(localized: "android.enterprise_invitation_restart")) {
                        enterprise.restartInvitation()
                        code = ""
                        consented = false
                    }
                }
            } else if let preview = enterprise.invitationPreview {
                Section {
                    Text(preview.organizationName).font(.title3.weight(.semibold))
                    Text(preview.legalEntityVerified ? String(localized: "android.enterprise_invitation_verified") : String(localized: "android.enterprise_invitation_unverified"))
                        .foregroundStyle(.secondary)
                } header: {
                    Text(String(localized: "android.enterprise_invitation_title"))
                }
                Section(String(localized: "android.enterprise_invitation_roles")) {
                    ForEach(preview.grants, id: \.self) { grant in
                        let role = EnterpriseCoordinator.roleLabel(grant.role)
                        let scope = grant.scope == "ORG"
                            ? String(localized: "android.enterprise_scope_organization")
                            : String(localized: "android.enterprise_scope_self")
                        Text(String(localized: "android.enterprise_invitation_role_scope", defaultValue: "\(role) · \(scope)"))
                    }
                }
                Section {
                    Text(String(localized: "android.enterprise_invitation_privacy"))
                        .font(.footnote)
                    if preview.confirmationRequired {
                        Text(String(localized: "android.enterprise_invitation_confirmation_required"))
                            .font(.footnote)
                    }
                    Toggle(String(localized: "android.enterprise_invitation_consent"), isOn: $consented)
                    Button(enterprise.invitationRetryAccept == true ? String(localized: "android.enterprise_invitation_retry") : String(localized: "android.enterprise_invitation_accept")) {
                        enterprise.decideInvitation(accept: true)
                    }
                    .buttonStyle(.borderedProminent)
                    .tint(AbastevoTheme.ink)
                    .disabled(!consented || enterprise.invitationBusy || enterprise.invitationRetryAccept == false)
                    Button(enterprise.invitationRetryAccept == false ? String(localized: "android.enterprise_invitation_retry") : String(localized: "android.enterprise_invitation_decline")) {
                        enterprise.decideInvitation(accept: false)
                    }
                    .disabled(enterprise.invitationBusy || enterprise.invitationRetryAccept == true)
                    Button(String(localized: "android.enterprise_invitation_restart")) {
                        enterprise.restartInvitation()
                        consented = false
                    }
                    .disabled(enterprise.invitationBusy)
                }
            } else {
                Section {
                    Text(String(localized: "android.enterprise_invitation_intro"))
                    TextField(String(localized: "android.enterprise_invitation_code_label"), text: $code)
                        .textInputAutocapitalization(.never)
                        .autocorrectionDisabled(true)
                    Button(String(localized: "android.enterprise_invitation_preview")) {
                        enterprise.previewInvitation(code: code)
                    }
                    .buttonStyle(.borderedProminent)
                    .tint(AbastevoTheme.ink)
                    .disabled(code.trimmingCharacters(in: .whitespacesAndNewlines).isEmpty || enterprise.invitationBusy)
                } footer: {
                    Text(String(localized: "android.enterprise_invitation_offline"))
                }
            }
            if enterprise.invitationBusy {
                Section {
                    ProgressView(String(localized: "android.enterprise_invitation_busy"))
                }
            }
            if let error = enterprise.invitationError {
                Section {
                    Text(EnterpriseCoordinator.accessMessage(error)).foregroundStyle(AbastevoTheme.errorRed)
                }
            }
        }
        .navigationTitle(String(localized: "android.enterprise_invitation_title"))
    }
}

// MARK: - Trips

/// Reading input shared by the trip start and finish: a meter reading or a written reason.
struct EnterpriseEvidenceFields: View {
    let meters: [EnterpriseReferenceMeter]
    @Binding var meterId: String?
    @Binding var valueText: String
    @Binding var reasonText: String

    var body: some View {
        Picker(String(localized: "enterprise.meter.reading"), selection: $meterId) {
            Text(String(localized: "enterprise.meter.no_reading")).tag(nil as String?)
            ForEach(meters) { meter in
                Text(meter.kind == .odometer ? String(localized: "android.enterprise_trips_meter_odometer") : String(localized: "android.enterprise_trips_meter_hours")).tag(Optional(meter.id))
            }
        }
        if let selected = meters.first(where: { $0.id == meterId }) {
            let odometerExample = (Decimal(1_843_205) / 10)
                .formatted(.number.grouping(.never).precision(.fractionLength(1)))
            let hoursExample = (Decimal(360_025) / 100)
                .formatted(.number.grouping(.never).precision(.fractionLength(2)))
            TextField(selected.kind == .odometer ? String(localized: "enterprise.meter.km_example", defaultValue: "Reading in km, e.g. \(odometerExample)") : String(localized: "enterprise.meter.hours_example", defaultValue: "Reading in hours, e.g. \(hoursExample)"), text: $valueText)
                .keyboardType(.decimalPad)
        } else {
            TextField(String(localized: "android.enterprise_trips_absence_reason"), text: $reasonText, axis: .vertical)
        }
    }

    /// The evidence, or nil with the message to show.
    static func evidence(meters: [EnterpriseReferenceMeter], meterId: String?, valueText: String, reasonText: String) -> (EnterpriseMeterEvidence?, String?) {
        if let meter = meters.first(where: { $0.id == meterId }) {
            let parsedValue = meter.kind == .odometer ? EnterpriseValues.parseKilometres(valueText) : EnterpriseValues.parseHours(valueText)
            guard let value = parsedValue else {
                return (nil, meter.kind == .odometer
                    ? String(localized: "enterprise.meter.km_invalid")
                    : String(localized: "enterprise.meter.hours_invalid"))
            }
            if let modulus = meter.modulus, value >= modulus {
                return (nil, String(localized: "enterprise.meter.over_limit"))
            }
            return (.reading(meterId: meter.id, segmentId: meter.segmentId, kind: meter.kind, readingId: EnterpriseValues.newId(),
                             value: value, modulus: meter.modulus), nil)
        }
        let reason = reasonText.trimmingCharacters(in: .whitespacesAndNewlines)
        guard EnterpriseValues.isValidText(reason, maxScalars: EnterpriseValues.maxReasonScalars) else {
            return (nil, String(localized: "enterprise.meter.reason_invalid"))
        }
        return (.missing(reason: reason), nil)
    }
}

struct EnterpriseTripStartView: View {
    @ObservedObject var enterprise: EnterpriseCoordinator
    @Environment(\.dismiss) private var dismiss
    @State private var assetId: String?
    @State private var meterId: String?
    @State private var valueText = ""
    @State private var reasonText = ""
    @State private var reviewAccepted = false
    @State private var applicable: EnterpriseChecklistApplicable?
    @State private var checklistMessage: String?
    @State private var checklistConsulted = false
    @State private var checklistFailed = false
    @State private var skipChecklist = false
    @State private var departureRunId: String?
    @State private var blocking = false
    @State private var showChecklist = false
    @State private var checklistStartedAt = ""
    @State private var loading = false
    @State private var error: String?

    private var asset: EnterpriseReferenceAsset? {
        assetId.flatMap { enterprise.findAsset($0) }
    }

    private var needsReview: Bool {
        guard let asset else { return false }
        return !asset.isActive || asset.assignmentEnded(nowMillis: EnterpriseLabels.nowMillis()) || !enterprise.assetsConfirmed
    }

    private var checklistReady: Bool {
        if blocking { return false }
        if departureRunId != nil { return true }
        if checklistConsulted && applicable?.required == false { return true }
        return checklistFailed && skipChecklist
    }

    var body: some View {
        Form {
            Section(String(localized: "enterprise.assets.asset")) {
                Picker(String(localized: "enterprise.assets.asset"), selection: $assetId) {
                    Text(String(localized: "android.enterprise_assets_pick_title")).tag(nil as String?)
                    ForEach(enterprise.assets) { item in
                        Text(EnterpriseCoordinator.assetLabel(item)).tag(Optional(item.id))
                    }
                }
                .onChange(of: assetId) { _, _ in
                    meterId = asset?.meters.first?.id
                    resetChecklist()
                }
                if let asset, needsReview {
                    Text(EnterpriseLabels.assetStatus(asset, nowMillis: EnterpriseLabels.nowMillis()))
                        .font(.footnote)
                        .foregroundStyle(AbastevoTheme.warningLight)
                    Toggle(String(localized: "enterprise.assets.acknowledge"), isOn: $reviewAccepted)
                }
            }
            if let asset {
                Section(String(localized: "android.enterprise_checklists_departure_title")) {
                    if loading {
                        ProgressView(String(localized: "enterprise.checklist.loading"))
                    } else if departureRunId != nil {
                        Text(blocking ? String(localized: "enterprise.checklist.trip_blocked") : String(localized: "android.enterprise_checklists_saved"))
                            .foregroundStyle(blocking ? AbastevoTheme.errorRed : .primary)
                    } else if checklistConsulted && applicable?.required == false {
                        Text(String(localized: "android.enterprise_checklists_none_required")).foregroundStyle(.secondary)
                    } else {
                        Button(String(localized: "enterprise.checklist.read_departure")) {
                            consultChecklist(asset.id)
                        }
                    }
                    if let checklistMessage {
                        Text(checklistMessage).font(.footnote).foregroundStyle(.secondary)
                    }
                    if checklistFailed {
                        Toggle(String(localized: "enterprise.checklist.skip"), isOn: $skipChecklist)
                    }
                }
                Section(String(localized: "android.enterprise_trips_departure_value")) {
                    EnterpriseEvidenceFields(meters: asset.meters, meterId: $meterId, valueText: $valueText, reasonText: $reasonText)
                }
            }
            Section {
                Button(String(localized: "android.enterprise_trips_start"), action: start)
                    .buttonStyle(.borderedProminent)
                    .tint(AbastevoTheme.accentLight)
                    .disabled(asset == nil || (needsReview && !reviewAccepted) || !checklistReady)
                if let error {
                    Text(error).font(.footnote).foregroundStyle(AbastevoTheme.errorRed)
                }
            } footer: {
                Text(String(localized: "enterprise.trip.device_notice"))
            }
        }
        .navigationTitle(String(localized: "android.enterprise_trips_start"))
        .sheet(isPresented: $showChecklist) {
            if let applicable {
                NavigationStack {
                    EnterpriseChecklistView(enterprise: enterprise, applicable: applicable, startedAt: checklistStartedAt) { runId, outcome in
                        departureRunId = runId
                        blocking = outcome == .blocking
                        showChecklist = false
                    }
                }
            }
        }
    }

    private func resetChecklist() {
        applicable = nil
        checklistMessage = nil
        checklistConsulted = false
        checklistFailed = false
        skipChecklist = false
        departureRunId = nil
        blocking = false
        reviewAccepted = false
    }

    private func consultChecklist(_ id: String) {
        loading = true
        checklistMessage = nil
        Task {
            let result = await enterprise.loadChecklist(assetId: id, context: "DEPARTURE")
            loading = false
            switch result {
            case .success(let value):
                applicable = value
                checklistConsulted = true
                checklistFailed = false
                if value.required {
                    checklistStartedAt = EnterpriseValues.formatInstant(epochMillis: EnterpriseLabels.nowMillis())
                    showChecklist = true
                }
            case .failure(let failure):
                checklistFailed = true
                checklistMessage = failure.message
            }
        }
    }

    private func start() {
        guard let asset else { return }
        let parsed = EnterpriseEvidenceFields.evidence(meters: asset.meters, meterId: meterId, valueText: valueText, reasonText: reasonText)
        guard let evidence = parsed.0 else {
            error = parsed.1
            return
        }
        switch enterprise.startTrip(assetId: asset.id, evidence: evidence, departureRunId: departureRunId) {
        case .success:
            dismiss()
        case .failure(let failure):
            error = failure.message
        }
    }
}

struct EnterpriseTripFinishView: View {
    @ObservedObject var enterprise: EnterpriseCoordinator
    let trip: EnterpriseTripRecord
    @Environment(\.dismiss) private var dismiss
    @State private var meterId: String?
    @State private var valueText = ""
    @State private var reasonText = ""
    @State private var error: String?

    private var meters: [EnterpriseReferenceMeter] {
        enterprise.findAsset(trip.assetId)?.meters ?? []
    }

    var body: some View {
        Form {
            Section(String(localized: "enterprise.trip.title")) {
                Text(trip.assetLabel).font(.headline)
                Text(String(localized: "enterprise.trip.start_time", defaultValue: "Started \(EnterpriseLabels.dateTime(millis: trip.startedAtMillis))")).foregroundStyle(.secondary)
            }
            Section(String(localized: "android.enterprise_trips_final_value")) {
                EnterpriseEvidenceFields(meters: meters, meterId: $meterId, valueText: $valueText, reasonText: $reasonText)
            }
            Section {
                Button(String(localized: "android.enterprise_trips_finish"), action: finish)
                    .buttonStyle(.borderedProminent)
                    .tint(AbastevoTheme.ink)
                if let error {
                    Text(error).font(.footnote).foregroundStyle(AbastevoTheme.errorRed)
                }
            }
        }
        .navigationTitle(String(localized: "android.enterprise_trips_finish"))
        .onAppear {
            if meterId == nil {
                meterId = trip.startMeterId ?? meters.first?.id
            }
        }
    }

    private func finish() {
        let parsed = EnterpriseEvidenceFields.evidence(meters: meters, meterId: meterId, valueText: valueText, reasonText: reasonText)
        guard let evidence = parsed.0 else {
            error = parsed.1
            return
        }
        switch enterprise.finishTrip(evidence: evidence) {
        case .success:
            dismiss()
        case .failure(let failure):
            error = failure.message
        }
    }
}

// MARK: - Checklist

struct EnterpriseChecklistView: View {
    @ObservedObject var enterprise: EnterpriseCoordinator
    let applicable: EnterpriseChecklistApplicable
    let startedAt: String
    let onDone: (String, EnterpriseChecklistOutcome) -> Void
    @Environment(\.dismiss) private var dismiss
    @State private var answers: [EnterpriseChecklistAnswer?]
    @State private var notes: [String]
    @State private var acknowledged = false
    @State private var error: String?

    init(enterprise: EnterpriseCoordinator, applicable: EnterpriseChecklistApplicable, startedAt: String,
         onDone: @escaping (String, EnterpriseChecklistOutcome) -> Void) {
        self.enterprise = enterprise
        self.applicable = applicable
        self.startedAt = startedAt
        self.onDone = onDone
        _answers = State(initialValue: Array(repeating: nil, count: applicable.items.count))
        _notes = State(initialValue: Array(repeating: "", count: applicable.items.count))
    }

    private var hasCriticalIssue: Bool {
        applicable.items.indices.contains { index in
            applicable.items[index].critical && answers[index] == EnterpriseChecklistAnswer.notOk
        }
    }

    var body: some View {
        Form {
            Section {
                Text(applicable.name ?? "").font(.headline)
                let blocksDeparture = applicable.policyMode == "BLOCK"
                Text(blocksDeparture
                     ? String(localized: "enterprise.checklist.critical_blocked")
                     : String(localized: "enterprise.checklist.critical_required"))
                    .font(.footnote)
                    .foregroundStyle(.secondary)
            }
            ForEach(applicable.items) { item in
                Section {
                    Text(item.label)
                    if item.critical {
                        Text(String(localized: "android.enterprise_checklists_critical")).font(.caption).foregroundStyle(AbastevoTheme.errorRed)
                    }
                    Picker(String(localized: "enterprise.checklist.answer"), selection: answerBinding(item.position - 1)) {
                        Text(String(localized: "android.enterprise_checklists_unanswered")).tag(nil as EnterpriseChecklistAnswer?)
                        Text(String(localized: "android.enterprise_checklists_answer_ok")).tag(Optional(EnterpriseChecklistAnswer.ok))
                        Text(String(localized: "android.enterprise_checklists_answer_not_ok")).tag(Optional(EnterpriseChecklistAnswer.notOk))
                        if item.optional {
                            Text(String(localized: "android.enterprise_checklists_answer_na")).tag(Optional(EnterpriseChecklistAnswer.notApplicable))
                        }
                    }
                    if answers[item.position - 1] == EnterpriseChecklistAnswer.notOk {
                        TextField(String(localized: "android.enterprise_checklists_note"), text: noteBinding(item.position - 1))
                        if item.photoOnNotOk {
                            Text(String(localized: "enterprise.checklist.photo_unavailable"))
                                .font(.caption)
                                .foregroundStyle(AbastevoTheme.warningLight)
                        }
                    }
                }
            }
            Section {
                if hasCriticalIssue && applicable.policyMode == "WARN" {
                    Toggle(String(localized: "enterprise.checklist.acknowledged"), isOn: $acknowledged)
                }
                Button(String(localized: "android.enterprise_checklists_complete"), action: submit)
                    .buttonStyle(.borderedProminent)
                    .tint(AbastevoTheme.ink)
                if let error {
                    Text(error).font(.footnote).foregroundStyle(AbastevoTheme.errorRed)
                }
            } footer: {
                Text(String(localized: "android.enterprise_checklists_company_rechecks"))
            }
        }
        .navigationTitle(String(localized: "android.enterprise_checklists_departure_title"))
        .toolbar {
            ToolbarItem(placement: .cancellationAction) {
                Button(String(localized: "android.enterprise_trips_cancel")) { dismiss() }
            }
        }
    }

    private func answerBinding(_ index: Int) -> Binding<EnterpriseChecklistAnswer?> {
        Binding(
            get: { index < answers.count ? answers[index] : nil },
            set: { value in
                if index < answers.count { answers[index] = value }
            }
        )
    }

    private func noteBinding(_ index: Int) -> Binding<String> {
        Binding(
            get: { index < notes.count ? notes[index] : "" },
            set: { value in
                if index < notes.count { notes[index] = value }
            }
        )
    }

    private func submit() {
        switch enterprise.completeChecklist(applicable, answers: answers, notes: notes, acknowledged: acknowledged, startedAt: startedAt) {
        case .success(let value):
            onDone(value.runId, value.outcome)
        case .failure(let failure):
            error = failure.message
        }
    }
}

// MARK: - Expenses

struct EnterpriseExpenseView: View {
    @ObservedObject var enterprise: EnterpriseCoordinator
    @Environment(\.dismiss) private var dismiss
    @State private var assetId: String?
    @State private var category = "TOLL"
    @State private var paidText = ""
    @State private var companyText = ""
    @State private var payer: EnterpriseExpensePayer = .company
    @State private var receipt: Data?
    @State private var pickerItem: PhotosPickerItem?
    @State private var preparing = false
    @State private var showCamera = false
    @State private var error: String?

    var body: some View {
        Form {
            Section(String(localized: "android.ledger_entry_add_expense")) {
                Picker(String(localized: "enterprise.assets.asset"), selection: $assetId) {
                    Text(String(localized: "android.enterprise_assets_pick_title")).tag(nil as String?)
                    ForEach(enterprise.assets) { item in
                        Text(EnterpriseCoordinator.assetLabel(item)).tag(Optional(item.id))
                    }
                }
                Picker(String(localized: "android.forum_topic_category_label"), selection: $category) {
                    ForEach(EnterpriseOperationBuilder.expenseCategories, id: \.self) { code in
                        Text(EnterpriseCoordinator.categoryLabel(code)).tag(code)
                    }
                }
                TextField(String(localized: "android.enterprise_expenses_paid"), text: $paidText)
                    .keyboardType(.decimalPad)
                TextField(String(localized: "enterprise.expense.company_amount"), text: $companyText)
                    .keyboardType(.decimalPad)
                Picker(String(localized: "android.enterprise_expenses_payer"), selection: $payer) {
                    Text(String(localized: "android.enterprise_scope_organization")).tag(EnterpriseExpensePayer.company)
                    Text(String(localized: "enterprise.expense.paid_employee")).tag(EnterpriseExpensePayer.employee)
                }
            }
            Section {
                if preparing {
                    ProgressView(String(localized: "enterprise.receipt.preparing"))
                } else if let receipt {
                    let size = ByteCountFormatStyle(style: .file).format(Int64(receipt.count))
                    #if canImport(UIKit)
                    if let image = UIImage(data: receipt) {
                        Image(uiImage: image)
                            .resizable()
                            .scaledToFit()
                            .frame(maxHeight: 180)
                    }
                    #endif
                    Text(String(localized: "enterprise.receipt.ready", defaultValue: "Photo ready (\(size)). Location and metadata removed."))
                        .font(.caption)
                        .foregroundStyle(.secondary)
                    Button(String(localized: "android.vehicle_remove_photo"), role: .destructive) {
                        self.receipt = nil
                    }
                } else {
                    PhotosPicker(selection: $pickerItem, matching: .images) {
                        Label(String(localized: "enterprise.receipt.choose_photo"), systemImage: "photo")
                    }
                    #if canImport(UIKit)
                    Button {
                        showCamera = true
                    } label: {
                        Label(String(localized: "enterprise.receipt.take_photo"), systemImage: "camera")
                    }
                    #endif
                }
            } header: {
                Text(String(localized: "android.enterprise_fuelings_receipt_optional"))
            } footer: {
                Text(String(localized: "enterprise.receipt.sent_after_expense"))
            }
            Section {
                Button(String(localized: "android.enterprise_expenses_save"), action: submit)
                    .buttonStyle(.borderedProminent)
                    .tint(AbastevoTheme.ink)
                    .disabled(assetId == nil || preparing)
                if let error {
                    Text(error).font(.footnote).foregroundStyle(AbastevoTheme.errorRed)
                }
            }
        }
        .navigationTitle(String(localized: "android.enterprise_expenses_new"))
        .onChange(of: pickerItem) { _, newItem in
            guard let item = newItem else { return }
            preparing = true
            Task {
                let data = try? await item.loadTransferable(type: Data.self)
                await prepare(data)
                pickerItem = nil
            }
        }
        #if canImport(UIKit)
        .sheet(isPresented: $showCamera) {
            IosCameraCaptureView { data in
                Task { await prepare(data) }
            }
        }
        #endif
    }

    private func prepare(_ data: Data?) async {
        preparing = true
        guard let data else {
            preparing = false
            error = String(localized: "enterprise.receipt.read_failed")
            return
        }
        let encoded = await EnterpriseReceiptEncoder.prepare(data)
        preparing = false
        if let encoded {
            receipt = encoded
            error = nil
        } else {
            error = String(localized: "enterprise.receipt.prepare_failed")
        }
    }

    private func submit() {
        guard let assetId else { return }
        switch enterprise.submitExpense(assetId: assetId, category: category, paidText: paidText, companyText: companyText,
                                        payer: payer, receipt: receipt) {
        case .success:
            dismiss()
        case .failure(let failure):
            error = failure.message
        }
    }
}

/// Re-encodes a receipt as a metadata-free JPEG (no EXIF or GPS) within the 5 MiB enterprise bound.
enum EnterpriseReceiptEncoder {
    static func prepare(_ data: Data) async -> Data? {
        return await Task.detached(priority: .userInitiated) { () -> Data? in
            let codec = IosImageIoPhotoCodec()
            guard let dims = codec.probeDims(bytes: data) else { return nil }
            let sample = PortablePhoto.sampleSizeForBounds(width: dims.width, height: dims.height)
            for attempt in 1...PortablePhoto.maxAttempts {
                if let encoded = codec.encode(source: data, request: PhotoEncodeRequest(sampleSize: sample, attempt: attempt)),
                   !encoded.isEmpty, encoded.count <= EnterpriseAttachmentWire.maxBytes {
                    return encoded
                }
            }
            return nil
        }.value
    }
}

// MARK: - Queue

struct EnterpriseQueueView: View {
    @ObservedObject var enterprise: EnterpriseCoordinator

    var body: some View {
        let snapshot = enterprise.snapshot
        List {
            Section(String(localized: "enterprise.queue.records")) {
                let items = (snapshot?.queue ?? []).reversed()
                if items.isEmpty {
                    Text(String(localized: "enterprise.queue.empty")).foregroundStyle(.secondary)
                }
                ForEach(Array(items)) { item in
                    VStack(alignment: .leading, spacing: 4) {
                        Text(item.summary).font(.subheadline.weight(.semibold))
                        Text(EnterpriseCoordinator.statusLabel(item)).font(.caption)
                        Text(EnterpriseLabels.dateTime(millis: item.createdAtMillis)).font(.caption2).foregroundStyle(.secondary)
                        if item.status == .exhausted || (item.status == .pending && item.attempts > 0) {
                            Button(String(localized: "enterprise.queue.retry_now")) { enterprise.retryNow(item.id) }
                                .font(.caption)
                        }
                        if item.status.isFinal {
                            Button(String(localized: "enterprise.queue.dismiss")) { enterprise.dismiss(item.id) }
                                .font(.caption)
                        }
                    }
                }
            }
            let attachments = snapshot?.attachments ?? []
            if !attachments.isEmpty {
                Section(String(localized: "enterprise.queue.receipts")) {
                    ForEach(attachments) { item in
                        VStack(alignment: .leading, spacing: 4) {
                            Text(EnterpriseCoordinator.attachmentLabel(item)).font(.subheadline)
                            if item.status == .exhausted {
                                Button(String(localized: "enterprise.queue.retry_now")) { enterprise.retryNow(item.id) }.font(.caption)
                            }
                            if item.status == .delivered || item.status == .refused {
                                Button(String(localized: "enterprise.queue.dismiss")) { enterprise.dismiss(item.id) }.font(.caption)
                            }
                        }
                    }
                }
            }
            let refusedTrips = (snapshot?.trips ?? []).filter { trip in
                snapshot?.queue.first(where: { $0.id == trip.startOperationId })?.status == .rejected
            }
            if !refusedTrips.isEmpty {
                Section(String(localized: "enterprise.queue.rejected_trips")) {
                    ForEach(refusedTrips) { trip in
                        HStack {
                            Text(trip.assetLabel)
                            Spacer()
                            Button(String(localized: "enterprise.queue.remove")) { enterprise.dismissRefusedTrip(trip.id) }
                        }
                    }
                }
            }
        }
        .navigationTitle(String(localized: "enterprise.queue.title"))
        .refreshable {
            enterprise.deliverNow()
        }
    }
}

// MARK: - Labels

enum EnterpriseLabels {
    static func nowMillis() -> Int64 {
        Int64(Date().timeIntervalSince1970 * 1000)
    }

    static func dateTime(millis: Int64) -> String {
        let date = Date(timeIntervalSince1970: TimeInterval(millis) / 1000)
        return date.formatted(date: .abbreviated, time: .shortened)
    }

    static func organizationState(_ state: String) -> String {
        switch state {
        case "SUSPENDED": return String(localized: "enterprise.context.suspended")
        case "ARCHIVED": return String(localized: "enterprise.context.archived")
        case "SETUP", "DRAFT": return String(localized: "enterprise.context.setup")
        default: return state
        }
    }

    static func assetStatus(_ asset: EnterpriseReferenceAsset, nowMillis: Int64) -> String {
        if !asset.isActive {
            return String(localized: "enterprise.assets.currently_unavailable", defaultValue: "Asset unavailable now (\(asset.state))")
        }
        if asset.assignmentEnded(nowMillis: nowMillis) {
            return String(localized: "enterprise.assets.assignment_ended")
        }
        switch asset.assignmentKind {
        case "TEMPORARY": return String(localized: "android.enterprise_assets_assignment_temporary")
        case "ADDITIONAL": return String(localized: "android.enterprise_assets_assignment_additional")
        default: return String(localized: "android.enterprise_assets_assignment_primary")
        }
    }
}

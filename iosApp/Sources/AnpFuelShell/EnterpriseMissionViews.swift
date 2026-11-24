import AnpFuelCore
import SwiftUI

@MainActor
struct EnterpriseMissionInboxView: View {
    @ObservedObject var enterprise: EnterpriseCoordinator

    var body: some View {
        List {
            Section {
                if enterprise.missionsLoaded {
                    ageNotice
                    if enterprise.missionsTruncated {
                        Text(String(localized: "android.enterprise_missions_truncated")).foregroundStyle(.secondary)
                    }
                } else {
                    Text(String(localized: "android.enterprise_missions_never")).foregroundStyle(.secondary)
                }
                Button(String(localized: "android.enterprise_missions_refresh")) { enterprise.refreshMissions() }
                    .disabled(!enterprise.canRefreshMissions)
                if enterprise.isOfflineProbe() {
                    Text(String(localized: "android.enterprise_missions_refresh_offline")).foregroundStyle(.secondary)
                }
                if enterprise.isRefreshingMissions {
                    ProgressView().accessibilityLabel(String(localized: "android.enterprise_missions_refresh"))
                }
                if let error = enterprise.missionAccessError {
                    Text(EnterpriseCoordinator.accessMessage(error))
                        .foregroundStyle(AbastevoTheme.errorRed)
                    if enterprise.missionsLoaded {
                        Text(String(localized: "android.enterprise_missions_refresh_failed")).foregroundStyle(.secondary)
                    }
                }
            }
            if enterprise.missionsLoaded {
                Section {
                    if enterprise.missionRows.isEmpty {
                        Text(String(localized: "android.enterprise_missions_empty")).foregroundStyle(.secondary)
                    }
                    ForEach(enterprise.missionRows) { row in
                        if let item = row.item {
                            NavigationLink {
                                EnterpriseMissionDetailView(enterprise: enterprise, id: row.id)
                            } label: {
                                VStack(alignment: .leading, spacing: 4) {
                                    Text(item.title).font(.headline)
                                    if let state = row.state {
                                        Text(EnterpriseMissionText.state(state)).font(.subheadline).foregroundStyle(.secondary)
                                    }
                                    if let status = EnterpriseMissionText.answer(row.response) {
                                        Text(status).font(.footnote).foregroundStyle(.secondary)
                                    }
                                }
                            }
                        } else if let status = EnterpriseMissionText.answer(row.response) {
                            Text(status).foregroundStyle(.secondary)
                        }
                    }
                }
            }
        }
        .navigationTitle(String(localized: "android.enterprise_missions_section_title"))
    }

    @ViewBuilder
    private var ageNotice: some View {
        if let age = enterprise.missionAgeMillis {
            if age < 60_000 {
                Text(String(localized: "android.enterprise_missions_age_now"))
            } else {
                let duration = elapsed(age)
                Text(String(localized: "android.enterprise_missions_age", defaultValue: "Updated \(duration) ago"))
            }
            if age >= EnterpriseMissionInbox.staleAfterMillis {
                Text(String(localized: "android.enterprise_missions_stale")).foregroundStyle(.secondary)
            }
        } else {
            Text(String(localized: "android.enterprise_missions_age_unknown")).foregroundStyle(.secondary)
            Text(String(localized: "android.enterprise_missions_stale")).foregroundStyle(.secondary)
        }
    }

    private func elapsed(_ millis: Int64) -> String {
        if millis >= 86_400_000 {
            let days = millis / 86_400_000
            return String(localized: "android.enterprise_missions_age_days", defaultValue: "\(days) d")
        }
        if millis >= 3_600_000 {
            let hours = millis / 3_600_000
            return String(localized: "android.enterprise_missions_age_hours", defaultValue: "\(hours) h")
        }
        let minutes = millis / 60_000
        return String(localized: "android.enterprise_missions_age_minutes", defaultValue: "\(minutes) min")
    }
}

@MainActor
private struct EnterpriseMissionDetailView: View {
    @ObservedObject var enterprise: EnterpriseCoordinator
    let id: String

    @State private var decline = false
    @State private var reason = ""
    @State private var refusal: String?

    private var row: EnterpriseMissionRow? { enterprise.missionRows.first { $0.id == id } }

    var body: some View {
        List {
            if let row, let item = row.item {
                Section {
                    if let state = row.state { Text(EnterpriseMissionText.state(state)).fontWeight(.semibold) }
                    if let start = EnterpriseValues.parseInstantMillis(item.startsAt),
                       let end = EnterpriseValues.parseInstantMillis(item.endsAt) {
                        let windowStart = EnterpriseLabels.dateTime(millis: start)
                        let windowEnd = EnterpriseLabels.dateTime(millis: end)
                        Text(String(localized: "android.enterprise_missions_window", defaultValue: "Planned: \(windowStart) – \(windowEnd)"))
                    }
                    Text(assetName(item.assetId))
                    if let plan = item.plan {
                        planDetails(plan)
                    } else {
                        Text(String(localized: "android.enterprise_missions_no_plan")).foregroundStyle(.secondary)
                    }
                }
                answerSection(row)
            } else if let row, let status = EnterpriseMissionText.answer(row.response) {
                Text(status).foregroundStyle(.secondary)
            } else {
                Text(String(localized: "android.enterprise_missions_refusal_unknown")).foregroundStyle(.secondary)
            }
        }
        .navigationTitle(row?.item?.title ?? String(localized: "android.enterprise_missions_section_title"))
    }

    @ViewBuilder
    private func planDetails(_ plan: EnterpriseMissionPlan) -> some View {
        if let destination = plan.destinationText {
            Text(String(localized: "android.enterprise_missions_destination", defaultValue: "Destination: \(destination)"))
        } else {
            Text(String(localized: "android.enterprise_missions_destination_unit"))
        }
        Text(String(localized: "android.enterprise_missions_reason", defaultValue: "Reason: \(plan.reason)"))
        switch plan.load {
        case .none:
            Text(String(localized: "android.enterprise_missions_load_none"))
        case .cargo:
            if let cargo = plan.cargo {
                Text(String(localized: "android.enterprise_missions_load_cargo", defaultValue: "Cargo: \(cargo)"))
            }
        case .passengers:
            if let count = plan.passengers {
                Text(String(localized: "android.enterprise_missions_load_passengers", defaultValue: "Passengers: \(count)"))
            }
        }
    }

    @ViewBuilder
    private func answerSection(_ row: EnterpriseMissionRow) -> some View {
        Section {
            if let status = EnterpriseMissionText.answer(row.response) { Text(status) }
            if let code = row.refusalCode, row.response == .refused {
                let reason = EnterpriseCoordinator.codeLabel(code)
                Text(String(localized: "android.enterprise_trips_reason", defaultValue: "Reason: \(reason)"))
                    .foregroundStyle(.secondary)
            }
            if row.canAnswer {
                Button(String(localized: "android.enterprise_missions_accept")) { respond(.accept) }
                Button(String(localized: "android.enterprise_missions_decline")) { decline.toggle() }
                if decline {
                    TextField(String(localized: "android.enterprise_missions_decline_reason"), text: $reason, axis: .vertical)
                    Button(String(localized: "android.enterprise_missions_save_decline")) { respond(.decline) }
                        .disabled(!EnterpriseValues.isValidText(reason, maxScalars: EnterpriseValues.maxReasonScalars))
                }
            }
            if row.canStartTrip {
                NavigationLink(String(localized: "android.enterprise_missions_start")) {
                    EnterpriseTripStartView(enterprise: enterprise, missionId: id)
                }
            } else if row.state == .assigned, row.response == .saved, row.queuedResponse == .accept {
                Text(String(localized: "android.enterprise_missions_start_not_confirmed")).foregroundStyle(.secondary)
            }
            if let tripId = row.tripId {
                NavigationLink(String(localized: "android.enterprise_missions_open_trip")) {
                    EnterpriseMissionTripView(enterprise: enterprise, tripId: tripId)
                }
            }
            if let refusal { Text(refusal).foregroundStyle(AbastevoTheme.errorRed) }
        }
    }

    private func assetName(_ id: String?) -> String {
        guard let id else { return String(localized: "android.enterprise_missions_vehicle_any") }
        guard let asset = enterprise.findAsset(id) else { return String(localized: "android.enterprise_missions_vehicle_unknown") }
        let label = EnterpriseCoordinator.assetLabel(asset)
        return String(localized: "android.enterprise_missions_vehicle", defaultValue: "Vehicle: \(label)")
    }

    private func respond(_ value: EnterpriseMissionResponse) {
        switch enterprise.respondToMission(id, response: value, reason: value == .decline ? reason : nil) {
        case .success:
            reason = ""
            decline = false
            refusal = nil
        case .failure(let error):
            refusal = error.localizedDescription
        }
    }
}

@MainActor
private struct EnterpriseMissionTripView: View {
    @ObservedObject var enterprise: EnterpriseCoordinator
    let tripId: String

    var body: some View {
        List {
            if let held = enterprise.snapshot, let trip = held.trips.first(where: { $0.id == tripId }) {
                Section {
                    Text(trip.assetLabel).font(.headline)
                    Text(EnterpriseTripLifecycleLabels.state(trip.localState)).foregroundStyle(.secondary)
                    let started = EnterpriseLabels.dateTime(millis: trip.startedAtMillis)
                    Text(String(localized: "enterprise.trip.start_time", defaultValue: "Started \(started)"))
                    if let command = held.queue.first(where: { $0.id == trip.startOperationId }) {
                        Text(EnterpriseCoordinator.statusLabel(command)).foregroundStyle(.secondary)
                    }
                    if let command = held.queue.first(where: { $0.id == trip.finishOperationId }) {
                        Text(EnterpriseCoordinator.statusLabel(command)).foregroundStyle(.secondary)
                    }
                    if held.openTrip?.id == tripId {
                        EnterpriseTripLifecycleControls(enterprise: enterprise, trip: trip)
                        NavigationLink(String(localized: "android.enterprise_trips_finish")) {
                            EnterpriseTripFinishView(enterprise: enterprise, trip: trip)
                        }
                    }
                }
                if held.openTrip?.id == tripId, !held.locked {
                    EnterpriseTripAuthorizationSection(enterprise: enterprise, tripId: tripId)
                }
            } else {
                Text(String(localized: "enterprise.trip.none_open")).foregroundStyle(.secondary)
            }
        }
        .navigationTitle(String(localized: "android.enterprise_missions_open_trip"))
    }
}

private enum EnterpriseMissionText {
    static func state(_ state: EnterpriseMissionState) -> String {
        switch state {
        case .draft, .planned: return String(localized: "android.enterprise_missions_state_planning")
        case .assigned: return String(localized: "android.enterprise_missions_state_assigned")
        case .accepted: return String(localized: "android.enterprise_missions_state_accepted")
        case .declined: return String(localized: "android.enterprise_missions_state_declined")
        case .inProgress: return String(localized: "android.enterprise_missions_state_in_progress")
        case .completed: return String(localized: "android.enterprise_missions_state_completed")
        case .cancelled: return String(localized: "android.enterprise_missions_state_cancelled")
        }
    }

    static func answer(_ status: EnterpriseMissionAnswerStatus) -> String? {
        switch status {
        case .none: return nil
        case .saved: return String(localized: "android.enterprise_missions_answer_saved")
        case .accepted: return String(localized: "android.enterprise_missions_answer_accepted")
        case .declined: return String(localized: "android.enterprise_missions_answer_declined")
        case .refused: return String(localized: "android.enterprise_missions_answer_refused")
        case .changed: return String(localized: "android.enterprise_missions_answer_changed")
        case .missing: return String(localized: "android.enterprise_missions_answer_removed")
        case .outsidePage: return String(localized: "enterprise.missions_answer_outside_page")
        }
    }
}

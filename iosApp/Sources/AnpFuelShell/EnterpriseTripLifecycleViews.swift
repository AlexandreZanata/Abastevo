import AnpFuelCore
import SwiftUI

@MainActor
struct EnterpriseTripLifecycleControls: View {
    @ObservedObject var enterprise: EnterpriseCoordinator
    let trip: EnterpriseTripRecord
    @State private var choice: Choice?
    @State private var reason = ""
    @State private var error: String?

    private struct Choice: Identifiable {
        let id = UUID()
        let action: EnterpriseTripTransitionAction
        let partition: EnterprisePartition
        let tripId: String
    }

    private var currentTrip: EnterpriseTripRecord? {
        enterprise.snapshot?.trips.first { $0.id == trip.id }
    }

    var body: some View {
        if let currentTrip {
            Group {
                if currentTrip.admits(.pause) {
                    Button(EnterpriseTripLifecycleLabels.action(.pause)) { select(.pause) }
                }
                if currentTrip.admits(.resume) {
                    Button(EnterpriseTripLifecycleLabels.action(.resume)) { select(.resume) }
                }
                if currentTrip.admits(.cancel) {
                    Button(EnterpriseTripLifecycleLabels.action(.cancel), role: .destructive) { select(.cancel) }
                }
                if let error {
                    Text(error).font(.footnote).foregroundStyle(AbastevoTheme.errorRed)
                }
            }
            .sheet(item: $choice) { selected in
                NavigationStack {
                    Form {
                        if selected.action == .cancel {
                            Text(String(localized: "enterprise.trip_cancel.explanation"))
                        }
                        TextField(
                            selected.action == .cancel ? String(localized: "enterprise.trip_transition.reason_required")
                                : String(localized: "enterprise.trip_transition.reason_optional"),
                            text: $reason
                        )
                        Button(EnterpriseTripLifecycleLabels.action(selected.action),
                               role: selected.action == .cancel ? .destructive : nil) { commit(selected) }
                            .disabled(selected.action == .cancel && reason.trimmingCharacters(in: .whitespacesAndNewlines).isEmpty)
                        if let error { Text(error).foregroundStyle(AbastevoTheme.errorRed) }
                    }
                    .navigationTitle(EnterpriseTripLifecycleLabels.action(selected.action))
                    .toolbar {
                        ToolbarItem(placement: .cancellationAction) {
                            Button(String(localized: "android.feed_close")) { choice = nil }
                        }
                    }
                }
            }
        }
    }

    private func select(_ action: EnterpriseTripTransitionAction) {
        guard let partition = enterprise.snapshot?.partition else { return }
        reason = ""
        error = nil
        choice = Choice(action: action, partition: partition, tripId: trip.id)
    }

    private func commit(_ chosen: Choice) {
        switch enterprise.transitionTrip(
            chosen.action, tripId: chosen.tripId, reasonText: reason, expectedPartition: chosen.partition
        ) {
        case .success:
            choice = nil
        case .failure(let failure):
            error = failure.message
        }
    }
}

enum EnterpriseTripLifecycleLabels {
    static func action(_ action: EnterpriseTripTransitionAction) -> String {
        switch action {
        case .pause: return String(localized: "enterprise.trip_transition.pause")
        case .resume: return String(localized: "enterprise.trip_transition.resume")
        case .cancel: return String(localized: "enterprise.trip_transition.cancel")
        }
    }

    static func state(_ state: EnterpriseTripLocalState) -> String {
        switch state {
        case .running: return String(localized: "enterprise.trip.in_progress")
        case .paused: return String(localized: "enterprise.trip_transition.paused")
        case .cancelled: return String(localized: "enterprise.trip_transition.cancelled")
        case .finished: return String(localized: "android.enterprise_trips_finished_local")
        }
    }
}

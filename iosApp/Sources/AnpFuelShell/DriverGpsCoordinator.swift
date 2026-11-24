import AnpFuelCore
import CoreLocation
import Foundation
import Observation

@MainActor
@Observable
final class DriverGpsCoordinator {
    enum Phase: Equatable {
        case idle
        case requestingPermission
        case recording(metres: Int64?, weakSignal: Bool)
        case interrupted
        case stopped(metres: Int64?, incomplete: Bool)
        case permissionDenied
        case unavailable
        case storageUnavailable
    }

    struct Context: Equatable, Sendable {
        let scope: DriverGpsScope
        let sessionFamilyId: String?
        let shiftId: String
    }

    private struct Recording {
        let id = UUID()
        let context: Context
        let startedAt: Date
        let isCurrent: () -> Bool
        /// Stopped metres of this shift's earlier recordings.
        var priorMetres: Int64?
        var accumulator = DriverGpsAccumulator()
        var lastFix: CLLocation?

        var shiftMetres: Int64? { DriverGpsCheckpoint.combined(priorMetres, accumulator.distanceMetres) }
    }

    private(set) var phase: Phase = .idle
    private(set) var shiftId: String?
    @ObservationIgnored private var recording: Recording?
    @ObservationIgnored private let location: ActiveNavigationLocation
    @ObservationIgnored private let store: DriverGpsCheckpointStore

    init(location: ActiveNavigationLocation, store: DriverGpsCheckpointStore = DriverGpsCheckpointStore()) {
        self.location = location
        self.store = store
    }

    var isRunning: Bool {
        if case .recording = phase { return true }
        return phase == .requestingPermission
    }

    var stoppedMetres: Int64? {
        if case .stopped(let result, _) = phase { return result }
        return nil
    }

    func load(_ context: Context?) {
        if recording?.context == context, recording != nil { return }
        if recording != nil { interrupt() }
        shiftId = context?.shiftId
        guard let context else {
            phase = .idle
            return
        }
        do {
            if let checkpoint = try store.load(scope: context.scope, shiftId: context.shiftId) {
                phase = checkpoint.stopped
                    ? .stopped(metres: checkpoint.acknowledgedMetres, incomplete: checkpoint.isIncomplete)
                    : .interrupted
            } else {
                phase = .idle
            }
        } catch {
            phase = .storageUnavailable
        }
    }

    func start(_ context: Context, isCurrent: @escaping () -> Bool) {
        guard recording == nil, isCurrent() else { return }
        var run = Recording(context: context, startedAt: Date(), isCurrent: isCurrent)
        shiftId = context.shiftId
        do {
            // A restart keeps the earlier stopped metres and marks the shift's estimate incomplete.
            run.priorMetres = try store.begin(scope: context.scope, shiftId: context.shiftId, recordingId: run.id).priorMetres
        } catch {
            phase = .storageUnavailable
            return
        }
        recording = run
        phase = .requestingPermission
        let id = run.id
        location.start(consumer: .driver) { [weak self] event in
            self?.receive(event, recordingId: id)
        }
    }

    func stop() {
        guard let run = recording else { return }
        guard run.isCurrent() else {
            interrupt()
            return
        }
        detach()
        do {
            let checkpoint = try store.finish(
                scope: run.context.scope, shiftId: run.context.shiftId, recordingId: run.id,
                metres: run.accumulator.distanceMetres, droppedGap: run.accumulator.hasGap
            )
            phase = .stopped(metres: checkpoint.acknowledgedMetres, incomplete: checkpoint.isIncomplete)
        } catch {
            phase = .storageUnavailable
        }
    }

    func verifyOwner() {
        if let recording, !recording.isCurrent() { interrupt() }
    }

    /// The shift's stopped recordings summed, with the incomplete marker the shift record keeps.
    func finalEstimate(_ context: Context) throws -> DriverGpsShiftEstimate? {
        guard recording == nil else { return nil }
        return try store.load(scope: context.scope, shiftId: context.shiftId)?.shiftEstimate
    }

    func completed(scope: DriverGpsScope, shiftId: String) throws {
        if self.shiftId == shiftId {
            detach()
            phase = .idle
            self.shiftId = nil
        }
        try store.remove(scope: scope, shiftId: shiftId)
    }

    /// Logout or account deletion: stops collection and removes only the departing owner's checkpoints.
    func purge(scope: DriverGpsScope) throws {
        detach()
        phase = .idle
        shiftId = nil
        try store.purge(scope: scope)
    }

    /// Origin switch: stops collection and removes every owner's checkpoints at the previous origin.
    func purge(origin: String) throws {
        detach()
        phase = .idle
        shiftId = nil
        try store.purge(origin: origin)
    }

    private func interrupt() {
        detach()
        phase = .interrupted
    }

    private func detach() {
        recording = nil
        location.stop(consumer: .driver)
    }

    private func receive(_ event: ActiveNavigationLocation.Event, recordingId: UUID) {
        guard let run = recording, run.id == recordingId else { return }
        guard run.isCurrent() else {
            interrupt()
            return
        }
        switch event {
        case .requestingPermission:
            phase = .requestingPermission
        case .active:
            phase = .recording(metres: run.shiftMetres, weakSignal: false)
        case .permissionDenied:
            detach()
            phase = .permissionDenied
        case .serviceUnavailable:
            detach()
            phase = .unavailable
        case .weakSignal:
            recording?.accumulator.resetAnchor()
            recording?.lastFix = nil
            phase = .recording(metres: run.shiftMetres, weakSignal: true)
        case .fix(let fix):
            receiveFix(fix)
        }
    }

    private func receiveFix(_ fix: CLLocation) {
        guard var current = recording, CLLocationCoordinate2DIsValid(fix.coordinate), fix.timestamp >= current.startedAt else { return }
        let result = current.accumulator.append(
            accuracyMetres: fix.horizontalAccuracy, sampledAt: fix.timestamp.timeIntervalSince1970,
            receivedAt: Date().timeIntervalSince1970, simulated: fix.sourceInformation?.isSimulatedBySoftware == true,
            segmentMetres: current.lastFix.map { fix.distance(from: $0) }
        )
        if result != .rejected {
            current.lastFix = fix
            recording = current
        }
        phase = .recording(metres: current.shiftMetres, weakSignal: result == .rejected)
    }
}

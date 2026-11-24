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
        case stopped(metres: Int64?)
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
        var accumulator = DriverGpsAccumulator()
        var lastFix: CLLocation?
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
        if case .stopped(let result) = phase { return result }
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
                phase = checkpoint.stopped ? .stopped(metres: checkpoint.acknowledgedMetres) : .interrupted
            } else {
                phase = .idle
            }
        } catch {
            phase = .storageUnavailable
        }
    }

    func start(_ context: Context, isCurrent: @escaping () -> Bool) {
        guard recording == nil, isCurrent() else { return }
        let run = Recording(context: context, startedAt: Date(), isCurrent: isCurrent)
        shiftId = context.shiftId
        do {
            try store.begin(scope: context.scope, shiftId: context.shiftId, recordingId: run.id)
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
                metres: run.accumulator.distanceMetres
            )
            phase = .stopped(metres: checkpoint.acknowledgedMetres)
        } catch {
            phase = .storageUnavailable
        }
    }

    func verifyOwner() {
        if let recording, !recording.isCurrent() { interrupt() }
    }

    func finalMetres(_ context: Context) throws -> Int64? {
        guard recording == nil else { return nil }
        return try store.load(scope: context.scope, shiftId: context.shiftId)?.acknowledgedMetres
    }

    func completed(scope: DriverGpsScope, shiftId: String) throws {
        if self.shiftId == shiftId {
            detach()
            phase = .idle
            self.shiftId = nil
        }
        try store.remove(scope: scope, shiftId: shiftId)
    }

    func purge() throws {
        detach()
        phase = .idle
        shiftId = nil
        try store.purge()
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
            phase = .recording(metres: nil, weakSignal: false)
        case .permissionDenied:
            detach()
            phase = .permissionDenied
        case .serviceUnavailable:
            detach()
            phase = .unavailable
        case .weakSignal:
            recording?.accumulator.resetAnchor()
            recording?.lastFix = nil
            phase = .recording(metres: run.accumulator.distanceMetres, weakSignal: true)
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
        phase = .recording(metres: current.accumulator.distanceMetres, weakSignal: result == .rejected)
    }
}

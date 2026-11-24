import AnpFuelCore
import Foundation
import Combine

@MainActor
final class DriverAccountingModel: ObservableObject {
    @Published private(set) var entries: [DriverOutboxEntry] = []
    @Published private(set) var notice: String?
    @Published private(set) var isDispatching = false

    private weak var host: AppStateViewModel?
    private let outbox = DriverAccountingOutbox()
    private var task: Task<Void, Never>?
    private var wake: Task<Void, Never>?
    private var generation = UUID()
    private var foreground = false

    init(host: AppStateViewModel) { self.host = host }

    private var partition: DriverOutboxScope? {
        guard let host, let account = host.currentAccountIdOrNil else { return nil }
        return DriverOutboxScope(accountId: account, origin: host.snapshot.apiEnvironment.origin)
    }

    private var requestScope: AccountDataExportScope? { host?.accountDataExportScope }

    func foregrounded() {
        foreground = true
        refresh()
        dispatch()
    }

    func backgrounded() {
        foreground = false
        suspendDispatch()
    }

    func contextChanged() {
        suspendDispatch()
        refresh()
        dispatch()
    }

    func purge() {
        suspendDispatch()
        if let partition {
            do { try outbox.erase(partition) }
            catch { notice = String(localized: "driver.sync.storage") }
        }
        entries = []
    }

    func refresh() {
        guard let partition else {
            entries = []
            return
        }
        do {
            entries = try outbox.entries(in: partition).sorted { $0.createdAtMillis > $1.createdAtMillis }
        } catch {
            entries = []
            notice = String(localized: "driver.sync.storage")
        }
    }

    func queue(_ shift: DriverShiftRecord, platform: DriverPlatform, money: DriverEarningsMoney) -> Bool {
        guard let host, let partition, !host.isAuthWorking,
              !shift.isActive, host.driverVisibleShifts.contains(shift) else {
            notice = String(localized: "driver.sync.sign_in")
            return false
        }
        do {
            let localEntries = try outbox.entries(in: partition)
            let previous = localEntries.filter { $0.command.recordId == shift.id }
                .compactMap(\.receipt)
                .max { $0.revision < $1.revision }
            let knownEpoch = try outbox.knownEpoch(in: partition) ?? 0
            guard let timezone = shift.capturedTimezone, let command = DriverAccountingCommand.earning(
                forCompletedShift: shift, platform: platform, money: money, timezone: timezone,
                expectedRevision: previous?.revision ?? 0, epoch: knownEpoch
            ) else {
                notice = String(localized: "driver.sync.invalid")
                return false
            }
            try outbox.enqueue(command, in: partition, nowMillis: LedgerCoordinator.nowMillis())
            notice = nil
            refresh()
            dispatch()
            return true
        } catch DriverAccountingError.existingCommand {
            notice = String(localized: "driver.sync.already_queued")
        } catch DriverAccountingError.queueFull {
            notice = String(localized: "driver.sync.queue_full")
        } catch {
            notice = String(localized: "driver.sync.storage")
        }
        return false
    }

    func resumeWaiting() async {
        guard let host, let partition else { return }
        await host.ledger.refreshPremiumProof()
        guard self.partition == partition else { return }
        do {
            try outbox.resumeWaiting(in: partition, nowMillis: LedgerCoordinator.nowMillis())
            notice = nil
            refresh()
            dispatch()
        } catch {
            notice = String(localized: "driver.sync.storage")
        }
    }

    func discard(_ entry: DriverOutboxEntry) {
        guard let partition, entries.contains(entry), entry.state == .conflict else { return }
        do {
            try outbox.discardConflict(operationId: entry.id, in: partition)
            notice = nil
            refresh()
        } catch {
            notice = String(localized: "driver.sync.storage")
        }
    }

    func prepareVehicle(_ vehicle: VehicleRecord) async {
        guard let host, let requestScope, host.snapshot.vehicles.contains(vehicle) else {
            notice = String(localized: "driver.sync.connect")
            return
        }
        notice = nil
        do {
            try await host.communityGateway.registerDriverVehicle(vehicle, scope: requestScope)
            guard self.requestScope == requestScope, !Task.isCancelled else { return }
            notice = String(localized: "driver.sync.vehicle_registered")
        } catch {
            guard self.requestScope == requestScope, !Task.isCancelled else { return }
            notice = Self.vehicleRefusal(error)
        }
    }

    private func dispatch() {
        guard foreground, task == nil, let host, let partition, let requestScope else { return }
        let token = generation
        let client = host.communityGateway
        wake?.cancel()
        wake = nil
        task = Task { [weak self] in
            guard let self else { return }
            await drain(partition, requestScope: requestScope, client: client, token: token)
        }
    }

    private func drain(
        _ partition: DriverOutboxScope, requestScope: AccountDataExportScope,
        client: CommunityWriteClient, token: UUID
    ) async {
        guard generation == token, !Task.isCancelled, foreground else { return }
        isDispatching = true
        defer {
            if generation == token {
                task = nil
                isDispatching = false
                refresh()
                scheduleWake(for: partition)
            }
        }
        do {
            while generation == token, foreground, !Task.isCancelled,
                  self.partition == partition, self.requestScope == requestScope,
                  let claim = try outbox.claim(in: partition, nowMillis: LedgerCoordinator.nowMillis()) {
                let outcome: DriverOutboxOutcome
                do {
                    outcome = .synced(try await client.submitDriverAccounting(claim, scope: requestScope))
                } catch {
                    outcome = DriverOutboxOutcome.forFailure(error)
                }
                if generation != token || self.partition != partition {
                    // Purge may already have removed the lease. A stale settlement cannot recreate the old state.
                    try? outbox.settle(claim, outcome: .interrupted, nowMillis: LedgerCoordinator.nowMillis())
                    return
                }
                try outbox.settle(claim, outcome: outcome, nowMillis: LedgerCoordinator.nowMillis())
                refresh()
                if outcome == .interrupted { return }
            }
        } catch DriverAccountingError.staleClaim {
            // A replacement lease owns a result from this point.
        } catch {
            notice = String(localized: "driver.sync.storage")
        }
    }

    private func scheduleWake(for scope: DriverOutboxScope) {
        guard foreground, requestScope != nil, scope == partition,
              let next = try? outbox.nextEligibleMillis(in: scope), next != Int64.max else { return }
        let delay = max(0, next - LedgerCoordinator.nowMillis())
        wake = Task { [weak self] in
            do { try await Task.sleep(for: .milliseconds(delay)) } catch { return }
            guard let self, self.partition == scope, !Task.isCancelled else { return }
            dispatch()
        }
    }

    private func suspendDispatch() {
        generation = UUID()
        task?.cancel()
        task = nil
        wake?.cancel()
        wake = nil
        isDispatching = false
    }

    private static func vehicleRefusal(_ error: Error) -> String {
        if let transport = error as? SignedTransportError, case .refused(let status, _, _) = transport {
            if status == 403 { return String(localized: "ledger.refusal.capability") }
            if status == 401 { return String(localized: "ledger.refusal.sign_in_again") }
            if status == 409 { return String(localized: "driver.sync.conflict") }
        }
        if (error as? CommunityWriteError) == .invalid("cloud.invalid") {
            return String(localized: "driver.sync.vehicle_invalid")
        }
        if (error as? CommunityWriteError) == .invalid("cloud.conflict") {
            return String(localized: "driver.sync.conflict")
        }
        return String(localized: "driver.sync.vehicle_unavailable")
    }
}

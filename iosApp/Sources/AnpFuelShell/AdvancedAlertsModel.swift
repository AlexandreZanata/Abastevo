import SwiftUI
import UserNotifications
import AnpFuelCore

@MainActor
final class AdvancedAlertsModel: ObservableObject {
    @Published private(set) var scope: AccountDataExportScope?
    @Published private(set) var rules: [AdvancedAlertRuleRecord]?
    @Published private(set) var inbox: AdvancedAlertInbox?
    @Published private(set) var rulesError: AdvancedAlertError?
    @Published private(set) var inboxError: AdvancedAlertError?
    @Published private(set) var commandError: AdvancedAlertError?
    @Published private(set) var acknowledged = false
    @Published private(set) var busy = false
    @Published private(set) var pending = false
    @Published private(set) var retryAt: Date?
    @Published private(set) var notificationsEnabled = false
    @Published private(set) var notificationPermissionDenied = false

    private let store = AdvancedAlertStore()
    private var operation = UUID()

    func activate(on host: AppStateViewModel) async {
        if scope != host.advancedAlertScope {
            clearView()
        }
        scope = host.advancedAlertScope
        guard let scope else { return }
        updateStoredState(scope: scope)
        if !host.isOfflineSimulation { await refresh(on: host) }
    }

    func refresh(on host: AppStateViewModel, preserveCommandResult: Bool = false) async {
        guard let scope, let ticket = begin(host, scope: scope, online: true) else { return }
        defer { complete(ticket) }
        if !preserveCommandResult { acknowledged = false }
        let client = host.communityGateway
        async let readRules = loadRules(client, scope: scope)
        async let readInbox = loadInbox(client, scope: scope)
        let results = await (readRules, readInbox)
        guard valid(host, scope: scope, ticket: ticket, online: true) else { return }
        switch results.0 {
        case .success(let rows):
            rules = rows
            rulesError = nil
        case .failure(let error):
            rules = nil
            rulesError = error
        }
        switch results.1 {
        case .success(let page):
            inbox = page
            inboxError = nil
            await notify(from: page, host: host, scope: scope, ticket: ticket)
        case .failure(let error):
            inbox = nil
            inboxError = error
        }
    }

    func confirm(_ command: AdvancedAlertCommand, scope: AccountDataExportScope, on host: AppStateViewModel) async {
        guard self.scope == scope, !busy, host.advancedAlertScope == scope else { return }
        commandError = nil
        acknowledged = false
        do {
            try store.freeze(command, scope: scope)
            updateStoredState(scope: scope)
            if !host.isOfflineSimulation { await dispatch(scope: scope, on: host) }
        } catch {
            commandError = Self.alertError(error)
            updateStoredState(scope: scope)
        }
    }

    func dispatch(scope: AccountDataExportScope, on host: AppStateViewModel) async {
        guard let ticket = begin(host, scope: scope, online: true) else { return }
        commandError = nil
        acknowledged = false
        var shouldRefresh = false
        do {
            let receipt = try await store.dispatch(scope: scope, client: host.communityGateway)
            guard valid(host, scope: scope, ticket: ticket, online: true) else {
                complete(ticket)
                return
            }
            acknowledged = receipt != nil
            shouldRefresh = receipt != nil
        } catch {
            guard valid(host, scope: scope, ticket: ticket, online: true) else {
                complete(ticket)
                return
            }
            let refusal = Self.alertError(error)
            commandError = refusal
            shouldRefresh = refusal == .conflict
        }
        updateStoredState(scope: scope)
        complete(ticket)
        if shouldRefresh { await refresh(on: host, preserveCommandResult: true) }
    }

    func discardPending(scope: AccountDataExportScope, on host: AppStateViewModel) {
        guard self.scope == scope, !busy, host.advancedAlertScope == scope else { return }
        do {
            try store.discardPending(scope: scope)
            commandError = nil
            updateStoredState(scope: scope)
        } catch {
            commandError = Self.alertError(error)
        }
    }

    func erase(scope: AccountDataExportScope, on host: AppStateViewModel) async {
        guard let ticket = begin(host, scope: scope, online: true) else { return }
        acknowledged = false
        commandError = nil
        do {
            // Explicit erase supersedes a parked change. An active write lease still refuses erase.
            try store.discardPending(scope: scope)
            updateStoredState(scope: scope)
            try await host.communityGateway.eraseAdvancedAlerts(operationId: UUID().uuidString.lowercased(), scope: scope)
            guard valid(host, scope: scope, ticket: ticket, online: true) else {
                complete(ticket)
                return
            }
            try store.purge(scope: scope)
            Self.removeNotices(prefix: store.notificationIDPrefix(scope: scope))
            updateStoredState(scope: scope)
            acknowledged = true
        } catch {
            guard valid(host, scope: scope, ticket: ticket, online: true) else {
                complete(ticket)
                return
            }
            commandError = Self.alertError(error)
        }
        complete(ticket)
        // Even on a lost acknowledgement, read the authoritative current state before any repeat.
        await refresh(on: host, preserveCommandResult: true)
    }

    func setNotifications(_ enabled: Bool, scope: AccountDataExportScope, on host: AppStateViewModel) async {
        guard let ticket = begin(host, scope: scope, online: false) else { return }
        defer { complete(ticket) }
        let center = UNUserNotificationCenter.current()
        do {
            if enabled {
                let settings = await center.notificationSettings()
                guard valid(host, scope: scope, ticket: ticket, online: false) else { return }
                let permitted: Bool
                if settings.authorizationStatus == .notDetermined {
                    permitted = try await center.requestAuthorization(options: [.alert])
                } else {
                    permitted = Self.allowed(settings.authorizationStatus)
                }
                guard valid(host, scope: scope, ticket: ticket, online: false) else { return }
                notificationPermissionDenied = !permitted
                guard permitted else { return }
            } else {
                notificationPermissionDenied = false
                Self.removeNotices(prefix: store.notificationIDPrefix(scope: scope))
            }
            try store.setNotificationsEnabled(enabled, scope: scope)
            notificationsEnabled = enabled
            if enabled, let inbox { _ = try store.notificationCandidates(for: inbox, scope: scope) }
        } catch {
            guard valid(host, scope: scope, ticket: ticket, online: false) else { return }
            commandError = Self.alertError(error)
        }
    }

    func purge() {
        let prefix = scope.map { store.notificationIDPrefix(scope: $0) } ?? AdvancedAlertStore.notificationPrefix
        clearView()
        scope = nil
        store.purge()
        Self.removeNotices(prefix: prefix)
    }

    private func clearView() {
        operation = UUID()
        busy = false
        rules = nil
        inbox = nil
        rulesError = nil
        inboxError = nil
        commandError = nil
        acknowledged = false
        pending = false
        retryAt = nil
        notificationsEnabled = false
        notificationPermissionDenied = false
    }

    private func begin(_ host: AppStateViewModel, scope: AccountDataExportScope, online: Bool) -> UUID? {
        guard !busy, valid(host, scope: scope, ticket: operation, online: online) else { return nil }
        operation = UUID()
        busy = true
        return operation
    }

    private func complete(_ ticket: UUID) {
        if ticket == operation { busy = false }
    }

    private func valid(_ host: AppStateViewModel, scope: AccountDataExportScope, ticket: UUID, online: Bool) -> Bool {
        !Task.isCancelled && operation == ticket && self.scope == scope && host.advancedAlertScope == scope
            && (!online || !host.isOfflineSimulation)
    }

    private func updateStoredState(scope: AccountDataExportScope) {
        do {
            pending = try store.pending(scope: scope) != nil
            retryAt = try store.retryAvailableAt(scope: scope)
            notificationsEnabled = try store.notificationsEnabled(scope: scope)
        } catch {
            commandError = Self.alertError(error)
        }
    }

    private func loadRules(
        _ client: CommunityWriteClient, scope: AccountDataExportScope
    ) async -> Result<[AdvancedAlertRuleRecord], AdvancedAlertError> {
        do { return .success(try await client.advancedAlertRules(scope: scope)) }
        catch { return .failure(Self.alertError(error)) }
    }

    private func loadInbox(
        _ client: CommunityWriteClient, scope: AccountDataExportScope
    ) async -> Result<AdvancedAlertInbox, AdvancedAlertError> {
        do { return .success(try await client.advancedAlertInbox(scope: scope)) }
        catch { return .failure(Self.alertError(error)) }
    }

    private func notify(from inbox: AdvancedAlertInbox, host: AppStateViewModel, scope: AccountDataExportScope, ticket: UUID) async {
        guard notificationsEnabled, valid(host, scope: scope, ticket: ticket, online: true) else { return }
        let center = UNUserNotificationCenter.current()
        let settings = await center.notificationSettings()
        guard valid(host, scope: scope, ticket: ticket, online: true) else { return }
        notificationPermissionDenied = !Self.allowed(settings.authorizationStatus)
        guard !notificationPermissionDenied else { return }
        do {
            let candidates = try store.notificationCandidates(for: inbox, scope: scope)
            guard let item = candidates.last else { return }
            let id = store.notificationID(item.id, scope: scope)
            AdvancedAlertNotificationPresenter.install(on: center)
            let content = UNMutableNotificationContent()
            content.title = String(localized: "alerts.notification_title")
            content.body = String(localized: "alerts.notification_body")
            try await center.add(UNNotificationRequest(identifier: id, content: content, trigger: nil))
            guard valid(host, scope: scope, ticket: ticket, online: true) else {
                center.removePendingNotificationRequests(withIdentifiers: [id])
                center.removeDeliveredNotifications(withIdentifiers: [id])
                return
            }
            for candidate in candidates { try store.recordNotification(candidate.id, scope: scope) }
        } catch {
            guard valid(host, scope: scope, ticket: ticket, online: true) else { return }
            commandError = Self.alertError(error)
        }
    }

    private static func allowed(_ status: UNAuthorizationStatus) -> Bool {
        switch status {
        case .authorized, .provisional, .ephemeral: return true
        default: return false
        }
    }

    private static func removeNotices(prefix: String) {
        let center = UNUserNotificationCenter.current()
        center.getPendingNotificationRequests { requests in
            center.removePendingNotificationRequests(withIdentifiers: requests.map(\.identifier).filter { $0.hasPrefix(prefix) })
        }
        center.getDeliveredNotifications { notifications in
            let ids = notifications.map { $0.request.identifier }.filter { $0.hasPrefix(prefix) }
            center.removeDeliveredNotifications(withIdentifiers: ids)
        }
    }

    private static func alertError(_ error: Error) -> AdvancedAlertError {
        error as? AdvancedAlertError ?? .unavailable(retryAfterSeconds: nil)
    }
}

/// Retains the delegate for immediate local notices while the app is in the foreground.
@MainActor
private final class AdvancedAlertNotificationPresenter: NSObject, UNUserNotificationCenterDelegate {
    private static let shared = AdvancedAlertNotificationPresenter()

    static func install(on center: UNUserNotificationCenter) {
        if center.delegate == nil { center.delegate = shared }
    }

    nonisolated func userNotificationCenter(
        _ center: UNUserNotificationCenter, willPresent notification: UNNotification
    ) async -> UNNotificationPresentationOptions {
        notification.request.identifier.hasPrefix(AdvancedAlertStore.notificationPrefix) ? [.banner, .list] : []
    }
}

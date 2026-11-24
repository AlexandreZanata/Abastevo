import AnpFuelCore
import CoreTransferable
import Foundation
import SwiftUI
import UniformTypeIdentifiers

private struct AccountExportFile: Transferable {
    let bytes: Data
    let server: Bool

    static var transferRepresentation: some TransferRepresentation {
        DataRepresentation(exportedContentType: .json) { (archive: AccountExportFile) in
            archive.bytes
        }
        .suggestedFileName { $0.server ? "abastevo-account.json" : "abastevo-device.json" }
    }
}

@MainActor
struct AccountExportSection: View {
    @ObservedObject var viewModel: AppStateViewModel
    let alias: String

    private struct LocalOwner: Equatable {
        let accountId: String
        let origin: String
    }

    @State private var localArchive: (owner: LocalOwner, data: Data)?
    @State private var serverArchive: (scope: AccountDataExportScope, export: AccountDataExport)?
    @State private var exportTask: Task<Void, Never>?
    @State private var generation = UUID()
    @State private var failed = false

    private var localOwner: LocalOwner? {
        guard !viewModel.isAuthWorking, let accountId = viewModel.currentAccountIdOrNil else { return nil }
        return LocalOwner(accountId: accountId, origin: viewModel.snapshot.apiEnvironment.origin)
    }

    private var serverScope: AccountDataExportScope? { viewModel.accountDataExportScope }

    var body: some View {
        Section(String(localized: "account.connected_identity")) {
            Text(alias)
            Button(String(localized: "account.local_export")) {
                guard let localOwner else { return }
                localArchive = (localOwner, Data(viewModel.exportMyDataJson().utf8))
            }
            .disabled(localOwner == nil)
            if let localArchive, localArchive.owner == localOwner {
                let file = AccountExportFile(bytes: localArchive.data, server: false)
                shareArchive(file, title: String(localized: "account.export.share_local"))
            }
            Button(String(localized: "account.export.request_server"), action: requestServer)
                .disabled(serverScope == nil || exportTask != nil)
            if exportTask != nil {
                ProgressView(String(localized: "account.export.loading"))
            }
            if failed {
                Text(String(localized: "account.export.failed")).foregroundStyle(.secondary)
            } else if let serverArchive, serverArchive.scope == serverScope {
                serverResult(serverArchive.export)
            } else if localOwner != nil, serverScope == nil {
                Text(String(localized: "account.export.connect")).foregroundStyle(.secondary)
            }
        }
        .onChange(of: localOwner) { _, _ in
            localArchive = nil
            clearServerArchive()
        }
        .onChange(of: serverScope) { _, _ in clearServerArchive() }
        .onDisappear {
            localArchive = nil
            clearServerArchive()
        }
    }

    @ViewBuilder
    private func serverResult(_ export: AccountDataExport) -> some View {
        if export.isPartial {
            Text(String(localized: "account.export.partial")).font(.subheadline.weight(.semibold))
            ForEach(export.sections.filter { $0.status != .complete }) { section in
                VStack(alignment: .leading, spacing: 4) {
                    Text(section.id.exportTitle).font(.subheadline)
                    ForEach(section.issues, id: \.rawValue) { issue in
                        Text(issue.exportDescription).font(.footnote).foregroundStyle(.secondary)
                    }
                }
            }
        }
        let file = AccountExportFile(bytes: export.json, server: true)
        shareArchive(file, title: String(localized: "account.export.share_server"))
    }

    private func shareArchive(_ file: AccountExportFile, title: String) -> some View {
        ShareLink(item: file, preview: SharePreview(title)) {
            Label(title, systemImage: "square.and.arrow.up")
        }
    }

    private func requestServer() {
        guard let scope = serverScope, exportTask == nil else { return }
        clearServerArchive()
        let request = generation
        let client = viewModel.communityGateway
        let receipts = viewModel.identity.items(of: .forumTopic) + viewModel.identity.items(of: .forumComment)
        exportTask = Task {
            defer {
                if generation == request { exportTask = nil }
            }
            do {
                let result = try await client.exportAccountData(scope: scope, localForumReceipts: receipts)
                guard !Task.isCancelled, generation == request, scope == serverScope else { return }
                serverArchive = (scope, result)
            } catch {
                guard !Task.isCancelled, generation == request, scope == serverScope else { return }
                failed = true
            }
        }
    }

    private func clearServerArchive() {
        generation = UUID()
        exportTask?.cancel()
        exportTask = nil
        serverArchive = nil
        failed = false
    }
}

private extension AccountDataExport.SectionID {
    var exportTitle: String {
        switch self {
        case .cloudVehicles: return String(localized: "account.export.cloud_vehicles")
        case .cloudFavorites: return String(localized: "account.export.cloud_favorites")
        case .cloudPreferences: return String(localized: "account.export.cloud_preferences")
        case .cloudDriverEarnings: return String(localized: "account.export.cloud_driver_earnings")
        case .cloudDriverDistance: return String(localized: "account.export.cloud_driver_distance")
        case .cloudDriverCosts: return String(localized: "account.export.cloud_driver_costs")
        case .cloudDriverMaintenance: return String(localized: "account.export.cloud_driver_maintenance")
        case .cloudStatus: return String(localized: "account.export.cloud_status")
        case .cloudDevices: return String(localized: "account.export.cloud_devices")
        case .cloudSnapshots: return String(localized: "account.export.cloud_snapshots")
        case .subscriptions: return String(localized: "account.export.subscriptions")
        case .refuels: return String(localized: "account.export.refuels")
        case .expenses: return String(localized: "android.ledger_sync_expense")
        case .charges: return String(localized: "account.export.charges")
        case .calculatedReports: return String(localized: "account.export.calculated_reports")
        case .priceObservations: return String(localized: "account.export.price_observations")
        case .forum: return String(localized: "android.forum_screen_title")
        case .forumReactions: return String(localized: "account.export.forum_reactions")
        case .stationClaims: return String(localized: "account.export.station_claims")
        case .stationSuggestions: return String(localized: "android.suggest_mine_title")
        case .stationCorrections: return String(localized: "account.export.station_corrections")
        case .trainingConsent: return String(localized: "privacy.ocr_consent")
        case .blockedUsers: return String(localized: "android.blocked_users_title")
        case .qualityReports: return String(localized: "android.quality_reports_title")
        case .chargingObservations: return String(localized: "account.export.charging_observations")
        case .alertRules: return String(localized: "account.export.alert_rules")
        case .alertInbox: return String(localized: "account.export.alert_inbox")
        case .driverEarnings: return String(localized: "account.export.driver_earnings")
        case .signInProviders: return String(localized: "account.export.sign_in_providers")
        case .contributorBindings: return String(localized: "account.export.contributor_bindings")
        }
    }
}

private extension AccountDataExport.Issue {
    var exportDescription: String {
        switch self {
        case .noOwnerInventory: return String(localized: "account.export.no_inventory")
        case .knownForumItemsOnly: return String(localized: "account.export.known_forum_only")
        case .currentContributorOnly: return String(localized: "account.export.current_contributor_only")
        case .listLimitReached: return String(localized: "account.export.list_limit_reached")
        case .boundCloudDeviceRequired: return String(localized: "account.export.cloud_device_required")
        case .sessionOrProofRequired: return String(localized: "account.export.auth_required")
        case .serviceUnavailable: return String(localized: "account.export.unavailable")
        case .invalidResponse: return String(localized: "account.export.invalid_response")
        case .readFailed: return String(localized: "account.export.read_failed")
        }
    }
}

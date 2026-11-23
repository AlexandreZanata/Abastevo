import SwiftUI
import AnpFuelCore

/// Main Abastevo iPhone/iPad application shell synchronized with `main` (v3.1.18).
/// Preserves `TankFillViewModel` and `TankFillView` for P12 contract tests.
@main
struct AnpFuelApp: App {
    @StateObject private var viewModel = AppStateViewModel()

    var body: some Scene {
        WindowGroup {
            AbastevoRootView(viewModel: viewModel)
                .preferredColorScheme(viewModel.preferredColorScheme)
        }
    }
}

public struct AbastevoRootView: View {
    @ObservedObject var viewModel: AppStateViewModel

    public init(viewModel: AppStateViewModel) {
        self.viewModel = viewModel
    }

    public var body: some View {
        Group {
            if !viewModel.snapshot.termsAccepted {
                TermsGateView(viewModel: viewModel)
            } else if !viewModel.snapshot.onboardingCompleted {
                OnboardingFlowView(viewModel: viewModel)
            } else {
                mainFourTabShell
            }
        }
        .sheet(item: $viewModel.selectedStation) { station in
            StationDetailSheetView(viewModel: viewModel, station: station)
        }
        .sheet(item: $viewModel.activeSheet) { sheet in
            sheetDestination(sheet)
        }
    }

    private var mainFourTabShell: some View {
        VStack(spacing: 0) {
            AbastevoTopChromeBar(
                locationLabel: viewModel.municipalityLabel,
                weekLabel: viewModel.surveyWeekLabel,
                isOffline: viewModel.isOfflineSimulation,
                signedInAlias: viewModel.signedInAlias,
                onLocationTap: { viewModel.activeSheet = .locationPicker },
                onWeekTap: { viewModel.activeSheet = .weekPicker },
                onSettingsTap: { viewModel.activeSheet = .settings },
                onProfileTap: { viewModel.selectTab(.profile) }
            )

            Group {
                switch viewModel.selectedTab {
                case .explore:
                    ExploreRootView(viewModel: viewModel)
                case .community:
                    CommunityRootView(viewModel: viewModel)
                case .map:
                    MapRootView(viewModel: viewModel)
                case .profile:
                    ProfileRootView(viewModel: viewModel)
                }
            }
            .frame(maxWidth: .infinity, maxHeight: .infinity)

            AbastevoBottomTabBar(
                selectedTab: viewModel.selectedTab,
                locale: viewModel.currentLocale,
                onSelectTab: { viewModel.selectTab($0) }
            )
        }
    }

    @ViewBuilder
    private func sheetDestination(_ sheet: AppModalSheet) -> some View {
        switch sheet {
        case .locationPicker:
            LocationPickerSheetView(viewModel: viewModel)
        case .weekPicker:
            WeekPickerSheetView()
        case .priceHistory:
            PriceHistorySheetView()
        case .evExplore:
            EvChargingExploreSheetView(viewModel: viewModel)
        case .qualityReports:
            QualityReportsSheetView()
        case .suggestStation:
            SuggestStationSheetView(viewModel: viewModel)
        case .masterFaq:
            MasterFaqSheetView()
        case .capturePrice:
            CapturePriceSheetView(viewModel: viewModel)
        case .forumFeed:
            ForumFeedSheetView(viewModel: viewModel)
        case .contributors:
            ContributorsSheetView(viewModel: viewModel)
        case .myContributions:
            MyContributionsSheetView(viewModel: viewModel)
        case .evTripPlanner:
            EvTripPlannerSheetView(viewModel: viewModel)
        case .savedPlaces:
            SavedPlacesSheetView(viewModel: viewModel)
        case .mapDownloads:
            MapDownloadsSheetView(viewModel: viewModel)
        case .auth:
            AuthSheetView(viewModel: viewModel)
        case .accountManagement:
            AccountManagementSheetView(viewModel: viewModel)
        case .premium:
            PremiumStoreSheetView(model: viewModel.premiumStore)
        case .vehiclesLedger:
            VehiclesAndLedgerSheetView(viewModel: viewModel)
        case .driverHub:
            DriverHubSheetView(viewModel: viewModel)
        case .enterpriseHub:
            EnterpriseHubSheetView()
        case .arcadeGames:
            ArcadeGamesSheetView(viewModel: viewModel)
        case .trainingConsent:
            TrainingConsentSheetView()
        case .settings:
            SettingsSheetView(viewModel: viewModel)
        case .help:
            HelpSheetView()
        case .privacy:
            PrivacyDocumentSheet()
        }
    }
}

struct TankFillViewModel {
    let prices: [StationPrice]
    let capacityMilliLiters: Int64

    var result: TankFillResult? {
        try? TankFillUseCase.fillCheapest(
            prices: prices,
            capacityMilliLiters: capacityMilliLiters
        )
    }

    var totalText: String {
        guard let result = result,
              let text = try? PortableMoney.format(result.totalMilli) else {
            return "—"
        }
        return "R$ \(text)"
    }
}

struct TankFillView: View {
    let viewModel: TankFillViewModel

    var body: some View {
        VStack(alignment: .leading, spacing: 8) {
            Text("Cheapest tank fill").font(.headline)
            if let result = viewModel.result {
                Text(result.station)
                Text(viewModel.totalText).font(.title)
            } else {
                Text("No prices").foregroundStyle(.secondary)
            }
        }
        .padding()
    }
}

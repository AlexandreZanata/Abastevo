package com.anpfuel.app.navigation

import androidx.compose.foundation.layout.WindowInsets
import androidx.compose.foundation.layout.consumeWindowInsets
import androidx.compose.foundation.layout.padding
import androidx.compose.material3.Scaffold
import androidx.compose.runtime.Composable
import androidx.compose.runtime.getValue
import androidx.compose.ui.Modifier
import androidx.hilt.navigation.compose.hiltViewModel
import androidx.lifecycle.compose.collectAsStateWithLifecycle
import androidx.navigation.NavGraph.Companion.findStartDestination
import androidx.navigation.NavHostController
import androidx.navigation.NavType
import androidx.navigation.compose.NavHost
import androidx.navigation.compose.composable
import androidx.navigation.compose.currentBackStackEntryAsState
import androidx.navigation.navArgument
import androidx.navigation.navDeepLink
import com.anpfuel.app.capture.CaptureScreen
import com.anpfuel.app.ui.auth.AuthRoute
import com.anpfuel.app.ui.account.AccountScreen
import com.anpfuel.app.ui.community.CommunityScreen
import com.anpfuel.app.ui.components.LoadingState
import com.anpfuel.app.ui.history.HistoryScreen
import com.anpfuel.app.ui.help.HelpScreen
import com.anpfuel.app.ui.home.HomeScreen
import com.anpfuel.app.ui.location.LocationPickerScreen
import com.anpfuel.app.ui.onboarding.OnboardingScreen
import com.anpfuel.app.ui.privacy.PrivacyScreen
import com.anpfuel.app.ui.profile.ProfileScreen
import com.anpfuel.app.ui.settings.SettingsScreen
import com.anpfuel.app.ui.stations.StationsScreen
import com.anpfuel.app.ui.vehicle.VehicleScreen
import com.anpfuel.app.ui.weekpicker.WeekPickerRoute
import com.anpfuel.app.viewmodel.AppStartViewModel

@Composable
fun AnpAppNavHost(
    navController: NavHostController,
    darkTheme: Boolean,
    onToggleTheme: () -> Unit,
    modifier: Modifier = Modifier,
    appStartViewModel: AppStartViewModel = hiltViewModel(),
) {
    val startDestination by appStartViewModel.startDestination.collectAsStateWithLifecycle()

    if (startDestination == null) {
        LoadingState(modifier = modifier)
        return
    }

    AnpNavGraph(
        navController = navController,
        startDestination = startDestination!!,
        darkTheme = darkTheme,
        onToggleTheme = onToggleTheme,
        modifier = modifier,
    )
}

@Composable
fun AnpNavGraph(
    navController: NavHostController,
    startDestination: String,
    darkTheme: Boolean,
    onToggleTheme: () -> Unit,
    modifier: Modifier = Modifier,
) {
    val navBackStackEntry by navController.currentBackStackEntryAsState()
    val currentRoute = navBackStackEntry?.destination?.route
    val returnToCommunity: () -> Unit = {
        navController.navigate(Routes.COMMUNITY) {
            popUpTo(navController.graph.findStartDestination().id)
            launchSingleTop = true
        }
    }

    Scaffold(
        modifier = modifier,
        contentWindowInsets = WindowInsets(0, 0, 0, 0),
        bottomBar = {
            AbastevoBottomBar(
                currentRoute = currentRoute,
                onNavigateToTab = navController::navigateToTabBase,
            )
        },
    ) { innerPadding ->
        NavHost(
            navController = navController,
            startDestination = startDestination,
            // The shell already reserves the bottom bar and its system safe area.
            // Consume that padding so child scaffolds do not reserve it again.
            modifier = Modifier.padding(innerPadding).consumeWindowInsets(innerPadding),
        ) {
        composable(Routes.ONBOARDING) {
            OnboardingScreen(
                onNavigateToHome = {
                    navController.navigate(Routes.HOME) {
                        popUpTo(Routes.ONBOARDING) { inclusive = true }
                    }
                },
                onNavigateToLocation = {
                    navController.navigate(Routes.LOCATION) {
                        popUpTo(Routes.ONBOARDING) { inclusive = true }
                    }
                },
            )
        }
        composable(Routes.HOME) {
            HomeScreen(
                darkTheme = darkTheme,
                onToggleTheme = onToggleTheme,
                onNavigate = navController::navigate,
            )
        }
        composable(Routes.COMMUNITY) {
            CommunityScreen(
                darkTheme = darkTheme,
                onToggleTheme = onToggleTheme,
                onNavigate = navController::navigate,
            )
        }
        composable(Routes.PROFILE) {
            ProfileScreen(
                darkTheme = darkTheme,
                onToggleTheme = onToggleTheme,
                onNavigate = navController::navigate,
            )
        }
        composable(Routes.GAMES) {
            com.anpfuel.app.games.GamesScreen(onNavigateBack = { navController.popBackStack() })
        }
        composable(Routes.LOCATION) {
            LocationPickerScreen(
                onNavigateBack = { navController.popBackStack() },
                onNavigateToHome = {
                    navController.navigate(Routes.HOME) {
                        popUpTo(Routes.HOME) { inclusive = true }
                        launchSingleTop = true
                    }
                },
            )
        }
        composable(Routes.HISTORY) {
            HistoryScreen(onNavigateBack = { navController.popBackStack() })
        }
        composable(Routes.STATION_MANAGEMENT, arguments = listOf(navArgument("stationId") { type = NavType.StringType })) {
            com.anpfuel.app.ui.stationprofile.StationManagementScreen(
                onBack = { navController.popBackStack() }, onSignIn = { navController.navigate(Routes.AUTH) },
            )
        }
        composable(Routes.STATION_CLAIM, arguments = listOf(navArgument("stationId") { type = NavType.StringType })) {
            com.anpfuel.app.ui.stationprofile.StationClaimScreen(
                onBack = { navController.popBackStack() },
                onSignIn = { navController.navigate(Routes.AUTH) },
                onManage = { navController.navigate(Routes.stationManagement(it)) },
            )
        }
        composable(Routes.STATION_PAGE, arguments = listOf(
            navArgument("stationKey") { type = NavType.StringType },
            navArgument("fuelProduct") { type = NavType.StringType; defaultValue = "GASOLINE_REGULAR" },
        )) {
            com.anpfuel.app.ui.stations.StationPageScreen(
                onBack = { navController.popBackStack() },
                onSignIn = { navController.navigate(Routes.AUTH) },
                onUpdatePrice = { stationId, fuel -> navController.navigate(Routes.capture(stationId, fuel)) },
                onQuality = {stationId,fuel -> navController.navigate(Routes.stationQuality(stationId,fuel))},
            )
        }
        composable(Routes.STATION_QUALITY,arguments=listOf(
            navArgument("stationId"){type=NavType.StringType},
            navArgument("fuelProduct"){type=NavType.StringType;defaultValue="GASOLINE_REGULAR"},
        )) {
            com.anpfuel.app.ui.quality.QualityScreen(onBack={navController.popBackStack()},onReports={station,fuel,revision,sample ->
                navController.navigate(Routes.qualityReports(station,fuel,revision,sample))
            })
        }
        composable(Routes.QUALITY_REPORTS,arguments=listOf(
            navArgument("stationId"){type=NavType.StringType},
            navArgument("fuelProduct"){type=NavType.StringType;defaultValue="GASOLINE_REGULAR"},
            navArgument("revisionId"){type=NavType.StringType;defaultValue=""},
            navArgument("sampleRef"){type=NavType.StringType;defaultValue=""},
        )) {
            com.anpfuel.app.ui.quality.QualityReportsScreen(onBack={navController.popBackStack()},onSignIn={navController.navigate(Routes.AUTH)})
        }
        composable(Routes.OCCURRENCE_EXPLORER) {
            com.anpfuel.app.ui.occurrence.OccurrenceExplorerScreen(
                onBack = { navController.popBackStack() },
                onDetail = { recordId -> navController.navigate(Routes.occurrenceDetail(recordId)) },
            )
        }
        composable(Routes.OCCURRENCE_DETAIL, arguments = listOf(navArgument("recordId") { type = NavType.StringType })) {
            val recordId = it.arguments?.getString("recordId").orEmpty()
            com.anpfuel.app.ui.occurrence.OccurrenceDetailScreen(recordId = recordId, onBack = { navController.popBackStack() })
        }
        composable(Routes.STATION_PROFILE, arguments = listOf(navArgument("stationId") { type = NavType.StringType })) {
            com.anpfuel.app.ui.stationprofile.StationProfileScreen(
                onBack = { navController.popBackStack() },
                onClaim = { navController.navigate(Routes.stationClaim(it)) },
            )
        }
        composable(Routes.STATIONS) {
            StationsScreen(
                onNavigateBack = { navController.popBackStack() },
                onSuggestStation = { navController.navigate(Routes.SUGGEST) },
                onOpenMasterExplore = { navController.navigate(Routes.MASTER_EXPLORE) },
                onOpenStation = { stationKey, fuel -> navController.navigate(Routes.stationPage(stationKey, fuel)) },
            )
        }
        composable(
            route = Routes.STATIONS_WITH_FUEL,
            arguments = listOf(
                navArgument("fuelProduct") { type = NavType.StringType },
            ),
        ) {
            StationsScreen(
                onNavigateBack = { navController.popBackStack() },
                onSuggestStation = { navController.navigate(Routes.SUGGEST) },
                onOpenMasterExplore = { navController.navigate(Routes.MASTER_EXPLORE) },
                onOpenStation = { stationKey, fuel -> navController.navigate(Routes.stationPage(stationKey, fuel)) },
            )
        }
        composable(Routes.MASTER_EXPLORE) {
            com.anpfuel.app.ui.master.MasterExploreScreen(
                onNavigateBack = { navController.popBackStack() },
                onOpenOccurrences = { navController.navigate(Routes.OCCURRENCE_EXPLORER) },
            )
        }
        composable(Routes.VEHICLES) {
            VehicleScreen(
                onNavigateBack = { navController.popBackStack() },
                onOpenEntries = { vehicleId ->
                    navController.navigate(Routes.vehicleEntries(vehicleId.value))
                },
                onOpenCharging = { vehicleId ->
                    navController.navigate(Routes.chargingExplore(vehicleId.value))
                },
            )
        }
        composable(
            route = Routes.CHARGING_EXPLORE,
            arguments = listOf(
                navArgument("vehicleId") { type = NavType.StringType },
            ),
        ) { backStackEntry ->
            val vehicleId = backStackEntry.arguments?.getString("vehicleId").orEmpty()
            com.anpfuel.app.ui.charging.ChargingExploreScreen(
                vehicleId = vehicleId,
                onNavigateBack = { navController.popBackStack() },
            )
        }
        composable(
            route = Routes.VEHICLE_ENTRIES,
            arguments = listOf(
                navArgument("vehicleId") { type = NavType.StringType },
            ),
        ) { backStackEntry ->
            val vehicleId = backStackEntry.arguments?.getString("vehicleId").orEmpty()
            com.anpfuel.app.ui.ledger.LedgerEntriesScreen(
                vehicleId = vehicleId,
                onNavigateBack = { navController.popBackStack() },
                onOpenMonthly = { id -> navController.navigate(Routes.monthlyIndicators(id)) },
            )
        }
        composable(
            route = Routes.MONTHLY_INDICATORS,
            arguments = listOf(
                navArgument("vehicleId") { type = NavType.StringType },
            ),
        ) { backStackEntry ->
            val vehicleId = backStackEntry.arguments?.getString("vehicleId").orEmpty()
            com.anpfuel.app.ui.monthly.MonthlyIndicatorsScreen(
                vehicleId = vehicleId,
                onNavigateBack = { navController.popBackStack() },
            )
        }
        composable(
            route = "${Routes.AUTH}?provider={provider}&id_token={idToken}&nonce={nonce}&state={state}",
            arguments = listOf(
                navArgument("provider") { defaultValue = "" },
                navArgument("idToken") { defaultValue = "" },
                navArgument("nonce") { defaultValue = "" },
                navArgument("state") { defaultValue = "" },
            ),
            deepLinks = listOf(
                navDeepLink {
                    uriPattern =
                        "anpfuel://auth/callback?provider={provider}&id_token={idToken}&nonce={nonce}&state={state}"
                },
            ),
        ) { entry ->
            AuthRoute(
                providerArg = entry.arguments?.getString("provider").orEmpty(),
                idTokenArg = entry.arguments?.getString("idToken").orEmpty(),
                nonceArg = entry.arguments?.getString("nonce").orEmpty(),
                stateArg = entry.arguments?.getString("state").orEmpty(),
                onNavigateBack = { navController.popBackStack() },
            )
        }
        composable(Routes.ACCOUNT) {
            AccountScreen(
                onNavigateBack = { navController.popBackStack() },
                onNavigateToAuth = { navController.navigate(Routes.AUTH) },
            )
        }
        composable(Routes.SETTINGS) {
            SettingsScreen(
                onNavigateBack = { navController.popBackStack() },
                onNavigateToAuth = { navController.navigate(Routes.AUTH) },
                onNavigateToPrivacy = { navController.navigate(Routes.PRIVACY) },
                onNavigateToOnboarding = {
                    navController.navigate(Routes.ONBOARDING) {
                        popUpTo(Routes.HOME) { inclusive = true }
                    }
                },
                onNavigateToWeekPicker = {
                    navController.navigate(Routes.WEEK_PICKER)
                },
            )
        }
        composable(Routes.CONTRIBUTIONS) {
            com.anpfuel.app.ui.profile.MyContributionsScreen(
                onNavigateBack = { navController.popBackStack() },
                onContribute = { navController.navigate(Routes.CAPTURE) },
            )
        }
        composable(Routes.CAPTURE) {
            CaptureScreen(onNavigateBack = { navController.popBackStack() }, onReturnCommunity = returnToCommunity, onChooseCity = { navController.navigate(Routes.LOCATION) })
        }
        composable(Routes.SUGGEST) {
            com.anpfuel.app.ui.suggest.SuggestStationScreen(
                onNavigateBack = { navController.popBackStack() },
            )
        }
        composable(
            route = Routes.CAPTURE_WITH_TARGET,
            arguments = listOf(
                navArgument("stationId") { defaultValue = "" },
                navArgument("fuel") { defaultValue = "" },
            ),
        ) { entry ->
            CaptureScreen(
                onChooseCity = { navController.navigate(Routes.LOCATION) },
                onReturnCommunity = returnToCommunity,
                onNavigateBack = { navController.popBackStack() },
                stationId = entry.arguments?.getString("stationId")?.takeIf { it.isNotEmpty() },
                fuelProductWire = entry.arguments?.getString("fuel")?.takeIf { it.isNotEmpty() },
            )
        }
        composable(Routes.HELP) {
            HelpScreen(onNavigateBack = { navController.popBackStack() })
        }
        composable(Routes.PRIVACY) {
            PrivacyScreen(
                onNavigateBack = { navController.popBackStack() },
                onNavigateToAuth = { navController.navigate(Routes.AUTH) },
            )
        }
        composable(Routes.WEEK_PICKER) {
            val canNavigateBack = navController.previousBackStackEntry != null
            WeekPickerRoute(
                onNavigateBack = if (canNavigateBack) {
                    { navController.popBackStack() }
                } else {
                    null
                },
                onWeekSelected = {
                    if (canNavigateBack) {
                        navController.popBackStack()
                    } else {
                        navController.navigate(Routes.HOME) {
                            popUpTo(Routes.WEEK_PICKER) { inclusive = true }
                            launchSingleTop = true
                        }
                    }
                },
            )
        }
    }
}
}

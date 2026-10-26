package com.anpfuel.app.navigation

import androidx.compose.animation.EnterTransition
import androidx.compose.animation.ExitTransition
import androidx.compose.animation.fadeIn
import androidx.compose.animation.fadeOut
import androidx.compose.foundation.background
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.WindowInsets
import androidx.compose.foundation.layout.consumeWindowInsets
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.padding
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.Scaffold
import androidx.compose.runtime.Composable
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.saveable.rememberSaveable
import androidx.compose.runtime.setValue
import androidx.compose.ui.res.stringResource
import androidx.compose.ui.unit.dp
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
import com.anpfuel.app.R
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
import com.anpfuel.app.ui.premium.PremiumManagementScreen
import com.anpfuel.app.ui.premium.PremiumScreen
import com.anpfuel.app.ui.privacy.PrivacyScreen
import com.anpfuel.app.ui.profile.ProfileScreen
import com.anpfuel.app.ui.driver.DriverHubScreen
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
    val mapDiscovery: com.anpfuel.app.ui.map.MapDiscoveryViewModel = hiltViewModel()
    val isMapRoute = currentRoute == Routes.MAP || currentRoute == Routes.MAP_PLAN
    var mapHostStarted by rememberSaveable { mutableStateOf(isMapRoute) }
    var mapReady by remember { mutableStateOf(false) }
    var mapBottomOverlayInset by remember { mutableStateOf(164.dp) }
    LaunchedEffect(isMapRoute) {
        if (isMapRoute) mapHostStarted = true
    }
    val mapRecenter = stringResource(R.string.map_recenter)
    val mapInitError = stringResource(R.string.map_unavailable)
    val mapRetry = stringResource(R.string.map_retry)
    val mapLocationDenied = stringResource(R.string.map_host_location_denied)
    val mapGpsOff = stringResource(R.string.map_host_gps_off)
    val mapWaitingFix = stringResource(R.string.map_host_waiting_fix)
    val tabRoutes = setOf(Routes.HOME, Routes.COMMUNITY, Routes.MAP, Routes.PROFILE)

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
        Box(
            modifier = Modifier
                .fillMaxSize()
                .padding(innerPadding)
                .consumeWindowInsets(innerPadding),
        ) {
            if (mapHostStarted) {
                com.anpfuel.maps.engine.MapHost(
                    modifier = Modifier.matchParentSize(),
                    recenterDescription = mapRecenter,
                    initErrorMessage = mapInitError,
                    retryLabel = mapRetry,
                    locationDeniedMessage = mapLocationDenied,
                    gpsOffMessage = mapGpsOff,
                    waitingForFixMessage = mapWaitingFix,
                    onViewportChanged = { lat, lon, radius ->
                        mapDiscovery.onViewport(lat, lon, radius)
                    },
                    onReady = { mapReady = it },
                    active = isMapRoute,
                    recenterBottomInset = mapBottomOverlayInset,
                )
            }
            if (!isMapRoute) {
                Box(
                    Modifier
                        .matchParentSize()
                        .background(MaterialTheme.colorScheme.background),
                )
            }
            NavHost(
                navController = navController,
                startDestination = startDestination,
                modifier = Modifier.fillMaxSize(),
                enterTransition = {
                    if (initialState.destination.route in tabRoutes && targetState.destination.route in tabRoutes) {
                        EnterTransition.None
                    } else {
                        fadeIn()
                    }
                },
                exitTransition = {
                    if (initialState.destination.route in tabRoutes && targetState.destination.route in tabRoutes) {
                        ExitTransition.None
                    } else {
                        fadeOut()
                    }
                },
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
        composable(
            route = Routes.MAP,
            deepLinks = listOf(
                navDeepLink { uriPattern = "anpfuel://map" },
            ),
        ) {
            MapDestination(navController, mapDiscovery, mapReady, onBottomOverlayInsetChanged = { mapBottomOverlayInset = it })
        }
        composable(
            route = Routes.MAP_PLAN,
            arguments = listOf(navArgument("tripId") { type = NavType.StringType }),
        ) { entry ->
            val planViewModel: com.anpfuel.app.ui.map.MapPlanViewModel = hiltViewModel()
            val trip by planViewModel.trip.collectAsStateWithLifecycle()
            val tripId = entry.arguments?.getString("tripId").orEmpty()
            androidx.compose.runtime.LaunchedEffect(tripId) { planViewModel.load(tripId) }
            MapDestination(navController, mapDiscovery, mapReady, trip, onBottomOverlayInsetChanged = { mapBottomOverlayInset = it })
        }
        composable("map/saved") {
            com.anpfuel.app.ui.map.SavedScreen(
                onBack = { navController.popBackStack() },
                onOpenTrip = { navController.navigate(Routes.mapPlan(it)) },
            )
        }
        composable(Routes.MAP_CITY) {
            LocationPickerScreen(
                onNavigateBack = { navController.popBackStack() },
                onNavigateToHome = { navController.popBackStack() },
            )
        }
        composable("map/downloads") {
            com.anpfuel.app.ui.map.MapDownloadsScreen(
                onBack = { navController.popBackStack() },
            )
        }
        composable(
            route = Routes.MAP_CHARGING_COMMUNITY,
            arguments = listOf(
                navArgument("placeId") { type = NavType.StringType },
            ),
        ) {
            com.anpfuel.app.ui.charging.ChargingCommunityScreen(
                onNavigateBack = { navController.popBackStack() },
            )
        }
        composable(
            route = Routes.MAP_STATION_PAGE,
            arguments = listOf(
                navArgument("stationKey") { type = NavType.StringType },
                navArgument("fuelProduct") { type = NavType.StringType; defaultValue = "GASOLINE_REGULAR" },
            ),
            deepLinks = listOf(
                navDeepLink { uriPattern = "anpfuel://map/station/{stationKey}?fuelProduct={fuelProduct}" },
            ),
        ) {
            com.anpfuel.app.ui.stations.StationPageScreen(
                onBack = { navController.popBackStack() },
                onSignIn = { navController.navigate(Routes.AUTH) },
                onUpdatePrice = { stationId, fuel -> navController.navigate(Routes.capture(stationId, fuel)) },
                onQuality = { stationId, fuel -> navController.navigate(Routes.stationQuality(stationId, fuel)) },
            )
        }
        composable(Routes.PROFILE) {
            ProfileScreen(
                darkTheme = darkTheme,
                onToggleTheme = onToggleTheme,
                onNavigate = navController::navigate,
                onOpenPremium = {
                    navController.navigate(Routes.PREMIUM) { launchSingleTop = true }
                },
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
                onOpenCharging = { vehicleId -> navController.navigate(Routes.chargingExplore(vehicleId)) },
                onOpenVehicles = { navController.navigate(Routes.VEHICLES) },
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
                onOpenDriverHub = { navController.navigate(Routes.DRIVER_HUB) },
            )
        }
        composable(Routes.DRIVER_HUB) {
            DriverHubScreen(
                onNavigateBack = { navController.popBackStack() },
                onOpenVehicles = { navController.navigate(Routes.VEHICLES) },
                onOpenEntries = { vehicleId -> navController.navigate(Routes.vehicleEntries(vehicleId)) },
                onOpenMonthly = { vehicleId -> navController.navigate(Routes.monthlyIndicators(vehicleId)) },
                onOpenRefuel = { vehicleId -> navController.navigate(Routes.vehicleEntriesAction(vehicleId, "refuel")) },
                onOpenCharge = { vehicleId -> navController.navigate(Routes.vehicleEntriesAction(vehicleId, "charge")) },
                onOpenPremium = { navController.navigate(Routes.PREMIUM) { launchSingleTop = true } },
                onSignIn = { navController.navigate(Routes.AUTH) },
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
                onOpenCommunity = { placeId -> navController.navigate(Routes.chargingCommunity(placeId)) },
            )
        }
        composable(
            route = Routes.CHARGING_COMMUNITY,
            arguments = listOf(
                navArgument("placeId") { type = NavType.StringType },
            ),
        ) {
            com.anpfuel.app.ui.charging.ChargingCommunityScreen(
                onNavigateBack = { navController.popBackStack() },
            )
        }
        composable(
            route = Routes.VEHICLE_ENTRIES_ACTION,
            arguments = listOf(
                navArgument("vehicleId") { type = NavType.StringType },
                navArgument("action") { type = NavType.StringType; defaultValue = "" },
            ),
        ) { backStackEntry ->
            val vehicleId = backStackEntry.arguments?.getString("vehicleId").orEmpty()
            val action = backStackEntry.arguments?.getString("action").orEmpty()
            com.anpfuel.app.ui.ledger.LedgerEntriesScreen(
                vehicleId = vehicleId,
                onNavigateBack = { navController.popBackStack() },
                onOpenMonthly = { id -> navController.navigate(Routes.monthlyIndicators(id)) },
                initialDataset = when (action) {
                    "refuel" -> com.anpfuel.domain.premium.LedgerDataset.REFUEL
                    "charge" -> com.anpfuel.domain.premium.LedgerDataset.CHARGE
                    else -> null
                },
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
        composable(Routes.LEDGER_SYNC) {
            com.anpfuel.app.ui.sync.LedgerSyncScreen(onNavigateBack = { navController.popBackStack() })
        }
        composable(Routes.ACCOUNT) {
            AccountScreen(
                onNavigateToLedgerSync = { navController.navigate(Routes.LEDGER_SYNC) },
                onNavigateBack = { navController.popBackStack() },
                onNavigateToAuth = { navController.navigate(Routes.AUTH) },
                onOpenPremium = {
                    navController.navigate(Routes.PREMIUM) { launchSingleTop = true }
                },
            )
        }
        composable(Routes.PREMIUM) {
            PremiumScreen(
                onBack = { navController.popBackStack() },
                onSignIn = { navController.navigate(Routes.AUTH) },
                onManageSubscription = {
                    navController.navigate(Routes.PREMIUM_MANAGEMENT) { launchSingleTop = true }
                },
            )
        }
        composable(Routes.PREMIUM_MANAGEMENT) {
            PremiumManagementScreen(
                onBack = { navController.popBackStack() },
                onSignIn = { navController.navigate(Routes.AUTH) },
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

}

@Composable
private fun MapDestination(
    navController: NavHostController,
    discovery: com.anpfuel.app.ui.map.MapDiscoveryViewModel,
    mapReady: Boolean,
    initialPlan: com.anpfuel.domain.map.saved.SavedTrip? = null,
    onBottomOverlayInsetChanged: (androidx.compose.ui.unit.Dp) -> Unit,
) {
        val sheet: com.anpfuel.app.ui.map.MapSheetViewModel = hiltViewModel()
        com.anpfuel.app.ui.map.MapScreen(
        initialPlan = initialPlan,
        mapReady = mapReady,
        discovery = discovery,
        mapContent = {},
        onBottomOverlayInsetChanged = onBottomOverlayInsetChanged,
            onOpenCity = { navController.navigate(Routes.MAP_CITY) },
            periodContent = { changed ->
                com.anpfuel.app.ui.weekpicker.SurveyWeekChipAction(onWeekChanged = changed)
            },
            onOpenStation = { stationKey, fuel ->
                // Map origin keeps the MAP tab selected on return.
                try {
                    if (fuel == "CHARGING") {
                        navController.navigate(Routes.mapChargingCommunity(stationKey))
                    } else {
                        val product = com.anpfuel.domain.valueobject.FuelProduct.valueOf(fuel)
                        navController.navigate(Routes.mapStationPage(stationKey, product))
                    }
                } catch (_: Exception) {
                    navController.navigate(Routes.STATIONS)
                }
            },
            onOpenSearch = { navController.navigate(Routes.STATIONS) },
            onOpenDownloads = { navController.navigate("map/downloads") },
            onOpenSaved = { navController.navigate("map/saved") },
            onReportStation = { stationId, fuel ->
                try {
                    if (fuel == "CHARGING") {
                        navController.navigate(Routes.mapChargingCommunity(stationId))
                    } else {
                        require(com.anpfuel.domain.discovery.StationPageIdentity.parse(stationId) != null)
                        val wireFuel = runCatching {
                            com.anpfuel.data.mapper.WireFuelMapper.toWire(
                                com.anpfuel.domain.valueobject.FuelProduct.valueOf(fuel),
                            )
                        }.getOrDefault(fuel)
                        navController.navigate(Routes.capture(stationId, wireFuel))
                    }
                } catch (_: Exception) {
                    navController.navigate(Routes.CAPTURE)
                }
            },
            sheet = sheet,
            planner = hiltViewModel(),
            saved = hiltViewModel(),
        )
}

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
import com.anpfuel.app.ui.prices.PricesScreen
import com.anpfuel.app.ui.profile.ProfileScreen
import com.anpfuel.app.ui.search.SearchScreen
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
                onNavigateToTab = { targetRoute ->
                    if (currentRoute != targetRoute) {
                        navController.navigate(targetRoute) {
                            popUpTo(navController.graph.findStartDestination().id) {
                                saveState = true
                            }
                            launchSingleTop = true
                            restoreState = true
                        }
                    }
                },
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
        composable(Routes.SEARCH) {
            SearchScreen(
                onNavigateBack = { navController.popBackStack() },
                onNavigateToHome = {
                    navController.navigate(Routes.HOME) {
                        popUpTo(Routes.HOME) { inclusive = true }
                        launchSingleTop = true
                    }
                },
                onNavigateToLocationPicker = {
                    navController.navigate(Routes.LOCATION)
                },
            )
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
        composable(Routes.PRICES) {
            PricesScreen(
                onNavigateBack = { navController.popBackStack() },
                onNavigate = navController::navigate
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
            )
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
                onOpenStation = { stationKey, fuel -> navController.navigate(Routes.stationPage(stationKey, fuel)) },
            )
        }
        composable(Routes.VEHICLES) {
            VehicleScreen(
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
            CaptureScreen(onNavigateBack = { navController.popBackStack() }, onReturnCommunity = returnToCommunity)
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
                onReturnCommunity = returnToCommunity,
                onNavigateBack = { navController.popBackStack() },
                stationId = entry.arguments?.getString("stationId")?.takeIf { it.isNotEmpty() },
                fuelProductWire = entry.arguments?.getString("fuel")?.takeIf { it.isNotEmpty() },
            )
        }
        composable(Routes.HELP) {
            HelpScreen(onNavigateBack = { navController.popBackStack() })
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

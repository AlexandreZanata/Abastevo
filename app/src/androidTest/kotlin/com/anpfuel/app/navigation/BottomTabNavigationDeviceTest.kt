package com.anpfuel.app.navigation

import androidx.activity.ComponentActivity
import androidx.compose.foundation.layout.Column
import androidx.compose.material3.Text
import androidx.compose.runtime.getValue
import androidx.compose.ui.Modifier
import androidx.compose.ui.test.junit4.createAndroidComposeRule
import androidx.compose.ui.test.onNodeWithContentDescription
import androidx.compose.ui.test.performClick
import androidx.navigation.NavHostController
import androidx.navigation.compose.NavHost
import androidx.navigation.compose.composable
import androidx.navigation.compose.currentBackStackEntryAsState
import androidx.navigation.compose.rememberNavController
import androidx.test.ext.junit.runners.AndroidJUnit4
import com.anpfuel.app.ui.theme.AnpFuelTheme
import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Rule
import org.junit.Test
import org.junit.runner.RunWith

/** Real controller/bar regression harness; no app repositories, account or network. */
@RunWith(AndroidJUnit4::class)
class BottomTabNavigationDeviceTest {
    @get:Rule
    val compose = createAndroidComposeRule<ComponentActivity>()

    private lateinit var controller: NavHostController

    private fun launch(startDestination: String = Routes.HOME) {
        compose.setContent {
            controller = rememberNavController()
            val entry by controller.currentBackStackEntryAsState()
            AnpFuelTheme(dynamicColor = false) {
                Column {
                    NavHost(controller, startDestination, Modifier.weight(1f)) {
                        listOf(
                            Routes.HOME, Routes.COMMUNITY, Routes.MAP, Routes.PROFILE,
                            Routes.LOCATION, Routes.CONTRIBUTIONS,
                            Routes.SETTINGS, Routes.HELP, Routes.ONBOARDING,
                        ).forEach { route ->
                            composable(route) { Text(route) }
                        }
                    }
                    AbastevoBottomBar(
                        currentRoute = entry?.destination?.route,
                        onNavigateToTab = controller::navigateToTabBase,
                    )
                }
            }
        }
        compose.waitForIdle()
    }

    private fun open(route: String) {
        compose.runOnIdle { controller.navigate(route) }
    }

    private fun tap(tab: NavigationTab) {
        val label = compose.activity.getString(tab.titleRes)
        compose.onNodeWithContentDescription(label).performClick()
        compose.runOnIdle { assertEquals(tab.route, controller.currentDestination?.route) }
    }

    @Test
    fun exploreReturnsHomeFromLocationAndDeeperSettings() {
        launch()
        open(Routes.LOCATION)
        tap(NavigationTab.EXPLORE)
        open(Routes.CONTRIBUTIONS)
        open(Routes.LOCATION)
        tap(NavigationTab.EXPLORE)
        compose.runOnIdle { assertFalse(controller.popBackStack()) }
    }

    @Test
    fun communityAndProfileReturnToBaseWithoutChildBackHistory() {
        launch()
        tap(NavigationTab.COMMUNITY)
        open(Routes.CONTRIBUTIONS)
        tap(NavigationTab.COMMUNITY)
        compose.runOnIdle {
            controller.popBackStack()
            assertEquals(Routes.HOME, controller.currentDestination?.route)
        }
        tap(NavigationTab.PROFILE)
        open(Routes.SETTINGS)
        open(Routes.HELP)
        tap(NavigationTab.PROFILE)
        compose.runOnIdle {
            controller.popBackStack()
            assertEquals(Routes.HOME, controller.currentDestination?.route)
        }
    }

    @Test
    fun switchingTabsDoesNotRestorePreviouslyVisitedChildren() {
        launch()
        open(Routes.LOCATION)
        tap(NavigationTab.COMMUNITY)
        open(Routes.CONTRIBUTIONS)
        tap(NavigationTab.PROFILE)
        open(Routes.SETTINGS)
        tap(NavigationTab.EXPLORE)
        tap(NavigationTab.COMMUNITY)
        tap(NavigationTab.PROFILE)
        tap(NavigationTab.EXPLORE)
    }

    @Test
    fun repeatedTapsDoNotDuplicateAnyBasePage() {
        launch()
        NavigationTab.entries.forEach { tab ->
            repeat(3) { tap(tab) }
            if (tab != NavigationTab.EXPLORE) {
                compose.runOnIdle {
                    controller.popBackStack()
                    assertEquals(Routes.HOME, controller.currentDestination?.route)
                }
            }
        }
        compose.runOnIdle { assertFalse(controller.popBackStack()) }
    }

    @Test
    fun locationAsInitialDestinationCanReachEveryTabBase() {
        launch(startDestination = Routes.LOCATION)
        NavigationTab.entries.forEach(::tap)
    }
}

package com.anpfuel.app.navigation

import com.anpfuel.app.R
import org.junit.jupiter.api.Assertions.assertEquals
import org.junit.jupiter.api.Assertions.assertNull
import org.junit.jupiter.api.Assertions.assertTrue
import org.junit.jupiter.api.Test

class NavigationTabTest {

    @Test
    fun containsExactlyThreeTabsPerFrozenInformationArchitecture() {
        assertEquals(3, NavigationTab.entries.size)
        assertEquals(
            listOf(NavigationTab.EXPLORE, NavigationTab.COMMUNITY, NavigationTab.PROFILE),
            NavigationTab.entries,
        )
    }

    @Test
    fun exploreTabMapsToHomeRouteAndTitle() {
        assertEquals(Routes.HOME, NavigationTab.EXPLORE.route)
        assertEquals(R.string.nav_explore, NavigationTab.EXPLORE.titleRes)
    }

    @Test
    fun communityTabMapsToCommunityRouteAndTitle() {
        assertEquals(Routes.COMMUNITY, NavigationTab.COMMUNITY.route)
        assertEquals(R.string.nav_community, NavigationTab.COMMUNITY.titleRes)
    }

    @Test
    fun profileTabMapsToProfileRouteAndTitle() {
        assertEquals(Routes.PROFILE, NavigationTab.PROFILE.route)
        assertEquals(R.string.nav_profile, NavigationTab.PROFILE.titleRes)
    }

    @Test
    fun fromRouteResolvesTopLevelTabs() {
        assertEquals(NavigationTab.EXPLORE, NavigationTab.fromRoute(Routes.HOME))
        assertEquals(NavigationTab.EXPLORE, NavigationTab.fromRoute(Routes.EXPLORE))
        assertEquals(NavigationTab.COMMUNITY, NavigationTab.fromRoute(Routes.COMMUNITY))
        assertEquals(NavigationTab.PROFILE, NavigationTab.fromRoute(Routes.PROFILE))
    }

    @Test
    fun stationPageKeepsExploreSelected() {
        assertEquals(NavigationTab.EXPLORE, NavigationTab.fromRoute(Routes.STATION_PAGE))
    }

    @Test
    fun fromRouteReturnsNullForNonTopLevelRoutes() {
        assertNull(NavigationTab.fromRoute(Routes.SEARCH))
        assertNull(NavigationTab.fromRoute(Routes.LOCATION))
        assertNull(NavigationTab.fromRoute(Routes.PRICES))
        assertNull(NavigationTab.fromRoute(Routes.HISTORY))
        assertNull(NavigationTab.fromRoute(Routes.STATIONS))
        assertNull(NavigationTab.fromRoute(Routes.VEHICLES))
        assertNull(NavigationTab.fromRoute(Routes.SETTINGS))
        assertNull(NavigationTab.fromRoute(Routes.WEEK_PICKER))
        assertNull(NavigationTab.fromRoute(Routes.CAPTURE))
        assertNull(NavigationTab.fromRoute(Routes.AUTH))
        assertNull(NavigationTab.fromRoute(null))
    }
}

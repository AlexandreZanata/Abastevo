package com.anpfuel.app.navigation

import com.anpfuel.app.R
import org.junit.jupiter.api.Assertions.assertEquals
import org.junit.jupiter.api.Assertions.assertNull
import org.junit.jupiter.api.Assertions.assertTrue
import org.junit.jupiter.api.Test

class NavigationTabTest {

    @Test
    fun containsExactlyFourTabsPerMapInformationArchitecture() {
        assertEquals(4, NavigationTab.entries.size)
        assertEquals(
            listOf(NavigationTab.EXPLORE, NavigationTab.COMMUNITY, NavigationTab.MAP, NavigationTab.PROFILE),
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
    fun mapTabMapsToMapRouteAndTitle() {
        assertEquals(Routes.MAP, NavigationTab.MAP.route)
        assertEquals("map", Routes.MAP)
        assertEquals(R.string.nav_map, NavigationTab.MAP.titleRes)
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
        assertEquals(NavigationTab.MAP, NavigationTab.fromRoute(Routes.MAP))
        assertEquals(NavigationTab.PROFILE, NavigationTab.fromRoute(Routes.PROFILE))
    }

    @Test
    fun stationPageKeepsExploreSelected() {
        assertEquals(NavigationTab.EXPLORE, NavigationTab.fromRoute(Routes.STATION_PAGE))
    }

    @Test
    fun mapStationPageKeepsMapSelected() {
        assertEquals(NavigationTab.MAP, NavigationTab.fromRoute(Routes.MAP_STATION_PAGE))
        assertEquals(NavigationTab.MAP, NavigationTab.fromRoute("map/station/abc?fuelProduct=GASOLINE_REGULAR"))
    }

    @Test
    fun premiumPagesKeepProfileSelected() {
        assertEquals(NavigationTab.PROFILE, NavigationTab.fromRoute(Routes.PREMIUM))
        assertEquals(NavigationTab.PROFILE, NavigationTab.fromRoute(Routes.PREMIUM_MANAGEMENT))
    }

    @Test
    fun fromRouteReturnsNullForNonTopLevelRoutes() {
        assertNull(NavigationTab.fromRoute(Routes.GAMES))
        assertNull(NavigationTab.fromRoute(Routes.LOCATION))
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

package com.anpfuel.app.navigation

import androidx.navigation.NavGraph.Companion.findStartDestination
import androidx.navigation.NavHostController

/** Bottom-bar actions reuse a base entry and its state, never a child stack. */
internal fun NavHostController.navigateToTabBase(targetRoute: String) {
    if (currentDestination?.route == targetRoute) return
    if (popBackStack(targetRoute, inclusive = false)) return
    // Onboarding and the first week picker can remove the graph's start entry.
    val baseId = try {
        getBackStackEntry(Routes.HOME).destination.id
    } catch (_: IllegalArgumentException) {
        graph.findStartDestination().id
    }
    navigate(targetRoute) {
        popUpTo(baseId)
        launchSingleTop = true
        restoreState = false
    }
}

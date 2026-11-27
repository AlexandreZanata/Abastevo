package com.anpfuel.app.navigation

import androidx.navigation.NavGraph.Companion.findStartDestination
import androidx.navigation.NavHostController

/** Bottom-bar actions reuse a base entry and its state, never a child stack. */
internal fun NavHostController.navigateToTabBase(targetRoute: String) {
    if (currentDestination?.route == targetRoute) return
    if (popBackStack(targetRoute, inclusive = false)) return
    navigate(targetRoute) {
        popUpTo(graph.findStartDestination().id)
        launchSingleTop = true
        restoreState = false
    }
}

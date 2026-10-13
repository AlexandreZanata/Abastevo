package com.anpfuel.app.navigation

import androidx.navigation.NavGraph.Companion.findStartDestination
import androidx.navigation.NavHostController

/** Bottom-bar actions always open a base page, without restoring a child stack. */
internal fun NavHostController.navigateToTabBase(targetRoute: String) {
    navigate(targetRoute) {
        popUpTo(graph.findStartDestination().id)
        launchSingleTop = true
        restoreState = false
    }
}

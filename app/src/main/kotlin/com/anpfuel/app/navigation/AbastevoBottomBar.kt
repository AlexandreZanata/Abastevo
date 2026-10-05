package com.anpfuel.app.navigation

import androidx.compose.foundation.layout.height
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.filled.AccountCircle
import androidx.compose.material.icons.filled.AddAPhoto
import androidx.compose.material.icons.filled.Explore
import androidx.compose.material.icons.filled.Groups
import androidx.compose.material3.ExtendedFloatingActionButton
import androidx.compose.material3.Icon
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.NavigationBar
import androidx.compose.material3.NavigationBarItem
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.ui.Modifier
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.graphics.vector.ImageVector
import androidx.compose.ui.res.stringResource
import androidx.compose.ui.semantics.contentDescription
import androidx.compose.ui.semantics.semantics
import androidx.compose.ui.unit.dp
import com.anpfuel.app.R
import com.anpfuel.app.ui.theme.AbastevoActionBlue

/**
 * P19-T04: Top-level navigation tabs.
 *
 * Three tabs per frozen IA (P19-T02):
 * - Explorar (Routes.HOME): fuel & station discovery
 * - Comunidade (Routes.COMMUNITY): community activity, verification, discussions
 * - Perfil (Routes.PROFILE): free account, rights/privacy, preserved expert tools
 */
enum class NavigationTab(
    val route: String,
    val titleRes: Int,
    val icon: ImageVector,
) {
    EXPLORE(Routes.HOME, R.string.nav_explore, Icons.Default.Explore),
    COMMUNITY(Routes.COMMUNITY, R.string.nav_community, Icons.Default.Groups),
    PROFILE(Routes.PROFILE, R.string.nav_profile, Icons.Default.AccountCircle),
    ;

    companion object {
        fun fromRoute(route: String?): NavigationTab? = entries.firstOrNull { it.route == route }
    }
}

/**
 * P19-T04: Bottom navigation bar for top-level app tabs.
 */
@Composable
fun AbastevoBottomBar(
    currentRoute: String?,
    onNavigateToTab: (String) -> Unit,
    modifier: Modifier = Modifier,
) {
    // Compact bar (M3 default is 80.dp): trims the space above the icons
    // while keeping labels visible and 48.dp touch targets intact.
    NavigationBar(
        modifier = modifier.height(72.dp),
        containerColor = MaterialTheme.colorScheme.surface,
        contentColor = MaterialTheme.colorScheme.onSurface,
    ) {
        NavigationTab.entries.forEach { tab ->
            val isSelected = currentRoute == tab.route
            val label = stringResource(tab.titleRes)
            NavigationBarItem(
                selected = isSelected,
                onClick = { onNavigateToTab(tab.route) },
                icon = {
                    Icon(
                        imageVector = tab.icon,
                        contentDescription = label,
                    )
                },
                label = { Text(text = label) },
                alwaysShowLabel = true,
                modifier = Modifier.semantics {
                    contentDescription = label
                },
            )
        }
    }
}

/**
 * P19-T04: Persistent, labeled primary action ("Atualizar preço").
 *
 * It is an action (BUC-C02), not a fourth feed tab or mandatory onboarding step.
 * Uses the approved AbastevoActionBlue brand token with white icon and text,
 * guaranteeing contrast >= 4.5:1.
 */
@Composable
fun AbastevoUpdatePriceFab(
    onClick: () -> Unit,
    modifier: Modifier = Modifier,
) {
    val label = stringResource(R.string.nav_update_price)
    val a11yDesc = stringResource(R.string.a11y_nav_update_price)
    ExtendedFloatingActionButton(
        onClick = onClick,
        icon = {
            Icon(
                imageVector = Icons.Default.AddAPhoto,
                contentDescription = null,
            )
        },
        text = {
            Text(
                text = label,
                style = MaterialTheme.typography.labelLarge,
            )
        },
        containerColor = AbastevoActionBlue,
        contentColor = Color.White,
        modifier = modifier.semantics {
            contentDescription = a11yDesc
        },
    )
}

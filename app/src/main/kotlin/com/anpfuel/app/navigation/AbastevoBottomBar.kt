package com.anpfuel.app.navigation

import androidx.compose.foundation.background
import androidx.compose.foundation.interaction.MutableInteractionSource
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.navigationBarsPadding
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.selection.selectable
import androidx.compose.foundation.selection.selectableGroup
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.filled.AccountCircle
import androidx.compose.material.icons.filled.Explore
import androidx.compose.material.icons.filled.Groups
import androidx.compose.material.icons.filled.Map
import androidx.compose.material.icons.outlined.AccountCircle
import androidx.compose.material.icons.outlined.Explore
import androidx.compose.material.icons.outlined.Groups
import androidx.compose.material.icons.outlined.Map
import androidx.compose.material3.Icon
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.Surface
import androidx.compose.material3.Text
import androidx.compose.material3.ripple
import androidx.compose.runtime.Composable
import androidx.compose.runtime.remember
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.graphics.vector.ImageVector
import androidx.compose.ui.res.stringResource
import androidx.compose.ui.semantics.Role
import androidx.compose.ui.semantics.contentDescription
import androidx.compose.ui.semantics.semantics
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.unit.dp
import com.anpfuel.app.R
import com.anpfuel.app.ui.theme.AbastevoSpacing
import com.anpfuel.app.ui.theme.LocalAbastevoNavColors

/**
 * Top-level navigation tabs.
 *
 * Four tabs per MAP-17 (Explorar | Comunidade | Mapa | Perfil):
 * - Explorar (Routes.HOME): fuel & station discovery
 * - Comunidade (Routes.COMMUNITY): community activity, verification, discussions
 * - Mapa (Routes.MAP): native offline map with ANP/Community prices
 * - Perfil (Routes.PROFILE): free account, rights/privacy, preserved expert tools
 */
enum class NavigationTab(
    val route: String,
    val titleRes: Int,
    val icon: ImageVector,
    val unselectedIcon: ImageVector,
) {
    EXPLORE(Routes.HOME, R.string.nav_explore, Icons.Filled.Explore, Icons.Outlined.Explore),
    COMMUNITY(Routes.COMMUNITY, R.string.nav_community, Icons.Filled.Groups, Icons.Outlined.Groups),
    MAP(Routes.MAP, R.string.nav_map, Icons.Filled.Map, Icons.Outlined.Map),
    PROFILE(Routes.PROFILE, R.string.nav_profile, Icons.Filled.AccountCircle, Icons.Outlined.AccountCircle),
    ;

    companion object {
        fun fromRoute(route: String?): NavigationTab? = when {
            route == null -> null
            route.startsWith("map/") -> MAP
            route.startsWith("station/") -> EXPLORE
            route.startsWith("forum/") -> COMMUNITY
            route == Routes.STATION_PAGE -> EXPLORE
            route == Routes.MAP_STATION_PAGE || route == Routes.MAP_CHARGING_COMMUNITY -> MAP
            route == Routes.PREMIUM || route == Routes.PREMIUM_MANAGEMENT || route == Routes.DRIVER_HUB -> PROFILE
            else -> entries.firstOrNull { it.route == route }
        }
    }
}

private val PillShape = RoundedCornerShape(14.dp)
private val RippleRadius = 36.dp

/**
 * Fixed bottom navigation bar, shown on every screen.
 *
 * Solid brand chrome, extending under the gesture area. The selected tab is
 * told apart three ways: a light pill behind it, a filled icon and a bold
 * label; the others use outlined icons in a muted tone. Item cells keep
 * full-width touch targets; labels always stay visible.
 */
@Composable
fun AbastevoBottomBar(
    currentRoute: String?,
    onNavigateToTab: (String) -> Unit,
    modifier: Modifier = Modifier,
) {
    val colors = LocalAbastevoNavColors.current
    Surface(
        modifier = modifier,
        color = colors.container,
        contentColor = colors.unselected,
    ) {
        Row(
            modifier = Modifier
                .fillMaxWidth()
                .navigationBarsPadding()
                .padding(horizontal = AbastevoSpacing.xs)
                .selectableGroup(),
            horizontalArrangement = Arrangement.SpaceEvenly,
            verticalAlignment = Alignment.CenterVertically,
        ) {
            NavigationTab.entries.forEach { tab ->
                val isSelected = NavigationTab.fromRoute(currentRoute) == tab
                val label = stringResource(tab.titleRes)
                val interactionSource = remember { MutableInteractionSource() }
                Box(
                    contentAlignment = Alignment.Center,
                    modifier = Modifier
                        .weight(1f)
                        .selectable(
                            selected = isSelected,
                            onClick = { onNavigateToTab(tab.route) },
                            role = Role.Tab,
                            interactionSource = interactionSource,
                            indication = ripple(bounded = false, radius = RippleRadius),
                        )
                        .semantics {
                            contentDescription = label
                        }
                        .padding(vertical = AbastevoSpacing.sm),
                ) {
                    Column(
                        horizontalAlignment = Alignment.CenterHorizontally,
                        verticalArrangement = Arrangement.spacedBy(2.dp),
                        modifier = Modifier
                            .background(
                                color = if (isSelected) colors.indicator else Color.Transparent,
                                shape = PillShape,
                            )
                            .padding(horizontal = 10.dp, vertical = 6.dp),
                    ) {
                        Icon(
                            imageVector = if (isSelected) tab.icon else tab.unselectedIcon,
                            contentDescription = null,
                            tint = if (isSelected) colors.selectedIcon else colors.unselected,
                        )
                        Text(
                            text = label,
                            style = MaterialTheme.typography.labelMedium,
                            fontWeight = if (isSelected) FontWeight.Bold else FontWeight.Medium,
                            color = if (isSelected) colors.selectedLabel else colors.unselected,
                            maxLines = 1,
                        )
                    }
                }
            }
        }
    }
}

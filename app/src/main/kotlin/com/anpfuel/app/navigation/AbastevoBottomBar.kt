package com.anpfuel.app.navigation

import androidx.compose.foundation.background
import androidx.compose.foundation.clickable
import androidx.compose.foundation.interaction.MutableInteractionSource
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.navigationBarsPadding
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.selection.selectable
import androidx.compose.foundation.selection.selectableGroup
import androidx.compose.foundation.shape.CircleShape
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.filled.AccountCircle
import androidx.compose.material.icons.filled.AddAPhoto
import androidx.compose.material.icons.filled.Explore
import androidx.compose.material.icons.filled.Groups
import androidx.compose.material3.ExtendedFloatingActionButton
import androidx.compose.material3.Icon
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.Surface
import androidx.compose.material3.Text
import androidx.compose.material3.ripple
import androidx.compose.runtime.Composable
import androidx.compose.runtime.remember
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.draw.clip
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.graphics.vector.ImageVector
import androidx.compose.ui.res.stringResource
import androidx.compose.ui.semantics.Role
import androidx.compose.ui.semantics.contentDescription
import androidx.compose.ui.semantics.semantics
import androidx.compose.ui.unit.dp
import com.anpfuel.app.R
import com.anpfuel.app.ui.theme.AbastevoActionBlue

/**
 * Top-level navigation tabs.
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

/** Extra top padding is kept minimal; bottom mirrors the label gap. */
private val BarTopPadding = 2.dp
private val BarBottomPadding = 8.dp

/**
 * Fixed bottom navigation bar, shown on every screen.
 *
 * Tight layout: minimal space above the icons, the label gap below the
 * text, with the system safe area applied underneath. Item cells keep
 * full-width touch targets; labels always stay visible.
 */
@Composable
fun AbastevoBottomBar(
    currentRoute: String?,
    onNavigateToTab: (String) -> Unit,
    modifier: Modifier = Modifier,
) {
    Surface(
        modifier = modifier,
        color = MaterialTheme.colorScheme.surface,
        contentColor = MaterialTheme.colorScheme.onSurface,
    ) {
        Row(
            modifier = Modifier
                .fillMaxWidth()
                .navigationBarsPadding()
                .padding(top = BarTopPadding, bottom = BarBottomPadding)
                .selectableGroup(),
            horizontalArrangement = Arrangement.SpaceEvenly,
            verticalAlignment = Alignment.CenterVertically,
        ) {
            NavigationTab.entries.forEach { tab ->
                val isSelected = currentRoute == tab.route
                val label = stringResource(tab.titleRes)
                val interactionSource = remember { MutableInteractionSource() }
                Column(
                    horizontalAlignment = Alignment.CenterHorizontally,
                    modifier = Modifier
                        .weight(1f)
                        .selectable(
                            selected = isSelected,
                            onClick = { onNavigateToTab(tab.route) },
                            role = Role.Tab,
                            interactionSource = interactionSource,
                            indication = ripple(bounded = false),
                        )
                        .semantics {
                            contentDescription = label
                        },
                ) {
                    Box(
                        contentAlignment = Alignment.Center,
                        modifier = Modifier
                            .size(width = 64.dp, height = 32.dp)
                            .clip(CircleShape)
                            .background(
                                if (isSelected) {
                                    MaterialTheme.colorScheme.secondaryContainer
                                } else {
                                    MaterialTheme.colorScheme.surface
                                },
                            ),
                    ) {
                        Icon(
                            imageVector = tab.icon,
                            contentDescription = null,
                            tint = if (isSelected) {
                                MaterialTheme.colorScheme.onSecondaryContainer
                            } else {
                                MaterialTheme.colorScheme.onSurfaceVariant
                            },
                        )
                    }
                    Text(
                        text = label,
                        style = MaterialTheme.typography.labelMedium,
                        color = if (isSelected) {
                            MaterialTheme.colorScheme.onSurface
                        } else {
                            MaterialTheme.colorScheme.onSurfaceVariant
                        },
                    )
                }
            }
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
